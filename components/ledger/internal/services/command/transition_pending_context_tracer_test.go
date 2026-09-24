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
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestPendingContextCompletionIgnoresCurrentMode(t *testing.T) {
	for _, scenario := range []string{"off confirms", "off releases", "lookup failed", "legacy"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("AUDIT_LOG_ENABLED", "false")
			status, outcome := constant.APPROVED, tracerreservation.Confirmed
			if scenario == "off releases" {
				status, outcome = constant.CANCELED, tracerreservation.Released
			}
			uc, reader, engine, _, input := newTransitionEngineUseCase(t, status)
			reader.settings.Tracer.Mode = "off"
			if scenario == "lookup failed" || scenario == "legacy" {
				reader.settings.Tracer.Mode = "enforce"
			}
			legacy := &stubReserver{}
			uc.TracerReserver = legacy
			ctrl := gomock.NewController(t)
			store := NewMockTracerObligationStore(ctrl)
			now := fixedPendingCreatedAt.Add(time.Hour)
			uc.ContextTracer = &ContextTracerCoordinator{recovery: &TracerRecoveryProcessor{store: store, now: func() time.Time { return now }, config: TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", AttemptTimeout: time.Second}}}
			key := tracerreservation.Key{OrganizationID: input.OrganizationID, LedgerID: input.LedgerID, TransactionID: input.TransactionID}
			record := &tracerreservation.Pending{Key: key, ExecutionID: uuid.MustParse("88888888-8888-4888-8888-888888888888"), State: tracerreservation.Executing, ContractRevision: tracercontract.ReserveContractRevision, Scope: tracercontract.ReserveScope{TenantID: "tenant-a", IntegrationID: "producer", AssetNamespace: "origin-a"}}
			var lookupErr error
			if scenario == "legacy" {
				record = nil
			}
			if scenario == "lookup failed" {
				lookupErr = errors.New("primary unavailable")
			}
			store.EXPECT().Find(gomock.Any(), key).DoAndReturn(func(context.Context, tracerreservation.Key) (*tracerreservation.Pending, error) {
				require.Len(t, engine.requests, 1)
				return record, lookupErr
			})
			if scenario == "off confirms" || scenario == "off releases" {
				store.EXPECT().SetOutcome(gomock.Any(), key, outcome, now).Return(nil)
			}
			ctx := tmcore.ContextWithTenantID(t.Context(), "tenant-a")
			var err error
			if status == constant.CANCELED {
				_, err = uc.CancelTransactionV2(ctx, input)
			} else {
				_, err = uc.CommitTransactionV2(ctx, input)
			}
			require.NoError(t, err, "post-accounting reconciliation failures stay recoverable")
			require.Len(t, engine.requests, 1)
			if scenario == "legacy" {
				require.Equal(t, []uuid.UUID{input.TransactionID}, legacy.confirmedTxns)
			} else {
				require.Empty(t, legacy.confirmedTxns)
			}
			require.Empty(t, legacy.releasedTxns)
		})
	}
}
