# wa-gateway — Servidor MCP

O wa-gateway expõe um servidor **MCP (Model Context Protocol)** em
`POST /mcp` — transporte *Streamable HTTP*, JSON-RPC 2.0, só ferramentas.
Um agente de IA usa o WhatsApp **sem montar chamadas HTTP na mão**: cria
sessão, conecta número, envia mensagem, lê histórico, mexe em grupo.

Auth: o mesmo `X-Api-Key` da REST (ou `Authorization: Bearer`).
`protocolVersion`: `2024-11-05`.

---

## Ligar num cliente MCP

### Claude Desktop / Claude Code (`mcp.json`)
```jsonc
{
  "mcpServers": {
    "wa-gateway": {
      "type": "http",
      "url": "https://SEU-GATEWAY/mcp",
      "headers": { "X-Api-Key": "SUA_API_KEY" }
    }
  }
}
```

### n8n (nó *MCP Client*)
- Endpoint: `https://SEU-GATEWAY/mcp`
- Header: `X-Api-Key: SUA_API_KEY`

### Qualquer agente — handshake mínimo
```bash
BASE=https://SEU-GATEWAY ; KEY=SUA_API_KEY

# 1. initialize
curl -s $BASE/mcp -H "X-Api-Key: $KEY" -H 'content-type: application/json' -d '{
  "jsonrpc":"2.0","id":1,"method":"initialize",
  "params":{"protocolVersion":"2024-11-05","capabilities":{}}
}'

# 2. listar ferramentas
curl -s $BASE/mcp -H "X-Api-Key: $KEY" -H 'content-type: application/json' -d '{
  "jsonrpc":"2.0","id":2,"method":"tools/list"
}'
```

---

## Do zero ao "oi" mandado — só por MCP

```bash
BASE=https://SEU-GATEWAY ; KEY=SUA_API_KEY
call(){ curl -s $BASE/mcp -H "X-Api-Key: $KEY" -H 'content-type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}"; }

# 1. cria a sessão "vendas" e já inicia
call create_session '{"session":"vendas","webhookUrl":"https://meu-n8n/webhook/abc","events":"message,session.status"}'

# 2. pega o código de pareamento por número (um humano digita no celular)
call pair_phone '{"session":"vendas","phone":"5517999999999"}'
#    -> { "pairingCode": "ABCD-EFGH" }
#    (ou: call session_qr '{"session":"vendas"}' pra escanear o QR)

# 3. confere que conectou
call get_session '{"session":"vendas"}'      # status: "WORKING"

# 4. manda a mensagem
call send_text '{"session":"vendas","chatId":"5517988887777@s.whatsapp.net","text":"oi, tudo certo?"}'
```

> O agente **não escaneia QR nem digita código** — ele entrega o
> `pairingCode` / `qr` pra um humano. O resto (criar, iniciar, enviar, ler)
> é 100% automatizável.

---

## Ferramentas (20)

### Sessão / conexão
| Tool | Faz |
|---|---|
| `list_sessions` | lista todas as sessões e status |
| `get_session` | status/jid de uma sessão |
| `create_session` | cria (dá o nome) e opcionalmente inicia; aceita `webhookUrl`/`events` |
| `start_session` | inicia/reconecta |
| `stop_session` | para neste nó (não desloga) |
| `delete_session` | apaga o registro |
| `session_qr` | QR atual (texto) pra um humano escanear |
| `pair_phone` | código de pareamento por número (E.164) |

### Envio
| Tool | Faz |
|---|---|
| `send_text` | texto (`linkPreview` opcional) |
| `send_image` | imagem a partir de URL pública (o gateway baixa e reenvia) |
| `send_file` | documento a partir de URL pública |
| `send_location` | latitude/longitude |
| `send_poll` | enquete (≥2 opções) |
| `send_reaction` | reage com emoji (vazio remove) |
| `mark_read` | envia recibo de leitura |
| `forward_message` | encaminha uma mensagem guardada (por id) |

### Consulta
| Tool | Faz |
|---|---|
| `check_number` | número(s) têm WhatsApp? |
| `list_contacts` | agenda da sessão (`q` filtra, `limit` corta) |
| `get_contact_info` | perfil público (status, verificado) |
| `get_profile_picture` | URL da foto |
| `list_groups` | grupos da sessão |
| `list_chats` | conversas recentes (requer `MESSAGE_STORE=on`) |
| `chat_history` | últimas mensagens de uma conversa |

### Grupos
| Tool | Faz |
|---|---|
| `create_group` | cria com lista de participantes |
| `group_participants` | add / remove / promote / demote |

---

## Formato da resposta

`tools/call` devolve o resultado como texto JSON dentro de `content`:
```json
{ "jsonrpc":"2.0","id":1,"result":{
    "content":[{"type":"text","text":"{ \"id\": \"3EB0...\", \"timestamp\": 1712... }"}]
}}
```
Erro de ferramenta vem com `"isError": true` e a mensagem em `content[0].text`.

---

## Notas

- `create_session`/`start_session` respeitam o **lock de posse**: num cenário
  multi-nó, a sessão sobe no nó que pegou o lock.
- Ferramentas de envio exigem a sessão **ativa neste nó** (senão erro
  "sessão X não está ativa neste nó").
- Não há ferramenta pra criar API key por MCP — isso é operação de admin,
  feita pela REST (`POST /api/keys`) ou console.
