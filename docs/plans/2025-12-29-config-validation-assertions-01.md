# Config Validation Assertions Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add comprehensive configuration validation with assertions to prevent services from starting with invalid configuration.

**Architecture:** Extend the existing `pkg/assert` package with new configuration-focused predicates (ValidPort, ValidSSLMode, PositiveInt, InRangeInt). Add a `Validate()` method to each service's Config struct that calls these predicates. Services will fail fast at startup with clear error messages instead of cryptic runtime failures.

**Tech Stack:** Go 1.25+, existing `pkg/assert` package

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.25+
- Tools: Verify with `go version`, `go test`
- Access: None required (local development)
- State: Work from `fix/fred-several-ones-dec-13-2025` branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version        # Expected: go version go1.25.x
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/assert/... -v  # Expected: PASS
git status        # Expected: on fix/fred-several-ones-dec-13-2025 branch
```

## Historical Precedent

**Query:** "config validation assertions environment variables"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Task 1: Add ValidPort Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go:107` (append after line 107)

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Step 1: Write the failing test**

Create test in `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert_test.go` (append after line 571):

```go
// TestValidPort tests the ValidPort predicate for network port validation.
func TestValidPort(t *testing.T) {
	tests := []struct {
		name     string
		port     string
		expected bool
	}{
		{"valid port 80", "80", true},
		{"valid port 443", "443", true},
		{"valid port 8080", "8080", true},
		{"valid port 1", "1", true},
		{"valid port 65535", "65535", true},
		{"valid port 5432 postgres", "5432", true},
		{"invalid port 0", "0", false},
		{"invalid port negative", "-1", false},
		{"invalid port too high", "65536", false},
		{"invalid port way too high", "100000", false},
		{"invalid port empty", "", false},
		{"invalid port non-numeric", "abc", false},
		{"invalid port with spaces", " 80 ", false},
		{"invalid port decimal", "80.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, ValidPort(tt.port))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/assert/... -run TestValidPort -v`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./assert_test.go:XXX:XX: undefined: ValidPort
FAIL    github.com/LerianStudio/midaz/v3/pkg/assert [build failed]
```

**If you see different error:** Check file paths and imports

**Step 3: Write minimal implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go` (append after NonNegativeDecimal function, around line 107):

```go
// ValidPort returns true if port is a valid network port number (1-65535).
// The port must be a numeric string representing a value in the valid range.
//
// Note: Port 0 is invalid for configuration purposes (it's used for dynamic allocation).
// Empty strings, non-numeric values, and out-of-range values return false.
//
// Example:
//
//	assert.That(assert.ValidPort(cfg.DBPort), "DB_PORT must be valid port", "port", cfg.DBPort)
func ValidPort(port string) bool {
	if port == "" {
		return false
	}

	p, err := strconv.Atoi(port)
	if err != nil {
		return false
	}

	return p > 0 && p <= 65535
}
```

**Step 4: Add import for strconv**

The `predicates.go` file needs the `strconv` import. Update the import block at the top:

```go
import (
	"strconv"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)
```

**Step 5: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/assert/... -run TestValidPort -v`

**Expected output:**
```
=== RUN   TestValidPort
=== RUN   TestValidPort/valid_port_80
=== RUN   TestValidPort/valid_port_443
...
--- PASS: TestValidPort (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/assert
```

**Step 6: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/assert/predicates.go pkg/assert/assert_test.go && git commit -m "$(cat <<'EOF'
feat(assert): add ValidPort predicate for config validation

Validates network port strings are numeric and in range 1-65535.
This is the first predicate for the config validation foundation.
EOF
)"
```

**If Task Fails:**

1. **Test won't compile:**
   - Check: Import `strconv` is added to predicates.go
   - Fix: Ensure import block has `"strconv"`
   - Rollback: `git checkout -- pkg/assert/`

2. **Test fails:**
   - Run: `go test ./pkg/assert/... -run TestValidPort -v` (check which case failed)
   - Fix: Adjust implementation logic
   - Rollback: `git reset --hard HEAD`

---

## Task 2: Add ValidSSLMode Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go` (append after ValidPort)
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert_test.go` (append after TestValidPort)

**Prerequisites:**
- Task 1 completed
- ValidPort predicate exists

**Step 1: Write the failing test**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert_test.go`:

```go
// TestValidSSLMode tests the ValidSSLMode predicate for PostgreSQL SSL modes.
func TestValidSSLMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected bool
	}{
		{"disable", "disable", true},
		{"allow", "allow", true},
		{"prefer", "prefer", true},
		{"require", "require", true},
		{"verify-ca", "verify-ca", true},
		{"verify-full", "verify-full", true},
		{"empty string allowed", "", true},
		{"invalid mode", "invalid", false},
		{"typo disable", "disabel", false},
		{"uppercase", "DISABLE", false},
		{"mixed case", "Disable", false},
		{"with spaces", " disable ", false},
		{"partial match", "dis", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, ValidSSLMode(tt.mode))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/assert/... -run TestValidSSLMode -v`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./assert_test.go:XXX:XX: undefined: ValidSSLMode
FAIL
```

**Step 3: Write minimal implementation**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// ValidSSLMode returns true if mode is a valid PostgreSQL SSL mode.
// Valid modes are: disable, allow, prefer, require, verify-ca, verify-full.
// Empty string is also valid (uses PostgreSQL default).
//
// Note: SSL modes are case-sensitive per PostgreSQL documentation.
// Unknown modes will cause connection failures.
//
// Example:
//
//	assert.That(assert.ValidSSLMode(cfg.DBSSLMode), "DB_SSLMODE invalid", "mode", cfg.DBSSLMode)
func ValidSSLMode(mode string) bool {
	validModes := map[string]bool{
		"":            true, // Empty uses PostgreSQL default
		"disable":     true,
		"allow":       true,
		"prefer":      true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
	}

	return validModes[mode]
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/assert/... -run TestValidSSLMode -v`

**Expected output:**
```
=== RUN   TestValidSSLMode
=== RUN   TestValidSSLMode/disable
=== RUN   TestValidSSLMode/allow
...
--- PASS: TestValidSSLMode (0.00s)
PASS
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/assert/predicates.go pkg/assert/assert_test.go && git commit -m "$(cat <<'EOF'
feat(assert): add ValidSSLMode predicate for PostgreSQL config

Validates SSL mode strings against PostgreSQL-supported values.
Supports: disable, allow, prefer, require, verify-ca, verify-full.
EOF
)"
```

**If Task Fails:**

1. **Test fails on empty string:**
   - Check: Map includes `"": true`
   - Fix: Ensure empty string is in the validModes map

---

## Task 3: Add PositiveInt and InRangeInt Predicates

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go` (append after ValidSSLMode)
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert_test.go` (append after TestValidSSLMode)

**Prerequisites:**
- Task 2 completed

**Step 1: Write the failing tests**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert_test.go`:

```go
// TestPositiveInt tests the PositiveInt predicate for int type.
func TestPositiveInt(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		expected bool
	}{
		{"positive 1", 1, true},
		{"positive large", 1000000, true},
		{"zero", 0, false},
		{"negative", -1, false},
		{"negative large", -1000000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, PositiveInt(tt.n))
		})
	}
}

// TestInRangeInt tests the InRangeInt predicate for int type.
func TestInRangeInt(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		min      int
		max      int
		expected bool
	}{
		{"in range", 5, 1, 10, true},
		{"at min", 1, 1, 10, true},
		{"at max", 10, 1, 10, true},
		{"below min", 0, 1, 10, false},
		{"above max", 11, 1, 10, false},
		{"pool size valid", 50, 1, 100, true},
		{"pool size at max", 100, 1, 100, true},
		{"pool size zero invalid", 0, 1, 100, false},
		{"pool size over max", 101, 1, 100, false},
		{"inverted range always false", 5, 10, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, InRangeInt(tt.n, tt.min, tt.max))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/assert/... -run "TestPositiveInt|TestInRangeInt" -v`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./assert_test.go:XXX:XX: undefined: PositiveInt
./assert_test.go:XXX:XX: undefined: InRangeInt
FAIL
```

**Step 3: Write minimal implementation**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// PositiveInt returns true if n > 0.
// This is the int variant of Positive (which uses int64).
//
// Example:
//
//	assert.That(assert.PositiveInt(cfg.MaxWorkers), "MAX_WORKERS must be positive", "value", cfg.MaxWorkers)
func PositiveInt(n int) bool {
	return n > 0
}

// InRangeInt returns true if min <= n <= max.
// This is the int variant of InRange (which uses int64).
//
// Note: If min > max (inverted range), always returns false. This is fail-safe
// behavior - callers should ensure min <= max for correct results.
//
// Example:
//
//	assert.That(assert.InRangeInt(cfg.PoolSize, 1, 100), "POOL_SIZE out of range", "value", cfg.PoolSize)
func InRangeInt(n, minVal, maxVal int) bool {
	return n >= minVal && n <= maxVal
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/assert/... -run "TestPositiveInt|TestInRangeInt" -v`

**Expected output:**
```
=== RUN   TestPositiveInt
--- PASS: TestPositiveInt (0.00s)
=== RUN   TestInRangeInt
--- PASS: TestInRangeInt (0.00s)
PASS
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/assert/predicates.go pkg/assert/assert_test.go && git commit -m "$(cat <<'EOF'
feat(assert): add PositiveInt and InRangeInt predicates

Int variants for config validation where fields are int type.
Used for pool sizes, worker counts, and other bounded integers.
EOF
)"
```

---

## Task 4: Run Code Review (Checkpoint 1 - Predicates Complete)

**Prerequisites:**
- Tasks 1-3 completed
- All predicate tests passing

**Step 1: Verify all tests pass**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/assert/... -v`

**Expected output:**
```
=== RUN   TestThat_Pass
--- PASS: TestThat_Pass
...
=== RUN   TestValidPort
--- PASS: TestValidPort
=== RUN   TestValidSSLMode
--- PASS: TestValidSSLMode
=== RUN   TestPositiveInt
--- PASS: TestPositiveInt
=== RUN   TestInRangeInt
--- PASS: TestInRangeInt
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/assert
```

**Step 2: Dispatch all 3 reviewers in parallel**

- REQUIRED SUB-SKILL: Use requesting-code-review
- All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
- Wait for all to complete

**Step 3: Handle findings by severity (MANDATORY)**

**Critical/High/Medium Issues:**
- Fix immediately (do NOT add TODO comments for these severities)
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code at the relevant location
- Format: `TODO(review): [Issue description] (reported by [reviewer] on [date], severity: Low)`

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location

**Step 4: Proceed only when:**
- Zero Critical/High/Medium issues remain
- All Low issues have TODO(review): comments added
- All Cosmetic issues have FIXME(nitpick): comments added

---

## Task 5: Add Validate Method to Transaction Config

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go`

**Prerequisites:**
- Tasks 1-4 completed
- Predicates are available in pkg/assert

**Step 1: Write the failing test**

Create new test file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config_test.go`:

```go
package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfig_Validate_ValidConfig verifies Validate does not panic for valid config.
func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "localhost",
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "5432",
		PrimaryDBSSLMode:   "disable",
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 25,
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.NotPanics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidPort verifies Validate panics for invalid port.
func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "localhost",
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "99999", // Invalid port
		PrimaryDBSSLMode:   "disable",
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 25,
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_EmptyRequiredField verifies Validate panics for empty required fields.
func TestConfig_Validate_EmptyRequiredField(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "", // Empty required field
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "5432",
		PrimaryDBSSLMode:   "disable",
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 25,
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidSSLMode verifies Validate panics for invalid SSL mode.
func TestConfig_Validate_InvalidSSLMode(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "localhost",
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "5432",
		PrimaryDBSSLMode:   "invalid-mode", // Invalid SSL mode
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 25,
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidPoolSize verifies Validate panics for invalid pool sizes.
func TestConfig_Validate_InvalidPoolSize(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "localhost",
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "5432",
		PrimaryDBSSLMode:   "disable",
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 0, // Invalid: must be positive
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/internal/bootstrap/... -run TestConfig_Validate -v`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap [github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap.test]
./config_test.go:XX:XX: cfg.Validate undefined (type *Config has no field or method Validate)
FAIL
```

**Step 3: Write minimal implementation**

Add the Validate method to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go`.

Find the end of the Config struct (after line 163) and add:

```go
// Validate validates the configuration and panics with clear error messages if invalid.
// This method should be called immediately after loading configuration from environment.
// It uses assert predicates to provide consistent, informative error messages.
func (cfg *Config) Validate() {
	// Server configuration
	assert.NotEmpty(cfg.ServerAddress, "SERVER_ADDRESS is required",
		"field", "ServerAddress")

	// Primary database configuration
	assert.NotEmpty(cfg.PrimaryDBHost, "DB_HOST is required",
		"field", "PrimaryDBHost")
	assert.NotEmpty(cfg.PrimaryDBUser, "DB_USER is required",
		"field", "PrimaryDBUser")
	assert.NotEmpty(cfg.PrimaryDBName, "DB_NAME is required",
		"field", "PrimaryDBName")
	assert.That(assert.ValidPort(cfg.PrimaryDBPort), "DB_PORT must be valid port (1-65535)",
		"field", "PrimaryDBPort", "value", cfg.PrimaryDBPort)
	assert.That(assert.ValidSSLMode(cfg.PrimaryDBSSLMode), "DB_SSLMODE must be valid PostgreSQL SSL mode",
		"field", "PrimaryDBSSLMode", "value", cfg.PrimaryDBSSLMode)

	// Replica database configuration
	assert.NotEmpty(cfg.ReplicaDBHost, "DB_REPLICA_HOST is required",
		"field", "ReplicaDBHost")
	assert.NotEmpty(cfg.ReplicaDBUser, "DB_REPLICA_USER is required",
		"field", "ReplicaDBUser")
	assert.NotEmpty(cfg.ReplicaDBName, "DB_REPLICA_NAME is required",
		"field", "ReplicaDBName")
	assert.That(assert.ValidPort(cfg.ReplicaDBPort), "DB_REPLICA_PORT must be valid port (1-65535)",
		"field", "ReplicaDBPort", "value", cfg.ReplicaDBPort)
	assert.That(assert.ValidSSLMode(cfg.ReplicaDBSSLMode), "DB_REPLICA_SSLMODE must be valid PostgreSQL SSL mode",
		"field", "ReplicaDBSSLMode", "value", cfg.ReplicaDBSSLMode)

	// Database pool configuration
	assert.That(assert.InRangeInt(cfg.MaxOpenConnections, 1, 500), "DB_MAX_OPEN_CONNS must be 1-500",
		"field", "MaxOpenConnections", "value", cfg.MaxOpenConnections)
	assert.That(assert.InRangeInt(cfg.MaxIdleConnections, 1, 100), "DB_MAX_IDLE_CONNS must be 1-100",
		"field", "MaxIdleConnections", "value", cfg.MaxIdleConnections)

	// MongoDB configuration
	assert.NotEmpty(cfg.MongoDBHost, "MONGO_HOST is required",
		"field", "MongoDBHost")
	assert.NotEmpty(cfg.MongoDBName, "MONGO_NAME is required",
		"field", "MongoDBName")
	assert.That(assert.ValidPort(cfg.MongoDBPort), "MONGO_PORT must be valid port (1-65535)",
		"field", "MongoDBPort", "value", cfg.MongoDBPort)
	assert.That(assert.InRangeInt(cfg.MaxPoolSize, 1, 1000), "MONGO_MAX_POOL_SIZE must be 1-1000",
		"field", "MaxPoolSize", "value", cfg.MaxPoolSize)

	// Redis configuration
	assert.NotEmpty(cfg.RedisHost, "REDIS_HOST is required",
		"field", "RedisHost")
	assert.That(assert.InRangeInt(cfg.RedisPoolSize, 1, 1000), "REDIS_POOL_SIZE must be 1-1000",
		"field", "RedisPoolSize", "value", cfg.RedisPoolSize)

	// RabbitMQ configuration
	assert.NotEmpty(cfg.RabbitMQHost, "RABBITMQ_HOST is required",
		"field", "RabbitMQHost")
	assert.That(assert.ValidPort(cfg.RabbitMQPortHost), "RABBITMQ_PORT_HOST must be valid port (1-65535)",
		"field", "RabbitMQPortHost", "value", cfg.RabbitMQPortHost)
	assert.That(assert.ValidPort(cfg.RabbitMQPortAMQP), "RABBITMQ_PORT_AMQP must be valid port (1-65535)",
		"field", "RabbitMQPortAMQP", "value", cfg.RabbitMQPortAMQP)
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/internal/bootstrap/... -run TestConfig_Validate -v`

**Expected output:**
```
=== RUN   TestConfig_Validate_ValidConfig
--- PASS: TestConfig_Validate_ValidConfig (0.00s)
=== RUN   TestConfig_Validate_InvalidPort
--- PASS: TestConfig_Validate_InvalidPort (0.00s)
=== RUN   TestConfig_Validate_EmptyRequiredField
--- PASS: TestConfig_Validate_EmptyRequiredField (0.00s)
=== RUN   TestConfig_Validate_InvalidSSLMode
--- PASS: TestConfig_Validate_InvalidSSLMode (0.00s)
=== RUN   TestConfig_Validate_InvalidPoolSize
--- PASS: TestConfig_Validate_InvalidPoolSize (0.00s)
PASS
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/transaction/internal/bootstrap/config.go components/transaction/internal/bootstrap/config_test.go && git commit -m "$(cat <<'EOF'
feat(transaction): add Config.Validate() method with assertions

Validates all critical configuration fields at startup:
- Required strings (DB hosts, names, users)
- Port numbers (1-65535 range)
- SSL modes (PostgreSQL valid values)
- Pool sizes (bounded ranges)

Services will fail fast with clear error messages.
EOF
)"
```

**If Task Fails:**

1. **Import error:**
   - Check: `assert` package import in config.go
   - Fix: Should already be imported, verify import path

2. **Test fails on valid config:**
   - Run individual test: `go test -run TestConfig_Validate_ValidConfig -v`
   - Check: All required fields have valid values in test

---

## Task 6: Integrate Validate into Transaction InitServers

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go:170-173`

**Prerequisites:**
- Task 5 completed
- Validate method exists on Config

**Step 1: Modify InitServers to call Validate**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go`, find lines 169-173:

```go
	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for transaction",
		"package", "bootstrap",
		"function", "InitServers")
```

Replace with:

```go
	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for transaction",
		"package", "bootstrap",
		"function", "InitServers")

	// Validate configuration before proceeding
	cfg.Validate()
```

**Step 2: Verify existing tests still pass**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/internal/bootstrap/... -v`

**Expected output:**
```
=== RUN   TestConfig_Validate_ValidConfig
--- PASS: TestConfig_Validate_ValidConfig (0.00s)
...
PASS
```

**Step 3: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/transaction/internal/bootstrap/config.go && git commit -m "$(cat <<'EOF'
feat(transaction): call Config.Validate() in InitServers

Services now fail fast at startup if configuration is invalid.
This prevents cryptic runtime failures from bad config values.
EOF
)"
```

---

## Task 7: Add Validate Method to Onboarding Config

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go`
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config_test.go`

**Prerequisites:**
- Task 6 completed

**Step 1: Write the failing test**

Create `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config_test.go`:

```go
package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfig_Validate_ValidConfig verifies Validate does not panic for valid config.
func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := &Config{
		ServerAddress:            ":8080",
		PrimaryDBHost:            "localhost",
		PrimaryDBUser:            "postgres",
		PrimaryDBName:            "midaz",
		PrimaryDBPort:            "5432",
		PrimaryDBSSLMode:         "disable",
		ReplicaDBHost:            "localhost",
		ReplicaDBUser:            "postgres",
		ReplicaDBName:            "midaz",
		ReplicaDBPort:            "5432",
		ReplicaDBSSLMode:         "disable",
		MaxOpenConnections:       25,
		MaxIdleConnections:       5,
		MongoDBHost:              "localhost",
		MongoDBName:              "midaz_meta",
		MongoDBPort:              "27017",
		MaxPoolSize:              100,
		RedisHost:                "localhost:6379",
		RedisPoolSize:            10,
		TransactionGRPCAddress:   "localhost",
		TransactionGRPCPort:      "50051",
	}

	require.NotPanics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidPort verifies Validate panics for invalid port.
func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := &Config{
		ServerAddress:            ":8080",
		PrimaryDBHost:            "localhost",
		PrimaryDBUser:            "postgres",
		PrimaryDBName:            "midaz",
		PrimaryDBPort:            "invalid", // Invalid port
		PrimaryDBSSLMode:         "disable",
		ReplicaDBHost:            "localhost",
		ReplicaDBUser:            "postgres",
		ReplicaDBName:            "midaz",
		ReplicaDBPort:            "5432",
		ReplicaDBSSLMode:         "disable",
		MaxOpenConnections:       25,
		MaxIdleConnections:       5,
		MongoDBHost:              "localhost",
		MongoDBName:              "midaz_meta",
		MongoDBPort:              "27017",
		MaxPoolSize:              100,
		RedisHost:                "localhost:6379",
		RedisPoolSize:            10,
		TransactionGRPCAddress:   "localhost",
		TransactionGRPCPort:      "50051",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_MissingGRPCConfig verifies Validate panics for missing gRPC config.
func TestConfig_Validate_MissingGRPCConfig(t *testing.T) {
	cfg := &Config{
		ServerAddress:            ":8080",
		PrimaryDBHost:            "localhost",
		PrimaryDBUser:            "postgres",
		PrimaryDBName:            "midaz",
		PrimaryDBPort:            "5432",
		PrimaryDBSSLMode:         "disable",
		ReplicaDBHost:            "localhost",
		ReplicaDBUser:            "postgres",
		ReplicaDBName:            "midaz",
		ReplicaDBPort:            "5432",
		ReplicaDBSSLMode:         "disable",
		MaxOpenConnections:       25,
		MaxIdleConnections:       5,
		MongoDBHost:              "localhost",
		MongoDBName:              "midaz_meta",
		MongoDBPort:              "27017",
		MaxPoolSize:              100,
		RedisHost:                "localhost:6379",
		RedisPoolSize:            10,
		TransactionGRPCAddress:   "", // Empty required field
		TransactionGRPCPort:      "50051",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/onboarding/internal/bootstrap/... -run TestConfig_Validate -v`

**Expected output:**
```
./config_test.go:XX:XX: cfg.Validate undefined (type *Config has no field or method Validate)
FAIL
```

**Step 3: Write minimal implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go` (after the Config struct, around line 122):

```go
// Validate validates the configuration and panics with clear error messages if invalid.
// This method should be called immediately after loading configuration from environment.
func (cfg *Config) Validate() {
	// Server configuration
	assert.NotEmpty(cfg.ServerAddress, "SERVER_ADDRESS is required",
		"field", "ServerAddress")

	// Primary database configuration
	assert.NotEmpty(cfg.PrimaryDBHost, "DB_HOST is required",
		"field", "PrimaryDBHost")
	assert.NotEmpty(cfg.PrimaryDBUser, "DB_USER is required",
		"field", "PrimaryDBUser")
	assert.NotEmpty(cfg.PrimaryDBName, "DB_NAME is required",
		"field", "PrimaryDBName")
	assert.That(assert.ValidPort(cfg.PrimaryDBPort), "DB_PORT must be valid port (1-65535)",
		"field", "PrimaryDBPort", "value", cfg.PrimaryDBPort)
	assert.That(assert.ValidSSLMode(cfg.PrimaryDBSSLMode), "DB_SSLMODE must be valid PostgreSQL SSL mode",
		"field", "PrimaryDBSSLMode", "value", cfg.PrimaryDBSSLMode)

	// Replica database configuration
	assert.NotEmpty(cfg.ReplicaDBHost, "DB_REPLICA_HOST is required",
		"field", "ReplicaDBHost")
	assert.NotEmpty(cfg.ReplicaDBUser, "DB_REPLICA_USER is required",
		"field", "ReplicaDBUser")
	assert.NotEmpty(cfg.ReplicaDBName, "DB_REPLICA_NAME is required",
		"field", "ReplicaDBName")
	assert.That(assert.ValidPort(cfg.ReplicaDBPort), "DB_REPLICA_PORT must be valid port (1-65535)",
		"field", "ReplicaDBPort", "value", cfg.ReplicaDBPort)
	assert.That(assert.ValidSSLMode(cfg.ReplicaDBSSLMode), "DB_REPLICA_SSLMODE must be valid PostgreSQL SSL mode",
		"field", "ReplicaDBSSLMode", "value", cfg.ReplicaDBSSLMode)

	// Database pool configuration
	assert.That(assert.InRangeInt(cfg.MaxOpenConnections, 1, 500), "DB_MAX_OPEN_CONNS must be 1-500",
		"field", "MaxOpenConnections", "value", cfg.MaxOpenConnections)
	assert.That(assert.InRangeInt(cfg.MaxIdleConnections, 1, 100), "DB_MAX_IDLE_CONNS must be 1-100",
		"field", "MaxIdleConnections", "value", cfg.MaxIdleConnections)

	// MongoDB configuration
	assert.NotEmpty(cfg.MongoDBHost, "MONGO_HOST is required",
		"field", "MongoDBHost")
	assert.NotEmpty(cfg.MongoDBName, "MONGO_NAME is required",
		"field", "MongoDBName")
	assert.That(assert.ValidPort(cfg.MongoDBPort), "MONGO_PORT must be valid port (1-65535)",
		"field", "MongoDBPort", "value", cfg.MongoDBPort)
	assert.That(assert.InRangeInt(cfg.MaxPoolSize, 1, 1000), "MONGO_MAX_POOL_SIZE must be 1-1000",
		"field", "MaxPoolSize", "value", cfg.MaxPoolSize)

	// Redis configuration
	assert.NotEmpty(cfg.RedisHost, "REDIS_HOST is required",
		"field", "RedisHost")
	assert.That(assert.InRangeInt(cfg.RedisPoolSize, 1, 1000), "REDIS_POOL_SIZE must be 1-1000",
		"field", "RedisPoolSize", "value", cfg.RedisPoolSize)

	// gRPC configuration (required for transaction service communication)
	assert.NotEmpty(cfg.TransactionGRPCAddress, "TRANSACTION_GRPC_ADDRESS is required",
		"field", "TransactionGRPCAddress")
	assert.That(assert.ValidPort(cfg.TransactionGRPCPort), "TRANSACTION_GRPC_PORT must be valid port (1-65535)",
		"field", "TransactionGRPCPort", "value", cfg.TransactionGRPCPort)
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/onboarding/internal/bootstrap/... -run TestConfig_Validate -v`

**Expected output:**
```
=== RUN   TestConfig_Validate_ValidConfig
--- PASS: TestConfig_Validate_ValidConfig (0.00s)
=== RUN   TestConfig_Validate_InvalidPort
--- PASS: TestConfig_Validate_InvalidPort (0.00s)
=== RUN   TestConfig_Validate_MissingGRPCConfig
--- PASS: TestConfig_Validate_MissingGRPCConfig (0.00s)
PASS
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/onboarding/internal/bootstrap/config.go components/onboarding/internal/bootstrap/config_test.go && git commit -m "$(cat <<'EOF'
feat(onboarding): add Config.Validate() method with assertions

Validates all critical configuration fields at startup including
gRPC configuration for transaction service communication.
EOF
)"
```

---

## Task 8: Integrate Validate into Onboarding InitServers

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go:130-134`

**Prerequisites:**
- Task 7 completed

**Step 1: Modify InitServers to call Validate**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go`, find lines 130-134:

```go
	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for onboarding",
		"package", "bootstrap",
		"function", "InitServers")
```

Replace with:

```go
	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for onboarding",
		"package", "bootstrap",
		"function", "InitServers")

	// Validate configuration before proceeding
	cfg.Validate()
```

**Step 2: Also update InitServersWithOptions**

Find the similar block in `InitServersWithOptions` (around line 431-434) and add after the error check:

```go
	err = libCommons.SetConfigFromEnvVars(cfg)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Config")
	}

	// Validate configuration before proceeding
	cfg.Validate()
```

**Step 3: Verify tests pass**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/onboarding/internal/bootstrap/... -v`

**Expected output:**
```
PASS
```

**Step 4: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/onboarding/internal/bootstrap/config.go && git commit -m "$(cat <<'EOF'
feat(onboarding): call Config.Validate() in InitServers

Validates configuration in both InitServers and InitServersWithOptions
to ensure consistent fail-fast behavior.
EOF
)"
```

---

## Task 9: Add Validate Method to CRM Config

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/bootstrap/config.go`
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/bootstrap/config_test.go`

**Prerequisites:**
- Task 8 completed

**Step 1: Write the failing test**

Create `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/bootstrap/config_test.go`:

```go
package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfig_Validate_ValidConfig verifies Validate does not panic for valid config.
func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := &Config{
		ServerAddress:    ":8080",
		MongoDBHost:      "localhost",
		MongoDBName:      "midaz_crm",
		MongoDBPort:      "27017",
		MaxPoolSize:      100,
		HashSecretKey:    "test-hash-key-32-chars-long!!!!",
		EncryptSecretKey: "test-encrypt-key-32-chars-long!",
	}

	require.NotPanics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidPort verifies Validate panics for invalid port.
func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := &Config{
		ServerAddress:    ":8080",
		MongoDBHost:      "localhost",
		MongoDBName:      "midaz_crm",
		MongoDBPort:      "0", // Invalid port
		MaxPoolSize:      100,
		HashSecretKey:    "test-hash-key-32-chars-long!!!!",
		EncryptSecretKey: "test-encrypt-key-32-chars-long!",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_MissingCryptoKeys verifies Validate panics for missing crypto keys.
func TestConfig_Validate_MissingCryptoKeys(t *testing.T) {
	cfg := &Config{
		ServerAddress:    ":8080",
		MongoDBHost:      "localhost",
		MongoDBName:      "midaz_crm",
		MongoDBPort:      "27017",
		MaxPoolSize:      100,
		HashSecretKey:    "", // Missing required key
		EncryptSecretKey: "test-encrypt-key-32-chars-long!",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/crm/internal/bootstrap/... -run TestConfig_Validate -v`

**Expected output:**
```
./config_test.go:XX:XX: cfg.Validate undefined (type *Config has no field or method Validate)
FAIL
```

**Step 3: Write minimal implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/bootstrap/config.go` (after the Config struct, around line 46):

```go
// Validate validates the configuration and panics with clear error messages if invalid.
// This method should be called immediately after loading configuration from environment.
func (cfg *Config) Validate() {
	// Server configuration
	assert.NotEmpty(cfg.ServerAddress, "SERVER_ADDRESS is required",
		"field", "ServerAddress")

	// MongoDB configuration
	assert.NotEmpty(cfg.MongoDBHost, "MONGO_HOST is required",
		"field", "MongoDBHost")
	assert.NotEmpty(cfg.MongoDBName, "MONGO_NAME is required",
		"field", "MongoDBName")
	assert.That(assert.ValidPort(cfg.MongoDBPort), "MONGO_PORT must be valid port (1-65535)",
		"field", "MongoDBPort", "value", cfg.MongoDBPort)
	assert.That(assert.InRangeInt(cfg.MaxPoolSize, 1, 1000), "MONGO_MAX_POOL_SIZE must be 1-1000",
		"field", "MaxPoolSize", "value", cfg.MaxPoolSize)

	// Crypto configuration (required for data security)
	assert.NotEmpty(cfg.HashSecretKey, "LCRYPTO_HASH_SECRET_KEY is required",
		"field", "HashSecretKey")
	assert.NotEmpty(cfg.EncryptSecretKey, "LCRYPTO_ENCRYPT_SECRET_KEY is required",
		"field", "EncryptSecretKey")
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/crm/internal/bootstrap/... -run TestConfig_Validate -v`

**Expected output:**
```
=== RUN   TestConfig_Validate_ValidConfig
--- PASS: TestConfig_Validate_ValidConfig (0.00s)
=== RUN   TestConfig_Validate_InvalidPort
--- PASS: TestConfig_Validate_InvalidPort (0.00s)
=== RUN   TestConfig_Validate_MissingCryptoKeys
--- PASS: TestConfig_Validate_MissingCryptoKeys (0.00s)
PASS
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/crm/internal/bootstrap/config.go components/crm/internal/bootstrap/config_test.go && git commit -m "$(cat <<'EOF'
feat(crm): add Config.Validate() method with assertions

Validates MongoDB and crypto configuration at startup.
Ensures required encryption keys are present.
EOF
)"
```

---

## Task 10: Integrate Validate into CRM InitServers

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/bootstrap/config.go:52-56`

**Prerequisites:**
- Task 9 completed

**Step 1: Modify InitServers to call Validate**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/bootstrap/config.go`, find lines 52-56:

```go
	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for CRM",
		"package", "bootstrap",
		"function", "InitServers")
```

Replace with:

```go
	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for CRM",
		"package", "bootstrap",
		"function", "InitServers")

	// Validate configuration before proceeding
	cfg.Validate()
```

**Step 2: Verify tests pass**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/crm/internal/bootstrap/... -v`

**Expected output:**
```
PASS
```

**Step 3: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/crm/internal/bootstrap/config.go && git commit -m "$(cat <<'EOF'
feat(crm): call Config.Validate() in InitServers

CRM service now fails fast at startup if configuration is invalid.
EOF
)"
```

---

## Task 11: Run Code Review (Checkpoint 2 - All Components Complete)

**Prerequisites:**
- Tasks 1-10 completed
- All component tests passing

**Step 1: Run all tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/assert/... ./components/transaction/internal/bootstrap/... ./components/onboarding/internal/bootstrap/... ./components/crm/internal/bootstrap/... -v`

**Expected output:**
```
=== RUN   TestValidPort
--- PASS: TestValidPort
=== RUN   TestValidSSLMode
--- PASS: TestValidSSLMode
...
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/assert
PASS
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
PASS
ok      github.com/LerianStudio/midaz/v3/components/onboarding/internal/bootstrap
PASS
ok      github.com/LerianStudio/midaz/v3/components/crm/internal/bootstrap
```

**Step 2: Dispatch all 3 reviewers in parallel**

- REQUIRED SUB-SKILL: Use requesting-code-review
- All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
- Wait for all to complete

**Step 3: Handle findings by severity (MANDATORY)**

See Task 4 for handling guidelines.

---

## Task 12: Run Full Test Suite

**Prerequisites:**
- Task 11 completed (code review passed)

**Step 1: Run all package tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./... -count=1 2>&1 | head -100`

**Expected output:**
```
ok      github.com/LerianStudio/midaz/v3/pkg/assert
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
ok      github.com/LerianStudio/midaz/v3/components/onboarding/internal/bootstrap
ok      github.com/LerianStudio/midaz/v3/components/crm/internal/bootstrap
...
```

**Step 2: Run linter**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && golangci-lint run ./pkg/assert/... ./components/transaction/internal/bootstrap/... ./components/onboarding/internal/bootstrap/... ./components/crm/internal/bootstrap/...`

**Expected output:**
```
(no output - clean)
```

**Step 3: Verify no regressions**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...`

**Expected output:**
```
(no output - successful build)
```

**If Task Fails:**

1. **Lint errors:**
   - Fix: Address each lint issue
   - Re-run linter

2. **Build errors:**
   - Check: Import paths are correct
   - Fix: Missing imports or type errors

---

## Task 13: Final Verification and Summary

**Prerequisites:**
- Task 12 completed

**Step 1: View git log of changes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git log --oneline HEAD~10..HEAD`

**Expected output:**
```
XXXXXXX feat(crm): call Config.Validate() in InitServers
XXXXXXX feat(crm): add Config.Validate() method with assertions
XXXXXXX feat(onboarding): call Config.Validate() in InitServers
XXXXXXX feat(onboarding): add Config.Validate() method with assertions
XXXXXXX feat(transaction): call Config.Validate() in InitServers
XXXXXXX feat(transaction): add Config.Validate() method with assertions
XXXXXXX feat(assert): add PositiveInt and InRangeInt predicates
XXXXXXX feat(assert): add ValidSSLMode predicate for PostgreSQL config
XXXXXXX feat(assert): add ValidPort predicate for config validation
```

**Step 2: Count assertions added**

The implementation adds approximately:
- 4 new predicates: ValidPort, ValidSSLMode, PositiveInt, InRangeInt
- ~20 assertions in transaction Config.Validate()
- ~18 assertions in onboarding Config.Validate()
- ~7 assertions in CRM Config.Validate()
- **Total: ~45 assertions** across all components

**Step 3: Document completion**

All services now validate configuration at startup:
- Invalid port numbers (0, >65535, non-numeric) cause immediate failure
- Invalid SSL modes cause immediate failure
- Empty required fields cause immediate failure
- Out-of-range pool sizes cause immediate failure

Error messages are clear and include the field name and invalid value.

---

## Summary

**Files Modified:**
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go` - Added 4 new predicates
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert_test.go` - Added tests for new predicates
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go` - Added Validate() method and integration
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config_test.go` - Created tests
- `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go` - Added Validate() method and integration
- `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config_test.go` - Created tests
- `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/bootstrap/config.go` - Added Validate() method and integration
- `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/bootstrap/config_test.go` - Created tests

**Expected Outcome:**
- Services fail fast at startup with clear error messages
- ~45 configuration assertions total
- Zero invalid configurations reach runtime
- Consistent error format with field names and values
