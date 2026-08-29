# wa-gateway

![ci](https://github.com/fdsriopreto-code/wa-gateway/actions/workflows/ci.yml/badge.svg)

API HTTP multi-sessão de WhatsApp em Go. Reescrita enxuta inspirada no
teardown de Evolution API e WAHA — ver `../repos-analise/blueprint-wa-api-go.html`.

**Stack:** Go · [whatsmeow](https://github.com/tulir/whatsmeow) · PostgreSQL (fonte de verdade)
· Redis (cache · fila · locks · pub/sub) · chi · asynq.

## Estado — Fase 1 (MVP)

| Pronto | Item |
|---|---|
| ✅ | Config por env, logs estruturados (`slog`), `/health`, `/metrics` (Prometheus) |
| ✅ | Postgres com migrações embarcadas (goose) |
| ✅ | Redis: locks de posse de sessão, cache, pub/sub |
| ✅ | Barramento de eventos in-process com match por wildcard |
| ✅ | Engine `whatsmeow` — **tradução de todos os eventos** (mapeados + fallback `engine.*`) |
| ✅ | `SessionManager`: ciclo de vida, lock de posse (base p/ multi-nó), restore no boot |
| ✅ | Webhook: envelope estilo WAHA, HMAC-SHA256, entrega durável via asynq/Redis com retry |
| ✅ | WebSocket `/ws` best-effort, filtro por sessão + wildcard de eventos |
| ✅ | Auth por API key (chave-mestra de env + tabela `api_keys` com Argon2id) |
| ✅ | REST: sessões (CRUD + start/stop/logout/restart), QR, `sendText`, `sendImage` |
| ✅ | Fase 2 — envio: `sendFile`, `sendVideo`, `sendAudio` (nota de voz), `sendLocation`, `sendContact` |
| ✅ | Fase 2 — mensagens: `reaction`, `deleteMessage`, `editMessage`, `sendSeen`, `presence` (typing/recording) |
| ✅ | Fase 2 — contatos: checar número, info de perfil, foto de perfil |
| ✅ | Fase 2 — grupos: listar, info, criar, sair, participantes (add/remove/promote/demote), nome, tópico, invite link, entrar por link |
| ✅ | Fase 2 — fila de saída (`enqueue`): pacing por sessão via slot no Redis + jitter + limite diário (anti-ban) |
| ✅ | Fase 2 — mídia S3/MinIO opcional (`MEDIA_BACKEND=s3`) + `GET /api/media/{id}` |
| ✅ | Console web embarcado em `/` (sessões + QR, envio, grupos, contatos, fila, webhooks, eventos ao vivo, API keys) |
| ✅ | Fase 2 — reply/menções/link preview, sticker, enquete, parear-por-código, perfil (`/me`, recado, presença, block), grupo (foto/announce/locked) |
| ✅ | Fase 2 — persistência de mensagens/chats (`MESSAGE_STORE=on`): `/api/chats`, histórico, `/api/messages/{id}/download`, `forwardMessage` |
| ⬜ | Fase 2 (resto): labels do Business |
| ✅ | Integração: OpenAPI (`/openapi.json`) + Swagger (`/docs`), idempotência, `/ready`, teste de webhook |
| ✅ | **Servidor MCP** (`POST /mcp`) — 12 ferramentas para agentes de IA |
| ⬜ | Fase 3: multi-sessão por processo + roteamento entre nós + fan-out WS via Redis |
| ⬜ | Fase 4 (resto): plugin NATS/AMQP, conector de bot, Chatwoot |

## Rodar

### Com Docker (tudo junto)

```bash
docker compose up -d --build
curl -s localhost:3000/health | jq
```

### Local (Postgres + Redis via compose, app pelo Go)

```bash
docker compose up -d postgres redis
cp .env.example .env            # ajuste se precisar
set -a && . ./.env && set +a    # carrega o .env no shell (bash)
go run ./cmd/wa-gateway
```

No PowerShell, defina as variáveis com `$env:DATABASE_URL="..."` etc., ou use o compose.

## Integração (n8n, agentes de IA, SaaS)

- **OpenAPI 3** em **`GET /openapi.json`** e **Swagger UI** em **`GET /docs`** —
  fonte de verdade da API. Autenticação: header `X-Api-Key`.
- **Idempotência:** mande o header `Idempotency-Key: <uuid>` em qualquer `POST`.
  Retentativas (timeout do n8n, retry do fluxo) devolvem a mesma resposta sem
  reenviar a mensagem. Cache de 10 min por chave+API-key.
- **Teste de webhook:** `POST /api/sessions/{session}/webhook/test` dispara um
  evento sintético (`webhook.test`) para cada URL configurada e devolve o
  status HTTP de cada uma — valide o receptor antes de ligar de verdade.
- **Readiness:** `GET /ready` (200 só com o Postgres respondendo) para o
  health check do orquestrador.

### n8n — receber mensagens

1. No n8n, crie um workflow com o node **Webhook** (method `POST`), copie a URL.
2. No wa-gateway, salve na config da sessão:
   `{"webhooks":[{"url":"<url-do-n8n>","events":["message"],"hmac":{"secret":"…"}}]}`
   (ou use o editor visual no console → **Configurar**).
3. Clique **Testar webhooks** — deve chegar um `webhook.test` no n8n.
4. Cada mensagem recebida chega como o payload achatado (`id`, `chatId`,
   `from`, `body`, `type`, `media`…). Assine `message` (só recebidas) ou
   `message.any` (tudo).

### n8n — enviar / chamar a API

Use o node **HTTP Request**: `POST {BASE}/api/sendText`, header
`X-Api-Key: {KEY}`, body JSON `{"session":"default","chatId":"…","text":"{{ $json.resposta }}"}`.
Todos os endpoints estão no `/docs`. Para fluxos com retry, adicione o header
`Idempotency-Key: {{ $json.messageId }}` (ou um uuid do fluxo).

### Agente de IA — servidor MCP

`POST {BASE}/mcp` fala **Model Context Protocol** (JSON-RPC 2.0, transporte
Streamable HTTP), autenticado por `X-Api-Key` ou `Authorization: Bearer <key>`.
Métodos: `initialize`, `tools/list`, `tools/call`. Ferramentas expostas:

`list_sessions` · `get_session` · `send_text` · `send_location` · `send_poll` ·
`send_reaction` · `check_number` · `get_contact_info` · `get_profile_picture` ·
`list_groups` · `list_chats` · `chat_history`.

```bash
curl -s $BASE/mcp -H "X-Api-Key: $KEY" -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call",
       "params":{"name":"send_text","arguments":{"session":"default",
                 "chatId":"5511999999999@s.whatsapp.net","text":"oi do agente"}}}'
```

## Fluxo mínimo

```bash
KEY=dev-master-key
BASE=http://localhost:3000

# 1. cria e inicia a sessão
curl -s -XPOST $BASE/api/sessions -H "X-Api-Key: $KEY" -H 'content-type: application/json' \
  -d '{"name":"default","start":true,
       "config":{"webhooks":[{"url":"https://webhook.site/xxxx","events":["*"],
                              "hmac":{"secret":"s3cr3t"}}]}}'

# 2. pega o QR (repita até status = WORKING)
curl -s $BASE/api/default/auth/qr -H "X-Api-Key: $KEY"

# 3. envia texto
curl -s -XPOST $BASE/api/sendText -H "X-Api-Key: $KEY" -H 'content-type: application/json' \
  -d '{"session":"default","chatId":"5511999999999@s.whatsapp.net","text":"ola"}'

# 4. stream de eventos em tempo real
websocat "ws://localhost:3000/ws?session=default&events=*&api_key=$KEY"
```

## Console web

Servido em **`/`** pelo próprio binário (assets embarcados via `//go:embed`,
sem build de frontend, sem Node). Abra `http://localhost:3000/`, vá em
**Conexão**, cole a `API_KEY` (a master do `.env` serve) — fica só no
`localStorage` do navegador.

Áreas: **Dashboard** · **Sessões** (criar, start/stop/logout/restart, **QR em
imagem** com polling até conectar, editor de `config`) · **Console** de envio
(todos os tipos, com toggle *enfileirar*) · **Grupos** (listar, criar, entrar,
participantes, nome/tópico, invite link) · **Contatos** · **Fila de saída** ·
**Webhooks entregues** · **Eventos ao vivo** (WebSocket) · **API keys**
(gerar/revogar).

Endpoints de apoio: `GET /api/stats`, `GET /api/deliveries?session=`,
`GET /api/{session}/auth/qr.png`, `GET|POST /api/keys`, `DELETE /api/keys/{id}`.

## Endpoints

Corpo sempre inclui `session`. `chatId`/`jid` no formato `55...@s.whatsapp.net`
ou `...@g.us`. Mídia via `data` (base64 puro ou data URI).

| Método | Rota | Corpo principal |
|---|---|---|
| POST | `/api/sendText` | `chatId, text, quotedId?, quotedParticipant?, mentions[]?, linkPreview?` |
| POST | `/api/sendImage` \| `/api/sendFile` \| `/api/sendVideo` \| `/api/sendAudio` | `chatId, data, mimetype?, caption?, filename?, seconds?, gif?, voice?, quotedId?, mentions[]?` |
| POST | `/api/sendSticker` | `chatId, data` (base64 `.webp`) |
| POST | `/api/sendLocation` | `chatId, latitude, longitude, name?, address?` |
| POST | `/api/sendContact` | `chatId, contacts:[{name, phone?\|vcard?}]` |
| POST | `/api/sendPoll` | `chatId, name, options[] (≥2), selectable?` |
| POST | `/api/reaction` | `chatId, messageId, fromMe?, senderId?, emoji` (`""` remove) |
| POST | `/api/deleteMessage` | `chatId, messageId, fromMe?, senderId?` |
| POST | `/api/editMessage` | `chatId, messageId, text` |
| POST | `/api/sendSeen` | `chatId, messageId, fromMe?, senderId?` |
| POST | `/api/presence` | `chatId, state` (`typing`\|`recording`\|`paused`) |
| POST | `/api/sessions/{session}/auth/pair-code` | `phone` → devolve código de 8 dígitos (parear **sem QR**) |
| GET | `/api/{session}/me` | — (JID, LID, pushName, plataforma, devices) |
| PUT | `/api/{session}/profile/status` | `status` (meu recado) |
| POST | `/api/{session}/presence` | `available` (online/offline global) |
| GET | `/api/{session}/blocklist` · POST `/api/{session}/block` | `{jid, block}` |
| GET | `/api/contacts/check?session=&phone=` | — (aceita `phone` repetido ou lista com vírgula) |
| GET | `/api/contacts/info?session=&jid=` | — |
| GET | `/api/contacts/profile-picture?session=&jid=&preview=` | — |
| GET | `/api/groups?session=` | — |
| POST | `/api/groups` | `name, participants[]` |
| POST | `/api/groups/join` | `code` |
| GET | `/api/groups/{jid}?session=` | — |
| POST | `/api/groups/{jid}/leave` | — |
| POST | `/api/groups/{jid}/participants` | `action, participants[]` |
| PUT | `/api/groups/{jid}/name` \| `/topic` | `name` \| `topic` |
| PUT | `/api/groups/{jid}/photo` | `data` (base64 jpeg) |
| PUT | `/api/groups/{jid}/announce` \| `/locked` | `enabled` (só admins enviam / editam) |
| GET | `/api/groups/{jid}/invite-link?session=&reset=` | — |
| GET | `/api/outbox?session=&limit=` | lista os jobs da fila de saída |
| GET | `/api/media/{id}?redirect=` | stream do binário (ou 302 pra URL assinada) |
| GET | `/api/chats?session=&limit=` | conversas guardadas (histórico) |
| GET | `/api/chats/{chatId}/messages?session=&limit=&before=` | histórico de uma conversa |
| GET | `/api/messages/{id}/download?session=` | baixa a mídia de uma mensagem guardada |
| POST | `/api/forwardMessage` | `{session, toChatId, messageId}` — encaminha (marca como encaminhada) |

## Fila de saída (anti-ban)

Disparar várias mensagens seguidas do mesmo número é o caminho mais rápido pra
um ban. Em vez de BullMQ (que é Node), a fila roda sobre **asynq** (mesma stack
do webhook, Redis por baixo).

Qualquer endpoint de envio aceita `"enqueue": true` (e opcionalmente
`"delay": "30s"`). Em vez de enviar na hora, o gateway **reserva um slot de
tempo** no Redis pra aquela sessão: os jobs saem espaçados por
`OUTBOX_MIN_INTERVAL` + jitter aleatório (`OUTBOX_JITTER`), **preservando a
ordem de chegada**. Resposta: `202 { queued, jobId, runAt }`.

```bash
curl -s -XPOST $BASE/api/sendText -H "X-Api-Key: $KEY" -H 'content-type: application/json' \
  -d '{"session":"default","chatId":"...@s.whatsapp.net","text":"1","enqueue":true}'
# -> {"queued":true,"jobId":"...","runAt":"2026-08-29T18:40:03Z"}
```

Override por sessão em `config.outbox`:

```json
{"outbox": {"minIntervalMs": 8000, "jitterMs": 4000, "dailyLimit": 800}}
```

Batendo `dailyLimit` o enqueue responde `429 daily_limit`. Estado dos jobs em
`GET /api/outbox?session=`.

## Mídia (opcional)

Desligado por padrão. Com `MEDIA_BACKEND=s3` usa um bucket S3-compatível
(AWS S3, **MinIO**, Cloudflare R2). Sobe o MinIO junto com
`docker compose --profile media up -d` e descomente os `S3_*` no compose.

`GET /api/media/{id}` faz stream do binário guardado; `?redirect=true` devolve
302 pra uma URL temporária assinada (ou `S3_PUBLIC_BASE_URL` se setado).

**Ingestão automática:** com o backend ligado, toda mídia **recebida** (imagem,
áudio, vídeo, documento, sticker) é baixada, descriptografada
(`whatsmeow.DownloadAny`) e guardada no bucket. O evento `message` ganha:

```json
"media": { "id": "<msgId>", "url": "/api/media/<msgId>", "mimetype": "image/webp", "size": 248526 }
```

O download roda inline no recebimento (timeout 45s); se falhar,
`"media": { "error": "…" }`.

## Eventos

Nome canônico no padrão `domínio.ação` (estilo WAHA). Os que têm mapeamento
explícito estão em `internal/engine/whatsmeow/events.go`; **qualquer outro
evento do whatsmeow passa como `engine.<tipo>`** — cobertura total.

Assinatura por wildcard: `message.*`, `session.*`, `*`.

### Mensagens — payload normalizado

- **`message`** → só mensagens **recebidas** (não `fromMe`). É o que um bot assina.
- **`message.any`** → todas, inclusive as que você mandou.
- **`message.ack`** → recibos (`delivered` / `read` / `played` / `retry`).

O payload é achatado. Endereços `@lid` são resolvidos para o telefone
(`@s.whatsapp.net`) quando possível. O struct cru do whatsmeow **não vem por
padrão** — ligue `config.rawEvents: true` na sessão para receber `raw`.

```json
{
  "id": "A5010C563B75F82E469274B2B0606F5C",
  "chatId": "5517996778746@s.whatsapp.net",
  "chatLid": "161761386868832@lid",
  "from": "5517996778746@s.whatsapp.net",
  "fromMe": false,
  "isGroup": false,
  "pushName": "PRO TELHADOS RIO PRETO",
  "type": "text",
  "timestamp": 1756499088,
  "body": "Oi",
  "quotedId": "",
  "mentions": []
}
```

Campos extras conforme o caso: `media` (quando há mídia e o backend S3 está
ligado), `reaction`, `chatLid`, `author` (em grupo), `hasMedia` / `mediaType`,
e `raw` (só com `config.rawEvents: true`).

`type`: `text` · `image` · `video` · `audio` · `document` · `sticker` ·
`location` · `contact` · `reaction` · `poll` · `poll_vote` ·
`buttons_response` · `list_response` · `unknown`.

## Layout

```
cmd/wa-gateway        entrypoint + wiring + shutdown
internal/config       env
internal/observability slog + prometheus
internal/events       Event canônico + Bus in-process
internal/store        pgx + migrações goose
internal/cache        redis: locks, cache, pub/sub
internal/auth         API key + Argon2id
internal/engine       interface Engine + registry
internal/engine/whatsmeow  implementação + tradução de eventos
internal/session      SessionManager + config de runtime
internal/webhook      envelope + HMAC + dispatcher asynq
internal/outbox       fila de saída com pacing por sessão (asynq + slot no Redis)
internal/media        armazenamento de mídia: Disabled (padrão) | S3/MinIO
internal/ws           hub WebSocket
internal/httpapi      chi router + handlers
migrations            *.sql (goose, //go:embed)
```

## Notas

- O módulo se chama `wa-gateway` (path local). Para publicar, troque a
  primeira linha do `go.mod` e rode `go mod tidy`.
- `internal/engine/whatsmeow` é o único ponto acoplado à API do whatsmeow;
  se algum símbolo mudar numa atualização, o ajuste fica contido ali.
- As tabelas `whatsmeow_*` são criadas pelo `sqlstore` da própria engine no
  mesmo banco; as migrações daqui só cuidam das tabelas de aplicação.
