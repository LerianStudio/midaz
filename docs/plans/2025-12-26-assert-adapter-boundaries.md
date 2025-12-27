# Assert Adapter Boundaries Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add defensive assertions at adapter boundaries to catch invariant violations early and prevent silent data corruption in MongoDB, RabbitMQ, Redis, gRPC, and HTTP adapters.

**Architecture:** Assertions validate invariants at adapter boundaries (data entering/leaving the system). We use `pkg/assert` functions for programmer errors (panics on violation). External system responses that can legitimately fail should use defensive programming (check + return error) rather than assertions.

**Tech Stack:** Go 1.21+, `pkg/assert` package (That, NotNil, NotEmpty, NoError, Never, ValidUUID predicates)

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Tools: `go test`, `golangci-lint`
- Access: Local development setup with codebase cloned
- State: Working from branch `fix/fred-several-ones-dec-13-2025` or clean branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version               # Expected: go version go1.21+ darwin/arm64 (or similar)
cd /Users/fredamaral/repos/lerianstudio/midaz && git status  # Expected: clean or known changes
go build ./...           # Expected: no errors
```

---

## Part A: MongoDB Adapters

### Task 1: Add ToEntity Result Assertion in Alias MongoDB Adapter

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/mongodb/alias/alias.mongodb.go:145-152`

**Prerequisites:**
- File `components/crm/internal/adapters/mongodb/alias/alias.mongodb.go` exists
- Import `github.com/LerianStudio/midaz/v3/pkg/assert` already present (line 11)

**Step 1: Locate the ToEntity call in Create method**

The Create method at lines 145-152 calls `record.ToEntity()` and returns the result. After successful insertion, if ToEntity returns nil, it indicates data corruption during model conversion.

**Step 2: Add assertion after ToEntity call**

Find this code block (lines 145-152):
```go
	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert alias to model", err)

		return nil, err
	}

	return result, nil
```

Replace with:
```go
	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert alias to model", err)

		return nil, err
	}

	// ToEntity must return a valid entity after successful DB insertion.
	// A nil result here indicates data corruption in model conversion.
	assert.NotNil(result, "ToEntity must return valid alias after successful insertion",
		"repository", "AliasMongoDBRepository",
		"organizationID", organizationID,
		"aliasID", alias.ID)

	return result, nil
```

**Step 3: Verify the change compiles**

Run: `go build ./components/crm/...`

**Expected output:**
```
(no output - successful build)
```

**Step 4: Run existing tests**

Run: `go test ./components/crm/internal/adapters/mongodb/alias/... -v -count=1`

**Expected output:**
```
=== RUN   TestCreate
...
--- PASS: TestCreate
...
PASS
```

**If Task Fails:**

1. **Build fails:**
   - Check: Import statement for assert package exists
   - Fix: Add `"github.com/LerianStudio/midaz/v3/pkg/assert"` to imports
   - Rollback: `git checkout -- components/crm/internal/adapters/mongodb/alias/alias.mongodb.go`

2. **Tests fail:**
   - Run: `go test ./components/crm/internal/adapters/mongodb/alias/... -v` to see details
   - Check: Mock setup returns non-nil values for ToEntity

---

### Task 2: Add Required Fields Assertion in HolderLink ToEntity

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/mongodb/holder-link/holder-link.go:40-63`

**Prerequisites:**
- File exists at path
- Need to add assert import

**Step 1: Add assert import**

Find the import block (lines 1-8):
```go
package holderlink

import (
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)
```

Replace with:
```go
package holderlink

import (
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)
```

**Step 2: Add assertions in ToEntity for required fields**

Find the ToEntity method (lines 40-63):
```go
// ToEntity maps a MongoDB model to a holder link entity
func (hmm *MongoDBModel) ToEntity() *mmodel.HolderLink {
	var createdAt, updatedAt time.Time
	if hmm.CreatedAt != nil {
		createdAt = *hmm.CreatedAt
	}

	if hmm.UpdatedAt != nil {
		updatedAt = *hmm.UpdatedAt
	}

	holderLink := &mmodel.HolderLink{
		ID:        hmm.ID,
		HolderID:  hmm.HolderID,
		AliasID:   hmm.AliasID,
		LinkType:  hmm.LinkType,
		Metadata:  hmm.Metadata,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		DeletedAt: hmm.DeletedAt,
	}

	return holderLink
}
```

Replace with:
```go
// ToEntity maps a MongoDB model to a holder link entity
func (hmm *MongoDBModel) ToEntity() *mmodel.HolderLink {
	// Required fields must be present in stored documents.
	// Nil values here indicate data corruption or schema mismatch.
	assert.NotNil(hmm.ID, "HolderLink ID must not be nil in stored document",
		"model", "HolderLinkMongoDBModel")
	assert.NotNil(hmm.HolderID, "HolderLink HolderID must not be nil in stored document",
		"model", "HolderLinkMongoDBModel",
		"id", hmm.ID)
	assert.NotNil(hmm.AliasID, "HolderLink AliasID must not be nil in stored document",
		"model", "HolderLinkMongoDBModel",
		"id", hmm.ID)
	assert.NotNil(hmm.LinkType, "HolderLink LinkType must not be nil in stored document",
		"model", "HolderLinkMongoDBModel",
		"id", hmm.ID)

	var createdAt, updatedAt time.Time
	if hmm.CreatedAt != nil {
		createdAt = *hmm.CreatedAt
	}

	if hmm.UpdatedAt != nil {
		updatedAt = *hmm.UpdatedAt
	}

	holderLink := &mmodel.HolderLink{
		ID:        hmm.ID,
		HolderID:  hmm.HolderID,
		AliasID:   hmm.AliasID,
		LinkType:  hmm.LinkType,
		Metadata:  hmm.Metadata,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		DeletedAt: hmm.DeletedAt,
	}

	return holderLink
}
```

**Step 3: Verify compilation**

Run: `go build ./components/crm/...`

**Expected output:**
```
(no output - successful build)
```

**Step 4: Run tests**

Run: `go test ./components/crm/internal/adapters/mongodb/holder-link/... -v -count=1`

**Expected output:**
```
...
PASS
```

**If Task Fails:**

1. **Import error:**
   - Check: Import path is correct
   - Fix: Ensure import is `"github.com/LerianStudio/midaz/v3/pkg/assert"`

2. **Tests panic due to nil fields in mocks:**
   - Check: Test mocks must provide non-nil required fields
   - Fix: Update test fixtures to include ID, HolderID, AliasID, LinkType

---

### Task 3: Run Code Review for MongoDB Adapters

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

## Part B: RabbitMQ Adapters (Non-Critical)

### Task 4: Document Retry Count Type Validation Design Decision

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go:99-112`

**Prerequisites:**
- File exists
- Understanding: The current code silently falls back to 0 for unexpected types. This is intentional defensive programming for external message data.

**Step 1: Add documentation comment explaining the design decision**

Find the function (lines 99-112):
```go
// getRetryCount extracts the retry count from our custom retry tracking header.
// This header is set and incremented by this consumer on each republish.
// Returns 0 for first delivery (header not present).
func getRetryCount(headers amqp.Table) int {
	if val, ok := headers[retryCountHeader].(int32); ok {
		return int(val)
	}
	// Check int64 for compatibility
	if val, ok := headers[retryCountHeader].(int64); ok {
		return int(val)
	}

	return 0
}
```

Replace with:
```go
// getRetryCount extracts the retry count from our custom retry tracking header.
// This header is set and incremented by this consumer on each republish.
// Returns 0 for first delivery (header not present).
//
// DESIGN NOTE: Silent fallback to 0 for unexpected types is intentional.
// Headers come from external message brokers and may have unexpected types due to:
// - Different RabbitMQ client implementations
// - Message broker upgrades changing serialization
// - Third-party message producers
// Using assertions here would crash the consumer for recoverable conditions.
// Instead, we treat unknown types as "no retry count" (first delivery).
func getRetryCount(headers amqp.Table) int {
	if val, ok := headers[retryCountHeader].(int32); ok {
		return int(val)
	}
	// Check int64 for compatibility
	if val, ok := headers[retryCountHeader].(int64); ok {
		return int(val)
	}

	return 0
}
```

**Step 2: Verify compilation**

Run: `go build ./components/transaction/...`

**Expected output:**
```
(no output - successful build)
```

**If Task Fails:**

1. **Build fails:**
   - Check: No syntax errors in comment
   - Rollback: `git checkout -- components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go`

---

### Task 5: Add Defensive Logging for copyHeadersSafe Edge Case

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go:125-141`

**Prerequisites:**
- File exists
- Current implementation correctly handles nil input

**Step 1: Document the defensive nil check**

Find the function (lines 125-141):
```go
// copyHeadersSafe copies only allowlisted headers to prevent sensitive data propagation.
// This is a security measure to filter out auth tokens, PII, and internal paths (CWE-200).
func copyHeadersSafe(src amqp.Table) amqp.Table {
	if src == nil {
		return amqp.Table{}
	}

	dst := make(amqp.Table)

	for k, v := range src {
		if safeHeadersAllowlist[k] {
			dst[k] = v
		}
	}

	return dst
}
```

Replace with:
```go
// copyHeadersSafe copies only allowlisted headers to prevent sensitive data propagation.
// This is a security measure to filter out auth tokens, PII, and internal paths (CWE-200).
//
// DESIGN NOTE: Nil check is defensive programming, not assertion.
// Headers come from external AMQP messages and may be nil in edge cases:
// - Malformed messages from other systems
// - RabbitMQ protocol edge cases
// We return an empty table rather than panic to maintain consumer stability.
func copyHeadersSafe(src amqp.Table) amqp.Table {
	if src == nil {
		return amqp.Table{}
	}

	dst := make(amqp.Table)

	for k, v := range src {
		if safeHeadersAllowlist[k] {
			dst[k] = v
		}
	}

	return dst
}
```

**Step 2: Verify compilation**

Run: `go build ./components/transaction/...`

**Expected output:**
```
(no output - successful build)
```

---

### Task 6: Run Code Review for RabbitMQ Adapters

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously
   - Wait for all to complete

2. **Handle findings by severity (same rules as Task 3)**

3. **Proceed only when zero Critical/High/Medium issues remain**

---

## Part C: Redis Adapters

### Task 7: Document MGet Return Value Design Decision

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/redis/consumer.redis.go:172-222`

**Prerequisites:**
- File exists
- Understanding: MGet returns fewer values when keys don't exist in Redis - this is expected behavior

**Step 1: Add documentation explaining why assertion is not appropriate**

Find the MGet method and its return handling (around line 202-221):
```go
	out := make(map[string]string, len(keys))

	for i, v := range res {
		if v == nil {
			continue
		}

		switch vv := v.(type) {
		case string:
			out[keys[i]] = vv
		case []byte:
			out[keys[i]] = string(vv)
		default:
			out[keys[i]] = fmt.Sprint(v)
		}
	}

	logger.Infof("mget retrieved %d/%d values", len(out), len(keys))

	return out, nil
```

Replace with:
```go
	out := make(map[string]string, len(keys))

	// DESIGN NOTE: MGet intentionally returns fewer values than requested keys.
	// This is expected Redis behavior - missing keys return nil values.
	// We skip nil values rather than asserting, because:
	// 1. Key expiration between request and response is normal
	// 2. Cache misses are expected in distributed systems
	// 3. Callers handle missing keys via map lookup (ok pattern)
	// Using assertions here would crash for normal cache operations.
	for i, v := range res {
		if v == nil {
			continue
		}

		switch vv := v.(type) {
		case string:
			out[keys[i]] = vv
		case []byte:
			out[keys[i]] = string(vv)
		default:
			out[keys[i]] = fmt.Sprint(v)
		}
	}

	logger.Infof("mget retrieved %d/%d values", len(out), len(keys))

	return out, nil
```

**Step 2: Verify compilation**

Run: `go build ./components/transaction/...`

**Expected output:**
```
(no output - successful build)
```

---

### Task 8: Add Assertion for Lua Script Result Type

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/redis/consumer.redis.go:353-374`

**Prerequisites:**
- File exists
- Import for assert already present (line 19)

**Step 1: Locate executeBalanceScript and its result handling**

The method `executeBalanceScript` at lines 353-374 calls `convertResultToBytes` which handles type conversion. The Lua script is internal code we control, so unexpected result types indicate a programming error.

**Step 2: Add assertion in convertResultToBytes**

Find the function (lines 399-411):
```go
// convertResultToBytes converts Redis script result to bytes
func (rr *RedisConsumerRepository) convertResultToBytes(result any, logger libLog.Logger) ([]byte, error) {
	switch v := result.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		err := fmt.Errorf("%w: %T", ErrUnexpectedRedisResultType, result)
		logger.Warnf("Warning: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}
}
```

Replace with:
```go
// convertResultToBytes converts Redis script result to bytes
//
// NOTE: The Lua script (add_sub.lua) is internal code that MUST return string or []byte.
// Other types indicate a bug in the Lua script, not an external system issue.
// We use assert here because this is a programmer error, not a runtime condition.
func (rr *RedisConsumerRepository) convertResultToBytes(result any, logger libLog.Logger) ([]byte, error) {
	switch v := result.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		// This should never happen with our Lua script - indicates a programming error
		assert.Never("Lua script returned unexpected type - check add_sub.lua",
			"expected", "string or []byte",
			"actual_type", fmt.Sprintf("%T", result),
			"script", "add_sub.lua")

		// Unreachable, but satisfies compiler
		return nil, nil
	}
}
```

**Step 3: Verify compilation**

Run: `go build ./components/transaction/...`

**Expected output:**
```
(no output - successful build)
```

**Step 4: Run tests**

Run: `go test ./components/transaction/internal/adapters/redis/... -v -count=1 2>&1 | head -50`

**Expected output:**
```
...
PASS
```

**If Task Fails:**

1. **Build error about unreachable code:**
   - The `assert.Never` panics, so return after it is technically unreachable
   - This is intentional - compiler needs the return statement
   - If linter complains, add `//nolint:unreachable` comment

---

### Task 9: Run Code Review for Redis Adapters

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - Wait for all to complete

2. **Handle findings by severity (same rules as Task 3)**

3. **Proceed only when zero Critical/High/Medium issues remain**

---

## Part D: gRPC Adapters

### Task 10: Document Token Extraction Design Decision

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/grpc/out/balance.grpc.go:149-159`

**Prerequisites:**
- File exists
- Understanding: Empty token is valid for unauthenticated endpoints

**Step 1: Add documentation explaining why empty token is acceptable**

Find the function (lines 149-159):
```go
// extractAuthToken extracts the authorization token from context metadata.
// Returns empty string if no token is found.
func extractAuthToken(ctx context.Context) string {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if vals := md.Get(libConstant.MetadataAuthorization); len(vals) > 0 {
			return vals[0]
		}
	}

	return ""
}
```

Replace with:
```go
// extractAuthToken extracts the authorization token from context metadata.
// Returns empty string if no token is found.
//
// DESIGN NOTE: Empty token return is intentional, not an error.
// Not all gRPC calls require authentication:
// - Some endpoints are public
// - Internal service-to-service calls may use mTLS instead
// - Token validation happens at the receiving service
// Using assertions here would break legitimate unauthenticated flows.
func extractAuthToken(ctx context.Context) string {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if vals := md.Get(libConstant.MetadataAuthorization); len(vals) > 0 {
			return vals[0]
		}
	}

	return ""
}
```

**Step 2: Verify compilation**

Run: `go build ./components/onboarding/...`

**Expected output:**
```
(no output - successful build)
```

---

### Task 11: Add Assertions for Decimal Parsing in gRPC Response

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/grpc/out/balance.grpc.go:186-194`

**Prerequisites:**
- File exists
- Import for assert already present (line 11)

**Step 1: Locate the decimal parsing code in CreateBalanceSync**

Find the code block (lines 185-205):
```go
	// Convert proto response to native model
	available, err := decimal.NewFromString(resp.Available)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Balance")
	}

	onHold, err := decimal.NewFromString(resp.OnHold)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Balance")
	}

	return &mmodel.Balance{
		ID:             resp.Id,
		Alias:          resp.Alias,
		Key:            resp.Key,
		AssetCode:      resp.AssetCode,
		Available:      available,
		OnHold:         onHold,
		AllowSending:   resp.AllowSending,
		AllowReceiving: resp.AllowReceiving,
	}, nil
```

Replace with:
```go
	// Convert proto response to native model
	available, err := decimal.NewFromString(resp.Available)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Balance")
	}

	onHold, err := decimal.NewFromString(resp.OnHold)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Balance")
	}

	// Balance values from the transaction service must be non-negative.
	// Negative balances indicate a bug in the balance calculation service.
	assert.That(assert.NonNegativeDecimal(available),
		"available balance from gRPC must be non-negative",
		"available", available.String(),
		"response_id", resp.Id)
	assert.That(assert.NonNegativeDecimal(onHold),
		"onHold balance from gRPC must be non-negative",
		"onHold", onHold.String(),
		"response_id", resp.Id)

	return &mmodel.Balance{
		ID:             resp.Id,
		Alias:          resp.Alias,
		Key:            resp.Key,
		AssetCode:      resp.AssetCode,
		Available:      available,
		OnHold:         onHold,
		AllowSending:   resp.AllowSending,
		AllowReceiving: resp.AllowReceiving,
	}, nil
```

**Step 2: Verify compilation**

Run: `go build ./components/onboarding/...`

**Expected output:**
```
(no output - successful build)
```

**Step 3: Run tests**

Run: `go test ./components/onboarding/internal/adapters/grpc/out/... -v -count=1`

**Expected output:**
```
...
PASS
```

**If Task Fails:**

1. **Test fails with assertion panic:**
   - Check: Mock gRPC responses must return non-negative decimal strings
   - Fix: Update test fixtures to use "0" or positive values

---

### Task 12: Add Parameter Assertions in MapAuthGRPCError

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mgrpc/errors.go:15-56`

**Prerequisites:**
- File exists
- Need to add assert import

**Step 1: Add assert import**

Find the import block (lines 1-11):
```go
package mgrpc

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)
```

Replace with:
```go
package mgrpc

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)
```

**Step 2: Add parameter assertions**

Find the function (lines 13-56):
```go
// MapAuthGRPCError maps gRPC auth errors to domain errors and logs raw details.
// Returns the original error when it isn't an auth error.
func MapAuthGRPCError(ctx context.Context, err error, code, title, operation string) error {
	if err == nil {
		return nil
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
```

Replace with:
```go
// MapAuthGRPCError maps gRPC auth errors to domain errors and logs raw details.
// Returns the original error when it isn't an auth error.
//
// Parameters code, title, and operation are used in error responses and must be non-empty.
// Empty values indicate a programming error in the caller.
func MapAuthGRPCError(ctx context.Context, err error, code, title, operation string) error {
	// Parameter validation - these are programmer errors, not runtime conditions
	assert.NotEmpty(code, "error code must not be empty",
		"function", "MapAuthGRPCError")
	assert.NotEmpty(title, "error title must not be empty",
		"function", "MapAuthGRPCError",
		"code", code)
	assert.NotEmpty(operation, "operation description must not be empty",
		"function", "MapAuthGRPCError",
		"code", code)

	if err == nil {
		return nil
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
```

**Step 3: Verify compilation**

Run: `go build ./pkg/mgrpc/...`

**Expected output:**
```
(no output - successful build)
```

**Step 4: Run tests**

Run: `go test ./pkg/mgrpc/... -v -count=1`

**Expected output:**
```
...
PASS
```

**If Task Fails:**

1. **Tests panic due to empty parameters:**
   - Check: All test calls must provide non-empty code, title, operation
   - Fix: Update test calls with valid strings

---

### Task 13: Run Code Review for gRPC Adapters

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - Wait for all to complete

2. **Handle findings by severity (same rules as Task 3)**

3. **Proceed only when zero Critical/High/Medium issues remain**

---

## Part E: HTTP Adapters

### Task 14: Add UUID Format Assertion for Organization ID Header

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/http/in/alias.go:37-74`

**Prerequisites:**
- File exists
- Need to add assert import

**Step 1: Add assert import**

Find the import block (lines 1-15):
```go
package in

import (
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)
```

Replace with:
```go
package in

import (
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)
```

**Step 2: Add UUID validation for organizationID in CreateAlias**

Find the CreateAlias handler (lines 37-74), specifically the header extraction (around line 47):
```go
	payload := http.Payload[*mmodel.CreateAliasInput](c, p)
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := c.Get("X-Organization-Id")

	span.SetAttributes(
```

Replace with:
```go
	payload := http.Payload[*mmodel.CreateAliasInput](c, p)
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := c.Get("X-Organization-Id")

	// organizationID header should be validated by middleware before reaching handler.
	// If we get here with invalid UUID, it indicates middleware misconfiguration.
	assert.That(assert.ValidUUID(organizationID),
		"X-Organization-Id header must be valid UUID - check middleware configuration",
		"handler", "CreateAlias",
		"organizationID", organizationID)

	span.SetAttributes(
```

**Step 3: Verify compilation**

Run: `go build ./components/crm/...`

**Expected output:**
```
(no output - successful build)
```

---

### Task 15: Document holderID Parse Success in GetAllAliases

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/adapters/http/in/alias.go:285-304`

**Prerequisites:**
- File exists
- Understanding: After successful uuid.Parse, the result cannot be uuid.Nil unless the input was all zeros

**Step 1: Add documentation about parse behavior**

Find the holderID parsing code (lines 294-304):
```go
	var holderID uuid.UUID
	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		holderID, err = uuid.Parse(*headerParams.HolderID)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to parse holder ID", err)

			logger.Errorf("Failed to parse holder ID, Error: %s", err.Error())

			return pkg.ValidateInternalError(http.WithError(c, err), "CRM")
		}
	}
```

Replace with:
```go
	var holderID uuid.UUID
	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		holderID, err = uuid.Parse(*headerParams.HolderID)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to parse holder ID", err)

			logger.Errorf("Failed to parse holder ID, Error: %s", err.Error())

			return pkg.ValidateInternalError(http.WithError(c, err), "CRM")
		}
		// NOTE: After successful Parse, holderID is guaranteed to be a valid UUID.
		// It could be uuid.Nil (all zeros) only if the input was "00000000-0000-0000-0000-000000000000".
		// This is a valid UUID per RFC 4122, so we don't assert against uuid.Nil.
		// The service layer handles the semantic meaning of nil vs non-nil holder IDs.
	}
```

**Step 2: Verify compilation**

Run: `go build ./components/crm/...`

**Expected output:**
```
(no output - successful build)
```

---

### Task 16: Document WithError Design Decision

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/net/http/errors.go:18-32`

**Prerequisites:**
- File exists
- Understanding: Fiber context is always non-nil when handler is called; nil error is handled

**Step 1: Add documentation about parameter expectations**

Find the WithError function (lines 18-32):
```go
// WithError returns an error with the given status code and message.
func WithError(c *fiber.Ctx, err error) error {
	// Handle standard error types
	if handled, result := handleStandardErrors(c, err); handled {
		return result
	}

	// Handle special error types
	if handled, result := handleSpecialErrors(c, err); handled {
		return result
	}

	// Handle unknown errors
	return handleUnknownError(c, err)
}
```

Replace with:
```go
// WithError returns an error with the given status code and message.
//
// DESIGN NOTE: No assertions on c or err parameters.
// - c (fiber.Ctx): Fiber guarantees non-nil context when calling handlers.
//   If we receive nil, Fiber itself is broken - panic is appropriate.
// - err: May be nil in edge cases (defensive callers). We handle this gracefully
//   in handleUnknownError rather than asserting.
//
// The current behavior (implicit nil dereference for c, graceful handling for err)
// is intentional and appropriate for this HTTP boundary.
func WithError(c *fiber.Ctx, err error) error {
	// Handle standard error types
	if handled, result := handleStandardErrors(c, err); handled {
		return result
	}

	// Handle special error types
	if handled, result := handleSpecialErrors(c, err); handled {
		return result
	}

	// Handle unknown errors
	return handleUnknownError(c, err)
}
```

**Step 2: Verify compilation**

Run: `go build ./pkg/net/http/...`

**Expected output:**
```
(no output - successful build)
```

---

### Task 17: Run Code Review for HTTP Adapters

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - Wait for all to complete

2. **Handle findings by severity (same rules as Task 3)**

3. **Proceed only when zero Critical/High/Medium issues remain**

---

## Part F: Final Verification

### Task 18: Run Full Build and Lint

**Files:**
- None (verification only)

**Step 1: Run full build**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...`

**Expected output:**
```
(no output - successful build)
```

**Step 2: Run linter**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && golangci-lint run ./... 2>&1 | head -50`

**Expected output:**
```
(no new lint errors related to our changes)
```

**Step 3: Run affected tests**

Run: `go test ./components/crm/... ./components/transaction/... ./components/onboarding/... ./pkg/... -count=1 2>&1 | tail -20`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/crm/...
ok  	github.com/LerianStudio/midaz/v3/components/transaction/...
ok  	github.com/LerianStudio/midaz/v3/components/onboarding/...
ok  	github.com/LerianStudio/midaz/v3/pkg/...
```

**If Task Fails:**

1. **Build fails:**
   - Run: `go build ./... 2>&1` to see specific errors
   - Fix: Address each error individually

2. **Lint fails:**
   - Check: New errors vs pre-existing
   - Fix: Only address errors in files we modified

3. **Tests fail:**
   - Run: Specific test file with `-v` flag
   - Fix: Update mocks/fixtures to handle new assertions

---

### Task 19: Commit Changes

**Prerequisites:**
- All tests pass
- All code reviews complete

**Step 1: Stage changes**

Run: `git add components/crm/internal/adapters/mongodb/alias/alias.mongodb.go components/crm/internal/adapters/mongodb/holder-link/holder-link.go components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go components/transaction/internal/adapters/redis/consumer.redis.go components/onboarding/internal/adapters/grpc/out/balance.grpc.go pkg/mgrpc/errors.go components/crm/internal/adapters/http/in/alias.go pkg/net/http/errors.go`

**Step 2: Create commit**

Run:
```bash
git commit -m "$(cat <<'EOF'
feat(assert): add defensive assertions at adapter boundaries

Add assertions to catch invariant violations early:
- MongoDB: ToEntity result validation, required fields in HolderLink
- RabbitMQ: Document defensive programming design decisions
- Redis: Lua script result type assertion
- gRPC: Non-negative balance validation, parameter assertions
- HTTP: UUID format validation for organization ID header

Assertions are used for programmer errors (invariants that should never
be violated). External system responses use defensive programming
(check + handle) rather than assertions.
EOF
)"
```

**Expected output:**
```
[branch-name abc1234] feat(assert): add defensive assertions at adapter boundaries
 8 files changed, XX insertions(+), YY deletions(-)
```

---

## Summary

This plan adds defensive assertions at adapter boundaries following these principles:

1. **Assertions for programmer errors:** Used when invariants should never be violated (internal code, model conversions)

2. **Defensive programming for external data:** Check + handle for data from external systems (message queues, caches, user input)

3. **Documentation for design decisions:** When assertions are deliberately not used, document why

**Files Modified:**
- `components/crm/internal/adapters/mongodb/alias/alias.mongodb.go`
- `components/crm/internal/adapters/mongodb/holder-link/holder-link.go`
- `components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go`
- `components/transaction/internal/adapters/redis/consumer.redis.go`
- `components/onboarding/internal/adapters/grpc/out/balance.grpc.go`
- `pkg/mgrpc/errors.go`
- `components/crm/internal/adapters/http/in/alias.go`
- `pkg/net/http/errors.go`

**Total Tasks:** 19 (including 5 code review checkpoints)
**Estimated Time:** 60-90 minutes for full execution with code reviews
