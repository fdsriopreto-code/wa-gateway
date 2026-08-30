# n8n-nodes-wa-gateway

Node comunitário do n8n para o [wa-gateway](../../README.md).

Dois nodes:

| Node | Pra quê |
|---|---|
| **wa-gateway** | Enviar texto/imagem/documento/vídeo/áudio/localização/enquete, reagir, encaminhar, criar/gerenciar sessões, grupos, contatos, ler histórico. Aceita **binário do nó anterior** (converte pra base64 sozinho). |
| **wa-gateway Trigger** | Recebe eventos de uma sessão. Ao ativar o workflow, **registra a URL do webhook na config da sessão automaticamente** (e remove ao desativar). Valida HMAC-SHA256 se você definir um segredo. |

## Instalar

**Pelo painel do n8n** (Settings → Community Nodes → Install): `n8n-nodes-wa-gateway`

**Manual / dev:**
```bash
cd clients/n8n
npm install
npm run build
# copie a pasta dist para ~/.n8n/custom/  (ou use N8N_CUSTOM_EXTENSIONS)
```

## Credencial

`wa-gateway API`:
- **Base URL** — `https://seu-gateway.exemplo.com`
- **API Key** — a `API_KEY` do `.env` ou uma chave da tabela `api_keys`

## Exemplos

**Responder mensagem com IA:**
`wa-gateway Trigger` (events: `message`) → `AI Agent` → `wa-gateway` (Mensagem · Enviar texto, chatId = `{{$json.payload.chatId}}`, text = `{{$json.output}}`).

**Enviar uma imagem que veio de um HTTP Request / Google Drive:**
node anterior produz binário na propriedade `data` → `wa-gateway` (Mensagem · Enviar imagem, Fonte da mídia = *Binário do nó anterior*).

**Anti-ban:** marque *Enfileirar* nos envios pra passar pela fila com pacing por número.
