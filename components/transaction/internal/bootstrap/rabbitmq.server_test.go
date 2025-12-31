package bootstrap

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
)

func TestIsInfrastructureError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "PostgreSQL connection refused",
			err:      errors.New("dial tcp 127.0.0.1:5432: connect: connection refused"),
			expected: true,
		},
		{
			name:     "PostgreSQL connection reset",
			err:      errors.New("read tcp 127.0.0.1:5432: connection reset by peer"),
			expected: true,
		},
		{
			name:     "PostgreSQL server closed connection",
			err:      errors.New("server closed the connection unexpectedly"),
			expected: true,
		},
		{
			name:     "PostgreSQL connection closed",
			err:      errors.New("connection closed"),
			expected: true,
		},
		{
			name:     "PostgreSQL no connection to server",
			err:      errors.New("no connection to the server"),
			expected: true,
		},
		{
			name:     "PostgreSQL connection timed out",
			err:      errors.New("connection timed out"),
			expected: true,
		},
		{
			name:     "PostgreSQL could not connect",
			err:      errors.New("could not connect to server"),
			expected: true,
		},
		{
			name:     "PostgreSQL connection does not exist",
			err:      errors.New("connection does not exist"),
			expected: true,
		},
		{
			name:     "context deadline exceeded",
			err:      errors.New("context deadline exceeded"),
			expected: true,
		},
		{
			name:     "timeout error",
			err:      errors.New("operation timeout"),
			expected: true,
		},
		{
			name:     "context canceled",
			err:      errors.New("context canceled"),
			expected: true,
		},
		{
			name:     "Redis connection error",
			err:      errors.New("redis: connection pool timeout"),
			expected: true,
		},
		{
			name:     "Redis error (uppercase)",
			err:      errors.New("Redis: failed to connect"),
			expected: true,
		},
		{
			name:     "Valkey connection error",
			err:      errors.New("valkey: failed to connect"),
			expected: true,
		},
		{
			name:     "Valkey error (uppercase)",
			err:      errors.New("Valkey: connection error"),
			expected: true,
		},
		{
			name:     "RabbitMQ error",
			err:      errors.New("rabbitmq: channel closed"),
			expected: true,
		},
		{
			name:     "RabbitMQ error (uppercase)",
			err:      errors.New("RabbitMQ: connection failed"),
			expected: true,
		},
		{
			name:     "AMQP error",
			err:      errors.New("amqp: exception (504) channel error"),
			expected: true,
		},
		{
			name:     "AMQP error (uppercase)",
			err:      errors.New("AMQP: connection refused"),
			expected: true,
		},
		{
			name:     "validation error should not retry",
			err:      errors.New("invalid account ID format"),
			expected: false,
		},
		{
			name:     "business logic error should not retry",
			err:      errors.New("insufficient funds"),
			expected: false,
		},
		{
			name:     "entity not found should not retry",
			err:      errors.New("account not found"),
			expected: false,
		},
		{
			name:     "duplicate key error should not retry",
			err:      errors.New("duplicate key value violates unique constraint"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isInfrastructureError(tt.err)
			assert.Equal(t, tt.expected, result,
				"isInfrastructureError(%v) = %v, want %v", tt.err, result, tt.expected)
		})
	}
}

func TestHandlerBalanceCreateQueue_ValidationPanicsOnNilOrganizationID(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.Nil,
		LedgerID:       uuid.New(),
		AccountID:      uuid.New(),
		QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: json.RawMessage(`{"id":"test"}`)}},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_, _ = consumer.validateBalanceCreateMessage(body)
	}, "Expected panic on nil OrganizationID")
}

func TestHandlerBalanceCreateQueue_ValidationPanicsOnNilLedgerID(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.Nil,
		AccountID:      uuid.New(),
		QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: json.RawMessage(`{"id":"test"}`)}},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_, _ = consumer.validateBalanceCreateMessage(body)
	}, "Expected panic on nil LedgerID")
}

func TestHandlerBalanceCreateQueue_ValidationPanicsOnNilAccountID(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		AccountID:      uuid.Nil,
		QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: json.RawMessage(`{"id":"test"}`)}},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_, _ = consumer.validateBalanceCreateMessage(body)
	}, "Expected panic on nil AccountID")
}

func TestHandlerBalanceCreateQueue_ValidationPanicsOnEmptyQueueData(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		AccountID:      uuid.New(),
		QueueData:      []mmodel.QueueData{},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_, _ = consumer.validateBalanceCreateMessage(body)
	}, "Expected panic on empty QueueData")
}

func TestHandlerBalanceCreateQueue_ValidationPanicsOnMismatchedQueueDataID(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		AccountID:      uuid.New(),
		QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: json.RawMessage(`{"id":"test"}`)}},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_, _ = consumer.validateBalanceCreateMessage(body)
	}, "Expected panic on mismatched QueueData ID")
}

func TestHandlerBalanceCreateQueue_ValidationSucceedsWithValidMessage(t *testing.T) {
	accountID := uuid.New()
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		AccountID:      accountID,
		QueueData:      []mmodel.QueueData{{ID: accountID, Value: json.RawMessage(`{"id":"test"}`)}},
	}
	body, _ := json.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.NotPanics(t, func() {
		msg, err := consumer.validateBalanceCreateMessage(body)
		assert.NoError(t, err)
		assert.NotNil(t, msg)
	})
}

func TestHandlerBTOQueue_ValidationPanicsOnNilOrganizationID(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.Nil,
		LedgerID:       uuid.New(),
		QueueData:      []mmodel.QueueData{{ID: uuid.New()}},
	}
	body, _ := msgpack.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_, _ = consumer.validateBTOMessage(body)
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
		_, _ = consumer.validateBTOMessage(body)
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
		_, _ = consumer.validateBTOMessage(body)
	}, "Expected panic on empty QueueData")
}

func TestHandlerBTOQueue_ValidationPanicsOnMultipleQueueDataItems(t *testing.T) {
	message := mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData: []mmodel.QueueData{
			{ID: uuid.New()},
			{ID: uuid.New()},
		},
	}
	body, _ := msgpack.Marshal(message)

	consumer := &MultiQueueConsumer{}

	assert.Panics(t, func() {
		_, _ = consumer.validateBTOMessage(body)
	}, "Expected panic on multiple QueueData items")
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
		_, _ = consumer.validateBTOMessage(body)
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
