# ------------------------------------------------------
# Shared parameterized Go test scaffolding
# ------------------------------------------------------
# Extracted from the duplicated bodies of the root mk/tests.mk and the tracer
# component mk/tests.mk. Holds the gotestsum-detect / retry-on-fail execution
# pattern and the common test targets (test-unit, test-integration,
# coverage-integration, test-bench, test-all) plus wait-for-services. Includers
# override behavior through the `?=` knobs and the overridable macros below; the
# generated `go test` invocations stay byte-identical to the pre-extraction
# recipes for each includer.
#
# coverage-unit is deliberately NOT here: the generic version lives in
# mk/coverage-unit.mk (frozen CI contract, output reports/unit_coverage.out).
# Tracer keeps its own testhooks-tagged coverage-unit in its component tests.mk.
#
# Knobs (set BEFORE include to override the default):
#   GO_TEST_BUILD_TAGS   extra build tags threaded into every `go test`
#                        (empty for root components; tracer sets `testhooks`).
#                        Unit emits `-tags=<tags>` only when non-empty; integration
#                        emits `-tags=integration[,<tags>]`.
#   INTEG_RACE_FLAG      base race flag for integration/coverage-integration
#                        (default -race; tracer sets it EMPTY for a documented
#                        lib-commons race). LOW_RESOURCE=1 still forces it empty.
#   TEST_HEALTH_URL      health endpoint polled by wait-for-services
#                        (root: ledger URL; tracer: tracer URL).
#   INTEG_RUN_FLAG       -run flag for integration discovery (root defaults to
#                        `-run '$(RUN_PATTERN)'`; tracer overrides to `$(RUN_FLAG)`).
#   INTEG_TEST_ENV       env prefix on the integration `go test` line
#                        (root: `CHAOS=$(CHAOS) `; tracer: empty).
#   TEST_REPORTS_DIR     output dir (default ./reports).
#   RETRY_ON_FAIL        retry once on failure (default 0).
#   GO_TEST_LDFLAGS      extra ldflags threaded into every `go test` (default empty).
#   BENCH / BENCH_PKG    benchmark pattern / package filter.
#
# Overridable macros (redefine AFTER include — recipe expansion is late-bound):
#   integ_discover       shell that populates `pkgs` for the integration targets.
#   integ_chaos_notice   shell echo block printed before the integration run
#                        (root prints the CHAOS notice; tracer leaves it empty).
# ------------------------------------------------------

# Banner macro: includers that define $(print_title) (root) get it; otherwise
# fall back to $(title1) from mk/utils.mk. Both are no-op-safe @echo wrappers.
ifndef print_title
print_title = $(call title1,$(1))
endif

# A single, space-preserving literal — used to keep an optional flag token from
# collapsing two adjacent spaces in the assembled `go test` command.
empty :=
space := $(empty) $(empty)

TEST_REPORTS_DIR ?= ./reports
GOTESTSUM        := $(shell command -v gotestsum 2>/dev/null)
RETRY_ON_FAIL    ?= 0
GO_TEST_LDFLAGS  ?=

# Extra build tags threaded into every `go test`.
GO_TEST_BUILD_TAGS ?=

# Unit tag flag: empty when no extra tags, else `-tags=<tags> ` (TRAILING space so
# `-- $(_UNIT_TAGS_FLAG)-v` collapses to `-- -v` when empty, preserving the exact
# pre-extraction spacing in the assembled command).
ifeq ($(strip $(GO_TEST_BUILD_TAGS)),)
  _UNIT_TAGS_FLAG :=
  _INTEG_TAGS := integration
else
  _UNIT_TAGS_FLAG := -tags=$(GO_TEST_BUILD_TAGS)$(space)
  _INTEG_TAGS := integration,$(GO_TEST_BUILD_TAGS)
endif

# Health endpoint for wait-for-services.
TEST_HEALTH_URL  ?= http://localhost:3000
TEST_HEALTH_WAIT ?= 60

# Integration test filters.
# RUN: specific test name pattern. PKG: specific package to test.
RUN ?=
PKG ?=

# Computed run pattern (default '^TestIntegration' when RUN unset).
ifeq ($(RUN),)
  RUN_PATTERN := ^TestIntegration
else
  RUN_PATTERN := $(RUN)
endif

# Low-resource mode for limited machines (sets -p=1 -parallel=1, disables -race).
LOW_RESOURCE ?= 0

# Base integration race flag (overridable). Tracer sets it empty.
INTEG_RACE_FLAG ?= -race

ifeq ($(LOW_RESOURCE),1)
  LOW_RES_P_FLAG := -p 1
  LOW_RES_PARALLEL_FLAG := -parallel 1
  LOW_RES_RACE_FLAG :=
else
  LOW_RES_P_FLAG :=
  LOW_RES_PARALLEL_FLAG :=
  LOW_RES_RACE_FLAG := $(INTEG_RACE_FLAG)
endif

# Default integration run flag (root behavior). Tracer overrides to $(RUN_FLAG).
INTEG_RUN_FLAG ?= -run '$(RUN_PATTERN)'

# Default integration env prefix (empty). Root sets CHAOS=$(CHAOS).
INTEG_TEST_ENV ?=

#-------------------------------------------------------
# wait-for-services
#-------------------------------------------------------

define wait_for_services
	echo "Waiting for services to become healthy..."
	bash -c 'for i in $$(seq 1 $(TEST_HEALTH_WAIT)); do \
	  if curl -fsS $(TEST_HEALTH_URL)/health >/dev/null 2>&1; then \
	    echo "Services are up"; exit 0; \
	  fi; \
	  sleep 1; \
	done; echo "[error] Services not healthy after $(TEST_HEALTH_WAIT)s"; exit 1'
endef

.PHONY: wait-for-services
wait-for-services:
	$(call wait_for_services)

#-------------------------------------------------------
# Unit tests
#-------------------------------------------------------

.PHONY: test-unit
test-unit:
	$(call print_title,Running Go unit tests)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@set -e; export ALLOW_INSECURE_TLS=true; mkdir -p $(TEST_REPORTS_DIR); \
	pkgs=$$(go list ./... | awk '!/\/tests($|\/)/' | awk '!/\/api($|\/)/'); \
	if [ -z "$$pkgs" ]; then \
	  echo "No unit test packages found (outside ./tests)**"; \
	else \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    echo "Running unit tests with gotestsum"; \
	    gotestsum --format testname -- $(_UNIT_TAGS_FLAG)-v -race -count=1 $(GO_TEST_LDFLAGS) $$pkgs || { \
	      if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	        echo "Retrying unit tests once..."; \
	        gotestsum --format testname -- $(_UNIT_TAGS_FLAG)-v -race -count=1 $(GO_TEST_LDFLAGS) $$pkgs; \
	      else \
	        exit 1; \
	      fi; \
	    }; \
	  else \
	    go test $(_UNIT_TAGS_FLAG)-v -race -count=1 $(GO_TEST_LDFLAGS) $$pkgs; \
	  fi; \
	fi

#-------------------------------------------------------
# Cross-plane OpenAPI spec locks (offline)
#-------------------------------------------------------
# The ./tests/openapi package holds offline cross-plane locks over the committed
# native Huma OAS 3.1 dumps — chiefly the byte-identical RFC 9457 Error closure
# across the ledger and tracer planes that the SDK depends on. They read the yaml
# dumps only (no server, DB, or Docker), but live under ./tests, which test-unit
# deliberately excludes because that path is otherwise integration-only. Run them
# explicitly so the parity lock is actually enforced by the gate; ci invokes this
# after check-docs, so the dumps the locks read are the freshly-verified ones.
.PHONY: test-openapi-locks
test-openapi-locks:
	$(call print_title,Running cross-plane OpenAPI spec locks)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@go test -v -count=1 $(GO_TEST_LDFLAGS) ./tests/openapi/...

#-------------------------------------------------------
# Benchmark tests
#-------------------------------------------------------

BENCH ?= .
BENCH_PKG ?= ./...

.PHONY: test-bench
test-bench:
	$(call print_title,Running Go benchmark tests)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@echo "Benchmark pattern: $(BENCH)"
	@echo "Package: $(BENCH_PKG)"
	@go test -bench=$(BENCH) -benchmem -run=^$$ $(BENCH_PKG)

#-------------------------------------------------------
# Integration tests (testcontainers, no coverage)
#-------------------------------------------------------
# Default discovery (root): grep //go:build integration across the monorepo.
# Tracer redefines integ_discover after include with its find-based discovery.

define integ_discover
	if [ -n "$(PKG)" ]; then \
	  echo "Using specified package: $(PKG)"; \
	  pkgs=$$(go list -tags=$(_INTEG_TAGS) $(PKG) 2>/dev/null | tr '\n' ' '); \
	else \
	  echo "Finding packages with //go:build integration files..."; \
	  dirs=$$(grep -rl '^//go:build integration' --include='*_test.go' ./components ./pkg ./tests 2>/dev/null | xargs -n1 dirname 2>/dev/null | sort -u | tr '\n' ' '); \
	  pkgs=$$(if [ -n "$$dirs" ]; then go list -tags=$(_INTEG_TAGS) $$dirs 2>/dev/null | tr '\n' ' '; fi); \
	fi
endef

# Default chaos notice (root): print the CHAOS branch echoes. Tracer overrides empty.
define integ_chaos_notice
	if [ "$(CHAOS)" = "1" ]; then \
	  echo "CHAOS=1: Chaos tests (TestIntegration_Chaos_*) will run"; \
	else \
	  echo "Chaos tests will be skipped (set CHAOS=1 to include them)"; \
	fi
endef

.PHONY: test-integration
test-integration:
	$(call print_title,Running integration tests with testcontainers)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	@set -e; export ALLOW_INSECURE_TLS=true; mkdir -p $(TEST_REPORTS_DIR); \
	$(integ_discover); \
	if [ -z "$$(echo $$pkgs | tr -d ' ')" ]; then \
	  echo "No integration test packages found"; \
	else \
	  echo "Packages: $$pkgs"; \
	  echo "Running packages sequentially (-p=1) to avoid Docker container conflicts"; \
	  if [ "$(LOW_RESOURCE)" = "1" ]; then \
	    echo "LOW_RESOURCE mode: -parallel=1, race detector disabled"; \
	  fi; \
	  $(integ_chaos_notice); \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    echo "Running testcontainers integration tests with gotestsum"; \
	    $(INTEG_TEST_ENV)gotestsum --format testname -- \
	      -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) \
	      $(INTEG_RUN_FLAG) $$pkgs || { \
	      if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	        echo "Retrying integ tests once..."; \
	        $(INTEG_TEST_ENV)gotestsum --format testname -- \
	          -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	          -p 1 $(LOW_RES_PARALLEL_FLAG) \
	          $(INTEG_RUN_FLAG) $$pkgs; \
	      else \
	        exit 1; \
	      fi; \
	    }; \
	  else \
	    $(INTEG_TEST_ENV)go test -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) \
	      $(INTEG_RUN_FLAG) $$pkgs; \
	  fi; \
	fi

#-------------------------------------------------------
# Integration tests with coverage (covermode=atomic)
#-------------------------------------------------------

.PHONY: coverage-integration
coverage-integration:
	$(call print_title,Running integration tests with testcontainers (coverage enabled))
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	@set -e; export ALLOW_INSECURE_TLS=true; mkdir -p $(TEST_REPORTS_DIR); \
	$(integ_discover); \
	if [ -z "$$(echo $$pkgs | tr -d ' ')" ]; then \
	  echo "No integration test packages found"; \
	else \
	  echo "Packages: $$pkgs"; \
	  echo "Running packages sequentially (-p=1) to avoid Docker container conflicts"; \
	  if [ "$(LOW_RESOURCE)" = "1" ]; then \
	    echo "LOW_RESOURCE mode: -parallel=1, race detector disabled"; \
	  fi; \
	  $(integ_chaos_notice); \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    echo "Running testcontainers integration tests with gotestsum (coverage enabled)"; \
	    $(INTEG_TEST_ENV)gotestsum --format testname -- \
	      -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) \
	      $(INTEG_RUN_FLAG) -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/integration_coverage.out \
	      $$pkgs || { \
	      if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	        echo "Retrying integ tests once..."; \
	        $(INTEG_TEST_ENV)gotestsum --format testname -- \
	          -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	          -p 1 $(LOW_RES_PARALLEL_FLAG) \
	          $(INTEG_RUN_FLAG) -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/integration_coverage.out \
	          $$pkgs; \
	      else \
	        exit 1; \
	      fi; \
	    }; \
	  else \
	    $(INTEG_TEST_ENV)go test -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) \
	      $(INTEG_RUN_FLAG) -covermode=atomic -coverprofile=$(TEST_REPORTS_DIR)/integration_coverage.out \
	      $$pkgs; \
	  fi; \
	  echo "----------------------------------------"; \
	  go tool cover -func=$(TEST_REPORTS_DIR)/integration_coverage.out | grep total | awk '{print "Total coverage: " $$3}'; \
	  echo "----------------------------------------"; \
	fi

#-------------------------------------------------------
# Run all tests (unit + integration)
#-------------------------------------------------------

.PHONY: test-all
test-all:
	$(call print_title,Running all tests)
	$(call print_title,Running unit tests)
	$(MAKE) test-unit
	$(call print_title,Running integration tests)
	$(MAKE) test-integration
