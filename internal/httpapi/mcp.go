package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"wa-gateway/internal/engine"
)

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
func timeZero() time.Time { return time.Time{} }

// Servidor MCP (Model Context Protocol) — transporte "Streamable HTTP",
// só ferramentas (JSON-RPC 2.0 em POST /mcp). Deixa um agente de IA usar o
// WhatsApp sem montar chamadas HTTP na mão.

const mcpProtocol = "2024-11-05"

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func rpcOK(id json.RawMessage, result any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": rawOrNull(id), "result": result}
}
func rpcErr(id json.RawMessage, code int, msg string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": rawOrNull(id), "error": map[string]any{"code": code, "message": msg}}
}
func rawOrNull(id json.RawMessage) any {
	if len(id) == 0 {
		return nil
	}
	return id
}

func (d Deps) mcpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Allow", "POST")
		writeErr(w, http.StatusMethodNotAllowed, "use_post", "MCP: envie JSON-RPC via POST")
		return
	}
	var req rpcReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusOK, rpcErr(nil, -32700, "parse error"))
		return
	}
	ctx := r.Context()

	switch req.Method {
	case "initialize":
		writeJSON(w, http.StatusOK, rpcOK(req.ID, map[string]any{
			"protocolVersion": mcpProtocol,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "wa-gateway", "version": d.Version},
		}))
	case "notifications/initialized", "notifications/cancelled":
		w.WriteHeader(http.StatusAccepted)
	case "ping":
		writeJSON(w, http.StatusOK, rpcOK(req.ID, map[string]any{}))
	case "tools/list":
		list := make([]map[string]any, 0, len(mcpTools))
		for _, t := range mcpTools {
			list = append(list, map[string]any{"name": t.name, "description": t.desc, "inputSchema": t.schema})
		}
		writeJSON(w, http.StatusOK, rpcOK(req.ID, map[string]any{"tools": list}))
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		tool := findTool(p.Name)
		if tool == nil {
			writeJSON(w, http.StatusOK, rpcErr(req.ID, -32602, "ferramenta desconhecida: "+p.Name))
			return
		}
		res, err := tool.run(ctx, d, p.Arguments)
		if err != nil {
			writeJSON(w, http.StatusOK, rpcOK(req.ID, map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "erro: " + err.Error()}},
				"isError": true,
			}))
			return
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		writeJSON(w, http.StatusOK, rpcOK(req.ID, map[string]any{
			"content": []any{map[string]any{"type": "text", "text": string(b)}},
		}))
	default:
		writeJSON(w, http.StatusOK, rpcErr(req.ID, -32601, "método não suportado: "+req.Method))
	}
}

/* ---- ferramentas ---- */

type mcpTool struct {
	name, desc string
	schema     map[string]any
	run        func(ctx context.Context, d Deps, a map[string]any) (any, error)
}

func findTool(name string) *mcpTool {
	for i := range mcpTools {
		if mcpTools[i].name == name {
			return &mcpTools[i]
		}
	}
	return nil
}

// obj monta um JSON Schema de objeto rapidamente.
func obj(required []string, props map[string]any) map[string]any {
	return map[string]any{"type": "object", "required": required, "properties": props}
}
func pstr(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func pnum(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
func pbool(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func parr(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

func (d Deps) mcpEngine(session string) (engine.Engine, error) {
	if session == "" {
		return nil, fmt.Errorf("informe 'session'")
	}
	eng, ok := d.Manager.Engine(session)
	if !ok {
		return nil, fmt.Errorf("sessão %q não está ativa neste nó", session)
	}
	return eng, nil
}
func s(a map[string]any, k string) string { v, _ := a[k].(string); return v }
func f(a map[string]any, k string) float64 {
	switch n := a[k].(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}
func b(a map[string]any, k string) bool { v, _ := a[k].(bool); return v }
func slist(a map[string]any, k string) []string {
	arr, _ := a[k].([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if str, ok := x.(string); ok {
			out = append(out, str)
		}
	}
	return out
}

var mcpTools = []mcpTool{
	{
		name: "list_sessions", desc: "Lista as sessões de WhatsApp e o status de cada uma.",
		schema: obj(nil, map[string]any{}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			return d.Manager.List(ctx)
		},
	},
	{
		name: "get_session", desc: "Detalhes e status de uma sessão (jid, status de conexão).",
		schema: obj([]string{"session"}, map[string]any{"session": pstr("nome da sessão")}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			return d.Manager.Get(ctx, s(a, "session"))
		},
	},
	{
		name: "create_session", desc: "Cria uma sessão (dá o nome) e opcionalmente já inicia. Depois use session_qr ou pair_phone pra conectar o número.",
		schema: obj([]string{"session"}, map[string]any{
			"session":    pstr("nome único da sessão (ex.: vendas)"),
			"webhookUrl": pstr("opcional: URL pra receber eventos"),
			"events":     pstr("opcional: eventos do webhook, separados por vírgula (ex.: message,session.status). Default: *"),
			"start":      pbool("iniciar já (default true)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			name := s(a, "session")
			if name == "" {
				return nil, fmt.Errorf("informe 'session'")
			}
			var cfg json.RawMessage
			if u := s(a, "webhookUrl"); u != "" {
				evs := splitComma(s(a, "events"))
				if len(evs) == 0 {
					evs = []string{"*"}
				}
				cfg, _ = json.Marshal(map[string]any{
					"webhooks": []map[string]any{{"url": u, "events": evs}},
				})
			}
			rec, err := d.Manager.Upsert(ctx, name, cfg)
			if err != nil {
				return nil, err
			}
			start := true
			if v, ok := a["start"].(bool); ok {
				start = v
			}
			if start {
				if err := d.Manager.Start(ctx, name); err != nil {
					return nil, fmt.Errorf("criada, mas falhou ao iniciar: %w", err)
				}
				rec, _ = d.Manager.Get(ctx, name)
			}
			return rec, nil
		},
	},
	{
		name: "start_session", desc: "Inicia (ou reconecta) uma sessão já criada.",
		schema: obj([]string{"session"}, map[string]any{"session": pstr("nome da sessão")}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			if err := d.Manager.Start(ctx, s(a, "session")); err != nil {
				return nil, err
			}
			return d.Manager.Get(ctx, s(a, "session"))
		},
	},
	{
		name: "stop_session", desc: "Para uma sessão neste nó (sem deslogar o número).",
		schema: obj([]string{"session"}, map[string]any{"session": pstr("nome da sessão")}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			_ = d.Manager.Stop(ctx, s(a, "session"), false)
			return d.Manager.Get(ctx, s(a, "session"))
		},
	},
	{
		name: "delete_session", desc: "Apaga a sessão (para e remove o registro). Não desloga o device do WhatsApp.",
		schema: obj([]string{"session"}, map[string]any{"session": pstr("nome da sessão")}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			if err := d.Manager.Delete(ctx, s(a, "session")); err != nil {
				return nil, err
			}
			return map[string]any{"deleted": s(a, "session")}, nil
		},
	},
	{
		name: "session_qr", desc: "Devolve o código QR atual da sessão (texto) pra um humano escanear no WhatsApp → Aparelhos conectados. Só existe quando o status é SCAN_QR_CODE.",
		schema: obj([]string{"session"}, map[string]any{"session": pstr("nome da sessão")}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			code := eng.QR()
			if code == "" {
				return map[string]any{"status": string(eng.Status()), "qr": nil, "hint": "sem QR agora; se já pareou, o status vira WORKING"}, nil
			}
			return map[string]any{"status": string(eng.Status()), "qr": code}, nil
		},
	},
	{
		name: "pair_phone", desc: "Gera um código de pareamento por número (alternativa ao QR). O humano digita o código em WhatsApp → Aparelhos conectados → Conectar com número.",
		schema: obj([]string{"session", "phone"}, map[string]any{
			"session": pstr("nome da sessão"), "phone": pstr("número em E.164, só dígitos (ex.: 5517999999999)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			code, err := eng.PairPhone(ctx, s(a, "phone"))
			if err != nil {
				return nil, err
			}
			return map[string]any{"pairingCode": code}, nil
		},
	},
	{
		name: "send_text", desc: "Envia uma mensagem de texto. chatId no formato 5599999999999@s.whatsapp.net (ou ...@g.us para grupo).",
		schema: obj([]string{"session", "chatId", "text"}, map[string]any{
			"session": pstr("sessão"), "chatId": pstr("destino"), "text": pstr("texto"),
			"linkPreview": pbool("anexa preview do 1º link"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			return eng.SendText(ctx, s(a, "chatId"), s(a, "text"), engine.MessageOpts{LinkPreview: b(a, "linkPreview")})
		},
	},
	{
		name: "send_location", desc: "Envia uma localização.",
		schema: obj([]string{"session", "chatId", "latitude", "longitude"}, map[string]any{
			"session": pstr("sessão"), "chatId": pstr("destino"),
			"latitude": pnum("lat"), "longitude": pnum("lng"), "name": pstr("nome do lugar (opcional)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			return eng.SendLocation(ctx, s(a, "chatId"), engine.Location{
				Latitude: f(a, "latitude"), Longitude: f(a, "longitude"), Name: s(a, "name"),
			})
		},
	},
	{
		name: "send_poll", desc: "Cria uma enquete (mínimo 2 opções).",
		schema: obj([]string{"session", "chatId", "name", "options"}, map[string]any{
			"session": pstr("sessão"), "chatId": pstr("destino"),
			"name": pstr("pergunta"), "options": parr("opções"),
			"selectable": pnum("quantas opções podem ser marcadas (padrão 1)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			pick := int(f(a, "selectable"))
			return eng.SendPoll(ctx, s(a, "chatId"), s(a, "name"), slist(a, "options"), pick, engine.MessageOpts{})
		},
	},
	{
		name: "send_reaction", desc: "Reage a uma mensagem com um emoji (emoji vazio remove).",
		schema: obj([]string{"session", "chatId", "messageId", "emoji"}, map[string]any{
			"session": pstr("sessão"), "chatId": pstr("chat da mensagem"), "messageId": pstr("id da mensagem"),
			"emoji": pstr("emoji"), "fromMe": pbool("a mensagem alvo foi enviada por mim"),
			"senderId": pstr("em grupo: jid do autor da mensagem alvo"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			return eng.SendReaction(ctx, engine.MessageRef{
				ChatID: s(a, "chatId"), ID: s(a, "messageId"), FromMe: b(a, "fromMe"), SenderID: s(a, "senderId"),
			}, s(a, "emoji"))
		},
	},
	{
		name: "check_number", desc: "Verifica se um ou mais números têm WhatsApp. phone: E.164, separados por vírgula.",
		schema: obj([]string{"session", "phone"}, map[string]any{
			"session": pstr("sessão"), "phone": pstr("+5599... (vírgula p/ vários)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			return eng.CheckOnWhatsApp(ctx, splitComma(s(a, "phone")))
		},
	},
	{
		name: "list_contacts", desc: "Lista a agenda da sessão (nome, telefone, push name). q filtra por texto; limit corta.",
		schema: obj([]string{"session"}, map[string]any{
			"session": pstr("sessão"), "q": pstr("filtro por nome/telefone"), "limit": pnum("máx de contatos"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			all, err := eng.Contacts(ctx)
			if err != nil {
				return nil, err
			}
			if term := strings.ToLower(strings.TrimSpace(s(a, "q"))); term != "" {
				out := all[:0]
				for _, c := range all {
					if strings.Contains(strings.ToLower(c.FullName+" "+c.FirstName+" "+c.PushName+" "+c.BusinessName), term) ||
						strings.Contains(c.Phone, term) {
						out = append(out, c)
					}
				}
				all = out
			}
			if n := int(f(a, "limit")); n > 0 && n < len(all) {
				all = all[:n]
			}
			return map[string]any{"contacts": all, "count": len(all)}, nil
		},
	},
	{
		name: "get_contact_info", desc: "Perfil público de um contato (status, foto id, verificado).",
		schema: obj([]string{"session", "jid"}, map[string]any{
			"session": pstr("sessão"), "jid": pstr("5599...@s.whatsapp.net (vírgula p/ vários)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			return eng.GetUserInfo(ctx, splitComma(s(a, "jid")))
		},
	},
	{
		name: "get_profile_picture", desc: "URL da foto de perfil de um contato ou grupo.",
		schema: obj([]string{"session", "jid"}, map[string]any{
			"session": pstr("sessão"), "jid": pstr("jid do contato ou grupo"), "preview": pbool("miniatura"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			url, err := eng.GetProfilePicture(ctx, s(a, "jid"), b(a, "preview"))
			if err != nil {
				return nil, err
			}
			return map[string]string{"url": url}, nil
		},
	},
	{
		name: "list_groups", desc: "Lista os grupos da sessão com jid, nome e nº de participantes.",
		schema: obj([]string{"session"}, map[string]any{"session": pstr("sessão")}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			return eng.ListGroups(ctx)
		},
	},
	{
		name: "list_chats", desc: "Conversas recentes guardadas (histórico). Requer MESSAGE_STORE ligado.",
		schema: obj([]string{"session"}, map[string]any{
			"session": pstr("sessão"), "limit": pnum("máx. de conversas (padrão 50)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			return d.Store.ListChats(ctx, s(a, "session"), int(f(a, "limit")))
		},
	},
	{
		name: "chat_history", desc: "Últimas mensagens de uma conversa (do mais recente pro mais antigo).",
		schema: obj([]string{"session", "chatId"}, map[string]any{
			"session": pstr("sessão"), "chatId": pstr("jid da conversa"), "limit": pnum("máx. (padrão 30)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			lim := int(f(a, "limit"))
			if lim == 0 {
				lim = 30
			}
			return d.Store.ListMessages(ctx, s(a, "session"), s(a, "chatId"), lim, timeZero())
		},
	},
	{
		name: "send_image", desc: "Envia uma imagem a partir de uma URL pública. O gateway baixa e reenvia.",
		schema: obj([]string{"session", "chatId", "url"}, map[string]any{
			"session": pstr("sessão"), "chatId": pstr("destino"),
			"url": pstr("URL http(s) da imagem"), "caption": pstr("legenda (opcional)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			data, mime, err := fetchURL(ctx, s(a, "url"), 16<<20)
			if err != nil {
				return nil, err
			}
			return eng.SendImage(ctx, s(a, "chatId"), data, mime, s(a, "caption"))
		},
	},
	{
		name: "send_file", desc: "Envia um documento a partir de uma URL pública.",
		schema: obj([]string{"session", "chatId", "url"}, map[string]any{
			"session": pstr("sessão"), "chatId": pstr("destino"),
			"url": pstr("URL http(s) do arquivo"), "filename": pstr("nome do arquivo (opcional)"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			data, mime, err := fetchURL(ctx, s(a, "url"), 64<<20)
			if err != nil {
				return nil, err
			}
			name := s(a, "filename")
			if name == "" {
				name = "arquivo"
			}
			return eng.SendFile(ctx, s(a, "chatId"), engine.Media{Data: data, Mimetype: mime, Filename: name})
		},
	},
	{
		name: "mark_read", desc: "Marca uma mensagem como lida (envia o recibo de leitura).",
		schema: obj([]string{"session", "chatId", "messageId"}, map[string]any{
			"session": pstr("sessão"), "chatId": pstr("chat"), "messageId": pstr("id da mensagem"),
			"fromMe": pbool("a mensagem foi enviada por mim"), "senderId": pstr("grupo: jid do autor"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			ref := engine.MessageRef{ChatID: s(a, "chatId"), ID: s(a, "messageId"), FromMe: b(a, "fromMe"), SenderID: s(a, "senderId")}
			if err := eng.MarkRead(ctx, ref); err != nil {
				return nil, err
			}
			return map[string]bool{"ok": true}, nil
		},
	},
	{
		name: "create_group", desc: "Cria um grupo com uma lista de participantes.",
		schema: obj([]string{"session", "name", "participants"}, map[string]any{
			"session": pstr("sessão"), "name": pstr("nome do grupo"),
			"participants": parr("jids 5599...@s.whatsapp.net"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			return eng.CreateGroup(ctx, s(a, "name"), slist(a, "participants"))
		},
	},
	{
		name: "group_participants", desc: "Adiciona/remove/promove/rebaixa participantes de um grupo.",
		schema: obj([]string{"session", "groupJid", "action", "participants"}, map[string]any{
			"session": pstr("sessão"), "groupJid": pstr("...@g.us"),
			"action":       map[string]any{"type": "string", "enum": []string{"add", "remove", "promote", "demote"}},
			"participants": parr("jids"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			return eng.UpdateParticipants(ctx, s(a, "groupJid"), engine.ParticipantAction(s(a, "action")), slist(a, "participants"))
		},
	},
	{
		name: "forward_message", desc: "Encaminha uma mensagem guardada (por id) para outro chat.",
		schema: obj([]string{"session", "toChatId", "messageId"}, map[string]any{
			"session": pstr("sessão"), "toChatId": pstr("destino"), "messageId": pstr("id da mensagem guardada"),
		}),
		run: func(ctx context.Context, d Deps, a map[string]any) (any, error) {
			eng, err := d.mcpEngine(s(a, "session"))
			if err != nil {
				return nil, err
			}
			rec, err := d.Store.GetMessage(ctx, s(a, "session"), s(a, "messageId"))
			if err != nil {
				return nil, err
			}
			return eng.Forward(ctx, s(a, "toChatId"), engine.ForwardSource{Type: rec.Type, Body: rec.Body, Media: rec.Media})
		},
	},
}

// fetchURL baixa uma URL http(s) com limite de tamanho e devolve bytes+mime.
func fetchURL(ctx context.Context, url string, max int64) ([]byte, string, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, "", fmt.Errorf("url inválida (precisa começar com http/https)")
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("URL respondeu %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max))
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}
