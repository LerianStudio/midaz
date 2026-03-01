# k6 Midaz Tests

This project uses Grafana k6 for performance tests on Midaz.

## Getting Started

### 1. Install K6

Use [this](https://grafana.com/docs/k6/latest/set-up/install-k6/) instructions to install K6.

### 2. Clone the repository

```bash
git clone https://github.com/LerianStudio/k6-midaz-core.git
cd k6-midaz-core
```

### 3. Run Tests

The tests are organized into folders inside the tests folder. To execute a test, navigate to the folder and run the K6 command.

**Example:**
```bash
# ramp up test folder
cd tests/tps_ramp_up

# run test
k6 run 001_setup_accounts.js
```

### 4. Configuration File

The file `config/env.json` stores the configuration information for the tests, and new environments can be added. By default, the `dev` environment is used.

---

## Environment Variables

### Global Variables

| Variable | Values | Default | Description |
|----------|--------|---------|-------------|
| `ENVIRONMENT` | `dev`, `sandbox`, `vpc`, `capybara` | `dev` | Target environment |
| `AUTH_ENABLED` | `true`, `false` | `true` | Enable/disable authentication |
| `K6_ABORT_ON_ERROR` | `true`, `false` | `false` | Abort test on first HTTP error |
| `LOG` | `DEBUG`, `ERROR`, `OFF` | `OFF` | Log level |

### Examples

```bash
# Basic test
k6 run test.js -e ENVIRONMENT=dev

# Test without authentication
k6 run test.js -e ENVIRONMENT=dev -e AUTH_ENABLED=false

# Test with abort on error (useful for debugging)
k6 run test.js -e ENVIRONMENT=dev -e K6_ABORT_ON_ERROR=true

# Test with debug logging
k6 run test.js -e ENVIRONMENT=dev -e LOG=DEBUG
```

---

## PIX Load Tests

For comprehensive PIX domain load testing (Cash-In, Cash-Out, Collection, Refund), see the dedicated documentation:

- **[PIX Load Tests Commands](docs/PIX_LOAD_TESTS_COMMANDS.md)** - Complete reference with all commands and examples
- **[PIX Tests Technical Documentation](docs/PIX_TESTS_TECHNICAL_DOCUMENTATION.md)** - Detailed technical analysis of all PIX tests, metrics, thresholds, and BACEN compliance

### Quick Start - PIX Tests

```bash
# PIX with Dynamic Setup (Recommended) - Creates real accounts with balance
k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js \
  -e ENVIRONMENT=dev \
  -e NUM_ACCOUNTS=10 \
  -e TEST_TYPE=smoke

# PIX without authentication
k6 run tests/v3.x.x/pix_indirect_btg/pix-test-with-dynamic-setup.js \
  -e ENVIRONMENT=dev \
  -e AUTH_ENABLED=false

# PIX Cash-In
k6 run tests/load/cashin/run.js \
  -e ENVIRONMENT=dev \
  -e TEST_TYPE=smoke
```

### PIX Test Variables

| Variable | Values | Default | Description |
|----------|--------|---------|-------------|
| `ENVIRONMENT` | `dev`, `sandbox`, `vpc`, `capybara` | `dev` | Target environment |
| `AUTH_ENABLED` | `true`, `false` | `true` | Enable/disable authentication |
| `K6_ABORT_ON_ERROR` | `true`, `false` | `false` | Abort test on first HTTP error |
| `NUM_ACCOUNTS` | number | `10` | Number of accounts to create in setup |
| `TEST_TYPE` | `smoke`, `load`, `stress` | `smoke` | Test type |
| `LOG` | `DEBUG`, `ERROR`, `OFF` | `OFF` | Log level |

### Test Types

| Type | VUs | Duration | Use Case |
|------|-----|----------|----------|
| `smoke` | 1 | 1m | Quick validation |
| `load` | 10 | 5m | Normal production load |
| `stress` | 50 | 10m | Find system limits |

### Available PIX Tests

| Test | File | Description |
|------|------|-------------|
| Dynamic Setup | `pix-test-with-dynamic-setup.js` | Creates real accounts + PIX keys + balance |
| Collection | `collection-load-test.js` | PIX Collection (Cobranca) tests |
| Cashout | `cashout-load-test.js` | PIX Payment (Cash-out) tests |
| DICT | `dict-load-test.js` | DICT (PIX keys) tests |
| Cash-In | `tests/load/cashin/run.js` | PIX Cash-In tests |

---

## Environments

| Environment | Description | Services |
|-------------|-------------|----------|
| `dev` | Local development | `localhost:3000`, `localhost:4014` |
| `sandbox` | AWS sandbox | AWS ELB endpoints |
| `vpc` | Internal VPC (requires VPN) | Internal ALB |
| `capybara` | Alternative local | `192.168.0.2` |

---

## Console Output

When running tests, you'll see:

```
ENV: dev
AUTH: enabled
ABORT_ON_ERROR: disabled
```

This confirms which environment and settings are active.
