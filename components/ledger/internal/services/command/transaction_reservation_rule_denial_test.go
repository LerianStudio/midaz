// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestReserveTransaction_ClassifiesDenials(t *testing.T) {
	t.Parallel()

	enforce := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}
	advisory := mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed}

	cases := []struct {
		name     string
		settings mmodel.TracerSettings
		result   tracer.ReserveResult
		wantCode string
	}{
		{
			name:     "a limit deny keeps the limit code",
			settings: enforce,
			result:   tracer.ReserveResult{Denied: true, Decision: "DENY", Reason: "limit_exceeded"},
			wantCode: constant.ErrTransactionReservationDenied.Error(),
		},
		{
			name:     "a rule deny gets the rule code",
			settings: enforce,
			result:   tracer.ReserveResult{Denied: true, Decision: "DENY", Reason: "blocked merchant category", MatchedRuleIDs: []uuid.UUID{uuid.New()}},
			wantCode: constant.ErrTransactionReservationRuleDenied.Error(),
		},
		{
			name:     "a rule deny with no reason gets the rule code",
			settings: enforce,
			result:   tracer.ReserveResult{Denied: true, Decision: "DENY"},
			wantCode: constant.ErrTransactionReservationRuleDenied.Error(),
		},
		{
			name:     "a review keeps the review code",
			settings: enforce,
			result:   tracer.ReserveResult{Denied: true, Decision: "REVIEW", Reason: "rule_evaluation_error"},
			wantCode: constant.ErrTransactionReservationReview.Error(),
		},
		{
			name:     "an advisory rule deny proceeds",
			settings: advisory,
			result:   tracer.ReserveResult{Denied: true, Decision: "DENY", Reason: "blocked merchant category"},
		},
		{
			name:     "an advisory limit deny proceeds",
			settings: advisory,
			result:   tracer.ReserveResult{Denied: true, Decision: "DENY", Reason: "limit_exceeded"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, span, ended := recordingSpan(t)
			result := tc.result
			uc := &UseCase{TracerReserver: &stubReserver{result: &result}}

			out := uc.reserveTransaction(ctx, span, &libLog.NopLogger{}, tc.settings,
				uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp,
				reservationTTLDefault, reservationForCreate, false)

			assert.Empty(t, out.Handle.ReservationIDs, "a denied result holds no capacity")

			if tc.wantCode == "" {
				assert.Equal(t, reservationProceed, out.Kind, "advisory observes a denial but never blocks")
				assert.NoError(t, out.Err)

				return
			}

			require.Equal(t, reservationReject, out.Kind)

			var unprocessable pkg.UnprocessableOperationError
			require.ErrorAs(t, out.Err, &unprocessable)
			assert.Equal(t, tc.wantCode, unprocessable.Code)
			assert.NotContains(t, unprocessable.Message, "blocked merchant category",
				"the rule reason is fraud logic and never reaches the client")
			assertSpanNotError(t, ended())
		})
	}
}
