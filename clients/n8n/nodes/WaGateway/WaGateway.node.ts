import type {
	IExecuteFunctions,
	INodeExecutionData,
	INodeType,
	INodeTypeDescription,
	IHttpRequestMethods,
	IDataObject,
} from 'n8n-workflow';
import { NodeOperationError } from 'n8n-workflow';

type Op = { resource: string; operation: string; method: IHttpRequestMethods; path: (c: Ctx) => string; body?: (c: Ctx) => IDataObject };
type Ctx = { fn: IExecuteFunctions; i: number; get: (n: string, d?: unknown) => any };

/** helpers ------------------------------------------------------------------ */
async function mediaToBase64(c: Ctx): Promise<{ data: string; mimetype?: string; filename?: string }> {
	const source = c.get('mediaSource', 'binary') as string;
	if (source === 'base64') {
		return { data: c.get('data') as string, mimetype: (c.get('mimetype') as string) || undefined, filename: (c.get('filename') as string) || undefined };
	}
	if (source === 'url') {
		const url = c.get('url') as string;
		const buf = (await c.fn.helpers.httpRequest({ method: 'GET', url, encoding: 'arraybuffer' })) as Buffer;
		return { data: Buffer.from(buf).toString('base64'), mimetype: undefined, filename: (c.get('filename') as string) || undefined };
	}
	// binary (default)
	const prop = (c.get('binaryProperty', 'data') as string) || 'data';
	const bin = c.fn.helpers.assertBinaryData(c.i, prop);
	const buf = await c.fn.helpers.getBinaryDataBuffer(c.i, prop);
	return { data: buf.toString('base64'), mimetype: bin.mimeType, filename: bin.fileName };
}

const S = (c: Ctx) => encodeURIComponent(c.get('session') as string);
const JIDp = (c: Ctx) => encodeURIComponent(c.get('groupJid') as string);

/** operation table -------------------------------------------------------- */
const OPS: Op[] = [
	// ---- Session
	{ resource: 'session', operation: 'list', method: 'GET', path: () => '/api/sessions' },
	{ resource: 'session', operation: 'get', method: 'GET', path: (c) => `/api/sessions/${S(c)}` },
	{ resource: 'session', operation: 'create', method: 'POST', path: () => '/api/sessions',
		body: (c) => ({ name: c.get('name'), start: c.get('start', true), config: c.get('config', undefined) }) },
	{ resource: 'session', operation: 'start', method: 'POST', path: (c) => `/api/sessions/${S(c)}/start` },
	{ resource: 'session', operation: 'stop', method: 'POST', path: (c) => `/api/sessions/${S(c)}/stop` },
	{ resource: 'session', operation: 'restart', method: 'POST', path: (c) => `/api/sessions/${S(c)}/restart` },
	{ resource: 'session', operation: 'logout', method: 'POST', path: (c) => `/api/sessions/${S(c)}/logout` },
	{ resource: 'session', operation: 'qr', method: 'GET', path: (c) => `/api/${S(c)}/auth/qr` },
	{ resource: 'session', operation: 'pairCode', method: 'POST', path: (c) => `/api/sessions/${S(c)}/auth/pair-code`,
		body: (c) => ({ phone: String(c.get('phone')).replace(/\D/g, '') }) },
	{ resource: 'session', operation: 'me', method: 'GET', path: (c) => `/api/${S(c)}/me` },

	// ---- Message
	{ resource: 'message', operation: 'sendText', method: 'POST', path: () => '/api/sendText',
		body: (c) => ({ session: c.get('session'), chatId: c.get('chatId'), text: c.get('text'), linkPreview: c.get('linkPreview', false), ...queue(c) }) },
	{ resource: 'message', operation: 'sendLocation', method: 'POST', path: () => '/api/sendLocation',
		body: (c) => ({ session: c.get('session'), chatId: c.get('chatId'), latitude: c.get('latitude'), longitude: c.get('longitude'), name: c.get('name', ''), ...queue(c) }) },
	{ resource: 'message', operation: 'sendPoll', method: 'POST', path: () => '/api/sendPoll',
		body: (c) => ({ session: c.get('session'), chatId: c.get('chatId'), name: c.get('pollName'), options: String(c.get('pollOptions')).split('\n').map((x) => x.trim()).filter(Boolean), selectable: c.get('selectable', 1), ...queue(c) }) },
	{ resource: 'message', operation: 'react', method: 'POST', path: () => '/api/reaction',
		body: (c) => ({ session: c.get('session'), chatId: c.get('chatId'), messageId: c.get('messageId'), emoji: c.get('emoji', ''), fromMe: c.get('fromMe', false) }) },
	{ resource: 'message', operation: 'forward', method: 'POST', path: () => '/api/forwardMessage',
		body: (c) => ({ session: c.get('session'), toChatId: c.get('chatId'), messageId: c.get('messageId') }) },
	// media (sendImage / sendFile / sendVideo / sendAudio) resolvido no execute

	// ---- Group
	{ resource: 'group', operation: 'list', method: 'GET', path: (c) => `/api/groups?session=${S(c)}` },
	{ resource: 'group', operation: 'get', method: 'GET', path: (c) => `/api/groups/${JIDp(c)}?session=${S(c)}` },
	{ resource: 'group', operation: 'create', method: 'POST', path: () => '/api/groups',
		body: (c) => ({ session: c.get('session'), name: c.get('name'), participants: lines(c, 'participants') }) },
	{ resource: 'group', operation: 'participants', method: 'POST', path: (c) => `/api/groups/${JIDp(c)}/participants`,
		body: (c) => ({ session: c.get('session'), action: c.get('action'), participants: lines(c, 'participants') }) },
	{ resource: 'group', operation: 'setName', method: 'PUT', path: (c) => `/api/groups/${JIDp(c)}/name`,
		body: (c) => ({ session: c.get('session'), name: c.get('name') }) },
	{ resource: 'group', operation: 'inviteLink', method: 'GET', path: (c) => `/api/groups/${JIDp(c)}/invite-link?session=${S(c)}` },

	// ---- Contact
	{ resource: 'contact', operation: 'check', method: 'GET', path: (c) => `/api/contacts/check?session=${S(c)}&phone=${encodeURIComponent(c.get('phone') as string)}` },
	{ resource: 'contact', operation: 'info', method: 'GET', path: (c) => `/api/contacts/info?session=${S(c)}&jid=${encodeURIComponent(c.get('jid') as string)}` },
	{ resource: 'contact', operation: 'picture', method: 'GET', path: (c) => `/api/contacts/profile-picture?session=${S(c)}&jid=${encodeURIComponent(c.get('jid') as string)}` },

	// ---- Chat / histórico
	{ resource: 'chat', operation: 'list', method: 'GET', path: (c) => `/api/chats?session=${S(c)}&limit=${c.get('limit', 100)}` },
	{ resource: 'chat', operation: 'history', method: 'GET', path: (c) => `/api/chats/${encodeURIComponent(c.get('chatId') as string)}/messages?session=${S(c)}&limit=${c.get('limit', 50)}` },
];
const queue = (c: Ctx): IDataObject => (c.get('enqueue', false) ? { enqueue: true, delay: c.get('delay', '') || undefined } : {});
const lines = (c: Ctx, n: string) => String(c.get(n, '')).split('\n').map((x) => x.trim()).filter(Boolean);

/** node ------------------------------------------------------------------- */
export class WaGateway implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'wa-gateway',
		name: 'waGateway',
		icon: 'file:waGateway.svg',
		group: ['output'],
		version: 1,
		subtitle: '={{$parameter["operation"] + " · " + $parameter["resource"]}}',
		description: 'Envia mensagens de WhatsApp e gerencia sessões pelo wa-gateway',
		defaults: { name: 'wa-gateway' },
		inputs: ['main'],
		outputs: ['main'],
		credentials: [{ name: 'waGatewayApi', required: true }],
		properties: [
			{
				displayName: 'Recurso', name: 'resource', type: 'options', noDataExpression: true,
				options: [
					{ name: 'Mensagem', value: 'message' },
					{ name: 'Sessão', value: 'session' },
					{ name: 'Grupo', value: 'group' },
					{ name: 'Contato', value: 'contact' },
					{ name: 'Conversa (histórico)', value: 'chat' },
				],
				default: 'message',
			},
			// operations per resource
			opt('message', [
				['Enviar texto', 'sendText'], ['Enviar imagem', 'sendImage'], ['Enviar documento', 'sendFile'],
				['Enviar vídeo', 'sendVideo'], ['Enviar áudio', 'sendAudio'], ['Enviar localização', 'sendLocation'],
				['Enviar enquete', 'sendPoll'], ['Reagir', 'react'], ['Encaminhar', 'forward'],
			], 'sendText'),
			opt('session', [
				['Criar', 'create'], ['Listar', 'list'], ['Detalhes', 'get'], ['Iniciar', 'start'], ['Parar', 'stop'],
				['Reiniciar', 'restart'], ['Deslogar', 'logout'], ['QR (texto)', 'qr'], ['Parear por código', 'pairCode'], ['Meu perfil', 'me'],
			], 'create'),
			opt('group', [
				['Listar', 'list'], ['Detalhes', 'get'], ['Criar', 'create'], ['Participantes', 'participants'],
				['Renomear', 'setName'], ['Link de convite', 'inviteLink'],
			], 'list'),
			opt('contact', [['Checar número', 'check'], ['Info de perfil', 'info'], ['Foto de perfil', 'picture']], 'check'),
			opt('chat', [['Listar conversas', 'list'], ['Histórico da conversa', 'history']], 'history'),

			// ---- common fields
			str('session', 'Sessão', { required: true, show: { resource: ['message', 'group', 'contact', 'chat'] } }),
			str('session', 'Sessão', { required: true, show: { resource: ['session'], operation: ['get', 'start', 'stop', 'restart', 'logout', 'qr', 'pairCode', 'me'] } }),
			str('chatId', 'Chat ID', { placeholder: '5599999999999@s.whatsapp.net', required: true,
				show: { resource: ['message'], operation: ['sendText', 'sendImage', 'sendFile', 'sendVideo', 'sendAudio', 'sendLocation', 'sendPoll', 'react', 'forward'] } }),
			str('chatId', 'Chat ID', { required: true, show: { resource: ['chat'], operation: ['history'] } }),

			str('text', 'Texto', { typeOptions: { rows: 3 }, required: true, show: { resource: ['message'], operation: ['sendText'] } }),
			bool('linkPreview', 'Preview de link', false, { show: { resource: ['message'], operation: ['sendText'] } }),

			// media
			{
				displayName: 'Fonte da mídia', name: 'mediaSource', type: 'options',
				options: [
					{ name: 'Binário do nó anterior', value: 'binary' },
					{ name: 'URL pública', value: 'url' },
					{ name: 'Base64 / data URI', value: 'base64' },
				],
				default: 'binary',
				displayOptions: { show: { resource: ['message'], operation: ['sendImage', 'sendFile', 'sendVideo', 'sendAudio'] } },
			},
			str('binaryProperty', 'Propriedade binária', { default: 'data',
				show: { resource: ['message'], operation: ['sendImage', 'sendFile', 'sendVideo', 'sendAudio'], mediaSource: ['binary'] } }),
			str('url', 'URL', { placeholder: 'https://…/arquivo.jpg', required: true,
				show: { resource: ['message'], operation: ['sendImage', 'sendFile', 'sendVideo', 'sendAudio'], mediaSource: ['url'] } }),
			str('data', 'Base64', { typeOptions: { rows: 2 }, required: true,
				show: { resource: ['message'], operation: ['sendImage', 'sendFile', 'sendVideo', 'sendAudio'], mediaSource: ['base64'] } }),
			str('caption', 'Legenda', { show: { resource: ['message'], operation: ['sendImage', 'sendFile', 'sendVideo'] } }),
			str('filename', 'Nome do arquivo', { show: { resource: ['message'], operation: ['sendFile'] } }),
			bool('voice', 'Nota de voz (PTT)', false, { show: { resource: ['message'], operation: ['sendAudio'] } }),

			// location
			num('latitude', 'Latitude', { required: true, show: { resource: ['message'], operation: ['sendLocation'] } }),
			num('longitude', 'Longitude', { required: true, show: { resource: ['message'], operation: ['sendLocation'] } }),
			str('name', 'Nome do lugar', { show: { resource: ['message'], operation: ['sendLocation'] } }),

			// poll
			str('pollName', 'Pergunta', { required: true, show: { resource: ['message'], operation: ['sendPoll'] } }),
			str('pollOptions', 'Opções (uma por linha)', { typeOptions: { rows: 3 }, required: true, show: { resource: ['message'], operation: ['sendPoll'] } }),
			num('selectable', 'Opções selecionáveis', { default: 1, show: { resource: ['message'], operation: ['sendPoll'] } }),

			// react / forward
			str('messageId', 'Message ID', { required: true, show: { resource: ['message'], operation: ['react', 'forward'] } }),
			str('emoji', 'Emoji', { placeholder: '👍  (vazio = remove)', show: { resource: ['message'], operation: ['react'] } }),
			bool('fromMe', 'A mensagem alvo é minha', false, { show: { resource: ['message'], operation: ['react'] } }),

			// enqueue (anti-ban)
			bool('enqueue', 'Enfileirar (pacing anti-ban)', false, { show: { resource: ['message'], operation: ['sendText', 'sendImage', 'sendFile', 'sendVideo', 'sendAudio', 'sendLocation', 'sendPoll'] } }),
			str('delay', 'Delay extra', { placeholder: '30s', show: { resource: ['message'], operation: ['sendText', 'sendImage', 'sendFile', 'sendVideo', 'sendAudio', 'sendLocation', 'sendPoll'], enqueue: [true] } }),

			// session create
			str('name', 'Nome da sessão', { placeholder: 'default', required: true, show: { resource: ['session'], operation: ['create'] } }),
			bool('start', 'Iniciar já', true, { show: { resource: ['session'], operation: ['create'] } }),
			{ displayName: 'Config (JSON)', name: 'config', type: 'json', default: '', description: 'webhooks / outbox / rawEvents (opcional)',
				displayOptions: { show: { resource: ['session'], operation: ['create'] } } },
			str('phone', 'Telefone (E.164)', { placeholder: '5599999999999', required: true, show: { resource: ['session'], operation: ['pairCode'] } }),

			// group
			str('groupJid', 'Grupo (JID)', { placeholder: '...@g.us', required: true, show: { resource: ['group'], operation: ['get', 'participants', 'setName', 'inviteLink'] } }),
			str('name', 'Nome', { required: true, show: { resource: ['group'], operation: ['create', 'setName'] } }),
			str('participants', 'Participantes (um JID por linha)', { typeOptions: { rows: 3 }, show: { resource: ['group'], operation: ['create', 'participants'] } }),
			{ displayName: 'Ação', name: 'action', type: 'options', options: [
				{ name: 'Adicionar', value: 'add' }, { name: 'Remover', value: 'remove' }, { name: 'Promover', value: 'promote' }, { name: 'Rebaixar', value: 'demote' },
			], default: 'add', displayOptions: { show: { resource: ['group'], operation: ['participants'] } } },

			// contact
			str('phone', 'Telefones (vírgula)', { placeholder: '+5599...,+55...', required: true, show: { resource: ['contact'], operation: ['check'] } }),
			str('jid', 'JID(s) (vírgula)', { placeholder: '...@s.whatsapp.net', required: true, show: { resource: ['contact'], operation: ['info', 'picture'] } }),

			// chat
			num('limit', 'Limite', { default: 50, show: { resource: ['chat'] } }),
		],
	};

	async execute(this: IExecuteFunctions): Promise<INodeExecutionData[][]> {
		const items = this.getInputData();
		const out: INodeExecutionData[] = [];
		const creds = await this.getCredentials('waGatewayApi');
		const base = String(creds.baseUrl).replace(/\/$/, '');

		for (let i = 0; i < items.length; i++) {
			try {
				const resource = this.getNodeParameter('resource', i) as string;
				const operation = this.getNodeParameter('operation', i) as string;
				const c: Ctx = { fn: this, i, get: (n, d) => { try { return this.getNodeParameter(n, i, d as any); } catch { return d; } } };

				let method: IHttpRequestMethods = 'POST';
				let path = '';
				let body: IDataObject | undefined;

				const mediaOp = ['sendImage', 'sendFile', 'sendVideo', 'sendAudio'];
				if (resource === 'message' && mediaOp.includes(operation)) {
					const m = await mediaToBase64(c);
					path = '/api/' + operation;
					body = {
						session: c.get('session'), chatId: c.get('chatId'),
						data: m.data, mimetype: m.mimetype, filename: c.get('filename', m.filename) || m.filename,
						caption: c.get('caption', '') || undefined,
						voice: operation === 'sendAudio' ? c.get('voice', false) : undefined,
						...queue(c),
					};
				} else {
					const op = OPS.find((o) => o.resource === resource && o.operation === operation);
					if (!op) throw new NodeOperationError(this.getNode(), `operação não suportada: ${resource}.${operation}`);
					method = op.method;
					path = op.path(c);
					body = op.body ? op.body(c) : undefined;
				}

				const res = await this.helpers.httpRequestWithAuthentication.call(this, 'waGatewayApi', {
					method,
					url: base + path,
					body,
					json: true,
				});
				out.push({ json: (res ?? {}) as IDataObject, pairedItem: { item: i } });
			} catch (err) {
				if (this.continueOnFail()) {
					out.push({ json: { error: (err as Error).message }, pairedItem: { item: i } });
					continue;
				}
				throw err;
			}
		}
		return [out];
	}
}

/** property builders ---------------------------------------------------- */
function opt(resource: string, entries: [string, string][], def: string) {
	return {
		displayName: 'Operação', name: 'operation', type: 'options' as const, noDataExpression: true,
		options: entries.map(([name, value]) => ({ name, value, action: name })),
		default: def,
		displayOptions: { show: { resource: [resource] } },
	};
}
function str(name: string, displayName: string, o: any = {}) {
	return {
		displayName, name, type: 'string' as const,
		default: o.default ?? '', placeholder: o.placeholder, required: o.required,
		typeOptions: o.typeOptions,
		displayOptions: o.show ? { show: o.show } : undefined,
	};
}
function num(name: string, displayName: string, o: any = {}) {
	return {
		displayName, name, type: 'number' as const, default: o.default ?? 0, required: o.required,
		displayOptions: o.show ? { show: o.show } : undefined,
	};
}
function bool(name: string, displayName: string, def: boolean, o: any = {}) {
	return {
		displayName, name, type: 'boolean' as const, default: def,
		displayOptions: o.show ? { show: o.show } : undefined,
	};
}
