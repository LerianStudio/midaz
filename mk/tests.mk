# ------------------------------------------------------
# Test configuration (extracted from root Makefile)
# ------------------------------------------------------
TEST_ONBOARDING_URL ?= http://localhost:3000
TEST_TRANSACTION_URL ?= http://localhost:3001
TEST_HEALTH_WAIT ?= 60
TEST_FUZZTIME ?= 30s
START_LOCAL_DOCKER ?= 0

# Optional auth configuration (passed through to tests)
TEST_AUTH_URL ?=
TEST_AUTH_USERNAME ?=
TEST_AUTH_PASSWORD ?=

# Optional fuzz engine load controls
TEST_PARALLEL ?=
TEST_GOMAXPROCS ?=

# macOS ld64 workaround: newer ld emits noisy LC_DYSYMTAB warnings when linking test binaries with -race.
# If available, prefer Apple's classic linker to silence them.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
  # Prefer classic mode to suppress LC_DYSYMTAB warnings on macOS.
  # Set DISABLE_OSX_LINKER_WORKAROUND=1 to disable this behavior.
  ifneq ($(DISABLE_OSX_LINKER_WORKAROUND),1)
    GO_TEST_LDFLAGS := -ldflags="-linkmode=external -extldflags=-ld_classic"
  else
    GO_TEST_LDFLAGS :=
  endif
else
  GO_TEST_LDFLAGS :=
endif

define wait_for_services
	echo "Waiting for services to become healthy..."
	bash -c 'for i in $$(seq 1 $(TEST_HEALTH_WAIT)); do \
	  if curl -fsS $(TEST_ONBOARDING_URL)/health >/dev/null 2>&1 && curl -fsS $(TEST_TRANSACTION_URL)/health >/dev/null 2>&1; then \
	    echo "Services are up"; exit 0; \
	  fi; \
	  sleep 1; \
	done; echo "[error] Services not healthy after $(TEST_HEALTH_WAIT)s"; exit 1'
endef

.PHONY: wait-for-services
wait-for-services:
	$(call wait_for_services)


# ------------------------------------------------------
# Test tooling configuration
# ------------------------------------------------------

TEST_REPORTS_DIR ?= ./reports
GOTESTSUM := $(shell command -v gotestsum 2>/dev/null)
RETRY_ON_FAIL ?= 0

.PHONY: tools tools-gotestsum
tools: tools-gotestsum ## Install helpful dev/test tools

tools-gotestsum:
	@if [ -z "$(GOTESTSUM)" ]; then \
		echo "Installing gotestsum..."; \
		GO111MODULE=on go install gotest.tools/gotestsum@latest; \
	else \
		echo "gotestsum already installed: $(GOTESTSUM)"; \
	fi


#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: test
test:
	@./scripts/run-tests.sh


#-------------------------------------------------------
# Test Suite Aliases
#-------------------------------------------------------

# Unit tests
.PHONY: test-unit
test-unit:
	$(call print_title,Running Go unit tests)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@set -e; mkdir -p $(TEST_REPORTS_DIR)/unit; \
	pkgs=$$(go list ./... | awk '!/\/tests($|\/)/'); \
	if [ -z "$$pkgs" ]; then \
	  echo "No unit test packages found (outside ./tests)**"; \
	else \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    echo "Running unit tests with gotestsum"; \
	    gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/unit/unit.xml -- -v -race -count=1 $(GO_TEST_LDFLAGS) -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/unit/coverage.out $$pkgs || { \
	      if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	        echo "Retrying unit tests once..."; \
	        gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/unit/unit-rerun.xml -- -v -race -count=1 $(GO_TEST_LDFLAGS) -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/unit/coverage.out $$pkgs; \
	      else \
	        exit 1; \
	      fi; \
	    }; \
	  else \
	    go test -v -race -count=1 $(GO_TEST_LDFLAGS) -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/unit/coverage.out $$pkgs; \
	  fi; \
	fi

.PHONY: coverage-unit
coverage-unit: test-unit
	$(call print_title,Generate and open unit test coverage report)
	@set -e; \
	if [ -f $(TEST_REPORTS_DIR)/unit/coverage.out ]; then \
	  go tool cover -html=$(TEST_REPORTS_DIR)/unit/coverage.out -o $(TEST_REPORTS_DIR)/unit/coverage.html; \
	  echo "Coverage report generated at $(TEST_REPORTS_DIR)/unit/coverage.html"; \
	  if command -v open >/dev/null 2>&1; then \
	    open $(TEST_REPORTS_DIR)/unit/coverage.html; \
	  elif command -v xdg-open >/dev/null 2>&1; then \
	    xdg-open $(TEST_REPORTS_DIR)/unit/coverage.html; \
	  else \
	    echo "Open the file manually: $(TEST_REPORTS_DIR)/unit/coverage.html"; \
	  fi; \
	  echo "----------------------------------------"; \
	  go tool cover -func=$(TEST_REPORTS_DIR)/unit/coverage.out | grep total | awk '{print "Total coverage: " $$3}'; \
	  echo "----------------------------------------"; \
	else \
	  echo "coverage.out not found at $(TEST_REPORTS_DIR)/unit/coverage.out"; \
	  exit 1; \
	fi

# Integration tests (Go) – spins up stack, runs tests/integration
.PHONY: test-integration
test-integration:
	$(call print_title,Running Go integration tests)

ifeq ($(START_LOCAL_DOCKER),1)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
endif
	@set -e; mkdir -p $(TEST_REPORTS_DIR)/integration; \
	if [ "$(START_LOCAL_DOCKER)" = "1" ]; then \
	  trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	  $(MAKE) up-backend; \
	  $(MAKE) -s wait-for-services; \
	else \
	  echo "Skipping local backend startup (START_LOCAL_DOCKER=$(START_LOCAL_DOCKER))"; \
	fi; \
	if [ -n "$(GOTESTSUM)" ]; then \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/integration/integration.xml -- -v -race -count=1 $(GO_TEST_LDFLAGS) ./tests/integration || { \
	    if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	      echo "Retrying integration tests once..."; \
	      ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/integration/integration-rerun.xml -- -v -race -count=1 $(GO_TEST_LDFLAGS) ./tests/integration; \
	    else \
	      exit 1; \
	    fi; \
	  }; \
	else \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) go test -v -race -count=1 $(GO_TEST_LDFLAGS) ./tests/integration; \
	fi


# E2E tests (Apidog CLI) – brings up stack, runs Apidog JSON workflow, saves report
.PHONY: test-e2e
test-e2e:
	$(call print_title,Running E2E tests with Apidog CLI (with Docker stack))
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@set -e; \
	trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	$(MAKE) up-backend; \
	$(call wait_for_services); \
	mkdir -p ./reports/e2e; \
	echo "Running Apidog CLI via npx against tests/e2e/local.apidog-cli.json"; \
	npx --yes @apidog/cli@latest run ./tests/e2e/local.apidog-cli.json -r html,cli --out-dir ./reports/e2e || \
	npx --yes apidog-cli@latest run ./tests/e2e/local.apidog-cli.json -r html,cli --out-dir ./reports/e2e

# Property tests (model-level)
.PHONY: test-property
test-property:
	$(call print_title,Running property-based model tests)
	@set -e; mkdir -p $(TEST_REPORTS_DIR)/property; \
	if [ -n "$(GOTESTSUM)" ]; then \
	  gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/property/property.xml -- -v -race -timeout 120s -count=1 $(GO_TEST_LDFLAGS) ./tests/property || { \
	    if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	      echo "Retrying property tests once..."; \
	      gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/property/property-rerun.xml -- -v -race -timeout 120s -count=1 $(GO_TEST_LDFLAGS) ./tests/property; \
	    else \
	      exit 1; \
	    fi; \
	  }; \
	else \
	  go test -v -race -timeout 120s -count=1 $(GO_TEST_LDFLAGS) ./tests/property; \
	fi

# Chaos tests
.PHONY: test-chaos
test-chaos:
	$(call print_title,Running chaos tests)

ifeq ($(START_LOCAL_DOCKER),1)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
endif
	@set -e; mkdir -p $(TEST_REPORTS_DIR)/chaos; \
	if [ "$(START_LOCAL_DOCKER)" = "1" ]; then \
	  trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	  $(MAKE) up-backend; \
	  $(MAKE) -s wait-for-services; \
	else \
	  echo "Skipping local backend startup (START_LOCAL_DOCKER=$(START_LOCAL_DOCKER))"; \
	fi; \
	if [ -n "$(GOTESTSUM)" ]; then \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/chaos/chaos.xml -- -v -race -timeout 30m -count=1 $(GO_TEST_LDFLAGS) ./tests/chaos || { \
	    if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	      echo "Retrying chaos tests once..."; \
	      ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/chaos/chaos-rerun.xml -- -v -race -timeout 30m -count=1 $(GO_TEST_LDFLAGS) ./tests/chaos; \
	    else \
	      exit 1; \
	    fi; \
	  }; \
	else \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) go test -v -race -timeout 30m -count=1 $(GO_TEST_LDFLAGS) ./tests/chaos; \
	fi

# Fuzzy/robustness tests
.PHONY: test-fuzzy
test-fuzzy:
	$(call print_title,Running fuzz/robustness tests)

ifeq ($(START_LOCAL_DOCKER),1)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
endif
	@set -e; mkdir -p $(TEST_REPORTS_DIR)/fuzzy; \
	if [ "$(START_LOCAL_DOCKER)" = "1" ]; then \
	  trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	  $(MAKE) up-backend; \
	  $(MAKE) -s wait-for-services; \
	else \
	  echo "Skipping local backend startup (START_LOCAL_DOCKER=$(START_LOCAL_DOCKER))"; \
	fi; \
	if [ -n "$(GOTESTSUM)" ]; then \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/fuzzy/fuzzy.xml -- -v -race -count=1 $(GO_TEST_LDFLAGS) ./tests/fuzzy || { \
	    if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	      echo "Retrying fuzzy tests once..."; \
	      ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/fuzzy/fuzzy-rerun.xml -- -v -race -count=1 $(GO_TEST_LDFLAGS) ./tests/fuzzy; \
	    else \
	      exit 1; \
	    fi; \
	  }; \
	else \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) go test -v -race -count=1 $(GO_TEST_LDFLAGS) ./tests/fuzzy; \
	fi

# Fuzz engine run (uses Go's built-in fuzzing). Adjust TEST_FUZZTIME to control duration.
.PHONY: test-fuzz-engine
test-fuzz-engine:
	$(call print_title,Running Go fuzz engine on fuzzy tests)

ifeq ($(START_LOCAL_DOCKER),1)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
endif
	@set -e; mkdir -p $(TEST_REPORTS_DIR)/fuzz-engine; \
	if [ "$(START_LOCAL_DOCKER)" = "1" ]; then \
	  trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	  $(MAKE) up-backend; \
	  $(MAKE) -s wait-for-services; \
	else \
	  echo "Skipping local backend startup (START_LOCAL_DOCKER=$(START_LOCAL_DOCKER))"; \
	fi; \
	if [ -n "$(GOTESTSUM)" ]; then \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) GOMAXPROCS=$(TEST_GOMAXPROCS) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/fuzz-engine/fuzz-engine.xml -- -v -race -fuzz=Fuzz -run=^$$ -fuzztime=$(TEST_FUZZTIME) $(if $(TEST_PARALLEL),-parallel $(TEST_PARALLEL),) $(GO_TEST_LDFLAGS) ./tests/fuzzy || { \
	    if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	      echo "Retrying fuzz engine once..."; \
	      ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) GOMAXPROCS=$(TEST_GOMAXPROCS) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/fuzz-engine/fuzz-engine-rerun.xml -- -v -race -fuzz=Fuzz -run=^$$ -fuzztime=$(TEST_FUZZTIME) $(if $(TEST_PARALLEL),-parallel $(TEST_PARALLEL),) $(GO_TEST_LDFLAGS) ./tests/fuzzy; \
	    else \
	      exit 1; \
	    fi; \
	  }; \
	else \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) GOMAXPROCS=$(TEST_GOMAXPROCS) go test -v -race -fuzz=Fuzz -run=^$$ -fuzztime=$(TEST_FUZZTIME) $(if $(TEST_PARALLEL),-parallel $(TEST_PARALLEL),) $(GO_TEST_LDFLAGS) ./tests/fuzzy; \
	fi

# Security tests (run only when auth plugin enabled)
.PHONY: test-security
test-security:
	$(call print_title,Running security tests (requires PLUGIN_AUTH_ENABLED=true))
	@echo "Note: set TEST_REQUIRE_AUTH=true and TEST_AUTH_HEADER=\"Bearer <token>\" when plugin is enabled."
	ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) go test -v -race -count=1 $(GO_TEST_LDFLAGS) ./tests/integration -run Security

# Run all tests
.PHONY: test-all
test-all:
	$(call print_title,Running all tests)
	$(call print_title,Running unit tests)
	$(MAKE) test-unit
	$(call print_title,Running security tests)
	$(MAKE) test-security
	$(call print_title,Running integration tests)
	$(MAKE) test-integration
	$(call print_title,Running property tests)
	$(MAKE) test-property
	$(call print_title,Running chaos tests)
	$(MAKE) test-chaos
	$(call print_title,Running e2e tests)
	$(MAKE) test-e2e
	$(call print_title,Running integration e2e tests)
	$(MAKE) test-integration-e2e
	$(call print_title,Running fuzzy tests)
	$(MAKE) test-fuzzy
	$(call print_title,Running fuzz engine tests)
	$(MAKE) test-fuzz-engine


