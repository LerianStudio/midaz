// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newTestTransaction creates a transaction with the given operations for testing.
func newTestTransaction(ops []*operation.Operation) *transaction.Transaction {
	amount := decimal.NewFromInt(100)
	desc := "APPROVED"

	return &transaction.Transaction{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
		LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
		Description:    "Test transaction",
		Status: transaction.Status{
			Code:        desc,
			Description: &desc,
		},
		Amount:    &amount,
		AssetCode: "BRL",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Operations: ops,
	}
}

// newOverdraftOp creates an operation on the overdraft companion balance.
func newOverdraftOp(accountID, opType string, amount decimal.Decimal, afterAvailable decimal.Decimal) *operation.Operation {
	return &operation.Operation{
		ID:            uuid.Must(libCommons.GenerateUUIDv7()).String(),
		TransactionID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
		Type:          opType,
		AssetCode:     "BRL",
		AccountID:     accountID,
		BalanceKey:    constant.OverdraftBalanceKey,
		Direction:     "debit",
		Amount:        operation.Amount{Value: &amount},
		BalanceAfter:  operation.Balance{Available: &afterAvailable},
	}
}

// newDefaultOp creates an operation on the default balance.
func newDefaultOp(accountID, opType string, amount decimal.Decimal, afterAvailable decimal.Decimal) *operation.Operation {
	return &operation.Operation{
		ID:            uuid.Must(libCommons.GenerateUUIDv7()).String(),
		TransactionID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
		Type:          opType,
		AssetCode:     "BRL",
		AccountID:     accountID,
		BalanceKey:    constant.DefaultBalanceKey,
		Direction:     "credit",
		Amount:        operation.Amount{Value: &amount},
		BalanceAfter:  operation.Balance{Available: &afterAvailable},
	}
}

func TestBuildOverdraftEvents(t *testing.T) {
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	t.Run("nil transaction returns nil", func(t *testing.T) {
		items := buildOverdraftEvents(nil)
		assert.Nil(t, items)
	})

	t.Run("transaction with no operations returns nil", func(t *testing.T) {
		tran := newTestTransaction(nil)
		items := buildOverdraftEvents(tran)
		assert.Nil(t, items)
	})

	t.Run("transaction with only default operations returns nil", func(t *testing.T) {
		amt := decimal.NewFromInt(100)
		after := decimal.NewFromInt(200)
		tran := newTestTransaction([]*operation.Operation{
			newDefaultOp(accountID, "CREDIT", amt, after),
		})
		items := buildOverdraftEvents(tran)
		assert.Nil(t, items)
	})

	t.Run("overdraft drawn event on DEBIT companion op", func(t *testing.T) {
		amt := decimal.NewFromInt(50)
		afterAvail := decimal.NewFromInt(50)
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "DEBIT", amt, afterAvail),
		})

		items := buildOverdraftEvents(tran)
		require.Len(t, items, 1)
		assert.Equal(t, OverdraftActionDrawn, items[0].action)
		assert.Equal(t, accountID, items[0].payload.AccountID)
		assert.Equal(t, tran.ID, items[0].payload.TransactionID)
		assert.True(t, amt.Equal(items[0].payload.Amount))
		assert.True(t, afterAvail.Equal(items[0].payload.OverdraftBalance))
		assert.Nil(t, items[0].payload.OverdraftLimit, "OverdraftLimit should be nil until T-010")
	})

	t.Run("overdraft repaid event on CREDIT companion op with remaining balance", func(t *testing.T) {
		amt := decimal.NewFromInt(30)
		afterAvail := decimal.NewFromInt(20) // still has 20 outstanding
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "CREDIT", amt, afterAvail),
		})

		items := buildOverdraftEvents(tran)
		require.Len(t, items, 1)
		assert.Equal(t, OverdraftActionRepaid, items[0].action)
		assert.True(t, amt.Equal(items[0].payload.Amount))
		assert.True(t, afterAvail.Equal(items[0].payload.OverdraftBalance))
	})

	t.Run("overdraft cleared event on CREDIT companion op reaching zero", func(t *testing.T) {
		amt := decimal.NewFromInt(50)
		afterAvail := decimal.Zero // fully repaid
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "CREDIT", amt, afterAvail),
		})

		items := buildOverdraftEvents(tran)
		require.Len(t, items, 1)
		assert.Equal(t, OverdraftActionCleared, items[0].action)
		assert.True(t, amt.Equal(items[0].payload.Amount))
		assert.True(t, decimal.Zero.Equal(items[0].payload.OverdraftBalance))
	})

	t.Run("zero amount overdraft op is skipped", func(t *testing.T) {
		amt := decimal.Zero
		afterAvail := decimal.NewFromInt(50)
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "DEBIT", amt, afterAvail),
		})

		items := buildOverdraftEvents(tran)
		assert.Nil(t, items)
	})

	t.Run("nil amount value is skipped", func(t *testing.T) {
		op := newOverdraftOp(accountID, "DEBIT", decimal.NewFromInt(50), decimal.NewFromInt(50))
		op.Amount.Value = nil

		tran := newTestTransaction([]*operation.Operation{op})
		items := buildOverdraftEvents(tran)
		assert.Nil(t, items)
	})

	t.Run("nil operation in slice is skipped", func(t *testing.T) {
		amt := decimal.NewFromInt(50)
		afterAvail := decimal.NewFromInt(50)
		tran := newTestTransaction([]*operation.Operation{
			nil,
			newOverdraftOp(accountID, "DEBIT", amt, afterAvail),
		})

		items := buildOverdraftEvents(tran)
		require.Len(t, items, 1)
		assert.Equal(t, OverdraftActionDrawn, items[0].action)
	})

	t.Run("mixed transaction produces multiple events", func(t *testing.T) {
		// Scenario: one account draws overdraft, another gets repaid in same transaction.
		accountA := uuid.Must(libCommons.GenerateUUIDv7()).String()
		accountB := uuid.Must(libCommons.GenerateUUIDv7()).String()

		drawAmt := decimal.NewFromInt(80)
		drawAfter := decimal.NewFromInt(80)
		repayAmt := decimal.NewFromInt(40)
		repayAfter := decimal.NewFromInt(10)

		tran := newTestTransaction([]*operation.Operation{
			newDefaultOp(accountA, "DEBIT", decimal.NewFromInt(50), decimal.Zero),
			newOverdraftOp(accountA, "DEBIT", drawAmt, drawAfter),
			newDefaultOp(accountB, "CREDIT", decimal.NewFromInt(100), decimal.NewFromInt(100)),
			newOverdraftOp(accountB, "CREDIT", repayAmt, repayAfter),
		})

		items := buildOverdraftEvents(tran)
		require.Len(t, items, 2)
		assert.Equal(t, OverdraftActionDrawn, items[0].action)
		assert.Equal(t, OverdraftActionRepaid, items[1].action)
	})

	t.Run("OverdraftLimit is omitted from JSON when nil", func(t *testing.T) {
		amt := decimal.NewFromInt(50)
		afterAvail := decimal.NewFromInt(50)
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "DEBIT", amt, afterAvail),
		})

		items := buildOverdraftEvents(tran)
		require.Len(t, items, 1)

		raw, err := json.Marshal(items[0].payload)
		require.NoError(t, err)

		var asMap map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &asMap))
		_, hasLimit := asMap["overdraftLimit"]
		assert.False(t, hasLimit, "overdraftLimit should be omitted from JSON when nil")
	})
}

func TestClassifyOverdraftOperation(t *testing.T) {
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	tests := []struct {
		name           string
		opType         string
		amount         decimal.Decimal
		afterAvailable decimal.Decimal
		wantAction     string
		wantAmount     decimal.Decimal
		wantAfter      decimal.Decimal
	}{
		{
			name:           "DEBIT produces drawn",
			opType:         "DEBIT",
			amount:         decimal.NewFromInt(50),
			afterAvailable: decimal.NewFromInt(50),
			wantAction:     OverdraftActionDrawn,
			wantAmount:     decimal.NewFromInt(50),
			wantAfter:      decimal.NewFromInt(50),
		},
		{
			name:           "debit lowercase produces drawn",
			opType:         "debit",
			amount:         decimal.NewFromInt(25),
			afterAvailable: decimal.NewFromInt(75),
			wantAction:     OverdraftActionDrawn,
			wantAmount:     decimal.NewFromInt(25),
			wantAfter:      decimal.NewFromInt(75),
		},
		{
			name:           "CREDIT with remaining produces repaid",
			opType:         "CREDIT",
			amount:         decimal.NewFromInt(30),
			afterAvailable: decimal.NewFromInt(20),
			wantAction:     OverdraftActionRepaid,
			wantAmount:     decimal.NewFromInt(30),
			wantAfter:      decimal.NewFromInt(20),
		},
		{
			name:           "CREDIT reaching zero produces cleared",
			opType:         "CREDIT",
			amount:         decimal.NewFromInt(50),
			afterAvailable: decimal.Zero,
			wantAction:     OverdraftActionCleared,
			wantAmount:     decimal.NewFromInt(50),
			wantAfter:      decimal.Zero,
		},
		{
			name:           "unknown type produces empty",
			opType:         "HOLD",
			amount:         decimal.NewFromInt(10),
			afterAvailable: decimal.NewFromInt(10),
			wantAction:     "",
			wantAmount:     decimal.Zero,
			wantAfter:      decimal.Zero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := newOverdraftOp(accountID, tt.opType, tt.amount, tt.afterAvailable)
			action, amount, afterAvail := classifyOverdraftOperation(op)
			assert.Equal(t, tt.wantAction, action)
			assert.True(t, tt.wantAmount.Equal(amount), "expected amount %s, got %s", tt.wantAmount, amount)
			assert.True(t, tt.wantAfter.Equal(afterAvail), "expected afterAvail %s, got %s", tt.wantAfter, afterAvail)
		})
	}
}

func TestSendOverdraftEvents(t *testing.T) {
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	t.Run("events disabled does not publish", func(t *testing.T) {
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", "false")
		defer os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		uc := &UseCase{RabbitMQRepo: mockRabbitMQRepo}

		// No ProducerDefault expectations — it should not be called.
		amt := decimal.NewFromInt(50)
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "DEBIT", amt, amt),
		})

		uc.SendOverdraftEvents(context.Background(), tran)
	})

	t.Run("events enabled by default when env unset", func(t *testing.T) {
		os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE", "test-overdraft-exchange")
		os.Setenv("VERSION", "1.0.0")
		defer func() {
			os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE")
			os.Unsetenv("VERSION")
		}()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		uc := &UseCase{RabbitMQRepo: mockRabbitMQRepo}

		amt := decimal.NewFromInt(50)
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "DEBIT", amt, amt),
		})

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-overdraft-exchange", "midaz.balance.overdraft.drawn", gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc.SendOverdraftEvents(context.Background(), tran)
	})

	t.Run("drawn event publishes correct routing key", func(t *testing.T) {
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", "true")
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE", "test-overdraft-exchange")
		os.Setenv("VERSION", "1.0.0")
		defer func() {
			os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")
			os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE")
			os.Unsetenv("VERSION")
		}()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		uc := &UseCase{RabbitMQRepo: mockRabbitMQRepo}

		amt := decimal.NewFromInt(80)
		afterAvail := decimal.NewFromInt(80)
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "DEBIT", amt, afterAvail),
		})

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-overdraft-exchange", "midaz.balance.overdraft.drawn", gomock.Any()).
			DoAndReturn(func(_ context.Context, exchange, key string, message []byte) (*string, error) {
				// Verify the envelope structure.
				var evt map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(message, &evt))
				assert.Contains(t, string(evt["action"]), "overdraft.drawn")
				assert.Contains(t, string(evt["eventType"]), "balance")

				// Verify payload does not contain overdraftLimit.
				var payload map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(evt["payload"], &payload))
				_, hasLimit := payload["overdraftLimit"]
				assert.False(t, hasLimit, "overdraftLimit should be omitted from payload")
				return nil, nil
			}).
			Times(1)

		uc.SendOverdraftEvents(context.Background(), tran)
	})

	t.Run("cleared event publishes correct routing key", func(t *testing.T) {
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", "true")
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE", "test-overdraft-exchange")
		os.Setenv("VERSION", "1.0.0")
		defer func() {
			os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")
			os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE")
			os.Unsetenv("VERSION")
		}()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		uc := &UseCase{RabbitMQRepo: mockRabbitMQRepo}

		amt := decimal.NewFromInt(50)
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "CREDIT", amt, decimal.Zero),
		})

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-overdraft-exchange", "midaz.balance.overdraft.cleared", gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc.SendOverdraftEvents(context.Background(), tran)
	})

	t.Run("repaid event publishes correct routing key", func(t *testing.T) {
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", "true")
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE", "test-overdraft-exchange")
		os.Setenv("VERSION", "1.0.0")
		defer func() {
			os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")
			os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE")
			os.Unsetenv("VERSION")
		}()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		uc := &UseCase{RabbitMQRepo: mockRabbitMQRepo}

		amt := decimal.NewFromInt(30)
		afterAvail := decimal.NewFromInt(20) // not cleared
		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountID, "CREDIT", amt, afterAvail),
		})

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-overdraft-exchange", "midaz.balance.overdraft.repaid", gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc.SendOverdraftEvents(context.Background(), tran)
	})

	t.Run("no overdraft ops produces no publish calls", func(t *testing.T) {
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", "true")
		defer os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		uc := &UseCase{RabbitMQRepo: mockRabbitMQRepo}

		// Only default balance operations — no overdraft.
		amt := decimal.NewFromInt(100)
		afterAvail := decimal.NewFromInt(200)
		tran := newTestTransaction([]*operation.Operation{
			newDefaultOp(accountID, "CREDIT", amt, afterAvail),
		})

		// No expectations — ProducerDefault should not be called.
		uc.SendOverdraftEvents(context.Background(), tran)
	})

	t.Run("multiple overdraft ops produce multiple publish calls", func(t *testing.T) {
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", "true")
		os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE", "test-overdraft-exchange")
		os.Setenv("VERSION", "1.0.0")
		defer func() {
			os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")
			os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE")
			os.Unsetenv("VERSION")
		}()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		uc := &UseCase{RabbitMQRepo: mockRabbitMQRepo}

		accountA := uuid.Must(libCommons.GenerateUUIDv7()).String()
		accountB := uuid.Must(libCommons.GenerateUUIDv7()).String()

		tran := newTestTransaction([]*operation.Operation{
			newOverdraftOp(accountA, "DEBIT", decimal.NewFromInt(80), decimal.NewFromInt(80)),
			newOverdraftOp(accountB, "CREDIT", decimal.NewFromInt(40), decimal.Zero),
		})

		// Expect two calls: one drawn, one cleared.
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-overdraft-exchange", "midaz.balance.overdraft.drawn", gomock.Any()).
			Return(nil, nil).
			Times(1)

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-overdraft-exchange", "midaz.balance.overdraft.cleared", gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc.SendOverdraftEvents(context.Background(), tran)
	})
}

func TestIsOverdraftEventEnabled(t *testing.T) {
	tests := []struct {
		name     string
		envValue *string // nil means unset
		want     bool
	}{
		{name: "unset returns true", envValue: nil, want: true},
		{name: "empty string returns true", envValue: strPtr(""), want: true},
		{name: "true returns true", envValue: strPtr("true"), want: true},
		{name: "TRUE returns true", envValue: strPtr("TRUE"), want: true},
		{name: "false returns false", envValue: strPtr("false"), want: false},
		{name: "FALSE returns false", envValue: strPtr("FALSE"), want: false},
		{name: "False returns false", envValue: strPtr("False"), want: false},
		{name: "  false  with spaces returns false", envValue: strPtr("  false  "), want: false},
		{name: "other value returns true", envValue: strPtr("yes"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == nil {
				os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")
			} else {
				os.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", *tt.envValue)
			}
			defer os.Unsetenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")

			got := isOverdraftEventEnabled()
			assert.Equal(t, tt.want, got)
		})
	}
}

// strPtr is declared in update_balance_overdraft_test.go in the same package.
