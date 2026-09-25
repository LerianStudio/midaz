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

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestCreateContextTracerFencesAccounting(t *testing.T) {
	for _, scenario := range []string{"allow", "pending", "review", "deny", "advisory review", "lost response open", "lost response closed", "unknown fence"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("AUDIT_LOG_ENABLED", "false")
			ctrl := gomock.NewController(t)
			store, client := NewMockTracerObligationStore(ctrl), NewMockContextTracerReserver(ctrl)
			loader, evidence := NewMockTracerFactsLoader(ctrl), NewMockTracerAccountingEvidence(ctrl)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			organizationID := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
			ledgerID := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
			now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
			settings := mmodel.DefaultLedgerSettings()
			settings.Tracer.Mode, settings.Tracer.ValidationMode, settings.Tracer.FailPosture = "enforce", "rules-and-limits", "closed"
			if scenario == "advisory review" {
				settings.Tracer.Mode = "advisory"
			}
			if scenario == "lost response open" {
				settings.Tracer.FailPosture = "open"
			}
			allowed := scenario == "allow" || scenario == "pending" || scenario == "advisory review" || scenario == "lost response open"
			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
			finished := make(chan struct{})
			if allowed {
				redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(func(context.Context, string, string, time.Duration) error { close(finished); return nil })
			} else {
				redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil)
			}
			bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
			cfg := ContextTracerConfig{Facts: tracerreservation.Config{Bounds: bounds, MaxBodyBytes: 65536}, MaxReservations: 100, AdmissionTimeout: 250 * time.Millisecond}
			recovery, err := NewTracerRecoveryProcessor(store, client, evidence, TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", MaxBatch: 10, RetryInterval: time.Second, AttemptTimeout: time.Second}, func() time.Time { return now })
			require.NoError(t, err)
			coordinator, err := NewContextTracerCoordinator(recovery, loader, cfg)
			require.NoError(t, err)
			loader.EXPECT().EvaluationContext(gomock.Any(), organizationID, ledgerID, gomock.Any()).DoAndReturn(func(_ context.Context, _, _ uuid.UUID, entries []traceradapter.PreparedEntry) (tracercontract.Context, error) {
				require.Len(t, entries, 2, "pending still includes its credit destination")
				asset := tracercontract.AssetRef{Namespace: "origin-a", ID: "asset-usd", Code: "USD"}
				facts := tracercontract.Context{Accounts: []tracercontract.Account{}, Entries: []tracercontract.Entry{}}
				for _, entry := range entries {
					blocked := false
					facts.Accounts = append(facts.Accounts, tracercontract.Account{ID: entry.AccountID, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: asset})
					facts.Entries = append(facts.Entries, tracercontract.Entry{AccountID: entry.AccountID, Direction: entry.Direction, Amount: tracercontract.Amount(entry.Amount.String()), Asset: asset})
				}
				return facts, nil
			})
			var frozen tracerreservation.Intent
			persist := store.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, intent tracerreservation.Intent) (*tracerreservation.Record, error) {
				frozen = intent
				return &tracerreservation.Record{Intent: intent, State: tracerreservation.Prepared}, nil
			})
			reserve := client.EXPECT().Reserve(gomock.Any(), gomock.Any()).After(persist).DoAndReturn(func(_ context.Context, request tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
				require.Equal(t, frozen.Key.TransactionID, request.TransactionID)
				require.Equal(t, scenario == "pending", *request.LongLived)
				if scenario == "lost response open" || scenario == "lost response closed" {
					return nil, errors.New("response lost")
				}
				decision, reason := tracercontract.DecisionAllow, tracercontract.ReasonLimitsSatisfied
				if scenario == "review" || scenario == "advisory review" {
					decision, reason = tracercontract.DecisionReview, tracercontract.ReasonRuleReview
				}
				if scenario == "deny" {
					decision, reason = tracercontract.DecisionDeny, tracercontract.ReasonRuleDeny
				}
				return &tracercontract.ReserveResult{ContractRevision: request.ContractRevision, TransactionID: request.TransactionID, EvaluationID: uuid.MustParse("487458bf-78f8-4f83-84f4-3eb35b66db83"), Decision: decision, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{reason}}, nil
			})
			fenced := false
			if allowed || scenario == "unknown fence" {
				store.EXPECT().BeginExecution(gomock.Any(), gomock.Any(), now).After(reserve).DoAndReturn(func(context.Context, tracerreservation.Key, time.Time) error {
					if scenario == "unknown fence" {
						return errors.New("fence commit unknown")
					}
					fenced = true
					return nil
				})
			}
			if scenario != "pending" && scenario != "unknown fence" {
				outcome := tracerreservation.Released
				if allowed {
					outcome = tracerreservation.Confirmed
				}
				store.EXPECT().SetOutcome(gomock.Any(), gomock.Any(), outcome, now).DoAndReturn(func(_ context.Context, key tracerreservation.Key, _ tracerreservation.State, _ time.Time) error {
					require.Equal(t, frozen.Key, key)
					return nil
				})
			}
			source := translationBalance(organizationID, ledgerID, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "@source", constant.DefaultBalanceKey)
			target := translationBalance(organizationID, ledgerID, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "@target", constant.DefaultBalanceKey)
			executor := &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}, before: func(EngineExecution) error { require.True(t, fenced); return nil }}
			status, expectedStatus := constant.CREATED, constant.APPROVED
			input := createEngineTransaction(now)
			if scenario == "pending" {
				status, expectedStatus, input.Pending = constant.PENDING, constant.PENDING, true
				input.TransactionDate = nil
			}
			reader, factory := newReaderFactory(t)
			uc := &UseCase{MetricsFactory: factory, TransactionRedisRepo: redisRepo, TransactionReader: &createEngineReader{settings: settings, balances: []*mmodel.Balance{source, target}}, Engine: executor, AppliedTransactionCompleter: &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: expectedStatus}}, ContextTracer: coordinator}
			_, _, err = uc.CreateTransactionV2(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), CreateTransactionV2Input{OrganizationID: organizationID, LedgerID: ledgerID, Transaction: input, TransactionStatus: status, IdempotencyTTL: time.Minute})
			expectedMetric := "allow"
			switch scenario {
			case "review", "advisory review":
				expectedMetric = "review"
			case "deny":
				expectedMetric = "deny"
			case "lost response open":
				expectedMetric = "fail_open"
			case "lost response closed":
				expectedMetric = "unavailable"
			}
			require.Equal(t, map[string]int64{"admission/" + expectedMetric: 1}, collectTracerCounters(t, reader))
			if allowed {
				require.NoError(t, err)
				require.Len(t, executor.requests, 1)
				select {
				case <-finished:
				case <-time.After(time.Second):
					t.Fatal("idempotency outcome missing")
				}
			} else {
				require.Error(t, err)
				require.Empty(t, executor.requests)
			}
		})
	}
}
