## End-to-End API Tests

E2E tests exercise realistic user journeys against the local stack via HTTP, covering the main lifecycle from onboarding to posting transactions and validating balances.

### What’s covered
- **Onboarding flow**: create organization → ledger → portfolio/segment → accounts.
- **Transactions**: post inflow/outflow transactions and verify idempotency.
- **Balances**: read and assert account balances after operations.
- **Routing and resources**: basic CRUD and retrievals for the core entities.

These flows are defined in `tests/e2e/local.apidog-cli.json` and executed with Apidog CLI.

### How to run
```bash
make test-e2e
```

This command will:
- Start the backend stack (infra + services) with Docker Compose and wait for health.
- Run the Apidog scenario `tests/e2e/local.apidog-cli.json` via `npx`.
- Generate an HTML report in `reports/e2e/`.

Open the latest HTML file in `reports/e2e` to review detailed results.

### Prerequisites
- Docker installed and running.
- Local env files present for components (`make set-env` can bootstrap from `.env.example`).
- If authentication is enabled in your environment, provide a valid bearer token via your Apidog environment/variables (the collection uses `{{access_token}}`). No secrets are committed to the repo.

### Notes
- The Makefile uses `npx @apidog/cli` (fallback to `apidog-cli`) so you don’t need a global install.
- Reports are written directly to `reports/e2e` as HTML.

### Troubleshooting
- **Services not healthy**: run `make logs` to inspect container logs, then `make restart-backend`.
- **CLI/network issues**: ensure internet access for `npx` and that your npm registry can resolve `@apidog/cli`.
