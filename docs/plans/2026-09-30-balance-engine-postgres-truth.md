# Balance Engine on Postgres Truth Implementation Plan

> **For implementers:** Use ring-default:dispatching-workflows. Elaborate the
> current phase against the real code, execute a reviewed workflow, then elaborate
> the next phase. Alternatively, use ring-dev-team:running-dev-cycle.
> This document is the living source of truth for all five phases.

**Autorização:** somente documentos. A Fase 1 estar `Detailed` não autoriza implementar nem executar a bancada. Código, produção, provisionamento e PRs exigem autorização posterior.

**Goal:** tornar o Postgres do tenant a única verdade do dinheiro, com decisão, trilha e resultado no mesmo commit síncrono replicado antes do sucesso público. Esse requisito está aprovado; domínios de falha, evidência de hospedagem e postura degradada continuam na definição 2.

**Architecture:** um autorizador resolve identidades antes de decidir e grava cada lote em uma transação SQL. A bancada compara dono com fence e lotes concorrentes com row locks; chamada em processo, gRPC real e Redpanda usam o mesmo escritor. Postgres continua opção permanente sem infraestrutura nova obrigatória. Entregar sem major exige paridade, barreira e preservação dos contratos suportados, inclusive configuração. Incompatibilidade real bloqueia a remoção até evidência e decisão explícita, sem forçar versão.

**Tech Stack:** bancada em Go 1.27, linguagem do produto ainda na definição 9; PostgreSQL 17 local com comparação limitada em PG16, `pgx/v5` v5.11.0, `squirrel`, `golang-migrate`, Docker Compose e toxiproxy 2.12.0. Redpanda `v24.2.7` e `franz-go` v1.21.6 somente nos experimentos de broker. Registrar versões e imagens efetivas; PG16 local não equivale a RDS16.

**Execução futura:** worktree única `/srv/worktrees/balance-engine`, branch única `feature/balance-engine`. Nenhuma outra branch ou PR sem pedido do Fred. Quando a execução for autorizada, um workflow por fase e revisor diferente do implementador. Commits e pushes ficam com a sessão coordenadora, com o trailer `X-Lerian-Ref: 0x1`.

**Fontes:** alfarrábio `https://alfarrabio.lerian.net/reports/fred/midaz-motor-de-saldo`; `docs/architecture/engine.md`; reunião "Direcionamento Midaz" de 2026-10-14. Direção aprovada: intenção/recusa visíveis, corte antecipado inclusive sem gateway, autorizador por lote independente do transporte, nenhuma infraestrutura nova obrigatória, Redpanda dedicado quando usado, Tracer como módulo, alias por id, telemetria enxuta, estudo de leitura e desenho/migração de particionamento com prova de compatibilidade. As definições pendentes não são decisões aprovadas.

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | Escritores/transportes comparáveis, aprovações duráveis, cortes, proteção e falhas; PG16/17 e hipótese D medidos; revisão técnica antes de recomendar implementação | 1.1, 1.2, 1.3, 1.4, 1.5 | Detailed |
| 2 | Protocolo e esquema com particionamento provados; create v2 experimental e estudo de read model, sem habilitar clientes | 2.1, 2.2, 2.3, 2.4, 2.5 | Epic-level |
| 3 | Paridade v1/v2 ST/MT, NOTED, lifecycle, dívida SQL, replay/leitura e intenção/recusa visíveis no Console | 3.1, 3.2, 3.3, 3.4, 3.5 | Epic-level |
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

Os 8k vêm da observação relatada por Fred de uma conta quente de PI com limites reais, não de medição deste plano. O alvo é reproduzir aprovações duráveis sob essas restrições, não perseguir 100k TPS sintéticos. Revisão técnica de Felipe/Clara e confronto com os spikes disponíveis precedem recomendação de implementação; nenhum review nem resultado do time está concluído aqui.

| Medida | Alvo ou interpretação |
|---|---|
| TPS sustentado em uma conta limitada | 8.000 aprovações distintas com commit durável confirmado por segundo, backlog estável e drenagem medida |
| p99 do resultado, RTT de replicação medido de 1 ms | ≤ 15 ms, hipótese a validar; separar núcleo local, gRPC e Redpanda |
| Perda ou dupla aplicação de identidades aprovadas após falha | 0 nas 20 rodadas de cada cenário aplicável |
| Takeover com 1M saldos e 1M controles | Medir contra referência experimental de 5 s; orçamento público depende da definição 3 |
| Soak de 30 min | 8k aprovações duráveis/s sustentadas; p99 final ≤ 1,5× p99 do quinto minuto |
| Corte de admissão | Zero trabalho financeiro novo vencido na entrada/pós-espera; corte pre-COMMIT aborta lote inteiro. COMMIT já enviado e replay tardios são registrados, nunca reclassificados por prazo |
| Transporte | Broker ACK, confirmação SQL e resposta medidos separadamente; sem perda de respostas pendentes após recuperação |

Recusas, expirações, duplicatas e `unknown` não contam para a meta de 8k. Célula HA, RDS ou transporte não executada fica "não medido". RF1 no Redpanda mede recuperação de worker, não HA do broker.

## Definições pendentes do Fred

As definições abaixo permanecem abertas. A Fase 1 compara hipóteses experimentais sem escolher contratos públicos. Seus resultados valem para a configuração medida; decisões de produto e autorização de execução permanecem separadas.

| # | Decisão | Recomendação e custo | Bloqueia |
|---|---|---|---|
| 1 | Localização e ownership de block, limites, exceções e dívida de tarifa | Reunir autoridade financeira no banco do commit, incluindo criação, quitação e reversão da dívida. Definir quem escreve e quem apenas projeta consultas; Mongo não é autoridade. Custa mudar fronteiras com onboarding/fees. `account_control` é hipótese do bench, não decisão de cotas em fees | Epics 2.1, 2.2, 2.5, 3.2 |
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
| 12 | Autorizador no processo do ledger ou executável separado | Comparar baseline in-process com gRPC real e Redpanda no mesmo escritor. In-process reduz operação; separação permite escala/isolamento e custa roteamento e falhas de rede. Independente da integração do Tracer; nenhuma fronteira de deploy escolhida | Epics 2.1, 2.3, 2.4 e Phase 4 |

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

O bench mantém `unknown` em timeout, perda de resposta ou aviso de cancelamento da espera síncrona. O tratamento observa também notices do COMMIT; retorno sem erro sozinho não basta se a espera síncrona foi cancelada. Antes de reconhecer um resultado encontrado por replay, a mesma transação finalizadora incrementa `engine_request.durability_touch` e faz um COMMIT síncrono com escrita real de WAL, no primário autoritativo. Esse WAL posterior cobre o resultado anterior na ordem de replicação. `SELECT` seguido de COMMIT read-only não é barreira. Sem confirmação da nova barreira, permanece `unknown`. A prova vale para o primário único e a configuração de réplica da célula, não para hospedagens ou falhas não exercitadas.

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

**Context:** `components/ledger/migrations/transaction/` chega a 000046 neste HEAD. 44 acrescenta scheme; 45, optional em operation_transaction_route; 46, índice parcial não único de operation por balance_id/balance_version_after. A 46 usa `CREATE INDEX CONCURRENTLY`: executar cada arquivo via `golang-migrate`, sem transação externa nem concatenar outros DDL no mesmo Exec, seguindo `tests/integration/ledger_migrations_integration_test.go:110-113`. `operation.snapshot`, em `pkg/mmodel/operation.go`, contém `overdraftUsedBefore` e `overdraftUsedAfter`, não um create de 1 KB.

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

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBootstrap$' -timeout 30m -v`, com DSNs do compose. Conferir migração 46 e seu índice parcial, quatro tabelas bench, índice de pendências, totais de saldos/controles e duas execuções internas do bootstrap.

**Done when:** bootstrap repetível; tempos de 10k e 1M registrados, com limites por execução de 60 s e 10 min para a semeadura.

### Epic 1.2: Harness de carga com os dois escritores

**Goal:** comparação local do núcleo com a mesma carga, regras de lote, SQL, outbox e replicação.
**Scope:** `tests/bench/engine/`, `mk/tests.mk`, `.gitignore`.
**Dependencies:** Epic 1.1.
**Done when:** correctness gates passam e os blocos de medição produzem resultados reproduzíveis.
**Status:** Pending

#### Task 1.2.1: Gerador e métricas

- [ ] Done

**Context:** conta limitada quente e contraparte aleatória medem núcleo, não API. A carga de 8k deriva da necessidade relatada por Fred.

**Implementation vision:** `Writer` tem `Submit`/`Close`; pedidos carregam identidades, fingerprint, postings, `scheduledAt`, orçamento do caller e corte de admissão. Contadores separam aprovação durável distinta, recusa/motivo, expiração, replay, conflito e `unknown`.

Malha fechada usa 256 produtores. Aberta agenda `start + i/rate`, 8.000/s, e mede desde `scheduledAt`, incluindo atraso do gerador. Registrar envios, decisões, backlog/drain, `dbAckAt-scheduledAt` e `repliedAt-scheduledAt`. Cargas outflow/inflow/mixed, inteiros de 1 a 100; feeLeg divide crédito numa terceira perna. Limite desce só no débito. Fixtures de esgotamento concorrente, block/flags e multikey comprovam pisos por posting e controle compartilhado. Fast paths não contam na meta.

Nos testes dos escritores, reconstruir cada decisão aprovada na ordem/versão persistida e conferir piso, flags e limite, além da soma final. Submeter débitos concorrentes que esgotam saldo ou limite e verificar aprovações e recusas efetivas. Fila interrompida sem prova mantém `unknown`. Gerador nulo mede apenas a capacidade do gerador.

**Files:**
- Create: `tests/bench/engine/writer.go`, `tests/bench/engine/workload.go`, `tests/bench/engine/metrics.go`
- Test: `tests/bench/engine/workload_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestWorkloadKeysUnique|TestOpenLoopSchedule|TestPercentiles|TestWorkloadNullWriter' -timeout 5m -v`. Calendário/percentis determinísticos; medição limitada registra 20k envios/s ao nulo e atraso alcançado. As Tasks 1.2.2/1.2.3 verificam proteção nos escritores reais.

**Done when:** contadores distinguem resultados e fornecem identidade, ordem/versão e valores para as asserções por decisão dos escritores.

#### Task 1.2.2: Escritor com dono

- [ ] Done

**Context:** um dono por tenant, um lote em voo, estado provisório privado. Admissão na entrada e protocolo comum são obrigatórios.

**Implementation vision:** `writer_owner.go` drena até `maxBatch=256`. `takeover()` usa `UPDATE ... WHERE tenant_id=$tenant AND lease_until <= clock_timestamp()`, troca owner e incrementa epoch. Renovação exige owner/epoch e `lease_until > clock_timestamp()`, nunca `now()`. Lease de 3 s e renovação de 1 s são parâmetros de bench; renovar durante reload.

Carregar saldos por `balance.id` e controles pela chave composta, páginas de 50k. Mapa monetário por balanceID. Bench exclui outros writers durante reload; concorrência administrativa é gate da Epic 3.2. Erro de página/lease aborta sem mapa parcial.

`beforeBegin` antecede decisão/locks. Abrir SQL, renovar/travar fence válida, reclamar identidades canônicas, resolver anteriores, revalidar admissão e decidir sobre cópia privada. Inserir transaction/operation em lote, atualizar saldos/controles por IDs e versões esperadas, exigir todas as linhas, finalizar resultado/outbox. Crédito novo pode financiar débito posterior; replay nunca financia de novo. Divergência de versão aborta tudo e encerra dono para reload. Corte cruzado antes do COMMIT exige rollback/recomputação integral do contrato comum.

Confirmação publica memória e resposta. Erro/notice de replicação incerta mantém `unknown`, descarta cópia e encerra dono; sucessor resolve identidades com barreira WAL. Ticker não renova lease expirado. Takeover espera lock SQL em voo; expiry não revoga retroativamente o lote.

**Files:**
- Create: `tests/bench/engine/writer_owner.go`
- Test: `tests/bench/engine/writer_owner_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestOwnerWriterConservation|TestOwnerWriterIdentity|TestOwnerWriterMultiKey|TestOwnerWriterProtection|TestOwnerWriterAdmission' -timeout 30m -v`. Cinco rodadas com reset, 200k pedidos na conservação, join 1:1 de aprovação/transação e outbox completo. Cobrir duplicatas no lote/concurrentes, payload divergente, replay de crédito antes de débito, rollback inteiro, replay vencido, block/flags, esgotamento concorrente e controle multikey compartilhado. Pausas controladas na entrada, após locks e antes do COMMIT demonstram cada corte e nenhuma escrita financeira do lote abortado.

**Done when:** pisos por aprovação passam, rollback/unknown não publicam estado e replay não inicia dinheiro.

#### Task 1.2.3: Escritor sem dono

- [ ] Done

**Context:** baseline sem cache/lease; row locks de saldo não substituem exclusão de identidades.

**Implementation vision:** `writer_ownerless.go` usa 32 writers, mesma drenagem/batch. Abrir SQL, reclamar identidades em ordem canônica, resolver replay, travar saldos ordenados por balance.id e controles pela chave composta. Revalidar admissão, decidir na ordem original sobre cópia das linhas e persistir o mesmo SQL/outbox. Replay isolado também escreve touch WAL.

Ordem uniforme: offset de transporte, fence se houver, identidades, saldos, controles. `40P01` aborta tudo e permite uma recomputação limitada das mesmas identidades ainda admissíveis; nunca retry financeiro de resultado incerto. Corte pre-COMMIT ou conflito tardio invalida o lote inteiro. Depois de enviar COMMIT, timeout não prova rollback.

**Files:**
- Create: `tests/bench/engine/writer_ownerless.go`
- Test: `tests/bench/engine/writer_ownerless_test.go`

**Verification:** `go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestOwnerlessWriterConservation|TestOwnerlessWriterIdentity|TestOwnerlessWriterMultiKey|TestOwnerlessWriterProtection|TestOwnerlessWriterAdmission' -timeout 30m -v`. Mesmas cinco rodadas, volume e asserções da Task 1.2.2; registrar deadlocks/retries.

**Done when:** os mesmos gates de identidade, admissão e proteção passam antes do ranking de desempenho.

#### Task 1.2.4: Matriz local e comandos limitados

- [ ] Done

**Context:** latência injetada não é RTT medido; PG16 local não reproduz RDS. Tag `bench` fica fora dos shards e CI existentes.

**Implementation vision:** `toxiproxy.go` remove toxics antigos e configura `pg-repl`; erro aborta a célula. Registrar atraso, direção e RTT observado. Cada célula limita bootstrap de 10k a 60 s, aquece 60 s, mede 120 s e drena até 30 s. Identidades do aquecimento ficam fora da medição.

| Bloco | Células | Conteúdo |
|---|---|---|
| A | 18 | owner/ownerless × RTT-alvo 0/1/10 ms × outflow/inflow/mixed, malha fechada |
| B | 18 | Mesma matriz, malha aberta a 8.000/s |
| C | 20 | RTT-alvo 1 ms, outflow: owner batch 1/8/64/256; ownerless writers 1/4/32 × mesmos batches; esgotamento e feeLeg para ambos |
| V | 8 | PG16/PG17 × owner/ownerless × aberta/fechada; RTT-alvo 1 ms, outflow, batch 256, ownerless 32 writers |
| D | 4 opcionais | Dois escritores × aberta/fechada em RDS temporário autorizado |

A/B/C somam 56 células. Make usa `go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBenchHotAccount$' -timeout 90m -v` por bloco A/B, 100m em C e 45m em V/D. Cada célula permite até 270 s; V permite mais 120 s por troca de versão. `BENCH_MATRIX=full` exige bloco explícito. Quick faz uma célula owner, RTT-alvo 1 ms, outflow, medição de 20 s e timeout de 5 min. Sem cache de teste.

Em V, parametrizar imagem PG16/17, capturar `SELECT version()` e digest real, hardware e configs; volumes separados por major, nunca abrir volume 17 com binário 16. Rodar versões sequencialmente, mesma fixture e índices até 46, sem multiplicar toda a matriz nem declarar equivalência com RDS. Arquivos compose/env/bootstrap da Epic 1.1 recebem essa parametrização.

Por célula: aprovações duráveis, outros outcomes, p50/p99/p999, lote real, locks, backlog/drain, WAL/FPI, checkpoints, flush lag, CPU/GC. Habilitar e registrar `track_wal_io_timing=on`, `track_io_timing=on` e `wal_sync_method`; medir deltas de `pg_stat_wal` como `wal_sync`/`wal_sync_time` conforme método e versão. Capturar stats_reset; contador indisponível ou sem temporização habilitada é "não medido", nunca zero inventado. Não chamar fsync de tempo fim a fim de replicação. Mesmo outbox, índices e política nos escritores; backlog crescente torna candidato inelegível. Saída em `tests/bench/engine/out/<timestamp>.md`, ignorada pelo git.

Make sobe compose, exporta DSNs e limpa apenas recursos descartáveis autorizados. D exige autorização separada, acesso e instância não produtiva. Células locais não medem HTTP, Tracer nem Mongo; transporte fica na Epic 1.4.

**Files:**
- Create: `tests/bench/engine/toxiproxy.go`, `tests/bench/engine/bench_test.go`
- Modify: `tests/bench/engine/docker-compose.yml`, `tests/bench/engine/env.go`, `tests/bench/engine/bootstrap.go`
- Modify: `mk/tests.mk`, `.gitignore`

**Verification:** `BENCH_MATRIX=quick make test-bench-engine`; `make test-gate-selection`; `golangci-lint run --build-tags bench ./tests/bench/engine/`. Depois, comandos separados `BENCH_MATRIX=full BENCH_BLOCK=A make test-bench-engine`, B, C e V. Conferir seleção sem teste que parseie docs.

**Done when:** baseline PG16/17 e blocos registrados ou explicitamente não medidos, com versões, limites, custo de índices e métricas suportadas.

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

Comparar o conjunto de IDs que receberam aprovação durável, não apenas contagens: todos devem existir com o resultado e a transação correspondente. Verificar no máximo uma aplicação por identidade, conservação, outbox e resolução de `unknown` após restauração dentro do orçamento. Repetir inclui replay depois do deadline e cancel da espera síncrona com resultado local visível. Registrar apenas os resultados observados no ambiente experimental.

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

**Context:** deadline, offset e resposta são provas distintas da medição de latência.

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
**Done when:** relatório rastreável e decisão do Fred sobre a Fase 2.
**Status:** Pending

#### Task 1.5.1: Corrida de 30 minutos

- [ ] Done

**Context:** conta quente, outbox, checkpoints e snapshots longos afetam sustentação.

**Implementation vision:** `TestSoak30m`, gate `BENCH_SOAK=1`, exige writer/transporte explícitos. Candidato elegível roda mixed, RTT-alvo 1 ms, aberta de 8k por 30 min. Cada candidato tem comando próprio. A cada 5 min, leitor `REPEATABLE READ` varre operações com snapshot limitado a 60 s. Carga real do mecanismo, nenhuma CPU sintética.

Amostras por minuto: aprovações duráveis/s, outros outcomes, p99, backlog/drain, WAL/FPI, fsync e temporização suportada de 1.2.4, checkpoints, flush lag, dead tuples/HOT/autovacuum, tamanhos de intenção/outbox. Registrar idade de `relfrozenxid`/`datfrozenxid`, configurações de freeze/wraparound e snapshots que retêm xmin quando relevantes; 30 min não provam segurança de retenção indefinida. Sem purga durante o bench, explicitar acúmulo, relay e serviços não exercitados; retenção de produto é gate 2.2, sem TTL inventado. Gates: 8k aprovações duráveis/s, backlog estável, drain limitado e p99 final ≤ 1,5× quinto minuto. Reconstrução final verifica pisos/flags por decisão, limite compartilhado, conservação e unicidade.

**Files:**
- Create: `tests/bench/engine/soak_test.go`

**Verification:** `BENCH_SOAK=1 BENCH_WRITER="$writer" BENCH_TRANSPORT="$transport" go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestSoak30m$' -timeout 45m -v`. Definir writer owner/ownerless e transporte in-process/grpc/redpanda; ausência falha. Uma rodada por candidato declarado.

**Done when:** amostras/drain completos; candidato abaixo da meta recebe no-go, sem contar recusas como aprovação.

#### Task 1.5.2: Relatório, revisão técnica e recomendação

- [ ] Done

**Context:** a recomendação depende das medições e da revisão técnica do time, não da preferência por uma topologia.

**Implementation vision:** relatório `docs/plans/2026-09-30-balance-engine-results.md` reúne matrizes, versões/configs, índices até 46, payload/outbox, proteção por decisão, multikey, cortes/COMMIT tardio, replay/WAL, lease, takeover, resposta, soak e comparação D/E. Cada número aponta célula. Separar núcleo de transporte e HTTP completo não medido, PG16/17 local de RDS, RF1 de HA. D mostra commit-only/receipt-only/unknown sem promessa de reconciliação financeira.

Antes de recomendar implementação, Felipe e Clara revisam provas, limitações e resultados dos spikes disponíveis. Registrar evidência recebida, divergências e o que permanece não verificado; ausência não vira concordância ou número inventado. Sem essa revisão, recomendação fica bloqueada.

Elegibilidade exige correção, replay/durabilidade, proteção por decisão, 8k aprovações duráveis/s e gates de backlog/latência/soak. Sem candidato elegível, no-go. Entre elegíveis, ownerless a 80% do throughput owner favorece simplicidade, considerando caudas e operação; heurística não escolhe arquitetura. Redpanda precisa justificar custo contra in-process e gRPC com MESMO escritor. Publicação independente D não recebe selo zero perda só por passar caminho normal.

Comparar também transições entre processos/sistemas, pontos de falha e obrigações de recuperação. Reportar não medidos e opção permanente só Postgres. Atualizar `Phase Overview` somente após execução; definições 1–12 continuam abertas até decisão explícita. Revisão técnica não substitui aprovação de produto nem autoriza Phase 2.

**Files:**
- Create: `docs/plans/2026-09-30-balance-engine-results.md`
- Modify: `docs/plans/2026-09-30-balance-engine-postgres-truth.md`

**Verification:** revisão humana da origem dos números, cobertura e limites, seguida de Felipe/Clara e confronto dos spikes. Nenhum teste parseia prosa.

**Done when:** revisão técnica registrada; Fred recebe recomendação fundamentada e autoriza ou recusa Phase 2.

---

## Phase 2: Protocolo e adaptador experimental

Depende das provas da Phase 1, da revisão técnica de Felipe/Clara e das definições abaixo. Não habilita clientes: create v2 isolado não entrega paridade nem autoridade de leitura completa.

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
