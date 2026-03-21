# mk/coverage-unit.mk — Shared unit test coverage target
#
# Required variable (set before include):
#   COVERAGE_PACKAGES  Space-separated Go package patterns relative to MIDAZ_ROOT
#                      Example: ./components/crm/... ./pkg/...
#
# Optional overrides:
#   PKG                Override package list for a single invocation
#   TEST_REPORTS_DIR   Output directory (default: ./reports)
#   RETRY_ON_FAIL      Retry once on failure (default: 0)
#
# Output:
#   $(TEST_REPORTS_DIR)/unit_coverage.out
#
# Usage in component Makefile:
#   COVERAGE_PACKAGES := ./components/crm/... ./pkg/...
#   include $(MIDAZ_ROOT)/mk/coverage-unit.mk
#
# CI compatibility:
#   Target name (coverage-unit) and output path (reports/unit_coverage.out)
#   match go-pr-analysis.yml workflow expectations.

ifndef COVERAGE_PACKAGES
$(error COVERAGE_PACKAGES must be set before including coverage-unit.mk)
endif

MIDAZ_ROOT       ?= $(shell pwd)
TEST_REPORTS_DIR ?= ./reports
RETRY_ON_FAIL    ?= 0
GOTESTSUM        := $(shell command -v gotestsum 2>/dev/null)

# Resolve paths at parse time (before any shell cd in recipes)
_COVERAGE_OUT := $(abspath $(TEST_REPORTS_DIR))/unit_coverage.out
_CALLER_DIR   := $(CURDIR)

# macOS ld64 workaround: suppress LC_DYSYMTAB warnings when using -race
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
  ifneq ($(DISABLE_OSX_LINKER_WORKAROUND),1)
    GO_TEST_LDFLAGS := -ldflags="-linkmode=external -extldflags=-ld_classic"
  else
    GO_TEST_LDFLAGS :=
  endif
else
  GO_TEST_LDFLAGS :=
endif

.PHONY: coverage-unit
coverage-unit:
	@echo ""
	@echo "------------------------------------------"
	@echo "   Running unit tests with coverage"
	@echo "------------------------------------------"
	@set -e; mkdir -p $(abspath $(TEST_REPORTS_DIR)); \
	cd $(MIDAZ_ROOT); \
	if [ -n "$(PKG)" ]; then \
	  echo "Using specified package: $(PKG)"; \
	  pkgs=$$(go list $(PKG) 2>/dev/null | awk '!/\/tests($|\/)/' | awk '!/\/api($|\/)/' | tr '\n' ' '); \
	else \
	  pkgs=$$(go list $(COVERAGE_PACKAGES) | awk '!/\/tests($|\/)/' | awk '!/\/api($|\/)/' | tr '\n' ' '); \
	fi; \
	if [ -z "$$pkgs" ]; then \
	  echo "No unit test packages found"; \
	else \
	  echo "Packages: $$pkgs"; \
	  if [ -n "$(GOTESTSUM)" ]; then \
	    echo "Running unit tests with gotestsum (coverage enabled)"; \
	    gotestsum --format testname -- -v -race -count=1 $(GO_TEST_LDFLAGS) -covermode=atomic -coverprofile=$(_COVERAGE_OUT) $$pkgs || { \
	      if [ "$(RETRY_ON_FAIL)" = "1" ]; then \
	        echo "Retrying unit tests once..."; \
	        gotestsum --format testname -- -v -race -count=1 $(GO_TEST_LDFLAGS) -covermode=atomic -coverprofile=$(_COVERAGE_OUT) $$pkgs; \
	      else \
	        exit 1; \
	      fi; \
	    }; \
	  else \
	    go test -v -race -count=1 $(GO_TEST_LDFLAGS) -covermode=atomic -coverprofile=$(_COVERAGE_OUT) $$pkgs; \
	  fi; \
	  IGNORE_FILE=""; \
	  if [ -f "$(_CALLER_DIR)/.ignorecoverunit" ]; then \
	    IGNORE_FILE="$(_CALLER_DIR)/.ignorecoverunit"; \
	  elif [ -f "$(MIDAZ_ROOT)/.ignorecoverunit" ]; then \
	    IGNORE_FILE="$(MIDAZ_ROOT)/.ignorecoverunit"; \
	  fi; \
	  if [ -n "$$IGNORE_FILE" ]; then \
	    echo "Filtering coverage with $$IGNORE_FILE patterns..."; \
	    patterns=$$(grep -v '^#' "$$IGNORE_FILE" | grep -v '^$$' | tr '\n' '|' | sed 's/|$$//'); \
	    if [ -n "$$patterns" ]; then \
	      regex_patterns=$$(echo "$$patterns" | sed 's/\./\\./g' | sed 's/\*/.*/g'); \
	      head -1 $(_COVERAGE_OUT) > $(_COVERAGE_OUT).tmp; \
	      tail -n +2 $(_COVERAGE_OUT) | grep -vE "$$regex_patterns" >> $(_COVERAGE_OUT).tmp || true; \
	      mv $(_COVERAGE_OUT).tmp $(_COVERAGE_OUT); \
	      echo "Excluded patterns: $$patterns"; \
	    fi; \
	  fi; \
	  echo "----------------------------------------"; \
	  go tool cover -func=$(_COVERAGE_OUT) | grep total | awk '{print "Total coverage: " $$3}'; \
	  echo "----------------------------------------"; \
	fi
