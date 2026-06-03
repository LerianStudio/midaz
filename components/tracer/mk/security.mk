# ------------------------------------------------------
# Security scanning and vulnerability detection commands
# ------------------------------------------------------
# This file contains all security-related operations including
# static analysis (gosec) and vulnerability scanning (govulncheck).
#
# Requirements:
#   - gosec CLI (auto-installed if missing)
#   - govulncheck CLI (auto-installed if missing)
#   - Go source files in ./internal, ./pkg, or ./cmd
#
# Variables:
#   - SARIF: Enable SARIF output format (0|1, default: 0)
#            SARIF format is compatible with GitHub Security tab
#
# Usage:
#   make sec                        # Run all security checks
#   make sec SARIF=1                # Run with SARIF output for CI/CD
#   make sec-gosec                  # Run only gosec
#   make sec-govulncheck            # Run only govulncheck
#
# Tools:
#   - gosec: Go Security Checker - static analysis for security issues
#   - govulncheck: Go Vulnerability Checker - detects known CVEs
# ------------------------------------------------------

# Pinned tool versions for reproducible security scans
GOSEC_VERSION ?= v2.22.11
GOVULNCHECK_VERSION ?= v1.1.4

# SARIF output for GitHub Security tab integration (optional)
# Usage: make sec SARIF=1
SARIF ?= 0

#-------------------------------------------------------
# Commands (alphabetically ordered)
#-------------------------------------------------------

# Run all security checks (aggregator command)
# Executes both gosec and govulncheck sequentially
.PHONY: sec
sec:
	$(call title1,"Running security checks")
	@$(MAKE) sec-gosec SARIF=$(SARIF)
	@$(MAKE) sec-govulncheck
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Security checks completed$(GREEN) ✔️$(NC)"

# Run gosec static security analysis
# Scans internal/, pkg/, and cmd/ directories for security issues
# Optional: Set SARIF=1 to generate SARIF format output (gosec-report.sarif)
.PHONY: sec-gosec
sec-gosec:
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing gosec...$(NC)"; \
		go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION); \
	fi
	@if find ./internal ./pkg ./cmd -name "*.go" -type f 2>/dev/null | grep -q .; then \
		echo "$(CYAN)Running gosec on internal/, pkg/, and cmd/ folders...$(NC)"; \
		if [ "$(SARIF)" = "1" ]; then \
			echo "$(YELLOW)Generating SARIF output: gosec-report.sarif$(NC)"; \
			gosec -fmt sarif -out gosec-report.sarif ./internal/... ./pkg/... ./cmd/...; \
			echo "$(GREEN)$(BOLD)[ok]$(NC) SARIF report generated: gosec-report.sarif$(GREEN) ✔️$(NC)"; \
		else \
			gosec ./internal/... ./pkg/... ./cmd/...; \
		fi; \
	else \
		echo "$(YELLOW)No Go files found, skipping gosec$(NC)"; \
	fi

# Run govulncheck vulnerability scanner
# Checks internal/, pkg/, and cmd/ directories against Go vulnerability database
# Detects known CVEs in dependencies and standard library
.PHONY: sec-govulncheck
sec-govulncheck:
	@if ! command -v govulncheck >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing govulncheck...$(NC)"; \
		go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION); \
	fi
	@if find ./internal ./pkg ./cmd -name "*.go" -type f 2>/dev/null | grep -q .; then \
		echo "$(CYAN)Running govulncheck on internal/, pkg/, and cmd/ folders...$(NC)"; \
		govulncheck ./internal/... ./pkg/... ./cmd/...; \
	else \
		echo "$(YELLOW)No Go files found, skipping govulncheck$(NC)"; \
	fi
