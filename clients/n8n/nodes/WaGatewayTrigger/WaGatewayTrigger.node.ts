import type {
	IHookFunctions,
	IWebhookFunctions,
	INodeType,
	INodeTypeDescription,
	IWebhookResponseData,
	IDataObject,
	INodeExecutionData,
} from 'n8n-workflow';
import { createHmac, timingSafeEqual } from 'crypto';

/** Saídas do node — uma por tipo de mensagem. Ordem = índice do output. */
const OUTPUTS = ['Texto', 'Imagem', 'Áudio', 'Vídeo', 'Documento', 'Outros', 'Eventos', 'Enquete'] as const;
const IDX: Record<string, number> = { text: 0, image: 1, audio: 2, video: 3, document: 4 };
const OUT_OTHER = 5;
const OUT_EVENT = 6;
const OUT_POLL = 7;

/**
 * Trigger do wa-gateway: recebe eventos de uma sessão e **roteia por tipo**.
 * Cada tipo de mensagem sai na sua própria saída — sem precisar de Switch.
 * Opcionalmente baixa a mídia e anexa como binário (`data`), pronta pro
 * próximo node.
 */
export class WaGatewayTrigger implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'wa-gateway Trigger',
		name: 'waGatewayTrigger',
		icon: 'file:../WaGateway/waGateway.svg',
		group: ['trigger'],
		version: 2,
		subtitle: '={{$parameter["session"] + " · " + ($parameter["events"] || "*")}}',
		description: 'Recebe eventos do wa-gateway roteando por tipo (texto, imagem, áudio, vídeo, documento, enquete/voto…)',
		defaults: { name: 'wa-gateway Trigger' },
		inputs: [],
		outputs: ['main', 'main', 'main', 'main', 'main', 'main', 'main', 'main'],
		outputNames: [...OUTPUTS],
		credentials: [{ name: 'waGatewayApi', required: true }],
		webhooks: [
			{ name: 'default', httpMethod: 'POST', responseMode: 'onReceived', path: 'webhook' },
		],
		properties: [
			{
				displayName:
					'Ative o workflow (toggle no topo) para receber mensagens de verdade. Só no editor, o n8n usa uma URL de TESTE temporária.',
				name: 'activateNotice',
				type: 'notice',
				default: '',
			},
			{
				displayName: 'Sessão', name: 'session', type: 'string', default: '', required: true,
				description: 'Nome da sessão que vai disparar o fluxo',
			},
			{
				displayName: 'Eventos', name: 'events', type: 'string', default: 'message',
				placeholder: 'message  ·  message,message.poll_vote,group.update  ·  *',
				description:
					'O que o gateway envia (CSV). "message" = só mensagens recebidas; "message.poll_vote" = votos de enquete já decifrados; "*" = tudo. Eventos que não são mensagem saem em "Eventos"; votos e criação de enquete saem em "Enquete".',
			},
			{
				displayName: 'Baixar mídia (anexar como binário)', name: 'downloadMedia', type: 'boolean', default: true,
				description:
					'Whether to baixar a mídia (imagem/áudio/vídeo/documento) e anexar em binary.data, pronta pro próximo node',
			},
			{
				displayName: 'Segredo HMAC', name: 'secret', type: 'string', typeOptions: { password: true }, default: '',
				description: 'Se preenchido: assina o webhook e valida X-Webhook-Signature nas entradas',
			},
			{
				displayName: 'Registrar webhook na sessão automaticamente', name: 'autoRegister', type: 'boolean', default: true,
				description: 'Whether to add/remove esta URL em config.webhooks da sessão ao ativar/desativar o workflow',
			},
		],
	};

	webhookMethods = {
		default: {
			async checkExists(this: IHookFunctions): Promise<boolean> {
				if (!(this.getNodeParameter('autoRegister', true) as boolean)) return true;
				const url = this.getNodeWebhookUrl('default') as string;
				const cfg = await getConfig.call(this);
				return (cfg.webhooks || []).some((w) => w.url === url);
			},
			async create(this: IHookFunctions): Promise<boolean> {
				if (!(this.getNodeParameter('autoRegister', true) as boolean)) return true;
				const url = this.getNodeWebhookUrl('default') as string;
				const events = splitCsv(this.getNodeParameter('events', 'message') as string);
				const secret = this.getNodeParameter('secret', '') as string;
				const tag = nodeTag.call(this);
				const cfg = await getConfig.call(this);
				// tira a própria tag e QUALQUER variante desta URL (teste↔produção)
				// pra não ficar registrado 2x e o gateway disparar 2 execuções.
				const variants = urlVariants(url);
				cfg.webhooks = (cfg.webhooks || []).filter((w) => w._n8n !== tag && !variants.includes(String(w.url)));
				const wh: IDataObject = { url, events: events.length ? events : ['*'], _n8n: tag };
				if (secret) wh.hmac = { secret };
				cfg.webhooks.push(wh);
				await putConfig.call(this, cfg);
				return true;
			},
			async delete(this: IHookFunctions): Promise<boolean> {
				if (!(this.getNodeParameter('autoRegister', true) as boolean)) return true;
				const url = this.getNodeWebhookUrl('default') as string;
				const tag = nodeTag.call(this);
				const variants = urlVariants(url);
				try {
					const cfg = await getConfig.call(this);
					cfg.webhooks = (cfg.webhooks || []).filter((w) => w._n8n !== tag && !variants.includes(String(w.url)));
					await putConfig.call(this, cfg);
				} catch {
					/* sessão pode nem existir mais */
				}
				return true;
			},
		},
	};

	async webhook(this: IWebhookFunctions): Promise<IWebhookResponseData> {
		const req = this.getRequestObject();
		const body = (req.body || {}) as IDataObject;
		const secret = this.getNodeParameter('secret', '') as string;

		if (secret) {
			const sig = (req.headers['x-webhook-signature'] as string) || '';
			const raw =
				typeof req.rawBody === 'string'
					? Buffer.from(req.rawBody)
					: (req.rawBody as Buffer) || Buffer.from(JSON.stringify(body));
			const expected = 'sha256=' + createHmac('sha256', secret).update(raw).digest('hex');
			const a = Buffer.from(sig);
			const b = Buffer.from(expected);
			if (a.length !== b.length || !timingSafeEqual(a, b)) {
				return { webhookResponse: { statusCode: 401, body: 'assinatura inválida' } };
			}
		}

		const ev = String(body.event || '');
		const payload = (body.payload || {}) as IDataObject;
		const isMsg = ev === 'message' || ev === 'message.any';
		const type = String(payload.type || '');

		const item: INodeExecutionData = { json: body };
		let outIdx: number;

		if (ev === 'message.poll_vote') {
			// voto de enquete já decifrado: entrega o payload ACHATADO
			// (pollId, chatId, voter, voterName, selectedOptions, removed, timestamp)
			// pronto pra usar, na saída "Enquete".
			outIdx = OUT_POLL;
			item.json = { event: ev, session: body.session, ...payload };
		} else if (isMsg && type === 'poll') {
			// criação de enquete → também sai em "Enquete" (payload cru: id = pollId,
			// body = a pergunta). Ignora o poll_vote cru (esse vai por message.poll_vote).
			outIdx = OUT_POLL;
		} else if (isMsg && type === 'poll_vote') {
			// voto cru (sem opção resolvida) — some se você assina message.poll_vote.
			outIdx = OUT_OTHER;
		} else {
			outIdx = isMsg ? (IDX[type] ?? OUT_OTHER) : OUT_EVENT;
		}

		// baixa a mídia e anexa como binário (usa mediaMeta do evento —
		// não depende do store de mensagens)
		const wantBinary = this.getNodeParameter('downloadMedia', true) as boolean;
		const meta = payload.mediaMeta as IDataObject | undefined;
		const mediaUrl = (payload.media as IDataObject | undefined)?.url as string | undefined;
		if (wantBinary && isMsg && ['image', 'audio', 'video', 'document'].includes(type) && (meta || mediaUrl)) {
			try {
				const creds = await this.getCredentials('waGatewayApi');
				const b = String(creds.baseUrl).replace(/\/$/, '');
				const session = this.getNodeParameter('session') as string;
				let buf: { body: Buffer; headers: Record<string, string> };
				if (mediaUrl) {
					buf = (await this.helpers.httpRequestWithAuthentication.call(this, 'waGatewayApi', {
						method: 'GET', url: b + mediaUrl, encoding: 'arraybuffer', returnFullResponse: true,
					})) as any;
				} else {
					buf = (await this.helpers.httpRequestWithAuthentication.call(this, 'waGatewayApi', {
						method: 'POST',
						url: `${b}/api/${encodeURIComponent(session)}/media/download`,
						body: { type, ...meta },
						json: true,
						encoding: 'arraybuffer',
						returnFullResponse: true,
					})) as any;
				}
				const mime =
					(meta?.mimetype as string) || buf.headers['content-type'] || 'application/octet-stream';
				const ext = mime.split('/')[1]?.split(';')[0] || 'bin';
				const fname = (meta?.filename as string) || `${payload.id || 'media'}.${ext}`;
				item.binary = {
					data: await this.helpers.prepareBinaryData(Buffer.from(buf.body), fname, mime),
				};
			} catch (e) {
				item.json = { ...body, _mediaDownloadError: (e as Error).message };
			}
		}

		const out: INodeExecutionData[][] = OUTPUTS.map(() => []);
		out[outIdx] = [item];
		return { workflowData: out };
	}
}

/* ---------- helpers de config da sessão ---------- */
type Cfg = { webhooks?: IDataObject[] } & IDataObject;

function nodeTag(this: IHookFunctions): string {
	const wf = (this.getWorkflow?.() as { id?: string } | undefined)?.id ?? 'wf';
	const node = this.getNode?.()?.name ?? 'node';
	return `n8n:${wf}:${node}`;
}
async function base(this: IHookFunctions): Promise<string> {
	const creds = await this.getCredentials('waGatewayApi');
	return String(creds.baseUrl).replace(/\/$/, '');
}
async function getConfig(this: IHookFunctions): Promise<Cfg> {
	const session = this.getNodeParameter('session') as string;
	const res = (await this.helpers.httpRequestWithAuthentication.call(this, 'waGatewayApi', {
		method: 'GET',
		url: (await base.call(this)) + '/api/sessions/' + encodeURIComponent(session),
		json: true,
	})) as IDataObject;
	const cfg = (res.config as Cfg) || {};
	return { ...cfg, webhooks: Array.isArray(cfg.webhooks) ? cfg.webhooks : [] };
}
async function putConfig(this: IHookFunctions, cfg: Cfg): Promise<void> {
	const session = this.getNodeParameter('session') as string;
	await this.helpers.httpRequestWithAuthentication.call(this, 'waGatewayApi', {
		method: 'PUT',
		url: (await base.call(this)) + '/api/sessions/' + encodeURIComponent(session),
		body: { config: cfg },
		json: true,
	});
}
function splitCsv(s: string): string[] {
	return (s || '').split(',').map((x) => x.trim()).filter(Boolean);
}
/** as duas formas da URL de webhook do n8n: produção e teste. */
function urlVariants(url: string): string[] {
	const prod = url.replace('/webhook-test/', '/webhook/');
	const test = url.replace('/webhook/', '/webhook-test/');
	return Array.from(new Set([url, prod, test]));
}
