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

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

var errReserveDeadline = fmt.Errorf("%w: %w: %w", tracer.ErrTracerUnavailable, tracer.ErrTracerNoAnswer, context.DeadlineExceeded)

// cancelingReserver cancels the request context while the reserve is in
// flight, the way a client that disconnects mid-hold does, so the engine is
// never reached.
type cancelingReserver struct {
	*stubReserver

	cancel context.CancelFunc
}

func (r *cancelingReserver) Reserve(ctx context.Context, req tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	r.cancel()

	return r.stubReserver.Reserve(ctx, req)
}

func TestReserveTransaction_MarksOnlyUnansweredFailures(t *testing.T) {
	t.Parallel()

	rejected := fmt.Errorf("%w: tracer answered 422", tracer.ErrTracerRejected)
	enforceOpen := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}
	enforceClosed := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed}
	advisory := mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed}

	cases := []struct {
		name           string
		settings       mmodel.TracerSettings
		reserver       *stubReserver
		honoredSkip    bool
		wantUnanswered bool
	}{
		{name: "deadline under fail-open", settings: enforceOpen, reserver: &stubReserver{reserveErr: errReserveDeadline}, wantUnanswered: true},
		{name: "deadline under fail-closed", settings: enforceClosed, reserver: &stubReserver{reserveErr: errReserveDeadline}, wantUnanswered: true},
		{name: "deadline under advisory", settings: advisory, reserver: &stubReserver{reserveErr: errReserveDeadline}, wantUnanswered: true},
		{name: "a failure the tracer answered with", settings: enforceOpen, reserver: &stubReserver{reserveErr: errors.New("rpc error: code = Internal")}},
		{name: "a tracer that answered unavailable", settings: enforceOpen, reserver: &stubReserver{reserveErr: fmt.Errorf("%w: tenant inactive", tracer.ErrTracerUnavailable)}},
		{name: "a call that was never sent", settings: enforceOpen, reserver: &stubReserver{reserveErr: fmt.Errorf("%w: connection refused", tracer.ErrTracerUnavailable)}},
		{name: "a failure the tracer answered with under fail-closed", settings: enforceClosed, reserver: &stubReserver{reserveErr: fmt.Errorf("%w: tenant inactive", tracer.ErrTracerUnavailable)}},
		{name: "a refused request", settings: enforceOpen, reserver: &stubReserver{reserveErr: rejected}},
		{name: "a refused request under advisory", settings: advisory, reserver: &stubReserver{reserveErr: rejected}},
		{name: "a denied result", settings: enforceOpen, reserver: &stubReserver{result: &tracer.ReserveResult{Denied: true, Decision: "DENY", Reason: "limit_exceeded"}}},
		{name: "an allowed result", settings: enforceOpen, reserver: &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{uuid.New()}}}},
		{name: "an honored skip", settings: enforceOpen, reserver: &stubReserver{reserveErr: errReserveDeadline}, honoredSkip: true},
		{name: "mode off", settings: mmodel.TracerSettings{Mode: mmodel.TracerModeOff}, reserver: &stubReserver{reserveErr: errReserveDeadline}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, span, logger := anchorDeps()
			uc := &UseCase{TracerReserver: tc.reserver}
			queue := withImmediateUnansweredSettles(t, uc)
			transactionID := uuid.New()

			out := uc.reserveTransaction(ctx, span, logger, tc.settings,
				transactionID, decimal.NewFromInt(1000), "BRL", fixedReserveAccount, nil, fixedReserveTimestamp,
				reservationTTLDefault, reservationForCreate, tc.honoredSkip, "")

			assert.Equal(t, tc.wantUnanswered, out.Handle.Unanswered)

			uc.confirmReservations(ctx, span, logger, out.Handle)
			uc.releaseReservations(ctx, span, logger, out.Handle)
			queue.drain()

			if !tc.wantUnanswered {
				assert.Empty(t, tc.reserver.confirmedTransactions(), "an answered or unsent reserve is never settled by transaction")
				assert.Empty(t, tc.reserver.releasedTransactions())
			}

			if tc.wantUnanswered {
				assert.Empty(t, out.Handle.ReservationIDs)
				assert.Equal(t, transactionID, out.Handle.TransactionID, "the settle is addressed by transaction")
				assert.True(t, decimal.NewFromInt(1000).Equal(out.Handle.Amount))
				assert.Equal(t, "BRL", out.Handle.Asset)
			}
		})
	}
}

func TestUnansweredReservationSettle_ByTransaction(t *testing.T) {
	t.Parallel()

	transactionID := uuid.New()
	answeredID := uuid.New()
	unanswered := reservationHandle{TransactionID: transactionID, Amount: decimal.NewFromInt(1000), Asset: "BRL", Unanswered: true}
	answered := reservationHandle{TransactionID: transactionID, ReservationIDs: []uuid.UUID{answeredID}}

	cases := []struct {
		name        string
		handle      reservationHandle
		nilReserver bool
		wantByTxn   bool
		wantByID    []uuid.UUID
	}{
		{name: "an unanswered reserve settles by transaction", handle: unanswered, wantByTxn: true},
		{name: "an answered handle settles by id only", handle: answered, wantByID: []uuid.UUID{answeredID}},
		{name: "a nil reserver settles nothing", handle: unanswered, nilReserver: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := tracetest.NewSpanRecorder()
			ctx := context.Background()
			_, span := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("t").Start(ctx, "test")
			reserver := &stubReserver{confirmByTxnOutcome: tracer.ConfirmOutcome{Confirmed: 1}}
			uc := &UseCase{TracerReserver: reserver}
			queue := withImmediateUnansweredSettles(t, uc)

			if tc.nilReserver {
				uc.TracerReserver = nil
			}

			uc.confirmReservations(ctx, span, &libLog.NopLogger{}, tc.handle)
			uc.releaseReservations(ctx, span, &libLog.NopLogger{}, tc.handle)
			queue.drain()
			span.End()

			var actions []string

			for _, ended := range recorder.Ended() {
				for _, event := range ended.Events() {
					if event.Name != unansweredSettleSpanEvent {
						continue
					}

					for _, attr := range event.Attributes {
						assert.NotEqual(t, "app.reservation.amount", string(attr.Key), "the settle event carries no amount")

						if attr.Key == "app.reservation.action" {
							actions = append(actions, attr.Value.AsString())
						}
					}
				}
			}

			if tc.wantByTxn {
				assert.Equal(t, []uuid.UUID{transactionID}, reserver.confirmedTransactions())
				assert.Equal(t, []uuid.UUID{transactionID}, reserver.releasedTransactions())
				assert.Equal(t, []string{reservationActionConfirm, reservationActionRelease}, actions)
			} else {
				assert.Empty(t, reserver.confirmedTransactions())
				assert.Empty(t, reserver.releasedTransactions())
				assert.Empty(t, actions)
			}

			assert.Equal(t, tc.wantByID, reserver.confirmed())
			assert.Equal(t, tc.wantByID, reserver.released())
		})
	}
}

func TestCreateTransactionV2_SettlesUnansweredReserve(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	rejected := fmt.Errorf("%w: tracer answered 422", tracer.ErrTracerRejected)
	insufficientFunds := func(execution EngineExecution) error {
		return &accounting.Failure{
			Code: accounting.FailureInsufficientFunds, TransactionIndex: 0, PostingIndex: 0,
			BalanceRef: execution.Execution.Transactions[0].Postings[0].BalanceRef,
		}
	}
	allowedID := uuid.MustParse("a1a1a1a1-a1a1-4a1a-8a1a-a1a1a1a1a1a1")

	cases := []struct {
		name         string
		settings     mmodel.TracerSettings
		reserver     *stubReserver
		pending      bool
		engine       func(t *testing.T) Engine
		cancelAtHold bool
		wantCode     string
		wantErrIs    error
		wantErr      bool
		wantConfirm  bool
		wantRelease  bool
		wantByIDOnly []uuid.UUID
	}{
		{
			name:        "deadline under fail-open then applied confirms by transaction",
			settings:    mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			reserver:    &stubReserver{reserveErr: errReserveDeadline},
			wantConfirm: true,
		},
		{
			name:        "deadline under advisory then applied confirms by transaction",
			settings:    mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureClosed},
			reserver:    &stubReserver{reserveErr: errReserveDeadline},
			wantConfirm: true,
		},
		{
			name:        "deadline under fail-closed releases by transaction and rejects",
			settings:    mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
			reserver:    &stubReserver{reserveErr: errReserveDeadline},
			wantCode:    constant.ErrTransactionReservationUnavailable.Error(),
			wantErr:     true,
			wantRelease: true,
		},
		{
			name:     "deadline under fail-open then an engine refusal releases by transaction",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			reserver: &stubReserver{reserveErr: errReserveDeadline},
			engine: func(t *testing.T) Engine {
				return &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}, before: insufficientFunds}
			},
			wantCode:    constant.ErrInsufficientFunds.Error(),
			wantErr:     true,
			wantRelease: true,
		},
		{
			name:     "deadline under fail-open then an indeterminate engine outcome settles nothing",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			reserver: &stubReserver{reserveErr: errReserveDeadline},
			engine: func(*testing.T) Engine {
				return &createEngineErrorExecutor{err: errors.New("transport outcome unknown")}
			},
			wantErr: true,
		},
		{
			name:     "deadline then a request cancelled before the engine ran releases by transaction",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			reserver: &stubReserver{reserveErr: errReserveDeadline},
			engine: func(*testing.T) Engine {
				return &createEngineErrorExecutor{err: errors.New("the engine must not run")}
			},
			cancelAtHold: true,
			wantErrIs:    context.Canceled,
			wantErr:      true,
			wantRelease:  true,
		},
		{
			name:     "deadline on a pending create then an engine refusal releases by transaction",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			reserver: &stubReserver{reserveErr: errReserveDeadline},
			pending:  true,
			engine: func(t *testing.T) Engine {
				return &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}, before: insufficientFunds}
			},
			wantCode:    constant.ErrInsufficientFunds.Error(),
			wantErr:     true,
			wantRelease: true,
		},
		{
			name:     "deadline on a pending create leaves the settle to commit or cancel",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			reserver: &stubReserver{reserveErr: errReserveDeadline},
			pending:  true,
		},
		{
			name:     "an unavailable answer under fail-open needs no settle",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			reserver: &stubReserver{reserveErr: fmt.Errorf("%w: tenant inactive", tracer.ErrTracerUnavailable)},
		},
		{
			name:     "a refused request under enforce needs no settle",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
			reserver: &stubReserver{reserveErr: rejected},
			wantCode: constant.ErrTransactionReservationRejected.Error(),
			wantErr:  true,
		},
		{
			name:     "a refused request under advisory needs no settle",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureOpen},
			reserver: &stubReserver{reserveErr: rejected},
		},
		{
			name:     "a denied result needs no settle",
			settings: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			reserver: &stubReserver{result: &tracer.ReserveResult{Denied: true, Decision: "DENY", Reason: "blocked merchant category"}},
			wantCode: constant.ErrTransactionReservationRuleDenied.Error(),
			wantErr:  true,
		},
		{
			name:         "a handle keeps the by-id confirm",
			settings:     mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
			reserver:     &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{allowedID}}},
			wantByIDOnly: []uuid.UUID{allowedID},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rejectsBeforeEngine := tc.wantErr && tc.engine == nil
			tc.reserver.confirmByTxnOutcome = tracer.ConfirmOutcome{Confirmed: 1}

			ctrl := gomock.NewController(t)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil).Times(1)

			var idempotencySet chan struct{}

			switch {
			case rejectsBeforeEngine || tc.wantRelease:
				redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			case !tc.wantErr:
				idempotencySet = make(chan struct{})
				redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
					func(context.Context, string, string, time.Duration) error {
						close(idempotencySet)
						return nil
					},
				).Times(1)
			}

			organizationID := uuid.MustParse("c1c1c1c1-c1c1-4c1c-8c1c-c1c1c1c1c1c1")
			ledgerID := uuid.MustParse("d1d1d1d1-d1d1-4d1d-8d1d-d1d1d1d1d1d1")
			settings := mmodel.LedgerSettings{}
			settings.Tracer = tc.settings
			reader := &createEngineReader{settings: settings, balances: []*mmodel.Balance{
				translationBalance(organizationID, ledgerID, "e1e1e1e1-e1e1-4e1e-8e1e-e1e1e1e1e1e1", "@source", constant.DefaultBalanceKey),
				translationBalance(organizationID, ledgerID, "f1f1f1f1-f1f1-4f1f-8f1f-f1f1f1f1f1f1", "@target", constant.DefaultBalanceKey),
			}}

			var engine Engine = &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}}
			if tc.engine != nil {
				engine = tc.engine(t)
			}

			status := constant.CREATED
			persisted := constant.APPROVED

			if tc.pending {
				status = constant.PENDING
				persisted = constant.PENDING
			}

			ctx, cancel := context.WithCancel(tmcore.ContextWithTenantID(context.Background(), "tenant-unanswered"))
			defer cancel()

			var reserver TracerReserver = tc.reserver
			if tc.cancelAtHold {
				reserver = &cancelingReserver{stubReserver: tc.reserver, cancel: cancel}
			}

			uc := &UseCase{
				TransactionRedisRepo: redisRepo, TransactionReader: reader, Engine: engine,
				AppliedTransactionCompleter: &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: persisted}},
				EngineRecoveryAcknowledger:  &recordingEngineRecoveryAcknowledger{},
				TracerReserver:              reserver,
			}
			settles := withImmediateUnansweredSettles(t, uc)

			input := createEngineTransaction(time.Date(2026, time.September, 30, 10, 0, 0, 0, time.UTC))
			if tc.pending {
				input.Pending = true
				input.TransactionDate = nil
			}

			got, _, err := uc.CreateTransactionV2(
				ctx,
				CreateTransactionV2Input{
					OrganizationID: organizationID, LedgerID: ledgerID, Transaction: input,
					TransactionStatus: status, IdempotencyTTL: time.Minute,
				},
			)

			if tc.wantErr {
				require.Error(t, err)

				if tc.wantErrIs != nil {
					require.ErrorIs(t, err, tc.wantErrIs)
				}

				if tc.wantCode != "" {
					var business pkg.UnprocessableOperationError
					var unavailable pkg.ServiceUnavailableError

					switch {
					case errors.As(err, &business):
						assert.Equal(t, tc.wantCode, business.Code)
					case errors.As(err, &unavailable):
						assert.Equal(t, tc.wantCode, unavailable.Code)
					default:
						assert.Contains(t, err.Error(), tc.wantCode)
					}
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)

				select {
				case <-idempotencySet:
				case <-time.After(time.Second):
					t.Fatal("success did not populate the idempotency value")
				}
			}

			settles.drain()

			require.Equal(t, 1, tc.reserver.reserves())
			transactionID := tc.reserver.reserveRequests()[0].TransactionID

			if tc.wantConfirm {
				assert.Equal(t, []uuid.UUID{transactionID}, tc.reserver.confirmedTransactions(), "a movement that applied counts the spend once")
			} else {
				assert.Empty(t, tc.reserver.confirmedTransactions())
			}

			if tc.wantRelease {
				assert.Equal(t, []uuid.UUID{transactionID}, tc.reserver.releasedTransactions(), "a movement that never applied returns the capacity once")
			} else {
				assert.Empty(t, tc.reserver.releasedTransactions())
			}

			assert.Equal(t, tc.wantByIDOnly, tc.reserver.confirmed())
			assert.Empty(t, tc.reserver.released())
		})
	}
}
