package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ep descreve um endpoint para a spec OpenAPI. `body` e um exemplo de corpo
// (POST/PUT); `query` sao parametros de query (GET). Campos terminados em "?"
// sao opcionais.
type ep struct {
	method, path, tag, summary string
	body                       map[string]any
	query                      []string
}

// specEndpoints e a fonte de verdade da API para integracao (n8n, agentes de
// IA, SaaS). Servida como OpenAPI 3 em /openapi.json e Swagger UI em /docs.
var specEndpoints = []ep{
	// ---- sessões
	{"GET", "/api/sessions", "Sessions", "Listar sessões", nil, nil},
	{"POST", "/api/sessions", "Sessions", "Criar sessão", map[string]any{"name": "default", "start": true, "config": map[string]any{}}, nil},
	{"GET", "/api/sessions/{session}", "Sessions", "Ver sessão", nil, nil},
	{"PUT", "/api/sessions/{session}", "Sessions", "Atualizar config", map[string]any{"config": map[string]any{"webhooks": []any{map[string]any{"url": "https://…", "events": []string{"message"}}}}}, nil},
	{"DELETE", "/api/sessions/{session}", "Sessions", "Apagar sessão", nil, nil},
	{"POST", "/api/sessions/{session}/start", "Sessions", "Iniciar", map[string]any{}, nil},
	{"POST", "/api/sessions/{session}/stop", "Sessions", "Parar", map[string]any{}, nil},
	{"POST", "/api/sessions/{session}/restart", "Sessions", "Reiniciar", map[string]any{}, nil},
	{"POST", "/api/sessions/{session}/logout", "Sessions", "Deslogar", map[string]any{}, nil},
	{"GET", "/api/{session}/auth/qr", "Sessions", "QR (texto)", nil, nil},
	{"GET", "/api/{session}/auth/qr.png", "Sessions", "QR (imagem PNG)", nil, nil},
	{"POST", "/api/sessions/{session}/auth/pair-code", "Sessions", "Parear por código (sem QR)", map[string]any{"phone": "5517999999999"}, nil},
	{"POST", "/api/sessions/{session}/webhook/test", "Sessions", "Disparar evento de teste nos webhooks", map[string]any{}, nil},

	// ---- perfil
	{"GET", "/api/{session}/me", "Profile", "Meu perfil", nil, nil},
	{"PUT", "/api/{session}/profile/status", "Profile", "Mudar meu recado", map[string]any{"status": "disponível"}, nil},
	{"POST", "/api/{session}/presence", "Profile", "Presença global (online/offline)", map[string]any{"available": true}, nil},
	{"GET", "/api/{session}/blocklist", "Profile", "Lista de bloqueados", nil, nil},
	{"POST", "/api/{session}/block", "Profile", "Bloquear / desbloquear", map[string]any{"jid": "5517…@s.whatsapp.net", "block": true}, nil},

	// ---- envio
	{"POST", "/api/sendText", "Messaging", "Enviar texto. callbackUrl (opcional): POST quando a msg for entregue/lida/falhar.", map[string]any{"session": "default", "chatId": "5517999999999@s.whatsapp.net", "text": "olá", "linkPreview": false, "enqueue": false, "callbackUrl": "", "callbackData": map[string]any{}}, nil},
	{"POST", "/api/sendImage", "Messaging", "Enviar imagem. `data` (base64/data URI) OU `url` (o gateway baixa)", map[string]any{"session": "default", "chatId": "…@s.whatsapp.net", "url": "https://…/foto.jpg", "caption": "", "mimetype": ""}, nil},
	{"POST", "/api/sendFile", "Messaging", "Enviar documento. `data` OU `url`", map[string]any{"session": "default", "chatId": "…", "url": "https://…/doc.pdf", "filename": "doc.pdf", "mimetype": ""}, nil},
	{"POST", "/api/sendVideo", "Messaging", "Enviar vídeo. `data` OU `url`", map[string]any{"session": "default", "chatId": "…", "url": "https://…/video.mp4", "caption": ""}, nil},
	{"POST", "/api/sendAudio", "Messaging", "Enviar áudio. `data` OU `url`", map[string]any{"session": "default", "chatId": "…", "url": "https://…/audio.ogg", "voice": true}, nil},
	{"POST", "/api/sendSticker", "Messaging", "Enviar sticker (webp)", map[string]any{"session": "default", "chatId": "…", "data": "<base64 webp>"}, nil},
	{"POST", "/api/sendLocation", "Messaging", "Enviar localização", map[string]any{"session": "default", "chatId": "…", "latitude": -20.8, "longitude": -49.4, "name": ""}, nil},
	{"POST", "/api/sendContact", "Messaging", "Enviar contato", map[string]any{"session": "default", "chatId": "…", "contacts": []any{map[string]any{"name": "Fulano", "phone": "+5517…"}}}, nil},
	{"POST", "/api/sendPoll", "Messaging", "Enviar enquete", map[string]any{"session": "default", "chatId": "…", "name": "Pergunta?", "options": []string{"A", "B"}, "selectable": 1}, nil},
	{"POST", "/api/forwardMessage", "Messaging", "Encaminhar mensagem guardada", map[string]any{"session": "default", "toChatId": "…", "messageId": "<id>"}, nil},
	{"POST", "/api/reaction", "Messaging", "Reagir", map[string]any{"session": "default", "chatId": "…", "messageId": "<id>", "emoji": "👍", "fromMe": false}, nil},
	{"POST", "/api/editMessage", "Messaging", "Editar mensagem", map[string]any{"session": "default", "chatId": "…", "messageId": "<id>", "text": "novo", "fromMe": true}, nil},
	{"POST", "/api/deleteMessage", "Messaging", "Apagar (revoke)", map[string]any{"session": "default", "chatId": "…", "messageId": "<id>", "fromMe": true}, nil},
	{"POST", "/api/sendSeen", "Messaging", "Marcar como lida", map[string]any{"session": "default", "chatId": "…", "messageId": "<id>"}, nil},
	{"POST", "/api/presence", "Messaging", "Presença no chat (digitando…)", map[string]any{"session": "default", "chatId": "…", "state": "typing"}, nil},
	{"POST", "/api/sendInteractive", "Messaging", "Botões / lista / CTA (só engine cloud)", map[string]any{"session": "default", "chatId": "…", "type": "button", "body": "Escolha:", "buttons": []any{map[string]any{"id": "1", "title": "Sim"}, map[string]any{"id": "2", "title": "Não"}}}, nil},
	{"POST", "/api/sendTemplate", "Messaging", "Template aprovado (só engine cloud)", map[string]any{"session": "default", "chatId": "…", "name": "hello_world", "language": "en_US", "components": []any{}}, nil},
	{"GET", "/api/{session}/cloud/webhook", "Cloud API", "Webhook da Meta (verificação + recebimento) — sem X-Api-Key", nil, nil},

	// ---- labels (Business)
	{"GET", "/api/{session}/labels", "Labels", "Listar etiquetas", nil, nil},
	{"POST", "/api/{session}/labels", "Labels", "Criar/editar/apagar etiqueta", map[string]any{"labelId": "1", "name": "Cliente VIP", "color": 0, "deleted": false}, nil},
	{"POST", "/api/{session}/labels/chat", "Labels", "Etiquetar um chat", map[string]any{"chatId": "…@s.whatsapp.net", "labelId": "1", "on": true}, nil},
	{"POST", "/api/{session}/labels/message", "Labels", "Etiquetar uma mensagem", map[string]any{"chatId": "…", "messageId": "<id>", "labelId": "1", "on": true}, nil},

	// ---- contatos
	{"GET", "/api/contacts", "Contacts", "Listar agenda da sessão", nil, []string{"session", "q?", "limit?"}},
	{"GET", "/api/contacts/check", "Contacts", "Número está no WhatsApp?", nil, []string{"session", "phone"}},
	{"GET", "/api/contacts/info", "Contacts", "Info de perfil", nil, []string{"session", "jid"}},
	{"GET", "/api/contacts/profile-picture", "Contacts", "Foto de perfil", nil, []string{"session", "jid", "preview?"}},

	// ---- grupos
	{"GET", "/api/groups", "Groups", "Listar grupos", nil, []string{"session"}},
	{"POST", "/api/groups", "Groups", "Criar grupo", map[string]any{"session": "default", "name": "Grupo", "participants": []string{"5517…@s.whatsapp.net"}}, nil},
	{"POST", "/api/groups/join", "Groups", "Entrar por link", map[string]any{"session": "default", "code": "<código>"}, nil},
	{"GET", "/api/groups/{jid}", "Groups", "Info do grupo", nil, []string{"session"}},
	{"POST", "/api/groups/{jid}/leave", "Groups", "Sair do grupo", map[string]any{"session": "default"}, nil},
	{"POST", "/api/groups/{jid}/participants", "Groups", "Add/remove/promote/demote", map[string]any{"session": "default", "action": "add", "participants": []string{"5517…@s.whatsapp.net"}}, nil},
	{"PUT", "/api/groups/{jid}/name", "Groups", "Renomear", map[string]any{"session": "default", "name": "Novo nome"}, nil},
	{"PUT", "/api/groups/{jid}/topic", "Groups", "Tópico", map[string]any{"session": "default", "topic": "…"}, nil},
	{"PUT", "/api/groups/{jid}/photo", "Groups", "Foto do grupo", map[string]any{"session": "default", "data": "<base64 jpeg>"}, nil},
	{"PUT", "/api/groups/{jid}/announce", "Groups", "Só admins enviam", map[string]any{"session": "default", "enabled": true}, nil},
	{"PUT", "/api/groups/{jid}/locked", "Groups", "Só admins editam infos", map[string]any{"session": "default", "enabled": true}, nil},
	{"GET", "/api/groups/{jid}/invite-link", "Groups", "Link de convite", nil, []string{"session", "reset?"}},

	// ---- histórico
	{"GET", "/api/chats", "History", "Listar conversas", nil, []string{"session", "limit?"}},
	{"GET", "/api/chats/{chatId}/messages", "History", "Histórico da conversa", nil, []string{"session", "limit?", "before?"}},
	{"GET", "/api/messages/{id}/download", "History", "Baixar mídia de mensagem guardada", nil, []string{"session"}},
	{"POST", "/api/{session}/media/download", "History", "Baixar mídia direto do mediaMeta do evento (sem depender do store)", map[string]any{"type": "image", "directPath": "/v/…", "mimetype": "image/jpeg", "mediaKey": "<base64>", "fileEncSha256": "<base64>", "fileSha256": "<base64>", "fileLength": 12345}, nil},

	// ---- monitor
	{"GET", "/api/outbox", "Monitor", "Jobs da fila de saída", nil, []string{"session", "limit?"}},
	{"GET", "/api/deliveries", "Monitor", "Entregas de webhook", nil, []string{"session", "limit?"}},
	{"GET", "/api/media/{id}", "Monitor", "Baixar/stream de mídia guardada", nil, []string{"redirect?"}},
	{"DELETE", "/api/media/{id}", "Monitor", "Apagar uma mídia do storage + registro", nil, nil},
	{"POST", "/api/{session}/media/purge", "Monitor", "Apagar TODAS as mídias da sessão (ou só olderThan)", map[string]any{"olderThan": "168h"}, nil},
	{"GET", "/api/stats", "Monitor", "Estatísticas", nil, nil},

	// ---- leads / CRM
	{"GET", "/api/{session}/leads", "Leads", "Lista os leads (contatos) com métricas de conversa e origem", nil, []string{"status?", "stage?", "tag?", "q?", "source?", "sort?", "limit?", "offset?"}},
	{"GET", "/api/{session}/leads/stats", "Leads", "Funil: contagem por status/etapa, tempo médio de resposta, leads de anúncio/UTM", nil, nil},
	{"GET", "/api/{session}/leads/{chatId}", "Leads", "Detalhe de um lead (chatId ou só o número)", nil, nil},
	{"GET", "/api/{session}/polls/{messageId}", "Messaging", "Placar de uma enquete criada por esta sessão (opções + quem votou). Cache Redis ~30d. Votos em tempo real chegam pelo evento message.poll_vote.", nil, nil},
	{"PATCH", "/api/{session}/leads/{chatId}", "Leads", "Atualiza campos de CRM do lead", map[string]any{"stage": "qualificado", "owner": "ana", "tags": []string{"quente"}, "notes": "pediu proposta", "status": "closed"}, nil},

	// ---- OTP (código de verificação)
	{"POST", "/api/{session}/otp/send", "OTP", "Gera e envia um código de verificação por WhatsApp. callbackUrl (opcional) recebe um POST quando a msg for entregue.", map[string]any{"to": "5517999999999", "brand": "ACME", "codeLength": 6, "ttlSeconds": 300, "callbackUrl": "https://meuapp/otp-status", "callbackData": map[string]any{"userId": 123}}, nil},
	{"POST", "/api/{session}/otp/verify", "OTP", "Confere o código (por to ou por id)", map[string]any{"to": "5517999999999", "code": "123456"}, nil},
	{"POST", "/api/{session}/otp/cancel", "OTP", "Invalida o código ativo de um número", map[string]any{"to": "5517999999999"}, nil},

	// ---- campanhas (envio em massa)
	{"POST", "/api/{session}/campaign", "Campaigns", "Criar campanha (envio em massa pausado pelo pacing da fila)", map[string]any{"name": "promo julho", "kind": "text", "text": "Oi! Novidades...", "recipients": []string{"5517999999999", "5517888888888"}, "minIntervalMs": 5000, "jitterMs": 3000}, nil},
	{"GET", "/api/campaigns", "Campaigns", "Listar campanhas", nil, []string{"session?", "limit?"}},
	{"GET", "/api/campaigns/{id}", "Campaigns", "Detalhe da campanha (contagem por status + falhas)", nil, nil},
	{"POST", "/api/campaigns/{id}/stop", "Campaigns", "Parar a campanha (barra os jobs ainda não enviados)", map[string]any{}, nil},

	{"GET", "/api/cluster", "Admin", "Info do nó e nós vivos (multi-nó)", nil, nil},
	{"POST", "/api/deliveries/{id}/retry", "Admin", "Reenfileira uma entrega de webhook", nil, nil},

	// ---- mcp
	{"POST", "/mcp", "MCP", "Endpoint MCP (JSON-RPC 2.0) para agentes de IA. Métodos: initialize, tools/list, tools/call.", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}, nil},

	// ---- keys
	{"GET", "/api/keys", "Keys", "Listar API keys", nil, nil},
	{"POST", "/api/keys", "Keys", "Criar API key", map[string]any{"label": "app-x", "scopes": []string{"*"}}, nil},
	{"DELETE", "/api/keys/{id}", "Keys", "Revogar API key", nil, nil},
}

// openapiDoc monta o documento OpenAPI 3.0.3.
func (d Deps) openapiDoc(scheme, host string) map[string]any {
	paths := map[string]any{}
	for _, e := range specEndpoints {
		item, ok := paths[e.path].(map[string]any)
		if !ok {
			item = map[string]any{}
			paths[e.path] = item
		}
		op := map[string]any{
			"tags":        []string{e.tag},
			"summary":     e.summary,
			"operationId": opID(e.method, e.path),
			"responses": map[string]any{
				"200": map[string]any{"description": "ok"},
				"201": map[string]any{"description": "criado"},
				"400": map[string]any{"description": "requisição inválida"},
				"401": map[string]any{"description": "API key ausente/ inválida"},
				"409": map[string]any{"description": "sessão não ativa neste nó"},
			},
		}
		var params []any
		for _, seg := range pathParams(e.path) {
			params = append(params, map[string]any{
				"name": seg, "in": "path", "required": true,
				"schema": map[string]any{"type": "string"},
			})
		}
		for _, q := range e.query {
			name, req := strings.TrimSuffix(q, "?"), !strings.HasSuffix(q, "?")
			params = append(params, map[string]any{
				"name": name, "in": "query", "required": req,
				"schema": map[string]any{"type": "string"},
			})
		}
		if len(params) > 0 {
			op["parameters"] = params
		}
		if e.body != nil {
			op["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema":  map[string]any{"type": "object"},
						"example": e.body,
					},
				},
			}
		}
		item[strings.ToLower(e.method)] = op
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "wa-gateway",
			"version":     d.Version,
			"description": "API HTTP multi-sessão de WhatsApp. Autentique com o header `X-Api-Key`.",
		},
		"servers": []any{map[string]any{"url": scheme + "://" + host}},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"ApiKey": map[string]any{"type": "apiKey", "in": "header", "name": "X-Api-Key"},
			},
		},
		"security": []any{map[string]any{"ApiKey": []any{}}},
		"tags": []any{
			map[string]any{"name": "Sessions"}, map[string]any{"name": "Profile"},
			map[string]any{"name": "Messaging"}, map[string]any{"name": "Contacts"},
			map[string]any{"name": "Groups"}, map[string]any{"name": "History"},
			map[string]any{"name": "Monitor"}, map[string]any{"name": "Keys"},
			map[string]any{"name": "Campaigns", "description": "Envio em massa pausado pelo pacing anti-ban."},
			map[string]any{"name": "OTP", "description": "Código de verificação de número por WhatsApp (estilo Verify)."},
			map[string]any{"name": "MCP", "description": "Model Context Protocol — ferramentas p/ agentes de IA."},
		},
		"paths": paths,
	}
}

func (d Deps) openapiJSON(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil && !strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") {
		if r.Host == "" || strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.") {
			scheme = "http"
		}
	}
	host := r.Host
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(d.openapiDoc(scheme, host))
}

func (d Deps) docsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerHTML))
}

func opID(method, path string) string {
	s := strings.ToLower(method) + " " + path
	repl := strings.NewReplacer("/", "_", "{", "", "}", "", ".", "_", " ", "_", "-", "_")
	return strings.Trim(repl.Replace(s), "_")
}

func pathParams(path string) []string {
	var out []string
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			out = append(out, seg[1:len(seg)-1])
		}
	}
	return out
}

const swaggerHTML = `<!doctype html><html><head><meta charset="utf-8">
<title>wa-gateway · API</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
<style>body{margin:0}.topbar{display:none}</style></head>
<body><div id="ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({ url: "/openapi.json", dom_id: "#ui", persistAuthorization: true,
  presets: [SwaggerUIBundle.presets.apis], layout: "BaseLayout" });
</script></body></html>`
