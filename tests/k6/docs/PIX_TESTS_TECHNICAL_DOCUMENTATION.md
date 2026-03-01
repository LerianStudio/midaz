# Documentacao Tecnica - Suíte de Testes PIX

## Sumario Executivo

### Estado Atual da Suite de Testes PIX

O projeto k6-midaz possui uma suite de testes de carga robusta e bem estruturada para validacao de operacoes PIX, composta por **71 arquivos JavaScript** organizados em **3 suites principais**:

1. **PIX Cash-In** (Recebimento via Webhooks BTG)
2. **PIX Indirect BTG** (Cash-Out, Collection, Refund)
3. **PIX Webhook Outbound** (Entrega de Webhooks)

### Pontos Fortes

| Aspecto | Avaliacao |
|---------|-----------|
| **Cobertura de Cenarios** | Excelente - Cobre smoke, load, stress, spike e soak tests |
| **Conformidade BACEN** | Aderente - Inclui testes de 4h+ e SLAs especificos |
| **Metricas Customizadas** | Completas - Segregacao de erros tecnicos vs negocio |
| **Validacao de Duplicidade** | Presente - Testes de deteccao de EndToEndId duplicado |
| **Documentacao** | Boa - READMEs e comandos documentados |
| **Geracao de Payloads** | Robusta - Payloads validos por especificacao BACEN |
| **Idempotencia** | Implementada - Todas as chamadas incluem Idempotency-Key |

### Riscos Identificados

| Risco | Severidade | Descricao |
|-------|------------|-----------|
| **Dados de Teste Sinteticos** | Media | Contas geradas em runtime podem nao existir no CRM real |
| **Valores Simulados em Refund** | Alta | Valores de refund parcial sao simulados, nao correlacionados com cashout real |
| **Dependencia de Mock Server** | Media | Testes Outbound requerem mock server externo |
| **Ausencia de Contract Tests** | Media | Nao ha validacao de schema/contrato das APIs |
| **Think Times Configuraveis** | Baixa | Modo "stress" pode gerar carga irreal |

### Recomendacoes Priorizadas

1. **[ALTA]** Correlacionar valores de refund com cashout real (evitar inconsistencias de validacao)
2. **[ALTA]** Criar pre-provisionamento de contas de teste no ambiente alvo antes dos testes
3. **[MEDIA]** Adicionar testes de contrato (schema validation) para APIs criticas
4. **[MEDIA]** Implementar mock server integrado ou documentar setup do mock externo
5. **[BAIXA]** Adicionar testes de reconciliacao pos-transacao

---

## 1. PIX Cash-In (Recebimento de Pagamentos)

### 1.1 Visao Geral

Suite de testes para validacao do fluxo completo de Cash-In PIX via webhooks BTG Sync.

**Localizacao:** `tests/load/cashin/`

**Arquitetura do Fluxo:**

```
Fase 1: APROVACAO (status=INITIATED)
+-------------+     +------------------------------+     +-----------+
|  BTG Sync   | --> | POST /v1/payment/webhooks/   | --> | CRM/Midaz |
| (INITIATED) |     |      btg/events              |     | Validacao |
+-------------+     +------------------------------+     +-----------+
                                 |
                                 v
                  Response: {approvalId, status: ACCEPTED|DENIED}

Fase 2: LIQUIDACAO (status=CONFIRMED)
+-------------+     +------------------------------+     +-----------+
|  BTG Sync   | --> | POST /v1/transfers/webhooks  | --> |   Midaz   |
| (CONFIRMED) |     |                              |     | Settlement|
+-------------+     +------------------------------+     +-----------+
```

---

### 1.2 Testes Documentados

#### 1.2.1 Test: `cashinMainFlow`

| Campo | Valor |
|-------|-------|
| **Nome** | cashinMainFlow |
| **Tipo** | Carga (smoke/load/stress/spike/soak) |
| **Arquivo** | `tests/load/cashin/run.js:175-194` |
| **Objetivo** | Validar fluxo completo de Cash-In distribuido entre contas de teste |

**Contexto Funcional:**
- Etapa PIX: Recebimento de pagamento (Cash-In)
- Processo: Aprovacao + Liquidacao (2 fases)

**Pre-requisitos:**
- Plugin BR PIX rodando em `PLUGIN_PIX_URL`
- Endpoint `/v1/payment/webhooks/btg/events` disponivel
- Endpoint `/v1/transfers/webhooks` disponivel

**Entradas:**
```javascript
{
  "entity": "PixPayment",
  "status": "INITIATED",
  "endToEndId": "E30306294202601191234567890ABCD", // Formato BACEN
  "transactionIdentification": "txId32CharactersAlphanumeric12",
  "amount": 123.45,
  "paymentType": "IMMEDIATE",
  "initiationType": "DICT",
  "creditParty": {
    "branch": "0001",
    "account": "12345678",
    "taxId": "12345678901",
    "name": "Receiver Name",
    "key": "12345678901"
  },
  "debitParty": {
    "bank": "00000000",
    "branch": "0001",
    "account": "87654321",
    "taxId": "98765432109",
    "name": "Payer Name"
  }
}
```

**Passos Executados:**
1. Seleciona conta de teste baseada no VU (`accountIndex = __VU % accounts.length`)
2. Chama `fullCashinFlow()` com dados da conta
3. Envia webhook INITIATED para aprovacao
4. Aguarda think time configuravel (`afterApproval`)
5. Envia webhook CONFIRMED para liquidacao
6. Registra metricas e2e

**Saidas Esperadas:**
- Status HTTP 200 para aprovacao
- `approvalId` presente na resposta
- Status `ACCEPTED` ou `DENIED`
- Status HTTP 200/201 para liquidacao
- `transactionId` presente na resposta

**Validacoes Realizadas:**
```javascript
// Aprovacao
'CashIn approval - status 200': (r) => r.status === 200
'CashIn approval - has approvalId': () => body.approvalId !== undefined
'CashIn approval - status is ACCEPTED or DENIED': () => ['ACCEPTED', 'DENIED'].includes(body.status)
'CashIn approval - response time < 500ms': (r) => r.timings.duration < 500

// Liquidacao
'CashIn settlement - status 200 or 201': (r) => [200, 201].includes(r.status)
'CashIn settlement - has transactionId or id': () => body.transactionId !== undefined || body.id !== undefined
'CashIn settlement - response time < 1000ms': (r) => r.timings.duration < 1000
```

**Cenarios de Erro Cobertos:**
- Timeout na aprovacao (> 30s)
- Timeout na liquidacao (> 30s)
- Resposta JSON invalida
- Status desconhecido na aprovacao
- Falha no parse do body

**Dependencias Externas:**
- Plugin BR PIX (webhooks)
- CRM (validacao de conta)
- Midaz (ledger/liquidacao)

---

#### 1.2.2 Test: `cashinDuplicateScenario`

| Campo | Valor |
|-------|-------|
| **Nome** | cashinDuplicateScenario |
| **Tipo** | Integracao / Chaos Engineering |
| **Arquivo** | `tests/load/cashin/run.js:240-274` |
| **Objetivo** | Validar deteccao e rejeicao de Cash-In com EndToEndId duplicado |

**Contexto Funcional:**
- Etapa PIX: Prevencao de credito duplicado
- Criticidade: **ALTA** - Bug aqui causa perda financeira

**Pre-requisitos:**
- Plugin BR PIX rodando
- Armazenamento de EndToEndId funcional

**Entradas:**
- Primeira iteracao: Payload completo de Cash-In valido
- Iteracoes seguintes: Payload com `endToEndId` reutilizado

**Passos Executados:**
1. Iteracao 0: Cria Cash-In bem-sucedido e armazena `endToEndId`
2. Iteracoes 1-N: Tenta criar Cash-In com mesmo `endToEndId`
3. Valida rejeicao com status 409 Conflict

**Saidas Esperadas:**
- Iteracao 0: Status 200 com `approvalId`
- Iteracoes 1-N: Status **409 Conflict**

**Validacoes Realizadas:**
```javascript
'Duplicate error - status 409 Conflict': (r) => r.status === 409
'Duplicate error - has error code': () => body.code !== undefined || body.error !== undefined
'Duplicate error - message mentions duplicate': () =>
  (body.message || '').toLowerCase().includes('duplicate') ||
  (body.message || '').toLowerCase().includes('already exists')
```

**Cenarios de Erro Cobertos:**
- EndToEndId duplicado NAO rejeitado (BUG CRITICO)
- Resposta 200 para duplicata (BUG CRITICO)

**Observacoes:**
> **ATENCAO:** Se `cashin_duplicate_rejections = 0` durante este teste, indica **BUG CRITICO** de duplicidade nao detectada, podendo causar credito duplo ao cliente.

---

#### 1.2.3 Test: `cashinConcurrentScenario`

| Campo | Valor |
|-------|-------|
| **Nome** | cashinConcurrentScenario |
| **Tipo** | Carga / Concorrencia |
| **Arquivo** | `tests/load/cashin/run.js:200-212` |
| **Objetivo** | Testar comportamento sob multiplos Cash-In simultaneos para mesma conta |

**Contexto Funcional:**
- Etapa PIX: Validacao de concorrencia
- Cenario: Multiplas transferencias chegando simultaneamente

**Pre-requisitos:**
- Plugin BR PIX com tratamento de concorrencia
- Mecanismo de lock/fila por conta

**Entradas:**
- Todos os VUs utilizam `data.accounts[0]` (mesma conta)
- Payloads distintos com EndToEndIds unicos

**Passos Executados:**
1. Todos os VUs selecionam a mesma conta compartilhada
2. Enviam webhooks INITIATED simultaneamente
3. Registram tempo de resposta e status

**Saidas Esperadas:**
- Todas as requisicoes devem ser processadas
- Sem erros de race condition
- Sem credito duplicado

**Validacoes Realizadas:**
- Registro de metricas de aprovacao
- Log de status e duracao por VU

**Observacoes:**
> Valida se o sistema consegue processar multiplos Cash-In para a mesma conta sem inconsistencias de saldo.

---

#### 1.2.4 Test: `cashinBurstScenario`

| Campo | Valor |
|-------|-------|
| **Nome** | cashinBurstScenario |
| **Tipo** | Carga / Spike |
| **Arquivo** | `tests/load/cashin/run.js:218-229` |
| **Objetivo** | Testar comportamento sob rajada de requisicoes |

**Contexto Funcional:**
- Etapa PIX: Recebimento em massa
- Cenario: Black Friday, promocoes, etc.

**Pre-requisitos:**
- Sistema dimensionado para picos

**Configuracao:**
```javascript
executor: 'constant-arrival-rate',
rate: TEST_TYPE === 'spike' ? 50 : 20, // requisicoes/segundo
timeUnit: '1s',
duration: '1m',
preAllocatedVUs: 20,
maxVUs: 50
```

**Passos Executados:**
1. Seleciona conta aleatoria do pool
2. Executa `fullCashinFlow()` com delay minimo (50ms)
3. Mantem taxa constante de chegada

**Saidas Esperadas:**
- Sistema deve processar todas as requisicoes
- Latencia p95 pode aumentar, mas sem falhas em cascata

---

#### 1.2.5 Test: `invalidPayloadFlow`

| Campo | Valor |
|-------|-------|
| **Nome** | invalidPayloadFlow |
| **Tipo** | Validacao / Negativo |
| **Arquivo** | `tests/load/cashin/flows/cashin-flow.js:394-419` |
| **Objetivo** | Validar tratamento de erros para payloads invalidos |

**Contexto Funcional:**
- Etapa PIX: Validacao de entrada
- Cenario: Protecao contra payloads malformados

**Tipos de Erro Testados:**

| Tipo | Descricao | Resposta Esperada |
|------|-----------|-------------------|
| `MISSING_DOCUMENT` | Falta taxId do creditParty | 400/422 |
| `INVALID_AMOUNT` | Valor negativo | 400/422 |
| `ZERO_AMOUNT` | Valor zero (minimo BACEN: R$0.01) | 400/422 |
| `EXCESSIVE_AMOUNT` | Valor acima do limite | 400/422 |
| `BELOW_MINIMUM` | Valor abaixo de R$0.01 | 400/422 |
| `INVALID_KEY` | Formato de chave PIX invalido | 400/422 |
| `EMPTY_ENDTOENDID` | EndToEndId vazio | 400/422 |
| `MISSING_CREDIT_PARTY` | Objeto creditParty ausente | 400/422 |
| `INVALID_STATUS` | Status desconhecido | 400/422 |

**Validacoes Realizadas:**
```javascript
// Payloads invalidos devem retornar erro 4xx
const correctlyRejected = res.status >= 400 && res.status < 500;
// Resposta 200 para payload invalido e um bug potencial
const potentialBug = res.status === 200;
```

---

### 1.3 Metricas Customizadas - Cash-In

**Arquivo:** `tests/load/cashin/lib/metrics.js`

#### Metricas de Aprovacao (Fase 1)

| Metrica | Tipo | Descricao |
|---------|------|-----------|
| `cashin_approval_total` | Counter | Total de requisicoes de aprovacao |
| `cashin_approval_accepted` | Counter | Aprovacoes com status ACCEPTED |
| `cashin_approval_denied` | Counter | Aprovacoes com status DENIED |
| `cashin_approval_failed` | Counter | Falhas HTTP (nao business) |
| `cashin_approval_duration` | Trend | Latencia da aprovacao |
| `cashin_approval_success_rate` | Rate | Taxa de sucesso (ACCEPTED ou DENIED) |

#### Metricas de Liquidacao (Fase 2)

| Metrica | Tipo | Descricao |
|---------|------|-----------|
| `cashin_settlement_total` | Counter | Total de liquidacoes |
| `cashin_settlement_success` | Counter | Liquidacoes bem-sucedidas |
| `cashin_settlement_failed` | Counter | Liquidacoes com falha |
| `cashin_settlement_duration` | Trend | Latencia da liquidacao |
| `cashin_settlement_success_rate` | Rate | Taxa de sucesso |

#### Metricas de Erro (Segregadas)

| Metrica | Tipo | Descricao |
|---------|------|-----------|
| `cashin_business_errors` | Counter | Erros de negocio (esperados) |
| `cashin_technical_errors` | Counter | Erros tecnicos (5xx, timeout) |
| `cashin_duplicate_rejections` | Counter | Duplicatas corretamente rejeitadas |
| `cashin_crm_failures` | Counter | Falhas de validacao CRM |
| `cashin_midaz_failures` | Counter | Falhas no Midaz |
| `cashin_invalid_key_errors` | Counter | Erros de chave PIX invalida |
| `cashin_timeout_errors` | Counter | Erros de timeout |

---

### 1.4 Thresholds por Tipo de Teste - Cash-In

**Arquivo:** `tests/load/cashin/config/scenarios.js`

#### Smoke Test
```javascript
{
  http_req_duration: ['p(95)<500', 'p(99)<1000'],
  http_req_failed: ['rate<0.01'],
  cashin_approval_duration: ['p(95)<500', 'avg<300'],
  cashin_settlement_duration: ['p(95)<1000', 'avg<500'],
  cashin_technical_error_rate: ['rate<0.01'],
  checks: ['rate>0.99']
}
```

#### Load Test
```javascript
{
  http_req_duration: ['p(95)<800', 'p(99)<1500'],
  cashin_approval_duration: ['p(95)<500', 'avg<300'],
  cashin_settlement_duration: ['p(95)<1000', 'avg<500'],
  cashin_e2e_duration: ['p(95)<2000', 'avg<1200'],
  cashin_technical_error_rate: ['rate<0.01'],
  cashin_business_error_rate: ['rate<0.10']
}
```

#### Soak Test (4h+ - Requisito BACEN)
```javascript
{
  http_req_duration: ['p(95)<600', 'p(99)<1200', 'avg<400'],
  cashin_approval_duration: ['p(95)<400', 'avg<250'],
  cashin_settlement_duration: ['p(95)<800', 'avg<400'],
  cashin_e2e_duration: ['p(95)<1500', 'avg<800'],
  cashin_technical_error_rate: ['rate<0.005'], // < 0.5%
  iteration_duration: ['p(95)<3000']
}
```

---

## 2. PIX Indirect BTG (Cash-Out, Collection, Refund)

### 2.1 Visao Geral

Suite de testes para operacoes PIX outbound: criacao de cobrancas, pagamentos e estornos.

**Localizacao:** `tests/v3.x.x/pix_indirect_btg/`

---

### 2.2 Testes Documentados

#### 2.2.1 Test: `createCollectionFlow`

| Campo | Valor |
|-------|-------|
| **Nome** | createCollectionFlow |
| **Tipo** | Integracao / E2E |
| **Arquivo** | `tests/v3.x.x/pix_indirect_btg/flows/create-collection-flow.js:33-205` |
| **Objetivo** | Validar fluxo completo de criacao de cobranca PIX (QR Code dinamico) |

**Contexto Funcional:**
- Etapa PIX: Criacao de cobranca imediata
- Operacao: POST /v1/collections/immediate

**Pre-requisitos:**
- Token de autenticacao valido
- Conta cadastrada com chave PIX
- Arquivo `data/accounts.json` com contas de teste

**Entradas:**
```javascript
{
  "txId": "txId32CharactersAlphanumeric12", // 26-35 caracteres
  "receiverKey": "email@example.com",
  "amount": 123.45,
  "expirationSeconds": 3600,
  "debtor": {
    "name": "Test Debtor VU1",
    "document": "12345678901"
  },
  "description": "K6 Test Collection - VU 1 ITER 0",
  "additionalInfo": {
    "testId": "k6-1642000000000",
    "vuId": 1,
    "iteration": 0
  }
}
```

**Passos Executados:**
1. Gera TxID unico (32 caracteres alfanumericos)
2. Gera chave de idempotencia
3. Cria payload de cobranca
4. POST /v1/collections/immediate
5. Valida resposta e extrai `collectionId`
6. GET /v1/collections/immediate/{id} (verificacao)
7. (Opcional) PATCH para atualizar descricao
8. (Opcional) DELETE para cancelar cobranca

**Saidas Esperadas:**
- Status 201 Created para criacao
- `id` (collectionId) na resposta
- Status 200 para GET de verificacao
- QR Code EMV presente (se solicitado)

**Validacoes Realizadas:**
```javascript
// Criacao
'Collection created - status 201': (r) => r.status === 201
'Collection created - has id': () => body.id !== undefined

// Recuperacao
'Collection retrieved - status 200': (r) => r.status === 200

// Update (se ACTIVE)
'Collection updated - status 200': (r) => r.status === 200

// Delete (se ACTIVE)
'Collection deleted - status 200 or 204': (r) => [200, 204].includes(r.status)
```

**Cenarios de Erro Cobertos:**
- TxID duplicado (409 Conflict)
- Conta nao encontrada
- Chave PIX invalida
- Cobranca expirada
- Cobranca ja paga (nao pode deletar)

**Observacoes:**
> Por especificacao BACEN, apenas cobrancas com status `ACTIVE` podem ser atualizadas ou deletadas. O fluxo verifica o estado antes de tentar operacoes.

---

#### 2.2.2 Test: `fullCashoutFlow`

| Campo | Valor |
|-------|-------|
| **Nome** | fullCashoutFlow |
| **Tipo** | E2E / Carga |
| **Arquivo** | `tests/v3.x.x/pix_indirect_btg/flows/full-cashout-flow.js:24-103` |
| **Objetivo** | Validar fluxo completo de Cash-Out (pagamento PIX) |

**Contexto Funcional:**
- Etapa PIX: Pagamento/Transferencia
- Operacoes: Initiate -> Process

**Pre-requisitos:**
- Token de autenticacao valido
- Conta com saldo disponivel
- Chaves PIX de destino validas

**IMPORTANTE:** A iniciacao de pagamento expira em **5 MINUTOS**. O step de process deve ser chamado dentro desta janela.

**Entradas (Initiate):**
```javascript
{
  "receiverKey": "email@destino.com",
  "keyType": "EMAIL", // EMAIL, PHONE, CPF, CNPJ, RANDOM
  "amount": 100.00,
  "description": "K6 Cashout Test"
}
```

**Entradas (Process):**
```javascript
{
  "initiationId": "uuid-da-iniciacao",
  "amount": 100.00,
  "description": "K6 Cashout - VU 1 ITER 0"
}
```

**Passos Executados:**
1. Initiate: POST /v1/transfers/cashout/initiate
2. Valida resposta e extrai `transferId`
3. Aguarda think time (simula revisao do usuario)
4. Process: POST /v1/transfers/cashout/process
5. Valida status final

**Transicoes de Estado:**
```
CREATED -> PENDING (hold Midaz criado)
PENDING -> PROCESSING (enviado ao BTG)
PROCESSING -> COMPLETED/FAILED
```

**Saidas Esperadas:**
- Initiate: Status 200/201 com `transferId` e `endToEndId`
- Process: Status 200/201 com status COMPLETED

**Validacoes Realizadas:**
```javascript
// Initiate
'Transfer initiated - status 2xx': (r) => r.status >= 200 && r.status < 300
'Transfer initiated - has transferId': () => body.transferId !== undefined
'Transfer initiated - has endToEndId': () => body.endToEndId !== undefined

// Process
'Transfer processed - status 2xx': (r) => r.status >= 200 && r.status < 300
'Transfer processed - status COMPLETED or success': () =>
  body.status === 'COMPLETED' || body.success === true
```

**Cenarios de Erro Cobertos:**
- Saldo insuficiente
- Chave PIX nao encontrada
- Iniciacao expirada (> 5 min)
- Falha no BTG
- Falha no Midaz (hold)

---

#### 2.2.3 Test: `expiredInitiationFlow`

| Campo | Valor |
|-------|-------|
| **Nome** | expiredInitiationFlow |
| **Tipo** | Negativo / Timeout |
| **Arquivo** | `tests/v3.x.x/pix_indirect_btg/flows/full-cashout-flow.js:262-308` |
| **Objetivo** | Validar rejeicao de process apos expiracao da iniciacao |

**Contexto Funcional:**
- Etapa PIX: Validacao de timeout de iniciacao
- Cenario: Usuario demora mais de 5 minutos para confirmar

**NOTA:** Este teste leva **5+ minutos** para completar. Use com moderacao.

**Passos Executados:**
1. Initiate: Cria iniciacao de pagamento
2. Aguarda 310 segundos (5 min + 10s buffer)
3. Process: Tenta processar pagamento expirado
4. Valida erro de expiracao

**Saidas Esperadas:**
- Status 400 Bad Request
- Mensagem indicando expiracao

**Validacoes Realizadas:**
```javascript
'Expired error - status 400': (r) => r.status === 400
'Expired error - message mentions expiration': () =>
  (body.message || '').toLowerCase().includes('expired') ||
  (body.message || '').toLowerCase().includes('expirad')
```

---

#### 2.2.4 Test: `fullRefundFlow`

| Campo | Valor |
|-------|-------|
| **Nome** | fullRefundFlow |
| **Tipo** | E2E / Integracao |
| **Arquivo** | `tests/v3.x.x/pix_indirect_btg/flows/refund-flow.js:136-213` |
| **Objetivo** | Validar fluxo completo de estorno PIX |

**Contexto Funcional:**
- Etapa PIX: Estorno de pagamento
- Operacao: Cashout completo + Refund

**Pre-requisitos:**
- Token de autenticacao valido
- Transferencia completada para estornar
- Prazo de estorno nao expirado

**Codigos de Motivo BACEN:**

| Codigo | Descricao |
|--------|-----------|
| `BE08` | Erro bancario |
| `FR01` | Fraude |
| `MD06` | Cliente solicitou estorno (mais comum) |
| `SL02` | Servico especifico do agente credor |

**Regras de Valor de Estorno:**
- **Estorno total:** `amount == grossAmount` (sempre permitido)
- **Estorno parcial:** `amount <= netAmount` (apos deducao de taxa)
- **NUNCA:** `amount > grossAmount`

**Passos Executados:**
1. Executa `fullCashoutFlow()` para ter transferencia
2. Aguarda liquidacao (think time)
3. Calcula valor do estorno (total ou parcial)
4. POST /v1/refunds com reasonCode
5. Valida criacao do estorno

**Entradas:**
```javascript
{
  "transferId": "uuid-da-transferencia",
  "amount": 50.00, // null para estorno total
  "description": "K6 Refund - MD06 - VU 1"
}
// Header: X-Reason-Code: MD06
```

**Saidas Esperadas:**
- Status 201 Created
- `refundId` e `returnId` na resposta

**Observacoes:**
> **RISCO IDENTIFICADO:** Valores de estorno parcial sao atualmente simulados e nao correlacionados com o valor real do cashout. Isso pode causar inconsistencias de validacao.

---

#### 2.2.5 Test: `duplicate-txid` (Chaos Engineering)

| Campo | Valor |
|-------|-------|
| **Nome** | duplicate-txid |
| **Tipo** | Chaos / Negativo |
| **Arquivo** | `tests/v3.x.x/pix_indirect_btg/scenarios/chaos/duplicate-txid.js` |
| **Objetivo** | Validar rejeicao de cobrancas com TxID duplicado |

**Contexto Funcional:**
- Etapa PIX: Validacao de unicidade
- Regra BACEN: TxID deve ser unico por (tx_id + receiver_document)

**Passos Executados:**
1. Cria cobranca com TxID especifico
2. Tenta criar segunda cobranca com mesmo TxID
3. Valida rejeicao com 409 Conflict

**Saidas Esperadas:**
- Primeira requisicao: 201 Created
- Segunda requisicao: 409 Conflict

---

#### 2.2.6 Test: `invalid-payload` (Chaos Engineering)

| Campo | Valor |
|-------|-------|
| **Nome** | invalid-payload |
| **Tipo** | Chaos / Validacao |
| **Arquivo** | `tests/v3.x.x/pix_indirect_btg/scenarios/chaos/invalid-payload.js` |
| **Objetivo** | Validar tratamento de erros para payloads malformados |

**Contexto Funcional:**
- Etapa PIX: Validacao de entrada
- Protecao contra dados corrompidos

**Cenarios Testados:**
- Campos obrigatorios ausentes
- Tipos de dados incorretos
- Valores fora dos limites
- Formato de chave PIX invalido

---

### 2.3 Metricas Customizadas - PIX Indirect BTG

**Arquivo:** `tests/v3.x.x/pix_indirect_btg/lib/metrics.js`

#### Metricas de Collection

| Metrica | Tipo | Descricao |
|---------|------|-----------|
| `pix_collection_created` | Counter | Cobrancas criadas |
| `pix_collection_failed` | Counter | Falhas na criacao |
| `pix_collection_duplicate_txid` | Counter | TxIDs duplicados |
| `pix_collection_create_duration` | Trend | Latencia de criacao |
| `pix_collection_get_duration` | Trend | Latencia de recuperacao |
| `pix_collection_error_rate` | Rate | Taxa de erro |

#### Metricas de Cashout

| Metrica | Tipo | Descricao |
|---------|------|-----------|
| `pix_cashout_initiated` | Counter | Pagamentos iniciados |
| `pix_cashout_processed` | Counter | Pagamentos processados |
| `pix_cashout_failed` | Counter | Pagamentos com falha |
| `pix_cashout_idempotent_hit` | Counter | Hits de idempotencia |
| `pix_cashout_midaz_failed` | Counter | Falhas no Midaz |
| `pix_cashout_btg_failed` | Counter | Falhas no BTG |
| `pix_cashout_initiate_duration` | Trend | Latencia de iniciacao |
| `pix_cashout_process_duration` | Trend | Latencia de processamento |
| `pix_cashout_error_rate` | Rate | Taxa de erro |

#### Metricas de Refund

| Metrica | Tipo | Descricao |
|---------|------|-----------|
| `pix_refund_created` | Counter | Estornos criados |
| `pix_refund_failed` | Counter | Estornos com falha |
| `pix_refund_duration` | Trend | Latencia de estorno |
| `pix_refund_error_rate` | Rate | Taxa de erro |

#### Metricas Agregadas

| Metrica | Tipo | Descricao |
|---------|------|-----------|
| `pix_e2e_flow_duration` | Trend | Duracao total do fluxo |
| `pix_technical_error_rate` | Rate | Erros tecnicos (5xx) |
| `pix_business_error_rate` | Rate | Erros de negocio (validacao) |

---

### 2.4 Thresholds por Tipo de Teste - PIX Indirect BTG

**Arquivo:** `tests/v3.x.x/pix_indirect_btg/config/thresholds.js`

#### Load Test
```javascript
{
  http_req_duration: ['p(95)<800', 'p(99)<1500'],
  pix_collection_error_rate: ['rate<0.05'],
  pix_collection_create_duration: ['p(95)<1000', 'avg<500'],
  pix_cashout_error_rate: ['rate<0.05'],
  pix_cashout_initiate_duration: ['p(95)<1000', 'avg<500'],
  pix_cashout_process_duration: ['p(95)<2000', 'avg<1000'],
  pix_technical_error_rate: ['rate<0.01'],
  pix_business_error_rate: ['rate<0.10'],
  pix_e2e_flow_duration: ['p(95)<4000', 'avg<2000']
}
```

#### Soak Test (4h+ - Requisito BACEN)
```javascript
{
  http_req_duration: ['p(95)<600', 'p(99)<1200', 'avg<400'],
  pix_collection_error_rate: ['rate<0.02'],
  pix_cashout_error_rate: ['rate<0.02'],
  pix_refund_error_rate: ['rate<0.02'],
  pix_technical_error_rate: ['rate<0.005'], // < 0.5%
  pix_business_error_rate: ['rate<0.05'],
  pix_e2e_flow_duration: ['p(95)<5000', 'avg<2500']
}
```

---

## 3. PIX Webhook Outbound (Entrega de Webhooks)

### 3.1 Visao Geral

Suite de testes para validacao do sistema de entrega de webhooks para endpoints externos.

**Localizacao:** `tests/load/outbound/`

---

### 3.2 Testes Documentados

#### 3.2.1 Test: `webhookMainFlow`

| Campo | Valor |
|-------|-------|
| **Nome** | webhookMainFlow |
| **Tipo** | Carga |
| **Arquivo** | `tests/load/outbound/run.js:158-174` |
| **Objetivo** | Validar entrega de webhooks com rotacao de tipos de entidade |

**Contexto Funcional:**
- Etapa PIX: Notificacao a sistemas externos
- Cenario: DICT claims, refunds, infraction reports

**Tipos de Entidade Testados:**

| Flow | Entity | Descricao |
|------|--------|-----------|
| DICT | CLAIM | Reivindicacao de chave |
| DICT | REFUND | Devolucao |
| DICT | INFRACTION_REPORT | Relatorio de infracao |
| PAYMENT | PAYMENT_STATUS | Status de pagamento |
| PAYMENT | PAYMENT_RETURN | Devolucao de pagamento |
| COLLECTION | COLLECTION_STATUS | Status de cobranca |

**Pre-requisitos:**
- Mock server rodando em `MOCK_SERVER_URL`
- Endpoints de destino disponiveis

**Passos Executados:**
1. Seleciona tipo de entidade por rotacao (`__ITER % entityTypes.length`)
2. Gera payload de webhook
3. Envia para endpoint de destino
4. Valida resposta e atualiza circuit breaker

---

#### 3.2.2 Test: `deliverWebhookWithRetry`

| Campo | Valor |
|-------|-------|
| **Nome** | deliverWebhookWithRetry |
| **Tipo** | Resiliencia |
| **Arquivo** | `tests/load/outbound/flows/webhook-delivery.js:186-248` |
| **Objetivo** | Validar logica de retry com backoff exponencial |

**Contexto Funcional:**
- Etapa PIX: Garantia de entrega
- Cenario: Endpoint temporariamente indisponivel

**Configuracao:**
```javascript
{
  maxRetries: 3,           // Total: 1 inicial + 3 retries = 4 tentativas
  backoffMultiplier: 2     // Backoff: 1s, 2s, 4s
}
```

**Passos Executados:**
1. Tenta entregar webhook
2. Se falha retryable, aguarda backoff
3. Repete ate sucesso ou maxRetries
4. Se esgotado, envia para DLQ (Dead Letter Queue)

**Saidas Esperadas:**
- Sucesso: Entrega bem-sucedida (com ou sem retry)
- DLQ: Erro nao retryable ou retries esgotados

**Classificacao de Erros:**

| Status | Retryable | Acao |
|--------|-----------|------|
| 5xx | Sim | Retry com backoff |
| 429 | Sim | Retry respeitando Retry-After |
| 4xx | Nao | Enviar para DLQ |
| Timeout | Sim | Retry com backoff |

---

#### 3.2.3 Test: `circuitBreakerRecoveryFlow`

| Campo | Valor |
|-------|-------|
| **Nome** | circuitBreakerRecoveryFlow |
| **Tipo** | Resiliencia / Chaos |
| **Arquivo** | `tests/load/outbound/flows/failure-simulation.js:451-514` |
| **Objetivo** | Validar comportamento do circuit breaker |

**Contexto Funcional:**
- Etapa PIX: Protecao contra falhas em cascata
- Padrao: Circuit Breaker

**Configuracao do Circuit Breaker:**
```javascript
const CB_FAILURE_THRESHOLD = 5;  // Falhas consecutivas para abrir
const CB_TIMEOUT_MS = 30000;     // Tempo para tentar recuperacao
```

**Estados do Circuit Breaker:**
```
CLOSED -> (5 falhas) -> OPEN -> (30s) -> HALF_OPEN -> (sucesso) -> CLOSED
                                              |
                                              +-> (falha) -> OPEN
```

**Passos Executados:**
1. **Fase 1:** Dispara 6 falhas consecutivas para abrir o circuito
2. **Fase 2:** Aguarda 35 segundos para timeout
3. **Fase 3:** Tenta requisicao de probe (half-open)
4. Valida se circuito fecha apos sucesso

**Saidas Esperadas:**
- Circuit abre apos 5 falhas
- Circuit vai para half-open apos 30s
- Circuit fecha apos probe bem-sucedido

---

#### 3.2.4 Test: `mixedFailureScenario`

| Campo | Valor |
|-------|-------|
| **Nome** | mixedFailureScenario |
| **Tipo** | Chaos Engineering |
| **Arquivo** | `tests/load/outbound/flows/failure-simulation.js:527-599` |
| **Objetivo** | Simular condicoes reais com mix de falhas |

**Distribuicao de Cenarios:**

| Cenario | Peso | Descricao |
|---------|------|-----------|
| Normal | 50% | Entrega bem-sucedida |
| Slow | 20% | Resposta lenta (3-10s) |
| Intermittent | 20% | Falhas intermitentes (30%) |
| Rate Limit | 5% | Erro 429 |
| Outage | 5% | Erro 503 total |

---

### 3.3 Metricas Customizadas - Webhook Outbound

**Arquivo:** `tests/load/outbound/lib/metrics.js`

| Metrica | Tipo | Descricao |
|---------|------|-----------|
| `webhook_delivery_total` | Counter | Total de entregas |
| `webhook_delivery_success_count` | Counter | Entregas bem-sucedidas |
| `webhook_delivery_failed_count` | Counter | Entregas com falha |
| `webhook_delivery_duration` | Trend | Latencia de entrega |
| `webhook_delivery_success` | Rate | Taxa de sucesso |
| `claim_latency` | Trend | Latencia para CLAIM |
| `refund_latency` | Trend | Latencia para REFUND |
| `infraction_report_latency` | Trend | Latencia para INFRACTION_REPORT |
| `payment_status_latency` | Trend | Latencia para PAYMENT_STATUS |
| `retry_count` | Counter | Total de retries |
| `retry_success_count` | Counter | Retries bem-sucedidos |
| `retry_exhausted_count` | Counter | Retries esgotados |
| `circuit_breaker_trips` | Counter | Aberturas de circuit breaker |
| `circuit_breaker_recoveries` | Counter | Recuperacoes de circuit breaker |
| `dlq_entries` | Counter | Entradas na DLQ |
| `webhook_technical_errors` | Counter | Erros tecnicos |
| `webhook_business_errors` | Counter | Erros de negocio |
| `webhook_timeout_errors` | Counter | Erros de timeout |
| `webhook_server_errors` | Counter | Erros de servidor |

---

## 4. Geradores de Payload

### 4.1 Geradores PIX BACEN-Compliant

**Arquivo:** `tests/load/cashin/lib/generators.js`

#### TxID (Identificador de Transacao)
```javascript
// Formato BACEN: 26-35 caracteres alfanumericos
function generateTxId(length = 32) {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  // Resultado: "txId32CharactersAlphanumeric12"
}
```

#### EndToEndId
```javascript
// Formato BACEN: E + ISPB(8) + YYYYMMDDHHMMSS(14) + SEQ(11) = 34 chars
function generateEndToEndId(ispb = '30306294') {
  // Resultado: "E3030629420260119123456789012ABCD"
}
```

#### CPF com Digitos Verificadores Validos
```javascript
function generateCPF() {
  // Gera 9 digitos aleatorios
  // Calcula dv1 e dv2 conforme regra
  // Resultado: "12345678909" (11 digitos)
}
```

#### CNPJ com Digitos Verificadores Validos
```javascript
function generateCNPJ() {
  // Gera 8 digitos aleatorios + 0001 (filial)
  // Calcula dv1 e dv2 conforme regra
  // Resultado: "12345678000195" (14 digitos)
}
```

#### Chaves PIX por Tipo
```javascript
function generateEmailKey() // "usuario@test.com"
function generatePhoneKey() // "+5511999999999"
function generateRandomKey() // UUID v4
```

---

## 5. Como Executar os Testes

### 5.1 Pre-requisitos

```bash
# Verificar instalacao do k6
k6 version
# Esperado: k6 v1.5.0 ou superior

# Verificar conectividade com servicos
curl http://localhost:8080/health  # Plugin PIX
curl http://localhost:4014/health  # Midaz PIX
```

### 5.2 Variaveis de Ambiente

| Variavel | Descricao | Valores | Default |
|----------|-----------|---------|---------|
| `TEST_TYPE` | Tipo de teste | smoke, load, stress, spike, soak | smoke |
| `ENVIRONMENT` | Ambiente alvo | dev, sandbox, vpc | dev |
| `PLUGIN_PIX_URL` | URL do Plugin PIX | URL | http://localhost:8080 |
| `LOG` | Nivel de log | DEBUG, ERROR, OFF | OFF |
| `K6_THINK_TIME_MODE` | Modo de think time | fast, realistic, stress | realistic |

### 5.3 Comandos de Execucao

#### PIX Cash-In

```bash
# Smoke Test (validacao rapida)
k6 run tests/load/cashin/run.js -e TEST_TYPE=smoke

# Load Test (carga normal)
k6 run tests/load/cashin/run.js -e TEST_TYPE=load

# Stress Test (limite do sistema)
k6 run tests/load/cashin/run.js -e TEST_TYPE=stress

# Spike Test (picos de trafego)
k6 run tests/load/cashin/run.js -e TEST_TYPE=spike

# Soak Test (4h+ para conformidade BACEN)
k6 run tests/load/cashin/run.js -e TEST_TYPE=soak
```

#### PIX Indirect BTG

```bash
# Suite completa
k6 run tests/v3.x.x/pix_indirect_btg/main.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke

# Com debug
k6 run tests/v3.x.x/pix_indirect_btg/main.js -e ENVIRONMENT=dev -e TEST_TYPE=load -e LOG=DEBUG

# Cenarios individuais
k6 run tests/v3.x.x/pix_indirect_btg/scenarios/collection/smoke.js -e ENVIRONMENT=dev
k6 run tests/v3.x.x/pix_indirect_btg/scenarios/payment/smoke.js -e ENVIRONMENT=dev
```

#### PIX Webhook Outbound

```bash
# Suite completa
k6 run tests/load/outbound/run.js -e TEST_TYPE=load -e MOCK_SERVER_URL=http://localhost:9090

# Com simulacao de falhas
k6 run tests/load/outbound/run.js -e TEST_TYPE=stress -e SIMULATE_FAILURES=true -e FAILURE_RATE=0.3
```

### 5.4 Exportacao de Resultados

```bash
# JSON
k6 run tests/load/cashin/run.js -e TEST_TYPE=load --out json=results.json

# InfluxDB
k6 run tests/load/cashin/run.js -e TEST_TYPE=load --out influxdb=http://localhost:8086/k6

# Prometheus
k6 run tests/load/cashin/run.js -e TEST_TYPE=load --out experimental-prometheus-rw=http://localhost:9090/api/v1/write
```

### 5.5 Diferenca entre Ambientes

| Ambiente | Uso | Servicos |
|----------|-----|----------|
| **dev** | Desenvolvimento local | Todos locais via localhost |
| **sandbox** | Integracao/QA | Servicos em ambiente shared |
| **vpc** | Producao-like | ALB interno AWS |

### 5.6 Execucao de Testes Isolados

```bash
# Apenas teste de duplicidade
k6 run tests/v3.x.x/pix_indirect_btg/scenarios/chaos/duplicate-txid.js -e ENVIRONMENT=dev

# Apenas teste de expiracao
k6 run tests/v3.x.x/pix_indirect_btg/scenarios/payment/expiration.js -e ENVIRONMENT=dev

# Smoke test standalone Cash-In
k6 run tests/load/cashin/scenarios/smoke.js -e PLUGIN_PIX_URL=http://localhost:8080
```

### 5.7 Interpretacao de Falhas Comuns

| Sintoma | Causa Provavel | Acao |
|---------|----------------|------|
| Conexao recusada | Servico nao iniciado | Verificar se Plugin/Midaz estao rodando |
| 401 Unauthorized | Token expirado | Regenerar token ou verificar auth |
| 404 Not Found | Endpoint incorreto | Verificar URL e versao da API |
| 422 Unprocessable | Validacao falhou | Verificar payload e dados de teste |
| 500 Internal Error | Bug no servidor | Verificar logs do servico |
| 503 Service Unavailable | Servico sobrecarregado | Reduzir carga ou aumentar recursos |
| p95 > threshold | Performance degradada | Identificar gargalo (CRM/Midaz/DB) |
| duplicate_rejections = 0 | **BUG CRITICO** | Verificar validacao de EndToEndId |

### 5.8 Troubleshooting

```bash
# Verificar health dos servicos
curl http://localhost:8080/health
curl http://localhost:4014/health

# Verificar endpoints de webhook
curl -X POST http://localhost:8080/v1/payment/webhooks/btg/events \
  -H "Content-Type: application/json" \
  -d '{}'

# Executar com HTTP debug completo
k6 run tests/load/cashin/run.js -e TEST_TYPE=smoke --http-debug=full

# Verificar resultados
cat summary.json | jq '.overall'
cat summary.json | jq '.thresholds'
```

---

## 6. Avaliacao Critica e Lacunas

### 6.1 Cobertura de Cenarios

| Cenario | Coberto | Observacao |
|---------|---------|------------|
| Cash-In (Recebimento) | Sim | Fluxo completo com fases |
| Cash-Out (Pagamento) | Sim | Initiate + Process |
| Collection (Cobranca) | Sim | CRUD completo |
| Refund (Estorno) | Sim | Total e parcial |
| Duplicidade | Sim | EndToEndId e TxID |
| Concorrencia | Sim | Mesma conta |
| Burst/Spike | Sim | Constant arrival rate |
| Soak (4h+) | Sim | Requisito BACEN |
| Circuit Breaker | Sim | Para webhooks outbound |
| Retry/Backoff | Sim | Para webhooks outbound |
| Timeout de Iniciacao | Sim | 5 minutos |
| Payloads Invalidos | Sim | Varios tipos |

### 6.2 Lacunas Identificadas

| Lacuna | Impacto | Recomendacao |
|--------|---------|--------------|
| **Sem Contract Tests** | Quebras de API nao detectadas | Adicionar Pact ou similar |
| **Valores de Refund Simulados** | Validacoes podem falhar | Propagar valores reais do cashout |
| **Dados Sinteticos** | Testes podem falhar em ambientes reais | Pre-provisionar contas |
| **Sem Testes de Reconciliacao** | Inconsistencias nao detectadas | Adicionar fluxo de verificacao |
| **Mock Server Externo** | Dependencia nao documentada | Incluir mock ou documentar setup |
| **Sem Testes de QR Code Estatico** | Cenario comum nao coberto | Adicionar fluxo especifico |

### 6.3 Testes Frageis ou Redundantes

| Teste | Problema | Recomendacao |
|-------|----------|--------------|
| `expiredInitiationFlow` | Leva 5+ minutos | Executar apenas em soak tests |
| Think times em `stress` mode | Podem gerar carga irreal | Manter minimo realista |

### 6.4 Alinhamento com Regras BACEN

| Regra BACEN | Implementacao | Status |
|-------------|---------------|--------|
| TxID 26-35 chars | `generateTxId(32)` | OK |
| EndToEndId formato | `E + ISPB + timestamp + seq` | OK |
| Duracao minima soak 4h | `soakScenario` configurado | OK |
| Aprovacao < 500ms p95 | Threshold configurado | OK |
| Liquidacao < 1000ms p95 | Threshold configurado | OK |
| Erro tecnico < 1% | Threshold configurado | OK |
| Deteccao de duplicidade | Teste especifico | OK |
| Valor minimo R$0.01 | Teste de validacao | OK |

---

## 7. Dados de Teste

### 7.1 Arquivos de Dados

| Arquivo | Conteudo | Tamanho |
|---------|----------|---------|
| `data/accounts.json` | Contas de teste (SVGS, CACC) | 22KB |
| `data/pix-keys.json` | Chaves PIX por tipo | 34KB |
| `data/qr-codes.json` | QR Codes de teste | 1.7KB |

### 7.2 Estrutura de Conta de Teste

```json
{
  "id": "account-uuid",
  "document": "12345678901",
  "name": "Test Account",
  "branch": "0001",
  "account": "12345678",
  "accountType": "CACC",
  "ispb": "30306294"
}
```

### 7.3 Estrutura de Chave PIX

```json
{
  "emailKeys": [
    { "accountId": "...", "key": "user@test.com", "type": "EMAIL" }
  ],
  "phoneKeys": [
    { "accountId": "...", "key": "+5511999999999", "type": "PHONE" }
  ],
  "cpfKeys": [
    { "accountId": "...", "key": "12345678901", "type": "CPF" }
  ],
  "randomKeys": [
    { "accountId": "...", "key": "uuid-v4", "type": "RANDOM" }
  ]
}
```

---

## 8. Integracao CI/CD

### 8.1 GitHub Actions

```yaml
name: PIX Load Tests

on:
  schedule:
    - cron: '0 6 * * *'
  workflow_dispatch:

jobs:
  load-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install k6
        run: |
          sudo gpg -k
          sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
            --keyserver hkp://keyserver.ubuntu.com:80 \
            --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
          echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
            | sudo tee /etc/apt/sources.list.d/k6.list
          sudo apt-get update
          sudo apt-get install k6

      - name: Run Smoke Tests
        run: |
          k6 run tests/v3.x.x/pix_indirect_btg/main.js \
            -e ENVIRONMENT=sandbox -e TEST_TYPE=smoke

      - name: Check Results
        run: |
          RESULT=$(cat summary.json | jq -r '.overall')
          if [ "$RESULT" != "PASS" ]; then
            echo "Tests failed!"
            exit 1
          fi

      - name: Upload Results
        uses: actions/upload-artifact@v4
        with:
          name: k6-results
          path: summary.json
```

### 8.2 Script de Execucao

```bash
#!/bin/bash
# run_pix_tests.sh

ENVIRONMENT=${1:-dev}
TEST_TYPE=${2:-smoke}

echo "==================================="
echo "PIX Load Tests - $TEST_TYPE"
echo "Environment: $ENVIRONMENT"
echo "==================================="

# PIX Indirect BTG
k6 run tests/v3.x.x/pix_indirect_btg/main.js \
  -e ENVIRONMENT=$ENVIRONMENT \
  -e TEST_TYPE=$TEST_TYPE

# PIX Cash-In (se PLUGIN_PIX_URL definido)
if [ ! -z "$PLUGIN_PIX_URL" ]; then
  k6 run tests/load/cashin/run.js \
    -e TEST_TYPE=$TEST_TYPE \
    -e PLUGIN_PIX_URL=$PLUGIN_PIX_URL
fi

# Verificar resultado
RESULT=$(cat summary.json | jq -r '.overall')
echo "Overall Result: $RESULT"
exit $([[ "$RESULT" == "PASS" ]] && echo 0 || echo 1)
```

---

## Apendice A: Estrutura de Arquivos

```
tests/
├── load/
│   ├── cashin/
│   │   ├── config/
│   │   │   └── scenarios.js          # Thresholds e cenarios
│   │   ├── flows/
│   │   │   └── cashin-flow.js        # Fluxo completo
│   │   ├── lib/
│   │   │   ├── generators.js         # Geradores de payload
│   │   │   ├── metrics.js            # Metricas customizadas
│   │   │   └── validators.js         # Validadores de resposta
│   │   ├── scenarios/
│   │   │   └── smoke.js              # Smoke test standalone
│   │   ├── run.js                    # Orquestrador principal
│   │   └── README.md
│   └── outbound/
│       ├── config/
│       │   └── scenarios.js
│       ├── flows/
│       │   ├── webhook-delivery.js
│       │   └── failure-simulation.js
│       ├── lib/
│       │   ├── generators.js
│       │   ├── metrics.js
│       │   └── validators.js
│       └── run.js
└── v3.x.x/
    └── pix_indirect_btg/
        ├── config/
        │   └── thresholds.js
        ├── data/
        │   ├── accounts.json
        │   ├── pix-keys.json
        │   └── qr-codes.json
        ├── flows/
        │   ├── create-collection-flow.js
        │   ├── full-cashout-flow.js
        │   ├── payment-initiation-flow.js
        │   ├── refund-flow.js
        │   └── concurrent-payments-same-collection.js
        ├── lib/
        │   ├── generators.js
        │   ├── metrics.js
        │   └── validators.js
        ├── scenarios/
        │   ├── collection/
        │   │   ├── smoke.js
        │   │   ├── load.js
        │   │   ├── stress.js
        │   │   ├── spike.js
        │   │   └── soak.js
        │   ├── payment/
        │   │   ├── smoke.js
        │   │   ├── load.js
        │   │   ├── stress.js
        │   │   ├── spike.js
        │   │   ├── soak.js
        │   │   └── expiration.js
        │   └── chaos/
        │       ├── duplicate-txid.js
        │       └── invalid-payload.js
        └── main.js
```

---

*Documentacao gerada em: 2026-01-19*
*Versao do k6-midaz: main branch (commit 2675773)*
