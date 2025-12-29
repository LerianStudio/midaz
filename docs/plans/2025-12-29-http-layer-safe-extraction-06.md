# HTTP Layer Safe Extraction Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add safe extraction helpers for string parameters and headers in HTTP handlers, applying assertions consistently across all handlers to catch middleware wiring bugs early.

**Architecture:** Extend existing `pkg/net/http/locals.go` with new safe extraction helpers (`LocalString`, `LocalHeader`, `LocalHeaderUUID`) that assert on empty/invalid values. Add domain-specific predicates to `pkg/assert/predicates.go`. Apply these helpers consistently across CRM, Transaction, and Onboarding handlers.

**Tech Stack:** Go 1.21+, Fiber v2, testify for assertions

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Tools: `go test`, `golangci-lint`
- Access: Write access to repository
- State: Clean working tree on current branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version          # Expected: go1.21+
git status          # Expected: clean or known changes
go test ./pkg/net/http/... -count=1  # Expected: all pass
go test ./pkg/assert/... -count=1    # Expected: all pass
```

## Historical Precedent

**Query:** "http safe extraction headers parameters validation assertions"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Task 1: Add Domain-Specific Predicates to Assert Package

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go:107` (append after existing functions)
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert_test.go` (append tests)

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `pkg/assert/predicates.go`, `pkg/assert/assert_test.go`

**Step 1: Write the failing tests**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert_test.go`:

```go
// TestNotEmptyString tests the NotEmptyString predicate.
func TestNotEmptyString(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		expected bool
	}{
		{"non-empty string", "hello", true},
		{"string with spaces", "  hello  ", true},
		{"single space", " ", false},
		{"multiple spaces", "   ", false},
		{"tab", "\t", false},
		{"newline", "\n", false},
		{"empty string", "", false},
		{"mixed whitespace", " \t\n ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, NotEmptyString(tt.s))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestNotEmptyString /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/...`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./assert_test.go:XXX:XX: undefined: NotEmptyString
FAIL    github.com/LerianStudio/midaz/v3/pkg/assert [build failed]
```

**If you see different error:** Check the test file path and function name

**Step 3: Write minimal implementation**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go` after line 107:

```go

// NotEmptyString returns true if s is not empty and not just whitespace.
// This is stricter than NotEmpty which only checks for empty string.
//
// Example:
//
//	assert.That(assert.NotEmptyString(alias), "alias must not be empty", "alias", alias)
func NotEmptyString(s string) bool {
	return strings.TrimSpace(s) != ""
}
```

**Step 4: Add strings import if not present**

Check if `strings` is already imported in `predicates.go`. If not, add to imports:

```go
import (
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)
```

**Step 5: Run test to verify it passes**

Run: `go test -v -run TestNotEmptyString /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/...`

**Expected output:**
```
=== RUN   TestNotEmptyString
=== RUN   TestNotEmptyString/non-empty_string
=== RUN   TestNotEmptyString/string_with_spaces
...
--- PASS: TestNotEmptyString (0.00s)
PASS
```

**Step 6: Commit**

```bash
git add pkg/assert/predicates.go pkg/assert/assert_test.go
git commit -m "$(cat <<'EOF'
feat(assert): add NotEmptyString predicate for strict whitespace checking

Adds predicate that returns false for strings containing only whitespace,
unlike NotEmpty which only checks for empty string. Useful for validating
user inputs like aliases and codes that shouldn't be blank.
EOF
)"
```

**If Task Fails:**

1. **Test won't compile:**
   - Check: import statement for `strings` package
   - Fix: Add missing import
   - Rollback: `git checkout -- pkg/assert/`

2. **Test fails:**
   - Run: Check which case failed
   - Fix: Adjust predicate logic
   - Rollback: `git checkout -- pkg/assert/predicates.go`

---

## Task 2: Add truncateValue Helper to HTTP Package

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals.go:122` (append after existing functions)

**Prerequisites:**
- Task 1 completed
- Files must exist: `pkg/net/http/locals.go`

**Step 1: Add helper function**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals.go` after line 122:

```go

const maxValueLength = 100 // Truncate values longer than this for logging

// truncateValue truncates long values for logging safety.
// This prevents log bloat and reduces risk of sensitive data exposure.
func truncateValue(v string) string {
	if len(v) <= maxValueLength {
		return v
	}

	return v[:maxValueLength] + "..."
}
```

**Step 2: Verify build**

Run: `go build ./pkg/net/http/...`

**Expected output:** (no output means success)

**Step 3: Commit**

```bash
git add pkg/net/http/locals.go
git commit -m "$(cat <<'EOF'
feat(http): add truncateValue helper for safe logging

Truncates long string values to prevent log bloat and sensitive data
exposure in assertion messages. Used by safe extraction helpers.
EOF
)"
```

---

## Task 3: Add LocalString Safe Extraction Helper

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals.go` (append after truncateValue)
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals_test.go` (append tests)

**Prerequisites:**
- Task 2 completed
- Files must exist: `pkg/net/http/locals.go`, `pkg/net/http/locals_test.go`

**Step 1: Write the failing test**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals_test.go`:

```go

func TestLocalString_ValidParam(t *testing.T) {
	app := fiber.New()

	app.Get("/test/:alias", func(c *fiber.Ctx) error {
		result := LocalString(c, "alias")
		assert.Equal(t, "@person1", result)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test/@person1", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestLocalString_EmptyParam_Panics(t *testing.T) {
	app := fiber.New()

	// Route with optional param that won't be set
	app.Get("/test", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic but none occurred")

			panicMsg, ok := r.(string)
			require.True(t, ok, "panic value should be string")

			assert.Contains(t, panicMsg, "assertion failed: path parameter must not be empty")
			assert.Contains(t, panicMsg, "param=missing_param")
			assert.Contains(t, panicMsg, "path=/test")
			assert.Contains(t, panicMsg, "method=GET")
		}()

		LocalString(c, "missing_param")
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run 'TestLocalString' /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/...`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/net/http [github.com/LerianStudio/midaz/v3/pkg/net/http.test]
./locals_test.go:XXX:XX: undefined: LocalString
FAIL
```

**Step 3: Write implementation**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals.go` after truncateValue:

```go

// LocalString extracts a string path parameter and asserts it's not empty.
// Use this for required string path parameters like alias, code, etc.
// Panics with rich context if the parameter is empty, making middleware
// wiring bugs immediately visible.
//
// Example:
//
//	alias := http.LocalString(c, "alias")
func LocalString(c *fiber.Ctx, paramName string) string {
	val := c.Params(paramName)
	assert.NotEmpty(val, "path parameter must not be empty",
		"param", paramName,
		"path", c.Path(),
		"method", c.Method())

	return val
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -v -run 'TestLocalString' /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/...`

**Expected output:**
```
=== RUN   TestLocalString_ValidParam
--- PASS: TestLocalString_ValidParam (0.00s)
=== RUN   TestLocalString_EmptyParam_Panics
--- PASS: TestLocalString_EmptyParam_Panics (0.00s)
PASS
```

**Step 5: Commit**

```bash
git add pkg/net/http/locals.go pkg/net/http/locals_test.go
git commit -m "$(cat <<'EOF'
feat(http): add LocalString safe extraction helper for path params

Extracts string path parameters with assertion that they're not empty.
Catches middleware wiring bugs early with rich context in panic messages.
EOF
)"
```

**If Task Fails:**

1. **Test won't compile:**
   - Check: Function signature matches test expectations
   - Fix: Ensure LocalString takes (*fiber.Ctx, string) and returns string
   - Rollback: `git checkout -- pkg/net/http/`

---

## Task 4: Add LocalHeader Safe Extraction Helper

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals.go` (append after LocalString)
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals_test.go` (append tests)

**Prerequisites:**
- Task 3 completed

**Step 1: Write the failing test**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals_test.go`:

```go

func TestLocalHeader_ValidHeader(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		result := LocalHeader(c, "Authorization")
		assert.Equal(t, "Bearer token123", result)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer token123")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestLocalHeader_MissingHeader_Panics(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic but none occurred")

			panicMsg, ok := r.(string)
			require.True(t, ok, "panic value should be string")

			assert.Contains(t, panicMsg, "assertion failed: required header missing")
			assert.Contains(t, panicMsg, "header=X-Custom-Header")
			assert.Contains(t, panicMsg, "path=/test")
			assert.Contains(t, panicMsg, "method=GET")
		}()

		LocalHeader(c, "X-Custom-Header")
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run 'TestLocalHeader' /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/...`

**Expected output:**
```
./locals_test.go:XXX:XX: undefined: LocalHeader
FAIL
```

**Step 3: Write implementation**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals.go` after LocalString:

```go

// LocalHeader extracts a header value and asserts it's not empty.
// Use this for required headers like Authorization.
// Panics with rich context if the header is missing or empty.
//
// Example:
//
//	token := http.LocalHeader(c, "Authorization")
func LocalHeader(c *fiber.Ctx, headerName string) string {
	val := c.Get(headerName)
	assert.NotEmpty(val, "required header missing",
		"header", headerName,
		"path", c.Path(),
		"method", c.Method())

	return val
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -v -run 'TestLocalHeader' /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/...`

**Expected output:**
```
=== RUN   TestLocalHeader_ValidHeader
--- PASS: TestLocalHeader_ValidHeader (0.00s)
=== RUN   TestLocalHeader_MissingHeader_Panics
--- PASS: TestLocalHeader_MissingHeader_Panics (0.00s)
PASS
```

**Step 5: Commit**

```bash
git add pkg/net/http/locals.go pkg/net/http/locals_test.go
git commit -m "$(cat <<'EOF'
feat(http): add LocalHeader safe extraction helper for required headers

Extracts HTTP header values with assertion that they're not empty.
Catches middleware wiring bugs early with rich context in panic messages.
EOF
)"
```

---

## Task 5: Add LocalHeaderUUID Safe Extraction Helper

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals.go` (append after LocalHeader)
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals_test.go` (append tests)

**Prerequisites:**
- Task 4 completed

**Step 1: Write the failing test**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals_test.go`:

```go

func TestLocalHeaderUUID_ValidUUID(t *testing.T) {
	app := fiber.New()
	testUUID := "550e8400-e29b-41d4-a716-446655440000"

	app.Get("/test", func(c *fiber.Ctx) error {
		result := LocalHeaderUUID(c, "X-Organization-Id")
		assert.Equal(t, testUUID, result)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Organization-Id", testUUID)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestLocalHeaderUUID_InvalidUUID_Panics(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic but none occurred")

			panicMsg, ok := r.(string)
			require.True(t, ok, "panic value should be string")

			assert.Contains(t, panicMsg, "assertion failed: header must be valid UUID")
			assert.Contains(t, panicMsg, "header=X-Organization-Id")
			assert.Contains(t, panicMsg, "value=not-a-uuid")
			assert.Contains(t, panicMsg, "path=/test")
		}()

		LocalHeaderUUID(c, "X-Organization-Id")
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Organization-Id", "not-a-uuid")
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}

func TestLocalHeaderUUID_MissingHeader_Panics(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic but none occurred")

			panicMsg, ok := r.(string)
			require.True(t, ok, "panic value should be string")

			assert.Contains(t, panicMsg, "assertion failed: header must be valid UUID")
			assert.Contains(t, panicMsg, "header=X-Organization-Id")
		}()

		LocalHeaderUUID(c, "X-Organization-Id")
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	// No header set
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run 'TestLocalHeaderUUID' /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/...`

**Expected output:**
```
./locals_test.go:XXX:XX: undefined: LocalHeaderUUID
FAIL
```

**Step 3: Write implementation**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals.go` after LocalHeader:

```go

// LocalHeaderUUID extracts a header value and asserts it's a valid UUID.
// Use this for headers that must contain UUIDs like X-Organization-Id.
// Panics with rich context if the header is missing or not a valid UUID.
//
// Example:
//
//	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
func LocalHeaderUUID(c *fiber.Ctx, headerName string) string {
	val := c.Get(headerName)
	assert.That(assert.ValidUUID(val),
		"header must be valid UUID",
		"header", headerName,
		"value", truncateValue(val),
		"path", c.Path(),
		"method", c.Method())

	return val
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -v -run 'TestLocalHeaderUUID' /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/...`

**Expected output:**
```
=== RUN   TestLocalHeaderUUID_ValidUUID
--- PASS: TestLocalHeaderUUID_ValidUUID (0.00s)
=== RUN   TestLocalHeaderUUID_InvalidUUID_Panics
--- PASS: TestLocalHeaderUUID_InvalidUUID_Panics (0.00s)
=== RUN   TestLocalHeaderUUID_MissingHeader_Panics
--- PASS: TestLocalHeaderUUID_MissingHeader_Panics (0.00s)
PASS
```

**Step 5: Commit**

```bash
git add pkg/net/http/locals.go pkg/net/http/locals_test.go
git commit -m "$(cat <<'EOF'
feat(http): add LocalHeaderUUID safe extraction helper

Extracts HTTP header values with UUID validation. Returns the string
value after asserting it's a valid UUID. Used for headers like
X-Organization-Id that must be valid UUIDs.
EOF
)"
```

---

## Task 6: Run Full Test Suite for HTTP Package

**Prerequisites:**
- Tasks 1-5 completed

**Step 1: Run all HTTP package tests**

Run: `go test -v -count=1 /Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/...`

**Expected output:**
```
=== RUN   TestLocalUUID_ValidUUID
--- PASS: TestLocalUUID_ValidUUID (0.00s)
...
=== RUN   TestLocalString_ValidParam
--- PASS: TestLocalString_ValidParam (0.00s)
=== RUN   TestLocalString_EmptyParam_Panics
--- PASS: TestLocalString_EmptyParam_Panics (0.00s)
=== RUN   TestLocalHeader_ValidHeader
--- PASS: TestLocalHeader_ValidHeader (0.00s)
=== RUN   TestLocalHeader_MissingHeader_Panics
--- PASS: TestLocalHeader_MissingHeader_Panics (0.00s)
=== RUN   TestLocalHeaderUUID_ValidUUID
--- PASS: TestLocalHeaderUUID_ValidUUID (0.00s)
...
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/net/http
```

**Step 2: Run all assert package tests**

Run: `go test -v -count=1 /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/...`

**Expected output:**
```
=== RUN   TestNotEmptyString
--- PASS: TestNotEmptyString (0.00s)
...
PASS
```

**If Task Fails:**

1. **Tests fail:**
   - Check: Which specific test failed
   - Fix: Review implementation of that specific helper
   - Run: Individual test with `-v` flag for details

---

## Task 7: Apply LocalHeaderUUID to CRM Holder Handler

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/http/in/holder.go`

**Prerequisites:**
- Tasks 1-6 completed
- Files must exist: `holder.go`

**Step 1: Update CreateHolder (line 45)**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/http/in/holder.go`, find:

```go
	payload := http.Payload[*mmodel.CreateHolderInput](c, p)
	organizationID := c.Get("X-Organization-Id")
```

Replace with:

```go
	payload := http.Payload[*mmodel.CreateHolderInput](c, p)
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 2: Update GetHolderByID (line 101)**

Find:

```go
	id := http.LocalUUID(c, "id")
	organizationID := c.Get("X-Organization-Id")
```

Replace with:

```go
	id := http.LocalUUID(c, "id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 3: Update UpdateHolder (line 158)**

Find:

```go
	id := http.LocalUUID(c, "id")
	organizationID := c.Get("X-Organization-Id")
```

Replace with:

```go
	id := http.LocalUUID(c, "id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 4: Update DeleteHolderByID (line 224)**

Find:

```go
	id := http.LocalUUID(c, "id")
	organizationID := c.Get("X-Organization-Id")
```

Replace with:

```go
	id := http.LocalUUID(c, "id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 5: Update GetAllHolders (line 302)**

Find:

```go
	organizationID := c.Get("X-Organization-Id")
```

Replace with:

```go
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 6: Verify build**

Run: `go build ./components/crm/...`

**Expected output:** (no output means success)

**Step 7: Commit**

```bash
git add components/crm/internal/adapters/http/in/holder.go
git commit -m "$(cat <<'EOF'
refactor(crm): apply LocalHeaderUUID to holder handlers

Replace direct c.Get("X-Organization-Id") calls with LocalHeaderUUID
for consistent UUID validation. Catches middleware wiring bugs early.
EOF
)"
```

---

## Task 8: Apply LocalHeaderUUID to CRM Alias Handler

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/http/in/alias.go`

**Prerequisites:**
- Task 7 completed

**Note:** CreateAlias (line 50-57) already has an assertion. We update the other 4 handlers.

**Step 1: Update GetAliasByID (line 116)**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/http/in/alias.go`, find:

```go
	id := http.LocalUUID(c, "id")
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := c.Get("X-Organization-Id")
```

Replace with:

```go
	id := http.LocalUUID(c, "id")
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 2: Update UpdateAlias (line 176)**

Find:

```go
	id := http.LocalUUID(c, "id")
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := c.Get("X-Organization-Id")
```

Replace with:

```go
	id := http.LocalUUID(c, "id")
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 3: Update DeleteAliasByID (line 245)**

Find:

```go
	id := http.LocalUUID(c, "id")
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := c.Get("X-Organization-Id")
```

Replace with:

```go
	id := http.LocalUUID(c, "id")
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 4: Update GetAllAliases (line 333)**

Find:

```go
	organizationID := c.Get("X-Organization-Id")
```

Replace with:

```go
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 5: Remove redundant assertion from CreateAlias**

In CreateAlias function (lines 52-57), remove the now-redundant manual assertion since LocalHeaderUUID handles this:

Find:

```go
	organizationID := c.Get("X-Organization-Id")

	// organizationID header should be validated by middleware before reaching handler.
	// If we get here with invalid UUID, it indicates middleware misconfiguration.
	assert.That(assert.ValidUUID(organizationID),
		"X-Organization-Id header must be valid UUID - check middleware configuration",
		"handler", "CreateAlias",
		"organizationID", organizationID)
```

Replace with:

```go
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
```

**Step 6: Remove unused assert import if present**

Check if `"github.com/LerianStudio/midaz/v3/pkg/assert"` import is still used in alias.go. If not used elsewhere in the file, remove it.

**Step 7: Verify build**

Run: `go build ./components/crm/...`

**Expected output:** (no output means success)

**Step 8: Commit**

```bash
git add components/crm/internal/adapters/http/in/alias.go
git commit -m "$(cat <<'EOF'
refactor(crm): apply LocalHeaderUUID to alias handlers

Replace direct c.Get("X-Organization-Id") calls and manual assertions
with LocalHeaderUUID for consistent validation across all handlers.
EOF
)"
```

---

## Task 9: Apply LocalString to Transaction Balance Handler

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/balance.go`

**Prerequisites:**
- Task 8 completed

**Step 1: Update GetBalancesByAlias (line 420)**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/balance.go`, find:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	alias := c.Params("alias")
```

Replace with:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	alias := http.LocalString(c, "alias")
```

**Step 2: Update GetBalancesExternalByCode (line 499)**

Find:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	code := c.Params("code")
```

Replace with:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	code := http.LocalString(c, "code")
```

**Step 3: Verify build**

Run: `go build ./components/transaction/...`

**Expected output:** (no output means success)

**Step 4: Commit**

```bash
git add components/transaction/internal/adapters/http/in/balance.go
git commit -m "$(cat <<'EOF'
refactor(transaction): apply LocalString to balance handlers

Replace direct c.Params() calls with LocalString for alias and code
parameters to ensure non-empty path parameters.
EOF
)"
```

---

## Task 10: Apply LocalString to Transaction AssetRate Handler

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/assetrate.go`

**Prerequisites:**
- Task 9 completed

**Step 1: Update GetAllAssetRatesByAssetCode (line 184)**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/assetrate.go`, find:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	assetCode := c.Params("asset_code")
```

Replace with:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	assetCode := http.LocalString(c, "asset_code")
```

**Step 2: Verify build**

Run: `go build ./components/transaction/...`

**Expected output:** (no output means success)

**Step 3: Commit**

```bash
git add components/transaction/internal/adapters/http/in/assetrate.go
git commit -m "$(cat <<'EOF'
refactor(transaction): apply LocalString to assetrate handler

Replace direct c.Params("asset_code") with LocalString for consistent
path parameter validation.
EOF
)"
```

---

## Task 11: Apply LocalString to Onboarding Account Handler

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/http/in/account.go`

**Prerequisites:**
- Task 10 completed

**Step 1: Update GetAccountExternalByCode (line 295)**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/http/in/account.go`, find:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	code := c.Params("code")
```

Replace with:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	code := http.LocalString(c, "code")
```

**Step 2: Update GetAccountByAlias (line 350)**

Find:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	alias := c.Params("alias")
```

Replace with:

```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	alias := http.LocalString(c, "alias")
```

**Step 3: Verify build**

Run: `go build ./components/onboarding/...`

**Expected output:** (no output means success)

**Step 4: Commit**

```bash
git add components/onboarding/internal/adapters/http/in/account.go
git commit -m "$(cat <<'EOF'
refactor(onboarding): apply LocalString to account handlers

Replace direct c.Params() calls with LocalString for code and alias
parameters to ensure non-empty path parameters.
EOF
)"
```

---

## Task 12: Run Full Component Tests

**Prerequisites:**
- Tasks 7-11 completed

**Step 1: Run CRM component tests**

Run: `go test -v -count=1 ./components/crm/... -short`

**Expected output:**
```
ok      github.com/LerianStudio/midaz/v3/components/crm/...
```

**Step 2: Run Transaction component tests**

Run: `go test -v -count=1 ./components/transaction/... -short`

**Expected output:**
```
ok      github.com/LerianStudio/midaz/v3/components/transaction/...
```

**Step 3: Run Onboarding component tests**

Run: `go test -v -count=1 ./components/onboarding/... -short`

**Expected output:**
```
ok      github.com/LerianStudio/midaz/v3/components/onboarding/...
```

**If Task Fails:**

1. **Tests fail:**
   - Run: Individual test to see which fails
   - Check: Handler changes don't break existing functionality
   - Rollback: `git revert HEAD` if needed

---

## Task 13: Run Code Review

**Prerequisites:**
- Tasks 1-12 completed

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
- Format: `TODO(review): [Issue description] (reported by [reviewer] on [date], severity: Low)`
- This tracks tech debt for future resolution

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location
- Format: `FIXME(nitpick): [Issue description] (reported by [reviewer] on [date], severity: Cosmetic)`
- Low-priority improvements tracked inline

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

---

## Task 14: Final Verification and Documentation

**Prerequisites:**
- Task 13 completed with all issues resolved

**Step 1: Run full test suite**

Run: `go test -count=1 ./...`

**Expected output:**
```
ok      github.com/LerianStudio/midaz/v3/pkg/assert
ok      github.com/LerianStudio/midaz/v3/pkg/net/http
ok      github.com/LerianStudio/midaz/v3/components/crm/...
ok      github.com/LerianStudio/midaz/v3/components/transaction/...
ok      github.com/LerianStudio/midaz/v3/components/onboarding/...
```

**Step 2: Run linter**

Run: `golangci-lint run ./pkg/net/http/... ./pkg/assert/... ./components/crm/... ./components/transaction/... ./components/onboarding/...`

**Expected output:** (no output or only warnings means success)

**Step 3: Verify git status**

Run: `git status`

**Expected output:** Shows all changes committed

**Step 4: Final commit if needed**

If there are uncommitted changes from code review fixes:

```bash
git add -A
git commit -m "$(cat <<'EOF'
fix: address code review feedback for HTTP safe extraction

Apply fixes from code review including:
- [List specific fixes made]
EOF
)"
```

---

## Summary

This plan adds:
- 1 new predicate in `pkg/assert/predicates.go` (NotEmptyString)
- 4 new helpers in `pkg/net/http/locals.go` (truncateValue, LocalString, LocalHeader, LocalHeaderUUID)
- ~20 assertion applications across CRM, Transaction, and Onboarding handlers

**Expected Outcome:** HTTP layer catches middleware wiring bugs immediately with clear context instead of propagating empty strings to database layer. All handlers now validate required path parameters and headers consistently.

**Files Modified:**
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert_test.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/locals_test.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/http/in/holder.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/http/in/alias.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/balance.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/assetrate.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/http/in/account.go`
