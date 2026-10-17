# Balance Engine on Postgres Truth Implementation Plan

> **For implementers:** Use ring-default:dispatching-workflows. Elaborate the
> current phase against the real code, execute a reviewed workflow, then elaborate
> the next phase. Alternatively, use ring-dev-team:running-dev-cycle.
> This document is the living source of truth for all five phases.

**Autorização:** Fred autorizou a bancada local da Fase 1 em 2026-10-16. Em 2026-10-17, autorizou antecipar a Fase 2 em paralelo e trazer o lifecycle completo da Fase 3 para o primeiro marco funcional. O kernel começa dentro do ledger, com um binário/imagem e composição por responsabilidades inspirada nos papéis do Streaming Hub. A Fase 1 continua aberta, com provas próprias e slots de execução reservados. Subagents `claude-opus-5-5[1m]`, worktree/branch únicas e commits/pushes continuam autorizados. Produção, RDS, rollout, migração de clientes e PR continuam fora da autorização. As demais definições e a revisão técnica permanecem gates das entregas que dependem delas, não veto à preparação independente.

**Goal:** tornar o Postgres do tenant a única verdade do dinheiro, com decisão, trilha e resultado no mesmo commit síncrono replicado antes do sucesso público. Esse requisito está aprovado; domínios de falha, evidência de hospedagem e postura degradada continuam na definição 2.

**Architecture:** um autorizador resolve identidades antes de decidir e grava cada lote em uma transação SQL. A bancada compara dono com fence e lotes concorrentes com row locks; chamada em processo, gRPC real e Redpanda usam o mesmo escritor. Postgres continua opção permanente sem infraestrutura nova obrigatória. Entregar sem major exige paridade, barreira e preservação dos contratos suportados, inclusive configuração. Incompatibilidade real bloqueia a remoção até evidência e decisão explícita, sem forçar versão.

**Tech Stack:** bancada em Go 1.27, linguagem do produto ainda na definição 9; PostgreSQL 17 local com comparação limitada em PG16, `pgx/v5` v5.11.0, `squirrel`, `golang-migrate`, Docker Compose e toxiproxy 2.12.0. Redpanda `v24.2.7` e `franz-go` v1.21.6 somente nos experimentos de broker. Registrar versões e imagens efetivas; PG16 local não equivale a RDS16.

**Execução:** Midaz mantém a worktree `/srv/worktrees/balance-engine` e a branch `feature/balance-engine`. Fred autorizou uma exceção restrita para corrigir `lib-observability` e `lib-commons`, cada uma com sua worktree e branch `feat/sql-connector-notices`. As worktrees são `/srv/worktrees/midaz-kernel-observability` e `/srv/worktrees/midaz-kernel-commons`. Commits e pushes estão autorizados; PR, merge para `develop`, release e tag não estão. Subagents `claude-opus-5-5[1m]`, revisão independente e contrarian. Uma lane escreve por tree; pesquisa e revisão read-only rodam em paralelo. A sessão coordenadora integra, verifica e faz commits assinados com `X-Lerian-Ref: 0x1`.

**Lanes atuais (2026-10-17):** Epic 1.1 Done; Epic 1.2 Doing; Tasks 1.2.1–1.2.3 Done. O código M4 (células fechadas, Make, relatório persistido e estatísticas do checker) entrou em `26ff3bbd0`, após dois reviews independentes e contrarian sem Critical/High. O primeiro par canônico do bloco A, owner/ownerless com 60 s/120 s, passou com o checker completo; o bloco A não está completo e suas 16 células restantes nunca foram lançadas. Fred definiu a referência como mais de 8k TPS somados entre contas de um ledger; mais de 1k numa conta é ótimo, não um mínimo. Pediu também um teste grande para buscar o limite acima de 8k. O A restante fica suspenso, pois mede contenção concentrada. D1/D2 implementaram transferências distribuídas entre 10k contas do mesmo ledger, quatro patamares de concorrência e limites explícitos. Os smokes e a verificação integral passaram. A primeira campanha canônica concluiu três células, teve a quarta interrompida por OOM global do host e não iniciou as outras quatro; o maior valor observado foi 5.104,35 aprovações/s, sem máximo nem taxa sustentada; a revisão independente confirmou os números e a classificação da interrupção. Distribuição uniforme é fixture sintética escolhida para o primeiro teste, não tráfego representativo aprovado. Falhas/reload e limitações das libs dos transportes têm preparação read-only; a Task 1.3.2 foi concluída com 20 rodadas canônicas de cada teste no commit `1d606ac28`, sem falhas; a Task 1.3.3 registrou um reload de 1M em 6,831 s contra a referência experimental de 5 s. Uma lane escreve e executa a stack por vez. A Task 1.2.4 permanece aberta. A nova autorização permite preparação e implementação da Fase 2 em paralelo, sem dispensar as definições necessárias à ativação. Produção, RDS e PR continuam fora da autorização. As duas worktrees de libs descritas acima são a única exceção autorizada.

**Fontes:** alfarrábio `https://alfarrabio.lerian.net/reports/fred/midaz-motor-de-saldo`; `docs/architecture/engine.md`; reunião "Direcionamento Midaz" de 2026-10-14. Direção aprovada: intenção/recusa visíveis, corte antecipado inclusive sem gateway, autorizador por lote independente do transporte, nenhuma infraestrutura nova obrigatória, Redpanda dedicado quando usado, Tracer como módulo, alias por id, telemetria enxuta, estudo de leitura e desenho/migração de particionamento com prova de compatibilidade. As definições pendentes não são decisões aprovadas.

## Parallel advance approved on 2026-10-17

**Goal:** finish Phase 1 while implementing the Ledger Kernel through Midaz's existing APIs. The first functional milestone includes create, hold, commit, cancel and revert, with authoritative reads and controls. This advances Phase 2 and the necessary parts of Phase 3; it does not close either phase automatically.

| Lane | State | Next unit | Exit check |
|---|---|---|---|
| Phase 1 measurement | Attribution at `374b8f021` passed offline checks and two instrumented stack smokes at `0c27027ca`; 1/32 sessions, 4,240/12,034 checked approvals | Discriminating attribution experiment after the reserved fault unit | Smokes are not capacity; tracer cost remains unmeasured; interrupted cell archived before resets |
| Phase 1 failure proofs | Tasks 1.3.2 and 1.3.3 Done; A and C each passed 20 canonical rounds per writer; one 1M reload took 6.831 s against the 5 s reference | Promotion B and negative control remain; Task 1.3.1 stays open | C passed in 801.270 s at 0299b72f7, unchanged source and CLEAN teardown; no promotion claim |
| Phase 2 kernel and early Phase 3 lifecycle | Libraries, managed ST/MT COMMIT and SQL identity model evidence accepted; no product kernel activation | Finish product identity and durable Tracer resolution before atomic lifecycle integration | Identity proof covers 10 model cases, not HTTP, MT routing, retention or Tracer |
| Independent review and evidence | C canonical20, corrected identity proof and 1M reload evidence accepted | Promotion B remains prepared, not run | Reload measured against 5 s, not under it; identity model 10/10 PASS; earlier M2b stays incomplete |

**Scheduling:** one writer, one runtime lock. Phase 1 attribution receives the first write slot. API contract analysis runs alongside it. Alternate ready proof and product increments; Phase 2 cannot occupy all slots until its lifecycle finishes. Compile/test commands wait while their source files change. The existing runtime runner serializes all stack use. A blocked capacity run does not block safe fault preparation or product work.

**Names:** Ledger Kernel is the financial component, implemented as `pgengine`. Outbox relay delivers committed obligations. Do not create separate Authorizer/Committer services, a selector framework or empty run roles. In-process composition is approved; remote transport and separately deployed kernel operation are not implied.

**Boundary:** a snapshot of onboarding block state cannot replace a live financial barrier. Fred resolved definition 1: account blocks, block exceptions and fee debt become authoritative in transaction Postgres, ordered with money. Onboarding/Mongo may project those controls; whole databases are not merged and Tracer-owned limits stay in Tracer. Preserve current identity, replay and public deadlines until their proposed changes are approved. Partial implementation never activates a mixed-authority scope or silently falls back to Lua. Partitioning/global guards/FKs remain part of the schema work.

**Integration review gates:** resolving a stored identity requires a real WAL touch and notice-free synchronous commit before replay/conflict is final. Resolve authoritative identity ownership before fees and Tracer, including simultaneous first attempts; a SELECT miss is not ownership. Errors after COMMIT is sent remain indeterminate and cannot release a reservation. Known settlement, metadata, events and enabled audit are committed obligations. Read-your-writes uses the primary and must preserve metadata visibility. The ordinary infra compose does not provide the required named synchronous standby; use an owned, isolated local stack and verify replication. Product pool notice capture must use a supported Lerian-library mechanism; raw parallel pools and bypassing tenant-manager are not substitutes. ST and MT each need their own proof, including relay discovery. Implementing the settled control authority and proving the approved schema/partition remain gates before a complete functional milestone; no race-window shortcut is accepted.

**Source freeze:** run one measured cell or fault command at a time from a pinned source and migration set. No repository writes while that command runs, since the bench reads migrations at runtime. Alternate these bounded slots with product writing. Do not add a campaign/binary packaging framework merely to avoid this explicit scheduling boundary. New product migrations must not silently change an existing comparison's schema or duplicate the bench outbox schema.

This section records the approved advance; the detailed phase entries below retain their status and evidence. Design/plan reviews are complete and their prerequisites appear above. Global removal of Lua and old writers remains Phase 5, after full coverage and an approved cutover. No PR or client migration is authorized.

**Library prerequisite:** ordinary ST/MT pools did not expose `OnNotice`, and the existing telemetry setup reopened a configured connector by DSN, losing its callback. The additive `sqlobs.SetupConnector` path now preserves the connector in offline tests, including a real pgx driver with a local protocol responder. Full library unit tests with race passed. The accepted library commit `1c4e0345579674e990f3cfdc0db5e81b042245be` is pushed; repo-config lint and new-diff test lint passed. Twenty wider test-lint findings match the immutable existing baseline and remain outside this change. The original library tests did not exercise PostgreSQL replication; the managed-pool proof below now covers that seam. The accepted `lib-commons` commit `9651fbbe38efbb799b2a96a0a7c1c4113561d414` is pushed on its isolated branch. `NewWithNotice` and MT `WithOnNotice` preserve primary notices through telemetry and reconnect, with per-connection DSN parsing. Config, New and NewFromPools retain their existing contracts; apidiff and external consumer probes confirm comparability and source compatibility for the checked uses. Final ST/MT tests passed once with race against real PostgreSQL 16, on unchanged source, loopback-only containers and verified cleanup. Reviews/contrarian found no Critical/High/Medium. The pin includes earlier observability trace-shape changes; no release is authorized. The managed-pool increment adopts both immutable Go-resolved versions and preserves successful HTTP access logs at Info. On 2026-10-17 the real ST/MT proof passed under race on the owned PG17 HA fixture: deadline and cancelled SyncRep returned unknown; sibling, reused and replacement connections committed clean; each marker and its outbox row used the same SQL transaction. A-before-B ordering kills both shared-slot and last-writer-slot mutants. Final source verification passed in 10.76 s with unchanged hashes, CLEAN teardown and retained-dump integrity; vet, format, bench lint and both access-log tests passed. The earlier full unit suite passed 116 packages, and the non-volume bench regression passed 63 tests and 86 subtests with one expected opt-in skip. MT uses one tenant without a schema and an empty-tenant fixture outbox, not product MT routing. The notice API compile guard exists only in bench-tagged builds. No kernel API activation or existing benchmark writer change is included. Standby-loss C at `synchronous_commit=on` now passed 20 canonical rounds per writer on clean commit `0299b72f7`. Lifecycle, SQL identity, durable Tracer resolution and product recovery remain open.

### SQL identity model evidence accepted on 2026-10-17

`TestIdentityRaceProof` runs ten cases in an owned schema on PostgreSQL 17.11. Main accepted the corrected 516-line test, SHA256 `4c521c7e5887f700c8474096e3d905629c0bcedd15c93271e08bdbf93fd5374e`. A controlled fixture expiry makes the earlier read stale; replay uses its own RETURNING result. A no-row replay assigns no xid and supplies no replication barrier. A slow owner's fenced finalize can return no row yet retain a lock on the successor's row; ROLLBACK releases that lock without a replication wait. The suite also observes NOWAIT, gate retention, purge and WAL from a replay touch through the managed ST pool.

The final race baseline passed all ten cases in 1.618 s package time. M1, removing the replay state predicate, failed at the empty-result assertion. M4, replacing the fenced owner's ROLLBACK with COMMIT, failed at the no-replication-wait assertion while the successor's finalize remained blocked. Both mutants compiled. Every stage preserved source hashes, the retained dump and public-table totals; the proof schema was absent on both nodes and teardown was CLEAN. Vet, formatting and bench lint passed. Evidence: `/tmp/balance-engine-phase1.A5lpaQ/identity-closeout-verify.KpYhGW/` and `identity-closeout-author.HsYxs4/`. Earlier full regression and other mutants remain evidence of the earlier source, not reruns of this version.

This accepts PostgreSQL model behavior, not the product identity protocol. The in-statement replay/expiry race, lock and purge EPQ windows, financial side-effect rollback, socket-only loss, product key scope, MT, retention and Tracer remain unobserved here. No throughput or latency SLO follows from this run. Production APIs still use Redis/Lua. No product code, dependency or migration changed; Product Console impact is None. Added test volume is 516 lines for ten real-database cases and bounded cleanup; the correction removed 30 lines and two redundant cases before acceptance. Phase 1 and the full API milestone remain open.

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | Escritores/transportes comparáveis, aprovações duráveis, cortes, proteção e falhas; PG16/17 e hipótese D medidos; revisão técnica antes de recomendar implementação | 1.1, 1.2, 1.3, 1.4, 1.5 | Detailed |
| 2 | Kernel experimental integrado às APIs e esquema particionado; primeiro marco exige lifecycle e controles antecipados da Fase 3, sem habilitar clientes | 2.1, 2.2, 2.3, 2.4, 2.5 | Epic-level |
| 3 | Paridade v1/v2 ST/MT, NOTED, dívida SQL, replay/leitura e intenção/recusa no Console; lifecycle necessário ao primeiro marco avança com a Fase 2 | 3.1, 3.2, 3.3, 3.4, 3.5 | Epic-level |
| 4 | Tenant reconciliado migra atrás de barreira que cobre todos os escritores; ensaio e runbook aprovados | 4.1, 4.2, 4.3 | Epic-level |
| 5 | Caminhos antigos removidos após cobertura, drenagem e migração; docs e métricas refletem o motor único | 5.1, 5.2, 5.3 | Epic-level |

---

## Hoje e destino

### Hoje

As informações de produção desta seção são observações de 6 a 8 de outubro de 2026, não uma verificação atual. Naquele recorte, Lerian Cloud rodava Midaz 4.2.0 e `develop` estava em 4.3.0-beta. Não há acesso de produção autorizado neste plano.

No build desta branch, Lua no Valkey decide saldo em um EVAL. Depois, `finalizeCreateEngineResult`, em `components/ledger/internal/services/command/create_transaction_engine.go:177-245`, tenta publish com confirmação do RabbitMQ antes de retornar no modo async; sem dispatch confirmado, tenta completion SQL síncrona. Uma falha de projeção após aplicação financeira pode ficar para recovery e ainda permitir 201. Assim, o 201 não comprova confirmação SQL.

`execution.lua` relê a chave dentro do EVAL; valor vivo presente vence o seed, inclusive uma chave viva restaurada e atrasada diante do SQL. `query/get_balances.go` reconstrói seed pela trilha, mecanismo já presente na v4.2.0. Sem chave viva, a reconstrução não recupera write-behind que ainda não chegou ao SQL. Dois misses concorrentes não bastam para bifurcar se a chave publicada pelo primeiro permanece. Restaurar chave antiga pode regredir o saldo mesmo com SQL atualizado; perder a chave com trilha SQL atrasada é outra falha.

O recorte histórico tinha Valkey compartilhado, `appendonly=no`, snapshot diário e réplica assíncrona. A correção 4.2.1 paginou a limpeza de recovery; não comprova eliminação das demais causas. Listas de fee debt e formatos históricos de chave exigem reconciliação própria no cutover, conforme hotfix #2587.

Na base de código fc9df991e, `bootstrap/balance_reconciler.runner.go` chama `ListBalanceTrailPairs` de `adapters/postgres/balance/balance_trail.postgresql.go` e `ClassifyBalanceTrailPair` de `services/query/reconcile_balances_classify.go`. Esses arquivos, sob `components/ledger/internal/`, comparam saldo SQL com a operação mais recente por versão. Não lê saldo Redis, não percorre toda a cadeia, não detecta todos os forks e não repara dinheiro. `EngineRecoveryCleanupRunner`, em `components/ledger/internal/bootstrap/engine_recovery_cleanup.runner.go`, tem cadência própria, independente do recovery; `CleanupEngineRecovery` limpa artefatos elegíveis após checar ACK, durabilidade, retenção e dependências. Essas leituras estáticas não provam paridade de pending/fee debt/replay nem substituem a barreira da Phase 4.

Há obrigações separadas da gravação SQL: metadata no MongoDB, projeção de fee debt no MongoDB, eventos e acerto de reservas do Tracer. A fila de reservas sem resposta em `transaction_reservation_unanswered_settle.go` admite 64 acertos em voo, descarta quando cheia e não sobrevive ao restart. Uma reserva pode ter sido aplicada sem que o ledger consiga commitar; nesse caso não existe outbox de sucesso para recuperá-la. O TTL atual é fallback de 5 minutos para direct e 30 dias para pending, não substituto de resolução durável.

### Hospedagem observada

| Item | Lerian Cloud, recorte de 6 a 8/10/2026 |
|---|---|
| Motor | RDS for PostgreSQL 16.14 |
| Instância | `db.t4g.small`, 2 vCPU burstable, 2 GiB, gp3 de 100 a 500 GB |
| Replicação | Multi-AZ de instância |
| Compartilhamento | `onboarding` e `transaction` na mesma instância; endpoint de leitura observado igual ao primário |

Fonte de configuração: `lerian-infrastructure/environments/production/rds/ledger-shared-prd-postgres`. RDS Multi-AZ de instância e standby síncrono nomeado de Postgres próprio não são equivalentes: o standby de storage do RDS não aparece em `pg_stat_replication`; a AWS administra degradação e failover. A documentação da AWS descreve commits continuando durante recomposição do standby e failover por DNS, tipicamente 60 a 120 s. Os 2 a 5 ms adicionais citados pela AWS não são medição deste plano. Fred informou que muitos BYOC usam RDS, mas não aprovou a janela de durabilidade degradada. Isso permanece na definição 2.

### Bancos e escritores atuais

`onboarding` guarda organizations, ledgers, accounts e bloqueios de conta. `transaction` guarda transactions, operations, balances, routes e groups. Uma fence em `transaction` não ordena um controle atualizado em `onboarding`; a definição 1 resolve essa fronteira.

O inventário inclui mais que transações: PATCH de `allow_sending`, `allow_receiving` e `settings` em `update_balance.go` grava SQL sem incrementar a versão monetária, conforme `balance.postgresql.go:1282-1321`. Criação de saldo e companion de overdraft, mudanças de limite, close/delete, controles, exceções, fee debt, workers de sync e recovery também escrevem estado relevante. Na Fase 3, cada escritor deverá participar da mesma ordenação autoritativa ou invalidar transacionalmente o estado carregado. `WHERE version = esperada` sozinho não detecta mudanças que não alteram essa versão.

### Destino proposto

- O Postgres guarda a verdade financeira. Identidade e fingerprint precedem a decisão; transações, operações, saldos, controles, dívida de tarifa, resultado e obrigações pós-commit integram o mesmo commit financeiro. A definição 1 fixa localização e ownership dos controles e da dívida; Mongo pode projetar consultas, nunca autorizar a dívida.
- Dono ou ownerless depende da prova e do protocolo. Ambos usam estado provisório privado por lote, descartado em rollback ou resultado incerto. A unidade atômica inteira cabe no mesmo banco e domínio de ordenação do owner/partição, inclusive grupos cross-ledger no mesmo banco; não há 2PC entre shards implícito.
- Sucesso exige replicação confirmada segundo a definição 2. COMMIT sem confirmação preserva `unknown`, nunca recusa financeira ou nova identidade. Replay conhecido prevalece sobre deadline após a barreira de durabilidade. `SHOW synchronous_commit=on` sozinho não prova réplica nem cobre degradação de RDS; failover do banco só exige takeover se o dono não sobreviver.
- Pedidos autenticados, autorizados e estruturalmente válidos têm intenção, recusa e motivo visíveis ao operador do cliente, inclusive recusa Tracer anterior ao dinheiro, sem lançamentos fictícios. Não guardar payload não autorizado ou PII por default. Evidência, slot de idempotência e resolução de reserva incerta são responsabilidades distintas. Epic 2.1 define persistência/consulta/protocolo público; Epic 3.5 implementa sem rota ou schema escolhido aqui.
- Outbox cobre metadata, projeção de dívida, eventos e acertos conhecidos do Tracer; em B também cobre respostas. Tentativas de reserva sem resultado SQL exigem resolução durável própria antes de retirar a fila em memória. Retenção de resultados, tentativas não resolvidas e entregas depende do replay da definição 7.

O corte de admissão vale na entrada, antes de controles externos ou fila, e é rechecado após esperas. Antes de enviar COMMIT, prazo interno cruzado exige rollback do lote ainda não commitado, sem conservar decisões dependentes. Isso não revoga COMMIT já enviado: timeout pode coexistir com commit posterior. Identidades anteriores são resolvidas/replayed sem iniciar dinheiro de novo; incerteza não vira expiração. Orçamento do caller e corte interno anterior ficam explícitos no bench, inclusive sem gateway; defaults, margem, headers e resposta pública continuam na definição 11.

O autorizador usa `balance_id` e identidade explícita de conta nos controles. Alias resolve na borda; reuso de alias não transfere estorno ou pendência. `request_id` identifica pedido; `transaction_id` identifica movimento, ausente numa recusa.

Redpanda, quando usado, é dedicado ao consumo interno do ledger e independente do Streaming Hub. RabbitMQ pode continuar servindo entregas; sua retirada como infraestrutura obrigatória não foi aprovada. 202 é opção futura opt-in, nunca default, fora desta branch. Tracer recebe o payload Midaz e vira módulo; aposentar endpoint avulso é lane do time condicionada a consumidores e compatibilidade. Colocar cotas em fees não está decidido. A integração do Tracer é definição 10; executável do autorizador é definição 12.

Telemetria segue `docs/standards/telemetry.md`, com pedido, lote, commit e detalhe no erro. A redução de cerca de 70% citada pelo time é estimativa não medida. Estudo de read model é entrega da Epic 2.5, sem aprovação de banco novo. Particionamento integra desenho e migração das Epics 2.1/2.2; nenhuma chave ou granularidade está escolhida. Prova de unicidade global, FKs e compatibilidade precede a aplicação; impossibilidade bloqueia para decisão, não elimina o requisito.

### Topologias: onde o Redpanda entra

| Topologia | Caminho do resultado | Compra | Custa | Estado |
|---|---|---|---|---|
| A. Autorizador direto | Chamada em processo ou gRPC; o mesmo escritor confirma SQL antes da resposta | Menos componentes no caminho síncrono | gRPC acrescenta salto; owner exige roteamento/takeover, ownerless enfrenta contenção SQL | Recomendada com E; fronteira de processo aberta na definição 12 |
| B. Redpanda antes do autorizador | API faz validação não financeira e corte de admissão, publica intenção; worker decide; SQL grava resultado, offset e resposta no outbox | Backlog durável e distribuição por partição | Broker, consumidor e canal de resposta; ACK não é aprovação | Alternativa experimental, definição 8 |
| C. Como B, aceite assíncrono | 202 no aceite; cliente consulta depois | Dispensa espera síncrona | Muda contrato e adia recusas | Futuro opt-in, nunca default; fora desta branch |
| D. Broker independente e reconciliação | Publish assíncrono independente do SQL; variante exige recibo do broker antes de tentar SQL | Registro separado da intenção para comparação | Dois commits independentes permitem commit-only, receipt-only e unknown; recibo não prova dinheiro | Hipótese do Fred preservada para experimento, sem decisão tomada |
| E. Outbox atrás do commit | Mesmo commit grava obrigações; relay publica e confirma antes de marcá-las | Retoma entregas após falha sem dual write | Relay, lag e deduplicação at-least-once | Recomendada; serve A e B |

A com E é recomendação, não escolha aprovada. Postgres sozinho permanece opção de operação, não etapa provisória até comprar broker. B compara o mesmo escritor, payload e batching. D compara recibos com SQL autoritativo; reconciliação aponta divergências, nunca inventa aprovação, recupera dinheiro automaticamente ou reexecuta a partir do broker. Publish assíncrono independente não prova zero perda. As Tasks 1.4.3/1.4.4 informam a definição 8; um grupo por tenant no bench não define ST/MT do produto.

### O que sai do produto

A Phase 5 retira Lua, Valkey financeiro/idempotência migrada, write-behind, recovery e balance sync após substituição e drain. NOTED recebe substituto antes de apagar `write_transaction`; settle volátil só sai após resolução durável de reservas sem commit. Inventariar consumidores de `RABBITMQ_TRANSACTION_ASYNC` e demais configs/env antes de remover: se quebrar uso suportado, parar e pedir aprovação com evidência. Preservar o objetivo sem major: demonstrar compatibilidade ou bloquear a remoção para decisão explícita, sem inventar fallback. Remover um mecanismo interno não autoriza aposentar RabbitMQ inteiro.

### Custo e alvos da bancada

Uma transação simples grava 1 `transaction`, 2 `operation` e atualiza 2 `balance`. A base inclui migrações até 46: scheme em 44, links opcionais de rota em 45 e índice de operação/versão de saldo em 46. Contabilizar seus índices mais intenções, controles e outbox, sem reutilizar a estimativa incompleta de 18 a 23 entradas. `fillfactor` é hipótese experimental.

**Referência corrigida por Fred (2026-10-17):** "total em um ledger. um conjunto de contas passando de 8k tps esta 'bom'. uma conta unica passando de 1k tps esta OTIMO". Este plano atribuiu os 8k por erro a uma única conta quente. A unidade correta é o total de um ledger; 1k numa conta é referência qualitativa, não mínimo ou SLO. Fred pediu depois um teste grande para descobrir a capacidade acima de 8k. A execução deve distinguir carga enviada, aprovações distintas com commit durável e taxa mantida sem crescimento do backlog, com limites de memória, duração e carga definidos antes do run.

A conta quente do bench é fixture de contenção: cada pedido das cargas medidas debita ou credita o mesmo saldo limitado. Essas medições não são a carga agregada. O fixture distribuído usa duas contas ordinárias distintas por transferência, sorteadas uniformemente entre 10k contas do mesmo ledger. Essa escolha experimental não descreve a distribuição dos clientes; não há conta obrigatória em todo pedido nem requisito de 8k por conta. Taxas de aprovação e p99 de resposta já registrados, com contagens, métodos e commits, continuam válidos como medições dessa contenção. Toda comparação anterior "abaixo de 8k" é inválida como julgamento de aceitação, não um número falso. Revisão técnica de Felipe/Clara e confronto com os spikes disponíveis precedem recomendação de implementação; nenhum review nem resultado do time está concluído aqui.

| Medida | Alvo ou interpretação |
|---|---|
| TPS agregado de um ledger | Mais de 8.000 aprovações distintas duráveis/s é a referência boa de Fred; buscar o limite com carga crescente, backlog observado e drain medido. Não é requisito por conta nem teto do experimento |
| p99 do resultado, RTT de replicação medido de 1 ms | ≤ 15 ms, hipótese a validar; separar núcleo local, gRPC e Redpanda |
| Perda ou dupla aplicação de identidades aprovadas após falha | 0 nas 20 rodadas de cada cenário aplicável |
| Takeover com 1M saldos e 1M controles | Medir contra referência experimental de 5 s; orçamento público depende da definição 3 |
| Soak de 30 min | Sustentar a taxa selecionada no experimento agregado de um ledger; referência superior a 8k, p99 final ≤ 1,5× p99 do quinto minuto. A carga será detalhada após a busca de capacidade |
| Corte de admissão | Zero trabalho financeiro novo vencido na entrada/pós-espera; corte pre-COMMIT aborta lote inteiro. COMMIT já enviado e replay tardios são registrados, nunca reclassificados por prazo |
| Transporte | Broker ACK, confirmação SQL e resposta medidos separadamente; sem perda de respostas pendentes após recuperação |

Recusas, expirações, duplicatas e `unknown` não contam para a meta de 8k. Célula HA, RDS ou transporte não executada fica "não medido". RF1 no Redpanda mede recuperação de worker, não HA do broker.

## Definições pendentes do Fred

As definições abaixo registram os gates de produto. Fred resolveu a definição 1 para a autoridade dos controles financeiros e a definição 12 para a execução inicial dentro do ledger. A separação futura do kernel permanece aberta. A antecipação das fases não aprova as demais definições. A Fase 1 compara hipóteses sem escolher contratos públicos; seus resultados valem para a configuração medida.

| # | Decisão | Recomendação e custo | Bloqueia |
|---|---|---|---|
| 1 | Resolvida: autoridade financeira no transaction Postgres | Fred escolheu bloqueios de conta, exceções e dívida de tarifa no banco do commit, ordenados com o dinheiro. Onboarding/Mongo podem projetar consultas. Não funde bancos inteiros nem move limites do Tracer para ledger/fees; o schema `account_control` do bench não vira contrato de produto | Ownership resolvido para Epics 2.1, 2.2, 2.5, 3.2; implementação e provas pendentes |
| 2 | Quais falhas o zero perda cobre e o que fazer com replicação degradada | Separar contrato de hospedagem de checagens da aplicação; declarar domínios de falha e bloquear confirmações que não atendam ao contrato. A aceitação da janela degradada do RDS Multi-AZ de instância está aberta; BYOC usar RDS não a aprova | Epics 2.1, 2.2, 2.3 e Fase 4 |
| 3 | Indisponibilidade no takeover e espera do cliente durante failover do banco | Referência de 5 s para takeover, a medir. Tratar espera do RDS separadamente, sem somar takeover quando o owner sobrevive; definir timeout/unknown e reconexão sem prometer resposta antes do banco voltar | Epics 2.1, 2.3 e Fase 4 |
| 4 | Retry sem chave do cliente em create, commit, cancel e revert | Definir identidade explícita do pedido. Dois pagamentos legítimos com o mesmo corpo não são automaticamente retry; hash verifica equivalência sob uma identidade, não prova identidade sozinho. Derivação de transação mais ação para commit/cancel também exige aprovação. Sem política pública padrão escolhida | Epics 2.1, 3.1 |
| 5 | Interrupção no cutover | Pausa por tenant em janela agendada, com indisponibilidade explícita. Compra barreira verificável; custa interrupção | Fase 4 |
| 6 | Prioridade e garantia da ponte da linha 4.x | Lane separada para Valkey dedicado, retenção limitada e detector de fork; durabilidade de idempotência exige decisão própria. Nenhuma operação dessa lane é autorizada aqui | Fora desta branch |
| 7 | Replay durante execução e depois da retenção | Resultado resolvido volta congelado; original não resolvido preserva `unknown`. Definir espera, conflito, janela de replay e comportamento após purga. Retenção coordena resultados, tentativas desconhecidas, outbox entregue e consumidores idempotentes; sem TTL arbitrário ou deduplicação eterna de identidades apagadas | Epics 2.1, 2.2, 2.3, 2.5 e Phases 3–5 |
| 8 | Redpanda antes da decisão, independente para reconciliação, ou só após commit | Recomendar A com E; comparar B e as variantes D, sem transformar recibo em sucesso nem reconciliação em reexecução. Preservar opção permanente só Postgres. Desempenho sozinho não decide | Epics 2.1, 2.3 e Phase 4 |
| 9 | Go ou Rust no autorizador | Go por stack, equipe e libs Lerian. Medir CPU, GC e commit antes de recomendar outra linguagem; confronto com spikes e revisão técnica do time são gates | Epic 2.3 |
| 10 | Integração do módulo Tracer, ownership de cotas e reservas incertas | Preservar payload Midaz; definir limites e resolução durável de tentativas sem commit, separada da visibilidade de recusas e do outbox pós-commit. Cotas em fees não estão aprovadas. Retirada do endpoint avulso pertence ao time e exige prova de consumidores/compatibilidade | Epics 2.1, 2.3, 3.2, 3.5 e Phases 4, 5 |
| 11 | Deadline público, inclusive sem gateway | Admissão na entrada, recheck pós-espera e rollback antes do COMMIT se corte cruzado; após envio, preservar unknown ou resultado conhecido. Definir orçamento, margem anterior, default local, headers/propagação e resposta. Valores do bench não viram contrato | Epics 2.1, 2.3 e Phases 3, 4 |
| 12 | Execução inicial definida; separação futura aberta | Fred escolheu kernel dentro do ledger, com um binário/imagem e composição inspirada nos papéis do Streaming Hub. Comparações gRPC/Redpanda continuam no bench; não aprovam RPC, roteamento ou operação remota do produto. Independente da integração do Tracer | Fronteira inicial resolvida para Epics 2.1, 2.3, 2.4; separação futura exige definição |

## Fora do escopo desta branch

- Ponte operacional 4.x, produção e mudança de hospedagem. RDS temporário requer autorização própria. Nenhum stack obrigatório só AWS e nenhum TigerBeetle.
- Implementação/deploy de novo read store sem decisão posterior. O estudo de consultas, projeção por outbox, consistência/lag, retenção e licença está dentro da Epic 2.5; MongoDB/ClickHouse não estão escolhidos.
- 202 opt-in e seu fluxo assíncrono futuro. Visibilidade de intenções/recusas não é esse fluxo e pertence a esta entrega. Endpoints avulsos do Tracer e `transaction_type` seguem na lane do time.
- Manter caminhos internos obsoletos depois da Phase 5. APIs, configuração/env, eventos, dados e upgrade suportados continuam sob o gate de compatibilidade; nenhuma remoção silenciosa.
- Issues públicas sobre incidentes. Gaps fora do pedido ficam com a sessão coordenadora.

---

## Phase 1: Experimentos medidos

Objetivo: números e provas de correção da bancada, não código de produto. Novos arquivos ficam em `tests/bench/engine/`, tag `bench`, fora dos lanes de CI. Make e `.gitignore` só recebem integração desse pacote.

### Contrato comum do experimento

Este contrato vale para owner, ownerless, gRPC e Redpanda; não define o contrato público futuro.

1. Cada pedido tem `request_id` estável, `transaction_id` separado quando move dinheiro, postings congelados e fingerprint. Retry conserva identidade, payload e corte financeiro original; nova tentativa de resposta tem seu próprio orçamento, sem renovar admissão. Bench de um tenant: controles por `(organization_id, ledger_id, account_id)`, dinheiro por `balance_id`.
2. Na entrada, antes de controles externos ou enfileiramento financeiro, observar o corte de admissão. Vencidos só seguem para resolução/finalização sem dinheiro. Dentro da MESMA transação SQL do lote, reclamar identidades em ordem canônica com `INSERT ... ON CONFLICT DO NOTHING` e ler/travar conflitos em `READ COMMITTED`, antes de decidir. Não commitar claims separados. A primeira ocorrência fixa payload; duplicatas equivalentes compartilham resultado, divergentes recebem conflito.
3. Resolver existentes antes dos novos. Identidade em voo ou sem resolução confirmada mantém `unknown`; resultado conhecido é replay, mesmo vencido, após a barreira abaixo. Só identidade nova comprovada pode finalizar `expired`. Rechecar admissão depois da fila e das esperas por claims/locks, antes de decidir. Nenhum timeout de lookup converte desconhecido em recusa.
4. Decidir na ordem original sobre estado provisório privado. Persistir resultado, dinheiro e outbox; publicar memória só após confirmação. Antes de enviar COMMIT, verificar os cortes de TODAS as decisões financeiras novas do lote. Se algum cruzou, ROLLBACK do lote inteiro e descarte de decisões dependentes; a nova passagem resolve identidades, finaliza vencidos sem dinheiro e recomputa apenas pedidos ainda admissíveis. Finalização sem dinheiro e touch de replay não são nova autorização. Conflito tardio também invalida o lote inteiro, nunca só um posting.
5. Bench usual: caller dispõe de 30 s, admissão corta aos 28 s. Célula de backlog: caller 2 s, corte a 1,5 s. Carregar ambos os instantes desde `scheduledAt`; sem gateway, a entrada configura explicitamente esses mesmos orçamentos locais. Contexto SQL dura no máximo 10 s e respeita o orçamento restante da tentativa. A margem é experimental, não garantia de zero commit tardio. Depois de enviar COMMIT, timeout/notice incerto preserva `unknown`; não presumir rollback nem desfazer dinheiro. Defaults, headers e margem públicos ficam na definição 11.

Após identidade e prazo, validar block/flags, saldo e limite. Cada posting aprovado das contas limitadas do bench respeita seu piso de saldo e limite compartilhado; débito bloqueado nunca é aprovado. Verificar isso em cada decisão, inclusive esgotamento concorrente e multikey, não só conservação final. São invariantes dessas fixtures, não CHECK indiscriminado em classes externas/ilimitadas do produto. Recusa não muda dinheiro; overdraft e lifecycle completos exigem Phase 3.

**Durabilidade e replay:** no pgx v5.11.0, `pgconn.go:768-790` envia `CancelRequest` em `asyncClose`; `receiveMessage`, linhas 1613-1620, chama esse caminho após erro de leitura. Cancelar durante a espera de replicação síncrona no Postgres pode deixar o commit local sem replicação confirmada. Linha visível no primário, sem confirmação da réplica, não prova essa durabilidade.

O bench mantém `unknown` em timeout, perda de resposta ou aviso de cancelamento da espera síncrona. O tratamento observa também notices do COMMIT; retorno sem erro sozinho não basta se a espera síncrona foi cancelada. Antes de responder replay ou conflito de uma identidade existente, a mesma transação finalizadora incrementa `engine_request.durability_touch` e faz um COMMIT síncrono com escrita real de WAL, no primário autoritativo. Esse WAL posterior cobre o resultado anterior na ordem de replicação. `SELECT` seguido de COMMIT read-only não é barreira. Sem confirmação da nova barreira, permanece `unknown`. A prova vale para o primário único e a configuração de réplica da célula, não para hospedagens ou falhas não exercitadas.

**Relógios:** registrar `decisionStartedAt`, `dbAckAt` e `repliedAt`; Redpanda acrescenta `brokerAckAt`. Recebimento do ACK no host não é timestamp real de commit. SQL `now()` é início da transação; `clock_timestamp()` é relógio corrente do banco, não prova de instante de commit. O bench não inventa `committedAt`; mede confirmação e resposta. Fixtures temporais determinísticas usam relógio injetado; medições reais usam relógio do processo, sem gerar carga sintética de CPU.

**Cobertura:** o throughput usa uma chave por conta por simplicidade. Um fixture pequeno usa dois `balance_id` da mesma conta, contrapartes repetidas e controle compartilhado, provando que saldos não se confundem e limites não são duplicados. Isso não substitui a paridade funcional da Fase 3.

### Epic 1.1: Ambiente Postgres HA reproduzível para o bench

**Goal:** primário PG17, standby síncrono, latência controlada e esquema real com tabelas experimentais.
**Scope:** `tests/bench/engine/`.
**Dependencies:** autorização de execução da Fase 1, concedida em 2026-10-16.
**Done when:** bootstrap reproduzível, standby em `sync` e esquema validado.
**Status:** Done

#### Task 1.1.1: Compose com primário, standby e toxiproxy

- [x] Done

**Evidence (2026-10-16):** PG17.11 (`postgres:17`, digest `2d2b8998…`) e toxiproxy2.12.0 (`9378ed52…`); fresh start limitado e restart com volumes preservados passaram. Primário `standby1 | streaming | sync`, standby em recovery/streaming, mesmo system identifier, 12 settings herdadas e quatro binds loopback verificados. Três revisores e contrarian sem Critical/High/Medium. Prova de ambiente, não de durabilidade financeira/failover/TPS; bootstrap ainda pendente.

**Context:** `components/infra/docker-compose.yml:65-155` usa réplica assíncrona. O bench precisa de standby nomeado e falhas controladas; toxiproxy já é dependência do repo.

**Implementation vision:** três serviços com nomes fixos: `bench-pg-primary`, `bench-pg-standby` e `bench-toxiproxy`, projeto `midaz-bench-engine`. Postgres `17`, toxiproxy `ghcr.io/shopify/toxiproxy:2.12.0`. Portas do host em `127.0.0.1`: primário 15432, standby 15435, proxy de cliente 15434 e API toxiproxy 18474, com overrides próprios do bench. O proxy de replicação 5433 fica apenas na rede do compose. Credenciais descartáveis `bench`/`bench`, banco `bench_transaction`; habilitar conexão de replicação nessa rede com `host replication all 0.0.0.0/0 scram-sha-256`. Não alterar serviços de outras sessões.

Primário com `wal_level=replica`, `max_wal_senders=5`, `max_connections=300`, `shared_buffers=4GB`, `max_wal_size=8GB`, `checkpoint_timeout=15min`, `log_checkpoints=on`, `track_io_timing=on` e `track_wal_io_timing=on`. `init-primary.sql` persiste configurações via `ALTER SYSTEM`, incluindo `synchronous_commit=on` e `synchronous_standby_names='FIRST 1 (standby1)'`, sem reload no servidor temporário do entrypoint. O start definitivo as aplica; o base backup as transmite ao standby. Cada serviço define `cluster_name` próprio e a inicialização grava marcador do banco descartável para o guard do bootstrap.

`pg_basebackup -R --no-slot --checkpoint=fast` atravessa `pg-repl` com `application_name=standby1`. Readiness exige primário com uma réplica `streaming | sync` e standby em recovery com WAL receiver `streaming`; só o WAL streamer do base backup não basta. O standby herda a espera síncrona: após promoção, commits novos continuam bloqueados até reinstalar uma réplica válida. O teste prova sobrevivência dos commits anteriores nessa janela, nunca novas aprovações duráveis. `pg-client` separa falha de resposta de falha de replicação. Sem restart automático do primário morto. Volumes exclusivos do bench; nenhuma limpeza atinge recursos externos.

**Files:**
- Create: `tests/bench/engine/docker-compose.yml`
- Create: `tests/bench/engine/init-primary.sql`
- Create: `tests/bench/engine/toxiproxy.json`
- Create: `tests/bench/engine/README.md`

**Verification:** `docker compose -f tests/bench/engine/docker-compose.yml up -d --wait --wait-timeout 120`; consultar `pg_stat_replication` no DSN local e exigir `standby1 | sync`. A verificação de COMMIT bloqueado com standby ausente fica no teste limitado da Task 1.3.1, não em um INSERT manual sem timeout.

**Done when:** após o pull inicial das imagens, ambiente novo fica pronto dentro do limite do comando, com binds loopback e réplica verificada nos dois lados.

#### Task 1.1.2: Bootstrap do esquema e fixtures reais

- [x] Done

**Context:** `components/ledger/migrations/transaction/` chega a 000046 neste HEAD. 44 acrescenta scheme; 45, optional em operation_transaction_route; 46, índice parcial não único de operation por balance_id/balance_version_after. A 46 usa `CREATE INDEX CONCURRENTLY`: executar cada arquivo via `golang-migrate`, sem transação externa nem concatenar outros DDL no mesmo Exec, seguindo `tests/integration/ledger_migrations_integration_test.go:110-113`. `operation.snapshot`, em `pkg/mmodel/operation.go`, contém `overdraftUsedBefore` e `overdraftUsedAfter`, não um create de 1 KB.

**Implementation vision:** todos os Go novos usam tag `bench` e cabeçalho de licença. `requireDB(t)` pula testes DB apenas quando toda configuração está ausente; configuração parcial/inválida falha. Testes puros continuam executáveis sem banco. Variáveis `BENCH_PG_PRIMARY_DSN`, `BENCH_PG_CLIENT_PROXY_DSN`, `BENCH_PG_STANDBY_DSN` e `BENCH_TOXIPROXY_URL` apontam ao compose descartável. Antes de reset, validar DSNs loopback, banco/usuário `bench_transaction`/`bench`, marcador `midaz-bench-engine:disposable`, `cluster_name`, recovery e mesmo `system_identifier` nas três conexões. Exigir réplica `standby1 | streaming | sync` no primário e WAL receiver streaming no standby, com limite de 120 s. Só então recriar `public`, aplicar migrations via `lib-commons` Migrator, SQL da outbox da lib e DDL bench-local, depois COPY. Sem terminar sessões alheias nem reduzir durabilidade; perda de réplica no setup falha o run, limitado também pelo timeout do processo.

**Unidades de execução:** 1.1.2a concluída em 2026-10-16: duas reconstruções do schema até migration 46, outbox da lib e três tabelas experimentais. Guard recusa marcador errado, portas primary/standby trocadas e `synchronous_commit=local`, preservando probe. Seis testes e 18 subtests passaram contra a stack; supervisor repetiu cinco rodadas em ordem variável. Quatro revisores e contrarian sem Critical/High; testes redundantes removidos. 1.1.2b concluída: 10k duas vezes com hashes iguais; 1M uma vez por run, 1.000.004 saldos e 1.000.003 controles. Seed inclui guard, COPY, commit síncrono e VACUUM: 316,29–352,44 ms em 10k e 21,43–23,90 s em 1M nos runs registrados. Supervisor repetiu o teste completo; suite tem sete testes e 18 subtests. Em 1M, contagens/somas/colunas fixas cobrem todas as linhas; IDs/aliases têm amostra nomeada. Quatro revisores e contrarian sem Critical/High; assert redundante removido. 1.1.2c concluída: fixtures 4/5/8 obrigações; caso feeLeg grava cinco eventos e três obrigações privadas via libs na mesma SQL, zero publish externo; rollback não deixa linhas. Payload `transaction.posted`: 2311/2377/3375 B nas três fixtures corrigidas, sem padding. `payload::text` e `pg_column_size` medidos separadamente; não são bytes Kafka. Suite integrada final: nove testes e 21 subtests passaram, sem skips; supervisor repetiu com ordem variável. Quatro revisores e contrarian sem Critical/High; corrigidos description/fração temporal e removidos testes redundantes. Não exercitados: writers financeiros, fee engine real, rotas, MT, trace carrier, relay, failover ou TPS.

Semear N saldos ordinários, mais saldo quente e saldo de tarifa identificados separadamente; registrar o total exato esperado, sem chamar N de total quando há extras. Valores iniciais devem sustentar aquecimento, medição e soak sem esgotamento acidental. Controles têm saldo de limite suficiente, exceto na célula de esgotamento. A fixture multikey é separada e tem IDs estáveis. Fence inicial sem dono e lease já expirado pelo relógio do banco. `fillfactor=70` é hipótese anotada.

DDL compartilhado do bench, não migração de produto:

```sql
CREATE TABLE engine_writer_fence (
  tenant_id text PRIMARY KEY,
  owner_id text NOT NULL,
  epoch bigint NOT NULL,
  lease_until timestamptz NOT NULL
);
CREATE TABLE engine_request (
  tenant_id text NOT NULL,
  request_id uuid NOT NULL,
  payload_fingerprint bytea NOT NULL,
  transaction_id uuid,
  outcome text,
  reason text NOT NULL DEFAULT '',
  admission_cutoff timestamptz NOT NULL,
  decided_at timestamptz,
  result jsonb,
  durability_touch bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, request_id)
);
CREATE TABLE account_control (
  organization_id uuid NOT NULL,
  ledger_id uuid NOT NULL,
  account_id uuid NOT NULL,
  blocked boolean NOT NULL DEFAULT false,
  limit_remaining numeric NOT NULL,
  version bigint NOT NULL,
  PRIMARY KEY (organization_id, ledger_id, account_id)
);
```

Outbox usa `outbox_events` de `lib-commons/v7/commons/outbox/postgres`, trilha schema em `public`, não uma implementação própria. Após migrations do ledger, aplicar o SQL fornecido pelo módulo pinado em `go.mod`, sem copiar o DDL nem compartilhar o histórico `schema_migrations`. Preservar PK e três índices secundários. Escritores usam `CreateManyWithTx` no mesmo `*sql.Tx`; `pgx/stdlib` preserva `OnNotice`, com conexão dedicada por writer. Medir `SET LOCAL`, índices e `RETURNING` da lib. O batch máximo de 256 pedidos das fixtures fica abaixo do limite de 65535 parâmetros, verificado pela quantidade real de obrigações.

`outcome` nulo só existe como claim dentro da transação; todo commit de lote o finaliza. Rejeição/expiração não têm `transaction_id` financeiro. `decided_at` não é commit timestamp. Metadata e Tracer usam `aggregate_id=request_id`, tipo privado do bench, payload representativo e ID estável. Eventos passam por `lib-streaming.EmitBatch` com a transação da outbox: a lib gera o row ID e deriva o aggregate da partition key; o bench fornece `EventID` estável para correlação/dedup. Não sobrescrever a partition key para satisfazer o teste. Um fake do transporte externo, restrito à bancada, comprova zero publish nessa célula. Fee debt entra somente na fixture que a produzir. Extrair quantidade e formato das obrigações do caminho real de completion e mantê-los iguais nos dois escritores. Registrar bytes e quantidade no resultado. Sem chamar esses serviços no microbenchmark: medir gravação do outbox e declarar entrega externa fora dessa célula. A resposta do transporte B acrescenta sua obrigação própria, custo explícito desse transporte.

Usar a representação efetivamente persistida: create direto grava `transaction.body=NULL`; PENDING grava o corpo normalizado, conforme `transaction_completion_service.go:349-351`. A fixture `create_v2_body.json` representa a entrada, não preenchimento artificial da coluna SQL. Usar snapshot real por operação e medir separadamente os bytes novos de intenção, resultado e outbox. Não reduzir o outbox a uma linha vazia. Rodar bootstrap duas vezes produz o mesmo estado.

**Files:**
- Create: `tests/bench/engine/env.go`, `tests/bench/engine/env_test.go`
- Create: `tests/bench/engine/bootstrap.go`, `tests/bench/engine/schema_bench.sql`
- Create: `tests/bench/engine/seed.go`, `tests/bench/engine/seed_test.go`
- Create: `tests/bench/engine/obligations.go`, `tests/bench/engine/obligations_test.go`
- Create: `tests/bench/engine/testdata/create_v2_body.json`; snapshot usa `mmodel.OperationSnapshot`, sem fixture duplicada
- Test: `tests/bench/engine/bootstrap_test.go`

**Verification:** suite integrada executada: `go test -count=1 -shuffle=on -tags bench ./tests/bench/engine/ -timeout 30m -v`; obrigações: `-run 'TestObligationCatalog|TestOutboxOnlyEmitBatch'`; schema/guard: `-run 'TestBenchEnv|TestBootstrapSchema|TestBootstrapRejects'`. Seed isolado: `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBootstrap$' -timeout 30m -v`, com DSNs do compose. Conferir migração 46 e seu índice parcial válido, três tabelas bench, `outbox_events` da lib com enum e quatro índices, totais de saldos/controles e duas execuções internas do bootstrap.

**Done when:** bootstrap repetível; tempos de 10k e 1M registrados, com limites por execução de 60 s e 10 min para a semeadura.

### Epic 1.2: Harness de carga com os dois escritores

**Goal:** comparação local do núcleo com a mesma carga, regras de lote, SQL, outbox e replicação.
**Scope:** `tests/bench/engine/`, `mk/tests.mk`, `.gitignore`.
**Dependencies:** Epic 1.1.
**Done when:** correctness gates passam e os blocos de medição produzem resultados reproduzíveis.
**Status:** Doing

#### Task 1.2.1: Gerador e métricas

- [x] Done

**Evidência (2026-10-16):** quatro testes do gerador passaram com e sem `-race`; supervisor repetiu com `-race -shuffle=on`. Suite sem DSNs: nove testes passaram e quatro testes DB tiveram skip explícito. Supervisor executou também a suite integrada com DSNs e ordem variável: 13 testes e 26 subtests passaram, sem skips. Vet e lint pinado v2.14.0 passaram. A regressão de cancelamento falhou antes da correção: todos os 100.000 índices já vencidos para envio saíam após cancelamento no writer. Agora o envio para dentro do backlog; o teste confere os instantes realmente recebidos, sem renovar o corte. Revisores independentes e contrarian não deixaram Critical/High/Medium.

**Medição e limites:** run não-race de 40.000 pedidos ao nulo, alvo 20k/s: 19.997 envios/s; dispatch lag p50 0,533 ms, p99 1,066 ms e máximo 2,655 ms. Aprovações simuladas, sem ACK SQL; não mede capacidade financeira. `-race` não sustenta 20k/s e não serve como medição de capacidade. As cinco séries usam 40 B por pedido reservado, 576 MB em 14,4 milhões, sem contar o restante do processo; nesse run, a alocação inicial entrava no lag e os runs terminavam por quantidade ou abort do contexto. A M1 da Task 1.2.4 moveu a alocação para antes do início e deu ao gerador fechado uma janela de admissão.

**Context:** conta limitada quente e contraparte aleatória formam o fixture de contenção: medem núcleo, não API nem capacidade. A referência de 8k é agregada em um ledger, não neste fixture concentrado. A busca de capacidade usa uma carga distribuída separada.

**Implementation vision:** `Writer` tem `Submit`/`Close`; pedidos carregam identidades, fingerprint, postings, `scheduledAt`, orçamento do caller e corte de admissão. Contadores separam aprovação durável distinta, recusa/motivo, expiração, replay, conflito e `unknown`.

Malha fechada usa 256 produtores. Aberta agenda `start + i/rate`, 8.000/s, e mede desde `scheduledAt`, incluindo atraso do gerador. Registrar envios, decisões, backlog/drain, `dbAckAt-scheduledAt` e `repliedAt-scheduledAt`. Cargas outflow/inflow/mixed, inteiros de 1 a 100; feeLeg divide crédito numa terceira perna. Limite desce só no débito. Fixtures de esgotamento concorrente, block/flags e multikey comprovam pisos por posting e controle compartilhado. Fast paths não contam na meta.

Nos testes dos escritores, reconstruir cada decisão aprovada na ordem/versão persistida e conferir piso, flags e limite, além da soma final. Submeter débitos concorrentes que esgotam saldo ou limite e verificar aprovações e recusas efetivas. Fila interrompida sem prova mantém `unknown`. Gerador nulo mede apenas a capacidade do gerador.

**Files:**
- Create: `tests/bench/engine/writer.go`, `tests/bench/engine/workload.go`, `tests/bench/engine/metrics.go`
- Test: `tests/bench/engine/workload_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestWorkloadKeysUnique|TestOpenLoopSchedule|TestPercentiles|TestWorkloadNullWriter' -timeout 5m -v`. Calendário/percentis determinísticos; medição limitada registra 20k envios/s ao nulo e atraso alcançado. As Tasks 1.2.2/1.2.3 verificam proteção nos escritores reais.

**Done when:** contadores distinguem resultados e pedidos conservam identidade, postings e cortes; os testes dos escritores usam essa identidade para conferir ordem/versão e valores persistidos nas Tasks 1.2.2/1.2.3. O resultado do gerador não duplica a trilha SQL.

#### Task 1.2.2: Escritor com dono

- [x] Done

**Context:** um dono por tenant, um lote em voo, estado provisório privado. Admissão na entrada e protocolo comum são obrigatórios.

**Unidades de execução:** começar pelo suporte SQL comum, depois entregar a Task 1.2.3 e voltar ao owner. A primeira unidade cria `tests/bench/engine/sql_session.go` e `sql_session_test.go`: uma conexão por sessão via pgx stdlib, compatível com `*sql.Tx` das libs, observa notices por conexão e exige retorno sem erro e sem notice para confirmar COMMIT. Usar `libPostgres.NewFromPools` quando integrar outbox; não criar adapter próprio. Teste `TestSQLSessionCommit`, com `-count=1 -race -tags bench -timeout 5m`, prova commit limpo, cancelamento em SyncRep e timeout do pgx. Somente na stack descartável verificada: pausar replay, usar `SET LOCAL synchronous_commit=remote_apply`, cancelar apenas o PID próprio em SyncRep e retomar replay com cleanup limitado. Linha visível no primário comprova commit local, nunca durabilidade replicada. Não substitui as 20 rodadas de falhas da Epic 1.3.

**Suporte SQL concluído (2026-10-16):** teste real provou que cancelar a espera SyncRep deixa o commit local visível e devolve COMMIT sem erro, acompanhado de `WARNING 01000`; a sessão retorna `unknown`. O deadline do pgx também deixa commit local possível e retorna erro. A sessão isola e limpa notices por COMMIT, fixa `client_min_messages=warning` e elimina grafias duplicadas do parâmetro antes do connect. A combinação de `options` restritiva e chave com capitalização diferente passou em três rodadas supervisoras com `-race`; sem normalização, 8 de 24 conexões do probe aceitaram `error`. Outra sessão não herda o warning; após retomar replay, um COMMIT `remote_apply` limpo permite conferir as cinco linhas no standby. Suite integrada com ordem variável: 14 testes e 26 subtests, sem skips; vet e lint v2.14.0 passaram. Cleanup retomou replay. Isso prova classificação na stack local, não failover, writers financeiros, perda de ACK no proxy ou TPS.

**Obrigações compartilhadas concluídas (2026-10-16):** `obligations.go` recebe request ID explícito, `events.TransactionSource` e `benchOperation` já decididos; não reconstrói saldo ou versão inicial. As 34 obrigações comparadas nas três fixtures e dois pedidos conservaram IDs, aggregates e bytes, com 4/5/8 rows por fixture. O teste novo cobre dois débitos em chaves da mesma conta, crédito de 49 e tarifa de 1, versões não zero, alias/key e timestamps. Metadata usa `CreatedAt=UpdatedAt=TransactionDate` do create v2; tarifa deriva de `feeApplied` e `packageAppliedID` da metadata, sem parâmetro duplicado ou pacote fixo no builder. O ID da reserva Tracer é representativo e local, sem chamada real. Catalog, novo teste e outbox real passaram com `-race`; suite supervisora integrada: 15 testes, 26 subtests, sem skips. Review e contrarian corrigiram duas premissas erradas do brief, sem Critical/High/Medium restante. Nenhuma decisão financeira executada nesta unidade.

**Unidades do owner após screening (2026-10-16):** primeiro provar aquisição, renovação e exclusão SQL; depois reload completo paginado com renovação; somente então integrar cache, publicação pós-commit e parada por perda à fila existente. Não entregar um owner sem fence nem adiar ticker/paginação porque a fixture pequena é rápida. Preservar a política `renovar/travar` no começo de cada passagem; não trocar por lock-only para facilitar o teste. Nenhuma nova interface ou cópia da fila antes de existir o segundo consumidor.

A primeira unidade cria `writer_owner.go` e `writer_owner_test.go`. Aquisição e renovação usam transações curtas e o classificador existente de COMMIT; erro ou notice não entrega token utilizável. Renovação em uma transação já aberta exige tenant, owner, epoch e lease ainda válido, atualiza a validade e mantém a fence travada. Reutilizar essa operação na renovação independente e depois na passagem financeira. Tests reais, com `-race`, provam disputa, expiração, token antigo, espera por lock, recheck após renovação e COMMIT local com warning em SyncRep sem sucesso declarado. Mutants de remoção dos predicados de lease/epoch e do classificador devem falhar. Owner e tenant permanecem no SQL; o bench de um tenant não produz os tokens/linhas necessários para distinguir sua remoção isolada. Nenhum cache, Submit, reload ou medição de TPS nessa unidade; a task permanece aberta. Verificação: `go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestOwnerFence' -timeout 10m -v`, seguido das regressões sem conservação. Esperas usam estado observado no banco e polling limitado, nunca carga sintética.

**Fence concluída (2026-10-16):** três testes reais provam aquisição, renovação, expiração durante uma transação já aberta, epoch antigo e recheck depois de lock. Uma renovação em SyncRep mantém o sucessor bloqueado pela própria sessão; após cancelar a espera, retorna erro, embora a linha tenha commitado localmente, e o sucessor recusa a posse diante do lease renovado. Aquisição com COMMIT incerto devolve token vazio. O teste confere que BEGIN precedeu a expiração, sem depender de velocidade presumida.

Supervisor repetiu os três testes com race/shuffle três vezes: nove passes, 24,343 s. Suite completa sem conservação: 34 testes e 47 subtests, zero skips, 70,740 s. Vet, gofumpt, lint v2.14.0 e goleak passaram; réplica retomada, zero SyncRep, sessões e toxics restantes. Sete mutants de lease/epoch, relógio de transação, lock-only e bypass do classificador falharam comportamentalmente. Os três mutants de owner/tenant sobreviveram por construção, sem tokens artificiais. Review e contrarian sem Critical/High/Medium restante. Foram removidos setup duplicado de sessão, asserções redundantes e cleanup de teste que só acrescentava erro após cancelamento. Não há ticker, reload, cache, Submit nem execução financeira com owner; a Task 1.2.2 permanece aberta.

**Unidade de reload paginado (2026-10-16).** Reutilizar a sessão de uma conexão, o classificador de COMMIT e os scanners de saldo/controle. Ler páginas sem transação longa: saldos vivos por `id`, controles pela chave composta `(organization_id, ledger_id, account_id)`, sempre com ordem e cursor estrito. Ampliar somente o argumento de `queryRows` para a operação comum a `*sql.DB` e `*sql.Tx`; preservar os quatro callers existentes. Os dois leitores compartilham colunas e scanners. O fluxo financeiro ownerless conserva seu escopo atual de organization/ledger.

A aquisição confirmada valida o início. Entre páginas, consumir o tick pendente e renovar pelo SQL existente. Depois da última página, renovar incondicionalmente. Um erro de leitura, cancelamento ou renovação retorna ambos os mapas nulos. A renovação final comprova que nenhum takeover ocorreu durante as páginas. A consistência dos mapas também exige excluir writers que ignoram a fence. Não promete lease válido no instante do retorno: cada passagem financeira futura deve renovar/travar antes de usar o cache. O runtime posterior cria um único ticker de 1 s para lease de 3 s e páginas de 50k; a unidade de reload recebe esses valores sem criar goroutine de renovação, retry ou timeout próprio.

Tests reais usam páginas de três linhas, uma organization/ledger adicional válida e um saldo soft-deleted. Conferem chaves e valores, a última página cheia, a chave composta e a renovação durante uma leitura que atravessa o lease inicial. Locks observados no banco controlam expiração/takeover na última página e cancelamento de leitura. Todo erro deixa mapas nulos. Verificação: `go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestOwnerReload' -timeout 10m -v`; depois a suite completa com `-skip '^TestOwnerlessWriterConservation$'`, vet, gofumpt e lint v2.14.0. Mutants em scratch verificam os erros de paginação, ausência de renovação e retorno parcial. Arquivos: `writer_owner.go`, `writer_owner_test.go` e `batch_sql.go`; README e este plano registram apenas resultados executados. Sem cache publicado, fila, execução financeira owner ou reload de 1M nesta unidade.

**Reload concluído (2026-10-16):** o primeiro RED foi só de compilação, sem comportamento observado. O teste de paginação confere nove saldos vivos em páginas 3/3/3/0 e oito controles em 3/3/2 contra valores renderizados pelo PostgreSQL; renova durante uma leitura que ultrapassa o lease inicial. Os outros testes usam páginas de 50k sobre a fixture pequena: takeover na última página invalida o resultado; cancelamento da leitura de controles descarta os saldos já carregados. Ambos retornam mapas nulos. Cada liberação de lock no teste de paginação deixa cerca de 1,5 s para a renovação; não é medição de capacidade.

Duas regressões completas foram invalidadas por execuções sobrepostas com `DROP SCHEMA`; os logs foram preservados. A recuperação usou um único executor sob lock de processo: 37 testes e 47 subtests passaram com race/shuffle em 235,840 s. Após os ajustes finais, o supervisor repetiu os três testes com race/shuffle três vezes, 9/9 em 18,243 s, e a suite completa sem conservação passou em 75,302 s, com 37 testes e 47 subtests. Zero falhas ou skips; vet, gofumpt, lint v2.14.0 e goleak limpos. Réplica retomada, zero SyncRep, clientes ou toxics pendentes. Nenhum teste de volume foi repetido; as taxas ownerless anteriores precedem a leitura dos dois UUIDs adicionais no scanner de controles.

Review e contrarian terminaram sem Critical/High. Dezesseis mutants produziram nove falhas comportamentais e sete sobreviventes. O supervisor aplicou a troca de lock comprovada pelo contrarian: o teste de cancelamento passou a matar retorno de mapas preenchidos no erro da leitura de controles, em vez do retorno vazio na leitura inicial de saldos. A cobertura continua sem distinguir renovação removida de apenas um loop, retorno parcial em erro de tick e algumas variantes de cursor/order. Os retornos e o SQL foram conferidos; sobrevivência não prova equivalência. Foram removidas a query de ordenação de constantes da fixture e uma releitura da fence já implicada pelo sucesso. Suporte do bench +98 linhas líquidas, testes +186, código de produto inalterado. Cache, ticker do runtime, fila e execução financeira owner continuam pendentes; a Task 1.2.2 permanece aberta.

**Incremento de runtime owner e gates pequenos (2026-10-16).** Duas unidades sequenciais, com aceitação só após ambas: U3A integra estado owner opcional no batcher, uma sessão e um worker da fila existente; U3B compartilha os gates de identidade, multikey, proteção e admissão entre owner e ownerless, sem duplicar os corpos. Nenhuma medição de volume nesta unidade. Arquivos de suporte: `tests/bench/engine/batch.go`, `writer_owner.go`, `writer_ownerless.go`. Testes: `writer_owner_test.go`, `writer_gates_test.go`, `writer_ownerless_test.go`; README e plano registram resultados após execução.

O constructor adquire e faz reload sob contexto limitado do caller antes de servir. O owner mantém o ticker real de 1 s, para lease de 3 s, e o mesmo canal usado no reload de páginas de 50k. A goroutine da fila atende ticks e batches, sem renovação concorrente durante SQL. Renovação ociosa tem limite de 3 s; constructor falho, perda e saída normal param o ticker. Cada passagem financeira renova/trava a fence como primeiro statement após BEGIN, copia só as chaves envolvidas e reutiliza identidade, overlays, persistência e outbox. COMMIT confirmado marca o ACK antes de publicar o cache; rollback e unknown não publicam. Qualquer erro final torna o owner terminal: descarta mapas, preserva causa e responde unknown sem SQL até Close. Um ACK confirmado após expiração continua aprovação; a próxima renovação recusa nova escrita.

Provas reais de U3A cobrem renovação ociosa, batch que termina após expiração, unknown terminal também para pedido em saldo não tocado e sucessor que recarrega/replaya a identidade original. Pausar réplica somente após observar o owner dentro da passagem financeira evita testar uma renovação ociosa por engano. U3B preserva cinco resets por gate, budgets e expectativas; a pausa da recusa ocupada usa claim em voo, pois uma recusa pelo cache não toca saldo. Fixtures mudam antes do reload. Close conserva o contrato de chamada após todos os Submits retornarem; não criar teste com enqueue concorrente ao fechamento. Verificação: testes novos com `-race -tags bench -timeout 15m`, depois suite com `-skip '^TestOwnerlessWriterConservation$'`, vet, gofumpt, lint v2.14.0 e cleanup. Review independente e mutants em scratch desafiam publicação antecipada, ausência de publicação, renovação e serviço após unknown. A Task 1.2.2 permanece aberta até os gates de volume owner.

**Runtime e gates pequenos executados (2026-10-16).** O owner usa a fila existente, publica o cache após confirmação e para após unknown ou perda da posse. Os quatro gates financeiros compartilham os mesmos corpos e expectativas entre owner e ownerless, com cinco resets. O RED foi comportamental; testes de lease, sucessor e publicação passaram. Review independente e contrarian não encontraram Critical/High. Os mutants detectaram ausência de renovação, fence fora da passagem financeira, serviço após unknown, publicação ausente e publicação do estado de uma passagem abortada. O retorno incorreto de Close após perda ociosa, confirmado no review, foi corrigido abaixo. Comandos, resultados e limites estão no [README da bancada](../../tests/bench/engine/README.md#owner-writer). Código de produto inalterado; nenhuma prova de volume owner ou capacidade neste incremento.

**Runtime owner aceito (2026-10-16).** Review e contrarian terminaram sem Critical/High. A correção do único Medium confirmado faz Close reportar perda ociosa mesmo sem pedido posterior; preserva o primeiro erro de batch e o isolamento ownerless. `TestOwnerWriterIdleLoss` falhou por asserção antes da correção e detecta a remoção do deadline ocioso de 3 s. Esse prazo limita o contexto do cliente, não promete liberação do lock SQL exatamente em 3 s. A supervisão final passou: 11 testes e 42 subtests com race/shuffle, depois 44 testes e 68 subtests na suite sem volume, zero falhas/skips; vet, gofumpt, lint e cleanup limpos. Saíram duas asserções redundantes e cinco linhas de comentários. Suporte da bancada +150 linhas líquidas, testes +289, código de produto zero. Detalhes e limites estão no [README](../../tests/bench/engine/README.md#owner-writer). Owner 200k, reload de 1M e failover continuam sem prova; a Task 1.2.2 permanece aberta.

**Implementation vision:** `writer_owner.go` drena até `maxBatch=256`. `takeover()` usa `UPDATE ... WHERE tenant_id=$tenant AND lease_until <= clock_timestamp()`, troca owner e incrementa epoch. Renovação exige owner/epoch e `lease_until > clock_timestamp()`, nunca `now()`. Lease de 3 s e renovação de 1 s são parâmetros de bench; renovar durante reload.

Carregar saldos por `balance.id` e controles pela chave composta, páginas de 50k. Mapa monetário por balanceID. Bench exclui outros writers durante reload; concorrência administrativa é gate da Epic 3.2. Erro de página/lease aborta sem mapa parcial.

`beforeBegin` antecede decisão/locks. Abrir SQL, renovar/travar fence válida, reclamar identidades canônicas, resolver anteriores, revalidar admissão e decidir sobre cópia privada. Inserir transaction/operation em lote, atualizar saldos/controles por IDs e versões esperadas, exigir todas as linhas, finalizar resultado/outbox. Crédito novo pode financiar débito posterior; replay nunca financia de novo. Divergência de versão aborta tudo e encerra dono para reload. Corte cruzado antes do COMMIT exige rollback/recomputação integral do contrato comum.

Confirmação publica memória e resposta. Erro/notice de replicação incerta mantém `unknown`, descarta cópia e encerra dono; sucessor resolve identidades com barreira WAL. Ticker não renova lease expirado. Takeover espera lock SQL em voo; expiry não revoga retroativamente o lote.

**Files:**
- Create: `tests/bench/engine/writer_owner.go`
- Test: `tests/bench/engine/writer_owner_test.go`

**Verification:** `timeout 1860 go test -count=1 -failfast -tags bench ./tests/bench/engine/ -run '^TestOwnerWriter(Identity|MultiKey|Protection|Admission|Conservation)$' -timeout 30m -v`. Cinco rodadas com reset, 200k pedidos na conservação, join 1:1 de aprovação/transação e outbox completo. Cobrir duplicatas no lote/concurrentes, payload divergente, replay de crédito antes de débito, rollback inteiro, replay vencido, block/flags, esgotamento concorrente e controle multikey compartilhado. Pausas controladas na entrada, após locks e antes do COMMIT demonstram cada corte e nenhuma escrita financeira do lote abortado.

**Unidade de conservação owner (screening concluído, 2026-10-16).** Compartilhar o corpo atual de `TestOwnerlessWriterConservation` com o novo `TestOwnerWriterConservation`; somente o opener escolhe um owner ou 32 workers sem dono. Preservar cinco shapes, 200k por shape, 256 produtores, cap 256, budgets 30/28 s, trabalho de 5 min, Close separado de 30 s após join, reset, baseline e checker único. O writer vive no contexto do subtest; não cancelar um contexto de startup que também sustenta seu lifetime. O comando inteiro mantém os limites Go/externo. Mudanças apenas em `writer_gates_test.go`, README e plano. Primeiro executar smoke em overlay de scratch com 2k por shape nos dois caminhos, sem mudar o N canônico. Após review, executar uma vez o comando de verificação acima com regex ancorado, `-failfast` e `timeout 1860`. Falha de lease, deadline ou Close reprova o gate; sem restart ou ajuste de budget. Regressões comuns devem excluir ambos com `-skip '^TestOwner(less)?WriterConservation$'`. Nenhuma taxa do owner será apresentada como comparação controlada com as medições ownerless antigas.

**Entrada compartilhada aceita (2026-10-16).** Os dois writers usam o mesmo teste de conservação, com um owner ou 32 workers depois do baseline. O smoke de 2.000 pedidos aprovou todos nos cinco shapes dos dois writers, sem erro, atraso, publish ou deadlock. O overlay ficou em scratch; o teste canônico mantém 200k. A suite sem volume passou: 44 testes e 68 subtests, zero falhas/skips. Review e contrarian não encontraram Critical/High/Medium. O contrarian repetiu o smoke do owner e conferiu a posse na fence. O supervisor conferiu código, callers e logs. Foram corrigidos o comando canônico e a exclusão dos dois testes de volume nas regressões atuais.

**Gate completo do owner aceito (2026-10-16).** Uma única execução em `0d5aa04fa` mais a entrada compartilhada, sem race, retry, tuning ou redução de N, passou: cinco testes e 26 subtests, zero falhas/skips, 762,832 s dentro do limite de 30 min. Cada carga respondeu exatamente 200k aprovações, sem outro outcome, erro, atraso de entrada ou publish; Close retornou nil antes do checker. Uma passagem por lote e zero deadlocks aparecem no log, sem asserção própria. As taxas observadas, de 1.288 a 2.054 aprovações/s em malha fechada numa execução, ficaram abaixo de 8k (comparação inválida como aceitação: ver a [correção do alvo](#custo-e-alvos-da-bancada)); não medem capacidade nem se comparam às medições ownerless anteriores, que precedem mudanças compartilhadas de scanner, fila e batch. A leitura independente do mixed_fee retido, no primário e no standby, conferiu 200k pedidos/transações, 600k lançamentos, 1,6 milhão de obrigações PENDING, saldos, versões, limites, cortes e o gerador; os digests das sete tabelas coincidem. A fence final, owner `bench-conserve` no epoch 1, corrobora o caminho owner. Os quatro estados anteriores dependem do checker e do log. Tabela, p99 e limites estão no [README](../../tests/bench/engine/README.md#owner-writer). CPU, memória, WAL por pedido e lote máximo não foram medidos; Close em µs não mede liberação de lease. Com as provas anteriores de fence, reload, runtime e gates pequenos, a Task 1.2.2 está concluída; volume não prova correção universal.

**Done when:** pisos por aprovação passam, rollback/unknown não publicam estado e replay não inicia dinheiro. Atendido para o escritor com dono; a Task 1.2.4 ainda não foi implementada.

#### Task 1.2.3: Escritor sem dono

- [x] Done

**Context:** baseline sem cache/lease; row locks de saldo não substituem exclusão de identidades.

**Primeiro incremento:** um batch SQL diretamente chamável, sem fila ou `Submit`, já implementa o protocolo completo de identidade, admissão e confirmação. Uma lane escreve o núcleo e os testes atômicos; outra, sequencial, acrescenta provas de espera real, cutoff, COMMIT incerto e concorrência. Review e contrarian verificam a união antes do commit. Só depois entram 32 workers, `Submit`/`Close` e os cinco runs de 200k; esse incremento não conclui a Task 1.2.3.

**Primeiro incremento concluído (2026-10-16):** seis testes de batch passaram contra PostgreSQL real. O lote misto grava cinco aprovações, três recusas e uma expiração; cópias fazem replay ou conflito sem nova movimentação. O teste de oito sessões decide 24 identidades uma vez: dez aprovações esgotam o limite 100; 14 recusas e 80 replays preservam dinheiro e versões. A barreira do replay entra em SyncRep com a réplica ainda pausada; só responde após retomar a réplica. O cutoff pre-COMMIT desfaz inclusive o crédito que financiava outro débito. Um deadlock real produz uma recomputação, somente antes de COMMIT.

**Verificação supervisora:** três runs de batch/outbox com `-race -shuffle=on` passaram; suite integrada completa: 21 testes e 26 subtests, zero skips, 41,604 s sem overhead de race. Vet, format e lint v2.14.0 passaram. Standby retomado, zero SyncRep e zero sessões remanescentes. Mutants produziram falhas comportamentais ao remover os cortes, permitir retry após COMMIT, retirar o touch WAL ou usar o maior deadline. Review e contrarian não deixaram Critical/High nem Medium alcançável; guard de tamanho e setup duplicado foram removidos.

**Limites do incremento:** o menor deadline futuro pode produzir `unknown` para todo o lote sob backlog; custo ainda não medido. Replay-only ainda executa duas queries de lock com arrays vazios. Flags de envio/recebimento e recusa cruzando cutoff são cobertas pelos gates pequenos abaixo. Oito sessões comprovam a execução testada, não ausência universal de deadlocks. `Submit`/`Close` e 32 workers pertencem à unidade seguinte, concluída abaixo. Os cinco runs de 200k foram concluídos abaixo; owner/cache/fence, failover e a matriz de capacidade continuam pendentes.

**Decisões do incremento:** usar `decimal.Decimal` para ler saldo/limite sem cast que arredonde; construir `benchOperation` uma vez para SQL e eventos; preservar o outbox representativo de tarifas. A identidade existente impõe seu fingerprint a cada cópia recebida; a primeira cópia fixa somente identidades novas. Finalizar claims mesmo quando todas as decisões forem recusadas ou expiradas. Claim SQL usa `ORDER BY` explícito. Pedidos já vencidos após resolver identidades não travam saldos. O corte pre-COMMIT de qualquer nova aprovação/recusa invalida o lote completo e recompõe dependências. `40P01` admite uma repetição só antes de invocar COMMIT; qualquer erro/notice depois disso retorna `unknown` sem nova execução.

**Orçamento do incremento:** contexto de vida do writer/teste, limitado a 10 s por execução completa. Cada passagem respeita também o menor deadline de resposta ainda futuro. Deadline já decorrido não impede resolução/expiração, mas nunca renova admissão. Retry copia o pedido e altera apenas o deadline de resposta. `Submit` vai limitar a espera do caller separadamente, sem transformar cancelamento em prova de rollback. Sem novo contrato público.

**Fila e lifecycle concluídos (2026-10-16):** seis testes reais passaram. Foram observados 32 backends próprios em lock wait; os 32 pedidos aprovaram após liberação. Com cap 8 e nove tentativas aguardando o worker ocupado, o drain produziu dois lotes seguintes e respondeu a cada caller com sua própria identidade. Um erro de batch não cancelou outro worker durante Close. Caller cancelado recebeu `unknown`, sem cancelar o trabalho aceito. Pedido agendado 29 s antes esperou só o segundo restante e finalizou `expired`, sem renovar admissão. Trinta e dois pedidos com deadline de resposta já passado não chegaram ao SQL.

**Verificação da fila:** supervisor repetiu fila, batch e outbox três vezes com `-race -shuffle=on`: 39 testes passaram. Suite integrada completa: 27 testes e 26 subtests, zero skips, 48,970 s. Vet, format, lint v2.14.0 e goleak passaram; standby retomado, zero SyncRep e sessões remanescentes. Review e contrarian não deixaram Critical/High nem Medium alcançável. As correções eliminaram duas provas enganosas: aprovação após deadline com relógio parado e teste de Close concorrente com Submit, fora do contrato. Mutants falharam por remover drain/cap, trocar respostas, propagar erro pelo errgroup, renovar orçamento ou enfileirar caller já vencido.

**Limites da fila:** Close continua permitido uma vez, só depois que todos os Submits retornarem. Fila cheia com 8.192 tentativas continua pendente. Os 32 workers com cap 256 passaram nos cinco runs de 200k registrados abaixo. O teste de ausência de enqueue usa 32 tentativas porque o mutant deixa a escolha aleatória do select decidir; não é prova determinística do mutant. Nenhum resultado de TPS.

**Contrato da fila e lifecycle.** `writer_ownerless.go` recebe contexto próprio, batchers preparados e tamanho de lote experimental. Uma goroutine por sessão via errgroup da lib, 32 no baseline. Fila limitada a 8.192 tentativas, constante também para owner e variações de batch/workers; cada caller tem canal de resposta próprio com capacidade 1. Drenar somente o que já estiver enfileirado, até o limite do lote, sem espera para coalescer. SQL usa o contexto do writer, nunca o cancelamento de um caller. Nenhum retry automático de `unknown`.

Observar o cutoff na entrada e contar tentativas já vencidas, enfileiradas ou não. Enquanto o caller tiver orçamento de resposta e não estiver cancelado, as duas classes entram na mesma fila de resolução de identidade. Só SQL pode provar uma identidade nova e finalizar `expired`. `Submit` limita sua espera pelo deadline original e retorna `unknown` se o caller sair; trabalho aceito continua. `Close` ocorre uma vez, depois do último `Submit`, fecha admissão e drena antes de fechar recursos. Somente abort do writer ou deadline do próprio `Close` cancela SQL em voo. Erros de batch ficam por worker e chegam ao `Close` após o join, sem cancelar commits de outros workers durante drain normal. Sem suporte inventado a `Close` concorrente com `Submit`, orçamento inválido ou retry financeiro.

**Provas da unidade:** `go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestOwnerlessWriter(Workers|Drain|ErrorIsolation|CallerLeaves|CloseDeadline|ExpiredEntry)$' -timeout 15m -v`, com stack real. Observar os 32 backends próprios em lock wait antes de liberar a linha comprova uso efetivo das sessões. Cancelar o caller não cancela o commit; drain normal termina a aprovação aceita; deadline de Close aborta a espera sem fingir certeza de rollback financeiro. Conferir dados somente após drain e recursos sem vazamento. Sem fake entre fila e batch. Os cinco runs de 200k vêm em outra unidade sequencial.

**Implementation vision:** cada worker chama o batch comum: abrir SQL, reclamar identidades em ordem canônica, resolver replay, travar saldos ordenados por balance.id e controles pela chave composta. Revalidar admissão, decidir na ordem original sobre cópia das linhas e persistir o mesmo SQL/outbox. Replay isolado também escreve touch WAL.

Ordem uniforme: offset de transporte, fence se houver, identidades, saldos, controles. `40P01` aborta tudo e permite uma recomputação limitada das mesmas identidades ainda admissíveis; nunca retry financeiro de resultado incerto. Corte pre-COMMIT ou conflito tardio invalida o lote inteiro. Depois de enviar COMMIT, timeout não prova rollback.

**Files:**
- Create: `tests/bench/engine/batch.go`, `tests/bench/engine/batch_sql.go`, `tests/bench/engine/writer_ownerless.go`
- Test: `tests/bench/engine/batch_test.go`, `tests/bench/engine/writer_ownerless_test.go`, `tests/bench/engine/ledger_check_test.go`, `tests/bench/engine/writer_gates_test.go`

**Verification do primeiro incremento:** `go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestBatch' -timeout 15m -v`, com a stack descartável verificada. Conferir rows e versões por decisão, controle compartilhado, replay/conflict com touch WAL, finalização sem dinheiro, dependência de crédito descartada no rollback e recuperação de resposta após COMMIT incerto sem nova movimentação. Depois rodar a suite integrada em ordem variável, vet e lint pinado. Testes de volume continuam abaixo.

**Pré-requisitos concluídos (2026-10-16):** o checker de outbox agora usa contagem exata por obrigação em tempo linear, sem percorrer todo o mapa por evento. Evento ausente não é compensado por outro duplicado; IDs, transação, estado e metadata de tarifa são correlacionados. Mutants de emissão repetida, lançamento trocado, destino de metadata errado, Tracer incorreto e saldo anterior foram detectados no código final. O workload fornece seu relógio de runtime; fixtures continuam com relógio controlado e nenhum `_test.go` lê o relógio do sistema diretamente. Supervisor executou 17 testes focados e cinco subtests com race; suite completa: 27 testes e 26 subtests, zero skips, 47,621 s. Vet, format e lint passaram; cleanup sem sessões restantes. Review e contrarian sem Critical/High/Medium. Ainda não houve run de 200k; custo de memória do checker permanece para medir no piloto.

**Gates pequenos verificados (2026-10-16):** identidade, multikey, proteção e admissão passaram em cinco resets cada. Cópias e payloads divergentes disputam 128 identidades em 32 workers; o payload gravado define replay e conflito. O crédito já gasto não financia outro débito por replay. Dois saldos compartilham limite 1.000: 100 débitos de 10 aprovam e 200 recusam. O esgotamento de saldo e as flags recusam sem deixar lançamentos parciais. Uma recusa que cruza cutoff pre-COMMIT desfaz a passagem inteira e vira `expired` na recomputação.

`ledger_check_test.go` captura requests/respostas e compara o payload vencedor com o multiset exato dos lançamentos. O baseline ancora saldos, controles, versões e flags, inclusive linhas sem movimentação. A transação esperada vem do request capturado mesmo quando o caller recebe `unknown`. O snapshot exige reset e fixtures concluídos, nenhuma operação anterior e conexão nova por run. Aprovação SQL sem resposta nova `approved` aparece separada; um replay posterior não apaga essa contagem.

**Verificação supervisora dos gates:** 21 testes e 26 subtests com race/shuffle passaram em 65,840 s; suite completa: 31 testes e 47 subtests, zero skips, 61,381 s. Vet, format, lint v2.14.0 e goleak passaram. Standby retomado, zero SyncRep e sessões remanescentes. O probe de caller `unknown` com commit legítimo passou; o mutant que grava outra transação em todas as tabelas falhou pelo payload capturado. Mutants também demonstraram que saldo inicial errado e postings do payload perdedor escapavam dos checks antigos, mas falham nos novos. Foram removidos a query redundante de limite e um helper com um caller.

**Piloto outflow verificado (2026-10-16):** 200.000 pedidos aprovados, nenhum outro outcome, erro, atraso de admissão, publish ou deadlock. Setup 0,433 s, run 195,791 s, Close 31 µs e checker 22,104 s; teste completo 218,38 s. O run ficou no orçamento de 5 min. Foram 35.584 lotes de uma passagem, média observada de 5,62 pedidos, não lote de 256. A taxa foi 1.021 aprovações duráveis/s e o p99 de resposta 609,664 ms nesta execução local fechada. Isso fica abaixo do alvo (comparação inválida como aceitação: ver a [correção do alvo](#custo-e-alvos-da-bancada)), sem demonstrar capacidade máxima, matriz, sustentação ou variância.

A conferência read-only independente encontrou 200k identidades aprovadas, 200k transações, 400k lançamentos e 800k obrigações PENDING, sem publicação. Débitos e créditos somaram 10.103.541 cada; a soma dos oito saldos permaneceu 80.000.000.000. O RSS máximo observado foi 909.460 KiB entre o binário de teste e seus filhos reaped, incluindo `go list`; não é heap, soma de processos ou memória do Postgres. O checker coube no budget sem reescrita. As cargas com tarifa não fizeram parte do piloto; o gate completo abaixo as cobre.

**Review do piloto:** code, logic, performance e contrarian não encontraram Critical/High nem defeito financeiro alcançável. O contador duplicado de workers com erro foi removido. O probe RED de acknowledgements perdidos produziu 32 `unknown` e falhou, sem tratá-los como aprovações. Esse probe de 2.000 pedidos não substitui o piloto real de 200k. Deltas WAL e temporários cobrem a janela inteira, inclusive setup, checker e atividade de fundo; não atribuir o custo inteiro ao dinheiro ou ao checker. A regressão supervisora final passou: 21 testes/26 subtests com race, 31 testes/47 subtests sem race, zero skips, vet/gofumpt/lint e cleanup limpos. Conservação foi excluída explicitamente dessas regressões; um probe de 2.000 pedidos conferiu a remoção do contador duplicado. O piloto real não foi repetido nessa aceitação. Os cinco runs completos, registrados abaixo, mantiveram budgets e carga.

Cinco runs de conservação com reset e 200.000 pedidos cada: outflow, inflow, mixed, outflow com tarifa e mixed com tarifa. Usar quatro contrapartes ordinárias já semeadas, 32 workers, cap 256, 256 produtores em malha fechada e os orçamentos usuais. Cap 256 não é tamanho observado de lote. Cada run exige 200k aprovações, nenhum outro outcome, nenhum erro e conferência após Close. Capturar requests/respostas reais, vincular o payload vencedor aos postings e verificar baseline, piso por posting, flags, limite compartilhado, versões e outbox completo. Não contar aprovação local com caller `unknown` como sucesso durável. Piloto e runs mantêm orçamento de 5 min cada e Close separado de 30 s; falha para a expansão, sem aumentar os prazos para fazer passar.

O entrypoint de conservação reutiliza os helpers diretamente, sem `newGate`: contexto do teste para vida do writer, contexto de 5 min só para callers e contexto novo de 30 s para Close. Setup e baseline precedem o run; conexões lazy e prepares ainda entram no primeiro lote. Registrar erros, contagens e tempos antes das asserções. Close com erro bloqueia a conferência, pois um backend abandonado pode terminar o commit depois. Close limpo permite conferir inclusive o run abortado. O piloto executa só `outflow`, sem race, com limite externo de 600 s e timeout do teste de 9m30s. Não reduzir pedidos nem ampliar budgets por falha. Regressões comuns excluem os dois writers com `-skip '^TestOwner(less)?WriterConservation$'`; os cinco runs ownerless usam o comando separado abaixo.

Os fixtures pequenos cobrem concorrência de cópias/payloads, crédito de replay já gasto, esgotamento multikey e saldo, flags de envio/recebimento e recusa cruzando cutoff pre-COMMIT. Não repetir as provas de lifecycle e SyncRep já entregues. A fila cheia e a carga aberta de backlog permanecem na Task 1.2.4. Logs separam setup, run e checker; nenhuma taxa desse gate substitui a matriz ou o soak.

**Verification:** `go test -count=1 -failfast -tags bench ./tests/bench/engine/ -run 'TestOwnerlessWriterConservation|TestOwnerlessWriterIdentity|TestOwnerlessWriterMultiKey|TestOwnerlessWriterProtection|TestOwnerlessWriterAdmission' -timeout 30m -v`. Cinco runs de conservação e cinco repetições dos fixtures concorrentes pequenos; registrar deadlocks/retries. Primeiro executar apenas o subtest piloto de conservação, com timeout externo de 10 min. Race detector fica nos fixtures pequenos; volume executa sem race.

**Gate completo aceito (2026-10-16):** uma única execução em `d840eb78e`, sem race, retry, tuning ou redução de N, passou nos cinco runs e nos quatro gates pequenos. Foram cinco testes e 26 subtests, zero falhas/skips, 1.282,347 s dentro do limite de 30 min. Cada carga respondeu exatamente 200k aprovações; nenhum outro outcome, erro, atraso, publish ou deadlock. Todos os lotes fizeram uma passagem. O budget de 300 s é do run; cada pedido mantém 30 s para resposta e 28 s para admissão.

| Carga | Run (s) | Checker (s) | Aprovações/s observadas | p99 de resposta (ms) | Média de pedidos/lote |
|---|---:|---:|---:|---:|---:|
| outflow | 198,704* | 22,104 | 1.006,5* | 771,248* | 5,73 |
| inflow | 183,231 | 22,186 | 1.091,5 | 516,909 | 6,09 |
| mixed | 204,180 | 22,477 | 979,5 | 695,331 | 5,79 |
| outflow_fee | 274,877 | 36,133 | 727,6 | 837,249 | 5,59 |
| mixed_fee | 263,778 | 36,091 | 758,2 | 749,944 | 5,70 |

\* O executor usou uma espera que consumiu um core por cerca de 100 s durante outflow, contra a regra de não gerar carga sintética. O processo terminou; o supervisor também identificou e removeu um `tail -F` órfão do piloto. A correção financeira permanece provada, mas esse timing outflow está excluído da comparação. Nenhuma carga foi repetida para esconder a violação. As outras taxas descrevem só esses runs fechados, não capacidade máxima ou sustentada, e ficaram abaixo de 8k/s (comparação inválida como aceitação: ver a [correção do alvo](#custo-e-alvos-da-bancada)).

O contrarian verificou o último estado, mixed_fee, no primário e standby: 200k aprovações/transações, 600k lançamentos, 1,6 milhão de obrigações PENDING e soma de saldos 80.000.000.000. Cada pedido teve débito A, crédito A−1 e tarifa 1. Débitos e créditos somaram 10.212.408 cada; versões, pisos, limites e payloads conferiram. O supervisor confirmou esses totais em leitura própria. Os quatro estados anteriores foram apagados pelos resets e dependem do checker/log de cada run, não dessa leitura final.

**Limites da medição:** o maior run com tarifa usou 274,877 dos 300 s, margem de 25,123 s; isso não estabelece margem em repetições futuras. O `ru_maxrss` reportou 1.244.712 KiB e uma amostra manual de `ps` reportou 1.327.320 KiB; a divergência não foi isolada, então o pico real permanece desconhecido. Memória/CPU do Postgres e séries brutas de latência não foram preservadas. WAL e spill foram medidos na janela completa, incluindo setup, checkers e atividade de fundo, sem atribuição por pedido. Entrega dos eventos não foi testada. Matriz, backlog/fila cheia, owner, failover e soak continuam pendentes. Reviews de lógica/medição e contrarian não encontraram defeito financeiro; ressalvas de evidência foram corrigidas, sem novas abstrações ou guards.

**Done when:** os mesmos gates de identidade, admissão e proteção passam antes do ranking de desempenho. Atendido para o escritor sem dono; a Task 1.2.4 segue aberta.

#### Task 1.2.4: Matriz local e comandos limitados

- [ ] Done

**Context:** latência injetada não é RTT medido; PG16 local não reproduz RDS. Tag `bench` fica fora dos shards e CI existentes.

**Primeira unidade após screening (M1, 2026-10-16).** Acrescentar uma janela positiva ao gerador fechado, mantendo o limite finito de pedidos. O primeiro limite atingido para a admissão; o caller de uma célula por duração deve rejeitar `dispatched == n`, pois terminar por quantidade não completa a janela. A conservação mantém 200k e janela de 5 min, com o abort existente; o teste nulo mantém quantidade e janela de 1 min. Não usar `MaxInt64`, crescimento ilimitado das séries, novo modo ou sentinel. Ler o relógio uma vez por tentativa, antes de reclamar o índice, e reutilizar esse instante no pedido. A parada normal não cancela contexto: o join espera as respostas em voo, e o contexto externo continua sendo apenas abort.

Mover a alocação das cinco séries para `newRecorder(n)`, antes do início medido; `begin` só marca o início. O output passa de `scheduled` para `capacity`, sem confundir o teto com o número enviado. Atualizar todos os callers em `workload_test.go` e `writer_gates_test.go`. O gerador aberto preserva a grade absoluta e envia pedidos atrasados da janela, sem backpressure novo. Arquivos M1: `workload.go`, `metrics.go`, seus testes e os argumentos do gate de conservação; README e plano. Nenhuma mudança nos writers, SQL, lease, fila ou budgets.

Provar RED comportamental e GREEN no writer port existente, com relógio controlado: a janela para antes do limite de quantidade, preserva identidades e prazos, e o pedido em voo pode terminar depois da parada. Preservar testes de quantidade e abort. Não testar capacidade de slice como prova de exclusão da alocação; conferir a ordem real das chamadas. Verificar gerador/schedule/quantiles/parada com race e shuffle, suite completa excluindo `^TestOwner(less)?WriterConservation$`, smoke de 2k por shape nos dois writers via overlay de scratch, vet, gofumpt e lint v2.14.0. Não repetir os runs de 200k nesta unidade.

**M1 aceita (2026-10-16).** RED compilou e falhou por excesso de envios; GREEN passou com race e shuffle. Review independente e contrarian não encontraram Critical/High/Medium. O supervisor removeu a asserção que só repetia a capacidade passada ao constructor e limitou a afirmação de pré-alocação: o `make` precede a janela, mas page faults no primeiro acesso ainda podem ocorrer. A verificação final passou: cinco testes e sete subtests com race/shuffle em 25,390 s; suíte sem conservação com 45 testes e 70 subtests em 100,236 s, zero falhas/skips. Vet, gofumpt, lint v2.14.0 e cleanup limpos. Os smokes de 2k anteriores passaram nos cinco shapes dos dois writers. O probe de cancelamento no fim de um produtor falhou no mutant e passou no código real; a ausência de cancelamento normal também foi conferida na lib. No limite do timeout, a janela pode parar antes do timer de abort; a conservação continua exigindo 200k aprovações e reprova a execução incompleta. Nenhum run de 200k foi repetido. A Task 1.2.4 continua aberta; a próxima unidade é a medição de confirmações por janela e ocupação real.

**Segunda unidade após screening (M2, 2026-10-16), aceita.** Duas lanes de escrita sequenciais na mesma worktree, seguidas por três reviews independentes em paralelo e contrarian. A M2A separa confirmações por janela; a M2B mede os lotes retirados da fila. Nenhuma mudança no protocolo financeiro, SQL, lease, budgets ou contratos públicos.

- M2A: `recorder.begin(start, stop)` recebe o mesmo fim usado pelo gerador fechado ou pela grade aberta. Só uma aprovação nova com `dbAckAt` anterior a `stop` conta dentro da janela; igualdade e instantes posteriores contam depois da janela, inclusive pedidos enviados com atraso pelo gerador aberto. Aprovações sem ACK permanecem não medidas; replay e recusa não contam. O summary mostra duração e contagens, sem taxa por janela, pois a conservação pode terminar por quantidade. `dbAckAt` é a observação no host, não o instante do commit no servidor. Commit sem resposta ao caller continua fora dessas contagens e aparece apenas no checker posterior, sem atribuição temporal. Arquivos: `tests/bench/engine/metrics.go`, `workload.go` e `workload_test.go`.
- M2B: cada worker guarda o instante relativo ao início do writer e o tamanho de cada lote retirado, antes da execução. Após Close e join, uma agregação calcula quantidade de lotes, tentativas e maior lote no intervalo `[start, stop)`, inclusive lotes que falharam. Substituir o contador redundante pelo comprimento dos registros. Não confundir lotes retirados com aprovações, CPU ocupada ou todo trabalho em voo. Os registros custam 16 B por lote em amd64, mais capacidade excedente; crescimento, cópias e page faults permanecem custos não medidos. Remover logs que despejariam os registros inteiros. Arquivos: `tests/bench/engine/writer_ownerless.go`, `writer_ownerless_test.go` e `writer_gates_test.go`.

**Verification M2:** RED comportamental e GREEN para classificação antes/exatamente no fim da janela, ausência de ACK e exclusão de replay/recusa; conferir o wiring dos dois geradores. Reusar os testes reais de drain e workers para limites inclusivo/exclusivo, janela vazia, soma entre workers e maior lote. Executar testes focados com race/shuffle, a suíte sem conservação excluindo `^TestOwner(less)?WriterConservation$`, smokes de 2k nos dois writers por overlay descartável, vet, gofumpt e lint v2.14.0. Todo comando de bench usa o runner exclusivo da sessão; nenhum run de 200k nesta unidade. README e plano registram somente provas executadas. A Task 1.2.4 permanece aberta.

**Provas M2 (2026-10-16).** Os dois REDs falharam no comportamento, não na compilação; os GREENs passaram com race/shuffle. Três reviews e contrarian não deixaram Critical/High nem Medium comportamental. O contrarian executou 14 mutants distintos: 13 falharam nos testes; a fórmula de `noDbAck`, usada só no log, permanece sem asserção. O supervisor removeu duas asserções redundantes, distinguiu o instante de início da geração da fence e corrigiu os limites documentados. A verificação final passou: 12 testes e 29 subtests com race/shuffle em 50,847 s; suíte sem conservação com 45 testes e 70 subtests em 102,358 s, zero falhas/skips. Vet, gofumpt, lint v2.14.0 e cleanup limpos. Os smokes anteriores de 2k por overlay passaram nos cinco shapes dos dois writers, com 2.000 aprovações e tentativas retiradas em cada shape. Nenhum run de 200k foi repetido. O custo da instrumentação continua não medido; a Task 1.2.4 permanece aberta.

**Preparação da primeira célula (M3A, 2026-10-16), aceita.** Criar `tests/bench/engine/toxiproxy.go` e `toxiproxy_test.go` com o SDK Toxiproxy v2.12.0 já instalado. Controlar apenas `pg-repl`, conferir a configuração descartável, remover toxics antigos desse proxy e aplicar atraso inteiro em milissegundos downstream, com toxicity 1 e jitter 0. Zero significa ausência de toxic. Fazer read-back após a configuração e o cleanup; erro invalida a operação, sem retry automático nem reset global. Registrar cleanup antes da primeira mutação e remover o atraso somente sem workload em execução.

Observar `sql_roundtrip_pg_repl` com uma sessão finita de psql dentro do standby existente: três queries de aquecimento e 32 medidas, todas `SELECT 1`. O timing por query exclui startup e handshake, mas inclui processamento SQL e scheduling; network RTT continua não medido. Usar `LC_ALL=C`, timeout interno de 50 s e contexto externo de 60 s; validar exit code e quantidade de amostras, sem preencher ausências com zero. O preflight real confirmou psql 17.11, GNU timeout e o formato de timing nesse caminho, sem toxic. O teste real verifica o atraso configurado de 20 ms, preservação da conexão de replicação existente e cleanup; não exige duração exata no host compartilhado. RED/GREEN, teste do parser, race/shuffle, regressão sem conservação, vet e lint completam a unidade. Nenhuma célula por duração, run de 200k, comparação PG16 ou medição de capacidade nesta preparação.

**Provas M3A (2026-10-16).** RED comportamental e GREEN com race/shuffle passaram pela revisão independente e contrarian, sem Critical/High. O supervisor cortou comentários acima da régua e o retorno redundante da configuração. A verificação final passou nos dois testes focados e dois subtests com race/shuffle em 1,989 s; a regressão sem conservação passou em 47 testes e 72 subtests em 103,629 s, zero falhas/skips. Vet, gofumpt e lint v2.14.0 limpos. O cleanup final detectou um `engine.test` transitório de origem não identificada; o processo desapareceu antes da inspeção. Um recheck isolado confirmou zero processos, backends e toxics, com réplica síncrona. O contrarian comprovou cleanup após falha e timeout interno; em scratch, observou um commit síncrono com WAL passar de 0,770 ms para 20,869 ms e a mensagem downstream de 0,203 ms para 20,341 ms, sem reconexão. São observações isoladas. O mutant com direção upstream sobrevive aos testes commitados, mas falhou nessa prova unidirecional de scratch. Timeout externo e erros de timeout do SDK continuam sem execução. Nenhuma célula, run de 200k ou medição de capacidade rodou. A Task 1.2.4 permanece aberta.

**Primeira célula por duração (M3B, 2026-10-16), aceita.** Duas lanes de escrita sequenciais: observações PG17/Go limitadas e integração da célula owner. A célula `BENCH_MATRIX=quick` usa 10k contrapartes, outflow, 256 produtores, batch 256, atraso downstream de 1 ms, 60 s de aquecimento e 20 s medidos. Um writer, dois workloads e recorders, uma captura compartilhada e um único Close preservam a lease e permitem verificar todas as identidades. Limites finitos de 960k e 320k pedidos correspondem a 16k tentativas/s, não a capacidade medida. O término por quantidade invalida a janela. A captura e as séries são alocadas antes do aquecimento; seu custo permanece declarado.

Cada run recebe abort separado de `janela + caller + 2*answerGrace`; a parada normal não cancela o trabalho. Setup usa 60 s onde as dependências aceitam contexto, Close usa 30 s, snapshots usam 10 s e o checker usa 5 min. O comando quick passa para timeout de 15 min, com limite externo de 960 s, pois o limite anterior omitia verificação e cleanup; isso é limite de execução, não previsão. O SDK mantém seus limites próprios e migrations continuam dependendo do timeout do processo após `Up`. Coletar apenas dois snapshots PG17/Go nos limites da medição, sem reset de estatísticas nem CHECKPOINT forçado. Deltas PG incluem drain e atividade de fundo; reset invalida os deltas afetados, não a taxa independente. Reportar memória do processo e GC com seus intervalos; CPU/memória PG, lock wait cumulativo e network RTT permanecem não medidos se não houver coleta.

O run válido exige janela completa, ausência de erro de execução, Close drenado e checker financeiro aprovado. `expired` e `unknown` revelam atraso e não somem do relatório. Replay, conflito ou recusa não esperados neste fixture devem falhar. Uma taxa abaixo de 8k ou latência acima da hipótese não invalida a observação. Reportar carry-over desconhecido separadamente da fila, taxas por janela, outcomes, lotes reais e custos da captura. O primeiro smoke usa overlay descartável de 1 s/2 s; não substitui a célula canônica. Após reviews e contrarian, o supervisor executa a célula canônica uma vez, sem race, retry ou tuning. Arquivos sob `tests/bench/engine`: observações, `bench_test.go`, testes necessários, README e este plano. Make, relatórios persistidos, PG16 e blocos completos ficam para a unidade seguinte. A Task 1.2.4 permanece aberta.

**Provas M3B (2026-10-16).** `TestBenchHotAccount` roda somente com `BENCH_MATRIX=quick`; sem a variável faz skip e qualquer outro valor falha como não implementado. `TestRunPhase` provou, em RED e GREEN, que fase terminada por quantidade ou abortada é inválida e que a terminada pela janela é válida. Smokes de scratch de 1 s/2 s passaram sem e com race; um overlay com teto de 100 no aquecimento terminou por quantidade, falhou e não imprimiu taxa. Três reviews e contrarian não deixaram Critical/High; o contrarian matou o mutant que tirava do checker as identidades do aquecimento e confirmou que erro de Close bloqueia a verificação. O supervisor removeu os guards inalcançáveis de `stats_reset` NULL e de tipo das métricas Go; `checkpointerActivity=observed` passou a significar espera diferente de `CheckpointerMain` em qualquer leitura, inclusive `running`, ou contador crescente, e `unobserved` não prova ausência de checkpoint. A verificação final passou: 3 testes focados e 12 subtests com race/shuffle em 1,134 s; regressão sem volume com 50 testes e 84 subtests em 101,431 s, zero falhas e só o skip esperado do quick; vet, gofumpt e lint v2.14.0 limpos. A primeira tentativa canônica falhou antes de executar Go, pois `/usr/bin/time` não existe no host; o wrapper saiu e o pico de RSS do comando inteiro ficou sem medição. A única execução canônica passou em 325,716 s, sem overlay, race, retry ou tuning, com hashes das fontes iguais antes e depois e cleanup limpo.

**Resultado quick (2026-10-16).** PG 17.11, commit síncrono com `standby1`, `fdatasync`, 1 ms downstream configurado em `pg-repl`, host compartilhado com load average de 1 min de 33,5 antes e 57,3 depois. As 148.702 respostas das duas fases foram aprovadas, sem publish, e o checker conferiu todas as chamadas capturadas. O aquecimento aprovou 106.642, fora da medição, com unknown e fila zero na junção. A fase medida enviou, respondeu e aprovou 42.060: 41.804 com `dbAckAt` antes da parada e 256 na parada ou depois, no drain. A taxa válida é 41.804/20 s = 2.090,2 aprovações/s; resposta com p50 de 119,441 ms, p99 de 209,937 ms e máximo de 287,969 ms. Na janela, os workers retiraram 312 lotes com 41.995 tentativas, média de 134,6 e maior lote de 256; são tentativas retiradas, não envios nem aprovações. Na vida do writer, 1.102 passes e zero deadlock; Close levou 753,567 µs. A sonda SQL mediu mediana de 1,140 ms antes e 1,139 ms depois; não é network RTT. De S1 a S2, 20,059 s com drain e atividade de fundo: posição de escrita +467.363.168 B; `pg_stat_wal`, com flush preguiçoso, 482.472.321 B, 5.687 FPI, zero buffers cheios e 436 syncs em 278,119 ms; checkpointer em `CheckpointWriteDelay` nas duas leituras, 12.093 buffers escritos, atividade observada, não checkpoint provado; `flushLag@end` de 2,792 ms, uma amostra. O processo teve 14 ciclos de GC, 6.928.646.160 B alocados no heap, 11,703 s de CPU de usuário e 0,799 s de sistema; 622.472 KiB é o pico de RSS da vida do processo até S2, não do checker nem só do writer. Esses números do processo incluem gerador, captura, pgx e drain, sem atribuir causa à alocação. O checker levou 244,363 dos 300 s; o teste, 325,69 s dentro de 15 min e 960 s; as margens valem só para este run. É uma observação de 20 s em malha fechada, abaixo dos 8k/s (comparação inválida como aceitação: ver a [correção do alvo](#custo-e-alvos-da-bancada)) e acima da hipótese de 15 ms; não prova máximo, sustentação, failover nem capacidade, e não é comparação controlada com os gates de conservação nem com o ownerless. A Task 1.2.4 permanece aberta.

**Retomada M4 (2026-10-16), código aceito em 2026-10-17.** O novo pedido de acelerar a Fase 1 sucede o fechamento do quick; não reabre sua execução histórica. Quatro screenings terminaram. A reprodução do checker sobre tabelas TEMP reconstruídas dos dados retidos identificou 2.974.634.808 pares rejeitados pelo join filter no check de proteção: 278,571 s sem estatísticas, 0,164 s após `ANALYZE base_balance, base_control`. Os dados financeiros retidos não mudaram. Isso atribui o custo reproduzido, não mede novamente o checker Go completo nem altera o resultado canônico.

M4A acrescenta somente esse ANALYZE ao baseline, generaliza a célula fechada para owner e ownerless e grava resultados estruturados. M4B integra Make e documentação. Um autor escreve por vez; code review e logic review independentes rodam juntos, seguidos do contrarian e da supervisão. Primeiro passam smokes de 1 s/2 s com 10k contrapartes e checker completo; não contam como capacidade. Nenhum autor executa o quick canônico nem uma célula completa.

O escopo M4 é quick e bloco A: owner/ownerless32, atraso downstream de 0/1/10 ms e outflow/inflow/mixed. Cada célula usa processo próprio, conserva captura de warm-up e medição, baseline, Close único e todos os checks. Full exige `BENCH_BLOCK=A`; writer/delay/shape explícitos selecionam uma célula no teste Go. Make enumera o bloco e permite selecionar um par ou uma célula. Os full usam 60 s/120 s; os caps de memória continuam em 16k tentativas/s. Seletores inválidos falham antes de tocar o banco. Não acrescentar flags para tuning arbitrário.

Make grava log, proveniência e JSON em diretório único sob `reports/bench-engine/`, já ignorado. O JSON só existe após cleanup completo e saída zero do processo; uma falha preserva o log, remove apenas o JSON daquela execução e para sem retry. O report writer é o primeiro cleanup registrado e o último executado. Contagens e janela são campos estruturados; nenhum parser extrai taxa da prosa. Ausência de métrica não vira zero. A stage de checker deve permanecer diagnosticável mesmo quando falhar.

Limites M4: quick conserva Go 15 min/externo 960 s; full usa Go 18 min/externo 1140 s por célula, não por bloco. São tetos de execução, não previsões nem garantias de que cada stage pode consumir todo o seu limite. Setup 60 s, run janela + 32 s, Close 30 s, snapshots 10 s e checker 5 min permanecem. Timeout reprova e pode pular cleanup; o executor confere a stack antes de continuar. Make não sobe nem destrói containers. Blocos B/C/V, métricas adicionais, PG16 e transportes continuam em unidades seguintes; nenhum resultado escolhe arquitetura ou autoriza Fase 2.

**Provas M4 (2026-10-17).** Seleção e relatório tiveram RED/GREEN comportamentais. Dois reviews e contrarian terminaram sem Critical/High; o caminho relativo de relatório foi corrigido e comprovado no Make real com diretório padrão. A supervisão também demonstrou que, sem checar o setup expirado após o probe, o ownerless executava dinheiro; com o guard, falhou antes do writer e deixou zero pedidos/transações/lançamentos/outbox. Um par smoke de 1 s/2 s aprovou e conferiu 5.311 e 3.664 chamadas, sem alterar fontes; não mede capacidade. A verificação final passou em 5 testes/12 subtests com race e 53 testes/84 subtests sem volume, com apenas o skip esperado da célula não selecionada. Vet, gofumpt e lint pinado passaram. As 31 verificações do Make passaram com os flags padrão e `.SHELLFLAGS=-c`; mutants do caminho relativo e da remoção de pipefail falharam. Cleanup limpo. SIGINT, timeout externo real e volume de 120 s ainda não foram exercitados. Código de produto inalterado; Task 1.2.4 aberta.

**Primeiro par canônico M4 (2026-10-17).** O par A owner/ownerless com 1 ms e outflow rodou uma vez pelo Make em `26ff3bbd0`, árvore limpa, sem overlay, race, retry ou tuning, com 60 s de aquecimento e 120 s medidos. As duas células passaram, inclusive o checker completo. Owner: 206.603 aprovações com `dbAckAt` antes da parada / 120 s = 1.721,6916667/s; 301.833 respostas nas duas fases; p99 de resposta 283,857196 ms; checker 39,391109033 s. Ownerless com 32 workers: 101.309/120 = 844,2416667/s; 148.555 respostas; p99 948,190376 ms; checker 19,985965009 s. Cada célula teve 256 aprovações no drain, fora da taxa, e nenhum outro outcome, atraso, deadlock ou publish; as 450.388 respostas são aprovações conferidas pelo checker. Configuração e postings por índice são os mesmos; só o writer muda. A ordem foi fixa, owner antes de ownerless, sem contrabalanço, em host compartilhado cujo load average de 1 min passou de 165,36 para 233,65, que não é CPU%. O p99 conta desde `scheduledAt`, inclui o drain e não testa a hipótese de 15 ms: malha fechada, atraso apenas configurado, RTT de rede não medido. São medições de contenção numa conta quente; a vantagem do owner nesta célula não ordena os writers em capacidade geral sob outra distribuição. Relatórios no diretório ignorado `reports/bench-engine/full-20261017T004326Z-Rm4jRG/`; a sessão coordenadora leu JSON e logs brutos completos. O verificador read-only independente não refutou nenhum número e corroborou o estado ownerless retido no primário e no standby; o estado owner foi apagado pelo bootstrap seguinte e depende do checker e do log. As 16 células restantes do bloco A nunca foram lançadas e ficam suspensas em favor da carga agregada definida por Fred. A Task 1.2.4 permanece aberta.

**D1 aceita (2026-10-17).** O gerador agora produz transferências uniformes entre duas contas ordinárias distintas. O mínimo de duas contas vale só para o novo mix; as cargas antigas com uma contraparte continuam válidas. RED comportamental, nove testes offline com race/shuffle e contrarian passaram; a última execução levou 29,593 s. Os mutants de endpoints iguais, conta quente, conta de tarifa e índice fora da população falharam. Seis comparações descartáveis comprovaram os mesmos draws das cargas antigas; o teste commitado não fixa hashes opacos nem testa frequência estatística. Review e contrarian não deixaram Critical/High/Medium. Delta: suporte do bench +15/-4, testes +30/-17, produto zero. Na aceitação da D1, nenhuma célula selecionava o novo mix; a D2 acrescentou essa seleção. O teste grande vai escolher cada par entre 10k contas do mesmo ledger, não repetir apenas duas contas no total. D2 integra seleção, limites e comando antes da execução. Sem alteração de escritor, decisão SQL, durabilidade ou API.

**D2, próxima unidade autorizada (2026-10-17).** Integrar uma célula distribuída por processo, selecionada por `BENCH_MATRIX=capacity`, `BENCH_WRITER=owner|ownerless` e `BENCH_PRODUCERS=256|1024|4096|8192`. Exigir esses dois eixos e recusar block/delay/shape nesse modo; quick/A recusam o eixo novo. Cada célula mantém 10k contas ordinárias, dois endpoints distintos por transferência, fee desligada, um ledger, 1 ms downstream, batch 256 e os writers atuais de 1/32 workers. Todas usam o label `capacity/uniform`, aquecimento de 60 s e medição de 120 s. Os caps são 3.840.000 e 7.680.000 tentativas; atingir um cap invalida a janela, não limita silenciosamente a carga nem comprova 64k TPS.

D2 preserva o protocolo financeiro, a captura integral e todos os checks. Apenas capacity recebe checker de 30 min, Go de 43 min e limite externo de 2.640 s com kill-after de 30 s. São limites declarados antes do run, não previsão por extrapolação. Quick/A conservam seus limites. Acrescentar `shared_buffers`, `max_connections` e `work_mem` à leitura de configuração existente, sem alterar o Postgres. Arquivos: `cells_test.go`, `bench_test.go`, `measure.go`, `mk/tests.mk`, README e este plano. Um autor integra Go/Make/docs; code review e logic review rodam em paralelo, seguidos do contrarian. Testar seleção/rejeição, finalização do relatório, regressão sem volume e um par smoke de 1 s/2 s com checker completo. O supervisor reserva os runs canônicos para fonte commitada, enviada e congelada.

A campanha limitada terá até oito células, alternando a ordem dos writers entre patamares. Um serviço transitório local já comprovou limite de memória de 64 GiB, propagação de erro e término por prazo; não exercitou OOM. Antes de cada célula, exigir 128 GiB disponíveis e 512 GiB livres no filesystem do Docker. Esses checks não reservam recursos. O processo usa o GC padrão; o consumo do Postgres fica fora do limite do processo. O runner exclusivo e os checks de stack impedem reset concorrente. Qualquer falha, OOM ou timeout para a campanha e preserva o último estado para diagnóstico, sem retry automático. O checker pode exceder o limite em volume ainda não medido; nenhuma aprovação vira resultado verificado nesse caso.

Reportar taxa por ACK dentro da janela, p99, concorrência, ocupação dos lotes, custos observados e resultados após a parada. Conta distribuída reduz a concentração, mas não elimina conflitos entre lotes. Não atribuir causa a uma taxa estabilizada sem evidência de locks, CPU ou replicação. Essas células medem throughput fechado por configuração, não o máximo do ledger nem sustentação sob chegada aberta. A confirmação aberta será definida a partir dos resultados. Na preparação da D2, nenhuma célula canônica distribuída tinha sido executada; os smokes e a campanha estão registrados abaixo. A Fase 2 avança em paralelo por autorização posterior de Fred, sem dispensar estas provas.

**D2 aceita (2026-10-17), após reviews, contrarian e verificação final.** `BENCH_MATRIX=capacity` seleciona `capacity-<writer>-p<producers>` com o label `capacity/uniform` e o mix uniforme explícito, sem fallback de shape. O JSON da célula traz `checkerTimeoutNS`, 5 min em quick/A e 30 min em capacity, usado só pelo checker e pela linha de limites. O Make exige os dois eixos, recusa os demais antes do Go e roda uma célula com `timeout -k 30s 2640` e `-timeout 43m`; quick/A mantêm argv, limites e enumeração. RED comportamental e GREEN com race/shuffle; 66 verificações do Make com stubs, inclusive um mutant que mascarava eixo inválido. Comando, método, limites e distribuição sintética estão no [README](../../tests/bench/engine/README.md#distributed-capacity-cells). Os dois reviews e o contrarian não deixaram Critical/High/Medium. A regressão sem volume passou em 54 testes e 84 subtests, com somente a célula não selecionada em skip. O par smoke via Make usou 1 s/2 s e 8.192 produtores; owner aprovou 20.482 chamadas, ownerless 19.785. O checker integral e as queries de distribuição passaram: um ledger, endpoints ordinários distintos, nenhum lançamento em hot/fee/multikey. As execuções tocaram 9.843 e 9.819 saldos; cada uma teve 8.192 aprovações após a parada medida. Não são provas de capacidade. O probe real de checker com 1 ns falhou sem relatório; o mutant com 5 min constante passou, comprovando o consumo do budget. Cleanup limpo. O supervisor removeu o lookup transitório de shape desconhecido, passando o mix diretamente. A verificação final passou em cinco testes focados com race, 54 testes e 84 subtests de regressão em 116,033 s, com somente a célula não selecionada em skip. Vet, formatação, lint pinado e cleanup passaram. Na aceitação, as células canônicas de 60 s/120 s ainda não tinham rodado.

**Campanha de capacidade (2026-10-17).** Uma campanha rodou em `ebe432877`, limpo e enviado, com `GOFLAGS` vazio e sem overlay, race ou tuning, num serviço transitório com teto de memória de 64 GiB, sem swap e OOM em grupo. A escada fixa era owner 256, ownerless 256, ownerless 1024, owner 1024 e depois os dois writers em 4096 e 8192. Três células concluíram, uma foi interrompida e quatro nunca começaram. Relatórios no diretório ignorado `reports/bench-engine/capacity-campaign-20261017T024350Z-tkWPGS/`.

| Writer | Produtores | Aprovações na janela | Chamadas conferidas | Taxa na janela /s | p99 de resposta ms | Checker s |
|---|---:|---:|---:|---:|---:|---:|
| owner, 1 batcher | 256 | 193.667 | 284.665 | 1.613,8916667 | 294,480372 | 40,285921484 |
| ownerless, 32 | 256 | 612.522 | 900.438 | 5.104,35 | 240,617023 | 117,767281636 |
| ownerless, 32 | 1024 | 364.316 | 497.125 | 3.035,9666667 | 1.723,694592 | 67,513649706 |

A taxa divide aprovações distintas com `dbAckAt` antes da parada pelos 120 s; não soma lançamentos por conta nem chamadas das duas fases. O checker conferiu todas as 1.682.228 chamadas capturadas no aquecimento e na medição; esse total não é taxa. As 256, 256 e 1.024 aprovações após a parada, iguais aos produtores, ficaram fora da taxa, não do p99. Todas as respostas foram aprovações, sem unknown, expirada, recusa, replay, conflito ou atraso, e sem deadlock nem publish. Cada célula saiu com processo e cleanup zero e JSON validado pelo runner. O maior valor observado, 5.104,35/s, fica abaixo da referência de 8k; subir o ownerless de 256 para 1024 produtores não aumentou o throughput observado. A causa não foi isolada: CPU/memória do Postgres, lock wait, fsync do standby e RTT não foram medidos, então nada atribui a taxa a locks, Postgres ou host. Cada célula rodou uma vez, em ordem fixa, com GC padrão, captura pré-alocada e host compartilhado com load average de 1 min de 177,53 antes da primeira célula e 192,15 antes da quarta. São observações em malha fechada, não máximo, taxa sustentada sob chegada aberta nem ordenação geral dos writers.

Na janela medida, os workers retiraram 1.413, 261.829 e 57.098 lotes, com 193.923, 612.777 e 365.339 tentativas; os maiores lotes tiveram 256, 224 e 256 tentativas. São retiradas da fila, não aprovações. Os dados observados de WAL, GC e CPU do processo permanecem no JSON de cada célula, sem atribuição causal.

O quarto run, `capacity-owner-p1024`, começou às 02:56:54Z com stack limpa e passou pela guarda de memória disponível. A guarda não reserva memória. Às 02:59:37Z, um OOM global do host encerrou o teste, com pico de 7,55 GiB no serviço, abaixo do teto de 64 GiB. Não houve diagnóstico do processo que esgotou o host. O log não identifica a fase no instante da interrupção; a captura em memória se perdeu, sem verificação completa nem taxa aceita. Nada foi repetido depois da falha. O cleanup removeu apenas o atraso residual de replicação; naquela etapa, não apagou o ledger retido para diagnóstico. A stack ficou CLEAN às 03:02:11Z. Antes dos smokes de instrumentação posteriores, esse estado foi arquivado e verificado; agora existe somente no dump junto aos relatórios da campanha, sem restore executado. A revisão independente confirmou os números e a classificação do OOM. [Evidência e limites](../../tests/bench/engine/README.md#capacity-campaign). Confirmação aberta, failover e PG16 continuam sem execução.

**Implementation vision vigente:** usar a parada por duração e as séries preparadas na M1, sem cancelar COMMIT na parada normal. O guard de cada fase limita o join; Close tem seu limite próprio. `toxiproxy.go` controla somente `pg-repl`; erro invalida a célula. Registrar atraso downstream configurado, round trip SQL observado e confirmação SQL separadamente; RTT de rede permanece não medido. M4 implementa quick e A com os limites acima. B/C/V abaixo preservam o escopo planejado, mas seus comandos e bounds exigem elaboração contra o código antes de executar. Identidades do aquecimento ficam fora da taxa, nunca fora do checker.

| Bloco | Células | Conteúdo |
|---|---|---|
| A | 18 | owner/ownerless × atraso downstream 0/1/10 ms × outflow/inflow/mixed, malha fechada |
| B | 18 | Mesma matriz, malha aberta a 8.000/s |
| C | 20 | RTT-alvo 1 ms, outflow: owner batch 1/8/64/256; ownerless writers 1/4/32 × mesmos batches; esgotamento e feeLeg para ambos |
| V | 8 | PG16/PG17 × owner/ownerless × aberta/fechada; RTT-alvo 1 ms, outflow, batch 256, ownerless 32 writers |
| D | 4 opcionais | Dois escritores × aberta/fechada em RDS temporário autorizado |

**Qualificação após a correção do alvo (2026-10-17).** Os blocos acima preservam o desenho anterior de contenção, não um gate vigente de 8k por conta. A busca de capacidade agregada de um ledger tem prioridade e exige reelaborar suas células e limites antes da execução. O fixture distribuído é uniforme em 10k contas. D2 fixa os quatro patamares e os limites acima; a campanha canônica concluiu três células, registradas acima. Correção funcional, falhas e failover não mudam.

A/B/C somam 56 células planejadas. M4 executa `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBenchHotAccount$'` uma vez por célula. Full exige bloco A explícito e usa Go 18 min/externo 1140 s; quick conserva Go 15 min/externo 960 s. Os antigos limites de 270 s por célula e 90/100/45 min por bloco foram substituídos: omitiam checker e cleanup. Sem cache, retry ou aumento de budget depois de falha. Os comandos B/C/V/D não estão implementados.

Em V, parametrizar imagem PG16/17, capturar `SELECT version()` e digest real, hardware e configs; volumes separados por major, nunca abrir volume 17 com binário 16. Rodar versões sequencialmente, mesma fixture e índices até 46, sem multiplicar toda a matriz nem declarar equivalência com RDS. Arquivos compose/env/bootstrap da Epic 1.1 recebem essa parametrização.

Por célula: aprovações duráveis, outros outcomes, p50/p99/p999, lote real, locks, backlog/drain, WAL/FPI, checkpoints, flush lag, CPU/GC. Habilitar e registrar `track_wal_io_timing=on`, `track_io_timing=on` e `wal_sync_method`; medir deltas de `pg_stat_wal` como `wal_sync`/`wal_sync_time` conforme método e versão. Capturar stats_reset; contador indisponível ou sem temporização habilitada é "não medido", nunca zero inventado. Não chamar fsync de tempo fim a fim de replicação. Mesmo outbox, índices e política nos escritores; backlog crescente torna candidato inelegível. Saída de M4 em `reports/bench-engine/<run>/`: proveniência, log e JSON aceito por célula. O diretório já é ignorado pelo git; métricas ainda não coletadas continuam explícitas.

Make usa a stack já iniciada e defaults de DSNs loopback. Não cria, para nem remove containers; cada teste guarda o reset do schema descartável. D exige autorização separada, acesso e instância não produtiva. Células locais não medem HTTP, Tracer nem Mongo; transporte fica na Epic 1.4.

**Files:**
- Create: `tests/bench/engine/toxiproxy.go`, `tests/bench/engine/bench_test.go`
- Modify: `tests/bench/engine/docker-compose.yml`, `tests/bench/engine/env.go`, `tests/bench/engine/bootstrap.go`
- Modify: `mk/tests.mk`; relatórios reutilizam `reports/`, sem alteração de `.gitignore`

**Verification:** `BENCH_MATRIX=quick make test-bench-engine`; lint pinado com `--build-tags bench`. Após aceitação, `BENCH_MATRIX=full BENCH_BLOCK=A BENCH_DELAY_MS=1 BENCH_SHAPE=outflow make test-bench-engine` executa o primeiro par de 60 s/120 s em processos separados. O par passou em 2026-10-17. Não executar o A restante: a prioridade passou à carga distribuída de um ledger, não por falha do par. B/C/V dependem das unidades seguintes. A seleção normal de tags exclui bench. `make test-gate-selection` apresentou dois erros preexistentes em arquivos intactos, registrados no GAPS privado; não é evidência verde de M4. Nenhum teste parseia docs.

**Done when:** baseline PG16/17 e blocos registrados ou explicitamente não medidos, com versões, limites, custo de índices e métricas suportadas.

### Epic 1.3: Cenários de falha

**Goal:** provar identidade, conservação e durabilidade nas falhas locais, e medir takeover.
**Scope:** `tests/bench/engine/`.
**Dependencies:** Epic 1.2 e seus correctness gates.
**Done when:** falhas passam 20 rodadas por escritor aplicável, pelo mecanismo de repetição declarado em cada tarefa; limitações de HA ficam explícitas.
**Status:** Doing

#### Task 1.3.1: COMMIT incerto e barreira de replay

- [ ] Done

**Context:** perder resposta ou cancelar a espera síncrona pode deixar resultado local visível sem prova de flush no standby. A chave resolve duplicação; a barreira WAL resolve a confirmação da durabilidade.

**Implementation vision:** `TestLostCommitAck` roda owner e ownerless em comandos separados. Cada um tem 20 rodadas internas com rebuild e reset, em ordem A, C, B. Usar sinais observados de backend/replicação e ganchos limitados, não sleeps como prova.

- A, resposta perdida: segurar downstream em `pg-client` com toxic `timeout=0`, deixar o contexto SQL expirar, exigir `unknown`, remover toxic e resolver com a mesma identidade. Verificar o caminho real do pgx, inclusive cancel e notices, sem supor COMMIT aplicado ou abortado.
- C, standby ausente: parar standby, sincronizar o teste até o backend entrar na espera síncrona e expirar o contexto. Observar se o resultado local ficou visível. Tentar replay ainda sem standby; o touch WAL deve bloquear ou retornar `unknown`, nunca aprovação durável por simples SELECT. Restaurar standby e só então confirmar o resultado original pela barreira. Enquanto o original não estiver resolvido, nem `expired` nem recusa financeira o substituem.
- B, primário morto: matar o primário, promover standby e verificar imediatamente que todas as identidades antes confirmadas sobreviveram. O promovido inicialmente sem standby só prova sobrevivência dos commits anteriores; não emitir novas aprovações duráveis nessa janela. Recriar réplica a partir do novo primário, confirmar configuração síncrona e então resolver `unknown`. O primário antigo nunca volta como escritor independente.

Comparar o conjunto de IDs que receberam aprovação durável, não apenas contagens: todos devem existir com o resultado e a transação correspondente. Verificar no máximo uma aplicação por identidade, conservação, outbox e resolução de `unknown` após restauração dentro do orçamento. Repetir inclui replay depois do deadline e cancel da espera síncrona com resultado local visível. Registrar apenas os resultados observados no ambiente experimental.

Controle negativo, uma rodada por escritor: desligar espera síncrona e bloquear replicação, obter uma aprovação local cujo WAL o standby comprovadamente ainda não recebeu, matar primário e promover. O detector deve apontar a identidade perdida; atraso arbitrário sozinho não garante essa janela. RDS force-failover é variante D separada, apenas no ambiente temporário autorizado. Sem ela, comportamento RDS segue não medido.

**Files:**
- Create: `tests/bench/engine/failover_test.go`
- Create: `tests/bench/engine/compose.go`

**Verification:** `BENCH_WRITER=owner go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestLostCommitAck$' -timeout 180m -v`, depois ownerless. Orçamento por rodada: rebuild 120 s, A 60 s, C 90 s, B 240 s; 20 × 510 s = 170 min, mais até 5 min de controle negativo e margem. Estouro é falha, não redução silenciosa de rodadas.

**Unidade A, resposta perdida (2026-10-17, 20 rodadas canônicas por escritor concluídas):** só para A, `TestOwnerlessLostCommitAck` e `TestOwnerLostCommitAck` substituem `TestLostCommitAck`, o loop interno e o orçamento agregado por rodada. A repetição será externa, `-count=20 -failfast` por teste, após o commit aceito; C e B mantêm a visão acima. Só o escritor atravessa `pg-client`, sob `remote_apply`; setup, locks, polls e checker usam conexões diretas. Com o passe financeiro retido no lock de outbox, o replay pausado e o COMMIT observado em SyncRep, o teste instala `timeout=0` downstream apenas no `pg-client` verificado e retoma o replay. O servidor fica idle e o standby já tem a transação enquanto o contexto do próprio passe segue vivo; um QueryTracer de teste captura esse contexto no finalize, o mesmo de BEGIN e do COMMIT do pgx. O escritor responde `unknown` no orçamento de 10 s, `Close` carrega `context.DeadlineExceeded` e o backend sai pelo proxy. Após remover só o toxic nomeado, o ownerless reconecta pelo proxy e o replay após o cutoff responde `approved` sem mover dinheiro. O owner perdido não executa SQL; após o lease expirar, o sucessor, também pelo proxy curado, readquire, recarrega, faz o replay e aprova sobre as versões deixadas pelo COMMIT. O checker completo reconcilia respostas capturadas, identidades, lançamentos e baseline, sem publish.

Evidência de desenvolvimento, não canônica: autor e contrarian passaram uma rodada com race por teste; a regressão sem volume passou 62 testes e 84 subtests, com o skip esperado da célula opt-in; vet, gofumpt e lint v2.14.0 limpos; stack CLEAN antes e depois. Reviews code/logic/security/perf e contrarian terminaram sem Critical/High/Medium. Oito mutantes incorretos falharam. Dois expiraram o contexto SQL real antes da observação, enquanto a captura da resposta continuava bloqueada; o guard de contexto detectou ambos. O controle que usava apenas captura vazia passou indevidamente, demonstrando o falso positivo evitado. O mutante de retry após COMMIT não rodou. O ambiente tinha statement_timeout e transaction_timeout zerados, verificados; a prova não cobre outros valores. A verificação final dos ajustes de cleanup e check pós-Close passou em 22,299 s com race; vet, gofumpt e lint passaram, sem mudança nos hashes durante o run e com cleanup CLEAN. O run canônico passou 20 rodadas de cada teste no commit `736d4b5d90d9a6a727a7e58a1f40bb3c0e2fca21`, em 421,278 s, sem race, overlays ou retries. Hashes dos fontes coincidiram antes/depois; cleanup CLEAN, standby1 streaming sync, zero waiters SyncRep, backends externos e toxics. O processo usou GOMAXPROCS=4, GOMEMLIMIT=4GiB, cgroup de 8 GiB sem swap e pico de 208.904.192 bytes. Go timeout de 15 min e timeout externo de 930 s foram definidos antes do run. Evidência local: `/tmp/balance-engine-phase1.A5lpaQ/lost-ack-canonical.GU7U52/`. Continuam sem prova: perda antes do COMMIT, C sob `on`, promoção B, controle negativo, RST/half-open, cancel via TLS, latência do replay, RDS, comparação PG16/17, reload de 1M, transportes e soak.

**Unidade C, standby ausente sob `on` (2026-10-17, 20 rodadas canônicas por escritor concluídas):** `TestOwnerlessStandbyLoss` e `TestOwnerStandbyLoss` substituem a implementação e a repetição previstas acima somente para C. O teste verifica `synchronous_commit=on` em cada conexão do escritor. Desabilita apenas o proxy `pg-repl` verificado, exige zero WAL senders e libera o passe financeiro retido no lock de outbox. O COMMIT espera em SyncRep e responde `unknown` no budget de 10 s. O original fica visível no primário e ausente no standby. Ownerless: o retry após cutoff escreve somente o touch daquela identidade, espera em SyncRep e continua `unknown`. Owner: o owner perdido não faz SQL; a aquisição durante a queda deixa epoch 2 local, mas não retorna owner. A assertion distingue o COMMIT da aquisição de uma renovação posterior. Após restaurar e expirar esse lease, um sucessor novo assume epoch 3. Nos dois caminhos, o replay após restauração aprova o original sem mover dinheiro, com reconciliação completa e sem publish.

O cleanup restaura somente `pg-repl`, com contexto novo e limite total de 120 s para Enable e espera de replicação. Não repete restauração falha. Os budgets do protocolo não mudaram. A referência anterior de 90 s não cobre o pior caso de cleanup. Sem slot e com `wal_keep_size=0`, reciclagem de WAL durante uma queda longa pode impedir a restauração.

Autor: race e regressão sem volume passaram, com 65 testes, 86 subtests e um skip opt-in esperado. Quatro reviews terminaram sem Critical/High. Main corrigiu quatro comentários acima do ruler, retirou duas observações duplicadas e fortaleceu a attribution da aquisição. A verificação final passou com race em 37,351 s, seis sessões em `on`, vet/format/lint v2.14.0 limpos, fontes inalterados, dump preservado e pre/post CLEAN. Os mutantes de SELECT-only, WAL sem touch, aprovação prematura e corte omitido M2a falharam nos guards esperados. Os dois mutantes finais de aquisição sem durabilidade também falharam após compilar. M2b, controle adicional que omitia corte e observação, falhou no guard, mas registrou um backend de standby não identificado no post-state. Esse run continua incompleto, sem retry; o CLEAN posterior não explica sua causa. Evidência: `/tmp/balance-engine-phase1.A5lpaQ/standby-loss-author.SBbvtf/`, `standby-loss-contrarian.VbXidn/` e `standby-loss-final-verifier.Sfl1hS/`. A auditoria independente conferiu os logs anteriores; o run final cobre os ajustes posteriores.

**Canônico C aceito:** 20 rodadas por escritor passaram em 801,270 s no commit limpo `0299b72f71b709887d4a36afc3bb28a73916a004`, com `-count=20 -failfast -p 1`, sem race, overlays ou retries. As 120 conexões de escritor registraram `synchronous_commit=on`. Os hashes coincidiram antes/depois, o dump permaneceu íntegro e o checker completo passou. Pre/post CLEAN: standby1 streaming sync, zero waiters SyncRep, backends externos, toxics e test binaries. Limites definidos antes do run: Go 25 min, externo 1560 s com kill grace 30 s, service 1980 s; cgroup 8 GiB sem swap, GOMAXPROCS=4 e GOMEMLIMIT=4GiB. Pico medido 222.978.048 bytes, swap zero. Evidência local: `/tmp/balance-engine-phase1.A5lpaQ/standby-loss-canonical.D6gKex/`. Permanecem sem prova: parada do processo do standby, half-open, renovação ociosa sob queda, `remote_write`/quórum, timeouts de servidor não zero, latência de reconexão, promoção B e controle negativo. A Task 1.3.1 continua aberta.

**Done when:** 20 rodadas por escritor passam, controle negativo detecta perda e replay não confirma durabilidade durante ausência da réplica.

#### Task 1.3.2: Lease expirado e locks em voo

- [x] Done

**Context:** dono pausado antes do BEGIN e dono que já mantém lock SQL são situações diferentes.

**Implementation vision:** `TestStaleOwnerFenced` suspende a renovação de A enquanto observa seu passe de aquecimento dentro de SQL. O passe termina e aprova; A fica ocioso na fronteira entre lotes. Após expiry observado pelo banco, B adquire novo epoch, recarrega e aprova 1.000 transferências. Só então A recebe o lote stale, com o claim dessa identidade retido pelo teste. O UPDATE de renovação deve afetar zero linhas antes do INSERT de claim, retornando a causa de fence recusada, não timeout. O checker reconcilia 1.001 aprovações e um unknown, sem persistência financeira do lote stale.

Essa ordem substitui a pausa em `beforeBegin`: colocar aquisição, reload e 1.000 operações de B dentro do orçamento de 10 s de A era uma premissa não medida. Não aumenta o budget nem reduz as 1.000 operações. O teste prova fence antes do claim, não uma pausa dentro de um lote já retirado da fila atravessando takeover.

Estender `TestOwnerWriterLease`, que já observa o takeover esperando somente o lock de fence do passe em voo. Trocar o adquirente isolado por um sucessor real com application_name distinto. Após o COMMIT de A, B adquire e aprova sobre um saldo que A alterou, provando reload após aquisição. Isso realiza a intenção de `TestOwnerLockSerialization` sem duplicar o teste. Uma transação que já detém o lock não perde validade retroativamente.

Executar vinte rodadas via `-count=20 -failfast`, com reset e waits existentes limitados; sem loop interno ou retry automático. A suíte normal roda uma vez. Os correctness gates da Epic 1.2 já concluídos permitem esta unidade enquanto a medição 1.2.4 continua aberta.

**Files:**
- Create: `tests/bench/engine/failover_test.go`, pois a Task 1.3.1 ainda não criou esse arquivo
- Modify: `tests/bench/engine/writer_owner_test.go`, `tests/bench/engine/README.md`

**Verification:** `go test -count=20 -race -failfast -tags bench ./tests/bench/engine/ -run '^(TestStaleOwnerFenced|TestOwnerWriterLease)$' -timeout 20m -v`. A rodada canônica usa o mesmo comando sem `-race`, no commit aceito. Timeout externo de 1.260 s.

**Status (2026-10-17):** Done. O comando canônico passou 20 rodadas de cada teste no commit `1d606ac28224a80dd49b119f125704911b12860b`, em 221,052 s, sem race, overlays ou retries. Os hashes dos fontes versionados coincidiram antes/depois. A stack terminou CLEAN, com standby1 streaming sync, sem backends externos, waiters SyncRep, toxics ou binários de teste. O processo usou limite de 8 GiB, sem swap, com pico reportado de 211,6 MiB. Evidência local: `/tmp/balance-engine-phase1.A5lpaQ/owner-fault-canonical.z4dKjv/`.

Reviews code/logic/security e contrarian concluíram sem Critical/High/Medium. Autor e contrarian passaram uma rodada com race cada; o checker conferiu 1.001 aprovações e um unknown no teste stale, e três aprovações e dois unknown no teste de lock/reload. A regressão sem cargas de volume passou 60 testes e 84 subtests, com um skip esperado da célula opt-in. Vet, gofumpt e lint v2.14.0 passaram. Overlays independentes detectaram renovação ausente ou após claim, predicados enfraquecidos e reload anterior à aquisição. Um probe sem fence e sem claim retido confirmou que o cache não mascara a falta da fence: A aprovou indevidamente. Nenhuma dessas provas mede failover do Postgres, tempo de reload ou capacidade.

**Done when:** takeover prematuro e renovação de lease expirado são recusados; concorrência com lock em voo termina sem deadlock do próprio teste.

#### Task 1.3.3: Reload de 1M saldos e controles

- [x] Done

**Context:** medir reload informa a definição 3, mas não escolhe orçamento público nem prova controle concorrente de produto.

**Implementation vision:** `TestTakeoverReload`, `BENCH_BALANCES=1000000`, semeia 1M saldos ordinários e controles, mais extras declarados. Mede aquisição, páginas, renovação, primeiro lote e RSS. Cada renovação exige lease válido pelo relógio do banco; tempos do cliente não são margem de lease. Chave de paginação de controle é composta, sem perder contas entre organizations/ledgers. Falha de página ou renovação aborta tudo. No bench, nenhum writer externo altera controles durante o reload; no produto, snapshots e ordenação de TODOS os writers precisam da prova da Epic 3.2. Lazy loading é alternativa a estudar caso o número não sirva, não fallback aprovado.

**Status (2026-10-17):** Done. One canonical run on clean `d2aa5f63e9cce72958a2981572348b527323da44` (Go 1.27.2, PG 17.11), with `-failfast -p 1` and no race, overlay or retry, passed. The constructor (acquisition, 21 + 21 pages over 1,000,004 balances and 1,000,003 controls, renewals) took 6.830609745 s, 1.8306 s over the 5 s experimental reference; the seed is excluded. The first transfer answered in 7.502147 ms; the full checker passed with zero publishes. It was the first run with hard container caps (8 GiB per PG container, 256 MiB toxiproxy), beside the 8 GiB client cap and no swap, and is not compared with earlier uncapped runs. Pre/post CLEAN; sources, retained dump and containers unchanged. One warm run: no cold restart, failover, product API, MT, concurrent writer, live race detection, failure-path trace or throughput evidence. The miss informs definition 3 and sets no public budget. Table and limits: [bench README](../../tests/bench/engine/README.md#owner-1m-reload-task-133); evidence `/tmp/balance-engine-phase1.A5lpaQ/reload-1m-run.20261017T141838Z/`.

**Files:**
- Modify: `tests/bench/engine/failover_test.go`

**Verification:** `BENCH_BALANCES=1000000 go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestTakeoverReload$' -timeout 30m -v`, uma rodada completa, com tempo por página e RSS.

**Done when:** números registrados contra referência experimental de 5 s, sem esconder aquisição, renovação ou primeiro lote.

### Epic 1.4: Chamada em processo, gRPC e Redpanda

**Goal:** comparar in-process, salto gRPC real e Redpanda no mesmo autorizador/escritor elegível, batching, payload e Postgres; testar separadamente a hipótese D contra E.
**Scope:** `tests/bench/engine/`.
**Dependencies:** Epics 1.2 e 1.3. Fixar escritor/configuração; sem correção, nenhum transporte é candidato viável.
**Done when:** matriz separa ACK, SQL e resposta; pendências sobrevivem; admissão/pre-COMMIT funcionam; D registra divergências sem reexecutar dinheiro.
**Status:** Pending

#### Task 1.4.1: Broker local e ownership de partição

- [ ] Done

**Context:** Redpanda `v24.2.7` e franz-go v1.21.6 já constam do repo. O bench usa um tenant, não define grupos multi-tenant do produto.

**Implementation vision:** perfil `transport` acrescenta `bench-redpanda`, um nó, RF1, portas loopback 19092 e 19644. `kadm` cria `engine-intents` e `engine-replies`, cada um com uma partição; `foreign-events` tem oito. RF1 não prova sobrevivência à perda do broker.

Tabela bench-local `engine_consumer_offset` com `(tenant_id, topic, partition)` como PK, `next_offset`, `owner_id` e `epoch`. Em CADA assignment, o novo consumidor adquire ownership SQL sob lock, incrementa epoch e lê `next_offset` antes de consumir. Revocation interrompe trabalho local; só a aquisição SQL altera o epoch. Cada lote trava essa linha no início e exige owner/epoch/offset esperado, avançando `next_offset` na mesma transação financeira. Um antigo consumidor antes do BEGIN falha após takeover; se já detinha o lock, o takeover espera seu término.

`next_offset` é o próximo registro: depois de commit, o lote anterior não será reentregue por esse seek. A recuperação da resposta deve vir do outbox, não de republicação artificial da intenção. Fencing de partição existe nos dois escritores em B, mas não obriga o algoritmo financeiro a ser owner. Configuração de grupos, assignments e isolamento de tenants do produto depende da definição 8.

**Files:**
- Modify: `tests/bench/engine/docker-compose.yml`, `tests/bench/engine/schema_bench.sql`
- Modify: `tests/bench/engine/bootstrap.go`, `tests/bench/engine/env.go`

**Verification:** `docker compose -f tests/bench/engine/docker-compose.yml --profile transport up -d --wait --wait-timeout 120`; `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBootstrapTransport$' -timeout 5m -v`. Duas inicializações internas verificam tópicos, offset inicial e aquisição de epoch.

**Done when:** assignment toma ownership SQL e bootstrap repetido não reutiliza epoch de um consumidor anterior.

#### Task 1.4.2: Adaptadores equivalentes e resposta durável

- [ ] Done

**Context:** Epic 1.2 mede núcleo. Esta task mantém esse baseline in-process e mede gRPC entre processos e broker, inclusive com ownerless. Nenhuma célula equivale à API HTTP completa.

**Implementation vision:** compartilhar `authorizer.decide`, chamado só após identidade/locks, sobre estado provisório. Adaptadores não decidem dinheiro. In-process chama o escritor no processo do gerador; gRPC usa servidor separado; Redpanda usa consumidor. Mesmos payload, fila, escritor e lote. gRPC é contrato privado de bench, não fronteira de produto aprovada.

Todos executam validação não financeira e corte de entrada antes de controles/fila. Em B, antes de publish. Vencido segue apenas resolução/finalização pelo protocolo SQL sem dinheiro, não novo trabalho no broker; isso não decide sua rota pública. Duplicatas/incertos conservam identidade. Depois de esperas e antes de COMMIT, aplicar os cortes do contrato comum.

Redpanda `Submit` registra caller/tentativa antes de publish com `acks=all`. Correlação de entrega não integra fingerprint financeiro. Callers simultâneos têm canais próprios; conflito afeta só a tentativa divergente. Callback de `Produce` e espera limitada determinam ACK ou `unknown`; perda do ACK pode ocultar aceite. Callback tardio não cria nova identidade nem bloqueia aplicação. Limitar mapa de callers e limpar cancelados; ACK não aprova dinheiro.

Consumer sem auto-commit toma epoch no assignment e faz seek em `next_offset`. Lote trava ownership e exige offset esperado; claims, replay/barreira, decisão, dinheiro, resultado, outbox e avanço do offset entram na MESMA SQL. Não decidir antes do BEGIN. Rollback sem recomputação local ou COMMIT unknown encerra sessão e descarta prefetch; resolver anterior e reassumir pelo offset SQL antes do próximo lote. `PollRecords` avança cursor local: desabilitar auto-commit não rebobina. Resposta tem ID estável de entrega, payload congelado, request e correlação.

Relay aplica barreira WAL antes de publicar resposta encontrada no outbox: visibilidade local após restart não prova replicação. Confirma publish antes de marcar entregue. Crash entre publish e marca duplica entrega; receptor deduplica por ID de entrega sem perder correlação. Worker reinicia no próximo offset; relay recupera callers pendentes sem republicar intenção. Não apagar obrigação por ausência de canal em memória. Replay de novo registro gera resposta sem reaplicar dinheiro.

Medir `brokerAckAt-scheduledAt`, `dbAckAt-scheduledAt`, `repliedAt-scheduledAt`; in-process/gRPC não têm broker ACK. Confirmar mensagens só após durabilidade SQL, inclusive replay. No bench, nenhuma identidade ou dedup de consumidor é purgada durante ensaios; retenção de produto vem da definição 7.

**Files:**
- Create: `tests/bench/engine/authorizer.go`, `tests/bench/engine/transport_grpc.go`
- Create: `tests/bench/engine/transport_redpanda.go`, `tests/bench/engine/outbox_relay.go`
- Modify: `tests/bench/engine/writer_owner.go`, `tests/bench/engine/writer_ownerless.go`
- Test: `tests/bench/engine/transport_grpc_test.go`, `tests/bench/engine/transport_redpanda_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestGRPCTransportConservation|TestRedpandaTransportConservation|TestOwnerWriterConservation|TestOwnerlessWriterConservation' -timeout 45m -v`. Cinco rodadas por transporte e regressão in-process dos escritores; proteção/conservação por identidade, offset correto, outbox drenado e cortes respeitados. Cobrir rollback/unknown com cursor avançado e callers simultâneos: payload igual recebe replay, divergente recebe conflito na tentativa correta.

**Done when:** os três caminhos usam o mesmo escritor declarado; extração preserva provas locais e recuperação de resposta.

#### Task 1.4.3: Matriz, backlog e recuperação

- [ ] Done

**Context:** deadline, offset e resposta são provas distintas da medição de latência. As células a 8k abaixo herdaram o desenho da conta quente e aguardam reelaboração para o total de um ledger; as provas de backlog e recuperação permanecem independentes.

**Implementation vision:** `TestBenchTransport`, RTT-alvo 1 ms, outflow, mesmo escritor/configuração. Dez células: in-process × aberta/fechada a 8k, sem carga alheia; gRPC/Redpanda × aberta/fechada × sem/com carga alheia. Produtor real de `foreign-events`, limitado a 20k registros/s de 1 KB, registra taxa alcançada. Sem spin loop. Carga alheia mede interferência, não aprova broker compartilhado.

`TestTransportBacklog` cobre os três caminhos. Pausa de 5 s usa caller 2 s e corte interno 1,5 s; julgar cada pedido pelo corte próprio, inclusive chegadas no fim da pausa. Exercitar entrada já vencida sem controles/publish, expiração após espera e corte cruzado imediatamente antes de COMMIT com rollback integral. Replay aprovado continua aprovado; original unknown não vira expired; COMMIT enviado pode receber ACK depois do caller. Registrar cada caso, sem prometer impossibilidade de commit tardio.

Falhas de B, 20 rodadas internas por cenário, com reset:
- Worker cai após commit de dinheiro/offset/outbox e antes da resposta. Seek pula lote commitado; relay retoma callers ainda esperando sem republicar intenções.
- Relay cai após publish confirmado e antes da marca. Receptor deduplica entrega repetida.
- Transferência real da partição A→B por saída controlada do grupo. Verificar assignment; A pausado antes do BEGIN perde epoch. Variante com lock SQL: B espera término/rollback de A e só então assume o offset.
- Perder ACK de produce ou indisponibilizar broker. Caller recebe unknown limitado; callback tardio e retry conservam identidade.

Conferir IDs aprovados duráveis, pisos/flags por decisão, conservação, offset dos registros aceitos, cada caller e drain. Submit não equivale a offset: ACK perdido/retry pode produzir duplicatas. Não transformar unknown em aprovação.

**Files:**
- Modify: `tests/bench/engine/bench_test.go`, `tests/bench/engine/transport_redpanda.go`
- Modify: `tests/bench/engine/outbox_relay.go`, `tests/bench/engine/transport_redpanda_test.go`, `tests/bench/engine/transport_grpc_test.go`
- Modify: `mk/tests.mk`

**Verification:** `BENCH_BLOCK=transport make test-bench-engine` usa `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBenchTransport$' -timeout 55m -v`, 10 × 270 s = 45 min mais margem. Falhas em comando separado: `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestTransportBacklog|TestTransportRecovery' -timeout 120m -v`. Até 60 s por rodada/cenário, 20 rodadas, com setup limitado; `golangci-lint run --build-tags bench ./tests/bench/engine/`.

**Done when:** três caminhos comparáveis; cortes e rollback observados; respostas retomadas, assignment real e locks exercitados. Broker HA e HTTP completo continuam não medidos.

#### Task 1.4.4: Recibo independente contra outbox

- [ ] Done

**Context:** hipótese D do Fred separa publicação e SQL; recibo anterior ao SQL também não aprova dinheiro. Nenhuma variante está escolhida.

**Implementation vision:** mesmo escritor elegível, carga congelada de 100 identidades por rodada. Comparar D com publish assíncrono independente, D com ACK de recibo antes de tentar SQL, e E com obrigação no commit. Cada uma roda cinco cenários: entrega normal, crash no intervalo entre os dois efeitos, broker indisponível, falha SQL antes de COMMIT e perda da resposta do COMMIT. Vinte rodadas por cenário, no máximo 60 s cada. Usar ganchos de processo/rede e sinais observados, nunca CPU sintética.

Gerador registra tentativas, recibos, resultados conhecidos e tempos até ACK do broker, confirmação SQL e resposta; custo adicional é descritivo, não TPS sustentado desses lotes de falha. Após estabilização, comparar SQL autoritativo com broker e registrar ambos, commit-only, receipt-only e unknown. Ausência observada no SQL não prova aborto de COMMIT em voo. D independente permite efeitos em qualquer ordem; recibo-primeiro só restringe a ordem, sem eliminar receipt-only. Reconciliador experimental apenas relata evidência e pendências: não cria sucesso por ACK, não executa dinheiro, não compensa nem reaprova. E retoma publicação pelo outbox e exige dedup do receptor. Sem obrigação durável, processo morto pode perder a publicação de D; nenhuma conclusão de zero perda nasce só do caminho normal.

**Files:**
- Create: `tests/bench/engine/receipt_comparison_test.go`
- Modify: `tests/bench/engine/transport_redpanda.go`, `tests/bench/engine/outbox_relay.go`

**Verification:** `BENCH_RECEIPT_MODE=independent go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestReceiptComparison$' -timeout 120m -v`, repetir com `receipt-first` e `outbox`. Cada comando limita 5 × 20 × 60 s = 100 min mais setup. Registrar inclusive classificações não resolvidas.

**Done when:** relatório distingue evidência de broker/SQL, perda de entrega e incerteza; nenhuma reconciliação altera dinheiro. Resultados informam definição 8, não aprovam D.

### Epic 1.5: Corrida longa e relatório go/no-go

**Goal:** testar estabilidade dos candidatos elegíveis e apresentar recomendação, não escolher arquitetura automaticamente.
**Scope:** `tests/bench/engine/`, `docs/plans/`.
**Dependencies:** Epics 1.3 e 1.4.
**Done when:** relatório rastreável, revisão técnica e recomendação para as decisões de arquitetura ainda abertas. A implementação paralela da Fase 2 já foi autorizada.
**Status:** Pending

#### Task 1.5.1: Corrida de 30 minutos

- [ ] Done

**Context:** conta quente, outbox, checkpoints e snapshots longos afetam sustentação. A carga mixed a 8k numa conta quente abaixo é desenho anterior à correção do alvo. O soak deve usar a carga distribuída e a taxa selecionadas pela busca de capacidade agregada, ainda não executada.

**Implementation vision:** `TestSoak30m`, gate `BENCH_SOAK=1`, exige writer/transporte explícitos. Candidato elegível roda mixed, RTT-alvo 1 ms, aberta de 8k por 30 min. Cada candidato tem comando próprio. A cada 5 min, leitor `REPEATABLE READ` varre operações com snapshot limitado a 60 s. Carga real do mecanismo, nenhuma CPU sintética.

Amostras por minuto: aprovações duráveis/s, outros outcomes, p99, backlog/drain, WAL/FPI, fsync e temporização suportada de 1.2.4, checkpoints, flush lag, dead tuples/HOT/autovacuum, tamanhos de intenção/outbox. Registrar idade de `relfrozenxid`/`datfrozenxid`, configurações de freeze/wraparound e snapshots que retêm xmin quando relevantes; 30 min não provam segurança de retenção indefinida. Sem purga durante o bench, explicitar acúmulo, relay e serviços não exercitados; retenção de produto é gate 2.2, sem TTL inventado. Gates a detalhar após a busca de capacidade: taxa selecionada de aprovações duráveis/s no conjunto de contas de um ledger, backlog estável, drain limitado e p99 final ≤ 1,5× quinto minuto. Reconstrução final verifica pisos/flags por decisão, limite compartilhado, conservação e unicidade.

**Files:**
- Create: `tests/bench/engine/soak_test.go`

**Verification:** `BENCH_SOAK=1 BENCH_WRITER="$writer" BENCH_TRANSPORT="$transport" go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestSoak30m$' -timeout 45m -v`. Definir writer owner/ownerless e transporte in-process/grpc/redpanda; ausência falha. Uma rodada por candidato declarado.

**Done when:** amostras/drain completos; candidato abaixo da meta recebe no-go, sem contar recusas como aprovação.

#### Task 1.5.2: Relatório, revisão técnica e recomendação

- [ ] Done

**Context:** a recomendação depende das medições e da revisão técnica do time, não da preferência por uma topologia.

**Implementation vision:** relatório `docs/plans/2026-09-30-balance-engine-results.md` reúne matrizes, versões/configs, índices até 46, payload/outbox, proteção por decisão, multikey, cortes/COMMIT tardio, replay/WAL, lease, takeover, resposta, soak e comparação D/E. Cada número aponta célula. Separar núcleo de transporte e HTTP completo não medido, PG16/17 local de RDS, RF1 de HA. D mostra commit-only/receipt-only/unknown sem promessa de reconciliação financeira.

Antes de recomendar implementação, Felipe e Clara revisam provas, limitações e resultados dos spikes disponíveis. Registrar evidência recebida, divergências e o que permanece não verificado; ausência não vira concordância ou número inventado. Sem essa revisão, recomendação fica bloqueada.

Elegibilidade exige correção, replay/durabilidade, proteção por decisão, capacidade agregada no mesmo ledger contra a referência de mais de 8k aprovações duráveis/s e gates de backlog/latência/soak. Uma conta acima de 1k/s é referência excelente, não requisito de elegibilidade agregado. Sem candidato elegível, no-go. Entre elegíveis, ownerless a 80% do throughput owner favorece simplicidade, considerando caudas e operação; heurística não escolhe arquitetura. Redpanda precisa justificar custo contra in-process e gRPC com MESMO escritor. Publicação independente D não recebe selo zero perda só por passar caminho normal.

Comparar também transições entre processos/sistemas, pontos de falha e obrigações de recuperação. Reportar não medidos e opção permanente só Postgres. Atualizar `Phase Overview` somente após execução. As definições 1 e 12 já têm decisão explícita nos limites registrados acima; as demais continuam abertas. A Phase 2 foi antecipada por Fred, mas a revisão técnica e as provas continuam gates da recomendação final.

**Files:**
- Create: `docs/plans/2026-09-30-balance-engine-results.md`
- Modify: `docs/plans/2026-09-30-balance-engine-postgres-truth.md`

**Verification:** revisão humana da origem dos números, cobertura e limites, seguida de Felipe/Clara e confronto dos spikes. Nenhum teste parseia prosa.

**Done when:** revisão técnica registrada; Fred recebe a recomendação fundamentada para as decisões de arquitetura ainda abertas. A implementação paralela da Phase 2 não conclui esta tarefa nem autoriza ativação de clientes.

---

## Phase 2: Protocolo e adaptador experimental

Fred autorizou antecipar a implementação em paralelo às provas da Phase 1. As dependências abaixo continuam gates das entregas completas e da ativação, não impedem trabalho independente. Não habilita clientes. Create v2 isolado é incremento interno: o primeiro marco funcional inclui criar, reservar, confirmar, cancelar e estornar, com leitura e controles autoritativos.

### Epic 2.1: Especificação do protocolo e do esquema

**Goal:** definir identidade antes da decisão, lote provisório, escritores, unidade atômica/owner/partição, transporte e fronteira de deploy. Especificar admissão de entrada/pós-espera/pre-COMMIT, unknown/barreira WAL, hospedagem, replay v1/v2 ST/MT e reservas sem commit. Proibir acerto de reserva enquanto SQL unknown. Definir intenção/recusa/motivo visíveis, inclusive Tracer antes do dinheiro, separados de idempotência e recuperação; só pedidos autenticados, autorizados e estruturalmente válidos, com minimização de PII. Escolher protocolo público nesta epic, não neste plano. Desenhar particionamento com prova de unicidade global de requests/reverts, FKs, retenção e compatibilidade; UNIQUE particionado do PG exige chave de partição e não basta para guard global por original. Nenhum esquema escolhido agora.
**Scope:** `docs/architecture/`, contratos e desenho de dados.
**Dependencies:** Phase 1; definições 1, 2, 3, 4, 7, 8, 10, 11 e 12 respondidas.
**Done when:** Fred aprova protocolo e fronteiras após revisão técnica; esquema demonstra guard global/FKs e mesma-DB cross-ledger sem 2PC oculto. Sem desenho correto compatível de particionamento, bloquear para decisão explícita, não apagar a entrega.
**Status:** Pending

### Epic 2.2: Migrações e retenção do escritor

**Goal:** entregar esquema aprovado, incluindo particionamento, identidade/resultado, evidência de intenção/recusa, dívida de tarifa autoritativa, controles, outbox, fence se escolhida e guard global de revert. Não copiar DDL do bench automaticamente. Migração e retenção preservam identidade global, FKs e dados existentes. Definir limpeza de resultados resolvidos, outbox entregue e dedup dos consumidores dentro da janela de replay; unknown, reservas não resolvidas e obrigações pendentes não expiram por TTL arbitrário. Purga não pode permitir replay financeiro enquanto o contrato o protege; depois da janela vale definição 7.
**Scope:** `components/ledger/migrations/transaction/`, `tests/integration/` e manutenção de retenção.
**Dependencies:** Epic 2.1; definições 1, 2 e 7 refletidas no esquema.
**Done when:** ensaio em base existente prova compatibilidade, custo de índices, unicidade concorrente entre partições, FKs e limites de retenção antes de aplicar migração. Impossibilidade exige decisão, não substituição silenciosa por esquema não particionado.
**Status:** Pending

### Epic 2.3: Adaptador Postgres experimental para create v2

**Goal:** implementar contrato de accounting para create v2 JSON, inflow/outflow, ids de saldo e SQL da completion no commit financeiro, com outbox completo. Usar algoritmo e fronteira de processo aprovados; Tracer mantém payload Midaz. Cortar novos pedidos na entrada antes de controles/fila, resolver identidades anteriores sem reexecutar e revalidar antes da decisão/COMMIT. Recuperação só resolve evidência e entregas, nunca repete dinheiro. Timeout após envio não promete 5xx sem movimento. Instrumentar spans e métricas segundo os padrões do repo.
**Scope:** `components/ledger/internal/adapters/postgres/engine/`, `components/ledger/internal/services/command/`.
**Dependencies:** Epic 2.2; definições 2, 3, 4, 7, 8, 9, 10, 11 e 12 respondidas; revisão técnica e spikes disponíveis avaliados em 1.5.
**Done when:** create v2 experimental comprova corte, rollback pre-COMMIT, replay e unknown; 201 exige durabilidade replicada confirmada. Sem habilitação de clientes.
**Status:** Pending

### Epic 2.4: Seleção experimental e gate de tenant

**Goal:** selecionar motor só em testes/ambientes experimentais. Um tenant e os ledgers de um grupo cross-ledger não podem misturar autoridades; leitura/escrita mudam juntas após Phase 3 e barreira da Phase 4. Se usado, seletor `settings.engine` é temporário, não flip independente por ledger. Inventariar consumidores das APIs, eventos, configuração/env e upgrade antes da futura retirada; `RABBITMQ_TRANSACTION_ASYNC` não é presumido privado.
**Scope:** `components/ledger/internal/bootstrap/`, services query/command e configuração.
**Dependencies:** Epic 2.3; definição 12.
**Done when:** testes escolhem os dois motores, ativação fora do gate falha e inventário de contratos identifica qualquer quebra a submeter ao Fred antes de implementá-la. Nenhuma major ou camada de compatibilidade inventada.
**Status:** Pending

### Epic 2.5: Estudo de consultas e read model

**Goal:** entregar catálogo de queries reais, volume/índices, consistência e lag tolerável por consulta, incluindo dívida e intenções. Comparar Postgres primário/réplica com candidatos de read store, contrato de projeção via outbox, idempotência/rebuild, retenção, custo e licença. Infraestrutura extra permanece opcional; não escolher MongoDB ou ClickHouse por pressuposto. Separar projeção de consulta da autoridade financeira no mesmo commit SQL.
**Scope:** `docs/architecture/`, inventário de `services/query/` e `services/fees/`.
**Dependencies:** Epic 2.1; definições 1 e 7; evidências disponíveis da Phase 1.
**Done when:** estudo revisado por Felipe/Clara explicita queries, lag, outbox, retenção/licença e recomendação. Implementação/deploy de novo read store só com decisão posterior; o estudo não bloqueia a opção permanente só Postgres.
**Status:** Pending

---

## Phase 3: Paridade funcional, leitura e visibilidade

Cobrir contratos existentes e intenção/recusa observável. Nenhuma tarefa detalhada nesta fase antes de chegar à execução.

### Epic 3.1: Lifecycle e contratos existentes

**Goal:** preservar v1/v2 ST/MT em create, pending/commit/cancel, revert, grupos e replay: IDs, ordem, status, corpo congelado, scoping e skips por versão. NOTED recebe persistência/resposta equivalentes antes de retirar `write_transaction`. Grupo cross-ledger no mesmo banco cabe inteiro na unidade SQL e no owner/partição que o ordena; atravessar shards exige outra decisão, sem 2PC implícito. Revert mantém guard global; recusa Tracer não cria lançamentos.
**Scope:** services command, adaptadores do motor e contratos HTTP.
**Dependencies:** Phase 2; definições 4, 7 e 11.
**Done when:** lifecycle, grupos, NOTED e replay passam nas duas versões e modos de tenancy; resultado incerto preservado.
**Status:** Pending

### Epic 3.2: Todos os writers, dívida de tarifa e Tracer

**Goal:** ordenar block/unblock, exceções, limites, PATCH flags/settings, criação de saldo/overdraft, close/delete e demais writers, ou invalidar estado transacionalmente. Provar reload de saldos E controles com concorrência real. Criação, quitação parcial/total e reversão de dívida têm fonte financeira no MESMO commit SQL dos movimentos; projeção Mongo e queries podem permanecer via outbox, sem decidir cobrança a partir de dado atrasado. Tracer como módulo mantém payload Midaz e resolve tentativas sem commit, SQL unknown e acertos pós-commit duravelmente; TTL e fila volátil não bastam. Não mover cotas para fees sem definição.
**Scope:** adaptadores postgres/engine e mongodb, services command/query/fees, seam Tracer e writers administrativos.
**Dependencies:** Epic 3.1; definições 1 e 10.
**Done when:** inventário de escritores fechado; concorrência, restart e projeção atrasada não perdem nem quitam duas vezes a mesma dívida, nem violam saldo, flags ou limites. Reservas incertas sobrevivem sem depender só de TTL/fila em memória.
**Status:** Pending

### Epic 3.3: Batch atômico e account protection

**Goal:** batch v2 e grupos resolvem identidade antes da decisão, ordenam locks e descartam o lote inteiro em conflito/corte pre-COMMIT. Preservar replay congelado, versões por balance_id, multikey, fees e lifecycle. Validar piso de cada classe elegível e limite compartilhado por decisão aprovada, inclusive concorrência, sem impor CHECK que proíba overdraft legítimo.
**Scope:** services command e adaptador Postgres.
**Dependencies:** Epic 3.2; definições 7 e 11.
**Done when:** batch, duplicatas, multikey, proteção e deadline passam; nenhuma decisão usa crédito removido do lote; conservação final não substitui validação por posting.
**Status:** Pending

### Epic 3.4: Autoridade de leitura e chaos

**Goal:** saldo, transação, operações e replay dos tenants migrados usam Postgres, sem seed Valkey ou autoridade mista. Alias resolve na borda. Preservar point-in-time e corpo congelado; metadata e projeção de dívida têm lag/consistência definidos. Validar v1/v2 ST/MT, NOTED, retenção/replay e falhas ponta a ponta. Novo read store não entra sem decisão posterior ao estudo 2.5.
**Scope:** services query, contratos HTTP, `tests/integration/`, `tests/chaos/`.
**Dependencies:** Epic 3.3; definições 7, 10 e 11 implementadas; estudo 2.5.
**Done when:** integração/chaos provam leitura, replay e paridade. Impacto no Product Console registrado; mudanças consumidas pelo Console exigem PR pareado e validação integrada, não mock.
**Status:** Pending

### Epic 3.5: Intenção e recusa visíveis ao cliente

**Goal:** implementar protocolo de 2.1 para pedidos autenticados, autorizados e estruturalmente válidos: intenção, resultado e motivo de recusa, inclusive Tracer antes da autorização financeira. Preservar evidência sem transação fictícia, retenção arbitrária de PII ou confusão entre recusa, unknown e reserva pendente. Visibilidade não depende da existência de lançamento nem apenas do slot de idempotência. Sem rota/schema público escolhido neste plano.
**Scope:** services command/query, contratos HTTP e `LerianStudio/product-console`.
**Dependencies:** Epics 2.1, 2.2 e 3.4; definições 7, 10 e 11.
**Done when:** operador do cliente vê intenção/recusa/motivo mesmo com zero lançamentos; replay/restart preservam resultado e tenancy. Backend e Console têm PRs cruzados, fluxo integrado em stack real com Playwright e merge pareado; indisponibilidade da evidência segue o protocolo aprovado, nunca inventa recusa financeira.
**Status:** Pending

---

## Phase 4: Cutover por tenant

Ambiente, acesso, dados, janela e execução exigem autorização específica. Este plano não autoriza produção.

### Epic 4.1: Barreira e reconciliação

**Goal:** pausar TODOS os writers financeiros/controles, incluindo API, administração, consumers, recovery e sync; drenar trabalhos antigos e resolver unknown. Reconciliar saldos, operações, pending, dívida, exceções, intenções/recusas e replay contra trilha/evidência, inclusive formatos históricos. Igualdade Valkey/SQL ou resultado ok do reconciler de head não prova ausência de fork. Inconsistência bloqueia tenant para investigação, sem reparo automático. Após prova e contrato de hospedagem, importar estado/identidades/janelas de retenção e mudar leitura/escrita de todos os ledgers relacionados, sem autoridade mista.
**Scope:** `components/ledger/cmd/cutover/`, adaptadores redis/postgres e controle de writers.
**Dependencies:** Phase 3 completa, incluindo 3.5; definições 1, 2, 3, 5, 7, 8, 10, 11 e 12 atendidas.
**Done when:** ensaio prova barreira, reconciliação além do head, dívida SQL, replay retido, bloqueio do inconsistente e flip conjunto. Resultado desconhecido não desaparece no corte.
**Status:** Pending

### Epic 4.2: Ensaio em homologação

**Goal:** executar em ambiente autorizado, dados de teste ou cópia sanitizada autorizada e durabilidade aprovada. Demonstrar retorno seguro antes do primeiro commit novo; depois dele, não reabrir Valkey antigo como autoridade. Retorno posterior exige reconciliação e decisão operacional, não re-seed automático. Incluir migração/partições e consumidores/configuração no ensaio de upgrade.
**Scope:** homologação e runbook.
**Dependencies:** Epic 4.1; identificar e autorizar ambiente, dados e execução. A leitura de infraestrutura de 06/10 não localizou RDS de ledger fora de produção; não presumir homologação disponível.
**Done when:** registrar indisponibilidade, drain, falhas e recuperação com evidência; contrato de upgrade preservado ou quebra demonstrada e aprovada antes da execução.
**Status:** Pending

### Epic 4.3: Runbook aprovado

**Goal:** separar contrato de hospedagem, checagens da aplicação e evidências operacionais. Cobrir dimensionamento, barreira, reconciliação, retenção/limpeza, bloqueio de inconsistência e limites de rollback. Degradação do provedor não se comprova com `SHOW synchronous_commit`; failover de banco não é automaticamente takeover de owner.
**Scope:** `docs/runbooks/`.
**Dependencies:** Epic 4.2.
**Done when:** operador independente executa ensaio autorizado pelo runbook; rollout continua sujeito à janela aprovada.
**Status:** Pending

---

## Phase 5: Deleção

### Epic 5.1: Remover caminhos substituídos

**Goal:** após migração de todos os tenants, retirar Lua, Valkey financeiro/idempotência migrada, write-behind, recovery, backup queue, balance sync, async interno e seletor. Preservar NOTED e v1/v2 ST/MT. Provar drain de metadata, dívida, eventos/Tracer e cobertura de reservas sem commit antes de apagar settle volátil. Revalidar inventário 2.4 contra develop: remover `RABBITMQ_TRANSACTION_ASYNC` ou outro contrato suportado exige prova de consumidores/compatibilidade; quebra real para aqui até aprovação, sem fallback ou major inventados. RabbitMQ inteiro e endpoints avulsos do Tracer não são deleções autorizadas desta lane. 202 futuro segue opt-in e separado.
**Scope:** `components/ledger/`, `pkg/`.
**Dependencies:** todos os tenants migrados; definições 7/10 implementadas; inventários drenados e gate de compatibilidade resolvido.
**Done when:** caminhos antigos substituídos ausentes, retenção/replay preservados, contratos/integração/chaos e `make ci` verdes. Busca textual não prova remoção ou paridade sozinha.
**Status:** Pending

### Epic 5.2: Documentação e observabilidade

**Goal:** atualizar engine, scoping, llms, guias, métricas e dashboards para comportamento verificado: autoridade SQL, projeções, intenção/recusa, sucesso/unknown, hospedagem, configuração e retenção. Histórico permanece identificado como histórico. Telemetria enxuta segue os padrões, sem promessa de redução medida antes de medi-la.
**Scope:** `docs/`, raiz, `components/infra/grafana/`.
**Dependencies:** Epic 5.1.
**Done when:** docs descrevem comportamento e limites; impacto de Console avaliado e entrega pareada quando necessário, sem teste que parseie prosa.
**Status:** Pending

### Epic 5.3: Delta de linhas

**Goal:** reportar delta da branch contra develop, separado em prod, test e docs; justificar crescimento pelo comportamento entregue.
**Scope:** este plano.
**Dependencies:** Epic 5.2.
**Done when:** delta e entrega constam no resultado final da `Phase Overview` e no relato ao Fred.
**Status:** Pending
