// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestCreateAtomicContextTracerCompletesAllMembers(t *testing.T) {
	for _, scenario := range []string{"allow", "second denies", "accounting refusal", "protected refusal", "cleanup unavailable", "accounting unknown"} {
		t.Run(scenario, func(t *testing.T) {
			withFastSharedRetrier(t)
			ctrl := gomock.NewController(t)
			client, loader := NewMockContextTracerReserver(ctrl), NewMockTracerFactsLoader(ctrl)
			engine := &applyingAtomicTransactionBatchEngine{t: t}
			refusing := &refusingAtomicTransactionBatchEngine{transactionIndex: 1}
			unknown := &scriptedEngine{responses: []engineResponse{{err: &indeterminateAtomicTransactionBatchError{cause: errors.New("response lost")}}}}
			repository := &atomicTransactionBatchClaimRepositoryFake{}
			var selected Engine = engine
			switch scenario {
			case "accounting refusal", "protected refusal", "cleanup unavailable":
				selected = refusing
			case "accounting unknown":
				selected = unknown
			}
			if scenario == "protected refusal" {
				repository.abortErr = txRedis.ErrAtomicTransactionBatchRefusalProtected
			}
			if scenario == "cleanup unavailable" {
				repository.abortErr = errors.New("redis unavailable")
			}
			uc, input, transactionIDs, _ := atomicTransactionBatchExecutionFixture(t, repository, selected, &atomicTransactionBatchTracerFake{})
			reader, ok := uc.TransactionReader.(*atomicTransactionBatchSettingsReader)
			require.True(t, ok)
			reader.settings.Tracer = mmodel.DefaultLedgerSettings().Tracer
			reader.settings.Tracer.Mode, reader.settings.Tracer.ValidationMode, reader.settings.Tracer.FailPosture = "enforce", "rules-and-limits", "closed"
			bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
			cfg := ContextTracerConfig{Bounds: bounds, MaxReservations: 100, AdmissionTimeout: 250 * time.Millisecond}
			var err error
			uc.ContextTracer, err = NewContextTracerCoordinator(client, loader, cfg, uc.Clock)
			require.NoError(t, err)
			loader.EXPECT().EvaluationContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _, _ uuid.UUID, entries []traceradapter.PreparedEntry) (tracercontract.Context, error) {
				facts := tracercontract.Context{Accounts: []tracercontract.Account{}, Entries: []tracercontract.Entry{}}
				for _, entry := range entries {
					blocked := false
					asset := entry.AssetCode
					facts.Accounts = append(facts.Accounts, tracercontract.Account{ID: entry.AccountID, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: asset})
					facts.Entries = append(facts.Entries, tracercontract.Entry{AccountID: entry.AccountID, Direction: entry.Direction, Amount: tracercontract.Amount(entry.Amount.String()), Asset: asset})
				}
				return facts, nil
			}).Times(2)
			var reserved []uuid.UUID
			client.EXPECT().Reserve(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
				require.Empty(t, engine.executions, "every member reserves before the single batch execution")
				reserved = append(reserved, request.TransactionID)
				decision, reason := tracercontract.DecisionAllow, tracercontract.ReasonLimitsSatisfied
				if scenario == "second denies" && request.TransactionID == transactionIDs[1] {
					decision, reason = tracercontract.DecisionDeny, tracercontract.ReasonRuleDeny
				}
				return &tracercontract.ReserveResult{ContractRevision: request.ContractRevision, TransactionID: request.TransactionID, EvaluationID: uuid.NewSHA1(request.TransactionID, []byte("evaluation")), Decision: decision, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{reason}}, nil
			}).Times(2)
			var completed []uuid.UUID
			switch scenario {
			case "allow":
				client.EXPECT().ConfirmByTransaction(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
					assert.Len(t, engine.executions, 1, "confirm follows the batch execution")
					completed = append(completed, transactionID)
					return completionResult(transactionID, "CONFIRMED"), nil
				}).Times(2)
			case "second denies", "accounting refusal":
				client.EXPECT().ReleaseByTransaction(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
					completed = append(completed, transactionID)
					return completionResult(transactionID, "RELEASED"), nil
				}).Times(2)
			}
			_, err = uc.CreateAtomicTransactionBatchV2(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), input)
			require.Equal(t, transactionIDs, reserved)
			switch scenario {
			case "allow", "accounting refusal":
				require.Equal(t, transactionIDs, completed)
			case "second denies":
				require.ElementsMatch(t, transactionIDs, completed)
			default:
				require.Empty(t, completed, "an unprotected or unknown outcome leaves reservations to the Tracer TTL")
			}
			if scenario == "allow" {
				require.NoError(t, err)
				require.Len(t, engine.executions, 1)
			} else {
				require.Error(t, err)
				require.Empty(t, engine.executions)
				if selected == refusing {
					require.Len(t, refusing.executions, 1)
					require.Equal(t, 1, repository.aborts)
				} else if selected == unknown {
					require.Len(t, unknown.requests, 1)
					require.Zero(t, repository.aborts)
				}
			}
		})
	}
}

func TestReserveAtomicContextBatchUsesEachLedgerScope(t *testing.T) {
	ctrl := gomock.NewController(t)
	client, loader := NewMockContextTracerReserver(ctrl), NewMockTracerFactsLoader(ctrl)
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-0000000000f1")
	ledgerIDs := []uuid.UUID{
		uuid.MustParse("01994f13-29b7-7000-8000-0000000000f2"),
		uuid.MustParse("01994f13-29b7-7000-8000-0000000000f3"),
	}
	now := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	run := atomicTransactionBatchTracerTestRun(2)

	for index := range run.items {
		item := &run.items[index]
		item.organizationID = organizationID
		item.ledgerID = ledgerIDs[index]
		balance := item.prepared.pool.ExplicitBalances[0]
		balance.AccountID = uuid.NewSHA1(organizationID, []byte{byte(index)}).String()
		balance.AssetCode = item.input.Send.Asset
		balance.AccountType = "deposit"
		entryAlias := "0#" + balance.Alias + "#" + balance.Key
		item.input.Send.Source.From = []mtransaction.FromTo{{AccountAlias: entryAlias}}
		item.validate = &mtransaction.Responses{From: map[string]mtransaction.Amount{
			entryAlias: {Value: item.input.Send.Value, Asset: item.input.Send.Asset},
		}}
		item.ledgerSettings.Tracer = mmodel.TracerSettings{
			Mode:           mmodel.TracerModeEnforce,
			FailPosture:    mmodel.TracerFailPostureClosed,
			ValidationMode: string(tracercontract.ValidationRulesAndLimits),
			TimeoutMs:      100 + index*100,
		}
	}

	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	coordinator, err := NewContextTracerCoordinator(client, loader, ContextTracerConfig{
		Bounds: bounds, MaxReservations: 100, AdmissionTimeout: time.Second,
	}, func() time.Time { return now })
	require.NoError(t, err)
	uc := &UseCase{ContextTracer: coordinator}

	for index, ledgerID := range ledgerIDs {
		item := &run.items[index]
		loader.EXPECT().EvaluationContext(gomock.Any(), organizationID, ledgerID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _, _ uuid.UUID, entries []traceradapter.PreparedEntry) (tracercontract.Context, error) {
				facts := tracercontract.Context{}
				for _, entry := range entries {
					blocked := false
					asset := entry.AssetCode
					facts.Accounts = append(facts.Accounts, tracercontract.Account{ID: entry.AccountID, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: asset})
					facts.Entries = append(facts.Entries, tracercontract.Entry{AccountID: entry.AccountID, Direction: entry.Direction, Amount: tracercontract.Amount(entry.Amount.String()), Asset: asset})
				}

				return facts, nil
			},
		)
		client.EXPECT().Reserve(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
			require.Equal(t, item.transactionID, request.TransactionID)
			require.Equal(t, ledgerID.String(), request.ContextID)

			return &tracercontract.ReserveResult{
				ContractRevision: request.ContractRevision,
				TransactionID:    request.TransactionID,
				EvaluationID:     uuid.NewSHA1(request.TransactionID, []byte("cross-ledger")),
				Decision:         tracercontract.DecisionAllow,
				Controls:         tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated},
				ReservationIDs:   []uuid.UUID{},
				Reasons:          []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied},
			}, nil
		})
	}

	ctx, span, logger := anchorDeps()
	defer span.End()
	ctx = tmcore.ContextWithTenantID(ctx, "tenant-a")
	require.NoError(t, uc.reserveAtomicTransactionBatch(ctx, span, logger, run))

	for index := range run.items {
		attempt := run.items[index].tracerReservation.ContextAttempt
		require.NotNil(t, attempt)
		require.Equal(t, run.items[index].ledgerSettings.Tracer, attempt.Settings)
	}
}

func TestAtomicContextBatchRecoveredMemberCompletesThroughContextClient(t *testing.T) {
	for status, action := range map[string]string{constant.APPROVED: reservationActionConfirm, constant.CANCELED: reservationActionRelease} {
		t.Run(status, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockContextTracerReserver(ctrl)
			coordinator, err := NewContextTracerCoordinator(client, NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, time.Now)
			require.NoError(t, err)

			legacy := &stubReserver{}
			uc := &UseCase{ContextTracer: coordinator, TracerReserver: legacy}
			transactionID := uuid.New()

			if action == reservationActionConfirm {
				client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(completionResult(transactionID, "CONFIRMED"), nil)
			} else {
				client.EXPECT().ReleaseByTransaction(gomock.Any(), transactionID).Return(completionResult(transactionID, "RELEASED"), nil)
			}

			uc.reconcileAtomicTransactionBatchRecoveredMember(context.Background(), &transaction.Transaction{ID: transactionID.String(), AssetCode: "BRL"}, status)

			require.Empty(t, legacy.confirmedTxns, "a recovered context member must not settle through the legacy contract")
			require.Empty(t, legacy.releasedTxns)
		})
	}
}
