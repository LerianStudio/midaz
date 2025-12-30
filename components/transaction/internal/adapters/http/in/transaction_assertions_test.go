package in

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestValidateDoubleEntry_DebitsNotEqualCredits_Panics(t *testing.T) {
	// Create operations where debits != credits (invalid double-entry)
	debitAmount := decimal.NewFromInt(100)
	creditAmount := decimal.NewFromInt(99) // Mismatched!

	operations := []*mmodel.Operation{
		{
			Type:   constant.DEBIT,
			Amount: mmodel.OperationAmount{Value: &debitAmount},
		},
		{
			Type:   constant.CREDIT,
			Amount: mmodel.OperationAmount{Value: &creditAmount},
		},
	}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on debits != credits")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "double-entry") || strings.Contains(panicMsg, "debits must equal credits"),
			"panic message should mention double-entry violation, got: %s", panicMsg)
	}()

	validateDoubleEntry(operations)
}

func TestValidateDoubleEntry_DebitsEqualCredits_NoPanic(t *testing.T) {
	amount := decimal.NewFromInt(100)

	operations := []*mmodel.Operation{
		{
			Type:   constant.DEBIT,
			Amount: mmodel.OperationAmount{Value: &amount},
		},
		{
			Type:   constant.CREDIT,
			Amount: mmodel.OperationAmount{Value: &amount},
		},
	}

	assert.NotPanics(t, func() {
		validateDoubleEntry(operations)
	})
}

func TestValidateDoubleEntry_MultipleOperations_DebitsEqualCredits(t *testing.T) {
	fifty := decimal.NewFromInt(50)
	hundred := decimal.NewFromInt(100)

	operations := []*mmodel.Operation{
		{Type: constant.DEBIT, Amount: mmodel.OperationAmount{Value: &fifty}},
		{Type: constant.DEBIT, Amount: mmodel.OperationAmount{Value: &fifty}},
		{Type: constant.CREDIT, Amount: mmodel.OperationAmount{Value: &hundred}},
	}

	assert.NotPanics(t, func() {
		validateDoubleEntry(operations)
	})
}

func TestValidateDoubleEntry_ZeroTotals_Panics(t *testing.T) {
	// Empty operations means zero totals
	operations := []*mmodel.Operation{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on zero totals")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "non-zero") || strings.Contains(panicMsg, "totals"),
			"panic message should mention non-zero totals, got: %s", panicMsg)
	}()

	validateDoubleEntry(operations)
}

func TestValidateTransactionCanBeReverted_FutureCreatedAt_Panics(t *testing.T) {
	handler := &TransactionHandler{}
	logger := &libLog.NoneLogger{}

	tracer := trace.NewNoopTracerProvider().Tracer("test")
	ctx := context.Background()
	_, span := tracer.Start(ctx, "test.validateTransactionCanBeReverted")

	transactionID := uuid.New()
	tran := &mmodel.Transaction{
		ID:        transactionID.String(),
		Status:    mmodel.Status{Code: constant.APPROVED},
		CreatedAt: time.Now().Add(time.Hour),
	}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic when created_at is in the future")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "created_at") || strings.Contains(panicMsg, "future"),
			"panic message should mention created_at or future, got: %s", panicMsg)
	}()

	_ = handler.validateTransactionCanBeReverted(&span, logger, transactionID, tran)

	t.Fatal("expected panic but none occurred")
}

func TestValidateTransactionStateTransition_InvalidTransition_Panics(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		target      string
		shouldPanic bool
	}{
		{"PENDING to APPROVED valid", "PENDING", "APPROVED", false},
		{"PENDING to CANCELED valid", "PENDING", "CANCELED", false},
		{"APPROVED to CANCELED invalid", "APPROVED", "CANCELED", true},
		{"CANCELED to APPROVED invalid", "CANCELED", "APPROVED", true},
		{"CREATED to APPROVED invalid", "CREATED", "APPROVED", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					r := recover()
					assert.NotNil(t, r, "expected panic on invalid transition")
					panicMsg := fmt.Sprintf("%v", r)
					assert.True(t, strings.Contains(panicMsg, "state transition") || strings.Contains(panicMsg, "transition"),
						"panic message should mention transition, got: %s", panicMsg)
				}()
			}

			validateTransactionStateTransition(tt.current, tt.target)

			if tt.shouldPanic {
				t.Fatal("expected panic but none occurred")
			}
		})
	}
}
