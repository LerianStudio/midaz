# Comandos de Testes de Carga PIX

Documentacao completa de todos os comandos disponiveis para execucao dos testes de carga do dominio PIX.

## Indice

1. [Pre-requisitos](#pre-requisitos)
2. [Variaveis de Ambiente](#variaveis-de-ambiente)
3. [Tabela de Comandos](#tabela-de-comandos)
4. [Setup Completo](#setup-completo)
5. [Exemplos Avancados](#exemplos-avancados)
6. [CI/CD Integration](#cicd-integration)

---

## Pre-requisitos

```bash
# Verificar instalacao do k6
k6 version

# Versao esperada: k6 v1.5.0 ou superior
```

---

## Variaveis de Ambiente

### Referencia Completa

| Variavel | Valores | Default | Descricao |
|----------|---------|---------|-----------|
| `ENVIRONMENT` | `dev`, `sandbox`, `vpc`, `capybara` | `dev` | Ambiente alvo |
| `AUTH_ENABLED` | `true`, `false` | **`true`** | Habilita autenticacao |
| `K6_ABORT_ON_ERROR` | `true`, `false` | **`false`** | Aborta no primeiro erro HTTP |
| `NUM_ACCOUNTS` | numero | `10` | Quantidade de contas no setup |
| `TEST_TYPE` | `smoke`, `load`, `stress` | `smoke` | Tipo de teste |
| `LOG` | `DEBUG`, `ERROR`, `OFF` | `OFF` | Nivel de log |

### Ambientes Disponiveis

| Ambiente | Descricao | URLs |
|----------|-----------|------|
| `dev` | Desenvolvimento local | `localhost:3000`, `localhost:4014` |
| `sandbox` | AWS sandbox | AWS ELB endpoints |
| `vpc` | VPC interna (requer VPN) | ALB interno |
| `capybara` | Local alternativo | `192.168.0.2` |

### Tipos de Teste

| Tipo | VUs | Duracao | Uso |
|------|-----|---------|-----|
| `smoke` | 1 | 1min | Validacao rapida |
| `load` | 10 | 5min | Carga normal de producao |
| `stress` | 50 | 10min | Encontrar limite do sistema |

---

## Tabela de Comandos

### Teste Principal (pix-test-with-dynamic-setup.js)

| Cenario | Comando |
|---------|---------|
| **Smoke + Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e K6_ABORT_ON_ERROR=false` |
| **Smoke - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e AUTH_ENABLED=false -e K6_ABORT_ON_ERROR=false` |
| **Load + Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=load -e K6_ABORT_ON_ERROR=false` |
| **Load - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=load -e AUTH_ENABLED=false -e K6_ABORT_ON_ERROR=false` |
| **Stress + Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=stress -e K6_ABORT_ON_ERROR=false` |
| **Stress - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e TEST_TYPE=stress -e AUTH_ENABLED=false -e K6_ABORT_ON_ERROR=false` |

### Cash-In (tests/load/cashin/run.js)

| Cenario | Comando |
|---------|---------|
| **Smoke + Auth** | `k6 run tests/load/cashin/run.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e K6_ABORT_ON_ERROR=false` |
| **Smoke - Sem Auth** | `k6 run tests/load/cashin/run.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e AUTH_ENABLED=false -e K6_ABORT_ON_ERROR=false` |
| **Load + Auth** | `k6 run tests/load/cashin/run.js -e ENVIRONMENT=dev -e TEST_TYPE=load -e K6_ABORT_ON_ERROR=false` |
| **Load - Sem Auth** | `k6 run tests/load/cashin/run.js -e ENVIRONMENT=dev -e TEST_TYPE=load -e AUTH_ENABLED=false -e K6_ABORT_ON_ERROR=false` |

### Collection (collection-load-test.js)

| Cenario | Comando |
|---------|---------|
| **Smoke + Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/collection-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e K6_ABORT_ON_ERROR=false` |
| **Smoke - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/collection-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e AUTH_ENABLED=false -e K6_ABORT_ON_ERROR=false` |

### Cashout (cashout-load-test.js)

| Cenario | Comando |
|---------|---------|
| **Smoke + Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/cashout-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e K6_ABORT_ON_ERROR=false` |
| **Smoke - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/cashout-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e AUTH_ENABLED=false -e K6_ABORT_ON_ERROR=false` |

### DICT (dict-load-test.js)

| Cenario | Comando |
|---------|---------|
| **Smoke + Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/dict-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e K6_ABORT_ON_ERROR=false` |
| **Smoke - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/dict-load-test.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke -e AUTH_ENABLED=false -e K6_ABORT_ON_ERROR=false` |

### Exemplos com Debug

| Cenario | Comando |
|---------|---------|
| **Debug Completo** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e LOG=DEBUG -e K6_ABORT_ON_ERROR=false` |
| **Debug + Abortar** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e LOG=DEBUG -e K6_ABORT_ON_ERROR=true` |
| **Debug - Sem Auth** | `k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js -e ENVIRONMENT=dev -e LOG=DEBUG -e AUTH_ENABLED=false -e K6_ABORT_ON_ERROR=false` |

---

## Setup Completo

O `pix-complete-setup.js` cria automaticamente todas as entidades necessarias:

| Step | Entidade | Descricao |
|------|----------|-----------|
| 0 | External Account | `@external/BRL` para transacoes |
| 1 | Account | Conta vinculada ao Org/Ledger fixo |
| 2 | Balance | Saldo inicial R$ 10.000 |
| 3 | Holder | Cliente no CRM |
| 4 | Alias | PIX Key (CPF como chave) |
| 5 | DICT Entry | Registro no DICT |

### IDs Fixos (Obrigatorios)

```
MIDAZ_ORGANIZATION_ID = 019be10f-df74-78ce-ac1c-0ef1e8d810fb
MIDAZ_LEDGER_ID       = 019be10f-fa03-77a3-b395-aa8c7974a2c0
```

---

## Nota sobre Defaults

```
K6_ABORT_ON_ERROR=false  <- JA E O DEFAULT (pode omitir)
AUTH_ENABLED=true        <- JA E O DEFAULT (pode omitir)
```

Portanto, o comando minimo com auth e sem abortar e:

```bash
k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js \
  -e ENVIRONMENT=dev \
  -e TEST_TYPE=smoke
```

---

## Exemplos Avancados

### Exportar Resultados para JSON

```bash
k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js \
  -e ENVIRONMENT=dev \
  -e TEST_TYPE=load \
  -e K6_ABORT_ON_ERROR=false \
  --out json=results.json
```

### Exportar para InfluxDB

```bash
k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js \
  -e ENVIRONMENT=dev \
  -e TEST_TYPE=load \
  -e K6_ABORT_ON_ERROR=false \
  --out influxdb=http://localhost:8086/k6
```

### Executar com VUs Customizados

```bash
k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js \
  -e ENVIRONMENT=dev \
  -e TEST_TYPE=load \
  -e K6_ABORT_ON_ERROR=false \
  --vus 100 \
  --duration 15m
```

### Executar com HTTP Debug

```bash
k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js \
  -e ENVIRONMENT=dev \
  -e TEST_TYPE=smoke \
  -e K6_ABORT_ON_ERROR=false \
  --http-debug=full
```

---

## CI/CD Integration

### GitHub Actions

```yaml
name: PIX Load Tests

on:
  schedule:
    - cron: '0 6 * * *'  # Daily at 6 AM
  workflow_dispatch:

jobs:
  load-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install k6
        run: |
          sudo gpg -k
          sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
          echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
          sudo apt-get update
          sudo apt-get install k6

      - name: Run Smoke Test
        run: |
          k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js \
            -e ENVIRONMENT=sandbox \
            -e NUM_ACCOUNTS=5 \
            -e TEST_TYPE=smoke \
            -e K6_ABORT_ON_ERROR=false

      - name: Upload Results
        uses: actions/upload-artifact@v4
        with:
          name: k6-results
          path: summary.json
```

### Script Bash

```bash
#!/bin/bash
# run_pix_tests.sh

ENVIRONMENT=${1:-dev}
TEST_TYPE=${2:-smoke}
AUTH_ENABLED=${3:-true}

echo "==================================="
echo "PIX Load Tests - $TEST_TYPE"
echo "Environment: $ENVIRONMENT"
echo "Auth: $AUTH_ENABLED"
echo "==================================="

k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js \
  -e ENVIRONMENT=$ENVIRONMENT \
  -e TEST_TYPE=$TEST_TYPE \
  -e AUTH_ENABLED=$AUTH_ENABLED \
  -e K6_ABORT_ON_ERROR=false \
  -e NUM_ACCOUNTS=10

echo "==================================="
echo "Tests completed!"
echo "==================================="
```

Uso:

```bash
# Smoke test em dev com auth
./run_pix_tests.sh dev smoke true

# Load test em sandbox sem auth
./run_pix_tests.sh sandbox load false
```

---

## Console Output

Ao executar os testes, voce vera:

```
ENV: dev
AUTH: enabled
ABORT_ON_ERROR: disabled
```

Isso confirma qual ambiente e configuracoes estao ativos.

---

## Interpretacao de Resultados

### Metricas Principais

| Metrica | Threshold Smoke | Threshold Load | Descricao |
|---------|-----------------|----------------|-----------|
| `http_req_duration p95` | < 2000ms | < 3000ms | Latencia P95 |
| `http_req_failed` | < 10% | < 15% | Taxa de falha HTTP |

### Arquivos de Saida

Apos cada execucao, o arquivo `summary.json` e gerado:

```bash
# Visualizar resultados
cat summary.json | jq .

# Verificar status geral
cat summary.json | jq '.testType'

# Verificar latencia
cat summary.json | jq '.latency'
```
