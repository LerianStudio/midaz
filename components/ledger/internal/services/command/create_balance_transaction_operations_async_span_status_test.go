// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestCreateOrUpdateTransaction_SpanClassContract is the T5 contract test for the async
// transaction-processing path (command.create_balance_transaction_operations_async.go).
// CreateOrUpdateTransaction opens its OWN child span (spanCreateTransaction) via the tracer
// argument rather than reading a span off ctx, so pinning recordCommandError's dispatch here
// also proves the card-4413 fix threads through a callee-owned child span correctly, not just
// a use case's top-level span.
func TestCreateOrUpdateTransaction_SpanClassContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		repoErr         error
		wantSpanStatus  codes.Code
		wantSpanEvent   string
		wantEventDetail string
	}{
		{
			name:            "technical repo failure sets the span to error",
			repoErr:         errors.New("connection reset by peer"),
			wantSpanStatus:  codes.Error,
			wantSpanEvent:   "exception",
			wantEventDetail: "Failed to create transaction on repo",
		},
		{
			name:           "business repo failure leaves the span unset",
			repoErr:        pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, constant.EntityTransaction),
			wantSpanStatus: codes.Unset,
			wantSpanEvent:  "Failed to create transaction on repo",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockTransactionRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				Return(nil, tt.repoErr).
				Times(1)

			uc := &UseCase{TransactionRepo: mockTransactionRepo}

			ctx, recorder := recordingContext()
			_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

			payload := transaction.TransactionProcessingPayload{
				Transaction: &transaction.Transaction{
					ID:     "test-transaction-id",
					Status: transaction.Status{Code: "APPROVED"},
				},
			}

			_, _, err := uc.CreateOrUpdateTransaction(ctx, &capturingLogger{}, tracer, payload)
			require.Error(t, err)

			require.Equal(t, tt.wantSpanStatus == codes.Unset, pkg.IsBusinessError(err),
				"case must exercise the error class it claims")

			span := findSpan(t, recorder, "command.create_balance_transaction_operations.create_transaction")

			assert.Equal(t, tt.wantSpanStatus, span.Status().Code,
				"span status must follow the error class (T5)")

			event, ok := findEvent(span, tt.wantSpanEvent)
			require.True(t, ok, "the failure must be recorded as span event %q; got %v", tt.wantSpanEvent, span.Events())

			if tt.wantEventDetail != "" {
				assert.Contains(t, eventText(event), tt.wantEventDetail,
					"the recorded event must name the failing operation")
			}
		})
	}
}
