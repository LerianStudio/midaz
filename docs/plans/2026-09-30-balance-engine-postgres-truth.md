# Balance Engine on Postgres Truth Implementation Plan

> **For implementers:** Use ring-default:dispatching-workflows. Elaborate the
> current phase against the real code, execute a reviewed workflow, then elaborate
> the next phase. Alternatively, use ring-dev-team:running-dev-cycle.
> This document is the living source of truth for all five phases.

**Autorização:** nenhum código autorizado, aguardando Fred. A Fase 1 estar `Detailed` não autoriza implementar nem executar a bancada. Esta revisão altera somente o plano; não autoriza produção, provisionamento, branches, worktrees ou PRs.

**Goal:** tornar o Postgres do tenant a única verdade do dinheiro, com decisão, trilha e resultado no mesmo commit. O sucesso público depende da confirmação da durabilidade contratada, ainda sujeita à definição 2.

**Architecture:** um autorizador recebe lotes de intenções, resolve identidades antes de decidir e grava o resultado em uma transação SQL por lote. A bancada compara dono com fence e lotes concorrentes com row locks, sem escolher arquitetura só por desempenho. Transporte direto e Redpanda usam o mesmo autorizador e escritor; o motor não exige infraestrutura além do Postgres. A troca é por tenant, atrás de barreira, depois da paridade funcional e de leitura, sem major. Valkey, Lua, write-behind, recovery e async interno só saem após cobertura dos caminhos atuais.

**Tech Stack:** Go 1.27, PostgreSQL 17 local, `pgx/v5` v5.11.0, `squirrel`, `golang-migrate`, Docker Compose e toxiproxy 2.12.0. Redpanda `v24.2.7` e `franz-go` v1.21.6 somente no eixo de transporte. O PG17 local não comprova o comportamento nem o desempenho do RDS16.

**Execução futura:** worktree única `/srv/worktrees/balance-engine`, branch única `feature/balance-engine`. Nenhuma outra branch ou PR sem pedido do Fred. Quando a execução for autorizada, um workflow por fase e revisor diferente do implementador. Commits e pushes ficam com a sessão coordenadora, com o trailer `X-Lerian-Ref: 0x1`.

**Fontes:** alfarrábio `https://alfarrabio.lerian.net/reports/fred/midaz-motor-de-saldo`; `docs/architecture/engine.md`; reunião "Direcionamento Midaz" de 2026-10-14. Direção aprovada: intenção, corte antecipado por deadline inclusive sem gateway, autorizador por lote independente do transporte, nenhuma infraestrutura nova obrigatória, Redpanda dedicado quando usado, Tracer como módulo, alias resolvido por id, estudo de réplica de leitura e particionamento, telemetria enxuta. As definições pendentes abaixo não são decisões aprovadas.

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | Bancada compara escritores e transportes, mede decisões aprovadas duráveis, exercita replay, deadline e falhas; relatório recomenda go/no-go | 1.1, 1.2, 1.3, 1.4, 1.5 | Detailed |
| 2 | Protocolo aprovado e adaptador Postgres experimental para create v2, sem habilitação de clientes | 2.1, 2.2, 2.3, 2.4 | Epic-level |
| 3 | Paridade v1/v2, ST/MT, NOTED, lifecycle, controles, fee debt, respostas/replay e autoridade de leitura comprovada | 3.1, 3.2, 3.3, 3.4 | Epic-level |
| 4 | Tenant reconciliado migra atrás de barreira que cobre todos os escritores; ensaio e runbook aprovados | 4.1, 4.2, 4.3 | Epic-level |
| 5 | Caminhos antigos removidos após cobertura, drenagem e migração; docs e métricas refletem o motor único | 5.1, 5.2, 5.3 | Epic-level |

---

## Hoje e destino

### Hoje

As informações de produção desta seção são observações de 6 a 8 de outubro de 2026, não uma verificação atual. Naquele recorte, Lerian Cloud rodava Midaz 4.2.0 e `develop` estava em 4.3.0-beta. Não há acesso de produção autorizado neste plano.

No build desta branch, Lua no Valkey decide saldo em um EVAL. Depois, `finalizeCreateEngineResult`, em `components/ledger/internal/services/command/create_transaction_engine.go:177-245`, tenta publish com confirmação do RabbitMQ antes de retornar no modo async; sem dispatch confirmado, tenta completion SQL síncrona. Uma falha de projeção após aplicação financeira pode ficar para recovery e ainda permitir 201. Logo, 201 pode sair sem SQL confirmado, mas a sequência não é universalmente Lua, resposta, publish, SQL.

`execution.lua:123-145` relê a chave dentro do EVAL; valor vivo presente vence o seed. `query/get_balances.go:328-437` tenta reconstruir seed atrasado pela trilha de operações. Dois cache misses concorrentes não forçam a mesma versão se a chave publicada pelo primeiro permanece. A janela residual exige perda/restauração do estado vivo e seed ainda atrasado, inclusive quando a própria trilha ainda não foi persistida. Isso distingue o build da branch do dossiê do incidente de 07/10, que registrou eviction, forks e reenvios após timeout.

O recorte histórico tinha Valkey compartilhado, `appendonly=no`, snapshot diário e réplica assíncrona. A correção 4.2.1 paginou a limpeza de recovery; não comprova eliminação das demais causas. As listas de fee debt não têm reseed exato, e o cutover precisa considerar formatos históricos de chave, conforme hotfix #2587.

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

- O banco `transaction` torna-se a autoridade financeira. Identidade e fingerprint congelado são resolvidos antes de decisões novas; transações, operações, saldos, controles, resultado e obrigações pós-commit entram no mesmo commit. Controles entre bancos dependem da definição 1.
- Dono ou ownerless é recomendação da Fase 1 e escolha do protocolo, não efeito automático do maior TPS. Com dono, há um lote em voo e estado provisório privado. Ownerless usa linhas travadas e estado provisório por lote. Ambos descartam esse estado em rollback ou resultado incerto.
- Sucesso somente com durabilidade confirmada. COMMIT sem confirmação vira `unknown`, nunca recusa financeira nem execução com identidade nova. Replay de resultado existente prevalece sobre deadline, depois de confirmar sua durabilidade. Contrato público de retry e deadline depende das definições 4, 7 e 11.
- Hospedagem e aplicação têm responsabilidades distintas. `SHOW synchronous_commit=on` é uma checagem necessária à proposta, mas não prova réplica síncrona nem cobre degradação do RDS. Falhas toleradas, postura degradada e DR regional precisam da aprovação da definição 2. Failover do banco não implica sempre troca do dono; takeover só se necessário.
- O outbox cobre metadata, projeção de fee debt, eventos e acertos conhecidos do Tracer; com transporte B, também cobre respostas pendentes. Tentativas de reserva ainda não resolvidas exigem tratamento durável próprio antes de eliminar a fila em memória, conforme definição 10.

O corte antecipado por deadline inclui clientes sem gateway. O experimento impede o início de uma NOVA decisão financeira após observar o prazo vencido; não promete zero commits após uma hora de parede. Um COMMIT iniciado antes pode terminar depois. Timeout durante COMMIT preserva `unknown`. Default, propagação, configuração e resposta pública ainda são definição 11; nenhum header nem default de 30 s foi aprovado.

O autorizador trabalha por `balance_id`, e controles por identidade explícita de conta. Alias é resolvido na borda; reuso de alias não transfere estornos ou pendências para outra conta. Intenção identifica o pedido, não a transação: `request_id` e `transaction_id` são distintos, e uma recusa não cria transação financeira.

O motor roda com Postgres; Redpanda é opcional e dedicado ao ledger quando usado. O relay pode publicar no broker já disponível, inclusive RabbitMQ sem Redpanda. Remover async interno não altera silenciosamente o contrato público; 202 com consulta é uma opção futura separada, fora desta branch. Tracer como módulo é direção aprovada; in-process versus gRPC e propriedade dos limites continuam abertos na definição 10.

Telemetria seguirá `docs/standards/telemetry.md`, com spans de pedido, lote e commit e detalhe no erro. A redução de cerca de 70% citada na reunião é estimativa do time, não uma medição nem meta de eliminação cega. O particionamento temporal e a réplica de leitura são estudos; nenhuma migração de particionamento está aprovada.

### Topologias: onde o Redpanda entra

| Topologia | Caminho do resultado | Compra | Custa | Estado |
|---|---|---|---|---|
| A. Autorizador direto | API encaminha por gRPC; escritor decide e confirma SQL; resposta volta à API | Menos componentes no caminho síncrono | Salto de processo; com owner há roteamento e takeover, com ownerless há contenção SQL | Recomendada com E; mede-se na Epic 1.4 |
| B. Redpanda antes do autorizador | API publica intenção, worker decide; SQL grava resultado, offset e obrigação de resposta; relay publica resposta | Backlog durável e distribuição por partição | Broker, consumidor e canal de resposta; ACK do broker não é aprovação financeira | Alternativa experimental, definição 8 |
| C. Como B, aceite assíncrono | 202 no aceite; cliente consulta resultado depois | Dispensa espera síncrona pelo resultado | Altera o contrato público e adia recusas | Opção futura, fora desta branch |
| D. Publish após commit, sem outbox | SQL confirma, depois publica fora da transação | Nenhuma garantia adicional de dinheiro | Crash pode deixar commit sem publish; nessa ordem estrita não há publish sem commit | Não adotada; usar E |
| E. Outbox atrás do commit | Mesmo commit grava obrigações; relay publica e confirma antes de marcá-las | Retoma trabalho após falha sem dual write | Relay, lag e deduplicação at-least-once | Recomendada; serve tanto A quanto B |

A recomendação é A com E. B precisa provar correção e custo com o mesmo escritor, payload, batching e banco de A. Seus resultados informam a definição 8, não a decidem. Um grupo de consumidor por tenant no bench de tenant único não escolhe a topologia compartilhada ST/MT do produto.

### O que sai do produto

Na Fase 5: scripts Lua, Valkey no dinheiro e na idempotência migrada, write-behind, recovery runner, `engine:{transactions}:recover`, `backup_queue:{transactions}`, balance sync e `RABBITMQ_TRANSACTION_ASYNC`. A fila em memória de acerto do Tracer só sai após cobertura de tentativas sem commit SQL. O caminho legado `write_transaction`, usado por NOTED fora do Engine, não pode ser apagado sem substituto com paridade. A entrega inclui o delta de linhas.

### Custo e alvos da bancada

Uma transação simples grava 1 `transaction`, 2 `operation` e atualiza 2 `balance`. As 18 a 23 entradas de índice estimadas pelas migrações não incluem todas as novas intenções, controles e obrigações de outbox; a bancada mede o custo completo. `fillfactor` em `balance` é hipótese experimental, não configuração de produção decidida.

| Medida | Alvo ou interpretação |
|---|---|
| TPS sustentado em uma conta limitada | 8.000 aprovações distintas com commit durável confirmado por segundo, backlog estável e drenagem medida |
| p99 do resultado, RTT de replicação medido de 1 ms | ≤ 15 ms, hipótese a validar; separar núcleo local, gRPC e Redpanda |
| Perda ou dupla aplicação de identidades aprovadas após falha | 0 nas 20 rodadas de cada cenário aplicável |
| Takeover com 1M saldos e 1M controles | Medir contra referência experimental de 5 s; orçamento público depende da definição 3 |
| Soak de 30 min | 8k aprovações duráveis/s sustentadas; p99 final ≤ 1,5× p99 do quinto minuto |
| Nova decisão iniciada após observar deadline vencido | 0; COMMIT tardio e replay tardio são registrados separadamente |
| Transporte | Broker ACK, confirmação SQL e resposta medidos separadamente; sem perda de respostas pendentes após recuperação |

Recusas, expirações, duplicatas e `unknown` não contam para a meta de 8k. Célula HA, RDS ou transporte não executada fica "não medido". RF1 no Redpanda mede recuperação de worker, não HA do broker.

## Definições pendentes do Fred

Nenhuma pergunta nesta revisão. Na retomada, uma por turno no template frio. Recomendações não autorizam execução nem fecham arquitetura por silêncio. A Fase 1 usa hipóteses locais independentes dessas respostas; sua execução continua aguardando autorização.

| # | Decisão | Recomendação e custo | Bloqueia |
|---|---|---|---|
| 1 | Autoridade de block, limites e exceções, hoje distribuída entre bancos | Levar controles autoritativos para `transaction` e ordenar todos os escritores. Compra atomicidade; custa mudar a fronteira com onboarding. `account_control` é só a hipótese da bancada | Epics 2.1, 2.2, 3.2 |
| 2 | Quais falhas o zero perda cobre e o que fazer com replicação degradada | Separar contrato de hospedagem de checagens da aplicação; declarar domínios de falha e bloquear confirmações que não atendam ao contrato. A aceitação da janela degradada do RDS Multi-AZ de instância está aberta; BYOC usar RDS não a aprova | Epics 2.1, 2.2, 2.3 e Fase 4 |
| 3 | Indisponibilidade no takeover e espera do cliente durante failover do banco | Referência de 5 s para takeover, a medir. Tratar espera do RDS separadamente, sem somar takeover quando o owner sobrevive; definir timeout/unknown e reconexão sem prometer resposta antes do banco voltar | Epics 2.1, 2.3 e Fase 4 |
| 4 | Retry sem chave do cliente em create, commit, cancel e revert | Definir identidade explícita do pedido. Dois pagamentos legítimos com o mesmo corpo não são automaticamente retry; hash verifica equivalência sob uma identidade, não prova identidade sozinho. Derivação de transação mais ação para commit/cancel também exige aprovação. Sem política pública padrão escolhida | Epics 2.1, 3.1 |
| 5 | Interrupção no cutover | Pausa por tenant em janela agendada, com indisponibilidade explícita. Compra barreira verificável; custa interrupção | Fase 4 |
| 6 | Prioridade e garantia da ponte da linha 4.x | Lane separada para Valkey dedicado, retenção limitada e detector de fork; durabilidade de idempotência exige decisão própria. Nenhuma operação dessa lane é autorizada aqui | Fora desta branch |
| 7 | Contrato público de replay, inclusive original ainda em voo | Mesmo corpo e identidade retornam resultado congelado quando resolvido; enquanto não resolvido, preservar `unknown`, nunca inventar resultado. Custa definir espera, conflito de payload e retenção | Epics 2.1, 2.3 e Fases 3, 4 |
| 8 | Redpanda antes da decisão ou somente atrás do commit | A com E; avaliar B com medições equivalentes e suas obrigações de resposta e ownership. Desempenho não decide topologia sozinho | Epics 2.1, 2.3 e Fase 4 |
| 9 | Go ou Rust no autorizador | Go por stack, equipe e reuso das libs Lerian. CPU, GC e commit serão medidos antes de recomendar outra linguagem, sem limiar arbitrário | Epic 2.3 |
| 10 | Tracer in-process ou gRPC; propriedade de limites e resolução de reservas incertas | Preservar a separação entre resultado SQL e reserva até especificar a integração. Resolver duravelmente tentativas sem commit e acertos pós-commit; não atribuir tudo ao outbox de sucesso | Epics 2.1, 2.3, 3.2 e Fases 4, 5 |
| 11 | Deadline público, inclusive clientes sem gateway | Corte antecipado de decisões novas; replay preserva resultado anterior e COMMIT em voo pode terminar tarde. Definir origem, default, propagação e resposta antes de mudar API; 30 s e 2 s são apenas parâmetros de bench | Epics 2.1, 2.3 e Fases 3, 4 |

## Fora do escopo desta branch

- Ponte operacional da linha 4.x, acesso a produção e mudança de hospedagem. RDS temporário requer autorização separada e nunca usa produção.
- Réplica de leitura: MongoDB e ClickHouse são candidatos para estudo, sem conclusão de licença neste plano. Particionamento temporal também é estudo, não migração obrigatória.
- 202 e consulta de resultado, além da lane do time para endpoints avulsos do Tracer e `transaction_type`.
- Compatibilidade interna com o motor antigo depois da Fase 5. Contratos públicos v1/v2 e ST/MT continuam preservados; a remoção interna não os revoga.
- Issues públicas sobre os incidentes. Gaps fora do pedido ficam em `~/.claude/gaps/midaz/`.

---

## Phase 1: Experimentos medidos

Objetivo: números e provas de correção da bancada, não código de produto. Novos arquivos ficam em `tests/bench/engine/`, tag `bench`, fora dos lanes de CI. Make e `.gitignore` só recebem integração desse pacote. Nenhum experimento roda nesta revisão.

### Contrato comum do experimento

Este contrato vale para owner, ownerless, gRPC e Redpanda; não define o contrato público futuro.

1. Cada pedido tem `request_id` estável, `transaction_id` separado quando houver movimento, postings congelados e fingerprint do payload. Retry repete a mesma identidade e o mesmo payload. O escopo do bench é um tenant; controles usam `(organization_id, ledger_id, account_id)` e dinheiro usa `balance_id`.
2. Dentro da MESMA transação SQL do lote, reclamar identidades distintas em ordem canônica antes de decisão ou escrita financeira. Usar `INSERT ... ON CONFLICT DO NOTHING`, depois ler/travar o resultado conflitante em `READ COMMITTED` na mesma ordem e comparar o fingerprint congelado; espera limitada pelo contexto. Não commitar claims separadamente. A primeira ocorrência no lote fixa o payload; duplicatas equivalentes compartilham a decisão, divergentes recebem conflito. Retry não renova deadline nem recalcula o payload original.
3. Resolver resultados existentes e duplicatas antes de avaliar pedidos novos. Uma execução anterior ainda bloqueando a identidade ou sem resolução confirmada mantém `unknown`. Resultado final existente prevalece sobre prazo vencido, condicionado à barreira de durabilidade abaixo. Somente pedidos únicos novos chegam à decisão.
4. Decidir em ordem original sobre estado provisório privado do lote. Nenhum saldo ou controle publicado em memória muda antes do commit. Persistir resultado, linhas financeiras e outbox; publicar o estado provisório só após confirmação. Rollback ou COMMIT incerto descarta tudo. Conflito descoberto depois de decisões exige rollback e recomputação do lote afetado inteiro; jamais retirar uma transação ou posting e conservar decisões que dependiam dele.
5. Não iniciar decisão financeira nova quando o relógio observado já alcançou o deadline. Entrada vencida só vira `expired` após resolver a identidade anterior, sem movimento. Revalidar imediatamente antes da decisão, após esperas por claims/locks. COMMIT já iniciado pode terminar tarde; timeout não o transforma em recusa. Deadline de pedido de 30 s, célula de 2 s e contexto SQL de 10 s são parâmetros locais, não defaults do produto.

No núcleo simplificado, depois do prazo e da identidade, verificar block/flags, saldo disponível e limite remanescente. Recusas não alteram dinheiro; aprovação atualiza apenas a cópia provisória e suas versões. Overdraft e lifecycle completos ficam na Fase 3, não estão cobertos pela carga de inteiros do bench.

**Durabilidade e replay:** no pgx v5.11.0, `pgconn.go:768-790` envia `CancelRequest` em `asyncClose`; `receiveMessage`, linhas 1613-1620, chama esse caminho após erro de leitura. Cancelar durante a espera de replicação síncrona no Postgres pode deixar o commit local sem replicação confirmada. Portanto, não pressupor "socket fecha sem cancel", não inventar opção do pgx e não tratar linha visível no primário como prova de durabilidade.

O bench mantém `unknown` em timeout, perda de resposta ou aviso de cancelamento da espera síncrona. O tratamento observa também notices do COMMIT; retorno sem erro sozinho não basta se a espera síncrona foi cancelada. Antes de reconhecer um resultado encontrado por replay, a mesma transação finalizadora incrementa `engine_request.durability_touch` e faz um COMMIT síncrono com escrita real de WAL, no primário autoritativo. Esse WAL posterior cobre o resultado anterior na ordem de replicação. `SELECT` seguido de COMMIT read-only não é barreira. Sem confirmação da nova barreira, permanece `unknown`. A prova depende de primário único e da configuração de réplica da célula; não é garantia mágica de exactly-once nem evidência de um 201 falso em produção.

**Relógios:** registrar `decisionStartedAt`, `dbAckAt` e `repliedAt`; Redpanda acrescenta `brokerAckAt`. Recebimento do ACK no host não é timestamp real de commit. SQL `now()` é início da transação; `clock_timestamp()` é relógio corrente do banco, não prova de instante de commit. O bench não inventa `committedAt`; mede confirmação e resposta. Fixtures temporais determinísticas usam relógio injetado; medições reais usam relógio do processo, sem gerar carga sintética de CPU.

**Cobertura:** o throughput usa uma chave por conta por simplicidade. Um fixture pequeno usa dois `balance_id` da mesma conta, contrapartes repetidas e controle compartilhado, provando que saldos não se confundem e limites não são duplicados. Isso não substitui a paridade funcional da Fase 3.

### Epic 1.1: Ambiente Postgres HA reproduzível para o bench

**Goal:** primário PG17, standby síncrono, latência controlada e esquema real com tabelas experimentais.
**Scope:** `tests/bench/engine/`.
**Dependencies:** autorização de execução da Fase 1.
**Done when:** bootstrap reproduzível, standby em `sync` e esquema validado.
**Status:** Pending

#### Task 1.1.1: Compose com primário, standby e toxiproxy

- [ ] Done

**Context:** `components/infra/docker-compose.yml:65-155` usa réplica assíncrona. O bench precisa de standby nomeado e falhas controladas; toxiproxy já é dependência do repo.

**Implementation vision:** três serviços com nomes fixos: `bench-pg-primary`, `bench-pg-standby` e `bench-toxiproxy`. Postgres `17`, toxiproxy `ghcr.io/shopify/toxiproxy:2.12.0`. Somente loopback nas portas do host: primário 5432, standby 5435, proxy de replicação 5433, proxy de cliente 5434 e API do toxiproxy 8474. Credenciais descartáveis `bench`/`bench`, banco `bench_transaction`; habilitar conexão de replicação na rede do compose com `host replication all 0.0.0.0/0 scram-sha-256`, sem expor portas fora do loopback.

Primário com `wal_level=replica`, `max_wal_senders=5`, `max_connections=300`, `shared_buffers=4GB`, `max_wal_size=8GB`, `checkpoint_timeout=15min` e `log_checkpoints=on`. Manter `synchronous_commit=on` como configuração do servidor, não argumento que impeça o controle negativo. `init-primary.sql` usa `ALTER SYSTEM` para `synchronous_standby_names='FIRST 1 (standby1)'`, só efetivo no start definitivo; configurar isso no servidor temporário do entrypoint bloquearia sua inicialização.

`pg_basebackup -R` atravessa `pg-repl` e configura `application_name=standby1`. Standby usa `hot_standby=on` e sobrepõe `synchronous_standby_names=''` para não esperar por si ao promover. Após promoção, essa ausência de standby deve ser tratada explicitamente no teste, nunca contada como durabilidade síncrona. O helper reinstala réplica e remove a sobreposição vazia no comando do promovido, preservando seu volume, antes de configurar espera síncrona e admitir novas confirmações duráveis. `pg-client` permite perder respostas independentemente da replicação. Volumes são exclusivos do bench; `down -v` só os apaga dentro do ambiente descartável autorizado.

**Files:**
- Create: `tests/bench/engine/docker-compose.yml`
- Create: `tests/bench/engine/init-primary.sql`
- Create: `tests/bench/engine/toxiproxy.json`
- Create: `tests/bench/engine/README.md`

**Verification:** `docker compose -f tests/bench/engine/docker-compose.yml up -d --wait --wait-timeout 120`; consultar `pg_stat_replication` no DSN local e exigir `standby1 | sync`. A verificação de COMMIT bloqueado com standby ausente fica no teste limitado da Task 1.3.1, não em um INSERT manual sem timeout.

**Done when:** ambiente novo fica pronto dentro do limite do comando, sem depender de serviços externos.

#### Task 1.1.2: Bootstrap do esquema e fixtures reais

- [ ] Done

**Context:** `components/ledger/migrations/transaction/` contém versões 000000 a 000043 neste HEAD. O padrão de `golang-migrate` está em `tests/integration/ledger_migrations_integration_test.go:110-113`. `operation.snapshot`, em `pkg/mmodel/operation.go:29-35`, contém duas strings: `overdraftUsedBefore` e `overdraftUsedAfter`, não um create de 1 KB.

**Implementation vision:** todos os Go novos usam tag `bench` e cabeçalho de licença. `requireDB(t)` pula somente testes que precisam do DSN; testes puros continuam executáveis sem banco. Variáveis `BENCH_PG_PRIMARY_DSN`, `BENCH_PG_CLIENT_PROXY_DSN`, `BENCH_PG_STANDBY_DSN` e `BENCH_TOXIPROXY_URL` apontam apenas ao compose descartável. Bootstrap valida esse destino, espera `standby1 | sync` com limite de 120 s, recria o schema, aplica migrações e DDL bench-local, depois semeia por COPY.

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
  deadline timestamptz NOT NULL,
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
CREATE TABLE engine_outbox (
  id uuid PRIMARY KEY,
  tenant_id text NOT NULL,
  request_id uuid NOT NULL,
  kind text NOT NULL,
  payload jsonb NOT NULL,
  delivered_at timestamptz
);
CREATE INDEX engine_outbox_pending
  ON engine_outbox (tenant_id, id) WHERE delivered_at IS NULL;
```

`outcome` nulo só existe como claim dentro da transação; todo commit de lote o finaliza. Rejeição/expiração não têm `transaction_id` financeiro. `decided_at` não é commit timestamp. Outbox inclui linhas, índices e payloads representativos das obrigações de metadata, eventos e Tracer; fee debt entra na fixture correspondente. Extrair quantidade e formato das obrigações do caminho real de completion e mantê-los iguais nos dois escritores. Registrar bytes e quantidade no resultado. Sem chamar esses serviços no microbenchmark: medir gravação do outbox e declarar entrega externa fora dessa célula. A resposta do transporte B acrescenta sua obrigação própria, custo explícito desse transporte.

Usar a representação efetivamente persistida: create direto grava `transaction.body=NULL`; PENDING grava o corpo normalizado, conforme `transaction_completion_service.go:349-351`. A fixture `create_v2_body.json` representa a entrada, não preenchimento artificial da coluna SQL. Usar snapshot real por operação e medir separadamente os bytes novos de intenção, resultado e outbox. Não reduzir o outbox a uma linha vazia. Rodar bootstrap duas vezes produz o mesmo estado.

**Files:**
- Create: `tests/bench/engine/main_test.go`, `tests/bench/engine/env.go`
- Create: `tests/bench/engine/bootstrap.go`, `tests/bench/engine/schema_bench.sql`
- Create: `tests/bench/engine/testdata/create_v2_body.json`, `tests/bench/engine/testdata/operation_snapshot.json`
- Test: `tests/bench/engine/bootstrap_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBootstrap$' -timeout 30m -v`, com DSNs do compose. Conferir migração 43, quatro tabelas bench, índice de pendências, totais de saldos/controles e duas execuções internas do bootstrap.

**Done when:** bootstrap repetível; tempos de 10k e 1M registrados, com limites por execução de 60 s e 10 min para a semeadura.

### Epic 1.2: Harness de carga com os dois escritores

**Goal:** comparação local do núcleo com a mesma carga, regras de lote, SQL, outbox e replicação.
**Scope:** `tests/bench/engine/`, `mk/tests.mk`, `.gitignore`.
**Dependencies:** Epic 1.1.
**Done when:** correctness gates passam e os blocos de medição produzem resultados reproduzíveis.
**Status:** Pending

#### Task 1.2.1: Gerador e métricas

- [ ] Done

**Context:** a carga principal toca uma conta limitada quente e uma contraparte aleatória. É um microbenchmark local do núcleo, não latência de API.

**Implementation vision:** `writer.go` define `Writer` com `Submit` e `Close`, implementado pelos dois escritores; requests incluem IDs distintos, fingerprint, postings, `scheduledAt` e deadline. Outcomes separam `approved`, `rejected`, `expired`, `unknown` e conflito de identidade. Replays têm indicador próprio para não contar como novo TPS.

Malha fechada com 256 produtores mede capacidade máxima. Malha aberta agenda `start + i/rate`, default 8.000/s, e mede desde `scheduledAt` para incluir atraso do gerador e evitar coordinated omission. Registrar envios reais, decisões novas, aprovações duráveis distintas, recusas por motivo, expirações, duplicatas, `unknown`, backlog e drain. Separar `dbAckAt-scheduledAt` de `repliedAt-scheduledAt`; não nomear nenhum deles `committedAt`.

Cargas `outflow`, `inflow` e `mixed` alternada; valores inteiros de 1 a 100, conservação de duas pernas. `feeLeg` divide o crédito e acrescenta terceira perna sem criar dinheiro. Limite só desce nos débitos; célula de esgotamento mostra aprovações e recusas separadas, inclusive a gravação das intenções recusadas. Os fast paths não satisfazem 8k. Fila interrompida sem prova do resultado retorna `unknown`; retry usa a mesma identidade. O gerador nulo testa sua própria capacidade, não prova que nunca haverá gargalo no harness real.

**Files:**
- Create: `tests/bench/engine/writer.go`, `tests/bench/engine/workload.go`, `tests/bench/engine/metrics.go`
- Test: `tests/bench/engine/workload_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestWorkloadKeysUnique|TestOpenLoopSchedule|TestPercentiles|TestWorkloadNullWriter' -timeout 5m -v`. Fixtures verificam identidades, calendário e percentis; medição limitada verifica 20k envios/s ao escritor nulo e registra atraso real do agendamento.

**Done when:** testes passam e todos os contadores distinguem aprovação durável de recusa, replay e resultado incerto.

#### Task 1.2.2: Escritor com dono

- [ ] Done

**Context:** um dono por tenant mantém um lote em voo. A ordenação de identidade precede a decisão, conforme contrato comum; o estado publicado não serve como buffer especulativo.

**Implementation vision:** `writer_owner.go` drena até `maxBatch`, default experimental 256. `takeover()` adquire por `UPDATE ... WHERE tenant_id=$tenant AND lease_until <= clock_timestamp()`, troca owner, incrementa epoch e retorna a nova geração. Renovação exige owner, epoch e `lease_until > clock_timestamp()`, usando relógio atual do banco, não `now()`. Lease de 3 s e renovação a cada 1 s são parâmetros do bench; renovar durante reload, não apenas depois de uma página demorada.

Carregar saldos por `balance.id` e controles pela chave composta da conta em páginas de 50k. Estado monetário usa `map[balanceID]`, nunca `map[accountID]`. O bench exclui outros escritores durante reload; a consistência com writers administrativos reais é gate das Epics 2.1 e 3.2. Perda de lease ou página com erro aborta o reload; não publicar mapa parcial.

Por lote: `beforeBegin` acontece antes de qualquer decisão ou lock; abrir SQL, renovar e travar a fence com owner, epoch e lease ainda válido; reclamar identidades canonicamente; resolver resultados anteriores; só então criar estado provisório e decidir novos pedidos. Inserir `transaction` e `operation` em lote; atualizar saldos por `balance_id` e controles por chave composta com versões esperadas, exigindo todas as linhas; finalizar intenções e inserir outbox. Um crédito novo pode financiar débito posterior no mesmo lote, mas um replay não o financia de novo. Divergência de versão aborta o lote e encerra o dono para reload, sem descarte silencioso.

COMMIT confirmado publica o estado provisório e libera respostas. Erro/notice que deixe replicação incerta retorna `unknown`, descarta o estado e encerra o dono; o novo dono resolve identidades sob o protocolo de replay e barreira WAL. O ticker não estende lease expirado. Se um lote já segura a linha da fence, um takeover concorrente espera a transação terminar; a expiração não revoga magicamente locks SQL.

**Files:**
- Create: `tests/bench/engine/writer_owner.go`
- Test: `tests/bench/engine/writer_owner_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestOwnerWriterConservation|TestOwnerWriterIdentity|TestOwnerWriterMultiKey' -timeout 30m -v`. Cinco rodadas internas com reset; 200k pedidos na conservação, join 1:1 entre aprovações distintas e transações, outbox esperado e nenhuma intenção incompleta. Identidade cobre duplicatas no lote e concorrentes, payload divergente, replay de crédito seguido de débito, rollback inteiro e replay depois do prazo. Multikey verifica dois saldos da mesma conta e um controle compartilhado.

**Done when:** todas as rodadas passam, sem publicação de estado em rollback/unknown e sem nova decisão para replay.

#### Task 1.2.3: Escritor sem dono

- [ ] Done

**Context:** baseline sem cache de estado nem lease de escritor. A exclusão de identidades continua obrigatória; row locks de saldo não substituem idempotência.

**Implementation vision:** `writer_ownerless.go` usa `writers` goroutines, default experimental 32, e a MESMA regra de drenagem e `maxBatch` do owner. Abrir SQL, reclamar identidades distintas em ordem canônica, resolver duplicatas/replays, depois travar saldos por `balance.id` ordenado e controles pela chave composta ordenada. Decidir pedidos novos em ordem original sobre cópia privada das linhas; persistir com as mesmas instruções e outbox do owner, sem a fence do escritor. Replay isolado também escreve o touch WAL antes de confirmar.

A ordem de classes de locks é uniforme: offset de transporte quando existir, fence de owner quando existir, identidades, saldos e controles. `40P01` aborta tudo e admite uma tentativa limitada de recomputação com as mesmas identidades; timeout não prova rollback do COMMIT. Qualquer conflito tardio invalida o lote inteiro. Sem retry financeiro sobre decisão anterior ainda não resolvida.

**Files:**
- Create: `tests/bench/engine/writer_ownerless.go`
- Test: `tests/bench/engine/writer_ownerless_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestOwnerlessWriterConservation|TestOwnerlessWriterIdentity|TestOwnerlessWriterMultiKey' -timeout 30m -v`, cinco rodadas internas com as mesmas asserções e volume da Task 1.2.2; registrar deadlocks e retries limitados.

**Done when:** correctness gates equivalentes passam nos dois escritores antes de comparar desempenho.

#### Task 1.2.4: Matriz local e comandos limitados

- [ ] Done

**Context:** latência injetada não é automaticamente RTT. A tag `bench` não deve entrar nos shards de integração nem nos lanes de CI existentes.

**Implementation vision:** `toxiproxy.go` remove toxics antigos e configura `pg-repl`; falha de configuração aborta a célula. Registrar atraso configurado, direção e RTT observado, sem chamar um toxic unilateral de RTT medido. Cada célula faz bootstrap de 10k, 60 s de aquecimento e 120 s de medição, com drain limitado a 30 s. Identidades do aquecimento ficam fora dos contadores da medição.

| Bloco | Células | Conteúdo |
|---|---|---|
| A | 18 | owner/ownerless × RTT-alvo 0/1/10 ms × outflow/inflow/mixed, malha fechada |
| B | 18 | Mesma matriz, malha aberta a 8.000/s; medir backlog, aprovações duráveis e drain |
| C | 20 | RTT-alvo 1 ms, outflow: owner com batch 1/8/64/256; ownerless com writers 1/4/32 × mesmos batches; mais esgotamento e feeLeg para ambos |
| D | 4 opcionais | Dois escritores × malha aberta/fechada em RDS temporário autorizado, com versão e configuração declaradas |

São 56 células locais × 180 s = 168 min só de carga, antes de bootstrap e drain. Executar A/B/C em comandos separados, não sob um timeout global de 120 min. Limites calculados: até 60 s de bootstrap + 180 s de carga + 30 s de drain por célula; A/B requerem 81 min mais margem, C requer 90 min mais margem. Make traduz `BENCH_BLOCK=A|B|C` para `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBenchHotAccount$' -timeout 90m -v` em A/B e `-timeout 100m` em C. Cada comando falha se exceder seu orçamento; isso não é promessa de duração. `BENCH_MATRIX=full` exige bloco explícito. Quick usa uma célula owner, RTT-alvo 1 ms, outflow, 20 s de medição e timeout de 5 min.

Por célula: TPS aprovado durável, demais outcomes, p50/p99/p999 de confirmação e resposta, tamanho real de lote, locks/deadlocks, backlog/drain, bytes de WAL e FPI, checkpoints, flush lag, CPU e pausas de GC observadas. Outbox tem o mesmo payload, índices e política de drenagem nos dois escritores; sua acumulação é registrada separadamente, sem alegar throughput sustentável de serviços externos não executados. Saída em `tests/bench/engine/out/<timestamp>.md`, ignorada pelo git. Aborto ou backlog crescente torna a célula inelegível, não um resultado rápido.

Make sobe compose, exporta DSNs e usa trap para limpar só recursos descartáveis. Bloco D não roda por default: exige autorização, host na VPC e instância temporária não produtiva. Sem medição, RDS fica "não medido". A comparação local também não mede gRPC, Tracer nem MongoDB; o salto de transporte será medido na Epic 1.4.

**Files:**
- Create: `tests/bench/engine/toxiproxy.go`, `tests/bench/engine/bench_test.go`
- Modify: `mk/tests.mk`, `.gitignore`

**Verification:** `BENCH_MATRIX=quick make test-bench-engine`; `make test-gate-selection`; `golangci-lint run --build-tags bench ./tests/bench/engine/`. Revisar seleção de tags sem escrever teste que parseie documentação. Depois, comandos separados `BENCH_MATRIX=full BENCH_BLOCK=A make test-bench-engine`, B e C.

**Done when:** blocos completos ou células explicitamente não medidas, com limites de execução, contadores separados e nenhum resultado de teste servido do cache.

### Epic 1.3: Cenários de falha

**Goal:** provar identidade, conservação e durabilidade nas falhas locais, e medir takeover.
**Scope:** `tests/bench/engine/`.
**Dependencies:** Epic 1.2 e seus correctness gates.
**Done when:** falhas passam 20 rodadas internas por escritor aplicável; limitações de HA ficam explícitas.
**Status:** Pending

#### Task 1.3.1: COMMIT incerto e barreira de replay

- [ ] Done

**Context:** perder resposta ou cancelar a espera síncrona pode deixar resultado local visível sem prova de flush no standby. A chave resolve duplicação; a barreira WAL resolve a confirmação da durabilidade.

**Implementation vision:** `TestLostCommitAck` roda owner e ownerless em comandos separados. Cada um tem 20 rodadas internas com rebuild e reset, em ordem A, C, B. Usar sinais observados de backend/replicação e ganchos limitados, não sleeps como prova.

- A, resposta perdida: segurar downstream em `pg-client` com toxic `timeout=0`, deixar o contexto SQL expirar, exigir `unknown`, remover toxic e resolver com a mesma identidade. Verificar o caminho real do pgx, inclusive cancel e notices, sem supor COMMIT aplicado ou abortado.
- C, standby ausente: parar standby, sincronizar o teste até o backend entrar na espera síncrona e expirar o contexto. Observar se o resultado local ficou visível. Tentar replay ainda sem standby; o touch WAL deve bloquear ou retornar `unknown`, nunca aprovação durável por simples SELECT. Restaurar standby e só então confirmar o resultado original pela barreira. Enquanto o original não estiver resolvido, nem `expired` nem recusa financeira o substituem.
- B, primário morto: matar o primário, promover standby e verificar imediatamente que todas as identidades antes confirmadas sobreviveram. O promovido inicialmente sem standby só prova sobrevivência dos commits anteriores; não emitir novas aprovações duráveis nessa janela. Recriar réplica a partir do novo primário, confirmar configuração síncrona e então resolver `unknown`. O primário antigo nunca volta como escritor independente.

Comparar o conjunto de IDs que receberam aprovação durável, não apenas contagens: todos devem existir com o resultado e a transação correspondente. Verificar no máximo uma aplicação por identidade, conservação, outbox e resolução de `unknown` após restauração dentro do orçamento. Repetir inclui replay depois do deadline e cancel da espera síncrona com resultado local visível. Isso é prova experimental, não alegação de vulnerabilidade de produção já demonstrada.

Controle negativo, uma rodada por escritor: desligar espera síncrona e bloquear replicação, obter uma aprovação local cujo WAL o standby comprovadamente ainda não recebeu, matar primário e promover. O detector deve apontar a identidade perdida; atraso arbitrário sozinho não garante essa janela. RDS force-failover é variante D separada, apenas no ambiente temporário autorizado. Sem ela, comportamento RDS segue não medido.

**Files:**
- Create: `tests/bench/engine/failover_test.go`
- Create: `tests/bench/engine/compose.go`

**Verification:** `BENCH_WRITER=owner go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestLostCommitAck$' -timeout 180m -v`, depois ownerless. Orçamento por rodada: rebuild 120 s, A 60 s, C 90 s, B 240 s; 20 × 510 s = 170 min, mais até 5 min de controle negativo e margem. Estouro é falha, não redução silenciosa de rodadas.

**Done when:** 20 rodadas por escritor passam, controle negativo detecta perda e replay não confirma durabilidade durante ausência da réplica.

#### Task 1.3.2: Lease expirado e locks em voo

- [ ] Done

**Context:** dono pausado antes do BEGIN e dono que já mantém lock SQL são situações diferentes.

**Implementation vision:** `TestStaleOwnerFenced` pausa A em `beforeBegin`, sem decisões nem locks; suspende sua renovação e espera expiração observada pelo banco. B adquire apenas após expiry, recebe novo epoch, recarrega e grava 1.000 transações. Ao liberar A, seu UPDATE condicionado por owner/epoch/lease afeta zero linhas antes de claims ou decisões. Nenhuma linha financeira do lote antigo aparece.

`TestOwnerLockSerialization` pausa A depois de adquirir a fence dentro de SQL. Iniciar takeover de B concorrentemente; ele espera o lock, não se exige avanço de B antes de liberar A. Após término de A e lease expirado, B pode assumir e deve carregar o estado resultante. O teste não espera que revogação externa invalide retroativamente uma transação que já detém o lock. Vinte rodadas internas por teste, com reset e waits limitados.

**Files:**
- Modify: `tests/bench/engine/failover_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestStaleOwnerFenced|TestOwnerLockSerialization' -timeout 20m -v`.

**Done when:** takeover prematuro e renovação de lease expirado são recusados; concorrência com lock em voo termina sem deadlock do próprio teste.

#### Task 1.3.3: Reload de 1M saldos e controles

- [ ] Done

**Context:** medir reload informa a definição 3, mas não escolhe orçamento público nem prova controle concorrente de produto.

**Implementation vision:** `TestTakeoverReload`, `BENCH_BALANCES=1000000`, semeia 1M saldos ordinários e controles, mais extras declarados. Mede aquisição, páginas, renovação, primeiro lote e RSS. Amostra validade do lease pelo relógio do banco. Chave de paginação de controle é composta, sem perder contas entre organizations/ledgers. Falha de página ou renovação aborta tudo. No bench, nenhum writer externo altera controles durante o reload; no produto, snapshots e ordenação de TODOS os writers precisam da prova da Epic 3.2. Lazy loading é alternativa a estudar caso o número não sirva, não fallback aprovado.

**Files:**
- Modify: `tests/bench/engine/failover_test.go`, `tests/bench/engine/bootstrap.go`

**Verification:** `BENCH_BALANCES=1000000 go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestTakeoverReload$' -timeout 30m -v`, uma rodada completa, com tempo por página e RSS.

**Done when:** números registrados contra referência experimental de 5 s, sem esconder aquisição, renovação ou primeiro lote.

### Epic 1.4: Eixo de transporte: gRPC contra Redpanda

**Goal:** comparar um salto gRPC real entre processos com Redpanda, usando o mesmo autorizador, escritor elegível, batching, payload e Postgres.
**Scope:** `tests/bench/engine/`.
**Dependencies:** Epics 1.2 e 1.3. Fixar escritor/configuração para a comparação; se nenhum cumprir correção, não medir transporte como candidato viável.
**Done when:** matriz distingue ACK do broker, confirmação SQL e resposta; respostas pendentes sobrevivem a falhas; deadline não inicia decisões vencidas.
**Status:** Pending

#### Task 1.4.1: Broker local e ownership de partição

- [ ] Done

**Context:** Redpanda `v24.2.7` e franz-go v1.21.6 já constam do repo. O bench usa um tenant, não define grupos multi-tenant do produto.

**Implementation vision:** perfil `transport` acrescenta `bench-redpanda`, um nó, RF1, portas loopback 19092 e 19644. `kadm` cria `engine-intents` e `engine-replies`, cada um com uma partição; `foreign-events` tem oito. RF1 não prova sobrevivência à perda do broker.

Tabela bench-local `engine_consumer_offset` com `(tenant_id, topic, partition)` como PK, `next_offset`, `owner_id` e `epoch`. Em CADA assignment, o novo consumidor adquire ownership SQL sob lock, incrementa epoch e lê `next_offset` antes de consumir. Revocation interrompe trabalho local, mas não incrementa epoch por mágica. Cada lote trava essa linha no início e exige owner/epoch/offset esperado, avançando `next_offset` na mesma transação financeira. Um antigo consumidor antes do BEGIN falha após takeover; se já detinha o lock, o takeover espera seu término.

`next_offset` é o próximo registro: depois de commit, o lote anterior não será reentregue por esse seek. A recuperação da resposta deve vir do outbox, não de republicação artificial da intenção. Fencing de partição existe nos dois escritores em B, mas não obriga o algoritmo financeiro a ser owner. Configuração de grupos, assignments e isolamento de tenants do produto depende da definição 8.

**Files:**
- Modify: `tests/bench/engine/docker-compose.yml`, `tests/bench/engine/schema_bench.sql`
- Modify: `tests/bench/engine/bootstrap.go`, `tests/bench/engine/env.go`

**Verification:** `docker compose -f tests/bench/engine/docker-compose.yml --profile transport up -d --wait --wait-timeout 120`; `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBootstrapTransport$' -timeout 5m -v`. Duas inicializações internas verificam tópicos, offset inicial e aquisição de epoch.

**Done when:** assignment toma ownership SQL e bootstrap repetido não reutiliza epoch de um consumidor anterior.

#### Task 1.4.2: Adaptadores equivalentes e resposta durável

- [ ] Done

**Context:** chamada em processo na Epic 1.2 é microbenchmark, não baseline de transporte. A comparação exige o mesmo escritor selecionado, inclusive se ownerless for elegível/preferido.

**Implementation vision:** extrair `authorizer.decide` compartilhado pelos escritores, chamado SOMENTE dentro do protocolo de identidade/locks e sobre estado provisório. Os adaptadores não decidem dinheiro. Um processo bench-local de servidor gRPC recebe o mesmo payload que o consumidor Redpanda e usa o mesmo escritor e fila; o gerador fica em processo distinto. Contrato gRPC é privado da bancada, sem criar API pública.

Redpanda `Submit` registra a espera por correlação de caller/tentativa antes de publicar e usa `acks=all`. A correlação de entrega fica fora do fingerprint financeiro; `request_id` controla aplicação e replay. Callers simultâneos não substituem canais uns dos outros, e payload divergente recebe conflito só na tentativa correspondente. `Produce` do franz-go é assíncrono: callback de entrega e espera limitada determinam ACK ou `unknown`. Timeout/perda do ACK pode deixar mensagem aceita; callback tardio não cria identidade financeira nova nem bloqueia a aplicação. Limitar o mapa de callers e limpar cancelados; retry repete identidade/payload. ACK do broker só mede aceite da intenção.

Consumer desabilita auto-commit no broker. No assignment toma epoch SQL, posiciona em `next_offset`, drena lote e executa a MESMA transação finalizadora dos escritores: ownership, claims, replay/barreira, decisão de novos pedidos, linhas financeiras, resultado, outbox e offset. Não decidir antes do BEGIN. O primeiro registro deve corresponder ao offset SQL esperado. Rollback não recuperado localmente ou COMMIT `unknown` interrompe a sessão de consumo e descarta batches prefetched; o próximo lote só chega após resolver a transação anterior e reassumir pelo `next_offset` SQL. `PollRecords` avança o cursor local; desabilitar auto-commit não o rebobina. Usar o restart/assignment já descrito, sem fila nova. A mesma transação inclui obrigação de resposta com ID de entrega estável, payload congelado, `request_id` e correlação da tentativa.

Relay lê pendências e aplica a barreira WAL do contrato comum aos resultados lidos antes de publicar `engine-replies`; visibilidade local do outbox após restart não prova replicação. Espera confirmação do publish e só depois marca entrega. Crash após publish e antes da marca pode duplicar a resposta; receptor deduplica pelo ID de entrega da tentativa, preserva sua correlação e entrega uma vez ao caller correto. Crash antes do publish deixa obrigação retomável. Worker reiniciado faz seek ao próximo offset, e relay independente resolve callers ainda pendentes sem republicar intenção. Caller encerrado pode consultar/repetir sua identidade depois; não apagar obrigação só porque não há canal em memória. Replays de mensagens novas com a mesma identidade retornam resultado anterior e geram a resposta necessária sem nova aplicação financeira.

Métricas: `brokerAckAt-scheduledAt`, `dbAckAt-scheduledAt`, `repliedAt-scheduledAt`. Broker ACK não é persistência financeira; gRPC mede os dois últimos. Confirmar mensagens só depois da durabilidade SQL, incluindo replay de commit local anteriormente incerto.

**Files:**
- Create: `tests/bench/engine/authorizer.go`, `tests/bench/engine/transport_grpc.go`
- Create: `tests/bench/engine/transport_redpanda.go`, `tests/bench/engine/outbox_relay.go`
- Modify: `tests/bench/engine/writer_owner.go`, `tests/bench/engine/writer_ownerless.go`
- Test: `tests/bench/engine/transport_grpc_test.go`, `tests/bench/engine/transport_redpanda_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestGRPCTransportConservation|TestRedpandaTransportConservation|TestOwnerWriterConservation|TestOwnerlessWriterConservation' -timeout 45m -v`. Cinco rodadas internas por transporte; conservação por identidade, offset final correto, outbox drenado e nenhuma decisão nova iniciada vencida. Incluir retomada após rollback/unknown com cursor já avançado e callers simultâneos sob a mesma identidade: payload igual recebe replay; payload divergente recebe conflito na tentativa correta.

**Done when:** ambos os transportes usam o escritor explicitamente escolhido; resultados e provas locais continuam iguais após extração.

#### Task 1.4.3: Matriz, backlog e recuperação

- [ ] Done

**Context:** medir transporte e provar retomada são células diferentes. Deadline, offset e resposta precisam ser observados separadamente.

**Implementation vision:** `TestBenchTransport`, RTT-alvo 1 ms, outflow, mesmo autorizador/escritor/configuração nos dois caminhos. Oito células: gRPC/Redpanda × malha fechada/aberta a 8k × sem/com carga alheia. A carga alheia é um produtor real limitado a 20k registros/s de 1 KB em `foreign-events`, sem spin loop. Registrar taxa alcançada. Não chamar shared broker de destino aprovado; é célula de interferência.

Backlog pausa execução por 5 s com deadline bench-local de 2 s. Julgar cada pedido pelo horário da sua decisão: os recebidos nos últimos 2 s da pausa ainda podem estar válidos ao retomar. Somente os já vencidos, sem resultado anterior, devem expirar. Acrescentar replay de aprovado depois do prazo, original ainda `unknown` depois do prazo e COMMIT iniciado antes que recebe ACK depois. Nenhuma dessas três situações vira recusa financeira automática.

Falhas, 20 rodadas internas por cenário com reset:
- Worker cai após commit de dinheiro/offset/outbox e antes da resposta. Reiniciar worker e relay; o seek pula o lote commitado. Sem republicar pedidos, todos os callers ainda esperando recebem seus resultados pelo outbox.
- Relay cai após confirmação do publish e antes da marca de entrega. Reinício pode publicar duplicata; receptor deduplica e conservação não muda.
- Transferir de fato a partição de A para B, por saída/remoção controlada de A do grupo, verificando assignment, em vez de apenas iniciar B e supor rebalance. A pausado em `beforeBegin` perde epoch após takeover SQL de B e não grava. Variante separada pausa A com lock antes do COMMIT: B espera, A termina ou aborta, então B assume e recarrega offset. Não esperar B avançar enquanto A detém o lock.
- Perder ACK de produce ou tornar broker indisponível durante entrega. Espera do caller é limitada e resultado é `unknown` sob a mesma identidade; callback tardio e retry não duplicam dinheiro.

Verificar conjunto de aprovações duráveis, conservação, offset por registros realmente aceitos, resposta de cada caller pendente e ausência de obrigações abandonadas. Offset não equivale ao número de chamadas Submit: retry/ACK perdido pode produzir registros duplicados. Depois da restauração, esperar drain dentro do limite, sem misturar `unknown` com aprovação.

**Files:**
- Modify: `tests/bench/engine/bench_test.go`, `tests/bench/engine/transport_redpanda.go`
- Modify: `tests/bench/engine/outbox_relay.go`, `tests/bench/engine/transport_redpanda_test.go`
- Modify: `mk/tests.mk`

**Verification:** `BENCH_BLOCK=transport make test-bench-engine` executa `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBenchTransport$' -timeout 45m -v`: 8 × 270 s = 36 min mais margem. Falhas separadas: `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestTransportBacklog|TestTransportRecovery' -timeout 120m -v`, até 60 s por rodada/cenário, 20 rodadas e margem de setup. `golangci-lint run --build-tags bench ./tests/bench/engine/`.

**Done when:** backlog classificado por pedido; respostas recuperadas sem republicação artificial; transferências reais e locks em voo exercitados. HA do broker continua não medida.

### Epic 1.5: Corrida longa e relatório go/no-go

**Goal:** testar estabilidade dos candidatos elegíveis e apresentar recomendação, não escolher arquitetura automaticamente.
**Scope:** `tests/bench/engine/`, `docs/plans/`.
**Dependencies:** Epics 1.3 e 1.4.
**Done when:** relatório rastreável e decisão do Fred sobre a Fase 2.
**Status:** Pending

#### Task 1.5.1: Corrida de 30 minutos

- [ ] Done

**Context:** conta quente, outbox, checkpoints e leituras longas podem alterar desempenho sustentado.

**Implementation vision:** `TestSoak30m`, gate `BENCH_SOAK=1`, exige escritor e transporte explícitos, sem default owner. Rodar candidato elegível em mixed, RTT-alvo 1 ms, malha aberta de 8k durante 30 min. Cada candidato ainda comparado recebe seu próprio comando. A cada 5 min, leitor abre `REPEATABLE READ` e varre operações mantendo snapshot por 60 s, com contexto limitado; esse leitor é carga real do mecanismo, não CPU sintética.

Amostras por minuto: aprovações duráveis/s, recusas/expiry/unknown, p99, backlog/drain, WAL/FPI, checkpoints, flush lag, dead tuples/HOT/autovacuum e tamanho das tabelas, inclusive intenção e outbox. Política de relay e serviços simulados ou não exercitados ficam declarados. Registrar CPU/GC sem presumir gargalo. Gates: 8k aprovadas duráveis/s sustentadas, backlog estável, drenagem dentro do limite e p99 final ≤ 1,5× o quinto minuto. HOT e bloat são diagnósticos, não limiares arbitrários que substituem esses gates. Conservação e unicidade verificadas no fim.

**Files:**
- Create: `tests/bench/engine/soak_test.go`

**Verification:** `BENCH_SOAK=1 BENCH_WRITER="$writer" BENCH_TRANSPORT="$transport" go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestSoak30m$' -timeout 45m -v`. Definir antes `writer` como owner ou ownerless e `transport` como grpc ou redpanda, conforme o candidato declarado; valores ausentes falham. Uma rodada completa por candidato.

**Done when:** amostras e drain completos; se candidato não sustenta a meta, registrar no-go, não substituir aprovações por recusas.

#### Task 1.5.2: Relatório e recomendação

- [ ] Done

**Context:** o relatório informa as escolhas do Fred e limita cada conclusão ao ambiente medido.

**Implementation vision:** `docs/plans/2026-09-30-balance-engine-results.md` reúne matrizes, configurações, payload/índices/outbox, rodadas de falha, multikey, lease, tempos de takeover, backlog, resposta e soak. Cada número aponta sua célula. Separar microbenchmark local de transporte entre processos; PG17 de RDS; RF1 de broker HA; referência experimental de compromisso de produto.

Elegibilidade vem antes do ranking: correção/replay/durabilidade passam, 8k aprovações duráveis/s sustentadas, backlog/drain estáveis e metas de latência/soak atendidas. Se nenhum candidato atende, no-go; não declarar vencedor a 80% de uma meta não alcançada. Entre candidatos elegíveis, ownerless a pelo menos 80% do throughput do owner favorece simplicidade; p99/p999 e custo operacional informam recomendação. Essa heurística não aprova arquitetura.

Redpanda só é recomendado no caminho síncrono se cumprir correção, meta e recuperação de resposta, e justificar o custo diante de gRPC no MESMO escritor. TPS igual ou maior e p99 menor são evidências, não autorização automática da definição 8. Reportar células não medidas e limitações. Atualizar `Phase Overview` e acrescentar resultado da Fase 1 somente após execução real; decisões 1 a 11 mantêm seu estado até Fred responder.

**Files:**
- Create: `docs/plans/2026-09-30-balance-engine-results.md`
- Modify: `docs/plans/2026-09-30-balance-engine-postgres-truth.md`

**Verification:** leitura humana das evidências, origem dos números e gates. Nenhum teste parseia a prosa do plano ou relatório.

**Done when:** Fred recebe go/no-go com limites e autoriza ou recusa a Fase 2.

---

## Phase 2: Protocolo e adaptador experimental

Depende da Fase 1 e das definições indicadas em cada epic. Esta fase não habilita clientes: create v2 isolado ainda não entrega autoridade de leitura nem paridade dos demais caminhos.

### Epic 2.1: Especificação do protocolo

**Goal:** especificar identidade/fingerprint antes da decisão, estado provisório, escritor e transporte escolhidos, deadline público, COMMIT incerto e barreira WAL de replay. Fixar controles, todos os writers e consistência do reload; contrato de hospedagem e comportamento degradado; resposta/replay v1/v2 e ST/MT; guard global de revert; obrigações de metadata, fee debt, eventos e Tracer. Reserva aplicada sem commit SQL exige resolução durável de tentativa, distinta do outbox de sucesso. Proibir confirmar/liberar reserva enquanto SQL estiver `unknown`. Estudar particionamento temporal sem impor migração: UNIQUE no PG exige a chave de partição; preservar unicidade global de revert, identidades e FKs se a opção for aprovada, sem adicionar tabelas de arquitetura por default. Estudos de réplica de leitura não mudam autoridade nem queries sem decisão.
**Scope:** `docs/architecture/`.
**Dependencies:** Phase 1; definições 1, 2, 3, 4, 7, 8, 10 e 11 respondidas.
**Done when:** protocolo aprovado por Fred, com compromissos públicos e hipóteses experimentais separados.
**Status:** Pending

### Epic 2.2: Migrações aprovadas do escritor

**Goal:** implementar somente estruturas aprovadas em 2.1: intenção e resultado, outbox, controles, fence se owner e guard de revert por unique index em `transaction.parent_transaction_id` no esquema não particionado. Alterações de fillfactor dependem da medição. Particionamento não é entrega obrigatória; se escolhido depois, necessita plano próprio de unicidade global/FKs e custo de cutover antes da migração. Não promover o DDL simplificado da bancada a modelo de produto automaticamente.
**Scope:** `components/ledger/migrations/transaction/`, `tests/integration/`.
**Dependencies:** Epic 2.1; definição 2 refletida nas garantias de persistência.
**Done when:** migrações validadas em base existente, testes e linter verdes, guard global e identidade preservados.
**Status:** Pending

### Epic 2.3: Adaptador Postgres experimental para create v2

**Goal:** implementar `command.Engine` para create v2 JSON, inflow e outflow com algoritmo aprovado, ids de saldo e outbox completo do caminho. SQL da completion entra no commit financeiro; entregas externas seguem as obrigações duráveis especificadas. Nenhuma decisão financeira é repetida por recuperação. Resposta perdida ou falha depois de enviar COMMIT retorna resultado incerto sob a mesma identidade, não promessa de 5xx sem movimento. Integração do Tracer segue a definição 10, sem fixar gRPC ou in-process por conveniência. Telemetria segue o padrão aprovado.
**Scope:** `components/ledger/internal/adapters/postgres/engine/`, `components/ledger/internal/services/command/`.
**Dependencies:** Epic 2.2; definições 2, 3, 4, 7, 8, 9, 10 e 11 respondidas.
**Done when:** contrato de accounting e integração create v2 verdes no modo experimental; falha comprovadamente anterior ao commit difere de resultado incerto; 201 exige durabilidade confirmada.
**Status:** Pending

### Epic 2.4: Seleção experimental e gate de tenant

**Goal:** permitir selecionar motor somente em testes/ambientes experimentais nesta fase. A disponibilidade para cliente depende da Fase 3 completa e da barreira da Fase 4. Se `settings.engine` representar o seletor, não expor flip independente por ledger; um tenant e todos os ledgers de um grupo cross-ledger não podem operar com autoridades monetárias mistas. Leitura e escrita mudam juntas no cutover. O seletor é temporário e sai na Fase 5.
**Scope:** `components/ledger/internal/bootstrap/`, `components/ledger/internal/services/query/`, `components/ledger/internal/services/command/`.
**Dependencies:** Epic 2.3.
**Done when:** testes selecionam ambos os motores; configuração inválida ou ativação fora do gate falha, sem habilitar clientes pelo create v2 isolado.
**Status:** Pending

---

## Phase 3: Paridade funcional e de leitura

Esta fase cobre contratos públicos existentes, não só operações que já chamam Engine. Definições 7, 10 e 11 são gates da integração e da liberação, não notas de rodapé.

### Epic 3.1: Lifecycle e contratos existentes

**Goal:** preservar v1/v2 e ST/MT em create, pending/commit/cancel, revert, grupos e replay: ids, ordem, status, corpo congelado, scoping e regras de skip específicas de cada versão. NOTED hoje contorna Engine e depende de `write_transaction`; manter sua persistência e resposta antes de retirar o caminho legado. Grupos cross-ledger não misturam autoridades; atomicidade só é local ao banco da transação SQL, não promessa gratuita entre bancos. Revert mantém guard global; recusa do Tracer não cria lançamentos.
**Scope:** `components/ledger/internal/services/command/`, adaptadores do motor e contratos HTTP.
**Dependencies:** Phase 2; definições 4, 7 e 11.
**Done when:** suítes de lifecycle, grupos, NOTED e resposta/replay passam nas duas versões e nos dois modos de tenancy, com resultado incerto preservado.
**Status:** Pending

### Epic 3.2: Todos os writers, controles, fees e Tracer

**Goal:** integrar block/unblock, exceções, limites, fee debt, PATCH de flags/settings, criação de saldo/overdraft, close/delete e demais escritores administrativos à ordenação autoritativa ou invalidação transacional do estado. Provar reload consistente de saldos E controles enquanto esses caminhos operam; versões monetárias sozinhas não cobrem writers atuais. Fee debt mantém projeção Mongo e suas consultas por obrigação durável de outbox, salvo decisão explícita de mover as queries. Integração do Tracer segue definição 10 e cobre reserve aplicado sem commit SQL, SQL unknown e acerto pós-commit; TTL de 5 min/30 dias continua fallback, nunca prova de resolução. A fila em memória só pode sair após tratamento durável desses casos.
**Scope:** adaptadores postgres/engine e mongodb, services command/query/fees, seam do Tracer e writers administrativos.
**Dependencies:** Epic 3.1; definições 1 e 10.
**Done when:** inventário de writers fechado; testes concorrentes de controle/reload, account closing, overdraft, fee debt e reservas sobrevivem a restart sem depender só de TTL ou fila volátil.
**Status:** Pending

### Epic 3.3: Batch atômico e account protection

**Goal:** batch v2, `resolveEngineAdmissions` e grupos usam identidade antes de decisão, locks canônicos e rollback do lote inteiro quando uma decisão dependente fica inválida. Preservar replay congelado do batch e versões de cada saldo, inclusive múltiplas chaves da mesma conta, fees e lifecycle. Nenhuma remoção isolada de posting após conflito de identidade.
**Scope:** `components/ledger/internal/services/command/`, adaptador Postgres.
**Dependencies:** Epic 3.2; definições 7 e 11.
**Done when:** suítes de batch, duplicatas concorrentes, multikey e proteção passam, com paridade de respostas e conservação.
**Status:** Pending

### Epic 3.4: Autoridade de leitura e chaos

**Goal:** leitura de saldo, transação, operações e replay usa a autoridade Postgres dos tenants migrados, sem seed de dinheiro no Valkey nem mistura entre ledgers. Alias resolve na borda. Metadata e projeção Mongo de fee debt têm obrigações e comportamento de leitura definidos; replay não recompõe resposta congelada a partir do estado mutável atual. Preservar point-in-time e contratos existentes. Validar v1/v2, ST/MT, NOTED, falhas e concorrência ponta a ponta antes de liberar cutover.
**Scope:** `components/ledger/internal/services/query/`, contratos HTTP, `tests/integration/`, `tests/chaos/`.
**Dependencies:** Epic 3.3; definições 7, 10 e 11 implementadas.
**Done when:** integração e chaos verdes; leitura e paridade completas autorizam preparar, não executar automaticamente, a Fase 4. Avaliação de Product Console registrada antes de qualquer PR futuro, com entrega pareada se houver impacto.
**Status:** Pending

---

## Phase 4: Cutover por tenant

Nenhuma operação de produção está autorizada por esta descrição. Ambiente, acesso, janela e execução exigem autorização específica.

### Epic 4.1: Barreira e reconciliação

**Goal:** comando de cutover pausa TODOS os writers financeiros e de controles do tenant, incluindo API, administração, consumers, recovery e sync; drena trabalhos antigos e resolve resultados incertos. Reconcilia saldos, operações, pending, fee debt, exceções e replay contra trilha e evidência, lendo formatos históricos quando necessário. Igualdade entre cópia de Valkey e SQL não prova correção de um saldo já bifurcado. Tenant inconsistente fica bloqueado para investigação, sem reparo monetário automático. Só após reconciliação e validação do contrato de hospedagem, copiar/importar estado e janelas de replay, mudar leitura/escrita de todos os ledgers relacionados e retomar sem autoridade mista.
**Scope:** `components/ledger/cmd/cutover/`, adaptadores redis/postgres e controle dos writers.
**Dependencies:** Phase 3 completa; definições 2, 3, 5, 7, 8, 10 e 11 atendidas.
**Done when:** ensaio local prova barreira completa, reconciliação com evidência, bloqueio de tenant inconsistente e flip conjunto de leitura/escrita.
**Status:** Pending

### Epic 4.2: Ensaio em homologação

**Goal:** executar em ambiente autorizado com dados de teste ou cópia sanitizada autorizada, infraestrutura identificada e durabilidade aprovada. Antes do primeiro commit novo, demonstrar retorno seguro ao motor anterior; depois dele, não reabrir Valkey antigo como autoridade. Procedimento posterior exige reconciliação e decisão operacional explícita, não re-seed automático como rollback presumido.
**Scope:** homologação e runbook.
**Dependencies:** Epic 4.1; autorização de ambiente, dados e execução.
**Done when:** ensaio registrado com indisponibilidade, drain, evidência, falhas e recuperação; nenhuma conclusão baseada só em igualdade de cópias.
**Status:** Pending

### Epic 4.3: Runbook aprovado

**Goal:** documentar contrato de hospedagem da definição 2, checagens da aplicação e evidências operacionais separadas, dimensionamento medido, barreira, reconciliação, bloqueio de inconsistência e limites de rollback. Eventos de degradação do provedor não podem ser substituídos por `SHOW synchronous_commit`. Distinguir failover do banco de takeover do owner, quando houver.
**Scope:** `docs/runbooks/`.
**Dependencies:** Epic 4.2.
**Done when:** operador independente executa o ensaio autorizado pelo runbook; rollout de clientes continua sujeito à sua janela aprovada.
**Status:** Pending

---

## Phase 5: Deleção

### Epic 5.1: Remover caminhos substituídos

**Goal:** após todos os tenants migrarem, remover Lua, Valkey financeiro/idempotência migrada, write-behind, recovery, backup queue, balance sync, async interno e seletor temporário. Preservar substituto de NOTED e contratos v1/v2 ST/MT. Antes de apagar recovery, comprovar cobertura e drain de metadata, fee debt, eventos e Tracer; antes de apagar settle em memória, comprovar resolução durável de tentativas sem commit e reservas ainda incertas. 202 futuro continua separado da deleção interna. Atualizar a branch com develop antes de decidir o que apagar.
**Scope:** `components/ledger/`, `pkg/`.
**Dependencies:** todos os tenants migrados na Phase 4; definição 10 implementada; inventários de obrigações drenados.
**Done when:** caminhos de execução antigos ausentes, contratos/integração/chaos e `make ci` verdes. Busca textual é auxílio, não prova suficiente de remoção ou paridade.
**Status:** Pending

### Epic 5.2: Documentação e observabilidade

**Goal:** atualizar `docs/architecture/engine.md`, scoping, llms, guias locais, métricas e dashboards para o motor único, separando obrigações SQL e externas, sucesso e unknown, configuração de hospedagem e comportamento público. Referências históricas podem permanecer identificadas como históricas, não como caminho atual.
**Scope:** `docs/`, raiz, `components/infra/grafana/`.
**Dependencies:** Epic 5.1.
**Done when:** documentação descreve o comportamento verificado e suas limitações, sem testes que parseiem prosa.
**Status:** Pending

### Epic 5.3: Delta de linhas

**Goal:** reportar delta da branch contra develop, separado em prod, test e docs; justificar crescimento pelo comportamento entregue.
**Scope:** este plano.
**Dependencies:** Epic 5.2.
**Done when:** delta e entrega constam no resultado final da `Phase Overview` e no relato ao Fred.
**Status:** Pending
