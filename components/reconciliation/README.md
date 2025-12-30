# Reconciliation Worker

Continuous database consistency monitoring for Midaz.

## Quick Start

```bash
# Copy environment config
cp .env.example .env

# Start (requires infra running)
make up

# Check status
make status

# View full report
make report
```

## Checks Performed

| Check | Purpose | Severity |
|-------|---------|----------|
| Balance Consistency | balance = Σcredits - Σdebits | WARNING/CRITICAL |
| Double-Entry | credits = debits per transaction | CRITICAL |
| Referential Integrity | No orphan entities | WARNING/CRITICAL |
| Sync (Redis-PG) | Version consistency | WARNING |
| Orphan Transactions | Transactions with operations | CRITICAL |

## API Endpoints

- `GET /health` - Liveness probe
- `GET /reconciliation/status` - Quick status
- `GET /reconciliation/report` - Full report
- `POST /reconciliation/run` - Manual trigger

## Configuration

See `.env.example` for all options.

Key settings:
- `RECONCILIATION_INTERVAL_SECONDS` - Check interval (default: 300)
- `SETTLEMENT_WAIT_SECONDS` - Wait for async settlement (default: 300)
- `DISCREPANCY_THRESHOLD` - Min discrepancy to report (default: 0)
