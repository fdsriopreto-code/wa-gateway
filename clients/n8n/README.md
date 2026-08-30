# n8n-nodes-wa-gateway

Node comunitário do n8n para o [wa-gateway](../../README.md).

Quatro nodes:

| Node | Pra quê |
|---|---|
| **wa-gateway** | Enviar texto/imagem/documento/vídeo/áudio/localização/enquete, reagir, encaminhar, criar/gerenciar sessões, grupos, contatos (incl. **listar agenda**), ler histórico. Aceita **binário do nó anterior** (converte pra base64 sozinho). |
| **wa-gateway Trigger** | Recebe eventos de uma sessão e **roteia por tipo**: saídas separadas para **Texto · Imagem · Áudio · Vídeo · Documento · Outros · Eventos** (sem precisar de Switch). Opcionalmente **baixa a mídia e anexa como binário** (`data`) pronta pro próximo node. Ao ativar o workflow, **registra a URL do webhook na sessão automaticamente**. Valida HMAC-SHA256 se você definir um segredo. |
| **wa-gateway Fila (debounce)** | Agrupa mensagens picadas por contato (Redis). `Enfileirar` → `Wait` → `Coletar`: se chegou mensagem nova na janela, sai por **Superado** (essa execução desiste); senão sai por **Continuar** com a fila unificada em `text` / `messages`. `Cancelar` descarta lotes pendentes depois que o bot respondeu. Precisa da credencial `Redis` do n8n. |
| **wa-gateway Pausa do bot** | Handoff humano por contato (Redis + TTL). `Pausar` quando você responde manualmente (`fromMe`), `Verificar` antes de deixar a IA responder (saídas **Ativo** / **Pausado**), `Retomar` pra devolver pro bot. Precisa da credencial `Redis` do n8n. |

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

**Atendente de IA (mensagem picada + handoff):**
```
wa-gateway Trigger ──▶ fromMe? ──true──▶ wa-gateway Pausa (Pausar)  ▶ (fim)
                            └──false──▶ wa-gateway Pausa (Verificar)
                                          ├─ Pausado ─▶ (fim)
                                          └─ Ativo ──▶ wa-gateway Fila (Enfileirar)
                                                        └▶ Wait 15s
                                                           └▶ wa-gateway Fila (Coletar)
                                                               ├─ Superado ─▶ NoOp
                                                               └─ Continuar ─▶ AI Agent
                                                                                └▶ wa-gateway (Enviar texto)
                                                                                   └▶ wa-gateway Fila (Cancelar)
```
`Coletar` entrega `{{ $json.text }}` (mensagens da janela unidas) pro agente. O `Cancelar` no fim aborta lotes que ainda estejam na janela de 15s.

**Anti-ban:** marque *Enfileirar* nos envios pra passar pela fila com pacing por número.
