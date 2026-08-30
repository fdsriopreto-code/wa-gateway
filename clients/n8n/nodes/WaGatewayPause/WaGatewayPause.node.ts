import type {
	IExecuteFunctions,
	INodeExecutionData,
	INodeType,
	INodeTypeDescription,
	IDataObject,
} from 'n8n-workflow';
import { NodeOperationError } from 'n8n-workflow';
import { redisFromCreds, redisKey } from '../shared/redis';

/**
 * wa-gateway Pausa do bot: handoff humano por contato.
 *
 *   Trigger -> fromMe? ── true ─> Pausar   (atendente assumiu; IA cala 30min)
 *                        └ false ─> Verificar
 *                                    ├ "Ativo"   -> segue pro atendimento
 *                                    └ "Pausado" -> encerra (NoOp)
 *   "Retomar" limpa a pausa (ex.: comando /voltar do atendente).
 */
export class WaGatewayPause implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'wa-gateway Pausa do bot',
		name: 'waGatewayPause',
		icon: 'file:../WaGateway/waGateway.svg',
		group: ['transform'],
		version: 1,
		subtitle: '={{ $parameter["operation"] + ": " + $parameter["key"] }}',
		description: 'Pausa/retoma o atendimento automático por contato (handoff humano) via Redis',
		defaults: { name: 'wa-gateway Pausa' },
		inputs: ['main'],
		outputs: ['main', 'main'],
		outputNames: ['Ativo', 'Pausado'],
		credentials: [{ name: 'redis', required: true }],
		properties: [
			{
				displayName: 'Operação',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				options: [
					{
						name: 'Verificar',
						value: 'check',
						action: 'Verifica se o bot esta pausado pra este contato',
						description: 'Sai por "Ativo" ou "Pausado"',
					},
					{
						name: 'Pausar',
						value: 'pause',
						action: 'Pausa o bot para este contato',
						description: 'Ex.: quando fromMe = true (voce respondeu manualmente)',
					},
					{
						name: 'Retomar',
						value: 'resume',
						action: 'Remove a pausa do contato',
					},
				],
				default: 'check',
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
				displayName: 'Duração da pausa (segundos)',
				name: 'ttl',
				type: 'number',
				default: 1800,
				displayOptions: { show: { operation: ['pause'] } },
				description: '0 = sem expiração (fica pausado até "Retomar")',
			},
			{
				displayName: 'Motivo',
				name: 'reason',
				type: 'string',
				default: 'handoff',
				displayOptions: { show: { operation: ['pause'] } },
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
		const active: INodeExecutionData[] = [];
		const paused: INodeExecutionData[] = [];
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
				const pk = redisKey(ns, 'pausa', key);
				const base = items[i].json;

				if (op === 'pause') {
					const ttl = Number(this.getNodeParameter('ttl', i, 1800)) || 0;
					const reason = String(this.getNodeParameter('reason', i, 'handoff'));
					await r.set(pk, reason);
					if (ttl > 0) await r.expire(pk, ttl);
					paused.push({ json: { ...base, _paused: true, _pauseReason: reason }, pairedItem: { item: i } });
				} else if (op === 'resume') {
					await r.del(pk);
					active.push({ json: { ...base, _paused: false }, pairedItem: { item: i } });
				} else {
					// check
					const v = await r.get(pk);
					if (v == null) {
						active.push({ json: { ...base, _paused: false }, pairedItem: { item: i } });
					} else {
						const ttl = await r.ttl(pk);
						paused.push({
							json: { ...base, _paused: true, _pauseReason: v, _pauseTtl: ttl },
							pairedItem: { item: i },
						});
					}
				}
			}
		} finally {
			r.disconnect();
		}

		return [active, paused];
	}
}
