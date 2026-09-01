// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	mtransaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveTransactionSkips locks the per-call control-skip resolution: a skip
// is honored ONLY when both the request asks for it AND the ledger opts in via
// its override. A skip requested without the matching opt-in returns the
// canonical 422 business error (ErrSkipNotPermitted) plus the label naming the
// rejected control. The two-key model is asserted exhaustively for fees and
// tracer, including the cross-control independence (one allowed, the other not).
func TestResolveTransactionSkips(t *testing.T) {
	skip := func(fees, tracer bool) *mtransaction.TransactionSkip {
		return &mtransaction.TransactionSkip{Fees: fees, Tracer: tracer}
	}
	settings := func(allowFee, allowTracer bool) mmodel.LedgerSettings {
		return mmodel.LedgerSettings{
			Overrides: mmodel.OverridePolicy{AllowFeeSkip: allowFee, AllowTracerSkip: allowTracer},
		}
	}

	tests := []struct {
		name           string
		input          mtransaction.Transaction
		settings       mmodel.LedgerSettings
		wantFeeSkip    bool
		wantTracerSkip bool
		wantLabel      string
		wantErr        bool
	}{
		{
			name:     "nil skip with no overrides resolves to no skips",
			input:    mtransaction.Transaction{Skip: nil},
			settings: settings(false, false),
		},
		{
			name:     "nil skip even with overrides allowed resolves to no skips",
			input:    mtransaction.Transaction{Skip: nil},
			settings: settings(true, true),
		},
		{
			name:           "fee skip requested and allowed is honored",
			input:          mtransaction.Transaction{Skip: skip(true, false)},
			settings:       settings(true, false),
			wantFeeSkip:    true,
			wantTracerSkip: false,
		},
		{
			name:           "tracer skip requested and allowed is honored",
			input:          mtransaction.Transaction{Skip: skip(false, true)},
			settings:       settings(false, true),
			wantFeeSkip:    false,
			wantTracerSkip: true,
		},
		{
			name:           "both requested and both allowed are honored",
			input:          mtransaction.Transaction{Skip: skip(true, true)},
			settings:       settings(true, true),
			wantFeeSkip:    true,
			wantTracerSkip: true,
		},
		{
			name:      "fee skip requested without opt-in is rejected with label",
			input:     mtransaction.Transaction{Skip: skip(true, false)},
			settings:  settings(false, false),
			wantLabel: "Fee skip not permitted",
			wantErr:   true,
		},
		{
			name:      "tracer skip requested without opt-in is rejected with label",
			input:     mtransaction.Transaction{Skip: skip(false, true)},
			settings:  settings(false, false),
			wantLabel: "Tracer skip not permitted",
			wantErr:   true,
		},
		{
			name:      "fee rejection short-circuits before tracer is evaluated",
			input:     mtransaction.Transaction{Skip: skip(true, true)},
			settings:  settings(false, true),
			wantLabel: "Fee skip not permitted",
			wantErr:   true,
		},
		{
			name:      "tracer rejection surfaces when fee is allowed",
			input:     mtransaction.Transaction{Skip: skip(true, true)},
			settings:  settings(true, false),
			wantLabel: "Tracer skip not permitted",
			wantErr:   true,
		},
		{
			name:           "skip struct present but both false resolves to no skips regardless of overrides",
			input:          mtransaction.Transaction{Skip: skip(false, false)},
			settings:       settings(true, true),
			wantFeeSkip:    false,
			wantTracerSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feeSkip, tracerSkip, label, err := resolveTransactionSkips(tt.input, tt.settings)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.wantLabel, label, "rejected control must be labeled for the single error branch")
				assert.False(t, feeSkip, "no skip may be honored once a rejection occurs")
				assert.False(t, tracerSkip, "no skip may be honored once a rejection occurs")

				// The rejection must be the canonical 422 skip-not-permitted business error.
				var bizErr pkg.UnprocessableOperationError
				require.True(t, errors.As(err, &bizErr), "skip rejection must be a business 422 error, got %T", err)
				assert.Equal(t, constant.ErrSkipNotPermitted.Error(), bizErr.Code)

				return
			}

			require.NoError(t, err)
			assert.Empty(t, label, "no rejection label on the success path")
			assert.Equal(t, tt.wantFeeSkip, feeSkip)
			assert.Equal(t, tt.wantTracerSkip, tracerSkip)
		})
	}
}
