import type {
	IHookFunctions,
	IWebhookFunctions,
	INodeType,
	INodeTypeDescription,
	IWebhookResponseData,
	IDataObject,
} from 'n8n-workflow';
import { createHmac, timingSafeEqual } from 'crypto';

/**
 * Trigger do wa-gateway: cria um webhook no n8n e o registra automaticamente
 * na config da sessão ao ativar o workflow (e remove ao desativar).
 */
export class WaGatewayTrigger implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'wa-gateway Trigger',
		name: 'waGatewayTrigger',
		icon: 'file:../WaGateway/waGateway.svg',
		group: ['trigger'],
		version: 1,
		subtitle: '={{$parameter["session"] + " · " + ($parameter["events"] || "*")}}',
		description: 'Recebe eventos de uma sessão do wa-gateway (mensagens, status, grupos…)',
		defaults: { name: 'wa-gateway Trigger' },
		inputs: [],
		outputs: ['main'],
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
				placeholder: 'message, session.status, group.update  ·  ou  *',
				description: 'Lista separada por vírgula. Aceita curinga: message.*, session.*, *',
			},
			{
				displayName: 'Segredo HMAC', name: 'secret', type: 'string', typeOptions: { password: true }, default: '',
				description: 'Se preenchido: assina o webhook e valida X-Webhook-Signature nas entradas',
			},
			{
				displayName: 'Registrar webhook na sessão automaticamente', name: 'autoRegister', type: 'boolean', default: true,
				description:
					'Whether to add/remove esta URL em config.webhooks da sessão ao ativar/desativar o workflow',
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
				// remove entradas antigas deste mesmo node (ex.: URL de teste) e a URL atual
				cfg.webhooks = (cfg.webhooks || []).filter((w) => w._n8n !== tag && w.url !== url);
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
				try {
					const cfg = await getConfig.call(this);
					cfg.webhooks = (cfg.webhooks || []).filter((w) => w._n8n !== tag && w.url !== url);
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
		const body = req.body as IDataObject;
		const secret = this.getNodeParameter('secret', '') as string;

		if (secret) {
			const sig = (req.headers['x-webhook-signature'] as string) || '';
			const raw = typeof req.rawBody === 'string' ? Buffer.from(req.rawBody) : (req.rawBody as Buffer) || Buffer.from(JSON.stringify(body));
			const expected = 'sha256=' + createHmac('sha256', secret).update(raw).digest('hex');
			const a = Buffer.from(sig);
			const b = Buffer.from(expected);
			if (a.length !== b.length || !timingSafeEqual(a, b)) {
				return { webhookResponse: { statusCode: 401, body: 'assinatura inválida' } };
			}
		}

		// filtro local extra (o gateway já filtra, mas caso a config divirja)
		const evName = (body.event as string) || '';
		const wanted = splitCsv(this.getNodeParameter('events', '') as string);
		if (wanted.length && !wanted.some((p) => matchEvent(p, evName))) {
			return { noWebhookResponse: false, workflowData: [[]] };
		}

		return { workflowData: [this.helpers.returnJsonArray([body])] };
	}
}

/* ---------- helpers de config da sessão ---------- */
type Cfg = { webhooks?: IDataObject[] } & IDataObject;

/** marca estável para achar as entradas deste node ao ativar/desativar */
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
function matchEvent(pattern: string, name: string): boolean {
	if (pattern === '*' || pattern === name) return true;
	if (pattern.endsWith('.*')) return name.startsWith(pattern.slice(0, -1)) || name === pattern.slice(0, -2);
	return false;
}
