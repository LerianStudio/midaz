package rabbitmq

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestGetRetryCount_ReturnsZeroForFirstDelivery(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{}
	count := getRetryCount(headers)
	assert.Equal(t, 0, count, "First delivery should have retry count 0")
}

func TestGetRetryCount_ReturnsIncrementedValue(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{
		retryCountHeader: int32(3),
	}
	count := getRetryCount(headers)
	assert.Equal(t, 3, count, "Should return stored retry count")
}

func TestGetRetryCount_HandlesInt64(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{
		retryCountHeader: int64(5),
	}
	count := getRetryCount(headers)
	assert.Equal(t, 5, count, "Should handle int64 retry count")
}

func TestSafeIncrementRetryCount_IncrementsCorrectly(t *testing.T) {
	t.Parallel()

	result := safeIncrementRetryCount(2)
	assert.Equal(t, int32(3), result, "Should increment by 1")
}

func TestSafeIncrementRetryCount_HandlesOverflow(t *testing.T) {
	t.Parallel()

	result := safeIncrementRetryCount(2147483647) // math.MaxInt32
	assert.Equal(t, int32(2147483647), result, "Should return MaxInt32 on overflow")
}

func TestMaxRetries_IsAtLeast4(t *testing.T) {
	t.Parallel()

	// Business error retry should have same max as panic retry
	assert.GreaterOrEqual(t, maxRetries, 4, "maxRetries should be at least 4")
}

func TestCopyHeaders_ReturnsEmptyTableForNilInput(t *testing.T) {
	t.Parallel()

	result := copyHeaders(nil)
	assert.NotNil(t, result, "Should return non-nil table")
	assert.Len(t, result, 0, "Should return empty table")
}

func TestCopyHeaders_CopiesAllHeaders(t *testing.T) {
	t.Parallel()

	original := amqp.Table{
		"key1": "value1",
		"key2": int32(42),
	}
	result := copyHeaders(original)

	assert.Equal(t, original["key1"], result["key1"], "Should copy string values")
	assert.Equal(t, original["key2"], result["key2"], "Should copy int32 values")

	// Verify it's a copy, not a reference
	result["key3"] = "new"
	_, exists := original["key3"]
	assert.False(t, exists, "Modifying copy should not affect original")
}

func TestBuildDLQName_AppendsSuffix(t *testing.T) {
	t.Parallel()

	result := buildDLQName("transactions")
	assert.Equal(t, "transactions.dlq", result, "Should append .dlq suffix")
}

func TestBusinessErrorContext_Exists(t *testing.T) {
	t.Parallel()

	// This test verifies the businessErrorContext struct exists
	// and has the expected fields
	bec := &businessErrorContext{
		logger:     nil,
		msg:        nil,
		queue:      "test-queue",
		workerID:   1,
		retryCount: 2,
		conn:       nil,
		err:        nil,
	}

	assert.Equal(t, "test-queue", bec.queue)
	assert.Equal(t, 1, bec.workerID)
	assert.Equal(t, 2, bec.retryCount)
}
