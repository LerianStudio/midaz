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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextTracerDispatchAndSettlement(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := NewMockTracerObligationStore(ctrl)
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	coordinator := &ContextTracerCoordinator{recovery: &TracerRecoveryProcessor{store: store, now: func() time.Time { return now }, config: TracerRecoveryConfig{AttemptTimeout: time.Second}}}
	key := tracerreservation.Key{OrganizationID: uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"), LedgerID: uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"), TransactionID: uuid.MustParse("5639dfb6-862e-4c2c-8a91-1f4f3ff54c9a")}
	attempt := ContextTracerAttempt{Key: key, IntentAttempted: true}
	require.ErrorIs(t, coordinator.BeginExecution(t.Context(), attempt), constant.ErrTracerContractUnavailable)
	attempt.Frozen = true
	unknown := errors.New("dispatch commit unknown")
	store.EXPECT().BeginExecution(gomock.Any(), key, now).Return(unknown).Times(1)
	require.ErrorIs(t, coordinator.BeginExecution(t.Context(), attempt), unknown)
	require.ErrorIs(t, coordinator.Conclude(t.Context(), attempt, tracerreservation.Executing), constant.ErrInvalidRequestBody)
	ctx, cancel := context.WithCancel(tmcore.ContextWithTenantID(t.Context(), "tenant-a"))
	cancel()
	store.EXPECT().SetOutcome(gomock.Any(), key, tracerreservation.Released, now).DoAndReturn(func(ctx context.Context, _ tracerreservation.Key, _ tracerreservation.State, _ time.Time) error {
		require.NoError(t, ctx.Err())
		require.Equal(t, "tenant-a", tmcore.GetTenantIDContext(ctx))
		_, bounded := ctx.Deadline()
		require.True(t, bounded)
		return nil
	})
	require.NoError(t, coordinator.Conclude(ctx, attempt, tracerreservation.Released))
}

func TestContextTracerPreparedGateNeedsNoDependencies(t *testing.T) {
	var coordinator *ContextTracerCoordinator
	for _, input := range []ContextTracerInput{
		{Settings: mmodel.TracerSettings{Mode: "off"}},
		{Settings: mmodel.TracerSettings{Mode: "enforce"}, HonoredSkip: true},
	} {
		attempt, err := coordinator.AdmitPrepared(t.Context(), input, mtransaction.Transaction{}, nil, nil)
		require.NoError(t, err)
		require.True(t, attempt.Skipped)
		require.NoError(t, coordinator.BeginExecution(t.Context(), attempt))
		require.NoError(t, coordinator.Conclude(t.Context(), attempt, tracerreservation.Confirmed))
	}
}

func TestContextTracerPendingCompletionSurvivesSettingsChanges(t *testing.T) {
	for _, scenario := range []string{"confirm", "release", "legacy", "lookup failed", "foreign producer"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := NewMockTracerObligationStore(ctrl)
			now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
			coordinator := &ContextTracerCoordinator{recovery: &TracerRecoveryProcessor{store: store, now: func() time.Time { return now }, config: TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", AttemptTimeout: time.Second}}}
			key := tracerreservation.Key{OrganizationID: uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"), LedgerID: uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"), TransactionID: uuid.MustParse("5639dfb6-862e-4c2c-8a91-1f4f3ff54c9a")}
			record := &tracerreservation.Pending{Key: key, ExecutionID: uuid.MustParse("88888888-8888-4888-8888-888888888888"), State: tracerreservation.Executing, ContractRevision: tracercontract.ReserveContractRevision, Scope: tracercontract.ReserveScope{TenantID: "tenant-a", IntegrationID: "producer", AssetNamespace: "origin-a"}}
			var lookupErr error
			if scenario == "legacy" {
				record = nil
			}
			if scenario == "lookup failed" {
				lookupErr = errors.New("primary unavailable")
			}
			if scenario == "foreign producer" {
				record.Scope.IntegrationID = "other-producer"
			}
			find := store.EXPECT().Find(gomock.Any(), key).Return(record, lookupErr)
			outcome := tracerreservation.Confirmed
			if scenario == "release" {
				outcome = tracerreservation.Released
			}
			if scenario == "confirm" || scenario == "release" {
				store.EXPECT().SetOutcome(gomock.Any(), key, outcome, now).After(find).Return(nil)
			}
			handled, err := coordinator.CompleteExisting(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), key, outcome)
			require.Equal(t, scenario != "legacy", handled, "only proven absence permits a legacy completion")
			if scenario == "lookup failed" || scenario == "foreign producer" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestContextTracerBatchDispatchRequiresEveryFence(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := NewMockTracerObligationStore(ctrl)
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	coordinator := &ContextTracerCoordinator{recovery: &TracerRecoveryProcessor{store: store, now: func() time.Time { return now }, config: TracerRecoveryConfig{MaxBatch: 2, AttemptTimeout: time.Second}}}
	key := tracerreservation.Key{OrganizationID: uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"), LedgerID: uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"), TransactionID: uuid.MustParse("5639dfb6-862e-4c2c-8a91-1f4f3ff54c9a")}
	second := key
	second.TransactionID = uuid.MustParse("88888888-8888-4888-8888-888888888888")
	attempts := []ContextTracerAttempt{{Key: key, IntentAttempted: true, Frozen: true}, {Key: second, IntentAttempted: true}}
	require.ErrorIs(t, coordinator.BeginBatchExecution(t.Context(), attempts), constant.ErrTracerContractUnavailable)
	attempts[1].Frozen = true
	attempts = append(attempts, ContextTracerAttempt{Skipped: true})
	failure := errors.New("batch fence commit unknown")
	store.EXPECT().BeginExecutions(gomock.Any(), []tracerreservation.Key{key, second}, now).Return(failure).Times(1)
	require.ErrorIs(t, coordinator.BeginBatchExecution(t.Context(), attempts), failure)
	require.NoError(t, coordinator.BeginBatchExecution(t.Context(), []ContextTracerAttempt{{Skipped: true}, {Skipped: true}, {Skipped: true}}))
}
