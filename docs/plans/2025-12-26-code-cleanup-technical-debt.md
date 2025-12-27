# Code Cleanup & Technical Debt Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Address 8 technical debt items across CRM, model validation, lexer, error handling, and gRPC components

**Architecture:** Modular cleanup with dependencies identified. Items are grouped by area: CRM/MongoDB (1), Globalization (1), ANTLR/Lexer (2 - both DEFER), Error Typing (3), gRPC (1 - future work).

**Tech Stack:** Go 1.21+, ANTLR 4.13.1, Fiber HTTP framework, gRPC

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Tools: `go`, `golangci-lint`, `make`
- Access: Repository write access
- State: Clean working tree on branch `fix/fred-several-ones-dec-13-2025`

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version          # Expected: go version go1.21+
git status          # Expected: clean working tree
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...  # Expected: success
```

---

## Summary of Items

| # | File | Line | Category | Priority | Action |
|---|------|------|----------|----------|--------|
| 1 | `components/crm/internal/adapters/mongodb/holder/holder.go` | 288 | CRM/Address | HIGH | Investigate & resolve |
| 2 | `pkg/mmodel/holder.go` | 24 | Globalization | MEDIUM | Document & plan strategy |
| 3 | `pkg/gold/parser/transaction_lexer.go` | 21 | ANTLR/Lexer | LOW | **DEFER** (auto-generated) |
| 4 | `pkg/gold/parser/transaction_lexer.go` | 229 | ANTLR/Lexer | LOW | **DEFER** (auto-generated) |
| 5 | `pkg/net/http/errors.go` | 15 | Error Typing | HIGH | Remove after analysis |
| 6 | `pkg/net/http/errors.go` | 105 | Error Typing | HIGH | Remove after analysis |
| 7 | `pkg/net/http/errors.go` | 144 | Error Typing | HIGH | Remove after analysis |
| 8 | `components/transaction/internal/adapters/grpc/in/routes.go` | 27 | gRPC Phase 2 | LOW | Document as future work |

---

## Batch 1: CRM Address Description Field (Item #1)

### Task 1.1: Investigate Address Description Field Usage

**Files:**
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/mongodb/holder/holder.go:276-290`
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/organization.go:213-244`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: Both files listed above

**Step 1: Examine Address model definition**

The `Address` struct is defined in `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/organization.go:213-244`:

```go
type Address struct {
    Line1   string  `json:"line1" example:"123 Financial Avenue" maxLength:"256"`
    Line2   *string `json:"line2" example:"Suite 1500" maxLength:"256"`
    ZipCode string  `json:"zipCode" example:"10001" maxLength:"20"`
    City    string  `json:"city" example:"New York" maxLength:"100"`
    State   string  `json:"state" example:"NY" maxLength:"100"`
    Country string  `json:"country" example:"US" minLength:"2" maxLength:"2"`
}
```

**Observation:** The `Address` struct does NOT have a `Description` field. The TODO at line 288 comments out code that attempts to map a non-existent field.

**Step 2: Verify the MongoDB model**

The `AddressMongoDBModel` at `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/mongodb/holder/holder.go:35-44`:

```go
type AddressMongoDBModel struct {
    Line1       *string `bson:"line_1,omitempty"`
    Line2       *string `bson:"line_2,omitempty"`
    ZipCode     *string `bson:"zip_code,omitempty"`
    City        *string `bson:"city,omitempty"`
    State       *string `bson:"state,omitempty"`
    Country     *string `bson:"country,omitempty"`
    Description *string `bson:"description,omitempty"`  // <-- EXISTS in MongoDB model
    IsPrimary   *bool   `bson:"is_primary,omitempty"`
}
```

**Conclusion:** There's a mismatch:
- `Address` (API model) has NO `Description` field
- `AddressMongoDBModel` (DB model) HAS a `Description` field

**Decision Required:** Should `Description` be:
a) Added to the API model `Address` struct?
b) Removed from `AddressMongoDBModel`?
c) Left as unused DB field for future use?

**Step 3: Document investigation result**

Run: `grep -rn "Description" /Users/fredamaral/repos/lerianstudio/midaz/components/crm/ --include="*.go" | head -20`

**Expected output:** Shows all Description field references in CRM component

---

### Task 1.2: Remove Dead Code or Align Models

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/mongodb/holder/holder.go:288`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/mongodb/holder/holder.go:503`

**Prerequisites:**
- Task 1.1 complete
- Decision made on Description field

**Option A: Remove unused Description field (RECOMMENDED if not needed)**

**Step 1: Remove from AddressMongoDBModel struct**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/mongodb/holder/holder.go:35-44`:

```go
type AddressMongoDBModel struct {
	Line1     *string `bson:"line_1,omitempty"`
	Line2     *string `bson:"line_2,omitempty"`
	ZipCode   *string `bson:"zip_code,omitempty"`
	City      *string `bson:"city,omitempty"`
	State     *string `bson:"state,omitempty"`
	Country   *string `bson:"country,omitempty"`
	IsPrimary *bool   `bson:"is_primary,omitempty"`
}
```

**Step 2: Remove commented-out lines with TODO**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/mongodb/holder/holder.go:281-289`:

Before:
```go
	return &AddressMongoDBModel{
		Line1:   &a.Line1,
		Line2:   a.Line2,
		ZipCode: &a.ZipCode,
		City:    &a.City,
		State:   &a.State,
		Country: &a.Country,
		// Description: &a.Description, // TODO: Check if this is needed
	}
```

After:
```go
	return &AddressMongoDBModel{
		Line1:   &a.Line1,
		Line2:   a.Line2,
		ZipCode: &a.ZipCode,
		City:    &a.City,
		State:   &a.State,
		Country: &a.Country,
	}
```

**Step 3: Remove from mapAddressToEntity**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/mongodb/holder/holder.go:496-504`:

Before:
```go
	return &mmodel.Address{
		Line1:   line1,
		Line2:   a.Line2,
		ZipCode: zipCode,
		City:    city,
		State:   state,
		Country: country,
		// Description: a.Description,
	}
```

After:
```go
	return &mmodel.Address{
		Line1:   line1,
		Line2:   a.Line2,
		ZipCode: zipCode,
		City:    city,
		State:   state,
		Country: country,
	}
```

**Step 4: Verify changes compile**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/crm/...`

**Expected output:** Build succeeds with no errors

**Step 5: Run tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/crm/... -v`

**Expected output:** All tests pass

**If Task Fails:**

1. **Build fails:**
   - Check: Missing imports or syntax errors
   - Fix: Review edits for typos
   - Rollback: `git checkout -- components/crm/internal/adapters/mongodb/holder/holder.go`

2. **Tests fail:**
   - Check: Test expects Description field
   - Fix: Update test to remove Description expectations
   - Rollback: `git reset --hard HEAD`

---

## Batch 2: Globalization Strategy (Item #2)

### Task 2.1: Document Current CPF/CNPJ Validation

**Files:**
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder.go:24-27`
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/withBody.go:298,310-316`

**Prerequisites:**
- Tools: Go 1.21+

**Step 1: Review current validation**

The `Document` field uses `validate:"required,cpfcnpj"` tag:
- Only validates Brazilian CPF (11 digits) and CNPJ (14 digits)
- Error message: "must be a valid CPF or CNPJ"

**Step 2: Identify international document types needed**

Common international identifiers:
- **USA:** SSN (Social Security Number), EIN (Employer ID)
- **EU:** VAT ID, NIF (Tax ID varies by country)
- **UK:** NI Number (National Insurance)
- **International:** Passport number
- **Generic:** TIN (Tax Identification Number)

**Step 3: Create globalization tracking issue**

This requires a design decision and is beyond a simple code cleanup. Document as future work.

---

### Task 2.2: Update TODO Comment with Tracking Reference

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder.go:24-27`

**Prerequisites:**
- Task 2.1 complete

**Step 1: Update TODO comment**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder.go:23-27`:

Before:
```go
	// The holder's identification document.
	// TODO(globalization): This validation is restricted to Brazilian CPF/CNPJ formats.
	// Midaz should support international document types (SSN, NIF, TIN, etc.) for global usage.
	// See: https://github.com/LerianStudio/midaz/issues/XXX (tracking issue to be created)
	Document string `json:"document" validate:"required,cpfcnpj" example:"91315026015"`
```

After:
```go
	// The holder's identification document.
	// NOTE(globalization): Validation currently supports Brazilian CPF/CNPJ formats only.
	// Future enhancement: Support international document types (SSN, NIF, TIN, passport, etc.)
	// Strategy: Add `documentType` field to select validator, or detect format automatically.
	// Requires: Product decision on supported countries/formats, migration plan for existing data.
	Document string `json:"document" validate:"required,cpfcnpj" example:"91315026015"`
```

**Step 2: Verify changes compile**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mmodel/...`

**Expected output:** Build succeeds

**If Task Fails:**

1. **Syntax error:**
   - Check: Comment formatting
   - Rollback: `git checkout -- pkg/mmodel/holder.go`

---

## Batch 3: ANTLR Lexer TODOs (Items #3, #4) - DEFER

### Task 3.1: Document ANTLR Auto-Generated Code Decision

**Files:**
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/parser/transaction_lexer.go` (AUTO-GENERATED)
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/Transaction.g4`

**Prerequisites:**
- None

**Decision: DEFER - Do Not Modify**

**Rationale:**

1. **File is auto-generated:** Header states `// Code generated from Transaction.g4 by ANTLR 4.13.1. DO NOT EDIT.`

2. **TODOs are ANTLR template artifacts:**
   - Line 21: `// TODO: EOF string` - Placeholder in struct for EOF field
   - Line 229: `// TODO: l.EOF = antlr.TokenEOF` - Commented assignment

3. **Parser already defines EOF correctly:**
   ```go
   // pkg/gold/parser/transaction_parser.go:178
   TransactionParserEOF = antlr.TokenEOF
   ```

4. **Modifying would be overwritten:** Any manual edits would be lost on ANTLR regeneration

**Recommended Action:**
- Leave as-is
- If cleanup desired, modify ANTLR code generation templates (out of scope)
- These TODOs do NOT affect functionality

**No code changes required for Items #3 and #4.**

---

## Batch 4: Error Typing Diagnostic Removal (Items #5, #6, #7)

### Task 4.1: Analyze Diagnostic Code Purpose

**Files:**
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/errors.go:15-16,105-139,144-145`

**Prerequisites:**
- None

**Step 1: Understand current diagnostic code**

The diagnostic code serves to:
1. Log unknown errors that fall through to 500 responses
2. Help identify which error types need proper typing in `pkg/errors.go`
3. Track error chains for debugging

**Step 2: Determine if error typing is complete**

Review `/Users/fredamaral/repos/lerianstudio/midaz/pkg/errors.go` - This file defines typed errors:
- `EntityNotFoundError` - 404
- `EntityConflictError` - 409
- `ValidationError` - 400
- `UnprocessableOperationError` - 422
- `UnauthorizedError` - 401
- `ForbiddenError` - 403
- `FailedPreconditionError` - 422
- `InternalServerError` - 500
- `ValidationKnownFieldsError` - 400
- `ValidationUnknownFieldsError` - 400

**Step 3: Check error handling coverage**

The `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/errors.go` `WithError` function handles:
- `handleStandardErrors`: NotFound, Conflict, Validation, Unprocessable, FailedPrecondition, Unauthorized, Forbidden
- `handleSpecialErrors`: ValidationFieldsErrors, ResponseError, libCommons.Response, InternalServerError
- `handleUnknownError`: Catches everything else -> logs diagnostic + returns 500

**Criteria for removal:** The diagnostic code should be removed when:
1. All expected error types are properly typed in `pkg/errors.go`
2. No new untyped errors are appearing in logs
3. Error handling coverage is complete

---

### Task 4.2: Verify Error Typing Completeness

**Files:**
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/errors.go`
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/errors.go`

**Prerequisites:**
- Task 4.1 complete

**Step 1: Run application and check for diagnostic logs**

Run the application with test traffic and grep for diagnostic output:

```bash
# In one terminal, run the service (adjust as needed for your setup)
cd /Users/fredamaral/repos/lerianstudio/midaz && make run-transaction 2>&1 | grep -i "DIAGNOSTIC"
```

**Expected output if typing complete:** No `[DIAGNOSTIC] Unknown error falling through to 500` logs

**Expected output if typing incomplete:** Log entries showing untyped error types

**Step 2: Search codebase for untyped error returns**

Run: `grep -rn "return err$" /Users/fredamaral/repos/lerianstudio/midaz/components/ --include="*.go" | grep -v "_test.go" | head -30`

**Expected output:** List of locations where raw errors are returned (may need typing)

---

### Task 4.3: Remove Diagnostic Code

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/errors.go:15-16,104-139,143-145`

**Prerequisites:**
- Task 4.2 complete
- Confirmed no untyped errors in production logs

**Step 1: Remove maxErrorChainDepth constant**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/errors.go:14-16`:

Before:
```go
// maxErrorChainDepth is the maximum depth for unwrapping error chains in diagnostic logging.
// TODO(diagnostic): REMOVE THIS AFTER INFRASTRUCTURE ERROR TYPING IS COMPLETE
const maxErrorChainDepth = 10
```

After:
```go
// (removed diagnostic constant)
```

**Step 2: Remove logUnknownErrorDetails function**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/errors.go:104-139`:

Remove the entire function:
```go
// logUnknownErrorDetails logs detailed information about errors for debugging.
// TODO(diagnostic): REMOVE THIS AFTER INFRASTRUCTURE ERROR TYPING IS COMPLETE
// This diagnostic function was added to identify which errors need proper typing.
// It helps trace what error types are falling through to 500 responses.
func logUnknownErrorDetails(err error) {
	// ... entire function body
}
```

**Step 3: Remove diagnostic call from handleUnknownError**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/errors.go:141-156`:

Before:
```go
// handleUnknownError handles unknown errors by converting them to InternalServerError
func handleUnknownError(c *fiber.Ctx, err error) error {
	// DIAGNOSTIC: Log details about unknown errors to identify which need typing
	// TODO(diagnostic): REMOVE after infrastructure error typing is complete
	logUnknownErrorDetails(err)

	internalErr := pkg.ValidateInternalError(err, "")

	var internalServerErr pkg.InternalServerError
	if errors.As(internalErr, &internalServerErr) {
		return InternalServerError(c, internalServerErr.Code, internalServerErr.Title, internalServerErr.Message)
	}

	// Fallback if ValidateInternalError doesn't return the expected type
	return InternalServerError(c, "INTERNAL_ERROR", "Internal Server Error", "An unexpected error occurred")
}
```

After:
```go
// handleUnknownError handles unknown errors by converting them to InternalServerError
func handleUnknownError(c *fiber.Ctx, err error) error {
	internalErr := pkg.ValidateInternalError(err, "")

	var internalServerErr pkg.InternalServerError
	if errors.As(internalErr, &internalServerErr) {
		return InternalServerError(c, internalServerErr.Code, internalServerErr.Title, internalServerErr.Message)
	}

	// Fallback if ValidateInternalError doesn't return the expected type
	return InternalServerError(c, "INTERNAL_ERROR", "Internal Server Error", "An unexpected error occurred")
}
```

**Step 4: Remove unused imports**

If `log` and `reflect` packages are no longer used after removal, remove them from imports:

Edit `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/errors.go:3-12`:

Before:
```go
import (
	"errors"
	"log"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
)
```

After:
```go
import (
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
)
```

**Step 5: Verify changes compile**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/net/http/...`

**Expected output:** Build succeeds with no errors

**Step 6: Run tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/net/http/... -v`

**Expected output:** All tests pass

**Step 7: Run linter**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && golangci-lint run ./pkg/net/http/...`

**Expected output:** No linting errors

**If Task Fails:**

1. **Build fails - unused import:**
   - Check: `log` or `reflect` still used elsewhere
   - Fix: Keep required imports
   - Rollback: `git checkout -- pkg/net/http/errors.go`

2. **Tests fail:**
   - Check: Tests depending on diagnostic logging
   - Fix: Update tests to not expect diagnostic output
   - Rollback: `git reset --hard HEAD`

---

## Batch 5: gRPC Stream Interceptor (Item #8) - Future Work

### Task 5.1: Document Phase 2 gRPC Streaming Work

**Files:**
- Analyze: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/grpc/in/routes.go:27`

**Prerequisites:**
- None

**Step 1: Understand current state**

Current implementation:
- `grpcPanicRecoveryInterceptor` is a `grpc.UnaryServerInterceptor`
- Handles panic recovery for unary (request-response) gRPC calls
- TODO suggests adding stream interceptor for streaming endpoints

**Step 2: Check for existing streaming endpoints**

Run: `grep -rn "StreamServer\|grpc.ServerStream" /Users/fredamaral/repos/lerianstudio/midaz/ --include="*.go"`

**Expected output:** No streaming gRPC endpoints currently exist

**Step 3: Update TODO to be more descriptive**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/grpc/in/routes.go:26-28`:

Before:
```go
// grpcPanicRecoveryInterceptor creates a unary interceptor that recovers from panics.
// TODO(phase2): Add stream interceptor variant for streaming gRPC endpoints if needed.
func grpcPanicRecoveryInterceptor(lg libLog.Logger) grpc.UnaryServerInterceptor {
```

After:
```go
// grpcPanicRecoveryInterceptor creates a unary interceptor that recovers from panics.
// NOTE(phase2): Stream interceptor variant needed if streaming gRPC endpoints are added.
// Implementation pattern: grpc.StreamServerInterceptor with similar panic recovery logic.
// Current state: No streaming endpoints exist, so unary-only interceptor is sufficient.
func grpcPanicRecoveryInterceptor(lg libLog.Logger) grpc.UnaryServerInterceptor {
```

**Step 4: Verify changes compile**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...`

**Expected output:** Build succeeds

**If Task Fails:**

1. **Syntax error:**
   - Check: Comment formatting
   - Rollback: `git checkout -- components/transaction/internal/adapters/grpc/in/routes.go`

---

## Batch 6: Code Review Checkpoint

### Task 6.1: Run Code Review

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

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location
- Format: `FIXME(nitpick): [Issue description] (reported by [reviewer] on [date], severity: Cosmetic)`

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

---

## Batch 7: Final Verification

### Task 7.1: Run Full Test Suite

**Prerequisites:**
- All previous batches complete

**Step 1: Run all tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./... -v -count=1`

**Expected output:** All tests pass

**Step 2: Run linter**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && golangci-lint run ./...`

**Expected output:** No linting errors

**Step 3: Build all components**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...`

**Expected output:** Build succeeds

---

### Task 7.2: Commit Changes

**Prerequisites:**
- Task 7.1 passes

**Step 1: Stage changes**

Run: `git add -A`

**Step 2: Commit**

```bash
git commit -m "$(cat <<'EOF'
refactor: clean up technical debt TODOs across codebase

- Remove unused Description field from AddressMongoDBModel (CRM)
- Update globalization TODO with strategy notes (pkg/mmodel)
- Remove diagnostic logging code from error handler (pkg/net/http)
- Document ANTLR lexer TODOs as auto-generated (no change)
- Update gRPC phase-2 TODO with implementation notes
EOF
)"
```

**Expected output:** Commit created successfully

---

## Summary

| Batch | Tasks | Estimated Time |
|-------|-------|----------------|
| 1 | CRM Address Description | 15-20 min |
| 2 | Globalization Strategy | 10 min |
| 3 | ANTLR Lexer (DEFER) | 5 min (documentation only) |
| 4 | Error Typing Diagnostic | 20-30 min |
| 5 | gRPC Stream (Future) | 5 min |
| 6 | Code Review | 15-30 min |
| 7 | Final Verification | 10 min |

**Total Estimated Time:** 80-110 minutes

**Dependencies:**
- Batch 4 (Error Typing) requires verification that no untyped errors appear in logs
- If untyped errors exist, defer Batch 4 until proper error types are added
- Batch 6 (Code Review) should run after Batches 1, 2, 4, 5

**Items Deferred:**
- Items #3, #4 (ANTLR Lexer): Auto-generated code, no modification
- Item #8 (gRPC Stream): No streaming endpoints exist, documented for future
