// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
)

// ShardRoutingSubscriber runs a goroutine that listens for shard-routing
// override updates on Redis PubSub and invalidates the per-pod route cache so
// stale entries are never used to dispatch balance operations to a source
// shard that a migration has already swept.
//
// Backoff on subscription failure is bounded by shardRoutingSubscriberMaxBackoff
// so a transient Redis outage self-heals once connectivity returns.
type ShardRoutingSubscriber struct {
	logger  libLog.Logger
	manager *internalsharding.Manager
	metrics *internalsharding.SubscriberMetrics

	// invalidations/decodeErrors mirror metrics.InvalidationsTotal/DecodeErrorsTotal
	// using atomics so bootstrap/telemetry can read them without racing with the
	// subscriber goroutine's updates (which themselves go through the manager's
	// handleRoutingUpdateMessage, under the cacheMu of the manager).
	// Today we only expose a pair of atomic counters; a future change can pipe
	// them into the telemetry.MetricsFactory via a registered observable gauge.
	invalidations atomic.Int64
	decodeErrors  atomic.Int64
}

const (
	shardRoutingSubscriberInitialBackoff = 250 * time.Millisecond
	shardRoutingSubscriberMaxBackoff     = 5 * time.Second
)

// NewShardRoutingSubscriber wires a subscriber to the given manager. Returns
// nil if the manager is nil or not enabled so callers can unconditionally
// append the result to a runnables slice with a simple nil check.
func NewShardRoutingSubscriber(logger libLog.Logger, manager *internalsharding.Manager) *ShardRoutingSubscriber {
	if manager == nil {
		return nil
	}

	return &ShardRoutingSubscriber{
		logger:  logger,
		manager: manager,
		metrics: &internalsharding.SubscriberMetrics{},
	}
}

// Run blocks until SIGINT/SIGTERM is received, supervising the underlying
// subscription with bounded exponential backoff on error.
func (s *ShardRoutingSubscriber) Run(_ *libCommons.Launcher) error {
	if s == nil || s.manager == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if s.logger != nil {
		s.logger.Info("ShardRoutingSubscriber started")
	}

	backoff := shardRoutingSubscriberInitialBackoff

	for {
		if ctx.Err() != nil {
			if s.logger != nil {
				s.logger.Info("ShardRoutingSubscriber: shutting down")
			}

			return nil
		}

		err := s.manager.SubscribeRoutingUpdates(ctx, uuid.Nil, uuid.Nil, s.metrics)
		s.snapshotMetrics()

		if err == nil {
			// Clean return (context cancelled or subscription channel closed).
			return nil
		}

		if s.logger != nil {
			s.logger.Warnf("ShardRoutingSubscriber: subscription returned err=%v; retrying in %s", err, backoff)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > shardRoutingSubscriberMaxBackoff {
			backoff = shardRoutingSubscriberMaxBackoff
		}
	}
}

// snapshotMetrics copies the subscriber metrics into the atomic counters so
// tests and a future telemetry exporter can read them without a data race.
func (s *ShardRoutingSubscriber) snapshotMetrics() {
	if s == nil || s.metrics == nil {
		return
	}

	s.invalidations.Store(s.metrics.InvalidationsTotal())
	s.decodeErrors.Store(s.metrics.DecodeErrorsTotal())
}

// Invalidations returns the total number of route-cache invalidations triggered
// by PubSub messages since the subscriber started. Exposed for tests and
// operational tooling (eg. /metrics, /debug endpoints).
func (s *ShardRoutingSubscriber) Invalidations() int64 {
	if s == nil {
		return 0
	}

	s.snapshotMetrics()

	return s.invalidations.Load()
}

// DecodeErrors returns the total number of malformed PubSub messages/channels
// observed since the subscriber started.
func (s *ShardRoutingSubscriber) DecodeErrors() int64 {
	if s == nil {
		return 0
	}

	s.snapshotMetrics()

	return s.decodeErrors.Load()
}
