package bootstrap

import (
	"encoding/json"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestUnmarshalAndValidateMessage_PanicsOnNilTransactionID(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:          "test-header",
		TransactionID:     uuid.Nil,
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Balances:          []mmodel.BalanceRedis{{ID: "balance-1"}},
		Validate:          &pkgTransaction.Responses{},
		ParserDSL:         pkgTransaction.Transaction{Send: pkgTransaction.Send{Asset: "USD", Value: decimal.NewFromInt(1)}},
		TTL:               time.Now().Add(-time.Hour),
		TransactionStatus: constant.CREATED,
		TransactionDate:   time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.Panics(t, func() {
		_, _, _ = consumer.unmarshalAndValidateMessage(string(body))
	}, "Expected panic on nil TransactionID")
}

func TestUnmarshalAndValidateMessage_PanicsOnEmptyHeaderID(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:          "",
		TransactionID:     uuid.New(),
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Balances:          []mmodel.BalanceRedis{{ID: "balance-1"}},
		Validate:          &pkgTransaction.Responses{},
		ParserDSL:         pkgTransaction.Transaction{Send: pkgTransaction.Send{Asset: "USD", Value: decimal.NewFromInt(1)}},
		TTL:               time.Now().Add(-time.Hour),
		TransactionStatus: constant.CREATED,
		TransactionDate:   time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.Panics(t, func() {
		_, _, _ = consumer.unmarshalAndValidateMessage(string(body))
	}, "Expected panic on empty HeaderID")
}

func TestUnmarshalAndValidateMessage_PanicsOnNilOrganizationID(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:          "test-header",
		TransactionID:     uuid.New(),
		OrganizationID:    uuid.Nil,
		LedgerID:          uuid.New(),
		Balances:          []mmodel.BalanceRedis{{ID: "balance-1"}},
		Validate:          &pkgTransaction.Responses{},
		ParserDSL:         pkgTransaction.Transaction{Send: pkgTransaction.Send{Asset: "USD", Value: decimal.NewFromInt(1)}},
		TTL:               time.Now().Add(-time.Hour),
		TransactionStatus: constant.CREATED,
		TransactionDate:   time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.Panics(t, func() {
		_, _, _ = consumer.unmarshalAndValidateMessage(string(body))
	}, "Expected panic on nil OrganizationID")
}

func TestUnmarshalAndValidateMessage_PanicsOnNilLedgerID(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:          "test-header",
		TransactionID:     uuid.New(),
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.Nil,
		Balances:          []mmodel.BalanceRedis{{ID: "balance-1"}},
		Validate:          &pkgTransaction.Responses{},
		ParserDSL:         pkgTransaction.Transaction{Send: pkgTransaction.Send{Asset: "USD", Value: decimal.NewFromInt(1)}},
		TTL:               time.Now().Add(-time.Hour),
		TransactionStatus: constant.CREATED,
		TransactionDate:   time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.Panics(t, func() {
		_, _, _ = consumer.unmarshalAndValidateMessage(string(body))
	}, "Expected panic on nil LedgerID")
}

func TestUnmarshalAndValidateMessage_PanicsOnNilValidate(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:          "test-header",
		TransactionID:     uuid.New(),
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Balances:          []mmodel.BalanceRedis{{ID: "balance-1"}},
		Validate:          nil,
		ParserDSL:         pkgTransaction.Transaction{Send: pkgTransaction.Send{Asset: "USD", Value: decimal.NewFromInt(1)}},
		TTL:               time.Now().Add(-time.Hour),
		TransactionStatus: constant.CREATED,
		TransactionDate:   time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.Panics(t, func() {
		_, _, _ = consumer.unmarshalAndValidateMessage(string(body))
	}, "Expected panic on nil Validate")
}

func TestUnmarshalAndValidateMessage_SucceedsWithValidMessage(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:          "test-header",
		TransactionID:     uuid.New(),
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Balances:          []mmodel.BalanceRedis{{ID: "balance-1"}},
		Validate:          &pkgTransaction.Responses{},
		ParserDSL:         pkgTransaction.Transaction{Send: pkgTransaction.Send{Asset: "USD", Value: decimal.NewFromInt(1)}},
		TTL:               time.Now().Add(-time.Hour),
		TransactionStatus: constant.CREATED,
		TransactionDate:   time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.NotPanics(t, func() {
		tx, skip, err := consumer.unmarshalAndValidateMessage(string(body))
		assert.NoError(t, err)
		assert.NotNil(t, tx.TransactionID)
		assert.False(t, skip)
	})
}

func TestUnmarshalAndValidateMessage_SkipsRecentMessage(t *testing.T) {
	message := mmodel.TransactionRedisQueue{
		HeaderID:          "test-header",
		TransactionID:     uuid.New(),
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Balances:          []mmodel.BalanceRedis{{ID: "balance-1"}},
		Validate:          &pkgTransaction.Responses{},
		ParserDSL:         pkgTransaction.Transaction{Send: pkgTransaction.Send{Asset: "USD", Value: decimal.NewFromInt(1)}},
		TTL:               time.Now(),
		TransactionStatus: constant.CREATED,
		TransactionDate:   time.Now().Add(-time.Hour),
	}
	body, _ := json.Marshal(message)

	consumer := &RedisQueueConsumer{}

	assert.NotPanics(t, func() {
		_, skip, err := consumer.unmarshalAndValidateMessage(string(body))
		assert.NoError(t, err)
		assert.True(t, skip)
	})
}

func TestNewRedisQueueConsumer_PanicsOnNilLogger(t *testing.T) {
	mockHandler := in.TransactionHandler{}

	assert.Panics(t, func() {
		NewRedisQueueConsumer(nil, mockHandler)
	}, "Expected panic on nil Logger")
}

func TestNewRedisQueueConsumer_SucceedsWithValidDependencies(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockHandler := in.TransactionHandler{}

	assert.NotPanics(t, func() {
		consumer := NewRedisQueueConsumer(mockLogger, mockHandler)
		assert.NotNil(t, consumer)
	})
}
