# Documentação de Testes PIX - IDs Fixos Obrigatórios

## Sumário

1. [Visão Geral](#visão-geral)
2. [IDs Fixos Obrigatórios](#ids-fixos-obrigatórios)
3. [Restrições](#restrições)
4. [Fluxo de Execução](#fluxo-de-execução)
5. [Estrutura de Arquivos](#estrutura-de-arquivos)
6. [Testes Disponíveis](#testes-disponíveis)
7. [Como Executar](#como-executar)
8. [Cenários de Teste Detalhados](#cenários-de-teste-detalhados)
9. [Troubleshooting](#troubleshooting)

---

## Visão Geral

Este documento descreve a estrutura e execução dos testes de PIX que utilizam **obrigatoriamente** os IDs de Organization e Ledger já existentes no sistema Midaz.

### Objetivo

Validar o comportamento completo do sistema PIX, garantindo:
- Consistência total de dados
- Isolamento do setup
- Reutilização exclusiva dos identificadores fornecidos

---

## IDs Fixos Obrigatórios

> ⚠️ **ATENÇÃO:** Estes valores são OBRIGATÓRIOS e NÃO devem ser alterados.

```bash
MIDAZ_ORGANIZATION_ID="019be10f-df74-78ce-ac1c-0ef1e8d810fb"
MIDAZ_LEDGER_ID="019be10f-fa03-77a3-b395-aa8c7974a2c0"
```

Todas as entidades criadas nos testes DEVEM estar associadas a estes IDs:
- ✅ Accounts
- ✅ Assets
- ✅ Balances
- ✅ Transactions
- ✅ Collections PIX
- ✅ Transfers PIX
- ✅ Refunds PIX

---

## Restrições

### ❌ PROIBIDO

| Ação | Motivo |
|------|--------|
| Criar Organization | Já existe e deve ser reutilizada |
| Criar Ledger | Já existe e deve ser reutilizado |
| Sobrescrever IDs | Viola o princípio de consistência |
| Mockar novos IDs | Compromete a integridade dos testes |
| Setup de Midaz/Org/Ledger em testes PIX | Fora do escopo de PIX |

### ✅ PERMITIDO

| Ação | Descrição |
|------|-----------|
| Criar Account | Vinculada ao Org/Ledger fixo |
| Criar Asset | Vinculado ao Org/Ledger fixo |
| Criar Balance | Vinculado à Account criada |
| Executar operações PIX | Collections, Transfers, Refunds |

---

## Fluxo de Execução

```
┌─────────────────────────────────────────────────────────────────────┐
│                    FLUXO CORRETO DE TESTES PIX                      │
└─────────────────────────────────────────────────────────────────────┘

  ┌────────────────────────────────────────────────────────────────┐
  │ STEP 0: UTILIZAR IDS EXISTENTES (NÃO CRIAR!)                   │
  │                                                                  │
  │   Organization: 019be10f-df74-78ce-ac1c-0ef1e8d810fb  [FIXO]   │
  │   Ledger:       019be10f-fa03-77a3-b395-aa8c7974a2c0  [FIXO]   │
  └────────────────────────────────────────────────────────────────┘
                              │
                              ▼
  ┌────────────────────────────────────────────────────────────────┐
  │ STEP 1: CRIAR/VERIFICAR ASSET (BRL)                            │
  │                                                                  │
  │   POST /v1/organizations/{orgId}/ledgers/{ledgerId}/assets     │
  │   Payload: { name, type: "currency", code: "BRL", status }     │
  └────────────────────────────────────────────────────────────────┘
                              │
                              ▼
  ┌────────────────────────────────────────────────────────────────┐
  │ STEP 2: CRIAR ACCOUNT                                          │
  │                                                                  │
  │   POST /v1/organizations/{orgId}/ledgers/{ledgerId}/accounts   │
  │   Payload: { assetCode, name, alias, type, status }            │
  └────────────────────────────────────────────────────────────────┘
                              │
                              ▼
  ┌────────────────────────────────────────────────────────────────┐
  │ STEP 3: EXECUTAR TESTES PIX                                    │
  │                                                                  │
  │   ├── Collection (Cobrança Imediata)                           │
  │   │   ├── Create     POST /v1/collections/immediate            │
  │   │   ├── Get        GET /v1/collections/immediate/{id}        │
  │   │   ├── GetByTxId  GET /v1/collections/immediate/txid/{txId} │
  │   │   ├── List       GET /v1/collections/immediate             │
  │   │   ├── Update     PUT /v1/collections/immediate/{id}        │
  │   │   └── Delete     DELETE /v1/collections/immediate/{id}     │
  │   │                                                            │
  │   ├── Transfer/Cashout (Pagamento)                             │
  │   │   ├── Initiate   POST /v1/transfers/cashout/initiate       │
  │   │   ├── Process    POST /v1/transfers/cashout/process        │
  │   │   ├── GetById    GET /v1/transfers/{id}                    │
  │   │   ├── GetByE2E   GET /v1/transfers/e2e/{endToEndId}        │
  │   │   └── List       GET /v1/transfers                         │
  │   │                                                            │
  │   └── Refund (Reembolso)                                       │
  │       ├── Create     POST /v1/transfers/{id}/refunds           │
  │       ├── GetById    GET /v1/transfers/{id}/refunds/{refundId} │
  │       └── List       GET /v1/transfers/{id}/refunds            │
  └────────────────────────────────────────────────────────────────┘
```

---

## Estrutura de Arquivos

```
tests/v3.x.x/pix_indirect_btg/
├── pix-full-validation-test.js    # ⭐ Teste completo com IDs fixos
├── validation-test.js             # Teste simples de validação
├── main.js                        # Orquestrador (smoke/load/stress)
├── PIX_TEST_DOCUMENTATION.md      # Esta documentação
│
├── config/
│   └── thresholds.js              # Configurações de limites
│
├── data/
│   ├── accounts.json              # 100 contas de teste
│   ├── pix-keys.json              # Chaves PIX (email/phone/CPF)
│   └── qr-codes.json              # QR codes para testes
│
├── flows/
│   ├── create-collection-flow.js  # Fluxo de cobrança
│   ├── full-cashout-flow.js       # Fluxo de pagamento
│   ├── payment-initiation-flow.js # Iniciação de pagamento
│   └── refund-flow.js             # Fluxo de reembolso
│
├── lib/
│   ├── generators.js              # Geradores de dados
│   ├── validators.js              # Validadores de resposta
│   └── metrics.js                 # Métricas customizadas
│
└── scenarios/
    ├── collection/                # Cenários de cobrança
    │   ├── smoke.js
    │   ├── load.js
    │   ├── stress.js
    │   ├── spike.js
    │   └── soak.js
    └── payment/                   # Cenários de pagamento
        └── smoke.js
```

---

## Testes Disponíveis

### 1. pix-full-validation-test.js (Recomendado)

**Objetivo:** Validação completa do sistema PIX com IDs fixos obrigatórios.

**Fluxo:**
1. Verifica/Cria Asset BRL
2. Cria Account vinculada ao Org/Ledger fixo
3. Executa testes de Collection (CRUD completo)
4. Executa testes de Cashout (Initiate + Process)
5. Executa testes de Refund

**Variáveis de ambiente:**

| Variável | Valores | Default | Descrição |
|----------|---------|---------|-----------|
| `ENVIRONMENT` | dev, sandbox, vpc | dev | Ambiente de execução |
| `LOG` | DEBUG, ERROR, OFF | OFF | Nível de log |
| `K6_ABORT_ON_ERROR` | true, false | false | Abortar em erro |
| `TEST_SCENARIO` | all, collection, cashout, refund | all | Cenário específico |
| `DURATION` | 1m, 5m, 1h, 4h, etc | (por tipo de teste) | Duração do teste |
| `MIN_VUS` | número inteiro | (por tipo de teste) | VUs mínimos/iniciais |
| `MAX_VUS` | número inteiro | (por tipo de teste) | VUs máximos |

---

## Parâmetros Dinâmicos

Os cenários de teste suportam parâmetros dinâmicos que permitem customizar duração e número de VUs sem modificar código:

### Parâmetros Disponíveis

| Parâmetro | Descrição | Exemplo |
|-----------|-----------|---------|
| `DURATION` | Duração total do teste. Para cenários com stages (ramping), os stages são escalados proporcionalmente. | `5m`, `30m`, `1h`, `4h` |
| `MIN_VUS` | VUs mínimos. Aplica-se a `startVUs` (ramping) ou `preAllocatedVUs` (arrival-rate). | `10`, `50`, `100` |
| `MAX_VUS` | VUs máximos. Aplica-se a `vus` (constant), targets dos stages, ou `maxVUs` (arrival-rate). | `50`, `200`, `500` |

### Comportamento por Tipo de Executor

| Executor | DURATION | MIN_VUS | MAX_VUS |
|----------|----------|---------|---------|
| `constant-vus` | Aplica `duration` | Ignora | Aplica `vus` |
| `ramping-vus` | Escala stages | Aplica `startVUs` | Escala targets |
| `constant-arrival-rate` | Aplica `duration` | Aplica `preAllocatedVUs` | Aplica `maxVUs` |
| `ramping-arrival-rate` | Escala stages | Aplica `preAllocatedVUs` | Aplica `maxVUs` |

### Exemplos de Uso

```bash
# Smoke test rápido (2 min, 5 VUs)
k6 run scenarios/collection/smoke.js -e DURATION=2m -e MAX_VUS=5

# Load test customizado (15 min, até 100 VUs)
k6 run scenarios/collection/load.js -e DURATION=15m -e MAX_VUS=100

# Stress test intenso (30 min, até 500 VUs)
k6 run scenarios/payment/stress.js -e DURATION=30m -e MAX_VUS=500

# Soak test curto para dev (30 min ao invés de 4h)
k6 run scenarios/collection/soak.js -e DURATION=30m -e MIN_VUS=50 -e MAX_VUS=150

# Main orchestrator com parâmetros
k6 run main.js -e TEST_TYPE=load -e DURATION=10m -e MAX_VUS=100
```

---

## Como Executar

### Execução Básica

```bash
# Teste completo com IDs fixos
k6 run tests/v3.x.x/pix_indirect_btg/pix-full-validation-test.js
```

### Com Debug

```bash
k6 run tests/v3.x.x/pix_indirect_btg/pix-full-validation-test.js \
  -e LOG=DEBUG \
  -e K6_ABORT_ON_ERROR=false
```

### Cenário Específico

```bash
# Apenas Collection
k6 run tests/v3.x.x/pix_indirect_btg/pix-full-validation-test.js \
  -e TEST_SCENARIO=collection

# Apenas Cashout
k6 run tests/v3.x.x/pix_indirect_btg/pix-full-validation-test.js \
  -e TEST_SCENARIO=cashout

# Apenas Refund
k6 run tests/v3.x.x/pix_indirect_btg/pix-full-validation-test.js \
  -e TEST_SCENARIO=refund
```

### Ambiente Diferente

```bash
# Sandbox
k6 run tests/v3.x.x/pix_indirect_btg/pix-full-validation-test.js \
  -e ENVIRONMENT=sandbox

# VPC
k6 run tests/v3.x.x/pix_indirect_btg/pix-full-validation-test.js \
  -e ENVIRONMENT=vpc
```

---

## Cenários de Teste Detalhados

### TEST 1: PIX Collection (Cobrança Imediata)

#### Objetivo
Validar o ciclo de vida completo de uma cobrança PIX imediata.

#### Pré-condições
- Organization e Ledger já existentes (IDs fixos)
- Account criada e ativa
- Token de autenticação válido

#### Passo a Passo

| Step | Operação | Endpoint | Validação |
|------|----------|----------|-----------|
| 1 | Criar Cobrança | POST /v1/collections/immediate | Status 201, retorna `id` e `txId` |
| 2 | Consultar por ID | GET /v1/collections/immediate/{id} | Status 200, dados consistentes |
| 3 | Consultar por TxID | GET /v1/collections/immediate/txid/{txId} | Status 200, dados consistentes |
| 4 | Listar Cobranças | GET /v1/collections/immediate | Status 200, array de items |
| 5 | Deletar Cobrança | DELETE /v1/collections/immediate/{id} | Status 200/204 |

#### Dados Utilizados

```javascript
{
  txId: "string (26-35 chars alphanumeric)",  // BACEN spec
  receiverKey: "email@example.com",            // Chave PIX gerada
  amount: "10.00 - 100.00",                    // Valor aleatório
  expirationSeconds: 3600,                     // 1 hora
  additionalInfo: {
    testId: "validation-{timestamp}",
    organizationId: "019be10f-df74-78ce-ac1c-0ef1e8d810fb",  // ID FIXO
    ledgerId: "019be10f-fa03-77a3-b395-aa8c7974a2c0"         // ID FIXO
  }
}
```

#### Resultado Esperado
- Cobrança criada com status ACTIVE
- Consultas retornam dados consistentes
- Deleção bem-sucedida (apenas se ACTIVE)

#### Possíveis Falhas

| Código | Causa | Ação |
|--------|-------|------|
| 400 | Payload inválido | Verificar formato do txId (26-35 chars) |
| 401 | Token inválido | Regenerar token |
| 404 | Account não encontrada | Verificar X-Account-Id |
| 409 | TxID duplicado | Gerar novo txId |
| 500 | Erro interno | Verificar logs do servidor |

---

### TEST 2: PIX Cashout/Transfer (Pagamento)

#### Objetivo
Validar o fluxo completo de pagamento PIX (Initiate → Process).

#### Pré-condições
- Organization e Ledger já existentes (IDs fixos)
- Account criada, ativa e com saldo suficiente
- Token de autenticação válido

#### Passo a Passo

| Step | Operação | Endpoint | Validação | Timeout |
|------|----------|----------|-----------|---------|
| 1 | Iniciar Pagamento | POST /v1/transfers/cashout/initiate | Status 201, retorna `id` | - |
| 2 | Aguardar Confirmação | (sleep) | - | 1-5s |
| 3 | Processar Pagamento | POST /v1/transfers/cashout/process | Status 200/201/202 | 5 min max |
| 4 | Consultar Transfer | GET /v1/transfers/{id} | Status 200 | - |

#### Dados Utilizados

**Initiate Payload:**
```javascript
{
  initiationType: "KEY",              // KEY, QR_CODE, ou MANUAL
  key: "email@example.com",           // Chave PIX do recebedor
  description: "PIX Cashout Validation"
}
```

**Process Payload:**
```javascript
{
  initiationId: "{id-from-initiate}",
  amount: "10.00 - 50.00",
  description: "K6 Cashout Process"
}
```

#### Resultado Esperado
- Initiate retorna `transferId` e `endToEndId`
- Process confirma o pagamento
- Status evolui: CREATED → PENDING → PROCESSING → COMPLETED

#### Estados da Transferência

```
CREATED    → Pagamento iniciado
PENDING    → Hold Midaz criado (PONTO SEM RETORNO #1)
PROCESSING → Enviado ao BTG (PONTO SEM RETORNO #2)
COMPLETED  → Pagamento concluído
FAILED     → Pagamento falhou (reversão automática em 4xx)
```

#### Possíveis Falhas

| Código | Causa | Ação |
|--------|-------|------|
| 400 | Initiation expirada | Process dentro de 5 minutos |
| 400 | Saldo insuficiente | Verificar balance da account |
| 401 | Token inválido | Regenerar token |
| 404 | Chave PIX não encontrada | Verificar formato da key |
| 422 | Validação de negócio | Verificar regras BACEN |

---

### TEST 3: PIX Refund (Reembolso)

#### Objetivo
Validar a criação e consulta de reembolsos PIX.

#### Pré-condições
- Organization e Ledger já existentes (IDs fixos)
- Transfer concluído com status COMPLETED
- Token de autenticação válido

#### Passo a Passo

| Step | Operação | Endpoint | Validação |
|------|----------|----------|-----------|
| 1 | Criar Refund | POST /v1/transfers/{id}/refunds | Status 201/200/202 |
| 2 | Consultar Refund | GET /v1/transfers/{id}/refunds/{refundId} | Status 200 |
| 3 | Listar Refunds | GET /v1/transfers/{id}/refunds | Status 200 |

#### Dados Utilizados

```javascript
// Headers
{
  "X-Reason": "MD06"  // Código BACEN obrigatório
}

// Payload
{
  amount: "10.00",    // Valor do reembolso (≤ netAmount)
  description: "PIX Refund Validation"
}
```

#### Códigos de Motivo BACEN

| Código | Descrição | Uso |
|--------|-----------|-----|
| BE08 | Bank error | Erro do banco |
| FR01 | Fraud | Fraude detectada |
| MD06 | Customer requested refund | Solicitação do cliente |
| SL02 | Creditor agent specific service | Serviço específico |

#### Regras de Valor do Refund

| Tipo | Condição | Permitido |
|------|----------|-----------|
| Total | amount == grossAmount | ✅ Sempre |
| Parcial | amount ≤ netAmount | ✅ Sim |
| Excesso | amount > grossAmount | ❌ Nunca |

#### Resultado Esperado
- Refund criado com sucesso
- Consultas retornam dados do refund
- Valor debitado da conta do recebedor

#### Possíveis Falhas

| Código | Causa | Ação |
|--------|-------|------|
| 400 | Valor excede limite | Verificar netAmount do transfer |
| 404 | Transfer não encontrado | Verificar transferId |
| 409 | Refund já existe | Verificar idempotency |
| 422 | Transfer não completado | Aguardar status COMPLETED |

---

## Troubleshooting

### Erro: Connection Refused (porta 3000/4014)

**Causa:** Serviços Midaz não estão rodando.

**Solução:**
```bash
# Verificar se os containers estão rodando
docker ps

# Iniciar containers se necessário
docker-compose up -d
```

### Erro: 500 - CRM Connection Error

**Causa:** Serviço PIX não consegue conectar ao CRM backend.

**Solução:**
- Verificar conectividade do CRM
- Verificar configuração do ambiente em `/config/env.json`

### Erro: 401 - Unauthorized

**Causa:** Token inválido ou expirado.

**Solução:**
- Verificar credenciais em `/config/env.json`
- Verificar se o serviço de auth está disponível

### Erro: 409 - Conflict (TxID duplicado)

**Causa:** TxID já existe no sistema.

**Solução:**
- O teste gera TxIDs únicos automaticamente
- Se persistir, verificar cleanup do banco

### Erro: 422 - Unprocessable Entity

**Causa:** Regra de negócio violada (BACEN).

**Solução:**
- Verificar formato do TxID (26-35 chars alphanumeric)
- Verificar valor do amount (positivo, 2 decimais)
- Verificar expirationSeconds (1 a 2.592.000)

---

## Métricas Coletadas

| Métrica | Descrição |
|---------|-----------|
| `http_req_duration` | Tempo de resposta das requisições |
| `http_req_failed` | Taxa de falhas |
| `pix_collection_created` | Collections criadas |
| `pix_collection_error_rate` | Taxa de erro em collections |
| `pix_cashout_initiated` | Pagamentos iniciados |
| `pix_cashout_processed` | Pagamentos processados |
| `pix_refund_created` | Refunds criados |

---

## Contato

Para dúvidas ou problemas, consulte a equipe de QA ou abra uma issue no repositório.
