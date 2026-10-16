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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// finalizeCreateDeferredLogMessage is the warn finalizeCreateEngineResult logs
// when the projection after a confirmed engine execution fails.
const finalizeCreateDeferredLogMessage = "Applied transaction projection deferred to recovery"

// errFinalizeCreateClosedPool reproduces the error database/sql returns for a
// call on a closed *sql.DB, which is what the projection meets when the tenant
// pool was closed after the engine ran and re-resolution also failed.
var errFinalizeCreateClosedPool = errors.New("sql: database is closed")

// finalizeCreateEngineFixture builds the run and the engine outcome a /v1 create
// reaches finalizeCreateEngineResult with. The outcome comes from one execution
// of a fixture engine that is never handed to the use case under test, so the
// finalize step can only reuse the execution the outcome already carries.
func finalizeCreateEngineFixture(t *testing.T, tenantID string) (*createTransactionRun, EngineExecutionOutcome) {
	t.Helper()

	organizationID := uuid.MustParse("5a5a5a5a-5a5a-45a5-85a5-5a5a5a5a5a5a")
	ledgerID := uuid.MustParse("6b6b6b6b-6b6b-46b6-86b6-6b6b6b6b6b6b")
	transactionID := uuid.MustParse("7c7c7c7c-7c7c-47c7-87c7-7c7c7c7c7c7c")
	executionID := uuid.MustParse("8d8d8d8d-8d8d-48d8-88d8-8d8d8d8d8d8d")
	transactionDate := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)

	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)

	run := &createTransactionRun{
		organizationID:  organizationID,
		ledgerID:        ledgerID,
		transactionID:   transactionID,
		transactionDate: transactionDate,
		input:           createEngineTransaction(transactionDate),
		status:          constant.CREATED,
	}

	normalizeSendLegs(run)

	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, run.input, run.status)
	require.NoError(t, err)

	run.validate = validate
	run.fromTo = append(run.fromTo, run.input.Send.Source.From...)
	run.fromTo = append(run.fromTo, run.input.Send.Distribute.To...)
	run.action = mtransaction.StatusToAction(run.status)

	fixture := &UseCase{TransactionReader: &createEngineReader{balances: []*mmodel.Balance{
		translationBalance(organizationID, ledgerID, "9e9e9e9e-9e9e-49e9-89e9-9e9e9e9e9e9e", "@source", constant.DefaultBalanceKey),
		translationBalance(organizationID, ledgerID, "afafafaf-afaf-4afa-8afa-afafafafafaf", "@target", constant.DefaultBalanceKey),
	}}}

	state, err := fixture.prepareCreateEngineExecution(ctx, run, false)
	require.NoError(t, err)

	prepared, err := fixture.buildCreateEngineExecution(run, createBalanceExecutionContext{
		executionID: executionID, tenantID: tenantID, headerID: "request-finalize",
		enqueuedAt: transactionDate, transactionUpdated: transactionDate, operationUpdated: transactionDate,
		guard: ExecutionGuard{TransactionID: transactionID, NextToken: constant.APPROVED},
	}, state)
	require.NoError(t, err)

	executor := &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}}
	outcome, err := ExecutePreparedEngine(ctx, executor, prepared)
	require.NoError(t, err)
	require.True(t, outcome.Executed)
	require.Len(t, executor.requests, 1, "the fixture engine runs exactly once to produce the outcome")

	return run, outcome
}

// collectEngineWriteBehindProjections returns "path/outcome" -> count for the
// engine write-behind projection counter.
func collectEngineWriteBehindProjections(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()

	var data metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &data))

	totals := make(map[string]int64)

	for _, scope := range data.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != engineWriteBehindProjectionMetric {
				continue
			}

			counter, ok := item.Data.(metricdata.Sum[int64])
			require.True(t, ok, "data type must be Sum[int64], got %T", item.Data)

			for _, point := range counter.DataPoints {
				path, _ := point.Attributes.Value("path")
				outcome, _ := point.Attributes.Value("outcome")
				totals[path.AsString()+"/"+outcome.AsString()] += point.Value
			}
		}
	}

	return totals
}

func countLogLines(lines []capturedLogLine, level int, msg string) int {
	count := 0

	for _, line := range lines {
		if line.Level == level && line.Msg == msg {
			count++
		}
	}

	return count
}

// TestFinalizeCreateEngineResult_ProjectionAfterEngine covers the response and
// recovery half of "Projeção da transação após o engine usa o pool atual": once
// the engine has confirmed the movement, a projection on the current pool is
// acknowledged, and a projection that meets a closed pool is deferred to
// recovery while the create still answers with the confirmed transaction. In
// neither case does finalizing execute the engine again.
func TestFinalizeCreateEngineResult_ProjectionAfterEngine(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	const tenantID = "tenant-finalize"

	tests := []struct {
		name          string
		completionErr error
		wantAcks      int
		wantMetric    map[string]int64
		wantDeferLogs int
	}{
		{
			name:       "projection on the current pool completes and acknowledges recovery",
			wantAcks:   1,
			wantMetric: map[string]int64{"sync/completed": 1},
		},
		{
			name:          "projection on a closed pool is deferred to recovery without acknowledgment",
			completionErr: fmt.Errorf("begin completion transaction: %w", errFinalizeCreateClosedPool),
			wantMetric:    map[string]int64{"sync/deferred": 1},
			wantDeferLogs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, outcome := finalizeCreateEngineFixture(t, tenantID)

			reader, factory := newReaderFactory(t)
			engine := &scriptedEngine{}
			completer := &createAppliedTransactionCompleter{
				outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
				err:     tt.completionErr,
			}
			acknowledger := &recordingEngineRecoveryAcknowledger{}
			logger := &capturingLogger{}

			uc := &UseCase{
				Engine:                      engine,
				AppliedTransactionCompleter: completer,
				EngineRecoveryAcknowledger:  acknowledger,
				MetricsFactory:              factory,
			}

			ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)

			got, err := uc.finalizeCreateEngineResult(ctx, logger, run, outcome)

			require.NoError(t, err, "a confirmed engine execution answers success whatever happens to the projection")
			require.NotNil(t, got)
			assert.Equal(t, run.transactionID.String(), got.ID)
			assert.Equal(t, constant.CREATED, got.Status.Code, "the response is the confirmed transaction materialized from the engine evidence")
			assert.Len(t, got.Operations, 2)

			assert.Empty(t, engine.requests, "finalizing must never execute the engine again")

			require.Len(t, completer.envelopes, 1, "the projection is attempted exactly once")
			assert.Equal(t, outcome.Prepared.Execution.Execution.ExecutionID, completer.envelopes[0].ExecutionID,
				"the projection completes the execution the outcome already carries")
			assert.Equal(t, tenantID, completer.envelopes[0].TenantID)

			require.Len(t, acknowledger.records, tt.wantAcks)

			if tt.wantAcks > 0 {
				assert.Same(t, completer.envelopes[0], acknowledger.records[0])
				assert.Equal(t, constant.APPROVED, acknowledger.completions[0].Outcome.TransactionStatus)
			}

			assert.Equal(t, tt.wantMetric, collectEngineWriteBehindProjections(t, reader))

			lines := logger.snapshot()
			assert.Equal(t, tt.wantDeferLogs, countLogLines(lines, libLog.LevelWarn, finalizeCreateDeferredLogMessage), rendered(lines))

			if tt.completionErr != nil {
				deferred := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
				assert.Contains(t, deferred, errFinalizeCreateClosedPool.Error(), "the deferral reports the closed-pool cause")
				assert.Contains(t, deferred, run.transactionID.String())
			}
		})
	}
}
