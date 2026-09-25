// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package command

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tracerobligation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

const (
	processRecoveryPhaseEnv = "MIDAZ_TRACER_RECOVERY_PROCESS_PHASE"
	processRecoveryDSNEnv   = "MIDAZ_TRACER_RECOVERY_PROCESS_DSN"
	processCrashExitCode    = 86
)

func TestTracerRecoverySurvivesProcessCrashAfterAccounting(t *testing.T) {
	infra := pgtestutil.SetupMigratedContainer(t, "transaction")
	executable, err := os.Executable()
	require.NoError(t, err)

	crashed := exec.CommandContext(t.Context(), executable, "-test.run=^TestTracerRecoveryProcessHelper$")
	crashed.Env = append(os.Environ(), processRecoveryPhaseEnv+"=crash", processRecoveryDSNEnv+"="+infra.DSN)
	output, err := crashed.CombinedOutput()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, string(output))
	require.Equal(t, processCrashExitCode, exitErr.ExitCode(), string(output))

	key := processRecoveryKey()
	var state, transactionStatus string
	require.NoError(t, infra.DB.QueryRowContext(t.Context(), `SELECT state FROM tracer_reservation_obligation WHERE transaction_id=$1`, key.TransactionID).Scan(&state))
	require.Equal(t, string(tracerreservation.Executing), state)
	require.NoError(t, infra.DB.QueryRowContext(t.Context(), `SELECT status FROM transaction WHERE id=$1`, key.TransactionID).Scan(&transactionStatus))
	require.Equal(t, constant.APPROVED, transactionStatus)

	restarted := exec.CommandContext(t.Context(), executable, "-test.run=^TestTracerRecoveryProcessHelper$")
	restarted.Env = append(os.Environ(), processRecoveryPhaseEnv+"=recover", processRecoveryDSNEnv+"="+infra.DSN)
	output, err = restarted.CombinedOutput()
	require.NoError(t, err, string(output))

	var deliveredAt sql.NullTime
	require.NoError(t, infra.DB.QueryRowContext(t.Context(), `SELECT state,delivered_at FROM tracer_reservation_obligation WHERE transaction_id=$1`, key.TransactionID).Scan(&state, &deliveredAt))
	require.Equal(t, string(tracerreservation.Confirmed), state)
	require.True(t, deliveredAt.Valid)

	// A third process proves acknowledgement is durable and no completion is replayed.
	replayed := exec.CommandContext(t.Context(), executable, "-test.run=^TestTracerRecoveryProcessHelper$")
	replayed.Env = append(os.Environ(), processRecoveryPhaseEnv+"=empty", processRecoveryDSNEnv+"="+infra.DSN)
	output, err = replayed.CombinedOutput()
	require.NoError(t, err, string(output))
}

// TestTracerRecoveryProcessHelper runs only as a child of the process-level test.
// The crash phase exits without running test cleanup, emulating a process death
// after accounting is durable and before the journal outcome is recorded.
func TestTracerRecoveryProcessHelper(t *testing.T) {
	phase := os.Getenv(processRecoveryPhaseEnv)
	if phase == "" {
		t.Skip("process helper")
	}

	db, err := sql.Open("pgx", os.Getenv(processRecoveryDSNEnv))
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	require.NoError(t, db.PingContext(t.Context()))

	ctx := processRecoveryContext(t.Context(), db)
	store, _, intent := processRecoveryFixture(t, ctx)

	switch phase {
	case "crash":
		_, err = store.Prepare(ctx, intent)
		require.NoError(t, err)
		require.NoError(t, store.BeginExecution(ctx, intent.Key, intent.CreatedAt))
		_, err = db.ExecContext(ctx, `INSERT INTO transaction (id,organization_id,ledger_id,description,status,amount,asset_code,chart_of_accounts_group_name,body,created_at,updated_at) VALUES ($1,$2,$3,'process crash evidence','APPROVED',10.125,'BTC','','{}',$4,$4)`, intent.Key.TransactionID, intent.Key.OrganizationID, intent.Key.LedgerID, intent.CreatedAt)
		require.NoError(t, err)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		os.Exit(processCrashExitCode)
	case "recover", "empty":
		client := &processRecoveryClient{expectedCalls: 1}
		if phase == "empty" {
			client.expectedCalls = 0
		}
		processor, buildErr := NewTracerRecoveryProcessor(store, client, store, TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", SingleTenant: true, MaxBatch: 10, RetryInterval: time.Second, AttemptTimeout: time.Second}, func() time.Time { return intent.PrepareDeadline.Add(time.Second) })
		require.NoError(t, buildErr)
		summary, runErr := processor.RunOnce(ctx)
		require.NoError(t, runErr)
		require.Equal(t, client.expectedCalls, summary.Delivered)
		require.Equal(t, client.expectedCalls, client.calls)
	default:
		t.Fatalf("unknown process helper phase %q", phase)
	}
}

func processRecoveryContext(ctx context.Context, db *sql.DB) context.Context {
	ctx = tmcore.ContextWithTenantID(ctx, "tenant-a")
	return tmcore.ContextWithPG(ctx, dbresolver.New(dbresolver.WithPrimaryDBs(db)), constant.ModuleTransaction)
}

func processRecoveryFixture(t *testing.T, ctx context.Context) (*tracerobligation.Repository, tracerreservation.Config, tracerreservation.Intent) {
	t.Helper()

	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	cfg := tracerreservation.Config{Bounds: bounds, MaxBodyBytes: 65536}
	raw, err := os.ReadFile("../../../../../pkg/tracercontract/testdata/reserve_request.json")
	require.NoError(t, err)
	request, err := tracercontract.DecodeReserveJSON(ctx, raw, cfg.MaxBodyBytes, bounds)
	require.NoError(t, err)
	key := processRecoveryKey()
	request.TransactionID = key.TransactionID
	request.RequestID = uuid.MustParse("47d56dde-bdea-4e9f-841a-ec50fa7efbeb")
	request.ContextID = key.LedgerID.String()
	instant := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	intent, err := tracerreservation.NewIntent(ctx, key, uuid.MustParse("5639dfb6-862e-4c2c-8a91-1f4f3ff54c9a"), tracercontract.ReserveScope{TenantID: "tenant-a", IntegrationID: "producer", AssetNamespace: "origin-a"}, request, instant, instant.Add(time.Second), cfg)
	require.NoError(t, err)
	store, err := tracerobligation.NewRepository(nil, cfg, true, 10)
	require.NoError(t, err)

	return store, cfg, intent
}

func processRecoveryKey() tracerreservation.Key {
	return tracerreservation.Key{
		OrganizationID: uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"),
		LedgerID:       uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"),
		TransactionID:  uuid.MustParse("cc847720-dc8c-4504-918a-f283facacbaa"),
	}
}

type processRecoveryClient struct {
	expectedCalls int
	calls         int
}

func (c *processRecoveryClient) Reserve(context.Context, tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
	return nil, errors.New("recovery must not reserve or execute accounting")
}

func (c *processRecoveryClient) ConfirmByTransaction(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	c.calls++
	return &tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transactionID, Status: string(tracerreservation.Confirmed)}, nil
}

func (c *processRecoveryClient) ReleaseByTransaction(context.Context, uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	return nil, errors.New("approved accounting evidence must not release capacity")
}
