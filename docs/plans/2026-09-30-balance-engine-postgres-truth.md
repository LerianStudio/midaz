# Balance Engine on Postgres Truth Implementation Plan

> **For implementers:** Use ring-default:dispatching-workflows (rolling-phase: elaborate
> the current phase against the real code, run it as a reviewed multi-agent
> workflow, then elaborate the next phase — repeat), or ring-dev-team:running-dev-cycle
> for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** o Postgres do tenant vira a única verdade do dinheiro: a decisão de saldo e a gravação da trilha acontecem no mesmo commit síncrono replicado, e o cliente só recebe 201 depois desse commit.

**Architecture:** um autorizador por tenant (dono com fence, ou lotes concorrentes com row lock, o que a Fase 1 medir melhor) recebe um lote de intenções, decide em sequência sobre estado carregado do Postgres e grava intenções, transações, operações, saldos e controles financeiros em uma transação Postgres por lote. O autorizador não sabe de onde o lote veio: o transporte (HTTP direto ao pod dono, ou Redpanda) é um adaptador à parte, e a Fase 1 mede os dois. O motor roda só com Postgres; o Redpanda é opcional e, quando existe, é dedicado ao ledger. Todo pedido carrega um deadline, cortado na entrada e de novo na decisão. O alias é resolvido uma vez na borda da API; o motor trabalha por id. O Valkey, o Lua, o write-behind por RabbitMQ, o recovery runner e o modo async saem do caminho do dinheiro. O adaptador novo implementa o porto `command.Engine` que já existe; a troca acontece por tenant, atrás de uma barreira de cutover, sem major, e a Fase 5 apaga o motor antigo.

**Tech Stack:** Go 1.27, PostgreSQL 17 no bench (o major do compose do repo; a produção roda RDS 16.14 e o caminho do commit é o mesmo), `pgx/v5` e `squirrel`, `golang-migrate`, docker compose (primário, standby síncrono e toxiproxy 2.12.0 do bench; Redpanda `v24.2.7`, a imagem que o repo já usa, só na célula de transporte), `franz-go` v1.21.6 (já em `go.mod`).

**Execução:** worktree única `/srv/worktrees/balance-engine`; branch única `feature/balance-engine` (o servidor do repo só aceita os prefixos `feature/`, `fix/`, `hotfix/`, `docs/`, `refactor/`, `build/`, `test/`, `chore/` e `ci/`); commits assinados com o trailer `X-Lerian-Ref: 0x1`; push ao fim de cada fase; nenhum PR até o Fred pedir; nenhuma outra branch. Executor: `ring-default:dispatching-workflows` nesta sessão, um workflow por fase, revisor diferente do implementador.

**Fontes:** alfarrábio `https://alfarrabio.lerian.net/reports/fred/midaz-motor-de-saldo` (hoje versus destino); `docs/architecture/engine.md` (motor atual); reunião "Direcionamento Midaz" de 2026-10-14, com os pontos que o Fred aprovou para este plano: intenção, deadline, motor por lote sem transporte, sem infra nova obrigatória, Redpanda dedicado, Tracer como módulo, alias por id, réplica de leitura e particionamento, telemetria enxuta.

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | Harness reprodutível mede os dois escritores sobre o esquema real, com commit síncrono e latência injetada; o mesmo autorizador medido por dois transportes (direto e Redpanda); deadline respeitado sob backlog; cenários de falha provam zero perda e zero duplicação; relatório go/no-go escrito | 1.1, 1.2, 1.3, 1.4, 1.5 | Detailed |
| 2 | Protocolo e contrato do autorizador especificados (intenção, deadline, transporte, telemetria, particionamento); migrações do escritor aplicadas; adaptador Postgres do `command.Engine` atende o create singular v2 em ledgers marcados `settings.engine=postgres` | 2.1, 2.2, 2.3, 2.4 | Epic-level |
| 3 | Pending, revert, grupos, controles, fee debt, batch v2 e leitura de saldo funcionam no adaptador Postgres; alias resolvido na borda e motor por id; suítes de integração e chaos verdes nos dois motores | 3.1, 3.2, 3.3, 3.4 | Epic-level |
| 4 | Um tenant migra do Valkey para o Postgres atrás de uma barreira, com ensaio em homologação e runbook | 4.1, 4.2, 4.3 | Epic-level |
| 5 | Lua, Valkey no caminho do dinheiro, write-behind, recovery runner, backup queue, balance sync e modo async apagados; docs e métricas refletem o motor único | 5.1, 5.2, 5.3 | Epic-level |

---

## Hoje e destino

### Hoje

Produção (Lerian Cloud): Midaz 4.2.0. `develop`: 4.3.0-beta.

O Lua dentro do Valkey decide o saldo em um EVAL e o cliente recebe 201 antes do Postgres saber. O Postgres recebe uma projeção depois, por RabbitMQ (write-behind) ou por completion síncrona. Se a projeção falha, o serviço registra o aviso "Applied transaction projection deferred to recovery" e ainda responde 201 (`components/ledger/internal/services/command/create_transaction_engine.go`, função `finalizeCreateEngineResult`). Um Valkey restaurado de snapshot com chave NÃO vazia vence o Postgres: `loadBalancePool` em `components/ledger/internal/adapters/redis/engine/scripts/engine/execution.lua` trata o valor do Redis como autoridade, e quando a chave falta `query.GetBalances` semeia da linha de saldo do Postgres, que atrasa pelo write-behind: sob eviction, dois pedidos concorrentes partem da mesma versão e o filtro de versão da projeção descarta um em silêncio. O saldo bifurca; não só volta no tempo. As listas de fee debt vivem só no Valkey, sem reseed exato (`docs/architecture/engine.md`, seção "Compatibility changes and rollout"). O Valkey de produção é compartilhado, `appendonly=no`, snapshot diário, réplica assíncrona. O cache guarda saldos em formatos de alias de releases anteriores (`<index>#<alias>#<key>` antes da 4.1; hotfix #2587): toda cópia do cache lê todos os formatos. Em 07/10 o próprio ledger encheu o Valkey compartilhado (artefatos de recovery sem TTL; a limpeza tratava 100 execuções por tenant a cada 30 min contra 40 mil transações por dia); a eviction despejou saldos quentes, chaves de idempotência e locks; em um cliente: 9 forks em 5 saldos, 15 pernas aprovadas fora do saldo, e duplicatas por reenvio depois do timeout de 29 s do gateway com a chave já despejada. O hotfix 4.2.1 pagina a limpeza.

Depois do commit, três obrigações correm fora dele: a metadata no MongoDB, os eventos de streaming e o acerto da reserva no tracer. Uma reserva que o tracer não respondeu é acertada por transação a partir de uma fila em memória (`components/ledger/internal/services/command/transaction_reservation_unanswered_settle.go`: 64 acertos em voo, fila cheia descarta, restart do pod perde a fila).

### Onde o ledger roda em produção

| Item | Lerian Cloud (`lerian-infrastructure`, `environments/production/rds/ledger-shared-prd-postgres`) |
|---|---|
| Motor | RDS for PostgreSQL 16.14 |
| Instância | `db.t4g.small` (2 vCPU burstable, 2 GiB), gp3 de 100 a 500 GB |
| Replicação | Multi-AZ de instância |
| Compartilhamento | uma instância para os bancos `onboarding` e `transaction`; o host de réplica é o mesmo endpoint do primário |

O Multi-AZ de instância replica o storage para a outra AZ de forma síncrona (a AWS declara 2 a 5 ms a mais por commit). O Postgres não enxerga esse standby: `pg_stat_replication` fica vazio e `synchronous_standby_names` não é nosso. Quando o standby some, o primário segue SEM replicação até o standby voltar; o failover troca o DNS em 60 a 120 s. Muitos clientes BYOC também rodam em RDS. (AWS, "Amazon RDS Under the Hood: Multi-AZ" e a página de failover Multi-AZ do guia do RDS.)

### Um tenant tem dois bancos Postgres

`onboarding` guarda organizations, ledgers, assets, portfolios, segments, accounts e os bloqueios de conta. `transaction` guarda transactions, operations, balances, routes e groups (`docs/infra/migrations.md`; `components/ledger/internal/bootstrap/config.postgres.onboarding.go:41` e `components/ledger/internal/bootstrap/config.postgres.transaction.go:45`). Uma fence no banco `transaction` não cobre atomicamente um controle gravado no banco `onboarding`: daí a definição 1.

### Destino: um commit, uma verdade

- O banco `transaction` do tenant é a única verdade do dinheiro.
- Um escritor por tenant decide em sequência. Um lote em voo; o lote seguinte cresce enquanto o commit anterior não volta. Teto de throughput = tamanho do lote dividido pelo ciclo de commit (lote 40 com ciclo de 8 ms = 5k/s, não 8k/s).
- Uma transação Postgres por lote, `synchronous_commit=on`. O commit carrega transações, operações, saldos, controles financeiros (block, limite, exceção de uso único, fee debt, pending, grupo), a identidade e o resultado da requisição, e a checagem da fence `(tenant_id, owner_id, epoch, lease_until)` com `rows affected = 1`, senão abort.
- A durabilidade do commit entre AZs é contrato de hospedagem: RDS Multi-AZ, ou standby síncrono nomeado em Postgres próprio. O ledger checa `SHOW synchronous_commit = on` no boot e recusa subir sem isso; não chama API da AWS; trata failover como conexão perdida (reconecta pelo DNS e repete o lote `unknown` com a mesma identidade). O modo cluster do RDS e o Postgres próprio ficam documentados como upgrade para quem não aceitar a janela sem réplica do Multi-AZ de instância. Região é DR assíncrono com RPO declarado. (Definição 2, decidida; veto do Fred aberto.)
- 201 só depois do commit confirmado. Ack de COMMIT perdido vira "unknown, retry com a mesma identidade", nunca falha e nunca nova execução.
- Failover do escritor: novo epoch; o novo dono recarrega saldos E controles antes do primeiro lote.
- As obrigações pós-commit (metadata no MongoDB, eventos de streaming, acerto da reserva no tracer) saem por um outbox gravado no mesmo commit e um relay que o drena.
- Cutover por tenant atrás de uma barreira, antes de apagar o Lua e o write-behind.
- Guard de revert: unique index em `transaction.parent_transaction_id`.
- Fork impossível por construção: o `UPDATE balance ... WHERE version = esperada` do lote aborta em divergência, nunca descarta em silêncio. Um unique index extra sobre (saldo, versão) só entra se a Fase 1 mostrar custo aceitável.
- Intenção de transação: todo pedido vira uma linha de intenção, com id gerado na aplicação, gravada no mesmo commit da decisão com o resultado (aprovada, ou recusada com o motivo, inclusive recusa do tracer). A intenção é a identidade do pedido para o replay interno (fila, lote `unknown`); uma intenção recusada não tem lançamento. O retry do cliente sem chave continua na definição 4.
- Deadline em todo pedido: a API corta o pedido vencido na entrada, antes do autorizador, e o autorizador recusa commitar um pedido vencido no momento da decisão. Pedido vencido recebe erro explícito, nunca 201 tardio. Nenhum gateway é pressuposto: o default vem de `settings` do ledger (30 s na ausência de configuração) e o cliente pode encurtar por pedido com um header. Clientes BYOC que chamam o ledger sem gateway ficam cobertos pelo default.
- Autorizador por lote, sem transporte: recebe um lote de intenções e o estado carregado, decide, devolve as linhas a commitar. O transporte é adaptador: direto (a API encaminha ao pod dono por gRPC) é o desenho base; consumidor de Redpanda é a alternativa que a Fase 1 mede com os mesmos lotes.
- Sem infra nova obrigatória, sem major: o motor roda só com Postgres. O Redpanda é opcional; quando existe, é dedicado ao ledger (nunca compartilhado com o Streaming Hub) e substitui o RabbitMQ como destino do relay do outbox. Sem Redpanda, o relay publica no RabbitMQ. O aceite assíncrono (202 e consulta) não é padrão; entra depois como opção, fora desta branch.
- Alias resolvido uma vez na borda da API; o motor identifica o saldo por id. Um alias reusado depois de delete nunca herda estornos ou pendências do dono anterior (definição do Fred de 2026-10-01).
- Tracer é módulo do Midaz (fechado em 14/10): a entrada é sempre o payload do Midaz e os endpoints avulsos de validação morrem (lane do time). Para o motor, abre a definição 10: a reserva roda dentro do autorizador ou segue por gRPC, e qual limite vai para o commit.
- O autorizador nasce no padrão novo de telemetria (`docs/standards/telemetry.md`; o time corta cerca de 70% da instrumentação): spans de pedido, lote e commit no caminho feliz; detalhe só no erro.

### Topologias: onde o Redpanda entra

Em todas, a verdade do saldo é o Postgres e o lote vira um commit. O Fred julga pelas transições entre componentes, onde os incidentes se concentram.

| Topologia | Caminho do 201 | Compra | Custa | Veredito |
|---|---|---|---|---|
| A. Autorizador direto | API valida e encaminha ao escritor do tenant; o escritor decide e commita o lote; 201 depois do commit. Dois componentes: o escritor e o Postgres | Menos transições; idempotência, fence e controles no mesmo commit | Roteamento ao pod dono e takeover são nossos (lease, epoch); a Fase 1 ainda não mediu o salto da API ao pod dono | Desenho do plano |
| B. Validador, Redpanda, autorizador | API valida sem saldo e publica síncrono num tópico por tenant; um worker por tenant consome, decide o lote e commita transação, operação, saldo e o offset consumido; a resposta volta por um canal. Quatro componentes: Redpanda, worker, Postgres e o canal de resposta | Roteamento e dono pela partição; backlog durável se o worker cair; retomada pelo offset no commit | Duas transições novas; canal de resposta não existe hoje; fila atrasada vira timeout sem backpressure ao cliente; reenvio do cliente é mensagem nova, então a chave de idempotência precisa estar no commit mesmo assim; worker revogado no rebalance aplica o lote duas vezes se o update do offset não for condicional | Célula da bancada (Epic 1.4), não baseline |
| C. Como B, aceite assíncrono | Igual a B, mas a API responde 202 ao publicar e o cliente consulta o resultado | Remove o canal de resposta; backlog deixa de virar timeout | Muda o contrato público (hoje o POST devolve 201 com o resultado); um limite negado só aparece depois | Entra depois como opção, nunca como padrão (aprovado em 14/10); fora desta branch |
| D. Autorizador direto, Redpanda em paralelo | Igual a A no 201; o autorizador também publica a decisão no Redpanda fora do commit; um worker reconcilia contra o Postgres | Nada a mais no caminho do 201 | Dual write: commit sem publish, ou publish sem commit, divergem; o worker compara o Postgres com uma cópia que pode estar errada, e o Postgres já é a verdade | Vira E quando o publish passa para trás do commit |
| E. Autorizador direto, Redpanda atrás do commit | Igual a A no 201; o commit grava o lote e a linha de outbox; um relay lê o outbox e publica no Redpanda (ou no RabbitMQ, onde não há Redpanda) | Cada lote commitado sai em ordem, sem dual write; alimenta o Streaming Hub, a metadata no Mongo, o acerto do tracer e a réplica de leitura | Um relay a operar; o lag do relay atrasa eventos, nunca o 201 | O outbox do plano; recomendado com A |

Recomendação: A com E. B entra na Fase 1 como eixo de transporte, com os mesmos lotes, a mesma replicação e as mesmas quedas; só vence se entregar TPS igual ou maior, p99 menor e zero perda na queda do worker (definição 8).

### O que sai do produto

Lua e os scripts em `components/ledger/internal/adapters/redis/engine/scripts/`; o Valkey no caminho do dinheiro; o write-behind por RabbitMQ; o recovery runner e a fila `engine:{transactions}:recover`; a `backup_queue:{transactions}`; o balance sync worker; o modo async (`RABBITMQ_TRANSACTION_ASYNC`); a fila em memória de acerto de reservas do tracer, absorvida pelo outbox. Deleção é entrega: a Fase 5 reporta o delta de linhas.

### Custo por transação no esquema atual

Uma transação de duas pernas grava 1 linha em `transaction`, 2 em `operation` e atualiza 2 em `balance`: entre 18 e 23 entradas de índice por transação (estimativa pelas migrações em `components/ledger/migrations/transaction/`; a Fase 1 mede). 8k TPS significam cerca de 24k linhas por segundo em transaction e operation, mais os updates das contrapartes. A conta quente (a conta única e limitada que recebe toda a carga) é a linha mais atualizada do banco: `fillfactor` baixo em `balance` é candidato desde a Fase 1.

### Alvos de aceitação da Fase 1

| Medida | Alvo |
|---|---|
| TPS sustentado em UMA conta limitada | 8.000 |
| p99 do 201, RTT 1 ms entre primário e standby | ≤ 15 ms (hipótese) |
| Perda ou duplicação após queda do primário no meio de um lote, 20 rodadas | 0 |
| Takeover com 1M saldos e 1M controles | ≤ orçamento da definição 3 (default 5 s) |
| Corrida de 30 min: p99 do último minuto sobre o p99 do quinto minuto | ≤ 1,5× |
| Sobrecarga de commit em RDS Multi-AZ de instância (só com a instância temporária) | medir; a AWS declara 2 a 5 ms |
| Pedido commitado depois do próprio deadline, sob backlog | 0 |
| Transporte Redpanda contra direto: TPS máximo, p99 a 8.000/s até a persistência e até a resposta, com e sem carga alheia no broker | medir |
| Perda ou dupla aplicação após queda do worker do Redpanda entre o commit e a resposta, e após rebalance com worker estale, 20 rodadas | 0 |

Uma célula não medida fica escrita como "não medido".

## Definições pendentes do Fred

Uma pergunta por turno, na ordem da tabela, no template frio (sistema, hoje, problema, opções com custo). A Fase 1 não depende de nenhuma delas. A definição 2 (quais falhas o zero perda sobrevive) está fechada no destino acima; a numeração fica para não quebrar as referências.

| # | Decisão | Recomendação (default se ele não gastar atenção) | Bloqueia |
|---|---|---|---|
| 1 | Onde vivem os controles financeiros autoritativos (block, limite, exceção): mover para o banco `transaction` como tabela do escritor, ou manter em `onboarding` e ordenar a leitura pelo escritor | Mover: tabela `account_control` no banco `transaction`; block e unblock viram comandos que passam pelo escritor. Custo: um comando novo entre onboarding e transaction; ganho: um só commit cobre tudo | Epic 2.1, 2.2 |
| 3 | Orçamento de indisponibilidade por tenant no takeover do escritor; e se o 504 no minuto do failover do RDS (60 a 120 s de DNS, acima do timeout de 29 s do gateway) é aceitável ou o gateway segura mais | ≤ 5 s no p99 (lease de 3 s mais reload). Custo: o reload completo precisa caber em memória; senão carga lazy. O failover do RDS é da AWS e se soma a esse orçamento; aceitar o 504 nesse minuto, porque a identidade no commit torna o reenvio seguro | Epic 2.1 (lease, reload) |
| 4 | Garantia de retry para requisição sem chave do cliente, em criar, confirmar, cancelar e estornar. Hoje confirmar e cancelar um pending em `/v2` não aceitam chave de idempotência (`StateTransactionRequestV2` só carrega os ids do caminho): um commit com resposta perdida não tem replay e o retry recebe conflito de estado | A intenção (id gerado na aplicação) cobre o replay interno; para o retry do cliente sem chave, identidade derivada do corpo (hash do intent) com a janela de retenção atual; corpo diferente é transação nova; confirmar e cancelar derivam a identidade do id da transação mais a ação. Custo: dois corpos iguais em janela curta colidem por desenho | Epic 2.1 (identidade), Epic 3.1 (pending) |
| 5 | Interrupção permitida no cutover | Pausa curta por tenant em janela agendada: 503 com Retry-After por alguns segundos. Custo: janela operacional; ganho: barreira simples e verificável | Epic 4.1 |
| 6 | Ponte da linha 4.x: vira lane agora, em paralelo a esta branch, com que prioridade; e a idempotência dela precisa sobreviver só à eviction ou também à perda do nó | Lane agora, antes da Fase 1: Valkey dedicado com `noeviction` e alarme de memória, teto de retenção nos artefatos de recovery, detector de fork como alerta; AOF e `WAITAOF` depois. Idempotência: `noeviction` cobre a eviction a custo zero no caminho quente; perda do nó só com Postgres ou AOF, e Postgres no caminho quente é o que o motor novo entrega. Custo: uma lane de infra e produto fora desta branch | nada nesta branch; exposição da produção |
| 7 | Contrato de retry publicado para o cliente: mesmo corpo e mesma chave devolvem o mesmo resultado, inclusive com o original ainda em voo | Publicar como contrato da API: a identidade no commit o torna verdadeiro por construção no motor novo; na 4.x vale só enquanto a chave existir. Custo: doc e um teste de contrato | Epic 2.1 (identidade) |
| 8 | Onde o Redpanda entra: só atrás do commit, como destino do relay do outbox (topologias A com E), ou no caminho síncrono antes da decisão (B) | A com E. B só se a célula de transporte da Fase 1 (Epic 1.4) entregar TPS igual ou maior, p99 menor a 8.000/s e zero perda na queda do worker. C (aceite assíncrono) entra depois como opção, nunca como padrão; D (dual write) descartada | Epic 2.1 (transporte); nada na Fase 1 |
| 9 | Linguagem do autorizador: Go ou Rust | Go. O ciclo de commit, não a CPU, é o teto; as libs da Lerian são obrigatórias no Go; Rust devolve uma fronteira de runtime que o Lua acabou de sair. Revisão só se a Fase 1 medir tempo em processo acima de 20% do ciclo de commit | Epic 2.3 |
| 10 | Tracer como módulo do Midaz: a reserva roda dentro do autorizador (uma transição a menos, limite da conta no mesmo commit) ou continua por gRPC com acerto pelo outbox; e qual limite vai para o commit (bloqueio e limite de conta) e qual fica em regra do Tracer (cotas de tarifa ficam no fees) | Fases 2 e 3 mantêm a reserva por gRPC com acerto pelo outbox, que é o que o plano já desenha; mover a regra para dentro do autorizador é fase própria depois do cutover. Limite de conta no commit; regras CEL no Tracer | Epic 3.2 |

## Fora do escopo desta branch

- Ponte da linha 4.x em produção (lane própria, não desta branch; prioridade é a definição 6). Ordem: Valkey dedicado ao ledger com `noeviction` e alarme de memória; teto de retenção em todo artefato de recovery (o 4.2.1 só pagina a limpeza); detector de fork como alerta a cada 5 min sobre `balance_version_before` repetida; depois `appendfsync always`, `WAITAOF` na MESMA conexão do EVAL (o adaptador usa `client.Process` em `components/ledger/internal/adapters/redis/engine/adapter.go`) e failover que nunca promove réplica atrasada. Em 07/10 nada disso estava aplicado; com `noeviction` e o vazamento, o ledger teria parado em vez de bifurcar. Reduz a janela; não fecha a bifurcação por snapshot nem a perda de fee debt.
- Mudar a Lerian Cloud para RDS Multi-AZ cluster ou para Postgres próprio no EKS: decisão de custo do Fred. O plano documenta as duas como upgrade do contrato de hospedagem.
- Backward compatibility com o motor antigo depois da Fase 5: não existe. O cutover é a única ponte.
- Réplica de leitura para consulta (Mongo primeira opção; ClickHouse depende da licença): estudo próprio, alimentado pelo outbox deste motor. Hoje o SaaS roda sem réplica por custo. O particionamento de `transaction` e `operation` por tempo entra neste plano (Epic 2.1 decide, 2.2 aplica), porque a consulta é o maior problema de hoje para o cliente.
- Aceite assíncrono (202 e consulta de resultado): opção futura, nunca padrão; fora desta branch.
- Endpoints avulsos de validação do Tracer e `transaction_type`: morrem na lane do time (fechado em 14/10), fora desta branch.
- Nenhum gap vira issue pública. Gaps fora do pedido vão para `~/.claude/gaps/midaz/`.

---

## Phase 1: Experimentos medidos

Objetivo da fase: números, não código de produto. Tudo vive em `tests/bench/engine/` (novo, build tag `bench`), fora dos lanes de CI. Ao fim: harness reprodutível, dois escritores comparáveis, dois transportes comparáveis, cenários de falha provados, corrida longa e relatório go/no-go.

### Epic 1.1: Ambiente Postgres HA reproduzível para o bench

**Goal:** um `docker compose` sobe primário 17 + standby síncrono com latência injetável entre eles, e um bootstrap aplica o esquema real do banco `transaction` mais as tabelas do escritor.
**Scope:** `tests/bench/engine/` (compose, bootstrap Go, DDL bench-local).
**Dependencies:** none
**Done when:** `pg_stat_replication` mostra `standby1 | sync`; o bootstrap aplica as 44 versões de migração reais (000000 a 000043) mais as tabelas do escritor e semeia N saldos em tempo conhecido.
**Status:** Pending

#### Task 1.1.1: Compose com primário, standby síncrono e toxiproxy na replicação

- [ ] Done

**Context:** o compose atual em `components/infra/docker-compose.yml:65-155` sobe primário e réplica ASSÍNCRONA (`wal_level=logical`, sem `synchronous_standby_names`). O bench precisa de commit síncrono e de latência controlada entre primário e standby, porque o RTT entre primário e standby muda o custo do commit. O repo já depende do toxiproxy (cliente `github.com/Shopify/toxiproxy/v2` em `go.mod:13`, módulo testcontainers em `go.mod:49`; uso em `tests/utils/chaos/network.go`). Mordor tem IP público: toda porta publicada fica em `127.0.0.1:`.

**Implementation vision:** novo `tests/bench/engine/docker-compose.yml` com três serviços na mesma rede, cada um com `container_name` fixo (os testes chamam `docker stop`, `docker kill` e `docker exec` por esse nome): `bench-pg-primary` (postgres:17; no `command`: `wal_level=replica`, `max_wal_senders=5`, `max_connections=300`, `shared_buffers=4GB`, `max_wal_size=8GB`, `checkpoint_timeout=15min`, `log_checkpoints=on`; `synchronous_commit` fica no default `on` e NÃO entra no `command`, porque uma opção de linha de comando vence `ALTER SYSTEM` e o controle negativo de 1.3.1 precisa mudar essa configuração em tempo de execução; `POSTGRES_HOST_AUTH_METHOD` com a linha `host replication all 0.0.0.0/0 scram-sha-256` no padrão de `components/infra/docker-compose.yml:81`); `bench-toxiproxy` (imagem `ghcr.io/shopify/toxiproxy:2.12.0`, a mesma versão do cliente em `go.mod:13`; API em `127.0.0.1:8474`; dois proxies por config JSON montada: `pg-repl` escuta `:5433` e aponta para `bench-pg-primary:5432`; `pg-client` escuta `:5434` e aponta para o mesmo primário, usado pelo harness para cortar respostas em 1.3.1); `bench-pg-standby` (postgres:17; `pg_basebackup -R --host=bench-toxiproxy --port=5433` com `primary_conninfo` carregando `application_name=standby1`; `hot_standby=on`; e `-c synchronous_standby_names=''` no `command`, porque o `pg_basebackup` copia o `postgresql.auto.conf` do primário e, sem essa sobreposição, o standby promovido em 1.3.1 esperaria para sempre por um `standby1` que não existe). No primário, `synchronous_standby_names` NÃO vai no `command`: o entrypoint da imagem sobe um servidor temporário só em socket local para criar o banco, e um commit síncrono ali espera para sempre por um standby que não consegue conectar. Vai em `init-primary.sql`, montado em `/docker-entrypoint-initdb.d/`, como `ALTER SYSTEM SET synchronous_standby_names = 'FIRST 1 (standby1)'`, que só vale no start definitivo. Portas no host, todas em loopback: primário `127.0.0.1:5432`, standby `127.0.0.1:5435`, proxies `127.0.0.1:5433` e `127.0.0.1:5434`, API do toxiproxy `127.0.0.1:8474`. Credenciais fixas do bench: usuário `bench`, senha `bench`, banco `bench_transaction` (o bench não é ambiente de produto; a senha em texto no compose é aceitável e o README diz isso). Decisões: `synchronous_commit=on` (flush remoto) e não `remote_apply`, porque zero perda exige o WAL no disco do standby, não a aplicação, e leitura na réplica não faz parte do bench; latência por toxic `latency` no proxy `pg-repl` em vez de `tc netem`, que exige `NET_ADMIN`; volumes nomeados e `down -v` reseta tudo. Casos reais: standby cai e o primário bloqueia todo commit (comportamento desejado; o harness registra como indisponibilidade, não como perda); toxiproxy reiniciado perde os toxics (o helper de 1.2.4 reaplica antes de cada célula); até o standby anexar pela primeira vez, todo commit no primário espera, então o bootstrap de 1.1.2 só roda depois de `pg_stat_replication` mostrar `sync`.

**Files:**
- Create: `tests/bench/engine/docker-compose.yml`
- Create: `tests/bench/engine/init-primary.sql`
- Create: `tests/bench/engine/toxiproxy.json`
- Create: `tests/bench/engine/README.md`

**Verification:** `docker compose -f tests/bench/engine/docker-compose.yml up -d --wait && psql "postgres://bench:bench@127.0.0.1:5432/bench_transaction" -c "select application_name, sync_state from pg_stat_replication"` imprime uma linha `standby1 | sync`. Em seguida `docker stop bench-pg-standby` e um `INSERT` no primário fica bloqueado até `docker start`.

**Done when:** as duas verificações passam do zero em menos de 2 min.

#### Task 1.1.2: Bootstrap do esquema real e das tabelas do escritor

- [ ] Done

**Context:** as migrações do banco `transaction` estão em `components/ledger/migrations/transaction/` (44 versões, 000000 a 000043: `transaction`, `operation`, `balance`, `transaction_group`, routes). A tabela `transaction` não tem coluna de chave de idempotência (`000000_create_transaction_table.up.sql`); no bench, a ligação entre requisição e transação vive só em `engine_request.transaction_id`. O bench grava no esquema REAL para que o custo de índice seja o de produção. `tests/integration/ledger_migrations_integration_test.go:110-113` mostra como aplicar essas migrações com `golang-migrate` sobre um `*sql.DB`. Todo arquivo Go novo carrega o cabeçalho de licença que `scripts/check-license-header.sh` exige.

**Implementation vision:** pacote Go `tests/bench/engine` com `//go:build bench` em todo arquivo. `main_test.go` define `env.go`-style leitura de quatro variáveis: `BENCH_PG_PRIMARY_DSN` (`127.0.0.1:5432`), `BENCH_PG_CLIENT_PROXY_DSN` (`127.0.0.1:5434`), `BENCH_PG_STANDBY_DSN` (`127.0.0.1:5435`) e `BENCH_TOXIPROXY_URL` (`http://127.0.0.1:8474`), e um helper `requireDB(t) *pgxpool.Pool` que faz `t.Skip` quando o DSN primário falta. `TestMain` NÃO pula o pacote inteiro: os testes puros de 1.2.1 rodam sem banco; só quem chama `requireDB` pula. `bootstrap.go` expõe `bootstrap(ctx, pool, balances int)`: espera `pg_stat_replication` mostrar `standby1 | sync` (até 120 s; senão erro), `DROP SCHEMA public CASCADE; CREATE SCHEMA public` (o bench é descartável), aplica as migrações reais com `golang-migrate` apontando para `components/ledger/migrations/transaction`, aplica `schema_bench.sql` (as três tabelas abaixo mais `ALTER TABLE balance SET (fillfactor = 70)`), e semeia por `COPY`: uma organization e um ledger fixos (UUIDs constantes no código), `balances` contas com `available` de 10^9, `allow_sending` e `allow_receiving` verdadeiros e as colunas de overdraft da migração 000031 no default (sem overdraft), uma conta quente com `available` de 10^12 e mesmos flags, uma conta de tarifa (`fee`, usada pela célula de perna de tarifa em 1.2.4), uma linha em `account_control` por conta com `limit_remaining` igual a 10^12, `blocked=false` e `version=0`, e a linha da fence do tenant em `engine_writer_fence` (`owner_id=''`, `epoch=0`, `lease_until=now()`), porque `takeover()` faz `UPDATE ... RETURNING` e sem linha não há dono. O `body` JSONB gravado em `transaction` e o `snapshot` de `operation` têm o tamanho de um create v2 real (um payload fixo de referência copiado de um teste de integração existente, cerca de 1 KB), para que os bytes de WAL medidos não saiam menores que os de produção. `pgx/v5` (`go.mod:33`) via `pgxpool`, sem ORM. `engine_request` é a intenção: `transaction_id` é o id gerado na aplicação, `outcome` o resultado e `reason` o motivo da recusa (`blocked`, `insufficient_funds`, `limit`, `expired`); uma intenção recusada não tem linha em `transaction`. Decisão: o DDL das tabelas do escritor é bench-local; a migração real é da Epic 2.2 e pode mudar de forma. Caso real: rodar duas vezes seguidas deixa o banco igual à primeira rodada.

O DDL que as Epics 1.2 e 1.3 compartilham (contrato do bench):

```sql
CREATE TABLE engine_writer_fence (
  tenant_id   text PRIMARY KEY,
  owner_id    text NOT NULL,
  epoch       bigint NOT NULL,
  lease_until timestamptz NOT NULL
);
CREATE TABLE engine_request (
  tenant_id      text NOT NULL,
  request_key    text NOT NULL,
  transaction_id uuid NOT NULL,
  outcome        text NOT NULL,
  reason         text NOT NULL DEFAULT '',
  deadline       timestamptz NOT NULL,
  committed_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, request_key)
);
CREATE TABLE account_control (
  organization_id uuid NOT NULL,
  ledger_id       uuid NOT NULL,
  account_id      uuid NOT NULL,
  blocked         boolean NOT NULL DEFAULT false,
  limit_remaining numeric NOT NULL,
  version         bigint NOT NULL,
  PRIMARY KEY (organization_id, ledger_id, account_id)
);
```

**Files:**
- Create: `tests/bench/engine/main_test.go`
- Create: `tests/bench/engine/env.go`
- Create: `tests/bench/engine/bootstrap.go`
- Create: `tests/bench/engine/schema_bench.sql`
- Create: `tests/bench/engine/testdata/create_v2_body.json`
- Test: `tests/bench/engine/bootstrap_test.go`

**Verification:** `BENCH_PG_PRIMARY_DSN=postgres://bench:bench@127.0.0.1:5432/bench_transaction go test -tags bench ./tests/bench/engine/ -run TestBootstrap -v` reporta `schema_migrations.version = 43` (a última do diretório), 3 tabelas bench presentes e `count(*) from balance` igual a N.

**Done when:** bootstrap com 10.000 saldos termina em menos de 60 s; com 1.000.000 em menos de 10 min; rodar duas vezes dá o mesmo estado.

### Epic 1.2: Harness de carga com os dois escritores

**Goal:** um gerador de carga da conta quente alimenta dois escritores intercambiáveis (dono sequencial com micro-lote; sem dono com row lock) e mede TPS, p50, p99, p999 e tamanho de lote por célula da matriz.
**Scope:** `tests/bench/engine/`, `mk/tests.mk`.
**Dependencies:** Epic 1.1
**Done when:** `BENCH_MATRIX=quick make test-bench-engine` imprime uma tabela e sai 0; `BENCH_MATRIX=full` preenche a matriz inteira.
**Status:** Pending

#### Task 1.2.1: Gerador de carga da conta quente

- [ ] Done

**Context:** o alvo do Fred é 8k TPS numa conta limitada. O gargalo de hoje é a serialização por conta dentro do EVAL (`docs/architecture/engine.md`, seção "Preflight, ordered execution, and commit"). O gerador produz a pior forma: toda transação toca a MESMA conta quente e uma contraparte aleatória.

**Implementation vision:** `writer.go` define o contrato que os dois escritores e o gerador compartilham: `type request struct{ key uuid.UUID; postings []posting; t0, scheduledAt, deadline time.Time }`, `type outcome struct{ status, reason string; committedAt time.Time }` com `status` em `approved | rejected | expired | unknown`, e `type Writer interface { Submit(ctx, request) (outcome, error); Close() }`. A interface se justifica por duas implementações reais neste pacote mais o escritor nulo. `workload.go` gera a carga em dois regimes: **malha fechada** (`concurrency` produtores, default 256, cada um espera o outcome antes da próxima requisição) mede o TPS máximo; **malha aberta** (`rate` requisições por segundo, default 8.000; o horário de cada envio é `start + i/rate`, calculado por aritmética e não por `time.Ticker`, que a 125 µs perde ticks quando o scheduler atrasa; o gerador dorme até o horário e envia sem esperar a resposta anterior) mede latência a uma taxa fixa, com latência contada a partir de `scheduledAt` e não do envio, para que uma parada do escritor apareça no p99 em vez de sumir (coordinated omission); a taxa alcançada é medida pelos envios reais, e é ela que decide a marca "não sustenta" de 1.2.4. Três cargas: `outflow` (débito da conta quente para uma contraparte aleatória entre as N; `available` e `limit_remaining` da quente mordem), `inflow` (crédito de uma contraparte aleatória na quente; só `blocked` morde) e `mixed` (50% de cada, intercalados, de modo que um crédito recém-aprovado financia um débito no mesmo lote). Variação `feeLeg`: cada transação ganha uma terceira perna creditando a conta de tarifa, uma segunda linha quente. `amount` inteiro entre 1 e 100. `metrics.go` guarda `committedAt - scheduledAt` por célula e calcula percentis por ordenação (120 s a 8k dão 960k amostras; cabe). Semântica do limite: `limit_remaining` só desce; a célula "limite esgota" semeia o limite em 50% do volume de débito esperado, então metade final dos débitos é rejeitada em memória sem tocar linha, e o relatório mostra TPS e p99 das duas metades separadas. Casos reais: `rejected` conta como decisão válida, à parte; `unknown` só nasce em 1.3.1 e o gerador reenvia com a mesma `key`; requisições enfileiradas e nunca enviadas quando um escritor encerra recebem `unknown` também; `deadline` é `scheduledAt` mais `d` (default 30 s, parametrizável por célula) e o relatório falha se algum `approved` tem `committedAt` depois do `deadline`.

**Files:**
- Create: `tests/bench/engine/writer.go`
- Create: `tests/bench/engine/workload.go`
- Create: `tests/bench/engine/metrics.go`
- Test: `tests/bench/engine/workload_test.go`

**Verification:** `go test -tags bench ./tests/bench/engine/ -run 'TestWorkloadKeysUnique|TestOpenLoopSchedule|TestPercentiles' -v` roda sem banco (nenhum desses testes chama `requireDB`) e mostra 100.000 chaves únicas, destinos cobrindo N contas, agendamento em malha aberta com desvio menor que 1 ms na média, e percentis corretos sobre uma amostra fixa.

**Done when:** os três testes passam e o gerador sustenta 20k requisições/s em malha fechada contra um escritor nulo (`Writer` que responde na hora), provando que o gargalo medido depois é do escritor e não do gerador.

#### Task 1.2.2: Escritor com dono (sequencial, micro-lote adaptativo)

- [ ] Done

**Context:** o destino aprovado: um escritor por tenant decide em sequência com um lote em voo. A fence entra no commit. Hoje os guards de execução vivem no Redis (`docs/architecture/engine.md`, seção "Execution guards, receipts, and recovery"); no bench, `engine_request` assume esse papel.

**Implementation vision:** `writer_owner.go`: uma goroutine consome a fila; enquanto o lote k está em COMMIT, o lote k+1 acumula até `maxBatch` (default 256, parametrizável porque 1.2.4 varre esse valor). Estado em memória: `map[accountID]balanceState{available, version}` e `map[accountID]controlState{blocked, limitRemaining, version}` carregados em `takeover()`: `UPDATE engine_writer_fence SET owner_id=$1, epoch=epoch+1, lease_until=now()+lease RETURNING epoch`, depois leitura por keyset em páginas de 50.000 (`balance` por `id`; `account_control` por `account_id`, porque a tabela não tem `id`), renovando o lease a cada página, porque 1M linhas levam mais que os 3 s do lease; o takeover falha se em algum momento `lease_until` já passou. Ao tirar da fila, pedido com `deadline` vencido recebe `expired` sem entrar no lote (o corte da entrada); na decisão, `now() > deadline` de novo → `expired`, com intenção gravada e motivo; depois `blocked` → rejected; `available < amount` e sem overdraft → rejected; `limit_remaining < amount` → rejected; senão aprovada e o estado em memória avança (um crédito aprovado no lote financia um débito seguinte no mesmo lote). Um gancho de teste `beforeBegin func()` (nil fora dos testes) dispara depois das decisões em memória e ANTES do `BEGIN`, fora de qualquer lock. Por lote, UMA transação Postgres com deadline de contexto de 10 s: `UPDATE engine_writer_fence SET lease_until=now()+lease WHERE tenant_id=$1 AND owner_id=$2 AND epoch=$3` (rows ≠ 1 → ROLLBACK e o dono encerra com erro `stale owner`); `INSERT transaction` multi-row; `INSERT operation` multi-row (2 por transação, 3 com `feeLeg`, com `balance_version` do estado em memória); `UPDATE balance AS b SET available = u.available, version = u.version FROM unnest($ids, $availables, $versions, $expected) AS u(...) WHERE b.id = u.id AND b.version = u.expected` em UMA instrução para todas as contas tocadas no lote (rows ≠ contas tocadas → ROLLBACK e o dono encerra: estado em memória divergiu); mesmo padrão para `account_control`; `INSERT engine_request ... ON CONFLICT (tenant_id, request_key) DO NOTHING` e, se alguma chave conflitou, a transação correspondente sai do lote e recebe o outcome já gravado (replay). Só depois do COMMIT retornar sem erro o escritor responde às requisições do lote. Erro no COMMIT sem resposta do servidor (erro de rede depois de enviar o comando, ou o deadline de 10 s, que no `pgx` v5 fecha o socket sem enviar cancel) → cada requisição do lote recebe `unknown`; o estado em memória é descartado; requisições ainda na fila recebem `unknown`; o dono encerra (1.3.1 exercita o retry). Proibido: `statement_timeout` ou cancel explícito durante o COMMIT, porque o Postgres responde a um cancel na espera de replicação síncrona com COMMIT bem-sucedido mais um WARNING, e isso seria um 201 falso. Decisões: `synchronous_commit` fica o do servidor, nunca `SET LOCAL synchronous_commit=off`; nenhum retry por versão estale; lease default 3 s renovado a cada lote e, na ausência de lotes, por um ticker de 1 s. Caso real: duas transações do lote na mesma contraparte viram uma só linha no `unnest`.

**Files:**
- Create: `tests/bench/engine/writer_owner.go`
- Test: `tests/bench/engine/writer_owner_test.go`

**Verification:** `go test -tags bench ./tests/bench/engine/ -run TestOwnerWriterConservation -v` contra o compose: 200.000 requisições com 256 produtores; ao fim, `sum(available)` de todas as contas igual ao valor semeado; `count(*) from transaction` igual ao número de outcomes `approved`; `count(*) from engine_request` igual ao total de requisições; `engine_request where outcome='approved'` faz join um para um com `transaction` por `transaction_id` (nenhuma requisição aprovada sem linha, nenhuma linha sem requisição).

**Done when:** o teste passa 5 vezes seguidas e o log imprime o lote médio e o máximo observados.

#### Task 1.2.3: Escritor sem dono (lotes concorrentes com row lock)

- [ ] Done

**Context:** baseline obrigatória antes de aceitar o dono: várias conexões concorrentes, cada uma com sua transação Postgres, `SELECT ... FOR UPDATE` nas linhas tocadas, decisão pela linha, gravação, COMMIT. Sem estado em memória, sem fence: a linha é a verdade e a `engine_request` garante o replay.

**Implementation vision:** `writer_ownerless.go`: `writers` goroutines (default 32), cada uma drena a fila até o MESMO `maxBatch` do dono (default 256; o que estiver na fila, sem esperar encher), coleta o conjunto de contas tocadas, ordena por `account_id` (ordem canônica evita deadlock entre escritores) e abre uma transação com o mesmo deadline de 10 s: `SELECT id, available, version FROM balance WHERE id = ANY($1) ORDER BY id FOR UPDATE`, o mesmo para `account_control`, corta os vencidos como em 1.2.2 e decide cada transação do lote pela linha (na ordem do lote, então um crédito financia um débito seguinte), grava com as mesmas instruções de 1.2.2 (sem a fence, com o `unnest`) e faz COMMIT. A comparação com o dono só vale porque os dois drenam com a mesma regra; um tamanho de lote menor aqui decidiria o resultado por parâmetro. Casos reais: `40P01` (deadlock detectado) devolve o lote para a fila uma vez e conta em `deadlocks`; `INSERT engine_request` em conflito recebe o outcome gravado; ack de COMMIT perdido ou deadline vira `unknown` como no dono, com as mesmas proibições de cancel no COMMIT.

**Files:**
- Create: `tests/bench/engine/writer_ownerless.go`
- Test: `tests/bench/engine/writer_ownerless_test.go`

**Verification:** `go test -tags bench ./tests/bench/engine/ -run TestOwnerlessWriterConservation -v` com as mesmas asserções de 1.2.2 mais `deadlocks` impresso.

**Done when:** o teste passa 5 vezes seguidas.

#### Task 1.2.4: Matriz de medição, alvo make e saída

- [ ] Done

**Context:** a comparação só vale com as mesmas condições: mesmo esquema, mesma carga, mesma latência de replicação. O repo já descobre pacotes por tag (`scripts/discover-tagged-test-packages.sh`, `make test-gate-selection`), e a CI roda seis shards de integração em todo PR (job `integration` em `.github/workflows/pr-validation.yml`, `scripts/run-integration-ci-shard.sh`); a tag `bench` não pode entrar em nenhum lane nem shard.

**Implementation vision:** `toxiproxy.go` fala com a API do toxiproxy (`BENCH_TOXIPROXY_URL`) usando o cliente `github.com/Shopify/toxiproxy/v2/client` já em `go.mod:13`: `setReplicationLatency(ms)` remove todos os toxics de `pg-repl` e adiciona um `latency` com `latency=ms, jitter=0`; falha da API é erro do teste (não medir sem saber a latência). `bench_test.go` roda `TestBenchHotAccount` em três blocos, cada célula com bootstrap de 10.000 saldos, 60 s de aquecimento e 120 s de medição. Bloco A, TPS máximo em malha fechada: {owner, ownerless} × {RTT 0, 1, 10 ms} × {outflow, inflow, mixed}. Bloco B, latência a 8.000/s em malha aberta: as mesmas 18 células; uma célula que não sustenta 8.000/s fica marcada "não sustenta" com o TPS que alcançou. Bloco C, só na célula RTT 1 ms outflow, em malha fechada (é TPS máximo que decide o vencedor): para o dono, varredura de `maxBatch` em {1, 8, 64, 256}; para o sem dono, varredura cruzada de `writers` em {1, 4, 32} por `maxBatch` em {1, 8, 64, 256}, porque em malha fechada com 256 produtores e 32 escritores cada commit carrega cerca de 256/32 requisições, qualquer que seja o `maxBatch`, e sem varrer `writers` o resultado sairia do parâmetro e não do desenho; mais a célula "limite esgota" e a célula `feeLeg` para os dois escritores na melhor configuração de cada um. Bloco D, opcional e condicionado ao go do Fred: owner e ownerless, outflow, malha fechada e aberta, contra um RDS Multi-AZ de instância temporário (PostgreSQL 18 ou 16, `db.m7g.large`, gp3) provisionado na conta devops a partir do módulo de `environments/devops/rds/` do `lerian-infrastructure` e destruído ao fim (cerca de US$ 15 por dia); o gerador roda de um host na mesma VPC, nunca de mordor pela internet. Sem o go, as células do bloco D ficam "não medido". Por célula: `TPS`, `p50`, `p99`, `p999`, lote médio e máximo (owner) ou `deadlocks` (ownerless), `pg_stat_wal.wal_bytes` e `wal_fpi` por segundo, `pg_stat_checkpointer.num_requested` (no PostgreSQL 17 os contadores de checkpoint saíram de `pg_stat_bgwriter`), `pg_stat_replication.flush_lag` máximo. Escreve tabela markdown em `tests/bench/engine/out/<timestamp>.md` (diretório no `.gitignore`) e imprime no log. `BENCH_MATRIX=quick` roda uma célula (owner, RTT 1 ms, outflow, malha aberta) por 20 s. Novo alvo em `mk/tests.mk`: `test-bench-engine` sobe o compose com `--wait`, exporta os DSNs, roda `go test -tags bench ./tests/bench/engine/ -run TestBenchHotAccount -timeout 120m -v` e derruba com `down -v` mesmo em falha (`trap`). O relatório registra como "não medido" o salto de rede entre o pod que recebe a requisição e o pod do dono, que este bench não tem, e o tracer, que fica fora do commit. Casos reais: célula que aborta (dono encerra por versão divergente) fica registrada como "abortou: motivo" e a matriz segue.

**Files:**
- Create: `tests/bench/engine/toxiproxy.go`
- Create: `tests/bench/engine/bench_test.go`
- Modify: `mk/tests.mk` (novo alvo `test-bench-engine` ao lado dos alvos de teste existentes)
- Modify: `.gitignore` (`tests/bench/engine/out/`)

**Verification:** `BENCH_MATRIX=quick make test-bench-engine` sai 0 e imprime uma tabela com uma célula. `make test-gate-selection` continua verde e nenhuma linha de `.github/workflows/` nem de `scripts/run-integration-ci-shard.sh` passa a tag `bench` ao `go test` (a tag não entrou em nenhum lane nem shard de CI) e `golangci-lint run --build-tags bench ./tests/bench/engine/` sai limpo (o `make lint` não passa a tag, então nunca veria este pacote).

**Done when:** as três verificações passam; `BENCH_MATRIX=full make test-bench-engine` produz os três blocos em menos de 120 min.

### Epic 1.3: Cenários de falha

**Goal:** provar zero perda e zero duplicação com queda do primário no meio de um lote, com dono estale barrado pela fence, e medir o takeover com 1M saldos.
**Scope:** `tests/bench/engine/`.
**Dependencies:** Epic 1.2
**Done when:** os três testes passam 20 rodadas seguidas e o tempo de takeover está no log.
**Status:** Pending

#### Task 1.3.1: COMMIT perdido e retry com a mesma identidade

- [ ] Done

**Context:** ack de COMMIT perdido vira "unknown, retry com a mesma identidade". O risco real: o primário grava, o standby dá flush, e a resposta ao cliente se perde; um retry sem identidade duplica a transação.

**Implementation vision:** `failover_test.go`, `TestLostCommitAck`, roda para os DOIS escritores (o vencedor de 1.5.2 precisa da prova, e ainda não se sabe qual é). O escritor conecta pelo proxy `pg-client` (`:5434`); o gerador guarda em memória o conjunto de `request_key` que recebeu `approved`. O teste controla as rodadas por dentro (subtestes `round-01` a `round-20`), porque `-count` não reconstrói o ambiente: um helper `rebuild(t)` faz `docker compose down -v`, `up -d --wait` e `bootstrap`. Dentro de cada rodada a ordem é A, depois C, depois B, porque só B destrói o par primário e standby; assim são 20 `rebuild` por escritor e não 60, e o orçamento de 120 min fecha (cerca de 2 min por rodada de carga mais reconstrução). Variante A, resposta perdida: com a carga em andamento, o teste adiciona o toxic `timeout` (downstream, `timeout=0`: o toxiproxy segura os bytes sem fechar o socket) em `pg-client`; o COMMIT em voo pode ter sido aplicado ou não; o deadline de 10 s do escritor fecha o socket, o lote recebe `unknown` e o escritor encerra. O teste remove o toxic, um escritor novo assume (dono: `takeover()` com epoch+1) e o gerador reenvia toda requisição `unknown` com a mesma `request_key`. Variante B, primário morto: `docker kill -s KILL bench-pg-primary`, `SELECT pg_promote()` no standby, DSN trocado para `127.0.0.1:5435` (o promovido fica sem standby, então a variante B segue com `synchronous_standby_names` vazio e o teste registra isso), mesmo retry. Variante C, standby fora do ar: `docker stop bench-pg-standby` com carga em andamento; todo commit fica esperando; a asserção é zero `approved` novos e só `unknown` até o `docker start`; depois do start, o retry com a mesma chave recebe o outcome gravado para os commits que o primário já tinha aplicado localmente. É a prova de "sem downgrade silencioso"; em RDS Multi-AZ de instância essa variante não existe, porque a AWS deixa o primário seguir sem replicação. Variante D, só com o RDS temporário do bloco D de 1.2.4: `aws rds reboot-db-instance --force-failover` com carga em andamento; as asserções são as mesmas, e o tempo até o primeiro `approved` depois do failover entra no relatório ao lado do orçamento da definição 3. Asserções em toda variante, lidas do nó que ficou primário: cada `request_key` aparece no máximo uma vez em `engine_request`; TODA chave que o gerador viu `approved` existe em `engine_request` com `outcome='approved'` e tem sua linha em `transaction` (é esta asserção que pega a perda de um commit confirmado; contagem sozinha não pega, porque as duas linhas somem juntas); `count(transaction)` igual a `count(engine_request where outcome='approved')`; conservação de saldos; toda requisição `unknown` terminou com um outcome definitivo. Controle negativo, uma rodada por escritor: variante B com replicação assíncrona forçada antes da carga (`ALTER SYSTEM SET synchronous_standby_names = ''` seguido de `SELECT pg_reload_conf()` no primário, e um toxic `latency` de 50 ms em `pg-repl` para garantir uma janela de commits confirmados que o standby ainda não tem; sem o atraso, o standby ficaria a menos de 1 ms e a perda seria só provável); o teste espera que a asserção de chaves aprovadas FALHE (o subteste passa se a falha foi detectada), provando que o teste enxerga perda. Decisão: promoção manual dentro do teste (a orquestração real de HA é infra, Fase 2). Caso real: COMMIT que nunca chegou ao servidor faz o retry executar de fato, e isso também é correto.

**Files:**
- Create: `tests/bench/engine/failover_test.go`
- Create: `tests/bench/engine/compose.go` (helper `rebuild`, `dockerKill`, `dockerStop`, `dockerStart`, `promote`)

**Verification:** `go test -tags bench ./tests/bench/engine/ -run TestLostCommitAck -timeout 120m -v` roda 20 rodadas de A, B e C por escritor mais o controle negativo.

**Done when:** 20 de 20 rodadas passam nas três variantes para os dois escritores, e o controle negativo detecta a perda.

#### Task 1.3.2: Dono estale barrado pela fence

- [ ] Done

**Context:** dois donos ao mesmo tempo é a falha clássica do lease: o antigo acorda depois de uma pausa e tenta gravar um lote decidido sobre estado velho.

**Implementation vision:** `TestStaleOwnerFenced` em `failover_test.go`: dono A (epoch 1) recebe o gancho `beforeBegin` de 1.2.2, que o bloqueia num canal DEPOIS das decisões em memória e ANTES do `BEGIN`, então A não segura lock nenhum (um gancho depois do `UPDATE` da fence prenderia a linha e o takeover de B ficaria esperando: deadlock do teste); dono B faz `takeover()` (epoch 2) e grava 1.000 transações; o teste libera A, que abre a transação do lote velho: o `UPDATE engine_writer_fence ... AND epoch=1` afeta 0 linhas, A faz ROLLBACK e encerra com `stale owner`. Asserções: nenhuma linha do lote de A em `transaction` ou `engine_request`; as versões em `balance` são as que B gravou; conservação.

**Files:**
- Modify: `tests/bench/engine/failover_test.go`

**Verification:** `go test -tags bench ./tests/bench/engine/ -run TestStaleOwnerFenced -count=20`.

**Done when:** 20 de 20 rodadas passam.

#### Task 1.3.3: Tempo de takeover com 1M saldos

- [ ] Done

**Context:** a definição 3 (orçamento de indisponibilidade) precisa de um número real: quanto custa o novo dono recarregar saldos e controles de um tenant grande.

**Implementation vision:** `TestTakeoverReload` em `failover_test.go`, gate `BENCH_BALANCES=1000000`: bootstrap com 1.000.000 saldos e controles; medir `takeover()` de ponta a ponta (incremento de epoch, keyset em páginas de 50.000 com renovação do lease a cada página, pronto para o primeiro lote), o RSS do processo antes e depois, e afirmar que `lease_until` nunca ficou no passado durante a carga (o teste lê a fence a cada segundo em paralelo). Decisão: carregar tudo na Fase 1 para ter o pior número; carga lazy por conta é otimização da Fase 2 só se o número estourar o orçamento. Caso real: página de keyset falha por timeout → o takeover falha inteiro e reporta (não existe takeover parcial).

**Files:**
- Modify: `tests/bench/engine/failover_test.go`
- Modify: `tests/bench/engine/bootstrap.go` (parâmetro N vindo de `BENCH_BALANCES`)

**Verification:** `BENCH_BALANCES=1000000 go test -tags bench ./tests/bench/engine/ -run TestTakeoverReload -v -timeout 30m` imprime tempo total, tempo por página e RSS.

**Done when:** o número está no log e no relatório de 1.5.2, com a comparação contra o default de 5 s.

### Epic 1.4: Eixo de transporte: direto contra Redpanda

**Goal:** o mesmo autorizador (o núcleo de 1.2.2, com os mesmos lotes) medido por dois transportes: direto (o gerador chama o escritor em processo, como em 1.2) e Redpanda (o gerador publica num tópico por tenant com `acks=all`; um worker por tenant consome, decide e commita o lote com o offset consumido no mesmo commit; a resposta volta por um tópico de respostas). Mede o custo do Redpanda no caminho do 201 e prova zero perda e zero dupla aplicação na queda e no rebalance do worker.
**Scope:** `tests/bench/engine/` (compose, transporte Redpanda, células).
**Dependencies:** Epic 1.2, Epic 1.3
**Done when:** a matriz de transporte tem TPS máximo, p99 a 8.000/s, latência até a persistência e até a resposta para as duas vias, com e sem carga alheia no Redpanda; a célula "backlog" mostra zero pedido commitado depois do deadline; 1.4.3 passa 20 rodadas.
**Status:** Pending

#### Task 1.4.1: Redpanda no compose e tabela de offset

- [ ] Done

**Context:** o Redpanda só entra nesta célula; o motor não depende dele (destino: sem infra nova obrigatória). O repo já usa a imagem `redpandadata/redpanda:v24.2.7` (suítes de carga e e2e) e o cliente `github.com/twmb/franz-go` v1.21.6 (`go.mod:51`, com `kadm` em `go.mod:52`). Mordor tem IP público: toda porta publicada fica em `127.0.0.1:`.

**Implementation vision:** serviço `bench-redpanda` no compose de 1.1.1 sob o perfil `transport`, para que as células de 1.2 e 1.3 subam sem ele: um nó, `--mode dev-container`, Kafka API em `127.0.0.1:19092`, admin em `127.0.0.1:19644`, `default_topic_replications=1`. O bootstrap cria dois tópicos por `kadm`: `engine-intents` com uma partição por tenant (o bench tem um tenant, então uma partição; a célula "carga alheia" cria `foreign-events` com 8 partições) e `engine-replies` com uma partição. Tabela bench-local nova em `schema_bench.sql`: `engine_consumer_offset (tenant_id text, topic text, partition int, next_offset bigint, epoch bigint, PRIMARY KEY (tenant_id, topic, partition))`, semeada em zero. Decisão: o offset gravado é o PRÓXIMO a consumir, e o update é condicional (`WHERE next_offset = $esperado AND epoch = $epoch`), porque um worker revogado no rebalance que ainda está no COMMIT precisa falhar nessa linha e não aplicar o lote duas vezes. Caso real: `kadm` em tópico já existente devolve `TOPIC_ALREADY_EXISTS` e o bootstrap segue.

**Files:**
- Modify: `tests/bench/engine/docker-compose.yml` (serviço `bench-redpanda`, perfil `transport`)
- Modify: `tests/bench/engine/schema_bench.sql` (`engine_consumer_offset`)
- Modify: `tests/bench/engine/bootstrap.go` (tópicos via `kadm`)
- Modify: `tests/bench/engine/env.go` (`BENCH_REDPANDA_BROKERS`, default `127.0.0.1:19092`)

**Verification:** `docker compose --profile transport up --wait` sobe o Redpanda; `go test -tags bench ./tests/bench/engine/ -run TestBootstrapTransport -v` mostra os dois tópicos e a linha de offset em zero.

**Done when:** a verificação passa duas vezes seguidas com o mesmo estado.

#### Task 1.4.2: Transporte Redpanda: produtor síncrono, worker por tenant, resposta por tópico

- [ ] Done

**Context:** o desenho da reunião de 14/10: a API valida sem saldo e publica de forma síncrona; um worker por tenant consome, decide o lote em memória e commita transação, operação, saldo e offset de uma vez; a resposta volta ao chamador por um canal. O bench mede esse canal, porque ele não existe hoje e é a parte mais cara do desenho.

**Implementation vision:** `transport_redpanda.go` implementa `Writer` sobre o núcleo de 1.2.2: o escritor com dono extrai `authorizer.decide(batch, state) (rows, outcomes)`, sem fila própria, e 1.2.2 e este transporte chamam a mesma função (é o contrato "motor por lote, sem transporte"). `Submit` publica a `request` serializada em `engine-intents` com chave `tenant_id` e `acks=all` (`kgo.RequiredAcks(kgo.AllISRAcks())`), espera o ack do produtor (o custo "até a persistência") e bloqueia num mapa `key → chan outcome`. Um worker (`kgo` consumer group `engine-<tenant>`, `FetchMaxWait` de 5 ms) drena até `maxBatch` registros ou até o fetch esvaziar, corta os vencidos, chama `authorizer.decide`, abre UMA transação Postgres com as instruções de 1.2.2 mais `UPDATE engine_consumer_offset SET next_offset = $novo WHERE ... AND next_offset = $esperado AND epoch = $epoch` (rows ≠ 1 → ROLLBACK e o worker encerra como `stale consumer`), faz COMMIT, e só então publica cada `outcome` em `engine-replies` com a `key` do pedido; o lado do produtor consome `engine-replies` e fecha o canal da `key`. Offsets do consumer group NÃO são commitados no broker (`DisableAutoCommit`, nenhum `CommitOffsets`): ao subir, o worker lê `engine_consumer_offset` e faz `seek` para `next_offset`; o broker só ordena e retém. Por pedido, duas latências: `ackedAt - scheduledAt` (persistido) e `repliedAt - scheduledAt` (o 201 equivalente). Casos reais: registro com `key` já em `engine_request` (reentrega depois de um COMMIT cujo offset avançou mas a resposta não saiu) recebe o outcome gravado, sem nova decisão; resposta publicada com produtor já encerrado é descartada; worker que perde a partição no rebalance com lote em COMMIT falha no update condicional e o lote não é aplicado duas vezes.

**Files:**
- Create: `tests/bench/engine/transport_redpanda.go`
- Modify: `tests/bench/engine/writer_owner.go` (extrair `authorizer.decide`, sem mudar o comportamento de 1.2.2)
- Test: `tests/bench/engine/transport_redpanda_test.go`

**Verification:** `go test -tags bench ./tests/bench/engine/ -run TestRedpandaTransportConservation -v` com as asserções de 1.2.2 mais: `next_offset` igual ao total de registros publicados; nenhum `approved` com `committedAt` depois do `deadline`.

**Done when:** o teste passa 5 vezes seguidas e `TestOwnerWriterConservation` continua passando depois da extração de `authorizer.decide`.

#### Task 1.4.3: Células de transporte, backlog, queda do worker e rebalance

- [ ] Done

**Context:** o Fred julga pelas transições entre componentes. A célula mostra o preço do Redpanda no caminho do 201 e prova que a queda e o rebalance do worker não perdem nem duplicam.

**Implementation vision:** `TestBenchTransport` em `bench_test.go`, RTT 1 ms, outflow, na melhor configuração de lote de 1.2.4: células {direto, Redpanda} × {malha fechada (TPS máximo), malha aberta a 8.000/s (p99 das duas latências)} × {sem carga alheia, com carga alheia}; "carga alheia" é um produtor paralelo publicando 20.000 registros/s de 1 KB em `foreign-events` no MESMO Redpanda (a lição do Valkey compartilhado: o destino exige Redpanda dedicado, e a célula mostra o custo de não ter). Célula "backlog": malha aberta a 8.000/s, `d` de 2 s, worker pausado por 5 s por um gancho de teste; todo pedido enfileirado durante a pausa recebe `expired`, nenhum é commitado depois do próprio deadline, e o relatório mostra quantos venceram. Falhas, 20 rodadas cada: (a) o gancho `afterCommit` encerra o worker entre o COMMIT e a publicação das respostas (o estado em memória morre com a goroutine, como num pod morto); o worker novo faz `seek` para `next_offset`, reentrega o lote, encontra as `key` em `engine_request` e responde com o outcome gravado; (b) rebalance: um segundo worker entra no mesmo consumer group enquanto o primeiro está parado no gancho `beforeCommit`; o primeiro perde a partição, o segundo avança `next_offset` e `epoch`, o primeiro acorda e falha no update condicional. Asserções nas duas: `sum(available)` igual ao semeado; `count(*) from transaction` igual aos `approved`; nenhuma `key` com duas linhas; `next_offset` igual ao total publicado. `make test-bench-engine` passa a subir o perfil `transport` e a rodar `TestBenchTransport` depois de `TestBenchHotAccount`; `BENCH_MATRIX=quick` não roda transporte. Caso real: produtor com `acks=all` e broker indisponível devolve erro e a `request` vira `unknown`.

**Files:**
- Modify: `tests/bench/engine/bench_test.go` (`TestBenchTransport`)
- Modify: `tests/bench/engine/transport_redpanda.go` (ganchos `beforeCommit` e `afterCommit`, nil fora dos testes)
- Modify: `mk/tests.mk` (perfil `transport` no alvo `test-bench-engine`)

**Verification:** `BENCH_MATRIX=full make test-bench-engine` produz a matriz de transporte com as oito células, a célula "backlog" e as duas falhas com 20 rodadas; `golangci-lint run --build-tags bench ./tests/bench/engine/` limpo.

**Done when:** as duas falhas passam 20 rodadas e a matriz tem as duas latências por célula.

### Epic 1.5: Corrida longa e relatório go/no-go

**Goal:** 30 min do escritor vencedor sem degradação e um relatório que decide owner versus ownerless, direto versus Redpanda, e go/no-go da Fase 2.
**Scope:** `tests/bench/engine/`, `docs/plans/`.
**Dependencies:** Epic 1.3, Epic 1.4
**Done when:** o relatório existe, cada número tem célula de origem, e o Fred aprova a Fase 2.
**Status:** Pending

#### Task 1.5.1: Corrida de 30 minutos

- [ ] Done

**Context:** a conta quente é a linha mais atualizada do banco. Com lote de 40, são 200 updates por segundo na mesma linha; sem lote, 8.000. Bloat de `balance`, checkpoints e autovacuum só aparecem com tempo.

**Implementation vision:** `soak_test.go`, `TestSoak30m`, gate `BENCH_SOAK=1`: escritor vencedor (parametrizável, default owner), RTT 1 ms, carga `mixed` em malha aberta a 8.000/s (latência a partir do agendamento, para que uma parada apareça no p99), 30 min. Em paralelo, um leitor de relatório: a cada 5 min abre uma transação `REPEATABLE READ` e varre `operation` por 60 s, segurando o snapshot, porque sem leituras longas o vacuum nunca é pressionado e o soak passaria em condições que a produção não tem. A cada 60 s amostra e imprime: TPS, p99 do minuto, `pg_stat_wal.wal_bytes` e `wal_fpi` delta, `pg_stat_user_tables.n_dead_tup`, `n_tup_hot_upd`, `n_tup_upd` e `autovacuum_count` de `balance`, `account_control` e `engine_writer_fence`, `pg_stat_checkpointer.num_requested`, `pg_stat_replication.flush_lag`, `pg_total_relation_size` de `balance`, `transaction`, `operation`. Falha se o p99 do último minuto passa de 1,5× o p99 do quinto minuto, se `n_dead_tup` de `balance` ou de `engine_writer_fence` cresce monotonicamente depois de dois autovacuums, ou se a razão `n_tup_hot_upd / n_tup_upd` de `balance` cai abaixo de 0,9 (o `fillfactor=70` existe para isso). Caso real: checkpoint forçado por `max_wal_size` aparece em `num_requested`; o relatório registra o pico de p99 naquele minuto em vez de suavizar.

**Files:**
- Create: `tests/bench/engine/soak_test.go`

**Verification:** `BENCH_SOAK=1 go test -tags bench ./tests/bench/engine/ -run TestSoak30m -timeout 45m -v` termina verde e imprime 30 linhas de amostra.

**Done when:** passa uma vez completa com o escritor vencedor.

#### Task 1.5.2: Relatório de resultados e decisão owner versus ownerless

- [ ] Done

**Context:** o Fred não lê código nem log. O relatório é o instrumento que decide a Fase 2.

**Implementation vision:** `docs/plans/2026-09-30-balance-engine-results.md` com: as matrizes de 1.2.4 e 1.4.3 na íntegra; os resultados de 1.3.1, 1.3.2, 1.3.3 e 1.4.3 (rodadas, falhas, tempo de takeover contra o default de 5 s, pedidos vencidos na célula "backlog"); a tabela do soak; a decisão owner versus ownerless pelo critério fixo: o melhor TPS sustentado de cada escritor na varredura do bloco C (RTT 1 ms, outflow; `maxBatch` para o dono, `writers` por `maxBatch` para o sem dono), desde que o escritor tenha passado 1.3.1 nas três variantes; se o ownerless entrega ao menos 80% do owner, o ownerless vence pela simplicidade (sem lease, sem fence, sem estado em memória), senão o owner vence; empate técnico no p99 a 8.000/s (bloco B) desempata pelo menor p999; a decisão direto versus Redpanda pelo critério fixo: o Redpanda só vence se entregar TPS máximo igual ou maior, p99 menor a 8.000/s até a resposta e zero perda e zero dupla aplicação em 1.4.3, e qualquer empate fica com o direto, que tem menos transições; a tabela go/no-go contra os alvos de aceitação deste plano. Célula não medida fica "não medido". O mesmo commit atualiza a `## Phase Overview` deste plano (Fase 1 → Complete) e acrescenta um bloco "Resultado da Fase 1" logo abaixo dela com a decisão do escritor e do transporte, porque as Epics 2.1 e 2.3 dependem delas.

**Files:**
- Create: `docs/plans/2026-09-30-balance-engine-results.md`
- Modify: `docs/plans/2026-09-30-balance-engine-postgres-truth.md`

**Verification:** leitura: cada número do relatório aponta a célula, o teste ou a rodada de origem; nenhum número sem origem.

**Done when:** o Fred lê o relatório e aprova a Fase 2 (gate humano).

---

## Phase 2: Protocolo e adaptador Postgres do motor

Depende do resultado da Fase 1 e das definições 1 a 4. Tarefas elaboradas quando a Fase 1 fechar.

### Epic 2.1: Especificação do protocolo

**Goal:** `docs/architecture/balance-engine-postgres.md` fixa o protocolo: o contrato do autorizador (entra um lote de intenções e o estado carregado; saem as linhas a commitar e os resultados; o transporte é adaptador, direto por gRPC ao pod dono como base e consumidor de Redpanda conforme a definição 8); a intenção como linha do mesmo commit, com resultado e motivo; o deadline (corte na entrada e na decisão; default em `settings` do ledger, 30 s na ausência; header para encurtar; erro explícito ao vencido, nunca 201 tardio); o padrão de telemetria do autorizador (span por pedido, lote e commit; detalhe só no erro); o particionamento de `transaction` e `operation` por tempo (chave e custo no cutover); fence e lease (ou row lock, se o ownerless vencer); identidade da requisição no commit (definição 4) para criar, confirmar, cancelar e estornar, e onde fica o estado de idempotência HTTP que hoje vive no Valkey; tratamento do commit incerto (deadline, sem cancel no COMMIT); controles no commit (definição 1); reload no takeover dentro do orçamento (definição 3); o contrato de hospedagem (definição 2: `SHOW synchronous_commit = on` no boot ou o ledger não sobe; nenhuma API da AWS; failover tratado como conexão perdida, com reconexão pelo DNS e replay do lote `unknown` com a mesma identidade); guard de revert por unique index; e o outbox durável gravado no mesmo commit, com o relay que o drena, para as obrigações que hoje correm depois do commit (metadata no MongoDB, eventos de streaming e acerto da reserva no tracer, em `applied_transaction_completer.go` e na fila em memória de `transaction_reservation_unanswered_settle.go`). O protocolo proíbe liberar ou confirmar reserva do tracer enquanto o resultado SQL for `unknown`.
**Scope:** `docs/architecture/`.
**Dependencies:** Phase 1; definições 1 a 4 e 8 respondidas.
**Done when:** o documento fixa cada escolha com o seu custo; o Fred aprova.
**Status:** Pending

### Epic 2.2: Migrações do banco transaction para o escritor

**Goal:** migrações reais em `components/ledger/migrations/transaction/` para a fence (se owner), a intenção (identidade, resultado, motivo, deadline), os controles (conforme definição 1), o particionamento decidido em 2.1, `fillfactor` em `balance`, e o unique index em `transaction.parent_transaction_id`; com testes de migração no padrão de `000042_create_transaction_group_test.go`.
**Scope:** `components/ledger/migrations/transaction/`, `tests/integration/`.
**Dependencies:** Epic 2.1
**Done when:** `make test-integration` verde; `scripts/migration_linter` verde; migração idempotente sobre banco existente.
**Status:** Pending

### Epic 2.3: Adaptador Postgres do command.Engine para o create singular

**Goal:** `components/ledger/internal/adapters/postgres/engine` implementa `command.Engine` (`components/ledger/internal/services/command/accounting_engine_port.go:52`) com o escritor vencedor da Fase 1; as linhas SQL que a completion grava hoje (`components/ledger/internal/adapters/postgres/completion/`) entram no mesmo commit, e as obrigações pós-commit (metadata no MongoDB, eventos, tracer) saem pelo outbox definido na Epic 2.1, então `AppliedTransactionCompleter` e a recovery não rodam para ledgers neste motor; o contrato `components/ledger/internal/domain/accounting/contract_test.go` passa; create v2 em JSON, inflow e outflow. O adaptador recebe ids: o alias é resolvido uma vez na borda, pela consulta de alias que já existe, nunca dentro do motor. Telemetria no padrão novo: um span por pedido, por lote e por commit; atributos de erro só no erro.
**Scope:** `components/ledger/internal/adapters/postgres/engine/` (novo), `components/ledger/internal/services/command/` (seleção do motor no `createTransactionWithEngine`).
**Dependencies:** Epic 2.2
**Done when:** os testes de integração do create v2 passam com o adaptador Postgres; 201 só depois do commit; uma queda do banco entre a decisão e o commit responde 5xx sem linha gravada e sem saldo movido.
**Status:** Pending

### Epic 2.4: Seleção do motor por ledger

**Goal:** `settings.engine = redis | postgres` (default `redis`) resolvido por ledger no bootstrap (`components/ledger/internal/bootstrap/config.go`) e nas consultas de política, no mesmo padrão de `query.GetCrossLedgerPolicy`; um ledger em `postgres` nunca toca o Valkey para dinheiro. Temporário: a Fase 5 apaga o seletor.
**Scope:** `components/ledger/internal/bootstrap/`, `components/ledger/internal/services/query/`, `components/ledger/internal/services/command/`.
**Dependencies:** Epic 2.3
**Done when:** a suíte de integração do create roda nos dois modos; `settings.engine` inválido falha o boot.
**Status:** Pending

---

## Phase 3: Paridade funcional

### Epic 3.1: Pending, revert e grupos

**Goal:** hold, commit e cancel de pending, com a identidade da requisição também no commit e no cancel (definição 4); uma recusa do tracer vira intenção gravada com motivo e sem lançamento; revert com o guard por unique index; grupos cross-ledger (`transaction_group`) com a atomicidade que o commit único dá de graça dentro de um banco e o protocolo atual entre bancos.
**Scope:** `components/ledger/internal/adapters/postgres/engine/`, `components/ledger/internal/services/command/` (revert, pending, group).
**Dependencies:** Phase 2
**Done when:** suítes de integração de pending, revert e grupo verdes com `settings.engine=postgres`.
**Status:** Pending

### Epic 3.2: Controles financeiros, fee debt e os seams de tracer e fees

**Goal:** block e unblock, exceção de uso único, limites e fee debt (as listas que hoje só existem no Valkey viram tabela) decididos e gravados no mesmo commit; os seams de tracer (`transaction_reservation_anchor.go`) e de fees (`applyFees`) continuam chamando o motor da mesma forma, conforme a definição 10 (default: reserva por gRPC e acerto pelo outbox).
**Scope:** `components/ledger/internal/adapters/postgres/engine/`, `components/ledger/internal/services/command/`, `components/ledger/internal/adapters/postgres/`.
**Dependencies:** Epic 3.1; definição 1.
**Done when:** suítes de account closing, fee debt e block exception verdes no motor Postgres.
**Status:** Pending

### Epic 3.3: Batch atômico v2 e account protection

**Goal:** o batch atômico v2 e a janela de `resolveEngineAdmissions` funcionam no motor Postgres com uma transação por batch.
**Scope:** `components/ledger/internal/services/command/`, adaptador.
**Dependencies:** Epic 3.2
**Done when:** suítes de batch v2 e multi-transaction account protection verdes.
**Status:** Pending

### Epic 3.4: Leitura de saldo e chaos

**Goal:** `query.GetBalances` lê só o Postgres em ledgers `postgres` (sem seed, sem cache de dinheiro) e busca por id, com o alias resolvido uma vez na borda; a leitura de transação por id hidrata a metadata das operações no caminho durável (hoje `resolveEngineTransactionFromPrimary` só carrega a metadata da transação e só a listagem reidrata as operações); point-in-time inalterado; `tests/chaos` verde nos dois motores.
**Scope:** `components/ledger/internal/services/query/`, `tests/chaos/`.
**Dependencies:** Epic 3.3
**Done when:** `make test-integration` e `make test-chaos-system` verdes com todos os ledgers de teste em `postgres`.
**Status:** Pending

---

## Phase 4: Cutover por tenant

### Epic 4.1: Barreira de cutover

**Goal:** comando de operador (`components/ledger/cmd/cutover`) que, por tenant: pausa a escrita conforme a definição 5, drena o Lua (aguarda receipts e recovery), desliga o balance sync worker do tenant, copia Valkey para Postgres (saldos, fee debt, pendências, exceções, e o estado de idempotência e replay das janelas de retry ainda abertas), lendo todo formato histórico do cache (hotfix #2587), verifica igualdade, flipa `settings.engine` e retoma. Pré-condições que o comando verifica: `synchronous_commit=on` no banco de destino. Pré-condições que o operador atesta no runbook: replicação síncrona entre AZs (Multi-AZ no RDS) e dimensionamento da instância (a `db.t4g.small` de hoje não serve).
**Scope:** `components/ledger/cmd/cutover/` (novo), adaptadores redis e postgres.
**Dependencies:** Phase 3; definição 5.
**Done when:** um tenant local migra do zero e a igualdade Valkey versus Postgres é verificada antes do flip.
**Status:** Pending

### Epic 4.2: Ensaio em homologação

**Goal:** o cutover roda em homologação com um tenant copiado de produção (o `lerian-infrastructure` não tem RDS de ledger fora de produção; localizar ou provisionar o de homologação é pré-condição); rollback definido: antes do primeiro commit Postgres, flipar de volta; depois dele, não há volta sem re-seed do Valkey (documentado).
**Scope:** homologação, runbook.
**Dependencies:** Epic 4.1
**Done when:** ensaio registrado com tempos e o rollback testado.
**Status:** Pending

### Epic 4.3: Runbook

**Goal:** `docs/runbooks/balance-engine-cutover.md` com pré-condições (incluindo o contrato de hospedagem da definição 2: Multi-AZ, `synchronous_commit=on`, monitoramento dos eventos de standby degradado do RDS, dimensionamento), passos, verificação e rollback.
**Scope:** `docs/runbooks/`.
**Dependencies:** Epic 4.2
**Done when:** um operador que não participou do plano executa o ensaio só pelo runbook.
**Status:** Pending

---

## Phase 5: Deleção

### Epic 5.1: Remover o motor antigo

**Goal:** apagar `components/ledger/internal/adapters/redis/engine/` (Go e Lua), o write-behind por RabbitMQ, o recovery runner e `engine:{transactions}:recover` (só depois que o relay do outbox da Epic 2.1 cobre metadata, eventos e tracer, porque a recovery hoje é quem garante essas escritas), a `backup_queue:{transactions}`, o balance sync worker, o modo async e `settings.engine`; o Valkey sai do caminho do dinheiro por completo, incluindo a idempotência HTTP se a Epic 2.1 a levou para o Postgres. A deleção começa por um merge da `develop`.
**Scope:** `components/ledger/`, `pkg/`.
**Dependencies:** todos os tenants migrados (Phase 4).
**Done when:** `grep -r "EVALSHA\|engine:{transactions}\|backup_queue" components/` vazio; `make ci` verde.
**Status:** Pending

### Epic 5.2: Documentação e observabilidade

**Goal:** `docs/architecture/engine.md` reescrito para o motor único, `docs/api/SCOPING.md`, `llms-full.txt`, `AGENTS.md`, `CLAUDE.md`, métricas e dashboards atualizados.
**Scope:** `docs/`, raiz, `components/infra/grafana/`.
**Dependencies:** Epic 5.1
**Done when:** nenhum documento cita Lua, Valkey ou write-behind como caminho do dinheiro.
**Status:** Pending

### Epic 5.3: Delta de linhas

**Goal:** relatório do delta de linhas da branch inteira (prod, test, docs) contra `develop`; crescimento líquido precisa de motivo escrito.
**Scope:** este plano.
**Dependencies:** Epic 5.2
**Done when:** o delta está na `## Phase Overview` e o Fred recebe o número.
**Status:** Pending
