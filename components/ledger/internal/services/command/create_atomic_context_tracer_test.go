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
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestCreateAtomicContextTracerFencesAllMembers(t *testing.T) {
	for _, scenario := range []string{"allow", "second denies", "fence unknown"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store, client := NewMockTracerObligationStore(ctrl), NewMockContextTracerReserver(ctrl)
			loader, evidence := NewMockTracerFactsLoader(ctrl), NewMockTracerAccountingEvidence(ctrl)
			engine := &applyingAtomicTransactionBatchEngine{t: t}
			uc, input, transactionIDs, executionID := atomicTransactionBatchExecutionFixture(t, &atomicTransactionBatchClaimRepositoryFake{}, engine, &atomicTransactionBatchTracerFake{})
			reader, ok := uc.TransactionReader.(*atomicTransactionBatchSettingsReader)
			require.True(t, ok)
			reader.settings.Tracer = mmodel.DefaultLedgerSettings().Tracer
			reader.settings.Tracer.Mode, reader.settings.Tracer.ValidationMode, reader.settings.Tracer.FailPosture = "enforce", "rules-and-limits", "closed"
			now := uc.Clock()
			bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
			cfg := ContextTracerConfig{Facts: tracerreservation.Config{Bounds: bounds, MaxBodyBytes: 65536}, MaxReservations: 100}
			recovery, err := NewTracerRecoveryProcessor(store, client, evidence, TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", MaxBatch: 10, RetryInterval: time.Second, AttemptTimeout: time.Second}, uc.Clock)
			require.NoError(t, err)
			uc.ContextTracer, err = NewContextTracerCoordinator(recovery, loader, cfg)
			require.NoError(t, err)
			loader.EXPECT().EvaluationContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _, _ uuid.UUID, entries []traceradapter.PreparedEntry) (tracercontract.Context, error) {
				facts := tracercontract.Context{Accounts: []tracercontract.Account{}, Entries: []tracercontract.Entry{}}
				for _, entry := range entries {
					blocked := false
					asset := tracercontract.AssetRef{Namespace: "origin-a", ID: "asset-brl", Code: entry.AssetCode}
					facts.Accounts = append(facts.Accounts, tracercontract.Account{ID: entry.AccountID, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: asset})
					facts.Entries = append(facts.Entries, tracercontract.Entry{AccountID: entry.AccountID, Direction: entry.Direction, Amount: tracercontract.Amount(entry.Amount.String()), Asset: asset})
				}
				return facts, nil
			}).Times(2)
			var persisted []uuid.UUID
			store.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, intent tracerreservation.Intent) (*tracerreservation.Record, error) {
				require.Equal(t, executionID, intent.ExecutionID)
				require.Equal(t, now.Add(1500*time.Millisecond), intent.PrepareDeadline)
				persisted = append(persisted, intent.Key.TransactionID)
				return &tracerreservation.Record{Intent: intent, State: tracerreservation.Prepared}, nil
			}).Times(2)
			client.EXPECT().Reserve(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
				require.Contains(t, persisted, request.TransactionID)
				decision, reason := tracercontract.DecisionAllow, tracercontract.ReasonLimitsSatisfied
				if scenario == "second denies" && request.TransactionID == transactionIDs[1] {
					decision, reason = tracercontract.DecisionDeny, tracercontract.ReasonRuleDeny
				}
				return &tracercontract.ReserveResult{ContractRevision: request.ContractRevision, TransactionID: request.TransactionID, EvaluationID: uuid.NewSHA1(request.TransactionID, []byte("evaluation")), Decision: decision, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{reason}}, nil
			}).Times(2)
			if scenario != "second denies" {
				store.EXPECT().BeginExecutions(gomock.Any(), gomock.Any(), now).DoAndReturn(func(_ context.Context, keys []tracerreservation.Key, _ time.Time) error {
					require.Len(t, keys, 2)
					require.Equal(t, transactionIDs, persisted)
					require.Empty(t, engine.executions)
					if scenario == "fence unknown" {
						return errors.New("batch fence commit unknown")
					}
					return nil
				})
			}
			if scenario != "fence unknown" {
				outcome := tracerreservation.Confirmed
				if scenario == "second denies" {
					outcome = tracerreservation.Released
				}
				store.EXPECT().SetOutcome(gomock.Any(), gomock.Any(), outcome, now).Return(nil).Times(2)
			}
			_, err = uc.CreateAtomicTransactionBatchV2(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), input)
			if scenario == "allow" {
				require.NoError(t, err)
				require.Len(t, engine.executions, 1)
			} else {
				require.Error(t, err)
				require.Empty(t, engine.executions)
			}
		})
	}
}
