# n8n-nodes-wa-gateway

Node comunitário do n8n para o [wa-gateway](../../README.md).

Seis nodes:

| Node | Pra quê |
|---|---|
| **wa-gateway** | Enviar texto/imagem/documento/vídeo/áudio/localização/**enquete**, **resultado da enquete** (placar), **botões e templates (Cloud API)**, reagir, encaminhar, criar/gerenciar sessões, grupos, contatos (incl. **listar agenda**), **etiquetas do Business**, ler histórico. Aceita **binário do nó anterior** (converte pra base64 sozinho). |
| **wa-gateway Trigger** | Recebe eventos de uma sessão e **roteia por tipo**: saídas separadas para **Texto · Imagem · Áudio · Vídeo · Documento · Outros · Eventos · Enquete** (sem precisar de Switch). A saída **Enquete** entrega o **voto já decifrado e achatado** (`pollId`, `voter`, `voterName`, `selectedOptions`, `removed`) quando você assina `message.poll_vote`. Opcionalmente **baixa a mídia e anexa como binário** (`data`). Ao ativar o workflow, **registra a URL do webhook na sessão automaticamente**. Valida HMAC-SHA256 se você definir um segredo. |
| **wa-gateway Fila (debounce)** | Agrupa mensagens picadas por contato (Redis). `Enfileirar` → `Wait` → `Coletar`: se chegou mensagem nova na janela, sai por **Superado** (essa execução desiste); senão sai por **Continuar** com a fila unificada em `text` / `messages`. `Cancelar` descarta lotes pendentes depois que o bot respondeu. Precisa da credencial `Redis` do n8n. |
| **wa-gateway Pausa do bot** | Handoff humano por contato (Redis + TTL). `Pausar` quando você responde manualmente (`fromMe`), `Verificar` antes de deixar a IA responder (saídas **Ativo** / **Pausado**), `Retomar` pra devolver pro bot. Precisa da credencial `Redis` do n8n. |
| **wa-gateway Agente** | Agente de IA **nativo do WhatsApp**. Conecte o **Chat Model** (OpenAI/Anthropic/Gemini/Ollama…) e, opcionalmente, um nó de **Memória** (chave = chatId) e nós de **Ferramenta** — reaproveita as credenciais que você já tem no n8n. Traz **ferramentas de WhatsApp embutidas** (`enviar_imagem`/`_audio`/`_documento`/`_localizacao`/`_botoes`, `checar_numero`, `listar_grupos`, `buscar_historico`, `escalar_para_humano`), **entende imagem/áudio recebidos** (baixa e manda pro modelo — visão/áudio) e **responde sozinho** pelo wa-gateway (com "digitando…"). Saídas: **Resposta · Handoff · Erro**. |
| **wa-gateway OTP** | Código de verificação de número por WhatsApp. `Enviar código` (o gateway gera, guarda o hash no Redis, manda — com `brand`, `template`, validade e **callback de entrega** opcionais), `Conferir código` (saídas **Válido** / **Inválido**, com `reason` e `attemptsLeft`) e `Cancelar código`. Rate limit / código errado / envio falho saem por **Falha / Inválido** sem quebrar o fluxo. Se o *seu* app já gera e guarda o código, não precisa deste node — use **wa-gateway** (Enviar texto) com `callbackUrl`. |

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

**Atendente de IA num nó só (o jeito rápido):**
```
wa-gateway Trigger ──▶ wa-gateway Agente
                         ▲ Modelo (OpenAI/Anthropic/…)   ▲ Memória (Postgres/Redis, key = {{$json.payload.chatId}})   ▲ Ferramentas (opcional)
                       ├─ Resposta ─▶ (já enviou pro WhatsApp)
                       ├─ Handoff  ─▶ notifica um humano
                       └─ Erro     ─▶ log
```

**Atendente de IA (controle fino: mensagem picada + handoff):**
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

**OTP num fluxo só:**
```
Webhook do seu app ──▶ wa-gateway OTP (Enviar código, to = {{$json.phone}}, brand = "MeuApp")
                         └─ Sucesso ─▶ responde { id } pro app

Webhook "conferir" ──▶ wa-gateway OTP (Conferir código, to = {{$json.phone}}, code = {{$json.code}})
                         ├─ Válido   ─▶ marca telefone verificado
                         └─ Inválido ─▶ devolve reason / attemptsLeft
```

