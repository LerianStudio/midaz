// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// unauthorizedErr is what the gRPC client returns when the tracer rejects the
// ledger's credential.
var unauthorizedErr = fmt.Errorf("%w: %w", tracer.ErrTracerUnauthorized, status.Error(grpccodes.Unauthenticated, "invalid credential"))

// credentialNotSentErr is what the gRPC client returns when the ledger could not
// obtain its seam credential, so the call never left.
var credentialNotSentErr = fmt.Errorf("%w: %w: %w", tracer.ErrTracerUnavailable, tracer.ErrTracerCredentialUnavailable, errors.New("access manager unreachable"))

// credentialRejectedSeries collects tracer_reservation_credential_rejected_total
// keyed by "<operation>/<reason>", asserting the series carries nothing else.
func credentialRejectedSeries(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	values := make(map[string]int64)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "tracer_reservation_credential_rejected_total" {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				attrs := dp.Attributes.ToSlice()
				require.Len(t, attrs, 2, "the series carries only the operation and reason attributes")

				labels := map[string]string{}
				for _, attr := range attrs {
					labels[string(attr.Key)] = attr.Value.AsString()
				}

				require.Contains(t, labels, "operation")
				require.Contains(t, labels, "reason")
				require.Contains(t, []string{"rejected", "not_sent"}, labels["reason"])

				values[labels["operation"]+"/"+labels["reason"]] = dp.Value
			}
		}
	}

	return values
}

func anySpanRed(spans []sdktrace.ReadOnlySpan) bool {
	for _, s := range spans {
		if s.Status().Code == codes.Error {
			return true
		}
	}

	return false
}

func TestReserveTransaction_CredentialFailure(t *testing.T) {
	t.Parallel()

	type expectation struct {
		reject      bool
		code        string
		wantSkipped bool
	}

	cases := []struct {
		name       string
		err        error
		wantSeries string
		settings   mmodel.TracerSettings
		want       expectation
	}{
		{
			name:       "rejected: advisory proceeds even under a closed posture",
			err:        unauthorizedErr,
			wantSeries: "reserve/rejected",
			settings:   mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
		},
		{
			name:       "rejected: enforce with a closed posture rejects with 0536",
			err:        unauthorizedErr,
			wantSeries: "reserve/rejected",
			settings:   mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
			want:       expectation{reject: true, code: constant.ErrTransactionReservationUnauthorized.Error()},
		},
		{
			name:       "rejected: enforce with an open posture proceeds with the reservation skipped",
			err:        unauthorizedErr,
			wantSeries: "reserve/rejected",
			settings:   mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			want:       expectation{wantSkipped: true},
		},
		{
			name:       "not sent: advisory proceeds even under a closed posture",
			err:        credentialNotSentErr,
			wantSeries: "reserve/not_sent",
			settings:   mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
		},
		{
			name:       "not sent: enforce with a closed posture rejects as an unavailable tracer",
			err:        credentialNotSentErr,
			wantSeries: "reserve/not_sent",
			settings:   mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
			want:       expectation{reject: true, code: constant.ErrTransactionReservationUnavailable.Error()},
		},
		{
			name:       "not sent: enforce with an open posture proceeds with the reservation skipped",
			err:        credentialNotSentErr,
			wantSeries: "reserve/not_sent",
			settings:   mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			want:       expectation{wantSkipped: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reader, factory := newReaderFactory(t)
			ctx, span, ended := recordingSpan(t)
			logger := &capturingLogger{}
			transactionID := uuid.New()
			uc := &UseCase{TracerReserver: &stubReserver{reserveErr: tc.err}, MetricsFactory: factory}

			out := uc.reserveTransaction(ctx, span, logger, tc.settings,
				transactionID, decimal.RequireFromString(alreadyReleasedAmount), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp,
				reservationTTLDefault, reservationForCreate, false)

			assert.False(t, out.Handle.Unanswered, "nothing was evaluated, so nothing is left to settle")
			assert.Empty(t, out.Handle.ReservationIDs)

			if tc.want.reject {
				require.Equal(t, reservationReject, out.Kind)

				var unavailable pkg.ServiceUnavailableError
				require.ErrorAs(t, out.Err, &unavailable)
				assert.Equal(t, tc.want.code, unavailable.Code)
			} else {
				assert.Equal(t, reservationProceed, out.Kind)
				assert.NoError(t, out.Err)
			}

			spans := ended()
			assert.True(t, anySpanRed(spans), "a credential failure is a technical failure")
			assert.Equal(t, tc.want.wantSkipped, spanHasSkippedMarker(spans))

			errorsLogged := logger.atLevelOrMoreSevere(libLog.LevelError)
			require.Len(t, errorsLogged, 1, "one Error log per credential failure")
			assert.Contains(t, errorsLogged[0].Fields, transactionID.String())
			assert.NotContains(t, rendered(logger.snapshot()), alreadyReleasedAmount, "no amount reaches the log")

			assert.Equal(t, map[string]int64{tc.wantSeries: 1}, credentialRejectedSeries(t, reader))
		})
	}
}

func TestReserveTransaction_CredentialRejectedIsNotUnavailable(t *testing.T) {
	t.Parallel()

	ctx, span, _ := recordingSpan(t)
	uc := &UseCase{TracerReserver: &stubReserver{reserveErr: unauthorizedErr}}

	out := uc.reserveTransaction(ctx, span, &libLog.NopLogger{},
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
		uuid.New(), decimal.NewFromInt(10), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp,
		reservationTTLDefault, reservationForCreate, false)

	require.Equal(t, reservationReject, out.Kind)

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, out.Err, &unavailable)
	assert.NotEqual(t, constant.ErrTransactionReservationUnavailable.Error(), unavailable.Code,
		"a rejected credential never reads as an unreachable tracer")
}

func TestReservationTransition_CredentialFailureIsRetriedLoggedAndCounted(t *testing.T) {
	type operation struct {
		name     string
		reserver func(err error) *stubReserver
		invoke   func(uc *UseCase, ctx context.Context, span trace.Span, logger libLog.Logger, transactionID uuid.UUID)
		attempts func(r *stubReserver) int
	}

	settings := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}

	byIDHandle := func(transactionID uuid.UUID) reservationHandle {
		handle := alreadyReleasedIdentity(transactionID)
		handle.ReservationIDs = []uuid.UUID{uuid.New()}

		return handle
	}

	operations := []operation{
		{
			name:     reservationConfirmOperationByID,
			reserver: func(err error) *stubReserver { return &stubReserver{confirmErr: err} },
			invoke: func(uc *UseCase, ctx context.Context, span trace.Span, logger libLog.Logger, transactionID uuid.UUID) {
				uc.confirmReservations(ctx, span, logger, byIDHandle(transactionID))
			},
			attempts: func(r *stubReserver) int { return len(r.confirmed()) },
		},
		{
			name:     reservationReleaseOperationByID,
			reserver: func(err error) *stubReserver { return &stubReserver{releaseErr: err} },
			invoke: func(uc *UseCase, ctx context.Context, span trace.Span, logger libLog.Logger, transactionID uuid.UUID) {
				uc.releaseReservations(ctx, span, logger, byIDHandle(transactionID))
			},
			attempts: func(r *stubReserver) int { return len(r.released()) },
		},
		{
			name:     reservationConfirmOperationByTransaction,
			reserver: func(err error) *stubReserver { return &stubReserver{confirmByTxnErr: err} },
			invoke: func(uc *UseCase, ctx context.Context, span trace.Span, logger libLog.Logger, transactionID uuid.UUID) {
				uc.confirmReservationsByTransaction(ctx, span, logger, settings, alreadyReleasedIdentity(transactionID), false)
			},
			attempts: func(r *stubReserver) int { return len(r.confirmedTransactions()) },
		},
		{
			name:     reservationReleaseOperationByTransaction,
			reserver: func(err error) *stubReserver { return &stubReserver{releaseByTxnErr: err} },
			invoke: func(uc *UseCase, ctx context.Context, span trace.Span, logger libLog.Logger, transactionID uuid.UUID) {
				uc.releaseReservationsByTransaction(ctx, span, logger, settings, alreadyReleasedIdentity(transactionID), false)
			},
			attempts: func(r *stubReserver) int { return len(r.releasedTransactions()) },
		},
	}

	reasons := []struct {
		reason string
		err    error
		msg    string
	}{
		{reason: "rejected", err: unauthorizedErr, msg: reservationCredentialRejectedTransitionMsg},
		{reason: "not_sent", err: credentialNotSentErr, msg: reservationCredentialUnavailableTransitionMsg},
	}

	for _, op := range operations {
		for _, rc := range reasons {
			t.Run(op.name+"/"+rc.reason, func(t *testing.T) {
				withFastSharedRetrier(t)

				reader, factory := newReaderFactory(t)
				ctx, span, _ := recordingSpan(t)
				logger := &capturingLogger{}
				transactionID := uuid.New()
				reserver := op.reserver(rc.err)
				uc := &UseCase{TracerReserver: reserver, MetricsFactory: factory}

				op.invoke(uc, ctx, span, logger, transactionID)

				sharedReservationRetrier.wait()

				assert.Greater(t, op.attempts(reserver), 1, "a credential failure goes through the retry transport")

				var inline []capturedLogLine

				for _, line := range logger.atLevelOrMoreSevere(libLog.LevelError) {
					if line.Msg == rc.msg {
						inline = append(inline, line)
					}
				}

				require.Len(t, inline, 1, "the inline failure is logged once at Error")
				assert.Contains(t, inline[0].Fields, transactionID.String())

				for _, line := range logger.snapshot() {
					assert.NotEqual(t, libLog.LevelWarn, line.Level, "a credential failure is not a Warn: %s", line.Msg)
				}

				assert.Equal(t, map[string]int64{op.name + "/" + rc.reason: 1}, credentialRejectedSeries(t, reader))
			})
		}
	}
}

// blockingMinter holds every mint until the test ends.
type blockingMinter struct{ release chan struct{} }

func (m blockingMinter) GetApplicationToken(ctx context.Context, _, _ string) (string, error) {
	select {
	case <-m.release:
	case <-ctx.Done():
	}

	return "", errors.New("mint released")
}

func TestReserveTransaction_AbandonedTokenWaitIsAnUnavailableTracer(t *testing.T) {
	t.Parallel()

	minter := blockingMinter{release: make(chan struct{})}
	t.Cleanup(func() { close(minter.release) })

	src, err := tracer.NewM2MTokenSource(minter, tracer.NewStaticCredentials("ledger", "s3cret"))
	require.NoError(t, err)

	client, err := tracer.NewTracerGRPCClient("passthrough:///tracer-unused",
		tracer.WithM2MCredentials(src), tracer.WithGRPCOperationTimeout(20*time.Millisecond))
	require.NoError(t, err)

	t.Cleanup(func() { _ = client.Close() })

	reader, factory := newReaderFactory(t)
	ctx, span, _ := recordingSpan(t)
	logger := &capturingLogger{}
	uc := &UseCase{TracerReserver: client, MetricsFactory: factory}

	out := uc.reserveTransaction(ctx, span, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(10), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp,
		reservationTTLDefault, reservationForCreate, false)

	assert.Equal(t, reservationProceed, out.Kind)
	assert.False(t, out.Handle.Unanswered, "nothing was sent, so nothing is left to settle")
	assert.Empty(t, logger.atLevelOrMoreSevere(libLog.LevelError), "a caller that stopped waiting for its token is not a credential failure")
	assert.Empty(t, credentialRejectedSeries(t, reader), "and is not counted as one")
}
