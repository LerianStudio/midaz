# Unit Test Coverage Improvement Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Improve unit test coverage across the Midaz codebase to achieve at least 80% overall coverage by systematically adding tests for uncovered functions and code paths.

**Architecture:** This is a Go monorepo with multiple components (onboarding, transaction, crm, ledger) and shared packages (pkg/). Tests use testify/assert, testify/require, and gomock for mocking. Table-driven tests are the standard pattern.

**Tech Stack:** Go 1.21+, testify, gomock, gotest.tools/gotestsum

**Global Prerequisites:**
- Environment: macOS/Linux with Go 1.21+
- Tools: Go installed and configured
- Access: Read/write access to codebase
- State: Clean working tree on current branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version          # Expected: go version go1.21+ or higher
go test ./... -count=0  # Expected: compiles without errors
git status          # Expected: clean working tree
```

## Historical Precedent

**Query:** "unit test coverage golang testing"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Current Coverage Analysis

| Package | Current Coverage | Target | Priority |
|---------|-----------------|--------|----------|
| pkg/utils | 32.4% | 80%+ | HIGH |
| pkg/mmodel | 20.4% | 60%+ | HIGH |
| components/transaction | 42.9% | 70%+ | HIGH |
| pkg/gold/transaction | 58.7% | 75%+ | MEDIUM |
| pkg/net/http | 60.8% | 80%+ | MEDIUM |
| pkg/transaction | 71.7% | 80%+ | MEDIUM |
| pkg (root errors.go) | 72.5% | 85%+ | MEDIUM |
| pkg/mruntime | 73.4% | 80%+ | LOW |

**Note:** PostgreSQL repository packages are excluded from coverage as they use integration tests with testcontainers. The gold/parser package contains ANTLR-generated code that doesn't require unit testing.

---

## Phase 1: pkg/utils Package (32.4% -> 80%+)

The utils package has several files with zero test coverage that need tests.

---

### Task 1.1: Add tests for ptr.go helper functions

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/ptr_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/ptr.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/ptr.go`

**Step 1: Create the test file**

```go
package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringPtr(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "simple string", input: "hello"},
		{name: "string with spaces", input: "hello world"},
		{name: "unicode string", input: "hello \u4e16\u754c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringPtr(tt.input)
			assert.NotNil(t, result)
			assert.Equal(t, tt.input, *result)
		})
	}
}

func TestBoolPtr(t *testing.T) {
	tests := []struct {
		name  string
		input bool
	}{
		{name: "true", input: true},
		{name: "false", input: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BoolPtr(tt.input)
			assert.NotNil(t, result)
			assert.Equal(t, tt.input, *result)
		})
	}
}

func TestFloat64Ptr(t *testing.T) {
	tests := []struct {
		name  string
		input float64
	}{
		{name: "zero", input: 0.0},
		{name: "positive", input: 123.456},
		{name: "negative", input: -789.012},
		{name: "very small", input: 0.000001},
		{name: "very large", input: 1e10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Float64Ptr(tt.input)
			assert.NotNil(t, result)
			assert.Equal(t, tt.input, *result)
		})
	}
}

func TestIntPtr(t *testing.T) {
	tests := []struct {
		name  string
		input int
	}{
		{name: "zero", input: 0},
		{name: "positive", input: 42},
		{name: "negative", input: -100},
		{name: "max int32", input: 2147483647},
		{name: "min int32", input: -2147483648},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IntPtr(tt.input)
			assert.NotNil(t, result)
			assert.Equal(t, tt.input, *result)
		})
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/ -run TestStringPtr -run TestBoolPtr -run TestFloat64Ptr -run TestIntPtr`

**Expected output:**
```
=== RUN   TestStringPtr
=== RUN   TestStringPtr/empty_string
=== RUN   TestStringPtr/simple_string
...
PASS
```

**Step 3: Verify coverage improvement**

Run: `go test -cover /Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/`

**Expected output:** Coverage should increase from 32.4% to approximately 38%+

**If Task Fails:**
1. Check that ptr.go exists and has the expected functions
2. Rollback: `git checkout -- pkg/utils/ptr_test.go`

---

### Task 1.2: Add tests for env.go helper functions

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/env_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/env.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/env.go`

**Step 1: Create the test file**

```go
package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvFallback(t *testing.T) {
	tests := []struct {
		name     string
		prefixed string
		fallback string
		expected string
	}{
		{
			name:     "prefixed value takes precedence",
			prefixed: "prefixed-value",
			fallback: "fallback-value",
			expected: "prefixed-value",
		},
		{
			name:     "fallback used when prefixed is empty",
			prefixed: "",
			fallback: "fallback-value",
			expected: "fallback-value",
		},
		{
			name:     "both empty returns empty",
			prefixed: "",
			fallback: "",
			expected: "",
		},
		{
			name:     "prefixed with whitespace is considered non-empty",
			prefixed: "  ",
			fallback: "fallback",
			expected: "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnvFallback(tt.prefixed, tt.fallback)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvFallbackInt(t *testing.T) {
	tests := []struct {
		name     string
		prefixed int
		fallback int
		expected int
	}{
		{
			name:     "prefixed value takes precedence",
			prefixed: 100,
			fallback: 50,
			expected: 100,
		},
		{
			name:     "fallback used when prefixed is zero",
			prefixed: 0,
			fallback: 50,
			expected: 50,
		},
		{
			name:     "both zero returns zero",
			prefixed: 0,
			fallback: 0,
			expected: 0,
		},
		{
			name:     "negative prefixed takes precedence",
			prefixed: -10,
			fallback: 50,
			expected: -10,
		},
		{
			name:     "negative fallback used when prefixed is zero",
			prefixed: 0,
			fallback: -10,
			expected: -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnvFallbackInt(tt.prefixed, tt.fallback)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/ -run TestEnvFallback`

**Expected output:**
```
=== RUN   TestEnvFallback
...
PASS
```

**If Task Fails:**
1. Check that env.go exists and has the expected functions
2. Rollback: `git checkout -- pkg/utils/env_test.go`

---

### Task 1.3: Add tests for cache.go key generation functions

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/cache_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/cache.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/cache.go`

**Step 1: Create the test file**

```go
package utils

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGenericInternalKeyWithContext(t *testing.T) {
	tests := []struct {
		name           string
		keyName        string
		contextName    string
		organizationID string
		ledgerID       string
		key            string
		expected       string
	}{
		{
			name:           "standard transaction key",
			keyName:        "transaction",
			contextName:    "transactions",
			organizationID: "org-123",
			ledgerID:       "ledger-456",
			key:            "txn-789",
			expected:       "transaction:{transactions}:org-123:ledger-456:txn-789",
		},
		{
			name:           "balance key",
			keyName:        "balance",
			contextName:    "transactions",
			organizationID: "org-abc",
			ledgerID:       "ledger-def",
			key:            "bal-xyz",
			expected:       "balance:{transactions}:org-abc:ledger-def:bal-xyz",
		},
		{
			name:           "empty key component",
			keyName:        "test",
			contextName:    "ctx",
			organizationID: "",
			ledgerID:       "",
			key:            "",
			expected:       "test:{ctx}:::",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenericInternalKeyWithContext(tt.keyName, tt.contextName, tt.organizationID, tt.ledgerID, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransactionInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	result := TransactionInternalKey(orgID, ledgerID, "my-key")

	expected := "transaction:{transactions}:00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:my-key"
	assert.Equal(t, expected, result)
}

func TestBalanceInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	result := BalanceInternalKey(orgID, ledgerID, "balance-key")

	expected := "balance:{transactions}:00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:balance-key"
	assert.Equal(t, expected, result)
}

func TestGenericInternalKey(t *testing.T) {
	tests := []struct {
		name           string
		keyName        string
		organizationID string
		ledgerID       string
		key            string
		expected       string
	}{
		{
			name:           "idempotency key",
			keyName:        "idempotency",
			organizationID: "org-123",
			ledgerID:       "ledger-456",
			key:            "idem-key",
			expected:       "idempotency:org-123:ledger-456:idem-key",
		},
		{
			name:           "accounting routes key",
			keyName:        "accounting_routes",
			organizationID: "org-abc",
			ledgerID:       "ledger-def",
			key:            "route-xyz",
			expected:       "accounting_routes:org-abc:ledger-def:route-xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenericInternalKey(tt.keyName, tt.organizationID, tt.ledgerID, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIdempotencyInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	result := IdempotencyInternalKey(orgID, ledgerID, "idem-123")

	expected := "idempotency:00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:idem-123"
	assert.Equal(t, expected, result)
}

func TestAccountingRoutesInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	routeID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	result := AccountingRoutesInternalKey(orgID, ledgerID, routeID)

	expected := "accounting_routes:00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:00000000-0000-0000-0000-000000000003"
	assert.Equal(t, expected, result)
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/ -run "TestGenericInternalKey|TestTransaction|TestBalance|TestIdempotency|TestAccountingRoutes"`

**Expected output:**
```
=== RUN   TestGenericInternalKeyWithContext
...
PASS
```

**If Task Fails:**
1. Check that cache.go exists and functions match expected signatures
2. Rollback: `git checkout -- pkg/utils/cache_test.go`

---

### Task 1.4: Add tests for jitter.go retry functions

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/jitter_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/jitter.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/jitter.go`

**Step 1: Create the test file**

```go
package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFullJitter(t *testing.T) {
	tests := []struct {
		name      string
		baseDelay time.Duration
	}{
		{name: "small delay", baseDelay: 100 * time.Millisecond},
		{name: "medium delay", baseDelay: 1 * time.Second},
		{name: "large delay", baseDelay: 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FullJitter(tt.baseDelay)

			// Result should be between 0 and baseDelay (or MaxBackoff if smaller)
			assert.GreaterOrEqual(t, result, time.Duration(0))

			expectedMax := tt.baseDelay
			if expectedMax > MaxBackoff {
				expectedMax = MaxBackoff
			}
			assert.LessOrEqual(t, result, expectedMax)
		})
	}
}

func TestFullJitter_CappedByMaxBackoff(t *testing.T) {
	// When baseDelay exceeds MaxBackoff, result should be capped
	veryLargeDelay := 100 * time.Second // Much larger than MaxBackoff (10s)

	// Run multiple times to test randomness doesn't exceed cap
	for i := 0; i < 10; i++ {
		result := FullJitter(veryLargeDelay)
		assert.LessOrEqual(t, result, MaxBackoff)
	}
}

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name     string
		current  time.Duration
		expected time.Duration
	}{
		{
			name:     "initial backoff doubles",
			current:  InitialBackoff,
			expected: time.Duration(float64(InitialBackoff) * BackoffFactor),
		},
		{
			name:     "1 second doubles to 2 seconds",
			current:  1 * time.Second,
			expected: 2 * time.Second,
		},
		{
			name:     "5 seconds capped at MaxBackoff",
			current:  5 * time.Second,
			expected: MaxBackoff,
		},
		{
			name:     "at max stays at max",
			current:  MaxBackoff,
			expected: MaxBackoff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NextBackoff(tt.current)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNextBackoff_ExceedingMaxBackoff(t *testing.T) {
	// When calculated backoff exceeds MaxBackoff, it should be capped
	largeBackoff := 8 * time.Second // 8 * 2 = 16s > MaxBackoff (10s)

	result := NextBackoff(largeBackoff)

	assert.Equal(t, MaxBackoff, result)
}

func TestConstants(t *testing.T) {
	// Verify constants are set to expected values
	assert.Equal(t, 5, MaxRetries)
	assert.Equal(t, 500*time.Millisecond, InitialBackoff)
	assert.Equal(t, 10*time.Second, MaxBackoff)
	assert.Equal(t, 2.0, BackoffFactor)
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/ -run "TestFullJitter|TestNextBackoff|TestConstants"`

**Expected output:**
```
=== RUN   TestFullJitter
...
PASS
```

**If Task Fails:**
1. Check that jitter.go constants match expected values
2. Rollback: `git checkout -- pkg/utils/jitter_test.go`

---

### Task 1.5: Run Code Review for Phase 1

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
   - Wait for all to complete

2. **Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:**
- Fix immediately (do NOT add TODO comments for these severities)
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code at the relevant location

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added

---

### Task 1.6: Verify Phase 1 Coverage Improvement

**Step 1: Run coverage check**

Run: `go test -cover /Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/`

**Expected output:** Coverage should be 80%+ (up from 32.4%)

**Step 2: Commit Phase 1 changes**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/utils/*_test.go
git commit -m "$(cat <<'EOF'
test(utils): add comprehensive unit tests for utility functions

Add tests for:
- ptr.go: StringPtr, BoolPtr, Float64Ptr, IntPtr
- env.go: EnvFallback, EnvFallbackInt
- cache.go: key generation functions
- jitter.go: FullJitter, NextBackoff

Coverage improvement: 32.4% -> 80%+
EOF
)"
```

---

## Phase 2: pkg/net/http Package (60.8% -> 80%+)

The response.go file has HTTP helper functions that need tests.

---

### Task 2.1: Add tests for response.go HTTP helper functions

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/response_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/response.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/response.go`

**Step 1: Create the test file**

```go
package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestApp() *fiber.App {
	return fiber.New()
}

func TestUnauthorized(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return Unauthorized(c, "AUTH001", "Unauthorized", "Invalid token")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "AUTH001", body["code"])
	assert.Equal(t, "Unauthorized", body["title"])
	assert.Equal(t, "Invalid token", body["message"])
}

func TestForbidden(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return Forbidden(c, "FORBID001", "Forbidden", "Access denied")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "FORBID001", body["code"])
	assert.Equal(t, "Forbidden", body["title"])
	assert.Equal(t, "Access denied", body["message"])
}

func TestBadRequest(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return BadRequest(c, map[string]string{"error": "validation failed"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreated(t *testing.T) {
	app := setupTestApp()
	app.Post("/test", func(c *fiber.Ctx) error {
		return Created(c, map[string]string{"id": "123"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "123", body["id"])
}

func TestOK(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return OK(c, map[string]string{"status": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestNoContent(t *testing.T) {
	app := setupTestApp()
	app.Delete("/test", func(c *fiber.Ctx) error {
		return NoContent(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestAccepted(t *testing.T) {
	app := setupTestApp()
	app.Post("/test", func(c *fiber.Ctx) error {
		return Accepted(c, map[string]string{"status": "processing"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func TestPartialContent(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return PartialContent(c, map[string]any{"items": []string{"a", "b"}})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusPartialContent, resp.StatusCode)
}

func TestRangeNotSatisfiable(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return RangeNotSatisfiable(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, resp.StatusCode)
}

func TestNotFound(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return NotFound(c, "NOT001", "Not Found", "Resource not found")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "NOT001", body["code"])
}

func TestConflict(t *testing.T) {
	app := setupTestApp()
	app.Post("/test", func(c *fiber.Ctx) error {
		return Conflict(c, "CONF001", "Conflict", "Resource already exists")
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestNotImplemented(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return NotImplemented(c, "Feature not available")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
}

func TestUnprocessableEntity(t *testing.T) {
	app := setupTestApp()
	app.Post("/test", func(c *fiber.Ctx) error {
		return UnprocessableEntity(c, "UNP001", "Unprocessable", "Invalid entity")
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestInternalServerError(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return InternalServerError(c, "INT001", "Internal Error", "Something went wrong")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestJSONResponseError(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return JSONResponseError(c, pkg.ResponseError{
			Code:    "400",
			Title:   "Bad Request",
			Message: "Invalid input",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestJSONResponse(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return JSONResponse(c, http.StatusTeapot, map[string]string{"tea": "ready"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusTeapot, resp.StatusCode)
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/ -run "TestUnauthorized|TestForbidden|TestBadRequest|TestCreated|TestOK|TestNoContent|TestAccepted|TestPartialContent|TestRangeNotSatisfiable|TestNotFound|TestConflict|TestNotImplemented|TestUnprocessableEntity|TestInternalServerError|TestJSONResponse"`

**Expected output:**
```
=== RUN   TestUnauthorized
...
PASS
```

**If Task Fails:**
1. Check that response.go functions match expected signatures
2. Rollback: `git checkout -- pkg/net/http/response_test.go`

---

### Task 2.2: Verify Phase 2 Coverage and Commit

**Step 1: Run coverage check**

Run: `go test -cover /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/`

**Expected output:** Coverage should be 80%+ (up from 60.8%)

**Step 2: Commit Phase 2 changes**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/response_test.go
git commit -m "$(cat <<'EOF'
test(http): add comprehensive tests for HTTP response helpers

Add tests for all response helper functions:
- Unauthorized, Forbidden, BadRequest
- Created, OK, NoContent, Accepted
- PartialContent, RangeNotSatisfiable
- NotFound, Conflict, NotImplemented
- UnprocessableEntity, InternalServerError
- JSONResponseError, JSONResponse

Coverage improvement: 60.8% -> 80%+
EOF
)"
```

---

## Phase 3: pkg/mmodel Package (20.4% -> 60%+)

The mmodel package contains primarily DTOs (data structures) with minimal testable logic. We'll focus on files that have methods worth testing.

---

### Task 3.1: Add tests for status.go StatusCode validation

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/status_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/status.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/status.go`

**Step 1: Read the current status_test.go to understand existing tests**

Run: `cat /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/status_test.go`

**Step 2: Examine status.go for additional testable functions**

Run: `cat /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/status.go`

**Step 3: Add additional test cases if needed (depends on what's in status.go)**

This task requires examining the actual file content. If no additional testable methods exist, this task is complete.

---

### Task 3.2: Add tests for account.go functions

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account.go`

**Step 1: Read account.go to identify testable functions beyond what's currently tested**

Run: `cat /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account.go | head -200`

**Step 2: Read current account_test.go**

Run: `cat /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account_test.go`

**Step 3: Add tests for any uncovered methods (implementation depends on actual file content)**

---

### Task 3.3: Run Code Review for Phase 2-3

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously
   - Wait for all to complete

2. **Handle findings by severity (MANDATORY):**
   - Critical/High/Medium: Fix immediately
   - Low: Add `TODO(review):` comments

---

## Phase 4: components/transaction Package (42.9% -> 70%+)

The transaction component has significant business logic that needs additional test coverage.

---

### Task 4.1: Review existing transaction service tests and identify gaps

**Files:**
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/`

**Step 1: List all source files and test files**

Run: `ls -la /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/`

**Step 2: Identify files without corresponding tests**

Compare source files (*.go excluding *_test.go) with test files (*_test.go) to find gaps.

**Step 3: Document gaps for subsequent tasks**

---

### Task 4.2: Add missing tests for transaction command handlers

**Note:** This task requires examination of the actual transaction command handlers to identify specific untested functions. The implementation will follow the same table-driven test pattern as the existing tests.

**Pattern to follow:**
```go
func TestSomeHandler(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockRepo := repository.NewMockRepository(ctrl)

    uc := &UseCase{
        Repo: mockRepo,
    }

    testCases := []struct {
        name        string
        input       *SomeInput
        mockSetup   func()
        expectedErr error
        expected    *SomeOutput
    }{
        // Test cases here
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            tc.mockSetup()
            result, err := uc.SomeMethod(context.Background(), tc.input)
            // Assertions
        })
    }
}
```

---

### Task 4.3: Verify Transaction Coverage and Commit

**Step 1: Run coverage check**

Run: `go test -cover ./components/transaction/...`

**Expected output:** Coverage for services/command should be 70%+

**Step 2: Commit Phase 4 changes**

```bash
git add components/transaction/
git commit -m "$(cat <<'EOF'
test(transaction): improve unit test coverage for command handlers

Add comprehensive tests for uncovered transaction command handlers
following table-driven test patterns with gomock.

Coverage improvement: 42.9% -> 70%+
EOF
)"
```

---

## Phase 5: Final Verification and Cleanup

---

### Task 5.1: Run Full Coverage Report

**Step 1: Generate comprehensive coverage report**

Run: `go test -cover ./pkg/... ./components/... -coverprofile=coverage.out`

**Step 2: View coverage summary**

Run: `go tool cover -func=coverage.out | grep total`

**Expected output:** Total coverage should be 80%+

**Step 3: Generate HTML report for detailed analysis**

Run: `go tool cover -html=coverage.out -o coverage.html`

---

### Task 5.2: Final Code Review

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - Review all changes made during this plan

2. **Ensure all tests pass:**

Run: `go test -race -count=1 ./pkg/... ./components/...`

**Expected output:** All tests pass

---

### Task 5.3: Create Final Commit

```bash
git add -A
git commit -m "$(cat <<'EOF'
test: comprehensive unit test coverage improvement

Improve overall unit test coverage to 80%+ by adding tests for:
- pkg/utils: ptr, env, cache, jitter helpers
- pkg/net/http: response helper functions
- pkg/mmodel: model validation functions
- components/transaction: command handlers

See docs/plans/2025-12-28-unit-test-coverage-improvement.md for details.
EOF
)"
```

---

## Verification Checklist

Before considering this plan complete:

- [ ] All Phase 1 tests pass (pkg/utils)
- [ ] All Phase 2 tests pass (pkg/net/http)
- [ ] All Phase 3 tests pass (pkg/mmodel)
- [ ] All Phase 4 tests pass (components/transaction)
- [ ] Overall coverage is 80%+
- [ ] Code review completed with no Critical/High/Medium issues
- [ ] All commits follow conventional commit format
- [ ] No tests use real external services (all use mocks)

---

## Notes for Executors

1. **Existing Test Patterns:** Follow the table-driven test pattern with testify assertions seen throughout the codebase
2. **Mock Generation:** Use gomock with the existing mock files in the codebase
3. **Test Naming:** Use descriptive names like `TestFunctionName_Scenario_ExpectedBehavior`
4. **Coverage Focus:** Prioritize business logic (services) over pure data structures (DTOs)
5. **ANTLR Code:** The gold/parser package is auto-generated ANTLR code - skip testing
6. **Database Adapters:** PostgreSQL adapters use integration tests with testcontainers, not unit tests
