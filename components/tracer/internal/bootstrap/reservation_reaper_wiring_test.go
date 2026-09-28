// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
)

// reaperWiringDeps returns the reaper's two collaborators. Neither is exercised
// during construction, so a repository over nil handles is enough to prove the
// boot path assembles the worker.
func reaperWiringDeps() (*limitServiceDeps, *command.RecordAuditEventCommand) {
	reservationRepo := postgres.NewUsageReservationRepositoryWithConnection(nil)
	deps := &limitServiceDeps{
		reservationRepo: reservationRepo,
		reaperRepo:      postgres.NewReservationReaperRepository(nil, nil, reservationRepo),
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
		expectNilConfig  bool
		expectedInterval time.Duration
		expectErrText    string
	}{
		{
			name:            "disabled returns nil config",
			enabled:         false,
			expectNilConfig: true,
		},
		{
			name:             "enabled with the default cadence",
			enabled:          true,
			expectedInterval: workers.DefaultReservationReaperInterval,
		},
		{
			name:             "enabled with an operator cadence",
			enabled:          true,
			intervalSeconds:  "5",
			expectedInterval: 5 * time.Second,
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

		worker, err := initReaperWorker(t.Context(), cfg, limitDeps.reaperRepo, auditor,
			testutil.NewMockLogger(), clock.RealClock{})
		require.NoError(t, err)
		assert.NotNil(t, worker, "an operator who enables the reaper must get a sweep")
	})

	t.Run("disabled builds nothing", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{ReservationReaperEnabled: false}

		worker, err := initReaperWorker(t.Context(), cfg, limitDeps.reaperRepo, auditor,
			testutil.NewMockLogger(), clock.RealClock{})
		require.NoError(t, err)
		assert.Nil(t, worker)
	})

	t.Run("a mistyped cadence stops startup", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{ReservationReaperEnabled: true, ReservationReaperIntervalSeconds: "thirty"}

		worker, err := initReaperWorker(t.Context(), cfg, limitDeps.reaperRepo, auditor,
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
	extras.ReaperConfig = workers.ReservationReaperWorkerConfig{ReapInterval: 5 * time.Second}
	extras.ReaperWorkerEnabled = true

	got := buildSupervisorDeps(&Config{ApplicationName: "tracer"}, extras, deps, deps.Compiler, nil, nil)

	assert.True(t, got.ReaperWorkerEnabled, "per-tenant reapers must stay enabled through the deps build")
	assert.Equal(t, reaperRepo, got.ReaperRepo)
	assert.Equal(t, auditor, got.ReaperAuditor)
	assert.Equal(t, 5*time.Second, got.ReaperConfig.ReapInterval)
}
