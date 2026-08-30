import type {
	IExecuteFunctions,
	INodeType,
	INodeTypeDescription,
	INodeExecutionData,
	IDataObject,
} from 'n8n-workflow';
import { NodeOperationError } from 'n8n-workflow';

/* Tipos de conexão de IA do n8n (string literais — estáveis entre versões). */
const CONN = {
	main: 'main',
	model: 'ai_languageModel',
	memory: 'ai_memory',
	tool: 'ai_tool',
} as const;

/* Shapes mínimos do LangChain — os objetos vêm das conexões, não bundlamos nada. */
type LCMessage = { content: any; tool_calls?: Array<{ name: string; args: any; id?: string }>; _getType?: () => string };
type LCModel = { bindTools?: (t: any[]) => LCModel; invoke: (msgs: any[], opts?: any) => Promise<LCMessage> };
type LCMemory = {
	loadMemoryVariables: (v: any) => Promise<Record<string, any>>;
	saveContext: (i: any, o: any) => Promise<void>;
};
type LCTool = { name: string; description?: string; invoke?: (a: any) => Promise<any>; call?: (a: any) => Promise<any> };

/* Ferramentas nativas do WhatsApp que o agente pode chamar (pré-ligadas na
 * credencial wa-gateway). Cada uma vira uma "function" pro modelo + um
 * executor que bate na REST. */
type WATool = {
	name: string;
	description: string;
	parameters: IDataObject;
	run: (ctx: Ctx, args: IDataObject) => Promise<any>;
};
type Ctx = { fn: IExecuteFunctions; base: string; session: string; chatId: string };

async function waPost(ctx: Ctx, path: string, body: IDataObject) {
	return ctx.fn.helpers.httpRequestWithAuthentication.call(ctx.fn, 'waGatewayApi', {
		method: 'POST', url: ctx.base + path, body, json: true,
	});
}
async function waGet(ctx: Ctx, path: string) {
	return ctx.fn.helpers.httpRequestWithAuthentication.call(ctx.fn, 'waGatewayApi', {
		method: 'GET', url: ctx.base + path, json: true,
	});
}

const WA_TOOLS: Record<string, WATool> = {
	enviar_imagem: {
		name: 'enviar_imagem',
		description: 'Envia uma imagem pro contato desta conversa a partir de uma URL pública.',
		parameters: { type: 'object', required: ['url'], properties: { url: { type: 'string' }, legenda: { type: 'string' } } },
		run: (c, a) => waPost(c, '/api/sendImage', { session: c.session, chatId: c.chatId, url: String(a.url), caption: a.legenda }),
	},
	enviar_audio: {
		name: 'enviar_audio',
		description: 'Envia um áudio/nota de voz a partir de uma URL pública.',
		parameters: { type: 'object', required: ['url'], properties: { url: { type: 'string' }, voz: { type: 'boolean' } } },
		run: (c, a) => waPost(c, '/api/sendAudio', { session: c.session, chatId: c.chatId, url: String(a.url), voice: a.voz !== false }),
	},
	enviar_documento: {
		name: 'enviar_documento',
		description: 'Envia um documento (PDF etc.) a partir de uma URL pública.',
		parameters: { type: 'object', required: ['url'], properties: { url: { type: 'string' }, nome: { type: 'string' }, legenda: { type: 'string' } } },
		run: (c, a) => waPost(c, '/api/sendFile', { session: c.session, chatId: c.chatId, url: String(a.url), filename: a.nome, caption: a.legenda }),
	},
	enviar_localizacao: {
		name: 'enviar_localizacao',
		description: 'Envia uma localização (latitude/longitude).',
		parameters: { type: 'object', required: ['latitude', 'longitude'], properties: { latitude: { type: 'number' }, longitude: { type: 'number' }, nome: { type: 'string' } } },
		run: (c, a) => waPost(c, '/api/sendLocation', { session: c.session, chatId: c.chatId, latitude: a.latitude, longitude: a.longitude, name: a.nome }),
	},
	enviar_botoes: {
		name: 'enviar_botoes',
		description: 'Envia até 3 botões de resposta rápida (só funciona em sessão Cloud API). botoes: [{id,titulo}].',
		parameters: {
			type: 'object', required: ['texto', 'botoes'],
			properties: { texto: { type: 'string' }, rodape: { type: 'string' }, botoes: { type: 'array', items: { type: 'object', properties: { id: { type: 'string' }, titulo: { type: 'string' } } } } },
		},
		run: (c, a) => waPost(c, '/api/sendInteractive', {
			session: c.session, chatId: c.chatId, type: 'button', body: String(a.texto), footer: a.rodape,
			buttons: ((a.botoes as any[]) || []).map((b) => ({ id: String(b.id), title: String(b.titulo) })),
		}),
	},
	checar_numero: {
		name: 'checar_numero',
		description: 'Verifica se um número tem WhatsApp. phone em formato internacional só com dígitos.',
		parameters: { type: 'object', required: ['phone'], properties: { phone: { type: 'string' } } },
		run: (c, a) => waGet(c, `/api/contacts/check?session=${encodeURIComponent(c.session)}&phone=${encodeURIComponent(String(a.phone))}`),
	},
	listar_grupos: {
		name: 'listar_grupos',
		description: 'Lista os grupos da sessão (jid, nome).',
		parameters: { type: 'object', properties: {} },
		run: (c) => waGet(c, `/api/groups?session=${encodeURIComponent(c.session)}`),
	},
	buscar_historico: {
		name: 'buscar_historico',
		description: 'Lê as últimas mensagens desta conversa guardadas no gateway (requer MESSAGE_STORE).',
		parameters: { type: 'object', properties: { limite: { type: 'number' } } },
		run: (c, a) => waGet(c, `/api/chats/${encodeURIComponent(c.chatId)}/messages?session=${encodeURIComponent(c.session)}&limit=${Number(a.limite) || 20}`),
	},
	escalar_para_humano: {
		name: 'escalar_para_humano',
		description: 'Use quando NÃO conseguir resolver e um atendente humano precisa assumir. Passe um motivo curto.',
		parameters: { type: 'object', required: ['motivo'], properties: { motivo: { type: 'string' } } },
		run: async (_c, a) => ({ handoff: true, motivo: String(a.motivo) }),
	},
};

export class WaGatewayAgent implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'wa-gateway Agente',
		name: 'waGatewayAgent',
		icon: 'file:../WaGateway/waGateway.svg',
		group: ['transform'],
		version: 1,
		subtitle: '={{ "sessão: " + $parameter["session"] }}',
		description:
			'Agente de IA nativo do WhatsApp: usa o modelo/memória que você já conectou no n8n, tem ferramentas de WhatsApp embutidas (enviar mídia, botões, checar número…), entende imagem/áudio recebidos e responde sozinho pelo wa-gateway.',
		defaults: { name: 'wa-gateway Agente' },
		inputs: [
			{ type: CONN.main },
			{ type: CONN.model, displayName: 'Modelo', required: true, maxConnections: 1 },
			{ type: CONN.memory, displayName: 'Memória', maxConnections: 1 },
			{ type: CONN.tool, displayName: 'Ferramentas' },
		] as any,
		outputs: [CONN.main, CONN.main, CONN.main] as any,
		outputNames: ['Resposta', 'Handoff', 'Erro'],
		credentials: [{ name: 'waGatewayApi', required: true }],
		properties: [
			{
				displayName:
					'Conecte um <b>Chat Model</b> (OpenAI, Anthropic, Gemini, Ollama…) na entrada "Modelo" — reaproveita a credencial que você já tem no n8n. Opcional: um nó de <b>Memória</b> (chave = chatId do contato) e nós de <b>Ferramenta</b>.',
				name: 'notice', type: 'notice', default: '',
			},
			{ displayName: 'Sessão wa-gateway', name: 'session', type: 'string', required: true, default: '', description: 'Nome da sessão que vai enviar a resposta' },
			{ displayName: 'Chat ID', name: 'chatId', type: 'string', default: '={{ $json.payload.chatId || $json.chatId }}', required: true },
			{ displayName: 'Mensagem do usuário', name: 'input', type: 'string', typeOptions: { rows: 2 }, default: '={{ $json.payload.text || $json.text || $json.body }}' },
			{
				displayName: 'Instruções (system prompt)', name: 'systemPrompt', type: 'string', typeOptions: { rows: 5 },
				default: 'Você é um atendente da empresa no WhatsApp. Responda curto, cordial e em português. Use as ferramentas quando precisar enviar mídia ou consultar algo. Se não resolver, use escalar_para_humano.',
			},
			{
				displayName: 'Ferramentas de WhatsApp embutidas', name: 'waTools', type: 'multiOptions', default: ['enviar_imagem', 'escalar_para_humano'],
				options: Object.values(WA_TOOLS).map((t) => ({ name: t.name, value: t.name, description: t.description })),
			},
			{ displayName: 'Responder no WhatsApp automaticamente', name: 'autoReply', type: 'boolean', default: true, description: 'Whether to mandar a resposta final de texto do agente de volta pro contato via wa-gateway' },
			{ displayName: 'Mostrar "digitando…" enquanto pensa', name: 'typing', type: 'boolean', default: true },
			{ displayName: 'Entender imagem/áudio recebidos', name: 'handleMedia', type: 'boolean', default: true, description: 'Whether to baixar a mídia do evento (payload.mediaMeta/media) e mandar pro modelo (visão / áudio). Depende do modelo suportar.' },
			{ displayName: 'Máx. de passos (tool calls)', name: 'maxSteps', type: 'number', default: 6 },
		],
	};

	async execute(this: IExecuteFunctions): Promise<INodeExecutionData[][]> {
		const items = this.getInputData();
		const outResp: INodeExecutionData[] = [];
		const outHandoff: INodeExecutionData[] = [];
		const outErr: INodeExecutionData[] = [];

		const creds = await this.getCredentials('waGatewayApi');
		const base = String(creds.baseUrl).replace(/\/$/, '');

		const model = (await this.getInputConnectionData(CONN.model, 0)) as LCModel | undefined;
		if (!model || typeof model.invoke !== 'function') {
			throw new NodeOperationError(this.getNode(), 'Conecte um Chat Model na entrada "Modelo".');
		}
		let memory: LCMemory | undefined;
		try { memory = (await this.getInputConnectionData(CONN.memory, 0)) as LCMemory; } catch { memory = undefined; }
		let extraTools: LCTool[] = [];
		try {
			const t = await this.getInputConnectionData(CONN.tool, 0);
			extraTools = (Array.isArray(t) ? t : t ? [t] : []) as LCTool[];
		} catch { extraTools = []; }

		for (let i = 0; i < items.length; i++) {
			try {
				const session = this.getNodeParameter('session', i) as string;
				const chatId = this.getNodeParameter('chatId', i) as string;
				const input = String(this.getNodeParameter('input', i, '') ?? '');
				const systemPrompt = this.getNodeParameter('systemPrompt', i) as string;
				const enabled = this.getNodeParameter('waTools', i, []) as string[];
				const autoReply = this.getNodeParameter('autoReply', i) as boolean;
				const typing = this.getNodeParameter('typing', i) as boolean;
				const handleMedia = this.getNodeParameter('handleMedia', i) as boolean;
				const maxSteps = this.getNodeParameter('maxSteps', i, 6) as number;

				const ctx: Ctx = { fn: this, base, session, chatId };

				if (typing && autoReply) {
					try { await waPost(ctx, '/api/presence', { session, chatId, state: 'typing' }); } catch { /* noop */ }
				}

				// ---- ferramentas: nativas + conectadas ----
				const activeWATools = enabled.map((n) => WA_TOOLS[n]).filter(Boolean);
				const toolSpecs = [
					...activeWATools.map((t) => ({ type: 'function', function: { name: t.name, description: t.description, parameters: t.parameters } })),
					...extraTools,
				];
				const bound = model.bindTools ? model.bindTools(toolSpecs) : model;

				// ---- histórico ----
				let history: any[] = [];
				if (memory) {
					try {
						const vars = await memory.loadMemoryVariables({});
						const h = vars.history ?? vars.chat_history;
						if (Array.isArray(h)) history = h;
						else if (typeof h === 'string' && h.trim()) history = [['system', 'Histórico:\n' + h]];
					} catch { /* memória vazia */ }
				}

				// ---- conteúdo humano (texto + mídia recebida) ----
				const j = items[i].json as IDataObject;
				const payload = (j.payload ?? j) as IDataObject;
				const humanContent = await buildHumanContent.call(this, ctx, input, payload, handleMedia, items[i], i);

				const msgs: any[] = [['system', systemPrompt], ...history, humanContent];

				let finalText = '';
				let handoff: { motivo: string } | null = null;
				const toolsUsed: string[] = [];

				for (let step = 0; step < Math.max(1, maxSteps); step++) {
					const ai = await bound.invoke(msgs);
					msgs.push(ai);
					const calls = ai.tool_calls ?? [];
					if (!calls.length) { finalText = contentToText(ai.content); break; }
					for (const c of calls) {
						toolsUsed.push(c.name);
						let result: any;
						try {
							const wa = activeWATools.find((t) => t.name === c.name);
							if (wa) {
								result = await wa.run(ctx, (c.args || {}) as IDataObject);
								if (result && result.handoff) handoff = { motivo: result.motivo };
							} else {
								const ext = extraTools.find((t) => t.name === c.name);
								result = ext ? await (ext.invoke ? ext.invoke(c.args) : ext.call!(c.args)) : `ferramenta ${c.name} não encontrada`;
							}
						} catch (e) {
							result = 'erro na ferramenta: ' + (e as Error).message;
						}
						msgs.push({ role: 'tool', tool_call_id: c.id ?? c.name, content: typeof result === 'string' ? result : JSON.stringify(result) });
					}
					if (handoff) { finalText = contentToText((msgs[msgs.length - 3] as LCMessage)?.content) || ''; break; }
				}

				if (memory && (input || finalText)) {
					try { await memory.saveContext({ input }, { output: finalText || (handoff ? '[escalado para humano]' : '') }); } catch { /* noop */ }
				}

				if (autoReply && finalText && !handoff) {
					try { await waPost(ctx, '/api/sendText', { session, chatId, text: finalText }); } catch (e) {
						outErr.push({ json: { ...j, _agentError: 'falha ao enviar resposta: ' + (e as Error).message, reply: finalText }, pairedItem: { item: i } });
						continue;
					}
				}

				const base0 = { chatId, reply: finalText, toolsUsed, steps: toolsUsed.length };
				if (handoff) outHandoff.push({ json: { ...j, ...base0, handoff: true, motivo: handoff.motivo }, pairedItem: { item: i } });
				else outResp.push({ json: { ...j, ...base0 }, pairedItem: { item: i } });
			} catch (err) {
				if (this.continueOnFail()) {
					outErr.push({ json: { error: (err as Error).message }, pairedItem: { item: i } });
					continue;
				}
				throw err;
			}
		}
		return [outResp, outHandoff, outErr];
	}
}

/* ---------- helpers ---------- */

function contentToText(c: any): string {
	if (typeof c === 'string') return c;
	if (Array.isArray(c)) return c.map((p) => (typeof p === 'string' ? p : p?.text ?? '')).join('').trim();
	return c == null ? '' : String(c);
}

async function buildHumanContent(
	this: IExecuteFunctions,
	ctx: Ctx,
	input: string,
	payload: IDataObject,
	handleMedia: boolean,
	item: INodeExecutionData,
	itemIndex: number,
): Promise<any> {
	const type = String(payload.type || '');
	if (!handleMedia || !['image', 'audio', 'video', 'document'].includes(type)) {
		return ['human', input || '(sem texto)'];
	}
	// já veio binário anexado pelo Trigger?
	let buf: Buffer | undefined;
	let mime = '';
	try {
		if (item.binary?.data) {
			buf = await this.helpers.getBinaryDataBuffer(itemIndex, 'data');
			mime = item.binary.data.mimeType || '';
		} else {
			const meta = payload.mediaMeta as IDataObject | undefined;
			const mediaUrl = (payload.media as IDataObject | undefined)?.url as string | undefined;
			if (mediaUrl) {
				const r = (await this.helpers.httpRequestWithAuthentication.call(this, 'waGatewayApi', {
					method: 'GET', url: ctx.base + mediaUrl, encoding: 'arraybuffer', returnFullResponse: true,
				})) as any;
				buf = Buffer.from(r.body); mime = r.headers['content-type'] || '';
			} else if (meta) {
				const r = (await this.helpers.httpRequestWithAuthentication.call(this, 'waGatewayApi', {
					method: 'POST', url: `${ctx.base}/api/${encodeURIComponent(ctx.session)}/media/download`,
					body: { type, ...meta }, json: true, encoding: 'arraybuffer', returnFullResponse: true,
				})) as any;
				buf = Buffer.from(r.body); mime = (meta.mimetype as string) || r.headers['content-type'] || '';
			}
		}
	} catch { /* sem mídia -> segue só com texto */ }

	if (!buf) return ['human', input || `(o cliente enviou ${type}, mas não deu pra baixar)`];
	const b64 = buf.toString('base64');
	const text = input || `(o cliente enviou ${type})`;
	if (type === 'image') {
		return { role: 'human', content: [
			{ type: 'text', text },
			{ type: 'image_url', image_url: { url: `data:${mime || 'image/jpeg'};base64,${b64}` } },
		] };
	}
	if (type === 'audio') {
		return { role: 'human', content: [
			{ type: 'text', text },
			{ type: 'input_audio', input_audio: { data: b64, format: (mime.split('/')[1] || 'ogg').split(';')[0] } },
		] };
	}
	return ['human', `${text}\n[anexo ${type} ${mime} — ${Math.round(buf.length / 1024)}KB]`];
}
