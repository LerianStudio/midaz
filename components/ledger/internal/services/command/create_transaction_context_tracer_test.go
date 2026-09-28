// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func completionResult(transactionID uuid.UUID, status string) *tracercontract.TransactionCompletionResult {
	return &tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transactionID, Status: status}
}

func TestCreateContextTracerCompletesAfterAccounting(t *testing.T) {
	for _, scenario := range []string{"allow", "pending", "engine refused", "review", "deny", "advisory review", "lost response open", "lost response closed", "engine unknown", "facts unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("AUDIT_LOG_ENABLED", "false")
			withFastSharedRetrier(t)
			ctrl := gomock.NewController(t)
			client, loader := NewMockContextTracerClient(ctrl), NewMockTracerFactsLoader(ctrl)
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
			admitted := scenario == "allow" || scenario == "pending" || scenario == "engine refused" || scenario == "advisory review" || scenario == "lost response open"
			posted := admitted && scenario != "engine refused"
			unknown := scenario == "engine unknown"
			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
			finished := make(chan struct{})
			switch {
			case unknown:
			case posted:
				redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(func(context.Context, string, string, time.Duration) error { close(finished); return nil })
			default:
				redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil)
			}
			bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
			cfg := ContextTracerConfig{Bounds: bounds, MaxReservations: 100, AdmissionTimeout: 250 * time.Millisecond}
			coordinator, err := NewContextTracerCoordinator(client, loader, cfg, func() time.Time { return now })
			require.NoError(t, err)
			loader.EXPECT().EvaluationContext(gomock.Any(), organizationID, ledgerID, gomock.Any()).DoAndReturn(func(_ context.Context, _, _ uuid.UUID, entries []traceradapter.PreparedEntry) (tracercontract.Context, error) {
				require.Len(t, entries, 2, "pending still includes its credit destination")
				if scenario == "facts unavailable" {
					return tracercontract.Context{}, constant.ErrTracerFactsUnavailable
				}
				asset := "USD"
				facts := tracercontract.Context{Accounts: []tracercontract.Account{}, Entries: []tracercontract.Entry{}}
				for _, entry := range entries {
					blocked := false
					facts.Accounts = append(facts.Accounts, tracercontract.Account{ID: entry.AccountID, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: asset})
					facts.Entries = append(facts.Entries, tracercontract.Entry{AccountID: entry.AccountID, Direction: entry.Direction, Amount: tracercontract.Amount(entry.Amount.String()), Asset: asset})
				}
				return facts, nil
			})
			executor := &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}}
			if scenario == "engine refused" {
				executor.before = func(execution EngineExecution) error {
					return &accounting.Failure{Code: accounting.FailureInsufficientFunds, TransactionIndex: 0, PostingIndex: 0, BalanceRef: execution.Execution.Transactions[0].Postings[0].BalanceRef}
				}
			}
			if unknown {
				executor.before = func(EngineExecution) error { return errors.New("connection reset after dispatch") }
			}
			var reserved uuid.UUID
			reserve := client.EXPECT().Reserve(gomock.Any(), gomock.Any()).MaxTimes(1).DoAndReturn(func(_ context.Context, request tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
				require.Empty(t, executor.requests, "Reserve precedes accounting")
				reserved = request.TransactionID
				require.Equal(t, scenario == "pending", *request.LongLived)
				if scenario == "lost response open" || scenario == "lost response closed" {
					return nil, traceradapter.ErrTracerUnavailable
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
			switch {
			case unknown, scenario == "facts unavailable":
				// An unknown engine outcome and an admission never sent settle nothing.
			case posted && scenario != "pending":
				client.EXPECT().ConfirmByTransaction(gomock.Any(), gomock.Any()).After(reserve).DoAndReturn(func(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
					assert.Len(t, executor.requests, 1, "confirm follows accounting")
					assert.Equal(t, reserved, transactionID)
					return completionResult(transactionID, "CONFIRMED"), nil
				})
			case !posted:
				client.EXPECT().ReleaseByTransaction(gomock.Any(), gomock.Any()).After(reserve).DoAndReturn(func(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
					assert.Equal(t, reserved, transactionID)
					if scenario == "engine refused" {
						assert.Len(t, executor.requests, 1, "release follows the refused execution")
					} else {
						assert.Empty(t, executor.requests, "a rejected admission never reaches accounting")
					}
					return completionResult(transactionID, "RELEASED"), nil
				})
			}
			source := translationBalance(organizationID, ledgerID, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "@source", constant.DefaultBalanceKey)
			target := translationBalance(organizationID, ledgerID, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "@target", constant.DefaultBalanceKey)
			status, expectedStatus := constant.CREATED, constant.APPROVED
			input := createEngineTransaction(now)
			if scenario == "pending" {
				status, expectedStatus, input.Pending = constant.PENDING, constant.PENDING, true
				input.TransactionDate = nil
			}
			reader, factory := newReaderFactory(t)
			logger := &capturingLogger{}
			uc := &UseCase{MetricsFactory: factory, TransactionRedisRepo: redisRepo, TransactionReader: &createEngineReader{settings: settings, balances: []*mmodel.Balance{source, target}}, Engine: executor, AppliedTransactionCompleter: &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: expectedStatus}}, ContextTracer: coordinator}
			_, _, err = uc.CreateTransactionV2(libObservability.ContextWithLogger(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), logger), CreateTransactionV2Input{OrganizationID: organizationID, LedgerID: ledgerID, Transaction: input, TransactionStatus: status, IdempotencyTTL: time.Minute})
			expected := map[string]int64{"admission/allow": 1}
			switch scenario {
			case "allow":
				expected["confirm/delivered"] = 1
			case "engine refused":
				expected["release/delivered"] = 1
			case "review":
				expected = map[string]int64{"admission/review": 1, "release/delivered": 1}
			case "advisory review":
				expected = map[string]int64{"admission/review": 1, "confirm/delivered": 1}
			case "deny":
				expected = map[string]int64{"admission/deny": 1, "release/delivered": 1}
			case "lost response open":
				// An unanswered admission skips the inline completion and counts as
				// failed there; the retrier delivers it.
				expected = map[string]int64{"admission/fail_open": 1, "confirm/failed": 1}
			case "lost response closed":
				expected = map[string]int64{"admission/unavailable": 1, "release/failed": 1}
			case "facts unavailable":
				expected = map[string]int64{"admission/context_invalid": 1}
			}
			sharedReservationRetrier.wait()
			require.Equal(t, expected, collectTracerCounters(t, reader))
			if posted {
				require.NoError(t, err)
				require.Len(t, executor.requests, 1)
				select {
				case <-finished:
				case <-time.After(time.Second):
					t.Fatal("idempotency outcome missing")
				}
			} else {
				require.Error(t, err)
				if unknown {
					require.Len(t, executor.requests, 1)
					warned := false
					for _, line := range logger.atLevelOrMoreSevere(libLog.LevelWarn) {
						warned = warned || (line.Msg == "Transaction outcome unknown, reservation left to tracer TTL" && strings.Contains(line.Fields, reserved.String()))
					}
					require.True(t, warned, "an unknown outcome names the transaction left to the tracer TTL")
					return
				}
				if scenario == "engine refused" {
					require.Len(t, executor.requests, 1)
				} else {
					require.Empty(t, executor.requests)
				}
			}
		})
	}
}
