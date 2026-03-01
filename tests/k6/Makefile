# ==============================================================================
# K6-MIDAZ - PIX Load Test Makefile
# ==============================================================================
#
# This Makefile provides parameterized commands for running load tests.
#
# ==============================================================================

# Default settings (can be overridden from CLI)
ENVIRONMENT ?= dev
MIN_VUS ?= 1
MAX_VUS ?= 50
DURATION ?= 5m
TEST_TYPE ?= load

# PIX test directory
PIX_TEST_DIR = tests/v3.x.x/pix_indirect_btg

# Base K6 command
K6_CMD = k6 run

# Common environment variables
K6_ENV = -e ENVIRONMENT=$(ENVIRONMENT) -e MIN_VUS=$(MIN_VUS) -e MAX_VUS=$(MAX_VUS) -e DURATION=$(DURATION)

# ==============================================================================
# HELP
# ==============================================================================

.PHONY: help
help: ## Show this help message
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo "║                    K6-MIDAZ - PIX Load Tests                                 ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo ""
	@echo "USAGE:"
	@echo "  make <comando> [ENVIRONMENT=dev] [MIN_VUS=1] [MAX_VUS=50] [DURATION=5m]"
	@echo ""
	@echo "PARÂMETROS:"
	@echo "  ENVIRONMENT  Execution environment: dev, sandbox, vpc (default: dev)"
	@echo "  MIN_VUS      Minimum number of VUs (default: 1)"
	@echo "  MAX_VUS      Maximum number of VUs (default: 50)"
	@echo "  DURATION     Test duration: 5m, 10m, 1h, etc (default: 5m)"
	@echo "  TEST_TYPE    Test type: smoke, load, stress, spike, soak (default: load)"
	@echo ""
	@echo "AVAILABLE COMMANDS:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "EXAMPLES:"
	@echo "  make test-dict ENVIRONMENT=dev MAX_VUS=50 DURATION=5m"
	@echo "  make test-collection ENVIRONMENT=sandbox MIN_VUS=10 MAX_VUS=100 DURATION=10m"
	@echo "  make test-cashout ENVIRONMENT=dev MAX_VUS=75 DURATION=15m"
	@echo "  make test-all ENVIRONMENT=dev MAX_VUS=50 DURATION=5m"
	@echo "  make test-full ENVIRONMENT=sandbox TEST_TYPE=load DURATION=30m"
	@echo ""

# ==============================================================================
# TESTES INDIVIDUAIS
# ==============================================================================

.PHONY: test-dict
test-dict: ## Executa teste de carga do DICT (busca de chaves PIX)
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo "║                         DICT LOAD TEST                                       ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo "Environment: $(ENVIRONMENT)"
	@echo "Min VUs: $(MIN_VUS)"
	@echo "Max VUs: $(MAX_VUS)"
	@echo "Duration: $(DURATION)"
	@echo ""
	$(K6_CMD) $(PIX_TEST_DIR)/dict-load-test.js $(K6_ENV)

.PHONY: test-collection
test-collection: ## Executa teste de carga de Collection (cobrança PIX)
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo "║                      COLLECTION LOAD TEST                                    ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo "Environment: $(ENVIRONMENT)"
	@echo "Min VUs: $(MIN_VUS)"
	@echo "Max VUs: $(MAX_VUS)"
	@echo "Duration: $(DURATION)"
	@echo ""
	$(K6_CMD) $(PIX_TEST_DIR)/collection-load-test.js $(K6_ENV)

.PHONY: test-cashout
test-cashout: ## Executa teste de carga de Cashout (pagamento PIX)
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo "║                        CASHOUT LOAD TEST                                     ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo "Environment: $(ENVIRONMENT)"
	@echo "Min VUs: $(MIN_VUS)"
	@echo "Max VUs: $(MAX_VUS)"
	@echo "Duration: $(DURATION)"
	@echo ""
	$(K6_CMD) $(PIX_TEST_DIR)/cashout-load-test.js $(K6_ENV)

# ==============================================================================
# TESTES COMBINADOS
# ==============================================================================

.PHONY: test-all
test-all: ## Executa TODOS os testes de carga em sequência (dict, collection, cashout)
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo "║                    EXECUTANDO TODOS OS TESTES                                ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo ""
	@echo ">>> [1/3] DICT Load Test..."
	@$(MAKE) test-dict ENVIRONMENT=$(ENVIRONMENT) MIN_VUS=$(MIN_VUS) MAX_VUS=$(MAX_VUS) DURATION=$(DURATION)
	@echo ""
	@echo ">>> [2/3] Collection Load Test..."
	@$(MAKE) test-collection ENVIRONMENT=$(ENVIRONMENT) MIN_VUS=$(MIN_VUS) MAX_VUS=$(MAX_VUS) DURATION=$(DURATION)
	@echo ""
	@echo ">>> [3/3] Cashout Load Test..."
	@$(MAKE) test-cashout ENVIRONMENT=$(ENVIRONMENT) MIN_VUS=$(MIN_VUS) MAX_VUS=$(MAX_VUS) DURATION=$(DURATION)
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo "║                    TODOS OS TESTES FINALIZADOS                               ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"

.PHONY: test-full
test-full: ## Executa o teste completo (main.js) com todos os cenários
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo "║                      PIX FULL TEST SUITE                                     ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo "Environment: $(ENVIRONMENT)"
	@echo "Test Type: $(TEST_TYPE)"
	@echo "Min VUs: $(MIN_VUS)"
	@echo "Max VUs: $(MAX_VUS)"
	@echo "Duration: $(DURATION)"
	@echo ""
	$(K6_CMD) $(PIX_TEST_DIR)/main.js -e ENVIRONMENT=$(ENVIRONMENT) -e TEST_TYPE=$(TEST_TYPE) -e MIN_VUS=$(MIN_VUS) -e MAX_VUS=$(MAX_VUS) -e DURATION=$(DURATION)

# ==============================================================================
# TESTES POR TIPO
# ==============================================================================

.PHONY: test-smoke
test-smoke: ## Executa smoke test (validação rápida)
	@echo "Executando Smoke Test..."
	$(K6_CMD) $(PIX_TEST_DIR)/main.js -e ENVIRONMENT=$(ENVIRONMENT) -e TEST_TYPE=smoke

.PHONY: test-load
test-load: ## Executa load test (carga normal)
	@echo "Executando Load Test..."
	$(K6_CMD) $(PIX_TEST_DIR)/main.js -e ENVIRONMENT=$(ENVIRONMENT) -e TEST_TYPE=load -e MIN_VUS=$(MIN_VUS) -e MAX_VUS=$(MAX_VUS) -e DURATION=$(DURATION)

.PHONY: test-stress
test-stress: ## Executa stress test (carga elevada)
	@echo "Executando Stress Test..."
	$(K6_CMD) $(PIX_TEST_DIR)/main.js -e ENVIRONMENT=$(ENVIRONMENT) -e TEST_TYPE=stress -e MIN_VUS=$(MIN_VUS) -e MAX_VUS=$(MAX_VUS) -e DURATION=$(DURATION)

.PHONY: test-spike
test-spike: ## Executa spike test (picos de carga)
	@echo "Executando Spike Test..."
	$(K6_CMD) $(PIX_TEST_DIR)/main.js -e ENVIRONMENT=$(ENVIRONMENT) -e TEST_TYPE=spike

.PHONY: test-soak
test-soak: ## Executa soak test (carga prolongada - BACEN 4h+)
	@echo "Executando Soak Test (4+ horas para conformidade BACEN)..."
	$(K6_CMD) $(PIX_TEST_DIR)/main.js -e ENVIRONMENT=$(ENVIRONMENT) -e TEST_TYPE=soak -e DURATION=$(DURATION)

# ==============================================================================
# UTILITÁRIOS
# ==============================================================================

.PHONY: check-k6
check-k6: ## Verifica se o K6 está instalado
	@which k6 > /dev/null 2>&1 || (echo "ERRO: K6 não encontrado. Instale com: brew install k6" && exit 1)
	@echo "K6 instalado: $$(k6 version)"

.PHONY: list-tests
list-tests: ## Lista todos os arquivos de teste disponíveis
	@echo "Arquivos de teste disponíveis:"
	@echo ""
	@find $(PIX_TEST_DIR) -name "*.js" -type f | sort | while read file; do \
		echo "  - $$file"; \
	done

.PHONY: clean
clean: ## Remove arquivos de resumo gerados
	@echo "Removendo arquivos de resumo..."
	@rm -f summary.json dict-summary.json collection-summary.json cashout-summary.json
	@echo "Limpeza concluída."

# Default target
.DEFAULT_GOAL := help
