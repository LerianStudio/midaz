// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextAdmissionPersistsBeforeReserve(t *testing.T) {
	for _, scenario := range []string{"allow", "deny", "review", "off", "skip", "facts unavailable", "journal unknown", "response lost", "controls missing"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := NewMockTracerObligationStore(ctrl)
			client := NewMockContextTracerReserver(ctrl)
			loader := NewMockTracerFactsLoader(ctrl)
			evidence := NewMockTracerAccountingEvidence(ctrl)
			instant := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
			bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
			cfg := ContextTracerConfig{Facts: tracerreservation.Config{Bounds: bounds, MaxBodyBytes: 65536}, MaxReservations: 100}
			raw, err := os.ReadFile("../../../../../pkg/tracercontract/testdata/reserve_request.json")
			require.NoError(t, err)
			request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, cfg.Facts.MaxBodyBytes, bounds)
			require.NoError(t, err)
			key := tracerreservation.Key{OrganizationID: uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"), LedgerID: uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"), TransactionID: request.TransactionID}
			settings := mmodel.DefaultLedgerSettings().Tracer
			settings.Mode, settings.ValidationMode = "enforce", "rules-and-limits"
			if scenario == "off" {
				settings.Mode = "off"
			}
			require.NotEmpty(t, request.Context.Accounts)
			entries := []traceradapter.PreparedEntry{{AccountID: request.Context.Accounts[0].ID, Direction: tracercontract.Debit, Amount: decimal.RequireFromString(string(request.Amount)), AssetCode: request.Asset.Code}}
			input := ContextTracerInput{Key: key, ExecutionID: uuid.MustParse("5639dfb6-862e-4c2c-8a91-1f4f3ff54c9a"), Settings: settings, Timestamp: instant, Amount: decimal.RequireFromString(string(request.Amount)), AssetCode: request.Asset.Code, Entries: entries, HonoredSkip: scenario == "skip"}
			if scenario != "off" && scenario != "skip" {
				var factsErr, errorJournal, errorReserve error
				if scenario == "facts unavailable" {
					factsErr = errors.New("official facts unavailable")
				}
				if scenario == "journal unknown" {
					errorJournal = errors.New("intent commit unknown")
				}
				if scenario == "response lost" {
					errorReserve = errors.New("reserve response lost")
				}
				read := loader.EXPECT().EvaluationContext(gomock.Any(), key.OrganizationID, key.LedgerID, entries).Return(request.Context, factsErr)
				if factsErr == nil {
					frozen := false
					persist := store.EXPECT().Prepare(gomock.Any(), gomock.Any()).After(read).DoAndReturn(func(ctx context.Context, intent tracerreservation.Intent) (*tracerreservation.Record, error) {
						require.NoError(t, intent.Validate(ctx, cfg.Facts))
						require.Equal(t, input.ExecutionID, intent.ExecutionID)
						require.Equal(t, key, intent.Key)
						require.Equal(t, time.Second+250*time.Millisecond, intent.PrepareDeadline.Sub(intent.CreatedAt))
						frozen = true
						return &tracerreservation.Record{Intent: intent, State: tracerreservation.Prepared}, errorJournal
					})
					if errorJournal == nil {
						decision := tracercontract.DecisionAllow
						if scenario == "deny" {
							decision = tracercontract.DecisionDeny
						}
						if scenario == "review" {
							decision = tracercontract.DecisionReview
						}
						client.EXPECT().Reserve(gomock.Any(), gomock.Any()).After(persist).DoAndReturn(func(_ context.Context, sent tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
							require.True(t, frozen)
							require.Equal(t, key.LedgerID.String(), sent.ContextID)
							require.Equal(t, reservationRequestID(key.TransactionID), sent.RequestID)
							require.Equal(t, request.Asset, sent.Asset)
							require.Equal(t, request.Context, sent.Context)
							require.Equal(t, tracercontract.ValidationRulesAndLimits, sent.ValidationMode)
							result := &tracercontract.ReserveResult{ContractRevision: sent.ContractRevision, TransactionID: sent.TransactionID, EvaluationID: uuid.MustParse("487458bf-78f8-4f83-84f4-3eb35b66db83"), Decision: decision, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}
							if decision == tracercontract.DecisionDeny {
								result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonRuleDeny}
							}
							if decision == tracercontract.DecisionReview {
								result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonRuleReview}
							}
							if scenario == "controls missing" {
								result.Controls.Rules = tracercontract.RulesNotRequested
							}
							return result, errorReserve
						})
					}
				}
			}
			recovery, err := NewTracerRecoveryProcessor(store, client, evidence, TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", MaxBatch: 10, RetryInterval: time.Second, AttemptTimeout: time.Second}, func() time.Time { return instant })
			require.NoError(t, err)
			coordinator, err := NewContextTracerCoordinator(recovery, loader, cfg)
			require.NoError(t, err)
			attempt, err := coordinator.Admit(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), input)
			switch scenario {
			case "off", "skip":
				require.NoError(t, err)
				require.True(t, attempt.Skipped)
				require.False(t, attempt.IntentAttempted)
			case "facts unavailable":
				require.Error(t, err)
				require.False(t, attempt.IntentAttempted)
			case "journal unknown":
				require.Error(t, err)
				require.True(t, attempt.IntentAttempted)
				require.False(t, attempt.Frozen)
			case "response lost", "controls missing":
				require.Error(t, err)
				require.True(t, attempt.Frozen)
				require.Nil(t, attempt.Result)
			default:
				require.NoError(t, err)
				require.True(t, attempt.Frozen)
				require.NotNil(t, attempt.Result)
			}
		})
	}
}
