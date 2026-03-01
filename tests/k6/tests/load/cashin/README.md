# PIX Cash-In Load Tests

Load testing suite for PIX Cash-In (receiving payments) flows via BTG Sync webhooks.

## Overview

This test suite validates the complete Cash-In PIX flow:

```
Phase 1: CASHIN APPROVAL (Webhook status=INITIATED)
┌─────────────┐     ┌──────────────────────────────┐     ┌───────────┐
│  BTG Sync   │ ──► │ POST /v1/payment/webhooks/   │ ──► │ CRM/Midaz │
│ (INITIATED) │     │      btg/events              │     │ Validation│
└─────────────┘     └──────────────────────────────┘     └───────────┘
                                   │
                                   ▼
                    Response: {approvalId, status: ACCEPTED|DENIED}

Phase 2: CASHIN SETTLEMENT (Webhook status=CONFIRMED)
┌─────────────┐     ┌──────────────────────────────┐     ┌───────────┐
│  BTG Sync   │ ──► │ POST /v1/transfers/webhooks  │ ──► │  Midaz    │
│ (CONFIRMED) │     │                              │     │ Settlement│
└─────────────┘     └──────────────────────────────┘     └───────────┘
```

## Test Types

| Type | Duration | VUs | Purpose |
|------|----------|-----|---------|
| **smoke** | 30s | 1 | Quick validation before larger tests |
| **load** | 10min | 10-50 | Normal production load simulation |
| **stress** | 15min | 50-200 | Find system breaking point |
| **spike** | 5min | 10-100 | Test sudden traffic bursts |
| **soak** | 4h+ | 20-200 | Memory leaks, resource exhaustion |

## Quick Start

```bash
# Smoke test (quick validation)
k6 run tests/load/cashin/run.js -e TEST_TYPE=smoke

# Load test
k6 run tests/load/cashin/run.js -e TEST_TYPE=load

# Stress test
k6 run tests/load/cashin/run.js -e TEST_TYPE=stress

# Spike test
k6 run tests/load/cashin/run.js -e TEST_TYPE=spike
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TEST_TYPE` | Test type: smoke, load, stress, spike, soak | `smoke` |
| `ENVIRONMENT` | Target environment: dev, staging, vpc | `dev` |
| `PLUGIN_PIX_URL` | Plugin BR PIX base URL | `http://localhost:8080` |
| `K6_THINK_TIME_MODE` | Think time: fast, realistic, stress | `realistic` |
| `CASHIN_FEE_ENABLED` | Enable fee calculation tests | `false` |

## Usage Examples

```bash
# Run against staging environment
k6 run tests/load/cashin/run.js \
  -e TEST_TYPE=load \
  -e ENVIRONMENT=staging \
  -e PLUGIN_PIX_URL=http://plugin-staging:8080

# Run stress test with fast think times
k6 run tests/load/cashin/run.js \
  -e TEST_TYPE=stress \
  -e K6_THINK_TIME_MODE=fast

# Run standalone smoke test
k6 run tests/load/cashin/scenarios/smoke.js

# Run with HTML report
k6 run tests/load/cashin/run.js -e TEST_TYPE=load --out json=results.json
```

## Metrics Reference

### Latency Metrics

| Metric | Description | Threshold |
|--------|-------------|-----------|
| `cashin_approval_duration` | Phase 1 latency | p95 < 500ms |
| `cashin_settlement_duration` | Phase 2 latency | p95 < 1000ms |
| `cashin_e2e_duration` | Full flow latency | p95 < 2000ms |

### Success Rate Metrics

| Metric | Description | Threshold |
|--------|-------------|-----------|
| `cashin_approval_success_rate` | Approval success % | > 95% |
| `cashin_settlement_success_rate` | Settlement success % | > 95% |

### Error Metrics

| Metric | Description |
|--------|-------------|
| `cashin_business_errors` | Expected errors (validation, denied) |
| `cashin_technical_errors` | System failures (5xx, timeouts) |
| `cashin_duplicate_rejections` | Correct duplicate rejections |
| `cashin_crm_failures` | CRM validation failures |
| `cashin_midaz_failures` | Midaz transaction failures |
| `cashin_invalid_key_errors` | Invalid PIX key errors |
| `cashin_timeout_errors` | Timeout errors |

## Interpreting Results

### Performance Issues

| Symptom | Likely Cause | Action |
|---------|--------------|--------|
| `cashin_approval_duration` p95 > 500ms | CRM or Entry Service bottleneck | Check CRM response times |
| `cashin_settlement_duration` p95 > 1000ms | Midaz bottleneck | Check Midaz transaction times |
| `cashin_technical_error_rate` > 1% | System instability | Check logs for 5xx errors |

### Business Logic Issues

| Symptom | Likely Cause | Action |
|---------|--------------|--------|
| `cashin_duplicate_rejections` = 0 in duplicate tests | **CRITICAL BUG** | Duplicate detection failing |
| High `cashin_crm_failures` | Test data issue or CRM unavailable | Verify test accounts exist |
| Many `cashin_approval_denied` | Business rules rejecting | Check denial reasons |

## File Structure

```
tests/load/cashin/
├── config/
│   └── scenarios.js       # Thresholds, scenarios, env config
├── lib/
│   ├── generators.js      # BTG webhook payload generators
│   ├── validators.js      # Response validation functions
│   └── metrics.js         # Custom metrics definitions
├── flows/
│   └── cashin-flow.js     # Full approval → settlement flow
├── scenarios/
│   └── smoke.js           # Standalone smoke test
├── run.js                 # Main entry point (all test types)
└── README.md              # This file
```

## Test Scenarios

### Main Flow (`cashin_flow`)
- Distributed Cash-In across test accounts
- Full approval → settlement flow
- Realistic think times between phases

### Concurrent (`cashin_concurrent`)
- Multiple VUs target the same account
- Tests account-level locking/concurrency
- Validates no double-crediting

### Burst (`cashin_burst`)
- Rapid-fire Cash-In requests
- Constant arrival rate
- Tests system behavior under sudden load

### Duplicates (`cashin_duplicates`)
- Creates Cash-In, then attempts duplicate
- Validates 409 Conflict response
- **Critical**: Duplicates must be rejected

## Troubleshooting

### Test Fails Immediately

```bash
# Check if Plugin PIX is running
curl http://localhost:8080/health

# Verify webhook endpoints exist
curl -X POST http://localhost:8080/v1/payment/webhooks/btg/events \
  -H "Content-Type: application/json" \
  -d '{}'
```

### High Error Rate

1. Check `cashin_technical_errors` vs `cashin_business_errors`
2. Technical errors indicate system issues
3. Business errors might be expected (validation failures)

### Slow Response Times

1. Check which phase is slow (approval vs settlement)
2. Approval slow → CRM or Entry Service
3. Settlement slow → Midaz transaction processing

### Duplicate Test Failing

If `cashin_duplicate_rejections` = 0 during duplicate tests:

1. **This is a critical bug** - duplicates are not being rejected
2. Check if EndToEndId validation is enabled
3. Check idempotency layer configuration

## CI/CD Integration

The test outputs `summary.json` with structured results:

```json
{
  "testType": "load",
  "overall": "PASS",
  "thresholds": {
    "approvalP95": { "value": 245, "pass": true },
    "settlementP95": { "value": 512, "pass": true },
    "technicalErrorRate": { "value": 0.001, "pass": true }
  }
}
```

Use in CI pipeline:

```yaml
- name: Run Load Tests
  run: k6 run tests/load/cashin/run.js -e TEST_TYPE=load

- name: Check Results
  run: |
    RESULT=$(cat summary.json | jq -r '.overall')
    if [ "$RESULT" != "PASS" ]; then
      echo "Load test failed!"
      exit 1
    fi
```

## Related Documentation

- [PIX Specification (BACEN)](https://www.bcb.gov.br/estabilidadefinanceira/pix)
- [k6 Documentation](https://k6.io/docs/)
- [Plugin BR PIX README](../../../README.md)
