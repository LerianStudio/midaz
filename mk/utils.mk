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
