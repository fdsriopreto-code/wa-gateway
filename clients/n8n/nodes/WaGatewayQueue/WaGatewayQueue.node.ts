import type {
	IExecuteFunctions,
	INodeExecutionData,
	INodeType,
	INodeTypeDescription,
	IDataObject,
} from 'n8n-workflow';
import { NodeOperationError } from 'n8n-workflow';
import { randomUUID } from 'crypto';
import { redisFromCreds, redisKey, COLLECT_LUA } from '../shared/redis';

/**
 * wa-gateway Fila (debounce): agrupa mensagens picadas por contato.
 *
 * Fluxo típico (1 execução por mensagem recebida):
 *   Trigger -> Enfileirar -> Wait 15s -> Coletar
 *     saída "Continuar" = esta era a mensagem mais recente; vem a fila
 *       unificada em `text` / `messages`
 *     saída "Superado"  = chegou mensagem nova durante a espera; encerre
 *       (NoOp) e deixe a execução mais nova responder por todas.
 *   Depois que o bot responder, chame "Cancelar" pra descartar lotes
 *   que ainda estejam na janela.
 */
export class WaGatewayQueue implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'wa-gateway Fila (debounce)',
		name: 'waGatewayQueue',
		icon: 'file:../WaGateway/waGateway.svg',
		group: ['transform'],
		version: 1,
		subtitle: '={{ $parameter["operation"] + ": " + $parameter["key"] }}',
		description: 'Agrupa mensagens picadas por contato no Redis (padrão debounce pra agente de IA)',
		defaults: { name: 'wa-gateway Fila' },
		inputs: ['main'],
		outputs: ['main', 'main'],
		outputNames: ['Continuar', 'Superado'],
		credentials: [{ name: 'redis', required: true }],
		properties: [
			{
				displayName: 'Operação',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				options: [
					{
						name: 'Enfileirar',
						value: 'enqueue',
						action: 'Adiciona a mensagem na fila do contato',
						description: 'Empilha a mensagem e marca esta execução como a mais recente',
					},
					{
						name: 'Coletar',
						value: 'collect',
						action: 'Coleta a fila se esta for a execucao mais recente',
						description: 'Use depois de um node Wait. Sai por "Continuar" com a fila unificada, ou por "Superado"',
					},
					{
						name: 'Cancelar',
						value: 'cancel',
						action: 'Descarta a fila do contato',
						description: 'Ex.: chame depois que o bot respondeu, pra abortar lotes pendentes',
					},
				],
				default: 'enqueue',
			},
			{
				displayName: 'Chave (contato)',
				name: 'key',
				type: 'string',
				required: true,
				default: '={{ $json.telefone || ($json.payload && $json.payload.chatId) || "" }}',
				description: 'Identificador do contato — normalmente o telefone ou chatId',
			},
			{
				displayName: 'Conteúdo',
				name: 'content',
				type: 'string',
				typeOptions: { rows: 2 },
				default: '={{ $json.conteudo_final || ($json.payload && ($json.payload.text || $json.payload.body)) || "" }}',
				displayOptions: { show: { operation: ['enqueue'] } },
			},
			{
				displayName: 'TTL da fila (segundos)',
				name: 'ttl',
				type: 'number',
				default: 300,
				displayOptions: { show: { operation: ['enqueue'] } },
				description: 'A fila e o marcador expiram nesse tempo se nada coletar',
			},
			{
				displayName: 'Token da execução',
				name: 'token',
				type: 'string',
				default: '={{ $json._waQueue && $json._waQueue.token }}',
				displayOptions: { show: { operation: ['collect'] } },
				description: 'Deixe como está — vem do Enfileirar pelo mesmo item',
			},
			{
				displayName: 'Separador',
				name: 'separator',
				type: 'string',
				default: '\\n',
				displayOptions: { show: { operation: ['collect'] } },
				description: 'Como juntar as mensagens da fila num texto só (\\n = quebra de linha)',
			},
			{
				displayName: 'Namespace',
				name: 'ns',
				type: 'string',
				default: '',
				description: 'Prefixo opcional pras chaves no Redis (ex.: nome da sessão)',
			},
		],
	};

	async execute(this: IExecuteFunctions): Promise<INodeExecutionData[][]> {
		const items = this.getInputData();
		const cont: INodeExecutionData[] = [];
		const superseded: INodeExecutionData[] = [];
		const creds = (await this.getCredentials('redis')) as IDataObject;
		const r = redisFromCreds(creds);

		try {
			for (let i = 0; i < items.length; i++) {
				const op = this.getNodeParameter('operation', i) as string;
				const ns = (this.getNodeParameter('ns', i, '') as string).trim();
				const key = String(this.getNodeParameter('key', i, '')).trim();
				if (!key) {
					throw new NodeOperationError(this.getNode(), 'Chave (contato) vazia', { itemIndex: i });
				}
				const listKey = redisKey(ns, 'fila', key);
				const markKey = redisKey(ns, 'fila-mark', key);
				const base = items[i].json;

				if (op === 'enqueue') {
					const content = this.getNodeParameter('content', i, '') as string;
					const ttl = Math.max(5, Number(this.getNodeParameter('ttl', i, 300)) || 300);
					const token = randomUUID();
					await r
						.multi()
						.rpush(listKey, JSON.stringify({ c: content, t: Date.now() }))
						.set(markKey, token)
						.expire(markKey, ttl)
						.expire(listKey, ttl)
						.exec();
					cont.push({ json: { ...base, _waQueue: { key, token, ns } }, pairedItem: { item: i } });
				} else if (op === 'collect') {
					const token = String(this.getNodeParameter('token', i, ''));
					const sep = String(this.getNodeParameter('separator', i, '\\n')).replace(/\\n/g, '\n').replace(/\\t/g, '\t');
					const raw = (await r.eval(COLLECT_LUA, 2, listKey, markKey, token)) as string[] | null;
					if (!raw) {
						superseded.push({ json: { ...base, _superseded: true }, pairedItem: { item: i } });
					} else {
						const parsed = raw.map((s) => {
							try {
								return JSON.parse(s) as { c: string; t: number };
							} catch {
								return { c: s, t: 0 };
							}
						});
						const text = parsed
							.map((p) => p.c)
							.filter((x) => x != null && x !== '')
							.join(sep);
						cont.push({
							json: {
								...base,
								key,
								count: parsed.length,
								text,
								messages: parsed.map((p) => ({ content: p.c, ts: p.t })),
								firstTs: parsed[0]?.t ?? null,
								lastTs: parsed[parsed.length - 1]?.t ?? null,
							},
							pairedItem: { item: i },
						});
					}
				} else {
					// cancel
					await r.del(listKey, markKey);
					cont.push({ json: { ...base, _cancelled: true }, pairedItem: { item: i } });
				}
			}
		} finally {
			r.disconnect();
		}

		return [cont, superseded];
	}
}
