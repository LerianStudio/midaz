// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestReserveTransaction_MarksRevert(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		purpose reservationPurpose
		want    bool
	}{
		{name: "a revert run marks the reservation as a revert", purpose: reservationForRevert, want: true},
		{name: "a create run does not", purpose: reservationForCreate, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tracerCtx, sp, logger := anchorDeps()
			capturing := &capturingReserver{result: &tracer.ReserveResult{}}
			uc := &UseCase{TracerReserver: capturing}

			uc.reserveTransaction(tracerCtx, sp, logger,
				mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
				uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp,
				reservationTTLDefault, tc.purpose, false, "")

			assert.Equal(t, tc.want, capturing.lastReq.Revert)
		})
	}
}

func TestReservationPurposeForAction(t *testing.T) {
	t.Parallel()

	assert.Equal(t, reservationForRevert, reservationPurposeForAction(constant.ActionRevert))

	for _, action := range []string{"", constant.ActionDirect, constant.ActionHold, constant.ActionCommit, constant.ActionCancel} {
		assert.Equal(t, reservationForCreate, reservationPurposeForAction(action), "action %q", action)
	}
}

func TestReserveTransaction_Review(t *testing.T) {
	t.Parallel()

	ruleID := uuid.New()

	t.Run("enforce rejects with the review code", func(t *testing.T) {
		t.Parallel()

		ctx, span, ended := recordingSpan(t)
		reserver := &stubReserver{result: &tracer.ReserveResult{
			Denied: true, Decision: "REVIEW", Reason: "manual review", MatchedRuleIDs: []uuid.UUID{ruleID},
		}}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(ctx, span, &libLog.NopLogger{},
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false, "")

		require.Equal(t, reservationReject, out.Kind)

		var unprocessable pkg.UnprocessableOperationError
		require.ErrorAs(t, out.Err, &unprocessable)
		assert.Equal(t, constant.ErrTransactionReservationReview.Error(), unprocessable.Code)

		spans := ended()
		assertSpanNotError(t, spans)

		attrs := spanAttributes(spans)
		assert.Equal(t, "REVIEW", attrs["app.tracer.decision"].AsString())
		assert.Equal(t, int64(1), attrs["app.tracer.matched_rule_count"].AsInt64())
	})

	t.Run("advisory proceeds", func(t *testing.T) {
		t.Parallel()

		tracerCtx, sp, logger := anchorDeps()
		reserver := &stubReserver{result: &tracer.ReserveResult{Denied: true, Decision: "REVIEW"}}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(tracerCtx, sp, logger,
			mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false, "")

		assert.Equal(t, reservationProceed, out.Kind, "advisory observes a review but never blocks")
		assert.Empty(t, out.Handle.ReservationIDs)
	})

	t.Run("enforce deny keeps the limit code", func(t *testing.T) {
		t.Parallel()

		ctx, span, ended := recordingSpan(t)
		reserver := &stubReserver{result: &tracer.ReserveResult{Denied: true, Decision: "DENY", Reason: "limit_exceeded"}}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(ctx, span, &libLog.NopLogger{},
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false, "")

		require.Equal(t, reservationReject, out.Kind)

		var unprocessable pkg.UnprocessableOperationError
		require.ErrorAs(t, out.Err, &unprocessable)
		assert.Equal(t, constant.ErrTransactionReservationDenied.Error(), unprocessable.Code)
		assertSpanNotError(t, ended())
	})

	t.Run("allow records the decision on the span", func(t *testing.T) {
		t.Parallel()

		ctx, span, ended := recordingSpan(t)
		reserver := &stubReserver{result: &tracer.ReserveResult{Decision: "ALLOW", ReservationIDs: []uuid.UUID{uuid.New()}}}
		uc := &UseCase{TracerReserver: reserver}

		out := uc.reserveTransaction(ctx, span, &libLog.NopLogger{},
			mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false, "")

		require.Equal(t, reservationProceed, out.Kind)

		attrs := spanAttributes(ended())
		assert.Equal(t, "ALLOW", attrs["app.tracer.decision"].AsString())
		assert.Equal(t, int64(0), attrs["app.tracer.matched_rule_count"].AsInt64())
	})
}

func TestReserveTransaction_TracerRejected(t *testing.T) {
	t.Parallel()

	rejected := fmt.Errorf("%w: tracer answered 422", tracer.ErrTracerRejected)

	cases := []struct {
		name        string
		settings    mmodel.TracerSettings
		wantKind    reservationOutcomeKind
		wantRejects bool
	}{
		{
			name:     "advisory proceeds",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
			wantKind: reservationProceed,
		},
		{
			name:        "enforce with open posture rejects",
			settings:    mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			wantKind:    reservationReject,
			wantRejects: true,
		},
		{
			name:        "enforce with closed posture rejects with the refusal code",
			settings:    mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
			wantKind:    reservationReject,
			wantRejects: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, span, ended := recordingSpan(t)
			reserver := &stubReserver{reserveErr: rejected}
			uc := &UseCase{TracerReserver: reserver}

			out := uc.reserveTransaction(ctx, span, &libLog.NopLogger{}, tc.settings,
				uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false, "")

			require.Equal(t, tc.wantKind, out.Kind)
			assert.Empty(t, out.Handle.ReservationIDs)

			if tc.wantRejects {
				var unprocessable pkg.UnprocessableOperationError
				require.ErrorAs(t, out.Err, &unprocessable, "a refusal is a 422, never the 503 of an unavailable tracer")
				assert.Equal(t, constant.ErrTransactionReservationRejected.Error(), unprocessable.Code)
			} else {
				assert.NoError(t, out.Err)
			}

			spans := ended()
			assertSpanNotError(t, spans)
			assert.False(t, spanHasSkippedMarker(spans),
				"a refusal is not an outage: the reservation must not be recorded as skipped")
		})
	}
}

// assertSpanNotError asserts no ended span carries an Error status: a tracer
// refusal is a business outcome, recorded as an event on a green span.
func assertSpanNotError(t *testing.T, spans []sdktrace.ReadOnlySpan) {
	t.Helper()

	require.NotEmpty(t, spans)

	for _, s := range spans {
		assert.NotEqual(t, codes.Error, s.Status().Code, "span %q must not be marked as an error", s.Name())
	}
}
