# Reporter Changelog

## [1.3.3](https://github.com/LerianStudio/reporter/releases/tag/v1.3.3)

- Fixes:
  - Removed app-level rate limiter.
  - Added `ALLOW_INSECURE_TLS` opt-out.

Contributors: @jeffersonrodrigues92, @lerian-studio.

[Compare changes](https://github.com/LerianStudio/reporter/compare/v1.3.2...v1.3.3)

---

## [1.3.2](https://github.com/LerianStudio/reporter/releases/tag/v1.3.2)

- Fixes:
  - Bump `lib-commons` to `v5.1.3` for improved `http/1.1` `tmclient` transport.

Contributors: @jeffersonrodrigues92, @lerian-studio.

[Compare changes](https://github.com/LerianStudio/reporter/compare/v1.3.1...v1.3.2)

---

## [1.3.1](https://github.com/LerianStudio/reporter/releases/tag/v1.3.1)

- Fixes:
  - Subscribe to env-scoped tenant-events channel.

Contributors: @jeffersonrodrigues92, @lerian-studio.

[Compare changes](https://github.com/LerianStudio/reporter/compare/v1.3.0...v1.3.1)

---

## [1.3.0](https://github.com/LerianStudio/reporter/releases/tag/v1.3.0)

- **Features**
  - Add static app token provider for single-tenant fetcher.
  - Rewrite health server with canonical `/readyz` contract.
  - Mount `/readyz` endpoint with self-probe and drain.
  - Canonical `/readyz` package and operations guide.
  - Add DSN credential redaction utility.

- **Fixes**
  - Realign annotations with runtime.
  - Require non-empty `PLUGIN_AUTH_ADDRESS` before wiring static provider.
  - Align dedup match with widened partial index filter.
  - Preserve datasource map when parsing if-block.
  - Preserve multi-dot tokens in numeric output cleaning.

- **Improvements**
  - Rename fetcher M2M envs to `FETCHER_M2M_CLIENT_{ID,SECRET}`.
  - Align regression test comment with current fix.
  - Broaden release paths-ignore to skip all `.github` changes.
  - Address review feedback on `/readyz` implementation.
  - Migrate legacy `/ready` paths to `/readyz` across test suite.

Contributors: @arthurkz, @bedatty, @brunobls, @gandalf-at-lerian, @lerian-studio.

[Compare changes](https://github.com/LerianStudio/reporter/compare/v1.2.0...v1.3.0)

---

## [1.2.0](https://github.com/LerianStudio/reporter/releases/tag/v1.2.0)

- Features:
  - Added support for multi-tenant isolation in the reporter component, including enhancements for MongoDB and RabbitMQ.
  - Introduced new endpoints for template builder configuration, including block and filter settings.
  - Implemented FetcherStorageAdapter and shared infrastructure for improved data handling.
  - Added comprehensive end-to-end test suite to ensure functionality and reliability.
  - Enhanced deadline management with new validation and query parameters.

- Fixes:
  - Resolved silent logging issues in multi-tenant notification handler.
  - Addressed various security vulnerabilities, including SSTI and LFI exploit chains.
  - Improved error handling and validation across multiple components.
  - Fixed issues with Docker configurations that affected AWS credential chains.
  - Corrected deadline deduplication errors and improved error mappings.

- Improvements:
  - Upgraded lib-commons and lib-auth dependencies to enhance functionality and security.
  - Increased test coverage significantly for worker components.
  - Enhanced logging and observability for better diagnosis and monitoring.
  - Refactored code to reduce complexity and improve maintainability.
  - Streamlined integration tests and assertions for better reliability.

Contributors: @arthur.ribeiro, @arthurkz, @bedatty, @bruno.souza, @brunoblsouza, @dependabot[bot], @fred, @gandalf, @jefferson.comff, @lucas.bedatty

[Compare changes](https://github.com/LerianStudio/reporter/compare/v1.1.1...v1.2.0)

---

## [1.1.0](https://github.com/LerianStudio/reporter/releases/tag/v1.1.0)

- **Features:**
  - Added Redis SetNX idempotency check for report creation.
  - Introduced readiness endpoint, handler constructors, config validation, and domain model constructors.

- **Fixes:**
  - Resolved critical bugs in report workflow, XSS validation, and Redis panic.
  - Improved error handling, nil guards, and log redaction.
  - Corrected WaitGroup Done placement in goroutine cleanup handlers.
  - Removed the required validation for env object storage endpoint.
  - Fixed health check on container and PDF report generation.

- **Improvements:**
  - Improved observability, type safety, and production hardening.
  - Enhanced test quality with build tags, env isolation, and chaos guards.
  - Improved code quality, observability, and split generate-report.go.
  - Standardized Docker Compose, Makefiles, response wrappers, and config.
  - Centralized os.Getenv calls and added thread-safe datasource map.

Contributors: @arthur.ribeiro, @arthurkz, @bruno.souza, @brunoblsouza, @dependabot[bot], @ferr3ira.gabriel, @jefferson.comff

[Compare changes](https://github.com/LerianStudio/reporter/compare/v1.0.0...v1.1.0)

