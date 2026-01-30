# ------------------------------------------------------
# Test configuration (extracted from root Makefile)
# ------------------------------------------------------
TEST_ONBOARDING_URL ?= http://localhost:3000
TEST_TRANSACTION_URL ?= http://localhost:3001
TEST_HEALTH_WAIT ?= 60

# Optional auth configuration (passed through to tests)
TEST_AUTH_URL ?=
TEST_AUTH_USERNAME ?=
TEST_AUTH_PASSWORD ?=

# Native fuzz test controls
# FUZZ: specific fuzz target name (e.g., FuzzCreateOrganization_LegalName)
# FUZZTIME: duration per fuzz target (default: 10s)
FUZZ ?=
FUZZTIME ?= 10s

# Integration test filter
# RUN: specific test name pattern (e.g., TestIntegration_AliasRepo_Create)
# PKG: specific package to test (e.g., ./components/transaction/...)
# Usage: make test-integration RUN=TestIntegration_AliasRepo_Create
#        make test-integration PKG=./components/transaction/...
#        make test-integration RUN=TestIntegration_Chaos_Redis PKG=./components/transaction/... CHAOS=1
RUN ?=
PKG ?=

# Computed run pattern: uses RUN if set, otherwise defaults to '^TestIntegration'
ifeq ($(RUN),)
  RUN_PATTERN := ^TestIntegration
else
  RUN_PATTERN := $(RUN)
endif

# Low-resource mode for limited machines (sets -p=1 -parallel=1, disables -race)
# Usage: make test-integration LOW_RESOURCE=1
#        make coverage-integration LOW_RESOURCE=1
LOW_RESOURCE ?= 0

# Computed flags for low-resource mode
ifeq ($(LOW_RESOURCE),1)
  LOW_RES_P_FLAG := -p 1
  LOW_RES_PARALLEL_FLAG := -parallel 1
  LOW_RES_RACE_FLAG :=
else
  LOW_RES_P_FLAG :=
  LOW_RES_PARALLEL_FLAG :=
  LOW_RES_RACE_FLAG := -race
endif

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
	@set -e; mkdir -p $(TEST_REPORTS_DIR); \
	pkgs=$$(go list ./... | awk '!/\/tests($|\/)/' | awk '!/\/api($|\/)/'); \
	if [ -z "$$pkgs" ]; then \
	  echo "No unit test packages found (outside ./tests)**"; \
	else \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    echo "Running unit tests with gotestsum"; \
	    gotestsum --format testname -- -v -race -count=1 $(GO_TEST_LDFLAGS) $$pkgs || { \
	      if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	        echo "Retrying unit tests once..."; \
	        gotestsum --format testname -- -v -race -count=1 $(GO_TEST_LDFLAGS) $$pkgs; \
	      else \
	        exit 1; \
	      fi; \
	    }; \
	  else \
	    go test -v -race -count=1 $(GO_TEST_LDFLAGS) $$pkgs; \
	  fi; \
	fi

# Unit tests with coverage (uses covermode=atomic)
# Supports PKG parameter to filter packages (e.g., PKG=./components/transaction/...)
# Supports .ignorecoverunit file to exclude patterns from coverage stats
.PHONY: coverage-unit
coverage-unit:
	$(call print_title,Running Go unit tests with coverage)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@set -e; mkdir -p $(TEST_REPORTS_DIR); \
	if [ -n "$(PKG)" ]; then \
	  echo "Using specified package: $(PKG)"; \
	  pkgs=$$(go list $(PKG) 2>/dev/null | awk '!/\/tests($|\/)/' | awk '!/\/api($|\/)/' | tr '\n' ' '); \
	else \
	  pkgs=$$(go list ./... | awk '!/\/tests($|\/)/' | awk '!/\/api($|\/)/'); \
	fi; \
	if [ -z "$$pkgs" ]; then \
	  echo "No unit test packages found (outside ./tests)"; \
	else \
	  echo "Packages: $$pkgs"; \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    echo "Running unit tests with gotestsum (coverage enabled)"; \
	    gotestsum --format testname -- -v -race -count=1 $(GO_TEST_LDFLAGS) -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/unit_coverage.out $$pkgs || { \
	      if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	        echo "Retrying unit tests once..."; \
	        gotestsum --format testname -- -v -race -count=1 $(GO_TEST_LDFLAGS) -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/unit_coverage.out $$pkgs; \
	      else \
	        exit 1; \
	      fi; \
	    }; \
	  else \
	    go test -v -race -count=1 $(GO_TEST_LDFLAGS) -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/unit_coverage.out $$pkgs; \
	  fi; \
	  if [ -f .ignorecoverunit ]; then \
	    echo "Filtering coverage with .ignorecoverunit patterns..."; \
	    patterns=$$(grep -v '^#' .ignorecoverunit | grep -v '^$$' | tr '\n' '|' | sed 's/|$$//'); \
	    if [ -n "$$patterns" ]; then \
	      regex_patterns=$$(echo "$$patterns" | sed 's/\./\\./g' | sed 's/\*/.*/g'); \
	      head -1 $(TEST_REPORTS_DIR)/unit_coverage.out > $(TEST_REPORTS_DIR)/unit_coverage_filtered.out; \
	      tail -n +2 $(TEST_REPORTS_DIR)/unit_coverage.out | grep -vE "$$regex_patterns" >> $(TEST_REPORTS_DIR)/unit_coverage_filtered.out || true; \
	      mv $(TEST_REPORTS_DIR)/unit_coverage_filtered.out $(TEST_REPORTS_DIR)/unit_coverage.out; \
	      echo "Excluded patterns: $$patterns"; \
	    fi; \
	  fi; \
	  echo "----------------------------------------"; \
	  go tool cover -func=$(TEST_REPORTS_DIR)/unit_coverage.out | grep total | awk '{print "Total coverage: " $$3}'; \
	  echo "----------------------------------------"; \
	fi

# System-level chaos tests (full stack with docker-compose)
# Starts the complete backend stack, runs chaos tests, then tears down.
.PHONY: test-chaos-system
test-chaos-system:
	$(call print_title,Running system-level chaos tests)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@set -e; mkdir -p $(TEST_REPORTS_DIR)/chaos; \
	trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	$(MAKE) up-backend; \
	$(MAKE) -s wait-for-services; \
	if [ -n "$(GOTESTSUM)" ]; then \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/chaos/chaos-system.xml -- -v -race -timeout 30m -count=1 $(GO_TEST_LDFLAGS) ./tests/chaos || { \
	    if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	      echo "Retrying chaos tests once..."; \
	      ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/chaos/chaos-system-rerun.xml -- -v -race -timeout 30m -count=1 $(GO_TEST_LDFLAGS) ./tests/chaos; \
	    else \
	      exit 1; \
	    fi; \
	  }; \
	else \
	  ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) go test -v -race -timeout 30m -count=1 $(GO_TEST_LDFLAGS) ./tests/chaos; \
	fi

# Native Go fuzz tests (coverage-guided mutation testing).
# Usage:
#   make test-fuzz                                    # Run all Fuzz* targets for 10s each
#   make test-fuzz FUZZTIME=30s                       # Run all Fuzz* targets for 30s each
#   make test-fuzz FUZZ=FuzzCreateOrganization_LegalName  # Run specific target
#   make test-fuzz FUZZ=FuzzCreateOrganization_LegalName FUZZTIME=60s
.PHONY: test-fuzz
test-fuzz:
	$(call print_title,Running Go native fuzz tests)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@set -e; mkdir -p $(TEST_REPORTS_DIR)/fuzz; \
	if [ -n "$(FUZZ)" ]; then \
	  echo "Running fuzz target: $(FUZZ) for $(FUZZTIME)"; \
	  pkg=$$(grep -r "func $(FUZZ)" --include='*_test.go' -l ./components ./pkg 2>/dev/null | head -1 | xargs dirname); \
	  if [ -z "$$pkg" ]; then \
	    echo "Error: Fuzz target '$(FUZZ)' not found"; exit 1; \
	  fi; \
	  go test -v -fuzz=$(FUZZ) -run='^$$' -fuzztime=$(FUZZTIME) $(GO_TEST_LDFLAGS) $$pkg; \
	else \
	  echo "Discovering all Fuzz* targets..."; \
	  targets=$$(grep -r "^func Fuzz" --include='*_test.go' -h ./components ./pkg 2>/dev/null | sed 's/func \(Fuzz[^(]*\).*/\1/' | sort -u); \
	  if [ -z "$$targets" ]; then \
	    echo "No Fuzz* targets found"; exit 0; \
	  fi; \
	  echo "Found targets: $$targets"; \
	  echo "Running each for $(FUZZTIME)..."; \
	  echo ""; \
	  for target in $$targets; do \
	    pkg=$$(grep -r "func $$target" --include='*_test.go' -l ./components ./pkg 2>/dev/null | head -1 | xargs dirname); \
	    echo "━━━ $$target ($$pkg) ━━━"; \
	    go test -v -fuzz=$$target -run='^$$' -fuzztime=$(FUZZTIME) $(GO_TEST_LDFLAGS) $$pkg || true; \
	    echo ""; \
	  done; \
	  echo "Fuzz testing complete. Check testdata/fuzz/ for corpus."; \
	fi

# Benchmark tests
# Run performance benchmarks for critical code paths.
# Usage:
#   make test-bench                          # Run all benchmarks
#   make test-bench BENCH=OperateBalances    # Run specific benchmark pattern
#   make test-bench BENCH_PKG=./pkg/transaction/...  # Run benchmarks in specific package
BENCH ?= .
BENCH_PKG ?= ./...

.PHONY: test-bench
test-bench:
	$(call print_title,Running Go benchmark tests)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@echo "Benchmark pattern: $(BENCH)"
	@echo "Package: $(BENCH_PKG)"
	@go test -bench=$(BENCH) -benchmem -run=^$$ $(BENCH_PKG)

# Integration tests with testcontainers (no coverage)
# These tests use the `integration` build tag and testcontainers-go to spin up
# ephemeral containers. No external Docker stack is required.
#
# NOTE: Integration tests always run with -p=1 (packages sequentially) because
# testcontainers can overwhelm Docker when creating many containers in parallel.
# This prevents transient failures like "port not found" or container timeouts.
#
# Requirements:
#   - Test files must follow the naming convention: *_integration_test.go
#   - Test functions must start with TestIntegration_ (e.g., TestIntegration_MyFeature_Works)
#   - Chaos tests use TestIntegration_Chaos_ prefix (e.g., TestIntegration_Chaos_Redis_NetworkPartition)
#
# Chaos tests (CHAOS=1):
#   Chaos tests are included in integration test files but skip themselves by default.
#   To run chaos tests alongside integration tests, set CHAOS=1:
#     make test-integration CHAOS=1
#   This enables network chaos injection, container restarts, and other failure scenarios.
.PHONY: test-integration
test-integration:
	$(call print_title,Running integration tests with testcontainers)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	@set -e; mkdir -p $(TEST_REPORTS_DIR); \
	if [ -n "$(PKG)" ]; then \
	  echo "Using specified package: $(PKG)"; \
	  pkgs=$$(go list $(PKG) 2>/dev/null | tr '\n' ' '); \
	else \
	  echo "Finding packages with *_integration_test.go files..."; \
	  dirs=$$(find ./components ./pkg -name '*_integration_test.go' 2>/dev/null | xargs -n1 dirname 2>/dev/null | sort -u | tr '\n' ' '); \
	  pkgs=$$(if [ -n "$$dirs" ]; then go list $$dirs 2>/dev/null | tr '\n' ' '; fi); \
	fi; \
	if [ -z "$$pkgs" ]; then \
	  echo "No integration test packages found"; \
	else \
	  echo "Packages: $$pkgs"; \
	  echo "Running packages sequentially (-p=1) to avoid Docker container conflicts"; \
	  if [ "$(LOW_RESOURCE)" = "1" ]; then \
	    echo "LOW_RESOURCE mode: -parallel=1, race detector disabled"; \
	  fi; \
	  if [ "$(CHAOS)" = "1" ]; then \
	    echo "CHAOS=1: Chaos tests (TestIntegration_Chaos_*) will run"; \
	  else \
	    echo "Chaos tests will be skipped (set CHAOS=1 to include them)"; \
	  fi; \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    echo "Running testcontainers integration tests with gotestsum"; \
	    CHAOS=$(CHAOS) gotestsum --format testname -- \
	      -tags=integration -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) \
	      -run '$(RUN_PATTERN)' $$pkgs || { \
	      if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	        echo "Retrying integ tests once..."; \
	        CHAOS=$(CHAOS) gotestsum --format testname -- \
	          -tags=integration -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	          -p 1 $(LOW_RES_PARALLEL_FLAG) \
	          -run '$(RUN_PATTERN)' $$pkgs; \
	      else \
	        exit 1; \
	      fi; \
	    }; \
	  else \
	    CHAOS=$(CHAOS) go test -tags=integration -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) \
	      -run '$(RUN_PATTERN)' $$pkgs; \
	  fi; \
	fi

# Integration tests with testcontainers (with coverage, uses covermode=atomic)
#
# NOTE: Integration tests always run with -p=1 (packages sequentially) because
# testcontainers can overwhelm Docker when creating many containers in parallel.
# This prevents transient failures like "port not found" or container timeouts.
#
# Chaos tests (CHAOS=1):
#   Chaos tests (TestIntegration_Chaos_*) skip themselves by default.
#   To include chaos tests in coverage, set CHAOS=1:
#     make coverage-integration CHAOS=1
.PHONY: coverage-integration
coverage-integration:
	$(call print_title,Running integration tests with testcontainers (coverage enabled))
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	@set -e; mkdir -p $(TEST_REPORTS_DIR); \
	if [ -n "$(PKG)" ]; then \
	  echo "Using specified package: $(PKG)"; \
	  pkgs=$$(go list $(PKG) 2>/dev/null | tr '\n' ' '); \
	else \
	  echo "Finding packages with *_integration_test.go files..."; \
	  dirs=$$(find ./components ./pkg -name '*_integration_test.go' 2>/dev/null | xargs -n1 dirname 2>/dev/null | sort -u | tr '\n' ' '); \
	  pkgs=$$(if [ -n "$$dirs" ]; then go list $$dirs 2>/dev/null | tr '\n' ' '; fi); \
	fi; \
	if [ -z "$$pkgs" ]; then \
	  echo "No integration test packages found"; \
	else \
	  echo "Packages: $$pkgs"; \
	  echo "Running packages sequentially (-p=1) to avoid Docker container conflicts"; \
	  if [ "$(LOW_RESOURCE)" = "1" ]; then \
	    echo "LOW_RESOURCE mode: -parallel=1, race detector disabled"; \
	  fi; \
	  if [ "$(CHAOS)" = "1" ]; then \
	    echo "CHAOS=1: Chaos tests (TestIntegration_Chaos_*) will run"; \
	  else \
	    echo "Chaos tests will be skipped (set CHAOS=1 to include them)"; \
	  fi; \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    echo "Running testcontainers integration tests with gotestsum (coverage enabled)"; \
	    CHAOS=$(CHAOS) gotestsum --format testname -- \
	      -tags=integration -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) \
	      -run '$(RUN_PATTERN)' -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/integration_coverage.out \
	      $$pkgs || { \
	      if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	        echo "Retrying integ tests once..."; \
	        CHAOS=$(CHAOS) gotestsum --format testname -- \
	          -tags=integration -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	          -p 1 $(LOW_RES_PARALLEL_FLAG) \
	          -run '$(RUN_PATTERN)' -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/integration_coverage.out \
	          $$pkgs; \
	      else \
	        exit 1; \
	      fi; \
	    }; \
	  else \
	    CHAOS=$(CHAOS) go test -tags=integration -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) \
	      -run '$(RUN_PATTERN)' -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/integration_coverage.out \
	      $$pkgs; \
	  fi; \
	  echo "----------------------------------------"; \
	  go tool cover -func=$(TEST_REPORTS_DIR)/integration_coverage.out | grep total | awk '{print "Total coverage: " $$3}'; \
	  echo "----------------------------------------"; \
	fi

# Run all coverage targets
.PHONY: coverage
coverage:
	$(call print_title,Running all coverage targets)
	$(MAKE) coverage-unit
	$(MAKE) coverage-integration

# Run all tests (excludes native fuzz engine which runs indefinitely)
# To include chaos tests: make test-all CHAOS=1
.PHONY: test-all
test-all:
	$(call print_title,Running all tests)
	$(call print_title,Running unit tests)
	$(MAKE) test-unit
	$(call print_title,Running integration tests)
	$(MAKE) test-integration


