import type {
	IExecuteFunctions,
	INodeExecutionData,
	INodeType,
	INodeTypeDescription,
	IHttpRequestMethods,
	IDataObject,
	JsonObject,
} from 'n8n-workflow';
import { NodeApiError } from 'n8n-workflow';

/**
 * wa-gateway OTP: código de verificação de número por WhatsApp.
 *
 * Enrola os endpoints /api/{session}/otp/{send,verify,cancel}. Duas saídas:
 *   0 "Sucesso / Válido"  — código enviado, ou conferência OK
 *   1 "Falha / Inválido"  — rate limit / envio falhou, ou código errado/expirado
 *
 * Ideal quando o gateway é o dono do código. Se o SEU app gera e guarda o
 * código, use o node "wa-gateway" (Enviar texto) com callbackUrl.
 */
export class WaGatewayOtp implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'wa-gateway OTP',
		name: 'waGatewayOtp',
		icon: 'file:../WaGateway/waGateway.svg',
		group: ['transform'],
		version: 1,
		subtitle: '={{ $parameter["operation"] + " · " + $parameter["session"] }}',
		description: 'Envia e confere código de verificação (OTP) por WhatsApp via wa-gateway',
		defaults: { name: 'wa-gateway OTP' },
		inputs: ['main'],
		outputs: ['main', 'main'],
		outputNames: ['Sucesso / Válido', 'Falha / Inválido'],
		credentials: [{ name: 'waGatewayApi', required: true }],
		properties: [
			{
				displayName: 'Operação',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				options: [
					{
						name: 'Enviar código',
						value: 'send',
						action: 'Gera um codigo e manda por WhatsApp',
						description: 'POST /api/{session}/otp/send',
					},
					{
						name: 'Conferir código',
						value: 'verify',
						action: 'Verifica o codigo digitado pelo usuario',
						description: 'POST /api/{session}/otp/verify — sai por "Válido" ou "Inválido"',
					},
					{
						name: 'Cancelar código',
						value: 'cancel',
						action: 'Invalida o codigo ativo de um numero',
						description: 'POST /api/{session}/otp/cancel',
					},
				],
				default: 'send',
			},
			{
				displayName: 'Sessão',
				name: 'session',
				type: 'string',
				required: true,
				default: '',
				description: 'Nome da sessão que vai enviar a mensagem',
			},
			{
				displayName: 'Número (to)',
				name: 'to',
				type: 'string',
				default: "={{ $json.to || $json.phone || $json.telefone || ($json.payload && $json.payload.chatId) || '' }}",
				placeholder: '5517999999999',
				description: 'Número em formato internacional (só dígitos) ou JID. No "Conferir", pode deixar vazio e usar o ID.',
				displayOptions: { show: { operation: ['send', 'verify', 'cancel'] } },
			},

			// ---- send
			{
				displayName: 'Marca (brand)',
				name: 'brand',
				type: 'string',
				default: '',
				placeholder: 'nome da empresa (aparece na mensagem)',
				displayOptions: { show: { operation: ['send'] } },
			},
			{
				displayName: 'Template',
				name: 'template',
				type: 'string',
				typeOptions: { rows: 2 },
				default: '',
				placeholder: '{{code}} é o seu código{{brand}}. Expira em {{minutes}} min.',
				description: 'Opcional. Placeholders: {{code}} {{brand}} {{minutes}} {{ttl}}',
				displayOptions: { show: { operation: ['send'] } },
			},
			{
				displayName: 'Tamanho do código',
				name: 'codeLength',
				type: 'number',
				default: 6,
				description: '4 a 10 dígitos',
				displayOptions: { show: { operation: ['send'] } },
			},
			{
				displayName: 'Validade (segundos)',
				name: 'ttlSeconds',
				type: 'number',
				default: 300,
				description: '30 a 1800',
				displayOptions: { show: { operation: ['send'] } },
			},
			{
				displayName: 'Callback de entrega (URL)',
				name: 'callbackUrl',
				type: 'string',
				default: '',
				placeholder: 'https://meuapp/hooks/wa?t=segredo',
				description: 'Opcional: o gateway faz 1 POST aqui quando a mensagem for entregue/lida/falhar',
				displayOptions: { show: { operation: ['send'] } },
			},
			{
				displayName: 'Callback: dados extras (JSON)',
				name: 'callbackData',
				type: 'json',
				default: '',
				description: 'Só usado se "Callback de entrega" estiver preenchido. Ecoado no corpo do callback (ex.: { "userId": 123 }).',
				displayOptions: { show: { operation: ['send'] } },
			},

			// ---- verify
			{
				displayName: 'Código',
				name: 'code',
				type: 'string',
				required: true,
				default: "={{ $json.code || $json.otp || '' }}",
				placeholder: '123456',
				displayOptions: { show: { operation: ['verify'] } },
			},
			{
				displayName: 'ID do desafio',
				name: 'id',
				type: 'string',
				default: '',
				placeholder: 'otp_… (alternativa ao número)',
				description: 'Devolvido pelo "Enviar código". Use quando não tiver o número em mãos.',
				displayOptions: { show: { operation: ['verify'] } },
			},
		],
	};

	async execute(this: IExecuteFunctions): Promise<INodeExecutionData[][]> {
		const items = this.getInputData();
		const ok: INodeExecutionData[] = [];
		const fail: INodeExecutionData[] = [];

		const creds = await this.getCredentials('waGatewayApi');
		const base = String(creds.baseUrl).replace(/\/$/, '');

		for (let i = 0; i < items.length; i++) {
			const operation = this.getNodeParameter('operation', i) as string;
			const session = this.getNodeParameter('session', i) as string;
			const to = String(this.getNodeParameter('to', i, '') ?? '').trim();

			let path = '';
			const body: IDataObject = {};

			if (operation === 'send') {
				path = `/api/${encodeURIComponent(session)}/otp/send`;
				body.to = to;
				const brand = this.getNodeParameter('brand', i, '') as string;
				const template = this.getNodeParameter('template', i, '') as string;
				const codeLength = this.getNodeParameter('codeLength', i, 0) as number;
				const ttlSeconds = this.getNodeParameter('ttlSeconds', i, 0) as number;
				const callbackUrl = this.getNodeParameter('callbackUrl', i, '') as string;
				if (brand) body.brand = brand;
				if (template) body.template = template;
				if (codeLength) body.codeLength = codeLength;
				if (ttlSeconds) body.ttlSeconds = ttlSeconds;
				if (callbackUrl) {
					body.callbackUrl = callbackUrl;
					const cd = this.getNodeParameter('callbackData', i, '') as string | IDataObject;
					const parsed = typeof cd === 'string' ? safeJson(cd) : cd;
					if (parsed !== undefined) body.callbackData = parsed;
				}
			} else if (operation === 'verify') {
				path = `/api/${encodeURIComponent(session)}/otp/verify`;
				body.code = String(this.getNodeParameter('code', i, '') ?? '').trim();
				const id = this.getNodeParameter('id', i, '') as string;
				if (id) body.id = id;
				if (to) body.to = to;
			} else {
				path = `/api/${encodeURIComponent(session)}/otp/cancel`;
				body.to = to;
			}

			try {
				const res = (await this.helpers.httpRequestWithAuthentication.call(this, 'waGatewayApi', {
					method: 'POST' as IHttpRequestMethods,
					url: base + path,
					body,
					json: true,
				})) as IDataObject;

				// "Conferir": código errado/expirado volta como valid:false
				if (operation === 'verify' && res && res.valid === false) {
					fail.push({ json: { ...items[i].json, ...res, operation }, pairedItem: { item: i } });
				} else {
					ok.push({ json: { ...items[i].json, ...(res || {}), operation }, pairedItem: { item: i } });
				}
			} catch (err) {
				const e = err as { httpCode?: string; message?: string; response?: { body?: IDataObject } };
				const code = String(e.httpCode || '');
				const bodyErr = e.response?.body || {};
				// Resultados "de negócio" esperados → saída 1, sem quebrar o fluxo:
				//   422 código inválido/expirado · 429 rate limit · 502 envio falhou
				const businessFail = ['422', '429', '502'].includes(code);
				if (businessFail || this.continueOnFail()) {
					fail.push({
						json: { ...items[i].json, operation, ok: false, error: e.message || 'erro', ...bodyErr },
						pairedItem: { item: i },
					});
					continue;
				}
				throw new NodeApiError(this.getNode(), err as JsonObject, { itemIndex: i });
			}
		}

		return [ok, fail];
	}
}

function safeJson(s: string): unknown {
	const t = (s || '').trim();
	if (!t) return undefined;
	try {
		return JSON.parse(t);
	} catch {
		return t;
	}
}
