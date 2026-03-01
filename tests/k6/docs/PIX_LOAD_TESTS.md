# PIX Load Tests - Documentação

## Visão Geral

Este documento descreve os testes de carga disponíveis para as operações PIX, incluindo DICT, Collection e Cashout.

## Pré-requisitos

- K6 instalado (`brew install k6` no macOS)
- Acesso ao ambiente de teste configurado

---

## Variáveis de Ambiente

| Variável | Valores | Default | Descrição |
|----------|---------|---------|-----------|
| `ENVIRONMENT` | `dev`, `sandbox`, `vpc`, `capybara` | `dev` | Ambiente alvo |
| `AUTH_ENABLED` | `true`, `false` | **`true`** | Habilita autenticação |
| `K6_ABORT_ON_ERROR` | `true`, `false` | **`false`** | Aborta no primeiro erro HTTP |
| `NUM_ACCOUNTS` | número | `10` | Quantidade de contas no setup |
| `TEST_TYPE` | `smoke`, `load`, `stress` | `smoke` | Tipo de teste |
| `LOG` | `DEBUG`, `ERROR`, `OFF` | `OFF` | Nível de log |

### Nota sobre Defaults

```
AUTH_ENABLED=true        <- JÁ É O DEFAULT (pode omitir)
K6_ABORT_ON_ERROR=false  <- JÁ É O DEFAULT (pode omitir)
```

---

## Estrutura dos Testes

```
tests/v3.x.x/pix_indirect_btg/
├── dict-load-test.js       # Teste de carga para DICT
├── collection-load-test.js # Teste de carga para Collection
├── cashout-load-test.js    # Teste de carga para Cashout
├── main.js                 # Teste completo (todos os cenários)
└── flows/
    ├── dict-flow.js
    ├── create-collection-flow.js
    └── full-cashout-flow.js
```

---

## Comandos Disponíveis

### Usando Makefile (Recomendado)

```bash
# Ver todos os comandos disponíveis
make help
```

### Parâmetros

| Parâmetro   | Descrição                      | Valores                    | Default |
|-------------|--------------------------------|----------------------------|---------|
| ENVIRONMENT | Ambiente de execução           | dev, sandbox, vpc          | dev     |
| AUTH_ENABLED | Habilita autenticação         | true, false                | true    |
| K6_ABORT_ON_ERROR | Aborta no primeiro erro  | true, false                | false   |
| MIN_VUS     | Número mínimo de VUs           | Número inteiro             | 1       |
| MAX_VUS     | Número máximo de VUs           | Número inteiro             | 50      |
| DURATION    | Duração do teste               | 5m, 10m, 30m, 1h, 4h       | 5m      |
| TEST_TYPE   | Tipo de teste (main.js apenas) | smoke, load, stress, spike | load    |

---

## PIX com Setup Dinâmico (Recomendado)

O teste `pix-test-with-dynamic-setup.js` cria automaticamente todas as entidades necessárias:

| Step | Entidade | Descrição |
|------|----------|-----------|
| 1 | Account | Conta vinculada ao Org/Ledger fixo |
| 2 | Balance | Saldo inicial R$ 10.000 |
| 3 | Holder | Cliente no CRM |
| 4 | Alias | PIX Key (CPF como chave) |
| 5 | DICT Entry | Registro no DICT |

### Comandos

| Cenário | Comando |
|---------|---------|
| **Smoke + Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke` |
| **Smoke - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e AUTH_ENABLED=false` |
| **Load + Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=load` |
| **Load - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=load -e AUTH_ENABLED=false` |
| **Stress + Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=stress` |
| **Stress - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=stress -e AUTH_ENABLED=false` |

---

## Testes Individuais

### 1. DICT Load Test

Testa operações do Diretório de Identificadores PIX:
- Lookup de chave PIX para pagamento
- Listagem de entradas DICT
- Validação em lote de chaves PIX

| Cenário | Comando |
|---------|---------|
| **Com Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/dict-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke` |
| **Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/dict-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e AUTH_ENABLED=false` |

```bash
# Com parâmetros personalizados
make test-dict ENVIRONMENT=dev MAX_VUS=50 DURATION=5m

# Comando K6 direto com VUs customizados
k6 run tests/v3.x.x/pix_indirect_btg/dict-load-test.js \
  -e ENVIRONMENT=dev \
  -e MIN_VUS=1 \
  -e MAX_VUS=50 \
  -e DURATION=5m
```

### 2. Collection Load Test

Testa operações de Cobrança PIX:
- Criação de cobrança imediata
- Consulta de cobrança
- Atualização de cobrança
- Remoção de cobrança
- Listagem de cobranças

| Cenário | Comando |
|---------|---------|
| **Com Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/collection-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke` |
| **Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/collection-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e AUTH_ENABLED=false` |

```bash
# Com parâmetros personalizados
make test-collection ENVIRONMENT=sandbox MIN_VUS=10 MAX_VUS=100 DURATION=10m

# Comando K6 direto com VUs customizados
k6 run tests/v3.x.x/pix_indirect_btg/collection-load-test.js \
  -e ENVIRONMENT=sandbox \
  -e MIN_VUS=10 \
  -e MAX_VUS=100 \
  -e DURATION=10m
```

### 3. Cashout Load Test

Testa operações de Pagamento PIX:
- Iniciação de pagamento por chave PIX
- Iniciação de pagamento por QR Code
- Processamento de pagamento
- Consulta de transferência

| Cenário | Comando |
|---------|---------|
| **Com Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/cashout-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke` |
| **Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/cashout-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e AUTH_ENABLED=false` |

```bash
# Com parâmetros personalizados
make test-cashout ENVIRONMENT=dev MAX_VUS=75 DURATION=15m

# Comando K6 direto com VUs customizados
k6 run tests/v3.x.x/pix_indirect_btg/cashout-load-test.js \
  -e ENVIRONMENT=dev \
  -e MIN_VUS=1 \
  -e MAX_VUS=75 \
  -e DURATION=15m
```

---

## Executar Todos os Testes

### Em Sequência (DICT → Collection → Cashout)

```bash
# Executa os 3 testes em sequência
make test-all ENVIRONMENT=dev MAX_VUS=50 DURATION=5m
```

### Teste Completo (main.js)

Executa todos os cenários (collection, payment, refund) simultaneamente:

| Cenário | Comando |
|---------|---------|
| **Com Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/main.js -e ENVIRONMENT=dev -e TEST_TYPE=load` |
| **Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/main.js -e ENVIRONMENT=dev -e TEST_TYPE=load -e AUTH_ENABLED=false` |

```bash
# Load test completo via Makefile
make test-full ENVIRONMENT=dev TEST_TYPE=load DURATION=30m

# Comando K6 direto com VUs customizados
k6 run tests/v3.x.x/pix_indirect_btg/main.js \
  -e ENVIRONMENT=dev \
  -e TEST_TYPE=load \
  -e MIN_VUS=10 \
  -e MAX_VUS=100 \
  -e DURATION=30m
```

---

## Tipos de Teste

### Smoke Test
Validação rápida com carga mínima (1 VU, 1 minuto).

```bash
make test-smoke ENVIRONMENT=dev
```

### Load Test
Simulação de carga normal de produção.

```bash
make test-load ENVIRONMENT=dev MAX_VUS=50 DURATION=30m
```

### Stress Test
Teste com carga elevada para encontrar limites do sistema.

```bash
make test-stress ENVIRONMENT=dev MAX_VUS=200 DURATION=25m
```

### Spike Test
Teste com picos repentinos de carga.

```bash
make test-spike ENVIRONMENT=dev
```

### Soak Test (BACEN)
Teste prolongado para conformidade BACEN (mínimo 4 horas).

```bash
make test-soak ENVIRONMENT=sandbox DURATION=4h
```

---

## Exemplos de Uso

### Desenvolvimento Local
```bash
# Smoke test rápido para validar mudanças
make test-smoke ENVIRONMENT=dev

# Teste de carga moderado
make test-dict ENVIRONMENT=dev MAX_VUS=20 DURATION=3m
```

### Ambiente de Sandbox
```bash
# Teste de carga completo
make test-all ENVIRONMENT=sandbox MAX_VUS=100 DURATION=10m

# Teste de stress
make test-stress ENVIRONMENT=sandbox MAX_VUS=300 DURATION=20m
```

### Produção/VPC
```bash
# Soak test para conformidade BACEN
make test-soak ENVIRONMENT=vpc DURATION=4h

# Teste completo com alta carga
make test-full ENVIRONMENT=vpc TEST_TYPE=load MAX_VUS=500 DURATION=1h
```

---

## Métricas Coletadas

### DICT
- `pix_dict_lookup_duration` - Latência de lookup de chave
- `pix_dict_list_duration` - Latência de listagem
- `pix_dict_check_duration` - Latência de validação em lote
- `pix_dict_error_rate` - Taxa de erro

### Collection
- `pix_collection_create_duration` - Latência de criação
- `pix_collection_get_duration` - Latência de consulta
- `pix_collection_update_duration` - Latência de atualização
- `pix_collection_delete_duration` - Latência de remoção
- `pix_collection_error_rate` - Taxa de erro

### Cashout
- `pix_cashout_initiate_duration` - Latência de iniciação
- `pix_cashout_process_duration` - Latência de processamento
- `pix_e2e_flow_duration` - Latência end-to-end
- `pix_cashout_error_rate` - Taxa de erro

---

## Arquivos de Saída

Após a execução, são gerados arquivos JSON com o resumo:

- `dict-summary.json` - Resumo do teste DICT
- `collection-summary.json` - Resumo do teste Collection
- `cashout-summary.json` - Resumo do teste Cashout
- `summary.json` - Resumo do teste completo (main.js)

Para limpar os arquivos de resumo:
```bash
make clean
```

---

## Thresholds (Limites de Aceitação)

### DICT
| Métrica | Limite |
|---------|--------|
| Lookup P95 | < 300ms |
| Lookup Avg | < 150ms |
| Error Rate | < 5% |

### Collection
| Métrica | Limite |
|---------|--------|
| Create P95 | < 1000ms |
| Create Avg | < 500ms |
| Error Rate | < 5% |

### Cashout
| Métrica | Limite |
|---------|--------|
| Initiate P95 | < 1000ms |
| Process P95 | < 2000ms |
| E2E Flow P95 | < 4000ms |
| Error Rate | < 5% |

---

## Troubleshooting

### K6 não encontrado
```bash
# macOS
brew install k6

# Linux
sudo apt-get install k6
```

### Erro de conexão
Verifique se o ambiente está acessível e se as URLs estão corretas em `config/env.json`.

### Erro 401 Unauthorized
Se receber erro 401, verifique:
- `AUTH_ENABLED=true` (padrão) requer credenciais válidas no ambiente
- Para testes sem autenticação: `-e AUTH_ENABLED=false`
- Ambiente sandbox/vpc requer autenticação válida

### Timeout em testes longos
Para testes de soak, aumente o timeout do terminal ou execute em background:
```bash
nohup make test-soak ENVIRONMENT=sandbox DURATION=4h > soak-test.log 2>&1 &
```
