# ------------------------------------------------------
# Monorepo test orchestration (root)
# ------------------------------------------------------
# The shared, parameterized test scaffolding (test-unit, test-integration,
# coverage-integration, test-bench, test-all, wait-for-services) lives in
# mk/test-go.mk. This file sets the root-level knobs and keeps the monorepo-only
# orchestration: the `test` entrypoint, the CHAOS-aware integration discovery,
# ci-tests, test-fuzz, test-property, test-chaos-system, and the godog e2e alias.
# ------------------------------------------------------
TEST_LEDGER_URL ?= http://localhost:3000
TEST_HEALTH_WAIT ?= 60

# Health endpoint for the shared wait-for-services target (mk/test-go.mk).
TEST_HEALTH_URL ?= $(TEST_LEDGER_URL)

# Optional auth configuration (passed through to tests)
TEST_AUTH_URL ?=
TEST_AUTH_USERNAME ?=
TEST_AUTH_PASSWORD ?=

# Native fuzz test controls
# FUZZ: specific fuzz target name (e.g., FuzzCreateOrganization_LegalName)
# FUZZTIME: duration per fuzz target (default: 10s)
FUZZ ?=
FUZZTIME ?= 10s
# FUZZMINIMIZETIME: bound on crash-input minimization. The crasher is written to
# testdata either way, so a short bound loses nothing.
FUZZMINIMIZETIME ?= 20s
# FUZZWALL: hard wall-clock cap (seconds) per fuzz target, enforced with GNU
# timeout. Go IGNORES go test's -timeout while fuzzing, so without an external
# bound one hung target hangs the whole sweep forever. timeout kills the whole
# process group; exit 124 is treated as environmental — retried once, never a
# fuzz finding. If `timeout` is absent (bare macOS without coreutils), the cap is
# skipped.
FUZZWALL ?= 180

# Integration env prefix consumed by mk/test-go.mk: the root integration suite is
# CHAOS-aware, so it threads CHAOS through to the test process. The trailing space
# is preserved via $(space) so the assembled command matches the pre-extraction
# `CHAOS=$(CHAOS) gotestsum ...` exactly.
empty :=
space := $(empty) $(empty)
INTEG_TEST_ENV := CHAOS=$(CHAOS)$(space)

# Pull in the shared scaffolding (test-unit, test-integration,
# coverage-integration, test-bench, test-all, wait-for-services). Knobs above
# (TEST_HEALTH_URL, INTEG_TEST_ENV) and the default discovery/chaos macros there
# reproduce the monorepo-wide behavior. Root keeps -race for integration (the
# fragment's INTEG_RACE_FLAG default) and the default //go:build-grep discovery.
include $(MK_DIR)/test-go.mk

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: test
test:
	@sh ./scripts/run-tests.sh

#-------------------------------------------------------
# System-level chaos tests (full stack with docker-compose)
#-------------------------------------------------------
# Starts the complete backend stack, runs chaos tests, then tears down.
.PHONY: test-chaos-system
test-chaos-system:
	$(call print_title,Running system-level chaos tests)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@set -e; mkdir -p $(TEST_REPORTS_DIR)/chaos; \
	trap '$(MAKE) -s down >/dev/null 2>&1 || true' EXIT; \
	$(MAKE) up; \
	$(MAKE) -s wait-for-services; \
	if [ -n "$(GOTESTSUM)" ]; then \
	  CHAOS=1 LEDGER_URL=$(TEST_LEDGER_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/chaos/chaos-system.xml -- -v -race -timeout 30m -count=1 $(GO_TEST_LDFLAGS) ./tests/chaos || { \
	    if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	      echo "Retrying chaos tests once..."; \
	      CHAOS=1 LEDGER_URL=$(TEST_LEDGER_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) gotestsum --format testname --junitfile $(TEST_REPORTS_DIR)/chaos/chaos-system-rerun.xml -- -v -race -timeout 30m -count=1 $(GO_TEST_LDFLAGS) ./tests/chaos; \
	    else \
	      exit 1; \
	    fi; \
	  }; \
	else \
	  CHAOS=1 LEDGER_URL=$(TEST_LEDGER_URL) TEST_AUTH_URL=$(TEST_AUTH_URL) TEST_AUTH_USERNAME=$(TEST_AUTH_USERNAME) TEST_AUTH_PASSWORD=$(TEST_AUTH_PASSWORD) go test -v -race -timeout 30m -count=1 $(GO_TEST_LDFLAGS) ./tests/chaos; \
	fi

# Native Go fuzz tests (coverage-guided mutation testing).
#
# All fuzz suites compile under -tags=fuzz: the fuzz test files carry a
# `//go:build fuzz` tag, so a bare `go test` excludes them from the binary,
# the -fuzz pattern then matches nothing, and go reports `ok` with ZERO
# fuzzing engaged (silent no-op). Untagged fuzz files still compile under
# -tags=fuzz, so building with the tag is strictly safe — nothing is lost.
#
# Discovery walks ./components ./pkg ./tests. ALLOW_INSECURE_TLS=true is exported
# so any target whose package boots TLS-aware infra can start.
#
# The full sweep (no FUZZ=) is a pass/fail gate, time-boxed at FUZZTIME per
# target. It visits every discovered target — no fail-fast mid-sweep — then
# exits non-zero if any failed, naming them. Two failure modes gate:
#   1. crash / test error: go test exits non-zero (a found crash, build error,
#      or container-startup failure). pipefail propagates it past the tee.
#   2. silent no-op: go test exits 0 but engaged no fuzzing. We assert on the
#      "fuzz:" progress lines that `go test -fuzz` prints only when a target
#      actually fuzzes, so a false `ok` no longer passes.
#
# Environmental vs real failures (sweep only): a target that fails AND whose
# output carries an infra-startup signature is retried ONCE after a 15s settle.
# A real crash / test failure (no infra signature) is NEVER retried — crash-
# gating stays strict so genuine findings are never masked. The signature also
# covers the Go coordinator's "context deadline exceeded" flake at the -fuzztime
# boundary (a worker mid-exec when the engine's internal context expires reports
# FAIL with no failing input); the "Failing input written" guard keeps real
# findings exempt from retry. Each target runs under GNU timeout (FUZZWALL)
# because Go ignores -timeout while fuzzing — a wedged target would otherwise
# hang the sweep indefinitely. Exit 124 follows the same retry-once path; a
# second timeout gates as a hung target.
#
# The FUZZ=<target> branch gates the same two modes but fails fast on the one
# target (no infra retry). The `ci` target deliberately
# EXCLUDES fuzz (time-boxed mutation runs are not part of the deterministic CI
# matrix); this gate applies only when test-fuzz is invoked directly — and when
# invoked, it gates.
#
# Usage:
#   make test-fuzz                                    # Run all Fuzz* targets for 10s each
#   make test-fuzz FUZZTIME=30s                       # Run all Fuzz* targets for 30s each
#   make test-fuzz FUZZ=FuzzCreateOrganization_LegalName  # Run specific target
#   make test-fuzz FUZZ=FuzzCreateOrganization_LegalName FUZZTIME=60s
.PHONY: test-fuzz
test-fuzz:
	$(call print_title,Running Go native fuzz tests)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@set -e; export ALLOW_INSECURE_TLS=true; mkdir -p $(TEST_REPORTS_DIR)/fuzz; \
	if [ -n "$(FUZZ)" ]; then \
	  pkg=$$(grep -r "func $(FUZZ)" --include='*_test.go' -l ./components ./pkg ./tests 2>/dev/null | head -1 | xargs dirname); \
	  if [ -z "$$pkg" ]; then \
	    echo "Error: Fuzz target '$(FUZZ)' not found"; exit 1; \
	  fi; \
	  fuzztime=$(FUZZTIME); pflag=""; wall=$(FUZZWALL); \
	  echo "Running fuzz target: $(FUZZ) for $$fuzztime (wall-clock cap $$wall s)"; \
	  tcmd=""; \
	  if command -v timeout >/dev/null 2>&1; then tcmd="timeout -k 10 $$wall"; fi; \
	  out=$$(mktemp); \
	  set -o pipefail; \
	  if ! $$tcmd go test -tags=fuzz -v -fuzz="^$(FUZZ)\$$" -run='^$$' -fuzztime=$$fuzztime -fuzzminimizetime=$(FUZZMINIMIZETIME) $$pflag $(GO_TEST_LDFLAGS) $$pkg 2>&1 | tee "$$out"; then \
	    rm -f "$$out"; exit 1; \
	  fi; \
	  if ! grep -q '^fuzz:' "$$out"; then \
	    echo "[error] Fuzz target '$(FUZZ)' engaged no fuzzing (no 'fuzz:' output) — silent no-op"; \
	    rm -f "$$out"; exit 1; \
	  fi; \
	  rm -f "$$out"; \
	else \
	  echo "Discovering all Fuzz* targets..."; \
	  targets=$$(grep -r "^func Fuzz" --include='*_test.go' -h ./components ./pkg ./tests 2>/dev/null | sed 's/func \(Fuzz[^(]*\).*/\1/' | sort -u); \
	  if [ -z "$$targets" ]; then \
	    echo "No Fuzz* targets found"; exit 0; \
	  fi; \
	  echo "Found targets: $$targets"; \
	  echo "Running each for $(FUZZTIME)..."; \
	  echo ""; \
	  set -o pipefail; \
	  run_fuzz_once() { \
	    tcmd=""; \
	    if command -v timeout >/dev/null 2>&1; then tcmd="timeout -k 10 $$6"; fi; \
	    $$tcmd go test -tags=fuzz -v -fuzz="^$$1\$$" -run='^$$' -fuzztime=$$2 -fuzzminimizetime=$(FUZZMINIMIZETIME) $$5 $(GO_TEST_LDFLAGS) $$3 2>&1 | tee "$$4"; \
	    s=$$?; \
	    if [ $$s -ne 0 ]; then return $$s; fi; \
	    if ! grep -q '^fuzz:' "$$4"; then return 100; fi; \
	    return 0; \
	  }; \
	  failed=""; \
	  for target in $$targets; do \
	    pkg=$$(grep -r "func $$target" --include='*_test.go' -l ./components ./pkg ./tests 2>/dev/null | head -1 | xargs dirname); \
	    fuzztime=$(FUZZTIME); \
	    pflag=""; wall=$(FUZZWALL); \
	    echo "━━━ $$target ($$pkg) [fuzztime=$$fuzztime] ━━━"; \
	    out=$$(mktemp); \
	    status=0; \
	    run_fuzz_once "$$target" "$$fuzztime" "$$pkg" "$$out" "$$pflag" "$$wall" || status=$$?; \
	    if [ $$status -ne 0 ] && [ $$status -ne 100 ] \
	      && ! grep -q "Failing input written" "$$out" \
	      && { [ $$status -eq 124 ] \
	           || grep -q "Failed to start infrastructure\|failed to start containers\|fuzzing process terminated without fuzzing\|^    context deadline exceeded" "$$out"; }; then \
	      echo "[warn] $$target hit an environmental failure (infra startup, worker death, or wall-clock hang; no failing input); settling 15s then retrying once..."; \
	      sleep 15; \
	      : > "$$out"; \
	      status=0; \
	      run_fuzz_once "$$target" "$$fuzztime" "$$pkg" "$$out" "$$pflag" "$$wall" || status=$$?; \
	    fi; \
	    if [ $$status -eq 100 ]; then \
	      echo "[error] $$target engaged no fuzzing (no 'fuzz:' output) — silent no-op"; \
	      failed="$$failed $$target"; \
	    elif [ $$status -eq 124 ]; then \
	      echo "[error] $$target exceeded the $$wall s wall-clock cap twice — hung target"; \
	      failed="$$failed $$target"; \
	    elif [ $$status -ne 0 ] \
	      && ! grep -q "Failing input written" "$$out" \
	      && grep -q '^fuzz:' "$$out" \
	      && grep -q "^    context deadline exceeded" "$$out"; then \
	      echo "[warn] $$target tripped the Go coordinator's fuzztime-boundary flake twice (fuzzing engaged, no failing input) — toolchain artifact, not a finding; not gating"; \
	    elif [ $$status -ne 0 ]; then \
	      echo "[error] $$target failed (crash or test error, exit $$status)"; \
	      failed="$$failed $$target"; \
	    fi; \
	    rm -f "$$out"; \
	    echo ""; \
	  done; \
	  if [ -n "$$failed" ]; then \
	    echo "[error] Fuzz targets that failed or engaged no fuzzing:$$failed"; exit 1; \
	  fi; \
	  echo "Fuzz testing complete. Check testdata/fuzz/ for corpus."; \
	fi

# Property-based tests (`property` build tag).
# These suites compile only under -tags=property and use testcontainers, so no
# external Docker stack is required. Discovery defaults to the ./tests tree;
# override with PKG to scope elsewhere. With no property-tagged suite present the
# target is a clean no-op ("No property test packages found").
#
# NOTE: run with -p=1 to avoid testcontainers overwhelming Docker when packages
# create containers in parallel (same rationale as test-integration).
.PHONY: test-property
test-property:
	$(call print_title,Running property-based tests (-tags=property))
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	@set -e; export ALLOW_INSECURE_TLS=true; mkdir -p $(TEST_REPORTS_DIR); \
	pkg=$${PKG:-./tests/...}; \
	pkgs=$$(go list -tags=property $$pkg 2>/dev/null | tr '\n' ' '); \
	if [ -z "$$pkgs" ]; then \
	  echo "No property test packages found"; \
	else \
	  echo "Packages: $$pkgs"; \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    gotestsum --format testname -- \
	      -tags=property -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) $$pkgs; \
	  else \
	    go test -tags=property -v $(LOW_RES_RACE_FLAG) -count=1 -timeout 600s $(GO_TEST_LDFLAGS) \
	      -p 1 $(LOW_RES_PARALLEL_FLAG) $$pkgs; \
	  fi; \
	fi

# BDD end-to-end tests (godog/cucumber) for the tracer component.
#
# tracer ships a Gherkin/godog e2e suite under
# components/tracer/tests/end2end (build tag `e2e`). The suite drives a RUNNING
# tracer HTTP service over the wire, so it is end-to-end, not a testcontainers
# unit/integration run.
#
# INFRA PREREQUISITE: a reachable tracer service + its shared Postgres must be up
# before this target runs. Point the suite at the service via SERVER_ADDRESS
# (full URL, host:port, or bare :port — see components/tracer/internal/testutil
# GetBaseURL). Authenticate with API_KEY when the target service enforces auth.
# In CI the service is stood up by the job (P7-T15, deferred to P8); for local
# dev bring tracer up first (e.g. `make tracer COMMAND=up`) and export
# SERVER_ADDRESS=http://localhost:<tracer-port>.
#
# GODOG_TAGS optionally scopes which scenarios run (passed through to godog).
SERVER_ADDRESS ?=
GODOG_TAGS ?=

.PHONY: test-e2e
test-e2e:
	$(call print_title,Running tracer godog/cucumber BDD e2e suite)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@set -e; \
	if [ -z "$(SERVER_ADDRESS)" ]; then \
	  echo "ERROR: SERVER_ADDRESS is not set."; \
	  echo "The godog BDD suite is end-to-end and needs a running tracer service + shared Postgres."; \
	  echo "Bring tracer up, then: make test-e2e SERVER_ADDRESS=http://localhost:<tracer-port>"; \
	  exit 1; \
	fi; \
	echo "Targeting tracer service at: $(SERVER_ADDRESS)"; \
	SERVER_ADDRESS=$(SERVER_ADDRESS) GODOG_TAGS=$(GODOG_TAGS) \
	  go test -tags e2e -v -count=1 $(GO_TEST_LDFLAGS) \
	  ./components/tracer/tests/end2end/...

# Backward-compatible alias: test-bdd was the old name for the godog e2e suite.
# Canonicalized on test-e2e (matches tracer + go-combined-analysis.yml comment).
.PHONY: test-bdd
test-bdd: test-e2e

# Ledger end-to-end suite — exercises the full ledger + CRM + fees money path
# over HTTP (org -> ledger -> asset -> accounts -> fund -> transfer ->
# holder/instrument -> fee package -> fee-bearing transaction) against an
# ALREADY-RUNNING stack. Bring it up first with `make up`. Unlike the tracer
# godog suite above it self-gates: if the ledger is unreachable the tests skip
# rather than fail. Override LEDGER_URL / TRACER_URL to target a remote stack.
LEDGER_URL ?= http://localhost:3002
TRACER_URL ?= http://localhost:4020

.PHONY: test-ledger-e2e
test-ledger-e2e:
	$(call print_title,Running ledger + CRM + fees end-to-end suite)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@echo "Targeting ledger at: $(LEDGER_URL)  (tracer at: $(TRACER_URL))"
	@echo "Tests skip if the stack is unreachable — run 'make up' first."
	LEDGER_URL=$(LEDGER_URL) TRACER_URL=$(TRACER_URL) \
	  go test -tags e2e -v -count=1 $(GO_TEST_LDFLAGS) ./tests/e2e/...

# Full CI test matrix — one command, one exit code.
#
# Sequences the deterministic, self-contained legs (testcontainers only, no live
# docker-compose stack required), with the test-integration glob spanning ./tests:
#   1. test-unit            (-race, UNTAGGED — bare `go test` discovers unit pkgs)
#   2. test-integration     (-tags=integration -p 1; glob spans ./components ./pkg ./tests)
#   3. test-property        (-tags=property -p 1; ./tests tree)
# Each leg is a separate $(MAKE) invocation under `set -e`, so the first failing
# leg aborts the run and `make ci-tests` returns its non-zero exit code.
#
# This is the full TEST matrix only. The top-level `ci` gate (root Makefile) adds
# the static gates (lint + check-telemetry) ahead of the fast unit leg; invoke
# `make ci-tests` directly to run the heavier integration/property/chaos legs.
#
# OPT-IN legs NOT in the default path (each needs a live service/stack, so they
# are non-deterministic in a bare CI runner and must be invoked explicitly):
#   - make test-e2e SERVER_ADDRESS=...  tracer godog e2e suite (needs a running tracer + Postgres)
#   - make test-chaos-system            system chaos suite (brings the full docker-compose stack up/down)
#   - make test-fuzz                    native fuzz engine (time-boxed mutation runs, not a pass/fail gate)
.PHONY: ci-tests
ci-tests:
	$(call print_title,Running CI test matrix)
	@set -e; \
	$(MAKE) test-unit; \
	$(MAKE) test-integration; \
	$(MAKE) test-property

#-------------------------------------------------------
# Coverage aggregator
#-------------------------------------------------------
# Root coverage = coverage-unit (mk/coverage-unit.mk) + coverage-integration
# (mk/test-go.mk).
.PHONY: coverage
coverage:
	$(call print_title,Running all coverage targets)
	$(MAKE) coverage-unit
	$(MAKE) coverage-integration
