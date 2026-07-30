# Catálogo de Leituras Transacionais (Read-Routing)

Catálogo normativo "uma verdade por leitura" para o roteamento de leituras
no caminho de comando do ledger (create / commit / cancel / revert) e nas leituras de
validação pura que participam desse caminho.

O objetivo é registrar, para cada leitura, se ela **semeia estado** que será escrito
logo em seguida (e portanto **precisa ler do primário** para não observar réplica
defasada antes do commit) ou se é apenas **validação / cache-fronted / fora do caminho
de comando** (e pode continuar servida pela réplica ou pelo cache).

Cada linha aponta o `file.go:line` real do call site. Os âmbitos amplos
(`organization_id`, `ledger_id`) não são repetidos aqui: as leituras vivem sob o mesmo
contexto de tenant/ledger do comando que as originou.

## Classificação (rótulos)

- **`read-before-write-que-semeia-estado`** → leitura cujo resultado alimenta o cálculo
  de balances/operações que serão persistidos no mesmo comando. Classificação: **primário**.
- **`validation-only`** → leitura que apenas valida regra/rota/estado e **não** semeia o
  balance a ser escrito. Pode ficar na **réplica**.
- **`cache-fronted`** → leitura servida por Redis (cache-aside) com fallback ao banco; o
  fallback herda a classificação do caminho. Prioridade baixa para roteamento explícito.
- **`not-on-command-path`** → leitura que não pertence ao caminho de mutação de balance
  (ex.: claim de idempotência em Redis). Sem roteamento de réplica/primário aplicável.

## Tabela do Catálogo

| Leitura (método + call site) | Fluxo | Origem atual | Classificação | Rótulo | Justificativa |
|---|---|---|---|---|---|
| `Query.GetBalances` → `BalanceRepo.ListByAliasesWithKeys` — `transaction_create.go:1291` (impl em `get_balances.go:36`) | create | Redis-cache + réplica (fallback) | **primário** | `read-before-write-que-semeia-estado` | Semeia os balances usados para montar/validar operações antes do commit. Roteada ao primário via `readrouting.WithPrimaryRead(ctx)` em `transaction_create.go:1289`. |
| `Query.GetBalances` (enriquecimento de overdraft) — `transaction_create.go:1333-1334` | create | Redis-cache + réplica (fallback) | **primário** | `read-before-write-que-semeia-estado` | Lê o balance do companion `#overdraft` para calcular o déficit que vira operação persistida. Herda o `ctx` já marcado em `:1289` (não requer marcação própria). |
| `Query.GetBalances` — `transaction_state_handlers.go:435` | commit / cancel | Redis-cache + réplica (fallback) | **primário** | `read-before-write-que-semeia-estado` | Semeia os balances da transição de estado antes do write. Roteada ao primário via `readCtx := readrouting.WithPrimaryRead(ctx)` marcado em `commitOrCancelTransaction` (`transaction_state_handlers.go:433`), imediatamente antes de `GetBalances` (`:435`), que recebe o `readCtx` dedicado. |
| `Query.GetBalances` (enriquecimento de overdraft no cancel) — `transaction_state_handlers.go:450-451` | cancel | Redis-cache + réplica (fallback) | **primário** | `read-before-write-que-semeia-estado` | Recalcula o leg de overdraft no cancelamento; semeia operação persistida. Roteada ao primário: recebe o mesmo `readCtx` dedicado marcado em `:433` (não requer marcação própria). |
| `TransactionRouteRepo.FindByID` (fallback da route-cache) — `get_or_create_transaction_route_cache.go:70`, via `ValidateAccountingRules` (`transaction_create.go` no create; `transaction_state_handlers.go:462` no commit/cancel) | create / commit / cancel | Redis puro + banco (fallback, negative caching) | réplica | `validation-only` `cache-fronted` | Valida a existência/config da rota; não semeia balance. Fallback ao banco só em cache miss. Permanece na réplica — no-op deliberado: leitura de validação pura. No commit/cancel roda sob o `ctx` não marcado (nunca sob o `readCtx` dedicado), ou seja, deliberadamente não roteada ao primário. |
| `Query.GetParsedLedgerSettings` → `GetLedgerSettings` — `get_ledger_settings_parsed.go:22` (cache-aside em `get_ledger_settings.go:32`, TTL 5min) | create / commit / cancel | Redis-cache + banco (fallback) | réplica | `validation-only` `cache-fronted` | Lê flags de validação (`validateAccountType`, `validateRoutes`); não semeia balance. Cache-aside best-effort. |
| `Command.CreateOrCheckTransactionIdempotency` — `create_transaction_idempotency.go:45` (`TransactionRedisRepo.SetNX`/`Get`) | create | Redis puro | — | `not-on-command-path` | Claim de idempotência em Redis (não Postgres); é um lock, não uma leitura que semeia balance. Fora do eixo réplica/primário. |
| `Query.GetWriteBehindTransaction` — `transaction_state_handlers.go:75` | commit / cancel | réplica | réplica | `validation-only` | Recupera a transação write-behind para validar o estado atual; não lê nem semeia balance. |
| `Query.GetParentByTransactionID` — `transaction_state_handlers.go:159` | revert | réplica | réplica | `validation-only` | Verifica existência do pai antes do revert; não semeia balance. |
| `Query.GetTransactionWithOperationsByID` — `transaction_state_handlers.go:174` | revert | réplica | réplica | `validation-only` | Lê a transação-alvo + operações para montar o reverso; a semente de balance do reverso vem do `GetBalances` do novo create, não daqui. |
| `Query.GetOperationRouteByID` — `transaction_state_handlers.go:222` | revert | réplica | réplica | `validation-only` | Resolve a operation route por id para validar o reverso; não semeia balance. |
| `Query.GetTransactionByID` — `get_id_transaction.go:23` | consulta pura | réplica | réplica | `validation-only` | Consulta de leitura por id fora da mutação de balance (também reutilizada por validações); não semeia estado a ser escrito. |

## Contagem / tally

- Total de linhas: **12**.
- **primário / semeia estado** (`read-before-write-que-semeia-estado`): **4** — todas roteadas ao primário (2 no create, 2 no commit/cancel); nenhuma pendente.
- **validation-only** (inclui as duas também rotuladas `cache-fronted`): **7**.
- **not-on-command-path**: **1** (idempotência Redis).

## Exceção de camada (marcação no handler)

O roteamento de leitura é intent-based: o *handler* marca a intenção no `ctx` e o
use case a herda transparentemente. A intenção é marcada **no handler**, não no use
case:

- create → `executeCreateTransaction` marca `ctx = readrouting.WithPrimaryRead(ctx)`
  em `transaction_create.go:1289`, imediatamente antes de `GetBalances` (`:1291`).
- commit / cancel → `commitOrCancelTransaction` (`transaction_state_handlers.go:313`)
  marca `readCtx := readrouting.WithPrimaryRead(ctx)` em `:433`, imediatamente antes de
  `GetBalances` (`:435`). Diferente do create, aqui a marcação usa um `ctx` dedicado
  (`readCtx`), escopado apenas às leituras de balance anteriores ao write; o `ctx` não
  marcado segue para as demais operações (validação, seed no Redis, escrita).

Isso é uma exceção deliberada à regra "sem lógica de infraestrutura no handler",
sustentada por dois mitigadores:

1. **Ponto único de convergência** — todas as leituras transacionais do fluxo passam
   por `GetBalances`, então a marcação vive em um único lugar por fluxo (create; commit/
   cancel), não espalhada pelos use cases.
2. **Wrap de uma linha, sem lógica de negócio** — a marcação é apenas
   `readrouting.WithPrimaryRead(ctx)` (ver `pkg/readrouting/intent.go`), transporte
   puro de intenção via `context.Context`; nenhuma decisão de negócio vaza para o handler.

## Consistência com o offload analítico

A regra **"uma verdade por leitura"** vale em conjunto com o offload analítico:

- Leituras **transacionais** que semeiam estado no caminho de comando → **primário**
  (nunca réplica, nunca offload analítico). São a fonte de verdade para o write.
- Leituras **analíticas / de consulta pura** (busca, listagem, relatórios) → réplica ou
  o offload de busca (Elasticsearch), jamais o primário.

Nenhuma leitura deve ter classificação ambígua: cada linha desta tabela tem exatamente
um rótulo primário de origem. Se uma consulta passar a semear estado, ela migra para
**primário** e entra nesta tabela; se deixar de semear, sai do caminho de comando.

## Locking reads

O único `SELECT ... FOR UPDATE` do componente é:

- `LedgerPostgreSQLRepository.UpdateSettingsAtomic` — `ledger.postgresql.go:955`
  (`SELECT settings FROM ledger ... FOR UPDATE`), dentro da própria transação de
  escrita (read-modify-write atômico das settings do ledger).

Esse lock **não** está no caminho transacional de balances (create/commit/cancel/
revert) — pertence à mutação atômica de settings do ledger e roda em sua própria tx.
Portanto **não requer roteamento**: já lê e escreve consistentemente na mesma conexão
de escrita. Registrado aqui por completude.
