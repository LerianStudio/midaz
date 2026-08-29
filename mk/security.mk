# ------------------------------------------------------
# Security scanning and vulnerability detection commands
# ------------------------------------------------------
# This file contains all security-related operations including
# static analysis (gosec) and vulnerability scanning (govulncheck).
#
# Requirements:
#   - gosec CLI (auto-installed at the pinned version if missing)
#   - govulncheck CLI (auto-installed at the pinned version if missing)
#
# Variables used from the including Makefile:
#   - SEC_SCAN_PATHS: Go package paths to scan (default: ./...).
#                     The root Makefile narrows this to `./components/... ./pkg/...`;
#                     component Makefiles use the default.
#   - SARIF: Enable SARIF output format (0|1, default: 0).
#            SARIF format is compatible with the GitHub Security tab.
#
# Usage:
#   make sec                        # Run all security checks
#   make sec SARIF=1                # Run with SARIF output for CI/CD
#   make sec-gosec                  # Run only gosec
#   make sec-govulncheck            # Run only govulncheck
# ------------------------------------------------------

# Pinned tool versions for reproducible security scans
GOSEC_VERSION ?= v2.29.0
GOVULNCHECK_VERSION ?= v1.1.4

# A tool on PATH counts as current only when `go version -m` shows it was
# built from the pinned module version by the active Go toolchain. A stale
# build from an older Go refuses to load the module ("package requires newer
# Go version"), and `gosec --version` cannot be used because `go install`
# builds print "Version: dev".
# $(1) binary, $(2) module path, $(3) pinned version
define sec_tool_current
{ command -v $(1) >/dev/null 2>&1 && go version -m "$$(command -v $(1))" | awk -v mod='$(2)' -v ver='$(3)' -v gov="$$(go env GOVERSION)" 'NR==1 && $$NF==gov {g=1} $$1=="mod" && $$2==mod && $$3==ver {m=1} END {exit !(g && m)}'; }
endef

# Package paths to scan. Components scan their whole module (./...);
# the root narrows to the monorepo source trees.
SEC_SCAN_PATHS ?= ./...

# SARIF output for GitHub Security tab integration (optional)
SARIF ?= 0

#-------------------------------------------------------
# Commands (alphabetically ordered)
#-------------------------------------------------------

# Run all security checks (aggregator command)
.PHONY: sec
sec:
	$(call title1,"Running security checks")
	@$(MAKE) sec-gosec SARIF=$(SARIF)
	@$(MAKE) sec-govulncheck
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Security checks completed$(GREEN) ✔️$(NC)"

# Run gosec static security analysis
# Optional: Set SARIF=1 to generate SARIF format output (gosec-report.sarif)
.PHONY: sec-gosec
sec-gosec:
	@export PATH="$$(go env GOPATH)/bin:$$PATH"; \
	if ! $(call sec_tool_current,gosec,github.com/securego/gosec/v2,$(GOSEC_VERSION)); then \
		echo "$(YELLOW)Installing gosec $(GOSEC_VERSION)...$(NC)"; \
		go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION); \
	fi; \
	if find . -name "*.go" -type f | grep -q .; then \
		echo "$(CYAN)Running gosec on $(SEC_SCAN_PATHS)...$(NC)"; \
		if [ "$(SARIF)" = "1" ]; then \
			echo "$(YELLOW)Generating SARIF output: gosec-report.sarif$(NC)"; \
			gosec -exclude-generated -fmt sarif -out gosec-report.sarif $(SEC_SCAN_PATHS); \
			echo "$(GREEN)$(BOLD)[ok]$(NC) SARIF report generated: gosec-report.sarif$(GREEN) ✔️$(NC)"; \
		else \
			gosec -exclude-generated $(SEC_SCAN_PATHS); \
		fi; \
	else \
		echo "$(YELLOW)No Go files found, skipping gosec$(NC)"; \
	fi

# Run govulncheck vulnerability scanner
# Checks against the Go vulnerability database for known CVEs
.PHONY: sec-govulncheck
sec-govulncheck:
	@export PATH="$$(go env GOPATH)/bin:$$PATH"; \
	if ! $(call sec_tool_current,govulncheck,golang.org/x/vuln,$(GOVULNCHECK_VERSION)); then \
		echo "$(YELLOW)Installing govulncheck $(GOVULNCHECK_VERSION)...$(NC)"; \
		go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION); \
	fi; \
	if find . -name "*.go" -type f | grep -q .; then \
		echo "$(CYAN)Running govulncheck on $(SEC_SCAN_PATHS)...$(NC)"; \
		govulncheck $(SEC_SCAN_PATHS); \
	else \
		echo "$(YELLOW)No Go files found, skipping govulncheck$(NC)"; \
	fi
