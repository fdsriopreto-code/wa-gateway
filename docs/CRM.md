# Leads / CRM

O gateway mantém, por `(sessão, contato)`, um **lead** com as métricas de
conversa que um CRM de leads precisa — sem você processar a torrente de
eventos. Alimentado por um consumidor do log de eventos (Redis Stream), então
vale pros dois motores (whatsmeow e Cloud API) e sobrevive a restart.

Ligado por padrão (`LEADS=on`). Desligar por sessão: `config.leads.enabled = false`.

## O que é medido (por lead)

| campo | significado |
|---|---|
| `status` | `new` · `waiting_us` (cliente falou, você não respondeu) · `waiting_them` (você respondeu, aguardando) · `closed` |
| `waitingSince` / `waitingSeconds` | desde quando está sem resposta sua (só em `waiting_us`) |
| `firstContactAt` | 1º contato |
| `lastInboundAt` / `lastOutboundAt` | última recebida / última enviada |
| `lastReadByThemAt` | última vez que **eles** leram uma mensagem sua (recibo `read`) |
| `inboundCount` / `outboundCount` | nº de mensagens de cada lado |
| `responseCount` / `avgResponseSeconds` | quantas vezes você respondeu e o tempo médio pra responder |
| `secondsSinceLastInbound` | há quanto tempo o cliente não fala |
| `lastMessage` / `lastMessageFromMe` | preview da última mensagem e de quem foi |
| `stage` `owner` `tags` `notes` | campos livres que **você** gerencia (pipeline do CRM) |
| `source` | **origem do lead** (ver abaixo) |

## Origem / atribuição (`source`)

Capturada **no 1º contato** e travada. Estrutura:

```jsonc
{
  "adReferral": {            // veio de um anúncio Click-to-WhatsApp (Meta)
    "sourceId": "120210…",   // ID do anúncio
    "sourceType": "ad",
    "sourceUrl": "https://fb.me/…",
    "ctwaClid": "AR-…",      // click id do Click-to-WhatsApp
    "headline": "…", "body": "…"
  },
  "utm": { "utm_source": "instagram", "utm_campaign": "julho-2026", "utm_medium": "cpc" },
  "clickIds": { "fbclid": "…", "gclid": "…" },
  "params": { "ref": "landing-home" },
  "firstMessage": "Olá, vim do anúncio"
}
```

- **`adReferral`** — as duas engines extraem sozinhas: whatsmeow lê
  `contextInfo.externalAdReply` da 1ª mensagem; a Cloud API lê o objeto
  `referral` do webhook.
- **`utm` / `clickIds` / `params`** — parseados do **texto** da 1ª mensagem.
  Funciona com link do tipo `wa.me/55…?text=vim%20de%20https://site/?utm_source=ig&utm_campaign=x`
  (o `?text=` chega como corpo da mensagem), com um link colado, ou com
  tokens soltos (`ref=landing utm_medium=cpc`).

## Endpoints

Todos sob `/api/{session}` (respeitam escopo de API key).

### `GET /api/{session}/leads`

Query: `status`, `stage`, `tag`, `q` (busca nome/número/última msg),
`source` (`ad` | `utm` | `any`), `sort` (`updated` padrão · `waiting`
longest-first · `recent`), `limit` (≤500), `offset`.

```bash
# quem está esperando resposta há mais tempo
curl "$BASE/api/vendas/leads?status=waiting_us&sort=waiting" -H "X-Api-Key: $KEY"
# leads que vieram de anúncio
curl "$BASE/api/vendas/leads?source=ad" -H "X-Api-Key: $KEY"
```

### `GET /api/{session}/leads/{chatId}`

`{chatId}` aceita o JID (`55…@s.whatsapp.net`) **ou só o número**. Devolve o
lead completo com os derivados (`avgResponseSeconds`, `waitingSeconds`, …).

### `PATCH /api/{session}/leads/{chatId}`  (ou `POST`)

```jsonc
{ "stage": "proposta", "owner": "ana", "tags": ["quente","promo"], "notes": "pediu orçamento", "status": "closed" }
```

`status` aceito no patch: `closed` · `waiting_us` · `waiting_them` · `new`.
Um `closed` que recebe nova mensagem **reabre** como `waiting_us`.

### `GET /api/{session}/leads/stats`

```json
{ "total": 342, "new": 12, "waitingUs": 8, "waitingThem": 40, "closed": 282,
  "stale": 3, "fromAds": 51, "fromUtm": 88, "avgResponseSeconds": 640,
  "byStage": { "qualificado": 30, "proposta": 12 } }
```

## Eventos (webhook / WS)

| evento | quando | payload |
|---|---|---|
| `lead.new` | 1ª mensagem de um contato novo | `{chatId, phone, pushName, source, firstMessage}` |
| `lead.stale` | lead em `waiting_us` há mais que `LEAD_STALE_AFTER` (default `2h`) — dispara **uma vez** | `{chatId, phone, waitingSince, waitingSeconds}` |

Assine `lead.*` num webhook pra empurrar leads/alertas pro seu CRM em tempo real.

## MCP

`list_leads`, `get_lead`, `update_lead` — um agente de IA qualifica e move
leads no funil sozinho.

## Config

| | onde |
|---|---|
| `LEADS` (`on`) | liga/desliga global |
| `LEAD_STALE_AFTER` (`2h`) | limiar do `lead.stale` e do contador `stale` no `/stats` |
| `config.leads.enabled` (por sessão) | `false` = não mantém lead dessa sessão |

Grupos, broadcast, status e newsletter são ignorados — só conversa 1:1.
