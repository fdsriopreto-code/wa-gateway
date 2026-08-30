# wa-gateway — Estado & Roadmap

O que já roda, o que falta, o que **não** vale a pena — com o porquê.
Atualizado em **2026-08-30**.

---

## ✅ Feito

### Núcleo
- Engine `wa-gateway` (whatsmeow) com tradução **total** de eventos
  (mapeados + fallback `engine.*`).
- Multi-sessão real (device certo por número no `sqlstore` compartilhado).
- Auto-reconexão no boot (`RestoreOwned`), inclusive retomando pareamento.
- Recuperação de device órfão quando o pareamento é interrompido por restart.
- PostgreSQL como fonte da verdade; migrations goose embarcadas (0001–0004).
- Redis: locks de posse (base do multi-nó), cache, pub/sub.

### Envio
- Texto, imagem, arquivo, vídeo, áudio/PTT, sticker, localização, contato,
  enquete.
- `MessageOpts`: reply (quoted), menções, link preview (scrape OG), forwarded.
- Reação, editar, apagar, marcar como lida, presença de chat ("digitando…").
- Encaminhar mensagem (texto e mídia — baixa, reenvia).

### Recebimento & entrega
- Payload de mensagem **normalizado** (achatado); `@lid` resolvido pra telefone.
- Ingestão de mídia **assíncrona** (pool de 4 workers) → S3/MinIO opcional.
- Webhooks: fila durável asynq, HMAC-SHA256, retry exponencial, pool de 6,
  cache de config por sessão, dedupe por URL.
- WebSocket ao vivo, com fan-out entre nós via Redis.
- Persistência de mensagens/chats/acks (`MESSAGE_STORE`, on por padrão).
- Histórico: `GET /api/chats`, `/api/chats/{id}/messages`, download de mídia.

### Conta / grupos / contatos
- Pareamento por QR e por código (`pair-code`).
- `me`, status/recado, presença global, bloquear/desbloquear, blocklist.
- Grupos: CRUD, participantes (add/remove/promote/demote), nome/tópico/foto,
  announce/locked, invite link, entrar por link.
- **Agenda:** `GET /api/contacts` (lista do contact store), check número,
  info de perfil, foto.
- `autoRead` / `autoOnline` por sessão.

### Integração
- **OpenAPI 3.0.3** gerado (`/openapi.json`) + Swagger UI (`/docs`).
- **MCP** (`POST /mcp`) — 20 ferramentas para agente de IA. Ver [MCP.md](MCP.md).
- `Idempotency-Key` (cache Redis 10 min).
- **Console web** embarcado (SPA vanilla, sem build): sessões + QR ao vivo,
  chat, playground de endpoints, eventos ao vivo, monitor, API keys, editor
  visual de webhooks estilo Evolution.
- **Node n8n** (`n8n-nodes-wa-gateway`, publicado): 6 nós — ação, Trigger
  roteador (7 saídas por tipo, baixa mídia), Fila (debounce), Pausa do bot,
  Agente de IA (modelo/memória/tools do n8n), **OTP** (enviar/conferir código,
  saídas Válido/Inválido).
- Fila de saída com *pacing* anti-ban (slot por sessão, jitter, teto diário).
- **Rate limit por chave de API** — token bucket no Redis, `429` + `Retry-After`
  + `X-RateLimit-*`; `RATE_LIMIT_RPS` (default 20), chave-mestra isenta.
- **WS multi-nó com heartbeat** — só propaga eventos via Redis pub/sub quando
  há >1 nó vivo (`wa:ws:nodes`); com 1 réplica, tráfego pub/sub = zero.
- **Roteamento de request entre nós** (`NODE_ADVERTISE_URL`) — request que
  chega no nó errado é encaminhado pro dono da sessão (reverse-proxy via
  `wa:lock:<s>` → `wa:node:addr:<id>`). Off por padrão. `GET /api/cluster`
  mostra o estado. Console → Monitoramento tem card de infra/nós.
- **Reenvio manual de webhook** — `POST /api/deliveries/{id}/retry`
  reenfileira usando o payload guardado (`webhook_deliveries.payload`,
  migração 0005). Botão "reenviar" nas entregas com falha no console.
- **Métricas de negócio** — `wa_messages_sent_total{session}` /
  `wa_messages_received_total{session}` no `/metrics` (via `Manager.emit`);
  `/api/stats.messages24h` (sent/received) → KPIs no dashboard.
- **Log durável de eventos (Redis Stream)** — `events.Stream` (`wa:events`,
  `MAXLEN ~100k`). webhook e inbox saíram do barramento in-process e passaram
  a **consumer groups** com `XACK` só no sucesso + `XAUTOCLAIM`. Crash no meio
  de um dispatch → evento reprocessa (mesmo nó ou outro do grupo). Fecha o
  **risco #1** da análise anterior. Testes com `miniredis`.
- **Escopo de API key por sessão** — `scopes` como `session:<nome>` /
  `session:*`; `sessionScopeMW` barra fora do escopo (403 `forbidden_session`),
  `GET /api/sessions` filtra, criar sessão bloqueado, `/ws` exige `?session=`.
  Console: campo "restringir a sessões" na criação de chave. Fecha o **risco #3**.
- **Cifra de secrets em repouso** — `internal/secret` (AES-256-GCM,
  `SECRET_KEY` + `SECRET_KEY_OLD` p/ rotação). `webhooks[].hmac.secret` sai
  cifrado (`enc:v1:…`) no Postgres; leitura decifra. Migração lazy.
  `/api/stats` e `/api/cluster` exigem `canAdmin`. Fecha o **risco #5**.
- **Labels do WhatsApp Business** — `engine.Labels/EditLabel/SetChatLabel/
  SetMessageLabel` (via `client.SendAppState` + `appstate.BuildLabel*`).
  `GET/POST /api/{s}/labels`, `/labels/chat`, `/labels/message`. Cache de
  labels montado dos eventos `LabelEdit` (re-sync no reconnect). MCP:
  `list_labels`, `label_chat`. n8n: recurso "Etiqueta" (node v0.4.0).
- **TTL / limpeza de mídia** — `MEDIA_TTL` global + `config.media.ttl` por
  sessão (`5m`…`720h`, `0`=nunca); `config.media.store:false` descarta sem
  subir pro S3. Coletor `media.Sink.RunGC` apaga vencidas do S3+DB a cada
  `MEDIA_GC_INTERVAL`. Manual: `DELETE /api/media/{id}`,
  `POST /api/{s}/media/purge`. Console: aba **Mídia** no cfgModal. Migração
  0007 (`media.expires_at`).
- **Enriquecimento de mídia** — `MEDIA_ENRICH` + `AI_API_KEY` (API compatível
  com OpenAI: OpenAI, Groq, Together…). Áudio/PTT recebido → `transcript`,
  imagem → `imageCaption` no payload do evento; webhook, n8n e WS ganham o
  texto sem download. Pacote `internal/enrich`; callback `engine.Deps.Enrich`
  injetado nas duas engines, roda no worker pool de mídia. Por sessão:
  `config.media.enrich` (*bool*). Console: seletor na aba **Mídia**.
  Follow-ups: (a) se `AI_API_KEY` setado mas `enrich:false` na sessão, a mídia
  ainda é baixada (descartada depois) — só desperdício de banda; (b) sem teto
  de chamadas de enrich por sessão/dia — enxurrada de áudios = custo.
- **Monitor de saúde de sessão** — `Manager.MonitorHealth` varre as sessões
  vivas a cada 20s: se o status fica fora de `WORKING` por mais que
  `SESSION_UNHEALTHY_AFTER` (2m), emite `session.unhealthy` (`{status, since,
  forSeconds}`); ao voltar a `WORKING`, `session.healthy`. `SCAN_QR_CODE`
  nunca alerta (aguarda ação). `GET /api/sessions[].health` (`healthy` /
  `waiting` / `unhealthy`) e banner vermelho no console. Testes com bus real.
  Follow-up: sessão que "flapa" (WORKING↔FAILED a cada <limiar) zera o
  `badSince` a cada volta e pode nunca alertar — trocar por razão de falha
  numa janela.
- **Campanhas / envio em massa** — `internal/campaign` + migração 0008
  (`campaigns`, `campaign_targets`). `POST /api/{s}/campaign`
  (`{name, kind, text|data, recipients[], minIntervalMs?, jitterMs?}`) grava
  os alvos e o `Runner` enfileira **um job da outbox por alvo** → mesmo pacing
  anti-ban, retry e registro do envio avulso. Progresso = agregado sobre
  `campaign_targets` (o `OutboxRecorder` detecta o id `camp:<id>:<n>`).
  `GET /api/campaigns[/{id}]`, `POST /api/campaigns/{id}/stop` (o gate do
  worker barra o que ainda não saiu). Retomável no boot (`Runner.Resume`),
  lock `wa:campaign:<id>` p/ 1 runner por campanha no cluster. Console:
  aba **Campanhas** (lista + progresso + criar campanha de texto). Mídia de
  campanha limitada a 5 MiB (o blob é replicado por alvo na fila).
  Follow-ups: (a) sem templating por destinatário (`{{nome}}`); (b) falha
  transitória marca o alvo `failed` antes do retry (cosmético até resolver);
  (c) sem MCP tool ainda.
- **Auto-resposta por sessão** (`internal/autoreply`) — `config.autoReply`:
  saudação 1x/contato (cooldown), regras `contains → reply`, `fallback`, e
  `onlyOutsideHours` + `hours` (TZ, janela, dias — janela pode cruzar a
  meia-noite). Consome o Redis Stream (consumer group), então serve os dois
  motores e sobrevive a restart. Anti ping-pong: 1 resposta/contato/30s.
  Envia pela fila de saída (paced). Console: aba **Auto-resposta** no
  cfgModal. Testes de horário/parse. Follow-ups: sem IA (é regra fixa —
  quem quer LLM usa o node n8n "wa-gateway Agente"); sem métrica dedicada.
- **OTP / código de verificação** (`internal/otp`, [OTP.md](OTP.md)) — serviço
  estilo "Verify" pra SaaS: `POST /api/{s}/otp/send` gera código numérico,
  guarda só o `HMAC-SHA256(SECRET_KEY, sessão|número|código)` no Redis com TTL
  + teto de tentativas (5 → `locked`) + cooldown de reenvio (60s) + teto por
  hora (5/número), manda pela sessão. `POST /api/{s}/otp/verify` (`{to|id, code}`)
  → `{valid, reason, attemptsLeft}`; código certo é one-shot. `POST .../otp/cancel`.
  `config.otp` por sessão (template/brand/ttl/limites). Métricas
  `wa_otp_sent_total` / `wa_otp_verify_total{result}`. Sem env nem tabela nova.
  Console: grupo **OTP** no Playground. Testes com miniredis.
  Follow-ups: (a) sessão Cloud API deveria mandar por *template de autenticação*
  aprovado (`config.otp.templateName`), não texto puro — a Meta pode barrar OTP
  em texto em escala; (b) `send`/`verify` precisam usar o mesmo formato de
  número (ou o fluxo por `id`).
- **StatusCallback por mensagem** (`internal/ackcb`) — `callbackUrl` +
  `callbackData` em `otp/send` e nos `POST /api/send*` diretos: o gateway
  guarda `wa:ackcb:<s>:<msgId>` (TTL 15m) e um consumer do stream `message.ack`
  faz **1 POST** na URL no 1º status terminal (`delivered`/`read`/`failed`),
  com `{messageId, session, to, status, timestamp, data}`. 3 tentativas.
  Pra SaaS que gera/guarda/confere o código no próprio banco e só quer o
  gateway como cano + confirmação de entrega. Não vale pra `enqueue:true`.
  Testes com miniredis + httptest.
- CI completo + release automático do node n8n por tag.

---

## 🔜 Próximo (vale a pena, planejado)

| Item | Por quê | Esboço |
|---|---|---|
| **Labels do WhatsApp Business** | eventos `label.*` já chegam, falta expor | `GET /api/{s}/labels`, associar/desassociar em chat/mensagem |
| **Plugins de saída (NATS / AMQP)** | quem não quer webhook HTTP | interface `Sink` no dispatcher, além do HTTP |
| **`docs/` versionado** | este conjunto — manter em dia a cada mudança grande | — |

---

## 🛠️ Endurecimento do motor Cloud API (pós-v1)

Buracos da 1ª versão, todos fechados:

- **Token cifrado** — `session.MapSecrets` (era `MapWebhookSecrets`) agora
  também cobre `cloud.accessToken` / `cloud.appSecret`.
- **Webhook síncrono + durável** — `Ingest` emite os eventos (→ Redis Stream)
  antes do 200; só o download de mídia é async (pool de 6). Fim da janela de
  perda entre 200 e processamento.
- **Idempotente** — event id = `wamid` da Meta → reentrega deduplica.
- **`SetStatusMessage`** implementado (about do perfil de negócio).

Resta (menor): `Ingest` de rajada com muita mídia pode encher o pool e emitir
só `mediaMeta` (baixa depois). Aceitável.

---

## ✅ Botões e listas — **resolvido pela via certa**

Como a pesquisa abaixo previu, a única forma confiável era a **API oficial**.
Feito: motor `cloud` (`internal/engine/cloud`) fala a WhatsApp Cloud API da
Meta. `POST /api/sendInteractive` (button/list/cta_url) e `POST
/api/sendTemplate` funcionam de verdade numa sessão `engine=cloud`; no
whatsmeow devolvem `501 not_supported`. Setup em [CLOUD.md](CLOUD.md).

<details><summary>Pesquisa original (por que não dá pelo whatsmeow)</summary>

### Botões e listas interativas — via whatsmeow

**Conclusão: não implementar rendering nativo.** Pesquisa (ago/2026):

- O `whatsmeow` **tem** os protos (`InteractiveMessage`, `NativeFlowMessage`,
  `ButtonsMessage`, `ListMessage`) e o cliente até *monta* a mensagem, mas o
  WhatsApp **não renderiza de forma confiável** quando enviada por cliente
  não-oficial (Web MD). Comportamento observado pela comunidade: header/body/
  footer aparecem, **botões/seções somem**; funciona num sistema operacional e
  falha em outro (iOS vs Android vs Web), sem padrão estável desde ~2023.
- Mantenedor do whatsmeow: *"Button messages are not supported"* / *"desde as
  últimas mudanças do WhatsApp só dá com a API oficial do WhatsApp Business"*.
- Discussões: whatsmeow #220, #279, #348, #534, #650, #711 — nenhuma solução
  confirmada estável em jan/2026.
- Risco: mensagens interativas "estranhas" de cliente não-oficial são um sinal
  clássico de spam → **aumenta chance de ban** do número.

**Alternativas que a gente já tem / dá pra ter barato:**

1. **Enquete (`sendPoll`)** — nativa, confiável, ótima pra "escolha uma opção".
2. **Menu numérico em texto** — "1️⃣ Falar com vendas / 2️⃣ Suporte…" + parser
   da resposta no n8n. Feio, mas 100% entregável.
3. **CTA por link** — `sendText` com `linkPreview` já monta card clicável.
4. **Futuro: 2º motor Meta Cloud API** — aí botões/listas/flows funcionam
   oficialmente. É a única via robusta. Encaixa na arquitetura sem dor: nova
   implementação de `engine.Engine`, `engine.Register("cloud-api", …)`,
   `sessions.engine = "cloud-api"`. Fica para quando houver demanda real
   (precisa de conta WhatsApp Business API + número aprovado + template).

Um endpoint experimental via `NativeFlowMessage` no whatsmeow foi descartado
em favor do motor `cloud` (acima), que é a via oficial e estável.

</details>

---

## 🔬 Análise crítica (2026-08-30)

Estado honesto depois do batch de escala/robustez.

### Riscos que continuam de pé

| # | Risco | Impacto | Mitigação atual | Fix de verdade |
|---|---|---|---|---|
| 1 | ~~Barramento in-process descartava evento sob pico.~~ **RESOLVIDO**: webhook/inbox consomem de `wa:events` com `XACK`+`XAUTOCLAIM`. `Stream.Append` agora espera **até 200ms** antes de desistir, buffer 16384, e o writer faz `XADD` **pipelined em lotes** (menos round-trips). Perda só num Redis realmente travado por >200ms sustentado. | mínimo | buffer 16384 + espera 200ms + pipeline + contador `dropped` | (aceitável) escrita síncrona acima de X% do buffer |
| 2 | ~~Cross-node buffrava 32MB do corpo.~~ **RESOLVIDO**: `sessionFromRequest` espia só os primeiros **512KB** (com um tokenizer que aguenta JSON truncado) e devolve o corpo intacto via `io.MultiReader`. | mínimo (só multi-nó) | peek 512KB + MultiReader | — |
| 7 | ~~`message.ack` antes do `SaveMessage`.~~ **MITIGADO**: `UpdateAckN` devolve linhas afetadas; se 0, o handler espera 400ms e re-tenta 1x. Se ainda 0, a mensagem não está no store (ex.: anterior ao `MESSAGE_STORE`) e segue. | raríssimo | 1 retry de 400ms | upsert de stub (se virar problema) |
| 8 | **Stream `MAXLEN ~100k`.** Se TODOS os consumidores ficarem fora por mais tempo que 100k eventos, o Redis pode aparar entradas ainda não-`XACK`. | perda só se o serviço inteiro ficar down sob alto volume | consumidores moram no mesmo binário do produtor → "todos down" = serviço down | subir o `MAXLEN`, ou trim só por idade (`MINID`) preservando o PEL |
| 3 | ~~Escopos de API key grosseiros.~~ **RESOLVIDO**: `scopes` como `session:<nome>` (ou `session:*`) + `sessionScopeMW` barram uma chave de tocar sessão fora do escopo; `GET /api/sessions` filtra; criar sessão é bloqueado; `/ws` exige `?session=`. Resta: `listDeliveries`/`retryDelivery` ainda não checam escopo de sessão (a entrega é buscada por `?session=`, então o middleware **já cobre**), mas endpoints puramente admin (`/api/keys`, `/api/stats`, `/api/cluster`) não exigem `canAdmin`. | endpoints admin abertos a qualquer chave `*` | escopo por sessão feito | gate `canAdmin` em `/api/stats`, `/api/cluster`, `/api/deliveries` |
| 4 | ~~Sem dead-letter de webhook.~~ **RESOLVIDO**: esgotadas as tentativas, o dispatcher emite `webhook.exhausted` no stream (`{deliveryId, url, event, attempts, lastError}`) + métrica `wa_webhook_deliveries_total{result="exhausted"}`. O próprio webhook pode assinar o evento pra ser avisado. Guarda anti-loop (não re-emite pra falha de um `webhook.exhausted`). | — | — |
| 5 | ~~Segredos em texto puro.~~ **RESOLVIDO** (`internal/secret`, `SECRET_KEY` = AES-256-GCM): `Manager.Upsert` sela `webhooks[].hmac.secret`, leitura decifra. Migração lazy. **Rotação sem downtime** via `SECRET_KEY_OLD` (CSV, só decifram). Sem `SECRET_KEY` = texto puro + warning no boot. | — | — |
| 6 | **`migrations` sobem em todo boot sem lock explícito.** 2 nós subindo juntos podem correr. | risco baixo (goose usa `schema_migrations`) | goose serializa por versão | advisory lock no Postgres antes do `Migrate` |

### O que ficou bom

- **Isolamento do motor** — a interface `Engine` segurou 6 features novas sem
  vazar whatsmeow pra fora. Meta Cloud API entra sem dor quando for a hora.
- **Degradação** — S3 fora, Redis fora (fail-open no rate limit), nó dono fora
  (502 claro) — nada derruba o processo.
- **Custo zero quando desligado** — rate limit (`RPS=0`), cross-node
  (`ADVERTISE_URL=""`), WS pub/sub (1 nó): todos viram um `if` e somem.
- **`spec.go` + `/openapi.json` + testes** mantêm a superfície REST honesta.
- **Observabilidade** cobre o que importa: HTTP, eventos, drops, entregas,
  rate limit, msgs enviadas/recebidas, sessões ativas, clientes WS.

### Ordem sugerida daqui

1. ~~Redis Stream no barramento~~ ✅
2. ~~Escopos de API key por sessão~~ ✅
3. ~~Cifrar segredos + `canAdmin` + rotação `SECRET_KEY`~~ ✅
4. ~~Riscos residuais #1/#2/#7~~ ✅
5. ~~Labels do WhatsApp Business~~ ✅
6. ~~Dead-letter de webhook~~ ✅
7. ~~Motor Meta Cloud API (botões/listas/templates)~~ ✅
8. Plugins NATS/AMQP · rate-limit adaptativo · painel de templates.

---

## 🧭 Princípios de crescimento

1. **A interface `Engine` é sagrada.** Nada fora de `internal/engine/whatsmeow`
   importa o whatsmeow. Motor novo = implementação nova, zero mudança no resto.
2. **`spec.go` é a fonte de verdade da API.** Endpoint sem linha lá não existe
   pro OpenAPI/docs — e o teste pega.
3. **Dois caminhos de evento.** `events.Bus` in-process = best-effort (WS).
   `events.Stream` (Redis Stream) = durável, at-least-once (webhook, inbox).
   Consumidor novo que precise de garantia → `stream.Consume` (consumer
   group), não `bus.Subscribe`.
4. **Config por sessão, não por deploy.** Qualquer comportamento novo de sessão
   entra em `session.Config` (JSONB), com default seguro.
5. **Degradar, não crashar.** Dependência opcional (S3) fora do ar = log + segue.
6. **Multi-nó desde já.** Todo estado compartilhado vai pro Postgres/Redis,
   nunca pra memória do processo.

---

## Fontes (pesquisa de botões/listas)

- [whatsmeow · Discussion #711 — Interactive message](https://github.com/tulir/whatsmeow/discussions/711)
- [whatsmeow · Discussion #650 — Buttons iOS/Web/Desktop, not Android](https://github.com/tulir/whatsmeow/discussions/650)
- [whatsmeow · Discussion #348 — How to send Interactive Message](https://github.com/tulir/whatsmeow/discussions/348)
- [Meta for Developers — Interactive reply buttons](https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/interactive-reply-buttons-messages)
- [pkg.go.dev — go.mau.fi/whatsmeow](https://pkg.go.dev/go.mau.fi/whatsmeow)
