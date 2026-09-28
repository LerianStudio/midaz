// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// recordingExpirer records the operations a reaper asked it to expire, so a
// wiring test can tell the worker received this instance and not another.
type recordingExpirer struct {
	mu    sync.Mutex
	calls []model.ReserveOperationIdentity
}

func (e *recordingExpirer) Execute(_ context.Context, key model.ReserveOperationIdentity, _ time.Time) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.calls = append(e.calls, key)

	return 1, nil
}

// reaperWiringDeps returns the reaper's two collaborators. Neither is exercised
// during construction, so a repository over nil handles is enough to prove the
// boot path assembles the worker.
func reaperWiringDeps() (*limitServiceDeps, *command.RecordAuditEventCommand) {
	reservationRepo := postgres.NewUsageReservationRepositoryWithConnection(nil)
	deps := &limitServiceDeps{
		reservationRepo:  reservationRepo,
		reaperRepo:       postgres.NewReservationReaperRepository(nil, nil, reservationRepo),
		operationExpirer: &recordingExpirer{},
	}

	return deps, command.NewRecordAuditEventCommand(nil)
}

// TestLoadReservationReaperConfig covers the two knobs that had no reader at
// boot: the enable flag and the sweep cadence. An operator who sets either one
// must see it take effect, and a mistyped cadence must stop startup instead of
// passing silently.
func TestLoadReservationReaperConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		enabled          bool
		intervalSeconds  string
		batchSize        string
		expectNilConfig  bool
		expectedInterval time.Duration
		expectedBatch    int
		expectErrText    string
	}{
		{
			name:            "disabled returns nil config",
			enabled:         false,
			expectNilConfig: true,
		},
		{
			name:             "enabled with the default cadence and batch size",
			enabled:          true,
			expectedInterval: workers.DefaultReservationReaperInterval,
			expectedBatch:    workers.DefaultReservationReaperBatchSize,
		},
		{
			name:             "enabled with an operator cadence and batch size",
			enabled:          true,
			intervalSeconds:  "5",
			batchSize:        "50",
			expectedInterval: 5 * time.Second,
			expectedBatch:    50,
		},
		{
			name:          "a mistyped batch size is rejected",
			enabled:       true,
			batchSize:     "many",
			expectErrText: "RESERVATION_REAPER_BATCH_SIZE",
		},
		{
			name:          "a zero batch size is rejected",
			enabled:       true,
			batchSize:     "0",
			expectErrText: "RESERVATION_REAPER_BATCH_SIZE",
		},
		{
			name:          "an out-of-range batch size is rejected",
			enabled:       true,
			batchSize:     "10001",
			expectErrText: "RESERVATION_REAPER_BATCH_SIZE",
		},
		{
			name:            "a mistyped cadence is rejected",
			enabled:         true,
			intervalSeconds: "thirty",
			expectErrText:   "RESERVATION_REAPER_INTERVAL_SECONDS",
		},
		{
			name:            "an out-of-range cadence is rejected",
			enabled:         true,
			intervalSeconds: "7200",
			expectErrText:   "RESERVATION_REAPER_INTERVAL_SECONDS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				ReservationReaperEnabled:         tt.enabled,
				ReservationReaperIntervalSeconds: tt.intervalSeconds,
				ReservationReaperBatchSize:       tt.batchSize,
			}

			got, err := LoadReservationReaperConfig(t.Context(), cfg, testutil.NewMockLogger())

			if tt.expectErrText != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrText)

				return
			}

			require.NoError(t, err)

			if tt.expectNilConfig {
				assert.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.expectedInterval, got.ReapInterval)
			assert.Equal(t, tt.expectedBatch, got.BatchSize)
		})
	}
}

// TestApplyReservationReaperDefaults covers the posture of the sweep an operator
// never configured. Every reservation carries an expiry the API returns, and the
// sweep is the only path that returns capacity when that expiry passes, so it
// must run unless the operator explicitly turned it off.
func TestApplyReservationReaperDefaults(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		envSet      bool
		wantEnabled bool
	}{
		{name: "unset enables the sweep", wantEnabled: true},
		{name: "explicit false disables the sweep", envValue: "false", envSet: true, wantEnabled: false},
		{name: "explicit true keeps the sweep", envValue: "true", envSet: true, wantEnabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envSet {
				t.Setenv("RESERVATION_REAPER_ENABLED", tt.envValue)
			} else {
				t.Setenv("RESERVATION_REAPER_ENABLED", "")
				require.NoError(t, os.Unsetenv("RESERVATION_REAPER_ENABLED"))
			}

			// The bool mirrors what lib-commons parsed from the environment: it
			// cannot tell "unset" from "false", so both arrive here as false.
			cfg := &Config{ReservationReaperEnabled: tt.envValue == "true"}

			ApplyReservationReaperDefaults(cfg)

			assert.Equal(t, tt.wantEnabled, cfg.ReservationReaperEnabled)
		})
	}
}

// TestInitReaperWorker builds the reaper the way boot does. Nothing constructed
// this worker before, so the sweep the enable flag promised never ran and an
// invalid cadence never failed startup.
func TestInitReaperWorker(t *testing.T) {
	t.Parallel()

	limitDeps, auditor := reaperWiringDeps()

	t.Run("enabled builds the worker", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{ReservationReaperEnabled: true, ReservationReaperIntervalSeconds: "5"}

		worker, err := initReaperWorker(t.Context(), cfg, limitDeps.reaperRepo, auditor, limitDeps.operationExpirer,
			testutil.NewMockLogger(), clock.RealClock{})
		require.NoError(t, err)
		assert.NotNil(t, worker, "an operator who enables the reaper must get a sweep")
	})

	t.Run("disabled builds nothing", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{ReservationReaperEnabled: false}

		worker, err := initReaperWorker(t.Context(), cfg, limitDeps.reaperRepo, auditor, limitDeps.operationExpirer,
			testutil.NewMockLogger(), clock.RealClock{})
		require.NoError(t, err)
		assert.Nil(t, worker)
	})

	t.Run("a missing operation expirer stops startup", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{ReservationReaperEnabled: true, ReservationReaperIntervalSeconds: "5"}

		worker, err := initReaperWorker(t.Context(), cfg, limitDeps.reaperRepo, auditor, nil,
			testutil.NewMockLogger(), clock.RealClock{})
		require.ErrorIs(t, err, workers.ErrNilOperationExpirer)
		assert.Nil(t, worker)
	})

	t.Run("a mistyped cadence stops startup", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{ReservationReaperEnabled: true, ReservationReaperIntervalSeconds: "thirty"}

		worker, err := initReaperWorker(t.Context(), cfg, limitDeps.reaperRepo, auditor, limitDeps.operationExpirer,
			testutil.NewMockLogger(), clock.RealClock{})
		require.Error(t, err)
		assert.Nil(t, worker)
	})
}

// TestInitWorkers_SingleTenantRegistersReaper proves the composed single-tenant
// boot path ends with a reaper on the Service, which is what Run() registers
// with the launcher.
func TestInitWorkers_SingleTenantRegistersReaper(t *testing.T) {
	t.Parallel()

	limitDeps, auditor := reaperWiringDeps()

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{ReservationReaperEnabled: true, ReservationReaperIntervalSeconds: "5"}

		svc, err := initWorkers(t.Context(), cfg, limitDeps, auditor, nil, nil, nil, nil, nil,
			testutil.NewMockLogger(), clock.RealClock{}, nil, nil, nil)
		require.NoError(t, err)
		require.NotNil(t, svc)
		assert.NotNil(t, svc.reaperWorker, "single-tenant boot must carry the reservation reaper")
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{ReservationReaperEnabled: false}

		svc, err := initWorkers(t.Context(), cfg, limitDeps, auditor, nil, nil, nil, nil, nil,
			testutil.NewMockLogger(), clock.RealClock{}, nil, nil, nil)
		require.NoError(t, err)
		require.NotNil(t, svc)
		assert.Nil(t, svc.reaperWorker)
	})
}

// TestBuildSupervisorDeps_PreservesReaperWiring locks the mechanism the
// multi-tenant path depends on: the reaper fields are set on the caller's
// supervisor extras, and buildSupervisorDeps must carry them through rather than
// blanking them with the MT-sourced fields it does own.
func TestBuildSupervisorDeps_PreservesReaperWiring(t *testing.T) {
	t.Parallel()

	deps, extras := newWiringTestDeps(t)

	reservationRepo := postgres.NewUsageReservationRepositoryWithConnection(nil)
	reaperRepo := postgres.NewReservationReaperRepository(nil, nil, reservationRepo)
	auditor := command.NewRecordAuditEventCommand(nil)

	extras.ReaperRepo = reaperRepo
	extras.ReaperAuditor = auditor
	extras.ReaperConfig = workers.ReservationReaperWorkerConfig{ReapInterval: 5 * time.Second, BatchSize: workers.DefaultReservationReaperBatchSize}
	extras.ReaperWorkerEnabled = true

	got := buildSupervisorDeps(&Config{ApplicationName: "tracer"}, extras, deps, deps.Compiler, nil, nil)

	assert.True(t, got.ReaperWorkerEnabled, "per-tenant reapers must stay enabled through the deps build")
	assert.Equal(t, reaperRepo, got.ReaperRepo)
	assert.Equal(t, auditor, got.ReaperAuditor)
	assert.Equal(t, 5*time.Second, got.ReaperConfig.ReapInterval)
}

// TestInitWorkers_SingleTenantReaperExpiresThroughTheOperationExpirer drives
// one sweep of the reaper single-tenant boot composes over a decision-owned
// expired row, proving the worker received limitDeps.operationExpirer.
func TestInitWorkers_SingleTenantReaperExpiresThroughTheOperationExpirer(t *testing.T) {
	t.Parallel()

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	conn := dbmocks.NewMockConnection(gomock.NewController(t))
	conn.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

	reservationRepo := postgres.NewUsageReservationRepositoryWithConnection(nil)
	expirer := &recordingExpirer{}
	limitDeps := &limitServiceDeps{
		reservationRepo:  reservationRepo,
		reaperRepo:       postgres.NewReservationReaperRepository(conn, nil, reservationRepo),
		operationExpirer: expirer,
	}

	operation := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(96001)}
	sqlMock.ExpectQuery(`SELECT r.id, r.reservation_expires_at, r.decision_id, d.integration_id, d.transaction_id FROM usage_reservations AS r`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "reservation_expires_at", "decision_id", "integration_id", "transaction_id"}).
			AddRow(testutil.MustDeterministicUUID(96002), testutil.FixedTime(), testutil.MustDeterministicUUID(96003), operation.IntegrationID, operation.TransactionID))

	cfg := &Config{ReservationReaperEnabled: true, ReservationReaperIntervalSeconds: "5"}

	svc, err := initWorkers(t.Context(), cfg, limitDeps, command.NewRecordAuditEventCommand(nil), nil, nil, nil, nil, nil,
		testutil.NewMockLogger(), clock.NewFixedClock(testutil.FixedTime()), nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, svc.reaperWorker)

	released, err := svc.reaperWorker.RunOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, released)
	assert.Equal(t, []model.ReserveOperationIdentity{operation}, expirer.calls)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

// TestWithReservationReaper_CarriesTheOperationExpirer locks the multi-tenant
// path: the supervisor deps must carry the same expirer the single-tenant
// reaper uses, or every per-tenant reaper would refuse to start.
func TestWithReservationReaper_CarriesTheOperationExpirer(t *testing.T) {
	t.Parallel()

	limitDeps, auditor := reaperWiringDeps()
	reaperConfig := workers.ReservationReaperWorkerConfig{ReapInterval: 5 * time.Second, BatchSize: 25}

	got := withReservationReaper(workers.WorkerSupervisorDeps{Service: "tracer"}, limitDeps, auditor, reaperConfig, true)

	assert.Equal(t, "tracer", got.Service, "fields it does not own survive")
	assert.Same(t, limitDeps.operationExpirer, got.ReaperExpirer)
	assert.Equal(t, limitDeps.reaperRepo, got.ReaperRepo)
	assert.Equal(t, auditor, got.ReaperAuditor)
	assert.Equal(t, reaperConfig, got.ReaperConfig)
	assert.True(t, got.ReaperWorkerEnabled)
}
