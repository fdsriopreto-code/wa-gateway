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
- CI completo + release automático do node n8n por tag.

---

## 🔜 Próximo (vale a pena, planejado)

| Item | Por quê | Esboço |
|---|---|---|
| **Roteamento de request entre nós** | hoje `POST` numa sessão de outro nó → 409 | o nó que recebe consulta o lock (`wa:lock:<s>` guarda o `nodeID`) e faz *reverse-proxy* pro dono; ou expõe o mapa sessão→nó pro LB |
| **Reenvio manual de webhook** | `POST /api/deliveries/{id}/retry` | precisa guardar o `deliverPayload` (body+secret) numa coluna `jsonb` — migração |
| **Métricas de negócio** | msgs enviadas/recebidas por sessão, lag da fila | contadores Prometheus com label `session` |
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
