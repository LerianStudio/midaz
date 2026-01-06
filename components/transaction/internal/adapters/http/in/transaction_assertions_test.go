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
	jwt "github.com/golang-jwt/jwt/v5"
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

func TestHasScope(t *testing.T) {
	required := "transactions:read_balance_status"

	cases := []struct {
		name   string
		claims jwt.MapClaims
		want   bool
	}{
		{name: "nil claims", claims: nil, want: false},
		{name: "scope string match", claims: jwt.MapClaims{"scope": "foo transactions:read_balance_status bar"}, want: true},
		{name: "scope string no match", claims: jwt.MapClaims{"scope": "foo bar"}, want: false},
		{name: "scp string match", claims: jwt.MapClaims{"scp": "transactions:read_balance_status"}, want: true},
		{name: "scope array match", claims: jwt.MapClaims{"scope": []any{"foo", "transactions:read_balance_status"}}, want: true},
		{name: "scp array match", claims: jwt.MapClaims{"scp": []any{"transactions:read_balance_status"}}, want: true},

		// Edge cases: invalid claim value types (non-string/non-slice)
		{name: "scope integer value", claims: jwt.MapClaims{"scope": 123}, want: false},
		{name: "scope boolean true", claims: jwt.MapClaims{"scope": true}, want: false},
		{name: "scope boolean false", claims: jwt.MapClaims{"scope": false}, want: false},
		{name: "scope nil value", claims: jwt.MapClaims{"scope": nil}, want: false},
		{name: "scope float value", claims: jwt.MapClaims{"scope": 3.14}, want: false},
		{name: "scope map value", claims: jwt.MapClaims{"scope": map[string]any{"foo": "bar"}}, want: false},
		{name: "scp integer value", claims: jwt.MapClaims{"scp": 456}, want: false},
		{name: "scp boolean true", claims: jwt.MapClaims{"scp": true}, want: false},

		// Edge cases: partial/substring matches that should NOT match (exact matching enforcement)
		{name: "scope partial prefix only", claims: jwt.MapClaims{"scope": "transactions:read"}, want: false},
		{name: "scope partial suffix only", claims: jwt.MapClaims{"scope": "read_balance_status"}, want: false},
		{name: "scope extra suffix", claims: jwt.MapClaims{"scope": "transactions:read_balance_status_extra"}, want: false},
		{name: "scope extra prefix", claims: jwt.MapClaims{"scope": "admin:transactions:read_balance_status"}, want: false},
		{name: "scope substring embedded", claims: jwt.MapClaims{"scope": "xtransactions:read_balance_statusy"}, want: false},
		{name: "scope array partial match", claims: jwt.MapClaims{"scope": []any{"transactions:read", "read_balance_status"}}, want: false},
		{name: "scope array with extra suffix", claims: jwt.MapClaims{"scope": []any{"transactions:read_balance_status_v2"}}, want: false},
		{name: "scp partial match", claims: jwt.MapClaims{"scp": "transactions:read"}, want: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasScope(tt.claims, required))
		})
	}

	assert.True(t, hasScope(jwt.MapClaims{}, ""))
}
