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
#                        (default -race). LOW_RESOURCE=1 forces it empty.
#   TEST_HEALTH_URL      health endpoint polled by wait-for-services
#                        (root: ledger URL; tracer: tracer URL).
#   INTEG_PACKAGE_PATTERNS
#                        package patterns searched for integration-tagged tests
#                        (root spans the monorepo; tracer narrows its own tree).
#   INTEG_TEST_ENV       env prefix on the integration `go test` line
#                        (root: `CHAOS=$(CHAOS) `; tracer: empty).
#   TEST_REPORTS_DIR     output dir (default ./reports).
#   RETRY_ON_FAIL        retry once on failure (default 0).
#   GO_TEST_LDFLAGS      extra ldflags threaded into every `go test` (default empty).
#   BENCH / BENCH_PKG    benchmark pattern / package filter.
#
# Overridable macros (redefine AFTER include — recipe expansion is late-bound):
#   integ_chaos_notice   shell echo block printed before the integration run
#                        (root prints the CHAOS notice; tracer leaves it empty).
# ------------------------------------------------------

# Banner macro: includers that define $(print_title) (root) get it; otherwise
# fall back to $(title1) from mk/utils.mk. Both are no-op-safe @echo wrappers.
ifndef print_title
print_title = $(call border,📝 $(1))
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

# Freeze the raw command-line value without expanding it as Make syntax, then
# export it for recipes to consume only as environment data. This prevents
# quotes, backticks, and $() in a regular expression from becoming shell code.
override MIDAZ_TEST_RUN := $(value RUN)
export MIDAZ_TEST_RUN

# Build tags select the integration lane. RUN is an explicit, opt-in narrowing
# of the exact Test functions derived from the selected tagged files; no test
# naming convention participates in default selection.

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

# Default integration env prefix (empty). Root sets CHAOS=$(CHAOS).
INTEG_TEST_ENV ?=

# Integration discovery is shared; includers change only the package scope and
# any additional build tags. The helpers fail closed on go list/grep errors and
# on empty package/test selection.
INTEG_PACKAGE_PATTERNS ?= ./components/... ./pkg/... ./tests/...
SELECTED_TEST_LISTER ?= $(MIDAZ_ROOT)/scripts/list-selected-go-tests.sh
TAGGED_TEST_FUNCTION_LISTER ?= $(MIDAZ_ROOT)/scripts/list-tagged-test-functions.sh
GO_TEST_EVENT_VERIFIER ?= $(MIDAZ_ROOT)/scripts/verify-selected-go-test-events.sh
GO_COVERPROFILE_MERGER ?= $(MIDAZ_ROOT)/scripts/merge-go-coverprofiles.sh

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
	@set -e; export ALLOW_INSECURE_TLS=true; export GOFLAGS="-buildvcs=false $${GOFLAGS:-}"; mkdir -p $(TEST_REPORTS_DIR); \
	pkgs=$$(go list ./... | awk '!/\/tests($|\/)/' | awk '!/\/api($|\/)/'); \
	if [ -z "$$pkgs" ]; then \
	  echo "[error] go list discovered no unit test packages outside ./tests." >&2; \
	  echo "[error] The repo root always has unit packages; empty discovery means 'go list ./...' failed" >&2; \
	  echo "[error] (e.g. a VCS-stamp error in a git worktree). Refusing to report a vacuous pass." >&2; \
	  exit 1; \
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
	@go test -buildvcs=false -v -count=1 $(GO_TEST_LDFLAGS) ./tests/openapi/...

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
# Build-tag discovery shared by root and component scopes.

define integ_discover
	if [ -n "$(PKG)" ]; then \
	  echo "Using specified package: $(PKG)"; \
	  package_patterns="$(PKG)"; \
	else \
	  echo "Finding packages selected by the integration build tag..."; \
	  package_patterns="$(INTEG_PACKAGE_PATTERNS)"; \
	fi; \
	tagged_tests=$$("$(TAGGED_TEST_FUNCTION_LISTER)" integration "$(_INTEG_TAGS)" $$package_patterns); \
	pkgs=$$(printf '%s\n' "$$tagged_tests" | awk '!seen[$$1]++ { print $$1 }')
endef

define integ_list
	selected_tests="$$tagged_tests"; \
	selection_dir=""; run_patterns_file=""; \
	if [ -n "$${MIDAZ_TEST_RUN:-}" ]; then \
	  selection_dir=$$(mktemp -d "$${TMPDIR:-/tmp}/midaz-test-selection.XXXXXX"); \
	  trap 'rm -rf "$$selection_dir"' EXIT; \
	  run_patterns_file="$$selection_dir/run-patterns.tsv"; \
	  selected_tests=$$(printf '%s\n' "$$tagged_tests" \
	    | "$(SELECTED_TEST_LISTER)" "$$MIDAZ_TEST_RUN" "$$run_patterns_file"); \
	fi; \
	pkgs=$$(printf '%s\n' "$$selected_tests" | awk '!seen[$$1]++ { print $$1 }'); \
	selected_test_count=$$(printf '%s\n' "$$selected_tests" | awk 'NF { count++ } END { print count + 0 }')
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
	$(integ_list); \
	if [ -z "$$(echo $$pkgs | tr -d ' ')" ]; then \
	  echo "[error] integration discovery returned zero packages" >&2; exit 1; \
	else \
	  echo "Packages: $$pkgs"; \
	  echo "Selected runnable tests: $$selected_test_count"; \
	  echo "Running packages sequentially (-p=1) to avoid Docker container conflicts"; \
	  if [ "$(LOW_RESOURCE)" = "1" ]; then \
	    echo "LOW_RESOURCE mode: -parallel=1, race detector disabled"; \
	  fi; \
	  $(integ_chaos_notice); \
	  if [ -n "$(GOTESTSUM)" ]; then echo "Running testcontainers integration tests with gotestsum"; fi; \
	  if [ -n "$(GOTESTSUM)" ]; then mkdir -p "$(TEST_REPORTS_DIR)/integration-events"; fi; \
	  package_index=0; \
	  for pkg in $$pkgs; do \
	    package_index=$$((package_index + 1)); \
	    package_test_names=$$(printf '%s\n' "$$selected_tests" \
	      | awk -v package="$$pkg" '$$1 == package { print $$2 }'); \
	    if [ -z "$$package_test_names" ]; then \
	      echo "[error] package $$pkg has zero selected integration tests" >&2; exit 1; \
	    fi; \
	    if [ -n "$$run_patterns_file" ]; then \
	      exact_pattern=$$(awk -F '\t' -v package="$$pkg" \
	        '$$1 == package { sub(/^[^\t]*\t/, ""); print; found=1; exit } END { if (!found) exit 1 }' \
	        "$$run_patterns_file"); \
	    else \
	      test_alternation=$$(printf '%s\n' "$$package_test_names" | paste -sd'|' -); \
	      test_anchor='$$'; \
	      exact_pattern="^($$test_alternation)$${test_anchor}"; \
	    fi; \
	    events_file=""; gotestsum_event_flag=""; \
	    if [ -n "$(GOTESTSUM)" ]; then \
	      events_file="$(TEST_REPORTS_DIR)/integration-events/$$package_index.json"; \
	      gotestsum_event_flag="--jsonfile=$$events_file"; \
	    elif [ -n "$$run_patterns_file" ]; then \
	      events_file="$$selection_dir/$$package_index.json"; \
	    fi; \
	    echo "Running $$pkg ($$(printf '%s\n' "$$package_test_names" | awk 'NF { count++ } END { print count + 0 }') tests)"; \
	    if [ -n "$(GOTESTSUM)" ]; then \
	      $(INTEG_TEST_ENV)gotestsum $$gotestsum_event_flag --format testname -- \
	        -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	        -p 1 $(LOW_RES_PARALLEL_FLAG) -run "$$exact_pattern" "$$pkg" || { \
	        if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	          echo "Retrying $$pkg once..."; \
	          $(INTEG_TEST_ENV)gotestsum $$gotestsum_event_flag --format testname -- \
	            -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	            -p 1 $(LOW_RES_PARALLEL_FLAG) -run "$$exact_pattern" "$$pkg"; \
	        else \
	          exit 1; \
	        fi; \
	      }; \
	    elif [ -n "$$events_file" ]; then \
	      if ! $(INTEG_TEST_ENV)go test -json -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	        -p 1 $(LOW_RES_PARALLEL_FLAG) -run "$$exact_pattern" "$$pkg" > "$$events_file"; then \
	        cat "$$events_file"; exit 1; \
	      fi; \
	      cat "$$events_file"; \
	    else \
	      $(INTEG_TEST_ENV)go test -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	        -p 1 $(LOW_RES_PARALLEL_FLAG) -run "$$exact_pattern" "$$pkg"; \
	    fi; \
	    if [ -n "$$run_patterns_file" ]; then \
	      "$(GO_TEST_EVENT_VERIFIER)" "$$exact_pattern" < "$$events_file"; \
	    fi; \
	  done; \
	  if [ -n "$$selection_dir" ]; then rm -rf "$$selection_dir"; trap - EXIT; fi; \
	fi

.PHONY: list-integration-tests
list-integration-tests:
	$(call print_title,Listing integration tests selected by build tags)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@set -e; export ALLOW_INSECURE_TLS=true; \
	$(integ_discover); \
	$(integ_list); \
	echo "Build tags: $(_INTEG_TAGS)"; \
	echo "Packages: $$pkgs"; \
	echo "Selected runnable tests: $$selected_test_count"; \
	printf '%s\n' "$$selected_tests"; \
	if [ -n "$$selection_dir" ]; then rm -rf "$$selection_dir"; trap - EXIT; fi

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
	$(integ_list); \
	if [ -z "$$(echo $$pkgs | tr -d ' ')" ]; then \
	  echo "[error] integration discovery returned zero packages" >&2; exit 1; \
	else \
	  echo "Packages: $$pkgs"; \
	  echo "Selected runnable tests: $$selected_test_count"; \
	  echo "Running packages sequentially (-p=1) to avoid Docker container conflicts"; \
	  if [ "$(LOW_RESOURCE)" = "1" ]; then \
	    echo "LOW_RESOURCE mode: -parallel=1, race detector disabled"; \
	  fi; \
	  $(integ_chaos_notice); \
	  if [ -n "$(GOTESTSUM)" ]; then echo "Running testcontainers integration tests with gotestsum (coverage enabled)"; fi; \
	  coverage_dir=$$(mktemp -d "$(TEST_REPORTS_DIR)/integration-coverage.XXXXXX"); \
	  trap 'rm -rf "$$coverage_dir"; if [ -n "$$selection_dir" ]; then rm -rf "$$selection_dir"; fi' EXIT; \
	  coverage_profiles=""; package_index=0; \
	  for pkg in $$pkgs; do \
	    package_index=$$((package_index + 1)); \
	    package_profile="$$coverage_dir/$$package_index.out"; \
	    coverage_profiles="$$coverage_profiles $$package_profile"; \
	    package_test_names=$$(printf '%s\n' "$$selected_tests" \
	      | awk -v package="$$pkg" '$$1 == package { print $$2 }'); \
	    if [ -z "$$package_test_names" ]; then \
	      echo "[error] package $$pkg has zero selected integration tests" >&2; exit 1; \
	    fi; \
	    if [ -n "$$run_patterns_file" ]; then \
	      exact_pattern=$$(awk -F '\t' -v package="$$pkg" \
	        '$$1 == package { sub(/^[^\t]*\t/, ""); print; found=1; exit } END { if (!found) exit 1 }' \
	        "$$run_patterns_file"); \
	    else \
	      test_alternation=$$(printf '%s\n' "$$package_test_names" | paste -sd'|' -); \
	      test_anchor='$$'; \
	      exact_pattern="^($$test_alternation)$${test_anchor}"; \
	    fi; \
	    events_file=""; gotestsum_event_flag=""; \
	    if [ -n "$$run_patterns_file" ]; then \
	      events_file="$$coverage_dir/$$package_index.json"; \
	      gotestsum_event_flag="--jsonfile=$$events_file"; \
	    fi; \
	    echo "Running $$pkg ($$(printf '%s\n' "$$package_test_names" | awk 'NF { count++ } END { print count + 0 }') tests)"; \
	    if [ -n "$(GOTESTSUM)" ]; then \
	      $(INTEG_TEST_ENV)gotestsum $$gotestsum_event_flag --format testname -- \
	        -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	        -p 1 $(LOW_RES_PARALLEL_FLAG) -run "$$exact_pattern" \
	        -covermode=atomic -coverprofile="$$package_profile" "$$pkg" || { \
	        if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	          echo "Retrying $$pkg once..."; \
	          $(INTEG_TEST_ENV)gotestsum $$gotestsum_event_flag --format testname -- \
	            -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	            -p 1 $(LOW_RES_PARALLEL_FLAG) -run "$$exact_pattern" \
	            -covermode=atomic -coverprofile="$$package_profile" "$$pkg"; \
	        else \
	          exit 1; \
	        fi; \
	      }; \
	    elif [ -n "$$events_file" ]; then \
	      if ! $(INTEG_TEST_ENV)go test -json -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	        -p 1 $(LOW_RES_PARALLEL_FLAG) -run "$$exact_pattern" \
	        -covermode=atomic -coverprofile="$$package_profile" "$$pkg" > "$$events_file"; then \
	        cat "$$events_file"; exit 1; \
	      fi; \
	      cat "$$events_file"; \
	    else \
	      $(INTEG_TEST_ENV)go test -tags=$(_INTEG_TAGS) -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	        -p 1 $(LOW_RES_PARALLEL_FLAG) -run "$$exact_pattern" \
	        -covermode=atomic -coverprofile="$$package_profile" "$$pkg"; \
	    fi; \
	    if [ -n "$$events_file" ]; then \
	      "$(GO_TEST_EVENT_VERIFIER)" "$$exact_pattern" < "$$events_file"; \
	    fi; \
	  done; \
	  "$(GO_COVERPROFILE_MERGER)" "$(TEST_REPORTS_DIR)/integration_coverage.out" $$coverage_profiles; \
	  rm -rf "$$coverage_dir"; \
	  if [ -n "$$selection_dir" ]; then rm -rf "$$selection_dir"; fi; \
	  trap - EXIT; \
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
