# Motor Cloud API (oficial da Meta)

Uma sessão pode falar a **WhatsApp Cloud API** oficial em vez do protocolo
Web (whatsmeow). Vantagens: **botões, listas, CTA e templates funcionam de
verdade**, sem risco de ban. Custo: precisa de uma conta WhatsApp Business
API aprovada e um número registrado na Meta; grupos, presença, enquetes e
edição/exclusão de mensagem **não existem** nessa API.

Os dois motores expõem a **mesma REST** e emitem os **mesmos eventos**
(`message`, `message.any`, `message.ack`) — o resto do gateway (webhooks, WS,
histórico, n8n) não muda.

---

## 1. Pré-requisitos (no lado da Meta)

1. App no [Meta for Developers](https://developers.facebook.com/) com o
   produto **WhatsApp** adicionado.
2. Um **número** registrado → você recebe o **Phone Number ID** e o
   **WhatsApp Business Account ID (WABA ID)**.
3. Um **token de acesso** (permanente, de System User — não o temporário de
   24h).
4. O **App Secret** (Configurações → Básico) — opcional, valida a assinatura
   do webhook.

## 2. Criar a sessão no wa-gateway

`POST /api/sessions` com `config.cloud`:

```jsonc
{
  "name": "oficial",
  "start": true,
  "config": {
    "cloud": {
      "phoneNumberId": "123456789012345",
      "accessToken":   "EAAG...",
      "wabaId":        "987654321098765",
      "verifyToken":   "um-segredo-que-voce-inventa",
      "appSecret":     "abc123...",
      "graphVersion":  "v21.0"
    },
    "webhooks": [
      { "url": "https://meu-n8n/webhook/xyz", "events": ["message","message.ack"] }
    ]
  }
}
```

Detectando `config.cloud` com `phoneNumberId` + `accessToken`, o gateway
marca a sessão como `engine: "cloud"`. `start` valida o token consultando o
número (status vira `WORKING`; **não há QR**).

## 3. Registrar o webhook na Meta

No painel do App → WhatsApp → Configuration → **Webhook**:

- **Callback URL:** `https://SEU-GATEWAY/api/oficial/cloud/webhook`
- **Verify token:** o mesmo `verifyToken` que você pôs na config
- Assine os campos **`messages`**

A Meta faz um `GET` de verificação (o gateway responde o `hub.challenge`).
Depois manda os eventos por `POST` — o gateway valida a assinatura
`X-Hub-Signature-256` (se `appSecret` estiver setado) e injeta no fluxo
normal de eventos.

> Esse endpoint é **público** (a Meta não manda `X-Api-Key`).

## 4. Enviar

Tudo pela REST normal, com `session` = o nome da sessão cloud:

| O quê | Endpoint |
|---|---|
| Texto, imagem, doc, vídeo, áudio, localização, contato, reação, marcar lida | os mesmos de sempre (`/api/sendText`, `/api/sendImage`, …) |
| **Botões / lista / CTA** | `POST /api/sendInteractive` |
| **Template aprovado** | `POST /api/sendTemplate` |

### Botões
```json
{
  "session": "oficial",
  "chatId": "5517999999999",
  "type": "button",
  "body": "Confirma o pedido?",
  "footer": "Loja X",
  "buttons": [
    { "id": "sim", "title": "Sim, confirmar" },
    { "id": "nao", "title": "Cancelar" }
  ]
}
```

### Lista
```json
{
  "session": "oficial", "chatId": "5517999999999",
  "type": "list", "body": "Escolha um horário", "buttonText": "Ver horários",
  "sections": [
    { "title": "Manhã", "rows": [
      { "id": "9h",  "title": "09:00" },
      { "id": "10h", "title": "10:00", "description": "última vaga" }
    ]}
  ]
}
```

### CTA de URL
```json
{ "session": "oficial", "chatId": "55…", "type": "cta_url",
  "body": "Veja o catálogo", "displayUrl": "Abrir catálogo",
  "url": "https://loja.exemplo/catalogo" }
```

### Template
```json
{
  "session": "oficial", "chatId": "5517999999999",
  "name": "confirmacao_pedido", "language": "pt_BR",
  "components": [
    { "type": "body", "parameters": [ { "type": "text", "text": "#4821" } ] }
  ]
}
```
`components` é o array **cru** da Graph API — passe exatamente o que a Meta
documenta pro seu template.

## 5. Receber a resposta de um botão/lista

Chega como um evento `message` normal, com:
```jsonc
{
  "type": "button",
  "body": "Sim, confirmar",        // o título escolhido
  "reply": { "id": "sim", "title": "Sim, confirmar" }
}
```
No n8n: rotear por `payload.reply.id`.

## 6. O que NÃO funciona (devolve `501 not_supported`)

Grupos (criar/listar/participantes), presença "digitando…", enquetes,
`forward`, `editMessage`, `deleteMessage`, `check number`, bloquear/
desbloquear, foto/status de perfil, labels. São limitações da Cloud API, não
do gateway — para essas, use uma sessão `whatsmeow`.

## 7. Detalhes de implementação

- **Token cifrado:** `cloud.accessToken` e `cloud.appSecret` são cifrados em
  repouso no Postgres se `SECRET_KEY` estiver setado (igual aos secrets de
  webhook). `GET /api/sessions/{s}` devolve decifrado.
- **Webhook síncrono + durável:** o `Ingest` parseia e emite os eventos
  (que entram no Redis Stream, `XACK`) **antes** de devolver 200. Só o
  download de mídia é assíncrono (pool interno de 6). Um crash entre o 200 e
  o processamento de texto/botão/recibo não perde nada.
- **Idempotente:** o `id` do evento emitido é o `wamid` da Meta — se a Meta
  reentregar (200 demorou), o dispatcher deduplica pelo mesmo TaskID.
- **`SetStatusMessage`** grava o `about` do perfil de negócio.
- `Me()` traz `verified_name`, número e `quality_rating`.

## 8. Limites conhecidos

- **Janela de 24h:** fora dela só dá pra iniciar conversa com **template**
  aprovado (regra da Meta).
- **Mídia por link externo não é aceita** aqui — o gateway sobe o binário
  pra Meta (`/media`) e envia por id. Mande o `data` (base64) como sempre.
- O status `sent` da Meta vira `message.ack type=sent` (o whatsmeow não tem
  esse estágio).
- Se o `Ingest` demorar (rajada com muita mídia), a mídia é baixada num pool
  de 6; se o pool encher, o evento sai só com `mediaMeta` (baixe depois via
  `POST /api/{s}/media/download`).
