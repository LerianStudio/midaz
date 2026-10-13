// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

var reconcilerTestTime = time.Date(2043, time.June, 7, 8, 9, 10, 0, time.UTC)

// fakeReconcilerLease scripts leadership: a held lease reports fresh once per
// freshNext, and every Hold and Release is counted.
type fakeReconcilerLease struct {
	held      bool
	freshNext bool
	err       error
	holds     int
	releases  int
}

func (lease *fakeReconcilerLease) Hold(context.Context) (bool, bool, error) {
	lease.holds++

	if lease.err != nil || !lease.held {
		return false, false, lease.err
	}

	fresh := lease.freshNext
	lease.freshNext = false

	return true, fresh, nil
}

func (lease *fakeReconcilerLease) Release(context.Context) { lease.releases++ }

// fakeTenantPG resolves tenants to a placeholder handle, failing the listed ones,
// and records the resolution order.
type fakeTenantPG struct {
	failing  map[string]bool
	resolved []string
}

type fakeReconcilerDB struct{ dbresolver.DB }

func (resolver *fakeTenantPG) GetDB(_ context.Context, tenantID string) (dbresolver.DB, error) {
	resolver.resolved = append(resolver.resolved, tenantID)

	if resolver.failing[tenantID] {
		return nil, errors.New("tenant pool unavailable")
	}

	return fakeReconcilerDB{}, nil
}

func newReconcilerMetricsReader(t *testing.T) (*sdkmetric.ManualReader, *metrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(provider.Meter("balance-reconciler-test"), nil)
	require.NoError(t, err)

	return reader, factory
}

// reconcilerMetricPoints returns each recorded point of the named instrument,
// keyed by its attributes rendered as "key=value,..." in attribute order.
func reconcilerMetricPoints(t *testing.T, reader *sdkmetric.ManualReader, name string) map[string]int64 {
	t.Helper()

	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))

	points := map[string]int64{}

	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}

			switch data := metric.Data.(type) {
			case metricdata.Gauge[int64]:
				for _, point := range data.DataPoints {
					points[renderReconcilerAttributes(point.Attributes)] = point.Value
				}
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					points[renderReconcilerAttributes(point.Attributes)] = point.Value
				}
			default:
				t.Fatalf("%s: unexpected data type %T", name, metric.Data)
			}
		}
	}

	return points
}

func renderReconcilerAttributes(set attribute.Set) string {
	rendered := ""

	for _, kv := range set.ToSlice() {
		if rendered != "" {
			rendered += ","
		}

		rendered += string(kv.Key) + "=" + kv.Value.Emit()
	}

	return rendered
}

type reconcilerFixture struct {
	runner *BalanceReconcilerRunner
	repo   *balance.MockRepository
	lease  *fakeReconcilerLease
	clock  *steppingClock
	reader *sdkmetric.ManualReader
	logger *cleanupCapturingLogger
}

func newReconcilerFixture(t *testing.T, cfg BalanceReconcilerConfig) *reconcilerFixture {
	t.Helper()

	repo := balance.NewMockRepository(gomock.NewController(t))
	lease := &fakeReconcilerLease{held: true, freshNext: true}
	clock := &steppingClock{now: reconcilerTestTime}
	reader, factory := newReconcilerMetricsReader(t)
	logger := &cleanupCapturingLogger{Logger: libLog.NewNop()}

	runner := NewBalanceReconcilerRunner(logger, &query.UseCase{BalanceRepo: repo}, lease, cfg).
		WithClock(clock.Now).
		WithMetricsFactory(factory)

	// Drop the defaulting Warns of the zero-valued knobs: tests read pass logs only.
	logger.entries = nil

	return &reconcilerFixture{runner: runner, repo: repo, lease: lease, clock: clock, reader: reader, logger: logger}
}

// reconcilerPair builds a pair outside the grace window whose trail head matches
// the balance unless the caller changes it.
func reconcilerPair(id uuid.UUID, version int64) mmodel.BalanceTrailPair {
	stamp := reconcilerTestTime.Add(-24 * time.Hour)

	return mmodel.BalanceTrailPair{
		BalanceID:      id,
		OrganizationID: uuid.MustParse("00000000-0000-0000-0000-0000000000a1"),
		LedgerID:       uuid.MustParse("00000000-0000-0000-0000-0000000000b1"),
		AccountID:      uuid.MustParse("00000000-0000-0000-0000-0000000000c1"),
		Version:        version,
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		UpdatedAt:      stamp,
		Trail: &mmodel.BalanceTrailHead{
			Version:    version,
			Available:  decimal.NewFromInt(100),
			OnHold:     decimal.Zero,
			RecordedAt: stamp,
		},
	}
}

func balanceID(n byte) uuid.UUID {
	var id uuid.UUID
	id[15] = n

	return id
}

func TestBalanceReconcilerRunnerSkipsThePassWhenTheLockIsHeldElsewhere(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 10})
	fixture.lease.held = false

	// No repository expectation: any page read fails the test.
	fixture.runner.pass(t.Context())

	assert.Equal(t, 1, fixture.lease.holds)
	assert.Equal(t, 0, fixture.lease.releases)
	assert.Empty(t, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name))
	assert.Empty(t, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerPassFailures.Name))
}

func TestBalanceReconcilerRunnerCountsALeaseFailure(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 10})
	fixture.lease.err = errors.New("redis unavailable")

	// No repository expectation: any page read fails the test.
	fixture.runner.pass(t.Context())

	assert.Equal(t, map[string]int64{"reason=lock,tenant_id=": 1},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerPassFailures.Name))
	assert.Len(t, fixture.logger.at(libLog.LevelWarn), 1)
}

func TestBalanceReconcilerRunnerPublishesASingleTenantScanThatEndsInOnePage(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 10})

	ahead := reconcilerPair(balanceID(1), 4)
	ahead.Trail.Version = 3
	ok := reconcilerPair(balanceID(2), 2)

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 10).
		DoAndReturn(func(context.Context, uuid.UUID, int) ([]mmodel.BalanceTrailPair, uuid.UUID, error) {
			fixture.clock.advance(250 * time.Millisecond)
			return []mmodel.BalanceTrailPair{ahead, ok}, ok.BalanceID, nil
		})

	fixture.runner.pass(t.Context())

	assert.Equal(t, map[string]int64{
		"direction=balance_ahead,tenant_id=":  1,
		"direction=trail_ahead,tenant_id=":    0,
		"direction=value_mismatch,tenant_id=": 0,
	}, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name))
	assert.Equal(t, map[string]int64{"tenant_id=": 0},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDeferred.Name))
	assert.Equal(t, map[string]int64{"tenant_id=": reconcilerTestTime.Add(250 * time.Millisecond).Unix()},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerLastScanTimestamp.Name))
	assert.Equal(t, map[string]int64{"tenant_id=": 250},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerLastScanDuration.Name))
	assert.Empty(t, fixture.runner.scans, "a completed scan resets the cursor")
	assert.Equal(t, 0, fixture.lease.releases, "leadership is kept after a pass")
}

func TestBalanceReconcilerRunnerResumesFromTheCursorWhenTheBudgetEndsMidScan(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 2, PassBudgetMs: 1000})

	first, second, third := reconcilerPair(balanceID(1), 1), reconcilerPair(balanceID(2), 1), reconcilerPair(balanceID(3), 1)
	third.Trail.Version = 2

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 2).
		DoAndReturn(func(context.Context, uuid.UUID, int) ([]mmodel.BalanceTrailPair, uuid.UUID, error) {
			fixture.clock.advance(time.Second)
			return []mmodel.BalanceTrailPair{first, second}, second.BalanceID, nil
		})

	fixture.runner.pass(t.Context())

	assert.Empty(t, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name))
	require.Contains(t, fixture.runner.scans, "")
	assert.Equal(t, second.BalanceID, fixture.runner.scans[""].cursor)
	assert.Equal(t, reconcilerTestTime, fixture.runner.scans[""].startedAt)

	fixture.clock.advance(time.Minute)

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), second.BalanceID, 2).
		Return([]mmodel.BalanceTrailPair{third}, third.BalanceID, nil)

	fixture.runner.pass(t.Context())

	assert.Equal(t, map[string]int64{
		"direction=balance_ahead,tenant_id=":  0,
		"direction=trail_ahead,tenant_id=":    1,
		"direction=value_mismatch,tenant_id=": 0,
	}, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name))
	// Duration spans both passes, measured from the first page of the scan.
	assert.Equal(t, map[string]int64{"tenant_id=": (time.Second + time.Minute).Milliseconds()},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerLastScanDuration.Name))
	assert.Empty(t, fixture.runner.scans)
}

func TestBalanceReconcilerRunnerKeepsTheCursorWhenAPageFails(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 2})

	first, second := reconcilerPair(balanceID(1), 1), reconcilerPair(balanceID(2), 1)

	gomock.InOrder(
		fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 2).
			Return([]mmodel.BalanceTrailPair{first, second}, second.BalanceID, nil),
		fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), second.BalanceID, 2).
			Return(nil, uuid.Nil, errors.New("statement timeout")),
	)

	fixture.runner.pass(t.Context())

	assert.Equal(t, map[string]int64{"reason=query,tenant_id=": 1},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerPassFailures.Name))
	assert.Empty(t, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name))
	require.Contains(t, fixture.runner.scans, "")
	assert.Equal(t, second.BalanceID, fixture.runner.scans[""].cursor)
	assert.Equal(t, 0, fixture.lease.releases)
}

func TestBalanceReconcilerRunnerContinuesPastATenantWhosePoolFailsAndRotates(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 10})
	resolver := &fakeTenantPG{failing: map[string]bool{"tenant-a": true}}
	fixture.runner.WithTenants(staticTenants{"tenant-b", "tenant-a"}, resolver)

	pair := reconcilerPair(balanceID(1), 1)
	pair.Available = decimal.NewFromInt(7)

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 10).
		DoAndReturn(func(ctx context.Context, _ uuid.UUID, _ int) ([]mmodel.BalanceTrailPair, uuid.UUID, error) {
			assert.Equal(t, "tenant-b", tmcore.GetTenantIDContext(ctx))
			assert.NotNil(t, tmcore.GetPGContext(ctx, "transaction"))

			return []mmodel.BalanceTrailPair{pair}, pair.BalanceID, nil
		}).Times(2)

	fixture.runner.pass(t.Context())

	assert.Equal(t, []string{"tenant-a", "tenant-b"}, resolver.resolved)
	assert.Equal(t, map[string]int64{"reason=pg_resolve,tenant_id=tenant-a": 1},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerPassFailures.Name))
	assert.Equal(t, map[string]int64{
		"direction=balance_ahead,tenant_id=tenant-b":  0,
		"direction=trail_ahead,tenant_id=tenant-b":    0,
		"direction=value_mismatch,tenant_id=tenant-b": 1,
	}, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name))

	fixture.runner.pass(t.Context())

	assert.Equal(t, []string{"tenant-a", "tenant-b", "tenant-b", "tenant-a"}, resolver.resolved,
		"the next pass starts one tenant later")
}

func TestBalanceReconcilerRunnerRecoversAPanickingPage(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 10})

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 10).
		DoAndReturn(func(context.Context, uuid.UUID, int) ([]mmodel.BalanceTrailPair, uuid.UUID, error) {
			panic("driver crashed")
		})

	require.NotPanics(t, func() { fixture.runner.pass(t.Context()) })

	assert.Equal(t, map[string]int64{"reason=panic,tenant_id=": 1},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerPassFailures.Name))
	assert.Equal(t, 0, fixture.lease.releases, "a panicking pass keeps leadership")
	assert.Empty(t, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name))
}

func TestBalanceReconcilerRunnerCountsDeferredBalancesApartFromDivergences(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 10})

	// Diverging but updated inside the default 90 min grace window.
	recent := reconcilerPair(balanceID(1), 5)
	recent.Trail.Version = 4
	recent.UpdatedAt = reconcilerTestTime.Add(-time.Minute)

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 10).
		Return([]mmodel.BalanceTrailPair{recent}, recent.BalanceID, nil)

	fixture.runner.pass(t.Context())

	assert.Equal(t, map[string]int64{"tenant_id=": 1},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDeferred.Name))
	assert.Equal(t, map[string]int64{
		"direction=balance_ahead,tenant_id=":  0,
		"direction=trail_ahead,tenant_id=":    0,
		"direction=value_mismatch,tenant_id=": 0,
	}, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name))
}

func TestBalanceReconcilerRunnerClampsNonPositiveSettingsToDefaults(t *testing.T) {
	runner := NewBalanceReconcilerRunner(libLog.NewNop(), &query.UseCase{}, &fakeReconcilerLease{},
		BalanceReconcilerConfig{IntervalMs: -1, GraceMs: 0, PageSize: -5, PassBudgetMs: 0})

	assert.Equal(t, BalanceReconcilerConfig{
		IntervalMs:   defaultBalanceReconcilerIntervalMs,
		GraceMs:      defaultBalanceReconcilerGraceMs,
		PageSize:     defaultBalanceReconcilerPageSize,
		PassBudgetMs: defaultBalanceReconcilerPassBudgetMs,
	}, runner.cfg)
}

func TestBalanceReconcilerLauncherRegistersTheRunnerOnlyWhenPresent(t *testing.T) {
	t.Parallel()

	runner := NewBalanceReconcilerRunner(libLog.NewNop(), &query.UseCase{}, &fakeReconcilerLease{}, BalanceReconcilerConfig{})

	assert.Contains(t, launcherAppNames(&Service{BalanceReconciler: runner}), "Balance Reconciler")
	assert.NotContains(t, launcherAppNames(&Service{}), "Balance Reconciler")
}

func TestBalanceReconcilerLauncherInitHonorsTheEnabledFlag(t *testing.T) {
	t.Parallel()

	disabled := initBalanceReconcilerRunner(&Config{BalanceReconcilerEnabled: false}, libLog.NewNop(),
		&query.UseCase{}, nil, false, nil, nil, nil)
	assert.Nil(t, disabled)

	// Without a Redis client no lock manager can be built, so there is no runner.
	missingLock := initBalanceReconcilerRunner(&Config{BalanceReconcilerEnabled: true}, libLog.NewNop(),
		&query.UseCase{}, nil, false, nil, nil, nil)
	assert.Nil(t, missingLock)
}

func TestBalanceReconcilerRunnerFreshLeadershipResetsAHalfDoneScan(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 2, PassBudgetMs: 1000})

	first, second := reconcilerPair(balanceID(1), 1), reconcilerPair(balanceID(2), 1)

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 2).
		DoAndReturn(func(context.Context, uuid.UUID, int) ([]mmodel.BalanceTrailPair, uuid.UUID, error) {
			fixture.clock.advance(time.Second)
			return []mmodel.BalanceTrailPair{first, second}, second.BalanceID, nil
		})

	fixture.runner.pass(t.Context())

	require.Contains(t, fixture.runner.scans, "")
	require.Equal(t, second.BalanceID, fixture.runner.scans[""].cursor)

	// Leadership was lost and taken back: another pod may have scanned meanwhile.
	fixture.lease.freshNext = true
	fixture.clock.advance(time.Minute)

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 2).
		Return([]mmodel.BalanceTrailPair{first}, first.BalanceID, nil)

	fixture.runner.pass(t.Context())

	assert.Equal(t, map[string]int64{"tenant_id=": 0},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerLastScanDuration.Name),
		"the restarted scan is timed from its own first page")
	assert.Empty(t, fixture.runner.scans)
}

func TestBalanceReconcilerRunnerKeepsTheLeaseBetweenPassesAndReleasesItOnShutdown(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 10})

	ticks := make(chan time.Time, 2)
	ticks <- reconcilerTestTime
	ticks <- reconcilerTestTime

	stopped := 0
	fixture.runner.newTicker = func(time.Duration) (<-chan time.Time, func()) {
		return ticks, func() { stopped++ }
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	pages := 0
	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 10).
		DoAndReturn(func(context.Context, uuid.UUID, int) ([]mmodel.BalanceTrailPair, uuid.UUID, error) {
			pages++
			assert.Equal(t, 0, fixture.lease.releases, "no release between passes")

			if pages == 3 {
				cancel()
			}

			return nil, uuid.Nil, nil
		}).Times(3)

	fixture.runner.run(ctx)

	assert.Equal(t, 3, fixture.lease.holds)
	assert.Equal(t, 1, fixture.lease.releases, "leadership is released once, on shutdown")
	assert.Equal(t, 1, stopped)
}

func TestBalanceReconcilerRunnerCountsUnverifiablePairsApartFromDivergences(t *testing.T) {
	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: 10})

	legacy := reconcilerPair(balanceID(1), 7)
	legacy.Trail.Version = 0
	ahead := reconcilerPair(balanceID(2), 3)
	ahead.Trail.Version = 2

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, 10).
		Return([]mmodel.BalanceTrailPair{legacy, ahead}, ahead.BalanceID, nil)

	fixture.runner.pass(t.Context())

	assert.Equal(t, map[string]int64{"tenant_id=": 1},
		reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerUnverifiable.Name))
	assert.Equal(t, map[string]int64{
		"direction=balance_ahead,tenant_id=":  1,
		"direction=trail_ahead,tenant_id=":    0,
		"direction=value_mismatch,tenant_id=": 0,
	}, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name))

	warns := fixture.logger.at(libLog.LevelWarn)
	require.Len(t, warns, 1, "an unverifiable balance is not logged as a divergence")
	assert.Equal(t, ahead.BalanceID.String(), warns[0].fields["balance_id"])

	infos := fixture.logger.at(libLog.LevelInfo)
	require.NotEmpty(t, infos)
	assert.Equal(t, int64(1), infos[len(infos)-1].fields["unverifiable_count"])
}

func TestBalanceReconcilerRunnerCapsDivergenceWarnsPerScan(t *testing.T) {
	const divergent = balanceReconcilerMaxDivergenceWarns + 7

	fixture := newReconcilerFixture(t, BalanceReconcilerConfig{PageSize: divergent + 1})

	pairs := make([]mmodel.BalanceTrailPair, 0, divergent)
	for i := range divergent {
		var id uuid.UUID
		id[14], id[15] = byte(i>>8), byte(i)

		pair := reconcilerPair(id, 2)
		pair.Trail.Version = 1
		pairs = append(pairs, pair)
	}

	fixture.repo.EXPECT().ListBalanceTrailPairs(gomock.Any(), uuid.Nil, divergent+1).
		Return(pairs, pairs[len(pairs)-1].BalanceID, nil)

	fixture.runner.pass(t.Context())

	divergenceWarns, summaries := 0, []cleanupLogEntry{}

	for _, entry := range fixture.logger.at(libLog.LevelWarn) {
		switch entry.msg {
		case "Balance diverges from its operation trail":
			divergenceWarns++
		default:
			summaries = append(summaries, entry)
		}
	}

	assert.Equal(t, balanceReconcilerMaxDivergenceWarns, divergenceWarns)
	require.Len(t, summaries, 1, "one Warn reports the suppressed divergences")
	assert.Equal(t, 7, summaries[0].fields["suppressed"])
	assert.Equal(t, map[string]int64{
		"direction=balance_ahead,tenant_id=":  divergent,
		"direction=trail_ahead,tenant_id=":    0,
		"direction=value_mismatch,tenant_id=": 0,
	}, reconcilerMetricPoints(t, fixture.reader, utils.BalanceReconcilerDivergences.Name),
		"the gauge counts every divergence, logged or not")
}
