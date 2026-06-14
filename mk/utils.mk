# Shell utility functions for Makefiles
# Standardized shell helpers included by the root Makefile and every
# component Makefile. Provides the shared title banners and the
# docker-compose command detection used across all fan-out targets.

# Docker version detection — picks `docker compose` (v2) when new enough,
# falls back to the legacy `docker-compose` binary otherwise.
DOCKER_VERSION := $(shell docker version --format '{{.Server.Version}}' 2>/dev/null || echo "0.0.0")
DOCKER_MIN_VERSION := 20.10.13

DOCKER_CMD := $(shell \
	if [ "$(shell printf '%s\n' "$(DOCKER_MIN_VERSION)" "$(DOCKER_VERSION)" | sort -V | head -n1)" = "$(DOCKER_MIN_VERSION)" ]; then \
		echo "docker compose"; \
	else \
		echo "docker-compose"; \
	fi \
)

# Border function for creating section headers
define border
	@echo ""; \
	len=$$(echo "$(1)" | wc -c); \
	for i in $$(seq 1 $$((len + 4))); do \
		printf "-"; \
	done; \
	echo ""; \
	echo "  $(1)  "; \
	for i in $$(seq 1 $$((len + 4))); do \
		printf "-"; \
	done; \
	echo ""
endef

# Title function with emoji
define title1
	@$(call border, "📝 $(1)")
endef

define title2
	@$(call border, "🔍 $(1)")
endef

# Check if a command is available, with an install hint on failure.
# Usage: $(call check_command,go,"Install Go from https://golang.org/doc/install")
# Lives here (not the root Makefile) so any includer — root AND every component —
# inherits it; mk/tests.mk relies on it.
define check_command
	@if ! command -v $(1) >/dev/null 2>&1; then \
		echo "Error: $(1) is not installed"; \
		echo "To install: $(2)"; \
		exit 1; \
	fi
endef

# Warn about any missing .env files across all components + infra.
# Iterates $(INFRA_DIR) $(GO_COMPONENTS), which the root Makefile defines; in a
# component context those are empty and the loop is a harmless no-op (no component
# target calls this — only root service-lifecycle targets do).
define check_env_files
	@for dir in $(INFRA_DIR) $(GO_COMPONENTS); do \
		if [ -f "$$dir/.env.example" ] && [ ! -f "$$dir/.env" ]; then \
			echo "Warning: $$dir/.env file is missing. Consider running 'make set-env'."; \
		fi; \
	done
endef
