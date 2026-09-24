// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package command

import (
	"os"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tracerobligation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func TestTracerRecoveryRestartAfterAcknowledgementFailure(t *testing.T) {
	infra := pgtestutil.SetupMigratedContainer(t, "transaction")
	ctx := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(infra.DB)), constant.ModuleTransaction)
	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	cfg := tracerreservation.Config{Bounds: bounds, MaxBodyBytes: 65536}
	raw, err := os.ReadFile("../../../../../pkg/tracercontract/testdata/reserve_request.json")
	require.NoError(t, err)
	request, err := tracercontract.DecodeReserveJSON(ctx, raw, cfg.MaxBodyBytes, bounds)
	require.NoError(t, err)
	key := tracerreservation.Key{OrganizationID: uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"), LedgerID: uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"), TransactionID: request.TransactionID}
	request.ContextID = key.LedgerID.String()
	instant := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	intent, err := tracerreservation.NewIntent(ctx, key, uuid.MustParse("5639dfb6-862e-4c2c-8a91-1f4f3ff54c9a"), tracercontract.ReserveScope{TenantID: "tenant-a", IntegrationID: "producer", AssetNamespace: "origin-a"}, request, instant, instant.Add(time.Second), cfg)
	require.NoError(t, err)
	store, err := tracerobligation.NewRepository(nil, cfg, true, 10)
	require.NoError(t, err)
	_, err = store.Prepare(ctx, intent)
	require.NoError(t, err)
	require.NoError(t, store.BeginExecution(ctx, key, instant))
	require.NoError(t, store.SetOutcome(ctx, key, tracerreservation.Confirmed, instant))
	_, err = infra.DB.ExecContext(ctx, `CREATE FUNCTION suppress_tracer_delivery() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN IF NEW.delivered_at IS NOT NULL THEN RETURN NULL; END IF; RETURN NEW; END; $$; CREATE TRIGGER suppress_tracer_delivery BEFORE UPDATE ON tracer_reservation_obligation FOR EACH ROW EXECUTE FUNCTION suppress_tracer_delivery()`)
	require.NoError(t, err)
	client := NewMockContextTracerReserver(gomock.NewController(t))
	response := &tracercontract.TransactionCompletionResult{ContractRevision: request.ContractRevision, TransactionID: key.TransactionID, Status: "CONFIRMED"}
	client.EXPECT().ConfirmByTransaction(gomock.Any(), key.TransactionID).Return(response, nil).Times(2)
	recoveryCfg := TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", MaxBatch: 10, RetryInterval: time.Second, AttemptTimeout: time.Second}
	processor, err := NewTracerRecoveryProcessor(store, client, store, recoveryCfg, func() time.Time { return instant })
	require.NoError(t, err)
	summary, err := processor.RunOnce(ctx)
	require.Error(t, err)
	require.Equal(t, 1, summary.Failed)
	var undelivered int
	require.NoError(t, infra.DB.QueryRowContext(ctx, `SELECT count(*) FROM tracer_reservation_obligation WHERE delivered_at IS NULL AND state='CONFIRMED'`).Scan(&undelivered))
	require.Equal(t, 1, undelivered)
	_, err = infra.DB.ExecContext(ctx, `DROP TRIGGER suppress_tracer_delivery ON tracer_reservation_obligation`)
	require.NoError(t, err)
	instant = instant.Add(2 * time.Second)
	// Reconstruct both collaborators: no in-memory retry queue or reserve handle
	// survives this restart, and the original fact body is never reloaded.
	restarted, err := tracerobligation.NewRepository(nil, cfg, true, 10)
	require.NoError(t, err)
	processor, err = NewTracerRecoveryProcessor(restarted, client, restarted, recoveryCfg, func() time.Time { return instant })
	require.NoError(t, err)
	summary, err = processor.RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Delivered)
	instant = instant.Add(time.Minute)
	summary, err = processor.RunOnce(ctx)
	require.NoError(t, err)
	require.Zero(t, summary.Claimed)
}
