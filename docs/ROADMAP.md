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
- **Node n8n** (`n8n-nodes-wa-gateway`, publicado): 4 nós —
  ação, Trigger roteador (7 saídas por tipo, baixa mídia), Fila (debounce),
  Pausa do bot.
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
- CI completo + release automático do node n8n por tag.

---

## 🔜 Próximo (vale a pena, planejado)

| Item | Por quê | Esboço |
|---|---|---|
| **Labels do WhatsApp Business** | eventos `label.*` já chegam, falta expor | `GET /api/{s}/labels`, associar/desassociar em chat/mensagem |
| **Plugins de saída (NATS / AMQP)** | quem não quer webhook HTTP | interface `Sink` no dispatcher, além do HTTP |
| **`docs/` versionado** | este conjunto — manter em dia a cada mudança grande | — |

---

## 🧪 Investigado — **não** vale a pena agora

### Botões e listas interativas

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

Se ainda assim quiser um endpoint **experimental** `POST /api/sendButtons`
(monta `NativeFlowMessage`, sem garantia de render) — é ~40 linhas em
`extras.go` + `send.go`. Marcado como *experimental* no OpenAPI. É só pedir.

---

## 🔬 Análise crítica (2026-08-30)

Estado honesto depois do batch de escala/robustez.

### Riscos que continuam de pé

| # | Risco | Impacto | Mitigação atual | Fix de verdade |
|---|---|---|---|---|
| 1 | **Barramento é in-process.** `events.Bus` entrega por canal Go. `SubscribeReliable` espera 250ms, mas sob pico sustentado ainda descarta → webhook não enfileirado (sem retry) / mensagem não persistida. | perda silenciosa de evento em pico | `maxWait` 250ms + contador `wa_bus_dropped_total` + log a cada 100 | trocar o backbone por **Redis Stream** (`XADD`/`XREADGROUP`) — consumidores com ack, replay, sem drop. É o próximo grande item. |
| 2 | **Cross-node buffra o corpo.** Com `NODE_ADVERTISE_URL` on, todo POST-JSON é lido inteiro (até 32MB) pra achar `session`. | latência/RAM em envio de mídia grande no modo multi-nó | só afeta multi-nó; single-node não paga nada | tokenizer streaming de JSON + `io.MultiReader` pra restaurar sem bufferizar tudo |
| 3 | **Escopos de API key são grosseiros.** Quase tudo exige `*`. `listDeliveries`/`retryDelivery` não checam `canAdmin` e uma chave escopada consegue ver entrega de outra sessão. | vazamento entre tenants num cenário multi-cliente | modelo hoje é "1 chave = tudo" | escopos por sessão (`session:vendas:*`) + `Principal.Can` com match de prefixo |
| 4 | **Sem dead-letter de webhook.** Depois de N tentativas o asynq desiste; a linha fica `failed` sem alerta. | entregas silenciosamente perdidas | botão "reenviar" manual no console | evento `webhook.exhausted` no próprio barramento + métrica |
| 5 | **Segredos em texto puro.** HMAC secret do webhook mora em `sessions.config` JSONB sem cifra. | quem lê o Postgres lê os segredos | acesso ao banco já é privilegiado | cifrar campos sensíveis com uma key de env (AES-GCM) |
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

1. **Redis Stream no barramento** (risco #1) — maior ganho de robustez.
2. **Escopos de API key por sessão** (risco #3) — destrava multi-tenant real.
3. **Cifrar segredos** (risco #5) — barato, fecha uma auditoria.
4. Labels Business · dead-letter de webhook · streaming do body no cross-node.

---

## 🧭 Princípios de crescimento

1. **A interface `Engine` é sagrada.** Nada fora de `internal/engine/whatsmeow`
   importa o whatsmeow. Motor novo = implementação nova, zero mudança no resto.
2. **`spec.go` é a fonte de verdade da API.** Endpoint sem linha lá não existe
   pro OpenAPI/docs — e o teste pega.
3. **Barramento não garante entrega.** Durabilidade é das filas asynq. Assinante
   novo que precise de garantia → fila própria, não só `bus.Match`.
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
