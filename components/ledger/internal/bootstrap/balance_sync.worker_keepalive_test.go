// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestBalanceSyncConfig_KeepaliveInterval documents how the keepalive interval is
// resolved: the knob is safe by default, so no value a deployment can supply stops
// the worker from starting or lets a scheduled key outlive a keepalive window.
func TestBalanceSyncConfig_KeepaliveInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured int
		want       time.Duration
	}{
		{name: "absent falls back to the default", configured: 0, want: 5 * time.Minute},
		{name: "unparseable env reads as zero and falls back", configured: 0, want: 5 * time.Minute},
		{name: "negative falls back to the default", configured: -1, want: 5 * time.Minute},
		{name: "configured value is honoured", configured: 60000, want: time.Minute},
		{name: "value above the ceiling is clamped", configured: 999999999, want: cachepolicy.BalanceTTL / 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worker := NewBalanceSyncWorker(newTestLogger(), &command.UseCase{}, BalanceSyncConfig{
				TTLKeepaliveIntervalMs: tt.configured,
			})

			assert.Equal(t, tt.want, worker.syncConfig.KeepaliveInterval())
		})
	}
}

// keepaliveProbe counts keepalive passes and reports each one on a channel so a test
// can wait for a pass instead of sleeping for one.
type keepaliveProbe struct {
	passes chan struct{}
}

func newKeepaliveProbe() *keepaliveProbe {
	return &keepaliveProbe{passes: make(chan struct{}, 16)}
}

func (p *keepaliveProbe) record() {
	select {
	case p.passes <- struct{}{}:
	default:
	}
}

// awaitPass waits for one keepalive pass, failing the test if none arrives.
func (p *keepaliveProbe) awaitPass(t *testing.T, msg string) {
	t.Helper()

	select {
	case <-p.passes:
	case <-time.After(5 * time.Second):
		require.FailNow(t, msg)
	}
}

// newKeepaliveWorker wires a worker whose only dependency is the mocked Redis repo,
// with the keepalive interval given in milliseconds.
func newKeepaliveWorker(t *testing.T, intervalMs int) (*BalanceSyncWorker, *redisTransaction.MockRedisRepository) {
	t.Helper()

	repo := redisTransaction.NewMockRedisRepository(gomock.NewController(t))

	worker := NewBalanceSyncWorker(
		newTestLogger(),
		&command.UseCase{TransactionRedisRepo: repo},
		BalanceSyncConfig{TTLKeepaliveIntervalMs: intervalMs},
	)

	return worker, repo
}

// TestBalanceSyncWorker_KeepaliveLoop covers the loop contract: a pass runs at start,
// the ticker keeps passing, a Redis failure is survived, and cancellation ends the
// goroutine.
func TestBalanceSyncWorker_KeepaliveLoop(t *testing.T) {
	t.Parallel()

	t.Run("runs a pass immediately and on every tick", func(t *testing.T) {
		t.Parallel()

		worker, repo := newKeepaliveWorker(t, 20)
		probe := newKeepaliveProbe()

		repo.EXPECT().
			RefreshBalanceSyncKeyTTLs(gomock.Any(), cachepolicy.BalanceTTL).
			DoAndReturn(func(context.Context, time.Duration) (int64, float64, error) {
				probe.record()

				return 1, 0, nil
			}).
			AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := worker.startTTLKeepalive(ctx)

		probe.awaitPass(t, "the start pass must run before the first tick")
		probe.awaitPass(t, "the ticker must keep refreshing scheduled keys")

		cancel()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			require.FailNow(t, "the keepalive goroutine must end with its context")
		}
	})

	t.Run("a failed pass never stops the loop", func(t *testing.T) {
		t.Parallel()

		worker, repo := newKeepaliveWorker(t, 20)
		probe := newKeepaliveProbe()

		repo.EXPECT().
			RefreshBalanceSyncKeyTTLs(gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, time.Duration) (int64, float64, error) {
				probe.record()

				return 0, 0, errors.New("redis: connection refused")
			}).
			AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := worker.startTTLKeepalive(ctx)

		probe.awaitPass(t, "the start pass must run")
		probe.awaitPass(t, "a pass that failed must not stop the next one")

		cancel()
		<-done
	})

	t.Run("a cancelled context runs no pass at all", func(t *testing.T) {
		t.Parallel()

		worker, repo := newKeepaliveWorker(t, 20)

		repo.EXPECT().RefreshBalanceSyncKeyTTLs(gomock.Any(), gomock.Any()).Times(0)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		done := worker.startTTLKeepalive(ctx)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			require.FailNow(t, "a keepalive started on a done context must return immediately")
		}
	})
}

// TestBalanceSyncWorker_OldestPendingAge covers the gauge fed by the keepalive pass.
func TestBalanceSyncWorker_OldestPendingAge(t *testing.T) {
	t.Parallel()

	t.Run("age is measured from the oldest score", func(t *testing.T) {
		t.Parallel()

		const pendingForSeconds = 120

		reader, factory := newBalanceSyncReaderFactory(t)
		worker, repo := newKeepaliveWorker(t, 20)
		worker.WithMetricsFactory(factory)

		oldestScore := float64(time.Now().Unix() - pendingForSeconds)

		repo.EXPECT().
			RefreshBalanceSyncKeyTTLs(gomock.Any(), gomock.Any()).
			Return(int64(1), oldestScore, nil).
			Times(1)

		worker.keepaliveOnce(context.Background())

		assert.InDelta(t, float64(pendingForSeconds), oldestPendingAge(t, reader), 5,
			"the gauge must report how long the oldest delta has been waiting")
	})

	t.Run("an empty schedule reports zero", func(t *testing.T) {
		t.Parallel()

		reader, factory := newBalanceSyncReaderFactory(t)
		worker, repo := newKeepaliveWorker(t, 20)
		worker.WithMetricsFactory(factory)

		repo.EXPECT().
			RefreshBalanceSyncKeyTTLs(gomock.Any(), gomock.Any()).
			Return(int64(0), float64(0), nil).
			Times(1)

		worker.keepaliveOnce(context.Background())

		assert.Zero(t, oldestPendingAge(t, reader), "nothing scheduled must clear the alert")
	})

	t.Run("nil factory is a no-op", func(t *testing.T) {
		t.Parallel()

		worker, repo := newKeepaliveWorker(t, 20)

		repo.EXPECT().
			RefreshBalanceSyncKeyTTLs(gomock.Any(), gomock.Any()).
			Return(int64(3), float64(1), nil).
			Times(1)

		require.NotPanics(t, func() {
			worker.keepaliveOnce(context.Background())
		})
	})
}

// oldestPendingAge returns the last value recorded on the pending-age gauge.
func oldestPendingAge(t *testing.T, reader *sdkmetric.ManualReader) float64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != utils.BalanceSyncOldestPendingAge.Name {
				continue
			}

			gauge, ok := m.Data.(metricdata.Gauge[int64])
			require.True(t, ok, "pending-age data type must be Gauge[int64], got %T", m.Data)
			require.Len(t, gauge.DataPoints, 1, "exactly one pending-age point must be recorded")

			return float64(gauge.DataPoints[0].Value)
		}
	}

	require.FailNow(t, "pending-age gauge was not recorded")

	return 0
}
