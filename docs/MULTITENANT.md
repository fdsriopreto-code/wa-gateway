# wa-gateway em SaaS multi-tenant

Checklist para rodar o gateway com muitos clientes, cada um com o próprio
WhatsApp. Referência das variáveis: [ARCHITECTURE.md §7](ARCHITECTURE.md#7-referência-de-configuração).

## 1. Isolamento entre clientes

- **Uma chave por cliente**, com escopo só da sessão dele:
  `POST /api/keys {"label":"cliente-42","scopes":["session:cliente-42"]}`.
  A chave-mestra (`API_KEY`) e chaves `*` ficam só no seu backend.
- Chave escopada **não** cria sessão, não lista sessões alheias, não faz
  campanha (salvo `ALLOW_SCOPED_CAMPAIGNS=true`) e tem anti-SSRF estrito:
  webhook, `callbackUrl` e mídia por URL dela não alcançam a rede interna.
- Ligue `SECRET_KEY` (cifra HMAC, tokens da Cloud API e URL do proxy em repouso).
- Ligue `METRICS_TOKEN`: os labels do `/metrics` listam o nome de todas as sessões.
- Se todo webhook é cadastrado pelo seu backend com chave `*`, considere
  `SSRF_BLOCK_PRIVATE=true` + `SSRF_ALLOW_HOSTS` com o que for interno de verdade.

## 2. Dimensionamento

Números de partida — **meça com teste de carga antes de vender capacidade**:

| Item | Ponto de partida |
|---|---|
| Sessões por nó | 200–300 (`MAX_SESSIONS_PER_NODE=250`) |
| RAM por sessão | ~10–30 MB (cresce com grupos, app state e mídia) |
| Conexões Postgres por nó | `DATABASE_MAX_CONNS` + `WA_STORE_MAX_CONNS` (padrão 30) |
| Postgres | `max_connections` ≥ nós × (as duas acima) + folga; PgBouncer em modo *session* se passar de ~300 |
| Redis | dedicado; `EVENTS_STREAM_MAXLEN` ≥ 10× o pico de eventos por minuto |

Para 1000 números: 4–5 nós com `MAX_SESSIONS_PER_NODE=250`,
`NODE_ADVERTISE_URL` em cada um (roteamento entre nós) e o mesmo Postgres/Redis.

Deploy: nó novo sobe, o antigo solta os locks ao desligar e o `ReconcileLoop`
redistribui (até 25 sessões por nó a cada 30 s). Faça rolling de **um nó por vez**:
reconectar centenas de números ao mesmo tempo, do mesmo IP, chama atenção.

## 3. Risco de bloqueio (API não oficial)

Responder só quem chamou é o perfil de menor risco, mas não é zero. O que ajuda:

- **`config.typing`** — "digitando…" proporcional ao tamanho da resposta antes de
  cada texto (`{"enabled":true}` usa 40 ms/caractere, teto 6 s).
- **`config.proxy` / `WA_PROXY`** — espalhe os números em IPs diferentes
  (proxy residencial/móvel por sessão é o ideal; 1000 números num IP de
  datacenter é sinal ruim).
- **Campanhas desligadas** pra chave de tenant (padrão). Um cliente fazendo
  disparo frio queima o número dele e suja o IP dos outros.
- **`config.outbox`** com `dailyLimit` por sessão se o cliente puder iniciar conversa.
- Número com histórico (não chip recém-ativado), foto e nome de perfil preenchidos.
- Trate queda como rotina: assine `session.status` / `session.unhealthy`,
  avise o cliente e facilite o QR de novo.
- Para quem não pode perder o número: motor **`cloud`** (Cloud API oficial da
  Meta), já suportado — ver [CLOUD.md](CLOUD.md).
