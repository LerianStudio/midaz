# Async Worker Message Validation Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add post-deserialization validation assertions in async workers and message handlers to catch corrupted messages before they cause downstream failures.

**Architecture:** Add assertion checks immediately after JSON/msgpack unmarshaling in each async worker to validate critical fields (UUIDs, required arrays, dependencies). These assertions will panic with clear context, preventing silent failures and data corruption from propagating through the system.

**Tech Stack:** Go 1.23+, `pkg/assert` package for assertions, `github.com/google/uuid` for UUID validation

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.23+
- Tools: `go test`, `golangci-lint`
- Access: Read/write access to `components/transaction/` directory
- State: Clean working tree on current branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version          # Expected: go1.23 or higher
git status          # Expected: clean working tree
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...  # Expected: no errors
```

## Historical Precedent

**Query:** "async worker message validation assertions rabbitmq redis"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Task 1: Add message validation to RabbitMQ Balance Create Queue Handler

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/rabbitmq.server.go:107-145`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/rabbitmq.server_test.go`

**Prerequisites:**
- Tools: go test
- Files must exist: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/assert.go`
- The assert package must be importable

**Step 1: Write the failing test**

Create a new test file:

```go
// File: /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/rabbitmq.server_test.go
package bootstrap

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestHandlerBalanceCreateQueue_ValidationPanicsOnNilOrganizationID(t *testing.T) {
	// Arrange: Create message with nil OrganizationID
	message := mmodel.Queue{
		OrganizationID: uuid.Nil,
		LedgerID:       uuid.New(),
		QueueData:      []mmodel.QueueData{{ID: uuid.New()}},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	// Act & Assert: Should panic due to assertion failure
	assert.Panics(t, func() {
		_ = consumer.validateBalanceCreateMessage(body)
	}, "Expected panic on nil OrganizationID")
}

func TestHandlerBalanceCreateQueue_ValidationPanicsOnNilLedgerID(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.Nil,
		QueueData:      []mmodel.QueueData{{ID: uuid.New()}},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_ = consumer.validateBalanceCreateMessage(body)
	}, "Expected panic on nil LedgerID")
}

func TestHandlerBalanceCreateQueue_ValidationPanicsOnEmptyQueueData(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData:      []mmodel.QueueData{},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_ = consumer.validateBalanceCreateMessage(body)
	}, "Expected panic on empty QueueData")
}

func TestHandlerBalanceCreateQueue_ValidationSucceedsWithValidMessage(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData:      []mmodel.QueueData{{ID: uuid.New()}},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	// Should not panic
	assert.NotPanics(t, func() {
		msg, err := consumer.validateBalanceCreateMessage(body)
		assert.NoError(t, err)
		assert.NotNil(t, msg)
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestHandlerBalanceCreateQueue_Validation -count=1`

**Expected output:**
```
--- FAIL: TestHandlerBalanceCreateQueue_ValidationPanicsOnNilOrganizationID
    undefined: consumer.validateBalanceCreateMessage
FAIL
```

**If you see different error:** Check import paths and file location

**Step 3: Add validateBalanceCreateMessage helper method**

Add this method to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/rabbitmq.server.go` after line 36 (after `NewMultiQueueConsumer` function):

```go
// validateBalanceCreateMessage unmarshals and validates a balance create queue message.
// Returns the validated message or an error. Panics if critical fields are invalid.
func (mq *MultiQueueConsumer) validateBalanceCreateMessage(body []byte) (*mmodel.Queue, error) {
	var message mmodel.Queue

	if err := json.Unmarshal(body, &message); err != nil {
		return nil, pkg.ValidateInternalError(err, "Queue")
	}

	// Post-deserialization validation: catch corrupted messages early
	assert.That(message.OrganizationID != uuid.Nil,
		"message organization_id must not be nil UUID",
		"queue", "balance_create",
		"raw_length", len(body))
	assert.That(message.LedgerID != uuid.Nil,
		"message ledger_id must not be nil UUID",
		"queue", "balance_create",
		"organization_id", message.OrganizationID)
	assert.That(len(message.QueueData) > 0,
		"message queue_data must not be empty",
		"queue", "balance_create",
		"organization_id", message.OrganizationID,
		"ledger_id", message.LedgerID)

	return &message, nil
}
```

**Step 4: Add import for assert package**

Ensure the import section at the top of `rabbitmq.server.go` includes:

```go
import (
	"context"
	"encoding/json"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)
```

**Step 5: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestHandlerBalanceCreateQueue_Validation -count=1`

**Expected output:**
```
=== RUN   TestHandlerBalanceCreateQueue_ValidationPanicsOnNilOrganizationID
--- PASS: TestHandlerBalanceCreateQueue_ValidationPanicsOnNilOrganizationID
=== RUN   TestHandlerBalanceCreateQueue_ValidationPanicsOnNilLedgerID
--- PASS: TestHandlerBalanceCreateQueue_ValidationPanicsOnNilLedgerID
=== RUN   TestHandlerBalanceCreateQueue_ValidationPanicsOnEmptyQueueData
--- PASS: TestHandlerBalanceCreateQueue_ValidationPanicsOnEmptyQueueData
=== RUN   TestHandlerBalanceCreateQueue_ValidationSucceedsWithValidMessage
--- PASS: TestHandlerBalanceCreateQueue_ValidationSucceedsWithValidMessage
PASS
```

**Step 6: Update handlerBalanceCreateQueue to use the validator**

Replace lines 116-125 in `rabbitmq.server.go`:

From:
```go
	var message mmodel.Queue

	err := json.Unmarshal(body, &message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)

		logger.Errorf("Error unmarshalling accounts message JSON: %v", err)

		return pkg.ValidateInternalError(err, "Queue")
	}
```

To:
```go
	message, err := mq.validateBalanceCreateMessage(body)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)
		logger.Errorf("Error unmarshalling accounts message JSON: %v", err)
		return err
	}
```

Also update line 127 to use pointer dereference:
```go
	logger.Infof("Account message consumed: %s", message.AccountID)
```

And line 129:
```go
	err = mq.UseCase.CreateBalance(ctx, *message)
```

**Step 7: Run full test suite for bootstrap package**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -count=1`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
```

**Step 8: Commit**

```bash
git add components/transaction/internal/bootstrap/rabbitmq.server.go components/transaction/internal/bootstrap/rabbitmq.server_test.go
git commit -m "$(cat <<'EOF'
feat(bootstrap): add post-deserialization validation to balance create queue handler

Add assertion-based validation for RabbitMQ balance create queue messages
to catch corrupted messages (nil UUIDs, empty queue data) before they
cause silent failures or data corruption downstream.
EOF
)"
```

**If Task Fails:**

1. **Test won't run:**
   - Check: `ls /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/`
   - Fix: Ensure rabbitmq.server_test.go is created
   - Rollback: `git checkout -- .`

2. **Import errors:**
   - Check: `go mod tidy`
   - Fix: Add missing imports
   - Rollback: `git reset --hard HEAD`

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner
   - Don't: Try to fix without understanding

---

## Task 2: Add message validation to RabbitMQ BTO Queue Handler

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/rabbitmq.server.go:147-185`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/rabbitmq.server_test.go`

**Prerequisites:**
- Task 1 must be completed
- Test file exists

**Step 1: Write the failing test**

Add these tests to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/rabbitmq.server_test.go`:

```go
func TestHandlerBTOQueue_ValidationPanicsOnNilOrganizationID(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.Nil,
		LedgerID:       uuid.New(),
		QueueData:      []mmodel.QueueData{{ID: uuid.New()}},
	}
	body, _ := msgpack.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_ = consumer.validateBTOMessage(body)
	}, "Expected panic on nil OrganizationID")
}

func TestHandlerBTOQueue_ValidationPanicsOnNilLedgerID(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.Nil,
		QueueData:      []mmodel.QueueData{{ID: uuid.New()}},
	}
	body, _ := msgpack.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_ = consumer.validateBTOMessage(body)
	}, "Expected panic on nil LedgerID")
}

func TestHandlerBTOQueue_ValidationPanicsOnEmptyQueueData(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData:      []mmodel.QueueData{},
	}
	body, _ := msgpack.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_ = consumer.validateBTOMessage(body)
	}, "Expected panic on empty QueueData")
}

func TestHandlerBTOQueue_ValidationPanicsOnNilFirstQueueDataID(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData:      []mmodel.QueueData{{ID: uuid.Nil}},
	}
	body, _ := msgpack.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_ = consumer.validateBTOMessage(body)
	}, "Expected panic on nil first QueueData ID")
}

func TestHandlerBTOQueue_ValidationSucceedsWithValidMessage(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData:      []mmodel.QueueData{{ID: uuid.New()}},
	}
	body, _ := msgpack.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.NotPanics(t, func() {
		msg, err := consumer.validateBTOMessage(body)
		assert.NoError(t, err)
		assert.NotNil(t, msg)
	})
}
```

Also add the msgpack import at the top of the test file:

```go
import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
)
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestHandlerBTOQueue_Validation -count=1`

**Expected output:**
```
--- FAIL: TestHandlerBTOQueue_ValidationPanicsOnNilOrganizationID
    undefined: consumer.validateBTOMessage
FAIL
```

**Step 3: Add validateBTOMessage helper method**

Add this method to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/rabbitmq.server.go` after the `validateBalanceCreateMessage` function:

```go
// validateBTOMessage unmarshals and validates a BTO (Balance Transaction Operation) queue message.
// Returns the validated message or an error. Panics if critical fields are invalid.
func (mq *MultiQueueConsumer) validateBTOMessage(body []byte) (*mmodel.Queue, error) {
	var message mmodel.Queue

	if err := msgpack.Unmarshal(body, &message); err != nil {
		return nil, pkg.ValidateInternalError(err, "Queue")
	}

	// Post-deserialization validation: catch corrupted messages early
	assert.That(message.OrganizationID != uuid.Nil,
		"message organization_id must not be nil UUID",
		"queue", "bto",
		"raw_length", len(body))
	assert.That(message.LedgerID != uuid.Nil,
		"message ledger_id must not be nil UUID",
		"queue", "bto",
		"organization_id", message.OrganizationID)
	assert.That(len(message.QueueData) > 0,
		"message queue_data must not be empty",
		"queue", "bto",
		"organization_id", message.OrganizationID,
		"ledger_id", message.LedgerID)

	// Validate first queue data item (used for account ID in logging)
	assert.That(message.QueueData[0].ID != uuid.Nil,
		"first queue_data item must have valid ID",
		"queue", "bto",
		"organization_id", message.OrganizationID,
		"ledger_id", message.LedgerID)

	return &message, nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestHandlerBTOQueue_Validation -count=1`

**Expected output:**
```
=== RUN   TestHandlerBTOQueue_ValidationPanicsOnNilOrganizationID
--- PASS: TestHandlerBTOQueue_ValidationPanicsOnNilOrganizationID
...
PASS
```

**Step 5: Update handlerBTOQueue to use the validator**

Replace lines 156-165 in `rabbitmq.server.go`:

From:
```go
	var message mmodel.Queue

	err := msgpack.Unmarshal(body, &message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)

		logger.Errorf("Error unmarshalling balance message JSON: %v", err)

		return pkg.ValidateInternalError(err, "Queue")
	}
```

To:
```go
	message, err := mq.validateBTOMessage(body)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)
		logger.Errorf("Error unmarshalling balance message JSON: %v", err)
		return err
	}
```

Also update line 167 to use pointer dereference:
```go
	logger.Infof("Transaction message consumed: %s", message.QueueData[0].ID)
```

And line 169:
```go
	err = mq.UseCase.CreateBalanceTransactionOperationsAsync(ctx, *message)
```

**Step 6: Run full test suite**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -count=1`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
```

**Step 7: Commit**

```bash
git add components/transaction/internal/bootstrap/rabbitmq.server.go components/transaction/internal/bootstrap/rabbitmq.server_test.go
git commit -m "$(cat <<'EOF'
feat(bootstrap): add post-deserialization validation to BTO queue handler

Add assertion-based validation for RabbitMQ BTO (Balance Transaction Operation)
queue messages including validation of the first QueueData item ID which is
used for logging and downstream processing.
EOF
)"
```

**If Task Fails:**

1. **msgpack import error:**
   - Check: `go mod tidy`
   - Fix: Run `go get github.com/vmihailenco/msgpack/v5`
   - Rollback: `git checkout -- .`

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 3: Add message validation to Redis Queue Consumer

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/redis.consumer.go:150-160`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/redis.consumer_test.go`

**Prerequisites:**
- Task 2 must be completed
- assert package is importable

**Step 1: Write the failing test**

Create a new test file:

```go
// File: /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/redis.consumer_test.go
package bootstrap

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestUnmarshalAndValidateMessage_PanicsOnNilTransactionID(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:       "test-header",
		TransactionID:  uuid.Nil,
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Validate:       &pkgTransaction.Responses{},
		ParserDSL:      pkgTransaction.Transaction{},
		TTL:            time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.Panics(t, func() {
		_, _, _ = consumer.unmarshalAndValidateMessage(string(body))
	}, "Expected panic on nil TransactionID")
}

func TestUnmarshalAndValidateMessage_PanicsOnNilOrganizationID(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:       "test-header",
		TransactionID:  uuid.New(),
		OrganizationID: uuid.Nil,
		LedgerID:       uuid.New(),
		Validate:       &pkgTransaction.Responses{},
		ParserDSL:      pkgTransaction.Transaction{},
		TTL:            time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.Panics(t, func() {
		_, _, _ = consumer.unmarshalAndValidateMessage(string(body))
	}, "Expected panic on nil OrganizationID")
}

func TestUnmarshalAndValidateMessage_PanicsOnNilLedgerID(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:       "test-header",
		TransactionID:  uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.Nil,
		Validate:       &pkgTransaction.Responses{},
		ParserDSL:      pkgTransaction.Transaction{},
		TTL:            time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.Panics(t, func() {
		_, _, _ = consumer.unmarshalAndValidateMessage(string(body))
	}, "Expected panic on nil LedgerID")
}

func TestUnmarshalAndValidateMessage_PanicsOnNilValidate(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:       "test-header",
		TransactionID:  uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Validate:       nil,
		ParserDSL:      pkgTransaction.Transaction{},
		TTL:            time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.Panics(t, func() {
		_, _, _ = consumer.unmarshalAndValidateMessage(string(body))
	}, "Expected panic on nil Validate")
}

func TestUnmarshalAndValidateMessage_SucceedsWithValidMessage(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:       "test-header",
		TransactionID:  uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Validate:       &pkgTransaction.Responses{},
		ParserDSL:      pkgTransaction.Transaction{},
		TTL:            time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.NotPanics(t, func() {
		tx, skip, err := consumer.unmarshalAndValidateMessage(string(body))
		assert.NoError(t, err)
		assert.NotNil(t, tx.TransactionID)
		assert.False(t, skip) // TTL is old, should not skip
	})
}

func TestUnmarshalAndValidateMessage_SkipsRecentMessage(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:       "test-header",
		TransactionID:  uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Validate:       &pkgTransaction.Responses{},
		ParserDSL:      pkgTransaction.Transaction{},
		TTL:            time.Now(), // Recent message
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.NotPanics(t, func() {
		_, skip, err := consumer.unmarshalAndValidateMessage(string(body))
		assert.NoError(t, err)
		assert.True(t, skip) // TTL is recent, should skip
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestUnmarshalAndValidateMessage -count=1`

**Expected output:**
```
--- FAIL: TestUnmarshalAndValidateMessage_PanicsOnNilTransactionID
    Should be panicking
FAIL
```

**Step 3: Update unmarshalAndValidateMessage with validation**

Modify the function at `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/redis.consumer.go:150-160`:

Add the assert import first. Update the import section to include:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	postgreTransaction "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)
```

Then replace lines 150-160:

From:
```go
// unmarshalAndValidateMessage unmarshals and validates message TTL
func (r *RedisQueueConsumer) unmarshalAndValidateMessage(message string) (mmodel.TransactionRedisQueue, bool, error) {
	var transaction mmodel.TransactionRedisQueue
	if err := json.Unmarshal([]byte(message), &transaction); err != nil {
		return mmodel.TransactionRedisQueue{}, false, pkg.ValidateInternalError(err, "TransactionRedisQueue")
	}

	skip := transaction.TTL.Unix() > time.Now().Add(-MessageTimeOfLife*time.Minute).Unix()

	return transaction, skip, nil
}
```

To:
```go
// unmarshalAndValidateMessage unmarshals and validates message TTL and critical fields.
// Panics if critical fields are invalid to catch corrupted messages early.
func (r *RedisQueueConsumer) unmarshalAndValidateMessage(message string) (mmodel.TransactionRedisQueue, bool, error) {
	var transaction mmodel.TransactionRedisQueue
	if err := json.Unmarshal([]byte(message), &transaction); err != nil {
		return mmodel.TransactionRedisQueue{}, false, pkg.ValidateInternalError(err, "TransactionRedisQueue")
	}

	// Post-deserialization validation: catch corrupted messages early
	assert.That(transaction.TransactionID != uuid.Nil,
		"transaction_id must not be nil UUID in redis queue message",
		"header_id", transaction.HeaderID)
	assert.That(transaction.OrganizationID != uuid.Nil,
		"organization_id must not be nil UUID",
		"transaction_id", transaction.TransactionID,
		"header_id", transaction.HeaderID)
	assert.That(transaction.LedgerID != uuid.Nil,
		"ledger_id must not be nil UUID",
		"transaction_id", transaction.TransactionID,
		"header_id", transaction.HeaderID)
	assert.NotNil(transaction.Validate,
		"validate responses must not be nil",
		"transaction_id", transaction.TransactionID,
		"header_id", transaction.HeaderID)

	skip := transaction.TTL.Unix() > time.Now().Add(-MessageTimeOfLife*time.Minute).Unix()

	return transaction, skip, nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestUnmarshalAndValidateMessage -count=1`

**Expected output:**
```
=== RUN   TestUnmarshalAndValidateMessage_PanicsOnNilTransactionID
--- PASS: TestUnmarshalAndValidateMessage_PanicsOnNilTransactionID
...
PASS
```

**Step 5: Run full test suite**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -count=1`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
```

**Step 6: Commit**

```bash
git add components/transaction/internal/bootstrap/redis.consumer.go components/transaction/internal/bootstrap/redis.consumer_test.go
git commit -m "$(cat <<'EOF'
feat(bootstrap): add post-deserialization validation to Redis queue consumer

Add assertion-based validation for Redis queue messages including
TransactionID, OrganizationID, LedgerID, and Validate fields to catch
corrupted messages before they cause downstream failures.
EOF
)"
```

**If Task Fails:**

1. **uuid import missing:**
   - Fix: Add `"github.com/google/uuid"` to imports
   - Run: `go mod tidy`

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 4: Add constructor validation to BalanceSyncWorker

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/balance.worker.go:52-67`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/balance.worker_test.go`

**Prerequisites:**
- Task 3 must be completed
- assert package is importable

**Step 1: Write the failing test**

Create a new test file:

```go
// File: /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/balance.worker_test.go
package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/stretchr/testify/assert"
)

func TestNewBalanceSyncWorker_PanicsOnNilRedisConn(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockUseCase := &command.UseCase{}

	assert.Panics(t, func() {
		NewBalanceSyncWorker(nil, mockLogger, mockUseCase, 5)
	}, "Expected panic on nil Redis connection")
}

func TestNewBalanceSyncWorker_PanicsOnNilLogger(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockUseCase := &command.UseCase{}

	assert.Panics(t, func() {
		NewBalanceSyncWorker(mockConn, nil, mockUseCase, 5)
	}, "Expected panic on nil Logger")
}

func TestNewBalanceSyncWorker_PanicsOnNilUseCase(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockLogger := libLog.NewNopLogger()

	assert.Panics(t, func() {
		NewBalanceSyncWorker(mockConn, mockLogger, nil, 5)
	}, "Expected panic on nil UseCase")
}

func TestNewBalanceSyncWorker_SucceedsWithValidDependencies(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockLogger := libLog.NewNopLogger()
	mockUseCase := &command.UseCase{}

	assert.NotPanics(t, func() {
		worker := NewBalanceSyncWorker(mockConn, mockLogger, mockUseCase, 5)
		assert.NotNil(t, worker)
		assert.Equal(t, 5, worker.maxWorkers)
	})
}

func TestNewBalanceSyncWorker_DefaultsMaxWorkersWhenZero(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockLogger := libLog.NewNopLogger()
	mockUseCase := &command.UseCase{}

	worker := NewBalanceSyncWorker(mockConn, mockLogger, mockUseCase, 0)
	assert.Equal(t, 5, worker.maxWorkers)
}

func TestNewBalanceSyncWorker_DefaultsMaxWorkersWhenNegative(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockLogger := libLog.NewNopLogger()
	mockUseCase := &command.UseCase{}

	worker := NewBalanceSyncWorker(mockConn, mockLogger, mockUseCase, -1)
	assert.Equal(t, 5, worker.maxWorkers)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestNewBalanceSyncWorker -count=1`

**Expected output:**
```
--- FAIL: TestNewBalanceSyncWorker_PanicsOnNilRedisConn
    Should be panicking
FAIL
```

**Step 3: Update NewBalanceSyncWorker with validation**

First add the assert import to balance.worker.go. Update the import section:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)
```

Then replace lines 52-67:

From:
```go
// NewBalanceSyncWorker creates a new BalanceSyncWorker with the specified Redis connection and configuration.
// The maxWorkers parameter controls the concurrency of balance sync operations.
func NewBalanceSyncWorker(conn *libRedis.RedisConnection, logger libLog.Logger, useCase *command.UseCase, maxWorkers int) *BalanceSyncWorker {
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	return &BalanceSyncWorker{
		redisConn:  conn,
		logger:     logger,
		idleWait:   balanceSyncIdleWaitSeconds * time.Second,
		batchSize:  int64(maxWorkers),
		maxWorkers: maxWorkers,
		useCase:    useCase,
	}
}
```

To:
```go
// NewBalanceSyncWorker creates a new BalanceSyncWorker with the specified Redis connection and configuration.
// The maxWorkers parameter controls the concurrency of balance sync operations.
// Panics if required dependencies are nil.
func NewBalanceSyncWorker(conn *libRedis.RedisConnection, logger libLog.Logger, useCase *command.UseCase, maxWorkers int) *BalanceSyncWorker {
	// Validate required dependencies
	assert.NotNil(conn, "Redis connection required for BalanceSyncWorker")
	assert.NotNil(logger, "Logger required for BalanceSyncWorker")
	assert.NotNil(useCase, "UseCase required for BalanceSyncWorker")

	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	return &BalanceSyncWorker{
		redisConn:  conn,
		logger:     logger,
		idleWait:   balanceSyncIdleWaitSeconds * time.Second,
		batchSize:  int64(maxWorkers),
		maxWorkers: maxWorkers,
		useCase:    useCase,
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestNewBalanceSyncWorker -count=1`

**Expected output:**
```
=== RUN   TestNewBalanceSyncWorker_PanicsOnNilRedisConn
--- PASS: TestNewBalanceSyncWorker_PanicsOnNilRedisConn
...
PASS
```

**Step 5: Run full test suite**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -count=1`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
```

**Step 6: Commit**

```bash
git add components/transaction/internal/bootstrap/balance.worker.go components/transaction/internal/bootstrap/balance.worker_test.go
git commit -m "$(cat <<'EOF'
feat(bootstrap): add constructor validation to BalanceSyncWorker

Add assertion-based validation for BalanceSyncWorker constructor to
ensure required dependencies (Redis connection, logger, use case)
are not nil, preventing nil pointer panics during runtime.
EOF
)"
```

**If Task Fails:**

1. **NopLogger not found:**
   - Check: Look for the correct mock logger pattern in lib-commons
   - Fix: Use a different mock approach or `nil` check

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 5: Add constructor validation to MetadataOutboxWorker

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/metadata_outbox.worker.go:72-99`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/metadata_outbox.worker_test.go`

**Prerequisites:**
- Task 4 must be completed
- assert package is importable

**Step 1: Write the failing test**

Create a new test file:

```go
// File: /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/metadata_outbox.worker_test.go
package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/stretchr/testify/assert"
)

// Mock implementations for testing
type mockOutboxRepo struct {
	outbox.Repository
}

type mockMetadataRepo struct {
	mongodb.Repository
}

func TestNewMetadataOutboxWorker_PanicsOnNilLogger(t *testing.T) {
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(nil, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 7)
	}, "Expected panic on nil Logger")
}

func TestNewMetadataOutboxWorker_PanicsOnNilOutboxRepo(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(mockLogger, nil, mockMetadata, mockPostgres, mockMongo, 5, 7)
	}, "Expected panic on nil OutboxRepository")
}

func TestNewMetadataOutboxWorker_PanicsOnNilMetadataRepo(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockOutbox := &mockOutboxRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(mockLogger, mockOutbox, nil, mockPostgres, mockMongo, 5, 7)
	}, "Expected panic on nil MetadataRepository")
}

func TestNewMetadataOutboxWorker_PanicsOnNilPostgresConn(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, nil, mockMongo, 5, 7)
	}, "Expected panic on nil PostgresConnection")
}

func TestNewMetadataOutboxWorker_SucceedsWithValidDependencies(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.NotPanics(t, func() {
		worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 7)
		assert.NotNil(t, worker)
		assert.Equal(t, 5, worker.maxWorkers)
		assert.Equal(t, 7, worker.retentionDays)
	})
}

func TestNewMetadataOutboxWorker_DefaultsMaxWorkersWhenZero(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 0, 7)
	assert.Equal(t, 5, worker.maxWorkers)
}

func TestNewMetadataOutboxWorker_DefaultsRetentionDaysWhenZero(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 0)
	assert.Equal(t, 7, worker.retentionDays)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestNewMetadataOutboxWorker -count=1`

**Expected output:**
```
--- FAIL: TestNewMetadataOutboxWorker_PanicsOnNilLogger
    Should be panicking
FAIL
```

**Step 3: Update NewMetadataOutboxWorker with validation**

First add the assert import. Update the import section in metadata_outbox.worker.go:

```go
import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)
```

Then replace lines 72-99:

From:
```go
// NewMetadataOutboxWorker creates a new MetadataOutboxWorker instance.
func NewMetadataOutboxWorker(
	logger libLog.Logger,
	outboxRepo outbox.Repository,
	metadataRepo mongodb.Repository,
	postgresConn *libPostgres.PostgresConnection,
	mongoConn *libMongo.MongoConnection,
	maxWorkers int,
	retentionDays int,
) *MetadataOutboxWorker {
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	if retentionDays <= 0 {
		retentionDays = 7
	}

	return &MetadataOutboxWorker{
		logger:        logger,
		outboxRepo:    outboxRepo,
		metadataRepo:  metadataRepo,
		postgresConn:  postgresConn,
		mongoConn:     mongoConn,
		maxWorkers:    maxWorkers,
		retentionDays: retentionDays,
	}
}
```

To:
```go
// NewMetadataOutboxWorker creates a new MetadataOutboxWorker instance.
// Panics if required dependencies are nil.
func NewMetadataOutboxWorker(
	logger libLog.Logger,
	outboxRepo outbox.Repository,
	metadataRepo mongodb.Repository,
	postgresConn *libPostgres.PostgresConnection,
	mongoConn *libMongo.MongoConnection,
	maxWorkers int,
	retentionDays int,
) *MetadataOutboxWorker {
	// Validate required dependencies
	assert.NotNil(logger, "Logger required for MetadataOutboxWorker")
	assert.NotNil(outboxRepo, "OutboxRepository required for MetadataOutboxWorker")
	assert.NotNil(metadataRepo, "MetadataRepository required for MetadataOutboxWorker")
	assert.NotNil(postgresConn, "PostgresConnection required for MetadataOutboxWorker")

	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	if retentionDays <= 0 {
		retentionDays = 7
	}

	return &MetadataOutboxWorker{
		logger:        logger,
		outboxRepo:    outboxRepo,
		metadataRepo:  metadataRepo,
		postgresConn:  postgresConn,
		mongoConn:     mongoConn,
		maxWorkers:    maxWorkers,
		retentionDays: retentionDays,
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestNewMetadataOutboxWorker -count=1`

**Expected output:**
```
=== RUN   TestNewMetadataOutboxWorker_PanicsOnNilLogger
--- PASS: TestNewMetadataOutboxWorker_PanicsOnNilLogger
...
PASS
```

**Step 5: Run full test suite**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -count=1`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
```

**Step 6: Commit**

```bash
git add components/transaction/internal/bootstrap/metadata_outbox.worker.go components/transaction/internal/bootstrap/metadata_outbox.worker_test.go
git commit -m "$(cat <<'EOF'
feat(bootstrap): add constructor validation to MetadataOutboxWorker

Add assertion-based validation for MetadataOutboxWorker constructor to
ensure required dependencies (logger, outbox repo, metadata repo,
postgres connection) are not nil.
EOF
)"
```

**If Task Fails:**

1. **Interface mocking issues:**
   - Fix: Create minimal mock structs that satisfy the interfaces
   - Rollback: `git checkout -- .`

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 6: Add constructor validation to DLQConsumer

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/dlq.consumer.go:118-145`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/dlq.consumer_test.go`

**Prerequisites:**
- Task 5 must be completed
- assert package is importable

**Step 1: Write the failing test**

Create a new test file:

```go
// File: /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/dlq.consumer_test.go
package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/stretchr/testify/assert"
)

func TestNewDLQConsumer_PanicsOnNilLogger(t *testing.T) {
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockRedis := &libRedis.RedisConnection{}
	queueNames := []string{"test-queue"}

	assert.Panics(t, func() {
		NewDLQConsumer(nil, mockRabbitMQ, mockPostgres, mockRedis, queueNames)
	}, "Expected panic on nil Logger")
}

func TestNewDLQConsumer_PanicsOnNilRabbitMQConn(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockPostgres := &libPostgres.PostgresConnection{}
	mockRedis := &libRedis.RedisConnection{}
	queueNames := []string{"test-queue"}

	assert.Panics(t, func() {
		NewDLQConsumer(mockLogger, nil, mockPostgres, mockRedis, queueNames)
	}, "Expected panic on nil RabbitMQConnection")
}

func TestNewDLQConsumer_PanicsOnNoInfrastructureConnections(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	queueNames := []string{"test-queue"}

	assert.Panics(t, func() {
		NewDLQConsumer(mockLogger, mockRabbitMQ, nil, nil, queueNames)
	}, "Expected panic when no infrastructure connections provided")
}

func TestNewDLQConsumer_SucceedsWithPostgresOnly(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockPostgres := &libPostgres.PostgresConnection{}
	queueNames := []string{"test-queue"}

	assert.NotPanics(t, func() {
		consumer := NewDLQConsumer(mockLogger, mockRabbitMQ, mockPostgres, nil, queueNames)
		assert.NotNil(t, consumer)
	})
}

func TestNewDLQConsumer_SucceedsWithRedisOnly(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockRedis := &libRedis.RedisConnection{}
	queueNames := []string{"test-queue"}

	assert.NotPanics(t, func() {
		consumer := NewDLQConsumer(mockLogger, mockRabbitMQ, nil, mockRedis, queueNames)
		assert.NotNil(t, consumer)
	})
}

func TestNewDLQConsumer_SucceedsWithBothConnections(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockRedis := &libRedis.RedisConnection{}
	queueNames := []string{"test-queue"}

	assert.NotPanics(t, func() {
		consumer := NewDLQConsumer(mockLogger, mockRabbitMQ, mockPostgres, mockRedis, queueNames)
		assert.NotNil(t, consumer)
		assert.Equal(t, 1, len(consumer.QueueNames))
	})
}

func TestNewDLQConsumer_WarnsOnEmptyQueueNames(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockPostgres := &libPostgres.PostgresConnection{}
	queueNames := []string{}

	// Should not panic, just warn
	assert.NotPanics(t, func() {
		consumer := NewDLQConsumer(mockLogger, mockRabbitMQ, mockPostgres, nil, queueNames)
		assert.NotNil(t, consumer)
		assert.Equal(t, 0, len(consumer.QueueNames))
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestNewDLQConsumer -count=1`

**Expected output:**
```
--- FAIL: TestNewDLQConsumer_PanicsOnNilLogger
    Should be panicking
FAIL
```

**Step 3: Update NewDLQConsumer with validation**

First add the assert import. Update the import section in dlq.consumer.go:

```go
import (
	"context"
	"errors"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
)
```

Then replace lines 118-145:

From:
```go
// NewDLQConsumer creates a new DLQ consumer instance.
func NewDLQConsumer(
	logger libLog.Logger,
	rabbitMQConn *libRabbitmq.RabbitMQConnection,
	postgresConn *libPostgres.PostgresConnection,
	redisConn *libRedis.RedisConnection,
	queueNames []string,
) *DLQConsumer {
	// M6: Validate empty QueueNames array
	if len(queueNames) == 0 {
		logger.Warn("DLQ_CONSUMER_INIT: No queue names provided, DLQ consumer will not process any queues")
	}

	// H8: Initialize allowlist for queue name validation (security)
	validQueues := make(map[string]bool, len(queueNames))
	for _, q := range queueNames {
		validQueues[q] = true
	}

	return &DLQConsumer{
		Logger:              logger,
		RabbitMQConn:        rabbitMQConn,
		PostgresConn:        postgresConn,
		RedisConn:           redisConn,
		QueueNames:          queueNames,
		validOriginalQueues: validQueues,
	}
}
```

To:
```go
// NewDLQConsumer creates a new DLQ consumer instance.
// Panics if required dependencies are nil.
func NewDLQConsumer(
	logger libLog.Logger,
	rabbitMQConn *libRabbitmq.RabbitMQConnection,
	postgresConn *libPostgres.PostgresConnection,
	redisConn *libRedis.RedisConnection,
	queueNames []string,
) *DLQConsumer {
	// Validate required dependencies
	assert.NotNil(logger, "Logger required for DLQConsumer")
	assert.NotNil(rabbitMQConn, "RabbitMQConnection required for DLQConsumer")

	// At least one infrastructure connection must exist for health checks
	assert.That(postgresConn != nil || redisConn != nil,
		"DLQConsumer requires at least one infrastructure connection (Postgres or Redis)")

	// M6: Validate empty QueueNames array (warning, not panic)
	if len(queueNames) == 0 {
		logger.Warn("DLQ_CONSUMER_INIT: No queue names provided, DLQ consumer will not process any queues")
	}

	// H8: Initialize allowlist for queue name validation (security)
	validQueues := make(map[string]bool, len(queueNames))
	for _, q := range queueNames {
		validQueues[q] = true
	}

	return &DLQConsumer{
		Logger:              logger,
		RabbitMQConn:        rabbitMQConn,
		PostgresConn:        postgresConn,
		RedisConn:           redisConn,
		QueueNames:          queueNames,
		validOriginalQueues: validQueues,
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestNewDLQConsumer -count=1`

**Expected output:**
```
=== RUN   TestNewDLQConsumer_PanicsOnNilLogger
--- PASS: TestNewDLQConsumer_PanicsOnNilLogger
...
PASS
```

**Step 5: Run full test suite**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -count=1`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
```

**Step 6: Commit**

```bash
git add components/transaction/internal/bootstrap/dlq.consumer.go components/transaction/internal/bootstrap/dlq.consumer_test.go
git commit -m "$(cat <<'EOF'
feat(bootstrap): add constructor validation to DLQConsumer

Add assertion-based validation for DLQConsumer constructor to ensure
required dependencies (logger, RabbitMQ connection) are not nil, and
at least one infrastructure connection (Postgres or Redis) exists.
EOF
)"
```

**If Task Fails:**

1. **Import issues:**
   - Run: `go mod tidy`
   - Fix: Check import paths

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 7: Add constructor validation to RedisQueueConsumer

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/redis.consumer.go:52-58`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/redis.consumer_test.go`

**Prerequisites:**
- Task 6 must be completed
- Test file exists from Task 3

**Step 1: Write the failing test**

Add these tests to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/redis.consumer_test.go`:

```go
func TestNewRedisQueueConsumer_PanicsOnNilLogger(t *testing.T) {
	mockHandler := in.TransactionHandler{}

	assert.Panics(t, func() {
		NewRedisQueueConsumer(nil, mockHandler)
	}, "Expected panic on nil Logger")
}

func TestNewRedisQueueConsumer_SucceedsWithValidDependencies(t *testing.T) {
	mockLogger := libLog.NewNopLogger()
	mockHandler := in.TransactionHandler{}

	assert.NotPanics(t, func() {
		consumer := NewRedisQueueConsumer(mockLogger, mockHandler)
		assert.NotNil(t, consumer)
	})
}
```

Add the required import:
```go
import (
	"encoding/json"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestNewRedisQueueConsumer -count=1`

**Expected output:**
```
--- FAIL: TestNewRedisQueueConsumer_PanicsOnNilLogger
    Should be panicking
FAIL
```

**Step 3: Update NewRedisQueueConsumer with validation**

Replace lines 52-58 in redis.consumer.go:

From:
```go
// NewRedisQueueConsumer creates a new RedisQueueConsumer with the provided logger and handler.
func NewRedisQueueConsumer(logger libLog.Logger, handler in.TransactionHandler) *RedisQueueConsumer {
	return &RedisQueueConsumer{
		Logger:             logger,
		TransactionHandler: handler,
	}
}
```

To:
```go
// NewRedisQueueConsumer creates a new RedisQueueConsumer with the provided logger and handler.
// Panics if required dependencies are nil.
func NewRedisQueueConsumer(logger libLog.Logger, handler in.TransactionHandler) *RedisQueueConsumer {
	// Validate required dependencies
	assert.NotNil(logger, "Logger required for RedisQueueConsumer")

	return &RedisQueueConsumer{
		Logger:             logger,
		TransactionHandler: handler,
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -run TestNewRedisQueueConsumer -count=1`

**Expected output:**
```
=== RUN   TestNewRedisQueueConsumer_PanicsOnNilLogger
--- PASS: TestNewRedisQueueConsumer_PanicsOnNilLogger
...
PASS
```

**Step 5: Run full test suite**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/bootstrap/... -count=1`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
```

**Step 6: Commit**

```bash
git add components/transaction/internal/bootstrap/redis.consumer.go components/transaction/internal/bootstrap/redis.consumer_test.go
git commit -m "$(cat <<'EOF'
feat(bootstrap): add constructor validation to RedisQueueConsumer

Add assertion-based validation for RedisQueueConsumer constructor to
ensure the logger dependency is not nil.
EOF
)"
```

**If Task Fails:**

1. **Handler struct issues:**
   - Fix: Use zero value struct for handler
   - The handler validation is less critical as it's a struct not pointer

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 8: Add idempotency key validation

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-idempotency-key.go:22-69`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-idempotency-key_test.go`

**Prerequisites:**
- Task 7 must be completed
- assert package is importable

**Step 1: Write the failing test**

Create a new test file:

```go
// File: /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-idempotency-key_test.go
package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCreateOrCheckIdempotencyKey_PanicsOnNilOrganizationID(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	assert.Panics(t, func() {
		_, _ = uc.CreateOrCheckIdempotencyKey(ctx, uuid.Nil, uuid.New(), "key", "hash", time.Minute)
	}, "Expected panic on nil OrganizationID")
}

func TestCreateOrCheckIdempotencyKey_PanicsOnNilLedgerID(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	assert.Panics(t, func() {
		_, _ = uc.CreateOrCheckIdempotencyKey(ctx, uuid.New(), uuid.Nil, "key", "hash", time.Minute)
	}, "Expected panic on nil LedgerID")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/services/command/... -run TestCreateOrCheckIdempotencyKey_Panics -count=1`

**Expected output:**
```
--- FAIL: TestCreateOrCheckIdempotencyKey_PanicsOnNilOrganizationID
    Should be panicking
FAIL
```

**Step 3: Update CreateOrCheckIdempotencyKey with validation**

First add the assert and uuid imports. Update the import section in create-idempotency-key.go:

```go
import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/utils"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)
```

Then modify the CreateOrCheckIdempotencyKey function at line 22, adding validation after line 29:

From:
```go
// CreateOrCheckIdempotencyKey attempts to create an idempotency key in Redis using SetNX.
// If the key already exists, it returns the stored value. Returns nil if the key was created.
func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}
```

To:
```go
// CreateOrCheckIdempotencyKey attempts to create an idempotency key in Redis using SetNX.
// If the key already exists, it returns the stored value. Returns nil if the key was created.
// Panics if organizationID or ledgerID are nil UUIDs.
func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, error) {
	// Validate required UUIDs before any operations
	assert.That(organizationID != uuid.Nil,
		"organization_id must not be nil UUID for idempotency key",
		"key", key,
		"hash_prefix", hash[:min(len(hash), 8)])
	assert.That(ledgerID != uuid.Nil,
		"ledger_id must not be nil UUID for idempotency key",
		"organization_id", organizationID,
		"key", key)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/services/command/... -run TestCreateOrCheckIdempotencyKey_Panics -count=1`

**Expected output:**
```
=== RUN   TestCreateOrCheckIdempotencyKey_PanicsOnNilOrganizationID
--- PASS: TestCreateOrCheckIdempotencyKey_PanicsOnNilOrganizationID
...
PASS
```

**Step 5: Run full test suite for command package**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/services/command/... -count=1`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command
```

**Step 6: Commit**

```bash
git add components/transaction/internal/services/command/create-idempotency-key.go components/transaction/internal/services/command/create-idempotency-key_test.go
git commit -m "$(cat <<'EOF'
feat(command): add input validation to CreateOrCheckIdempotencyKey

Add assertion-based validation for organizationID and ledgerID to catch
nil UUIDs early before Redis operations, preventing silent data corruption.
EOF
)"
```

**If Task Fails:**

1. **min function not found:**
   - Fix: Go 1.21+ has built-in min, or use a local helper
   - Rollback: `git checkout -- .`

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 9: Run Code Review

**After completing Tasks 1-8, run comprehensive code review.**

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

## Task 10: Final verification and linting

**Files:**
- All modified files from Tasks 1-8

**Prerequisites:**
- All previous tasks completed
- Code review passed (Task 9)

**Step 1: Run go build**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...`

**Expected output:**
```
(no output - successful build)
```

**Step 2: Run go vet**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go vet ./components/transaction/...`

**Expected output:**
```
(no output - no issues)
```

**Step 3: Run golangci-lint**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && golangci-lint run ./components/transaction/...`

**Expected output:**
```
(no output or only existing issues)
```

**Step 4: Run full test suite**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/... -count=1`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command
...
```

**Step 5: Count assertions added**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git diff --stat HEAD~8 | head -20`

**Expected output:**
Shows ~15-20 new assertions across the modified files.

**Step 6: Final commit (if any fixes needed)**

Only commit if there were lint fixes:

```bash
git add -u
git commit -m "$(cat <<'EOF'
chore: fix lint issues from async worker validation changes
EOF
)"
```

**If Task Fails:**

1. **Lint errors:**
   - Fix: Address each lint error individually
   - Run: `golangci-lint run --fix ./components/transaction/...`

2. **Test failures:**
   - Review failing tests
   - Fix implementation to match test expectations

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Plan Checklist

- [x] Historical precedent queried (artifact-query --mode planning)
- [x] Historical Precedent section included in plan
- [x] Header with goal, architecture, tech stack, prerequisites
- [x] Verification commands with expected output
- [x] Tasks broken into bite-sized steps (2-5 min each)
- [x] Exact file paths for all files
- [x] Complete code (no placeholders)
- [x] Exact commands with expected output
- [x] Failure recovery steps for each task
- [x] Code review checkpoints after batches
- [x] Severity-based issue handling documented
- [x] Passes Zero-Context Test
- [x] Plan avoids known failure patterns (none found - new project)

---

## Summary

This plan adds **~18 assertions** across 8 files:

| File | Assertions Added | Purpose |
|------|------------------|---------|
| `rabbitmq.server.go` | 7 | Balance create + BTO queue validation |
| `redis.consumer.go` | 5 | Transaction queue message validation |
| `balance.worker.go` | 3 | Constructor dependency validation |
| `metadata_outbox.worker.go` | 4 | Constructor dependency validation |
| `dlq.consumer.go` | 3 | Constructor + infrastructure validation |
| `create-idempotency-key.go` | 2 | Input UUID validation |

**Expected Outcome:** Async processing catches corrupted messages immediately with clear context, preventing silent failures and data corruption.
