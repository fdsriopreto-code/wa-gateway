# OTP — código de verificação por WhatsApp

Serviço embutido, estilo "Verify" (Twilio/Vonage), para SaaS que precisam
confirmar o número de WhatsApp de um usuário. O gateway gera o código, guarda
**só o HMAC** no Redis (TTL + teto de tentativas + cooldown), manda pela
sessão e responde `valid: true/false` na conferência.

Não precisa de tabela nova nem de env var extra. `SECRET_KEY` (se setada)
vira a chave do HMAC — recomendado em produção.

## Endpoints

Todos exigem `X-Api-Key` (a chave pode ter escopo `session:<nome>`).

### `POST /api/{session}/otp/send`

```jsonc
{
  "to": "5517999999999",     // obrigatório — número (dígitos, E.164) ou JID
  "brand": "ACME",           // opcional — aparece na mensagem
  "template": "...",         // opcional — sobrescreve o texto (ver placeholders)
  "codeLength": 6,           // opcional — 4..10, default 6
  "ttlSeconds": 300          // opcional — 30..1800, default 300
}
```

Resposta `200`:

```json
{
  "id": "otp_9f3a…",
  "to": "5517999999999",
  "expiresAt": "2026-08-30T18:05:00Z",
  "resendAfterSeconds": 60
}
```

Erros: `409 not_active` (sessão parada), `429 rate_limited` + `Retry-After`
(cooldown de reenvio ou teto por hora), `502 send_failed` (a mensagem não
saiu — o código é descartado, pode tentar de novo).

### `POST /api/{session}/otp/verify`

```jsonc
{ "to": "5517999999999", "code": "123456" }   // ou { "id": "otp_…", "code": "123456" }
```

Resposta `200` (código certo) ou `422` (errado/expirado):

```json
{ "valid": false, "reason": "mismatch", "attemptsLeft": 3 }
```

`reason`: `mismatch` · `expired` · `locked` (estourou as tentativas) ·
`not_found` (nunca enviado, ou já consumido). Um código certo é **consumido**
na hora (one-shot) e libera o cooldown.

### `POST /api/{session}/otp/cancel`

```json
{ "to": "5517999999999" }
```

Invalida o código ativo e o cooldown (ex.: o usuário trocou de número).

## Regras (defaults, sobrescrevíveis em `config.otp`)

| | default | onde muda |
|---|---|---|
| tamanho do código | 6 dígitos | `codeLength` na request / `config.otp.codeLength` |
| validade | 5 min | `ttlSeconds` / `config.otp.ttlSeconds` |
| tentativas de conferência | 5 (depois `locked`) | `config.otp.maxAttempts` |
| cooldown de reenvio | 60 s | `config.otp.resendAfterSeconds` |
| teto de envios | 5 / hora / número | `config.otp.hourlyCap` (0 = ilimitado) |
| texto | ver abaixo | `template` / `config.otp.template` / `config.otp.brand` |

`config.otp` (em `sessions.config`, via `PUT /api/sessions/{s}`):

```jsonc
{
  "otp": {
    "brand": "ACME",
    "template": "{{code}} é o seu código{{brand}}. Vale por {{minutes}} min.",
    "ttlSeconds": 600,
    "maxAttempts": 4,
    "hourlyCap": 8
  }
}
```

**Placeholders do template:** `{{code}}`, `{{brand}}` (com espaço à esquerda
quando setado), `{{minutes}}`, `{{ttl}}`.

## Segurança

- Só o `HMAC-SHA256(SECRET_KEY, session|numero|codigo)` vai pro Redis — um dump
  do Redis não revela códigos.
- Um código ativo por `(sessão, número)`; reenviar sobrescreve.
- Cooldown + teto por hora por número; rate-limit global por API key também vale.
- Código de uso único, apagado na 1ª conferência certa.
- Defina `SECRET_KEY` (64 hex) — sem ela, o HMAC usa um *pepper* fixo do código.

## Exemplo (curl)

```bash
# enviar
curl -sX POST "$BASE/api/minha-sessao/otp/send" -H "X-Api-Key: $KEY" \
  -H 'content-type: application/json' \
  -d '{"to":"5517999999999","brand":"ACME"}'

# conferir
curl -sX POST "$BASE/api/minha-sessao/otp/verify" -H "X-Api-Key: $KEY" \
  -H 'content-type: application/json' \
  -d '{"to":"5517999999999","code":"123456"}'
```
