// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
)

// Default configuration values for the WAL reconciler.
const (
	defaultReconcilerInterval     = 10 * time.Second
	defaultReconcilerLookback     = 5 * time.Minute
	defaultReconcilerGrace        = 30 * time.Second
	defaultReconcilerCompletedTTL = 10 * time.Minute
	defaultSeedTimeout            = 5 * time.Second
)

// walReconciler periodically scans the WAL for cross-shard entries that have not
// been confirmed as completed in Redpanda. When it finds one, it re-publishes a
// PREPARED commit intent so the existing recovery runner can drive the transaction
// to completion. This closes the reliability gap when AsyncCommitIntent is enabled
// and the publish fails after local commit.
//
// SECURITY NOTE: The reconciler republishes commit intents derived from WAL
// entries, so WAL integrity is a transitive requirement for the 2PC protocol.
// walHMACKeys are passed to wal.Replay so tampered frames are rejected before
// any intent is constructed.
type walReconciler struct {
	service      *authorizerService
	observer     wal.Observer
	logger       libLog.Logger
	walPath      string
	walHMACKeys  [][]byte
	interval     time.Duration
	lookback     time.Duration
	grace        time.Duration
	completedMu  sync.RWMutex
	completedSet map[string]time.Time
	completedTTL time.Duration

	// Redpanda connection config, populated from Config at construction time.
	redpandaBrokers         []string
	redpandaTLSEnabled      bool
	redpandaTLSInsecureSkip bool
	redpandaTLSCAFile       string
	redpandaSASLEnabled     bool
	redpandaSASLMechanism   string
	redpandaSASLUsername    string
	redpandaSASLPassword    string
}

func newWALReconciler(cfg *Config, svc *authorizerService, logger libLog.Logger, observer wal.Observer) *walReconciler {
	interval := cfg.WALReconcilerInterval
	if interval <= 0 {
		interval = defaultReconcilerInterval
	}

	lookback := cfg.WALReconcilerLookback
	if lookback <= 0 {
		lookback = defaultReconcilerLookback
	}

	grace := cfg.WALReconcilerGrace
	if grace <= 0 {
		grace = defaultReconcilerGrace
	}

	completedTTL := cfg.WALReconcilerCompletedTTL
	if completedTTL <= 0 {
		completedTTL = defaultReconcilerCompletedTTL
	}

	walHMACKeys := [][]byte{cfg.WALHMACKey}
	if len(cfg.WALHMACKeyPrevious) > 0 {
		walHMACKeys = append(walHMACKeys, cfg.WALHMACKeyPrevious)
	}

	return &walReconciler{
		service:      svc,
		observer:     observer,
		logger:       logger,
		walPath:      cfg.WALPath,
		walHMACKeys:  walHMACKeys,
		interval:     interval,
		lookback:     lookback,
		grace:        grace,
		completedSet: make(map[string]time.Time),
		completedTTL: completedTTL,

		redpandaBrokers:         cfg.RedpandaBrokers,
		redpandaTLSEnabled:      cfg.RedpandaTLSEnabled,
		redpandaTLSInsecureSkip: cfg.RedpandaTLSInsecureSkip,
		redpandaTLSCAFile:       cfg.RedpandaTLSCAFile,
		redpandaSASLEnabled:     cfg.RedpandaSASLEnabled,
		redpandaSASLMechanism:   cfg.RedpandaSASLMechanism,
		redpandaSASLUsername:    cfg.RedpandaSASLUsername,
		redpandaSASLPassword:    cfg.RedpandaSASLPassword,
	}
}

// Run starts the reconciler loop. It seeds the completed set from Redpanda on
// startup, then runs reconcile on a timer until the context is cancelled.
func (r *walReconciler) Run(ctx context.Context) {
	if r == nil {
		return
	}

	r.logger.Infof(
		"Authorizer WAL reconciler started: interval=%v lookback=%v grace=%v completed_ttl=%v",
		r.interval, r.lookback, r.grace, r.completedTTL,
	)

	r.seedCompletedSet(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Infof("Authorizer WAL reconciler stopped")
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

// newSeedConsumer creates a Kafka consumer client configured for seeding the completed set.
// Returns nil and logs a warning if the client cannot be created.
func (r *walReconciler) newSeedConsumer() *kgo.Client {
	if len(r.redpandaBrokers) == 0 {
		r.logger.Warnf("Authorizer WAL reconciler seed: no brokers configured")
		return nil
	}

	securityOptions, err := brokersecurity.BuildFranzGoOptions(brokersecurity.Config{
		TLSEnabled:            r.redpandaTLSEnabled,
		TLSInsecureSkipVerify: r.redpandaTLSInsecureSkip,
		TLSCAFile:             r.redpandaTLSCAFile,
		SASLEnabled:           r.redpandaSASLEnabled,
		SASLMechanism:         r.redpandaSASLMechanism,
		SASLUsername:          r.redpandaSASLUsername,
		SASLPassword:          r.redpandaSASLPassword,
	})
	if err != nil {
		r.logger.Warnf("Authorizer WAL reconciler seed: failed to build security options: %v", err)
		return nil
	}

	options := make([]kgo.Opt, 0, 3+len(securityOptions)) //nolint:mnd // 3 base options + security options
	options = append(options,
		kgo.SeedBrokers(r.redpandaBrokers...),
		kgo.ConsumeTopics(crossShardCommitTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)

	options = append(options, securityOptions...)

	client, err := kgo.NewClient(options...)
	if err != nil {
		r.logger.Warnf("Authorizer WAL reconciler seed: failed to create consumer: %v", err)
		return nil
	}

	return client
}

// tryExtractCompletedTxID attempts to extract a completed transaction ID from
// a Kafka record. Returns the transaction ID if the record represents a
// completed/committed intent, or empty string otherwise.
func tryExtractCompletedTxID(record *kgo.Record) string {
	if record == nil || len(record.Value) == 0 {
		return ""
	}

	var intent commitIntent
	if err := json.Unmarshal(record.Value, &intent); err != nil {
		return ""
	}

	if intent.Status != commitIntentStatusCompleted && intent.Status != commitIntentStatusCommitted {
		return ""
	}

	return strings.TrimSpace(intent.TransactionID)
}

// seedCompletedSet does a one-shot read of the cross-shard commit topic to
// populate the completed set. This prevents re-publishing intents for transactions
// that have already been completed. If seeding fails, the reconciler continues
// with an empty set (safe, just means more redundant re-publishes).
func (r *walReconciler) seedCompletedSet(ctx context.Context) {
	if r == nil || r.service == nil || r.service.pub == nil {
		return
	}

	client := r.newSeedConsumer()
	if client == nil {
		return
	}

	defer client.Close()

	// Poll in a loop until the context deadline is reached or the partition is drained.
	// A single PollFetches may not return all available records, so we loop to ensure
	// the completed set is fully populated before the reconciler begins its first cycle.
	pollCtx, cancel := context.WithTimeout(ctx, defaultSeedTimeout)
	defer cancel()

	// Cap seed entries to prevent unbounded memory growth on large topics.
	const maxSeedEntries = 100_000

	seeded := 0
	now := time.Now()
	capped := false

	for {
		fetches := client.PollFetches(pollCtx)
		if fetches.IsClientClosed() || pollCtx.Err() != nil {
			break
		}

		recordCount := 0

		fetches.EachRecord(func(record *kgo.Record) {
			recordCount++

			if capped {
				return
			}

			if seeded >= maxSeedEntries {
				r.logger.Warnf(
					"Authorizer WAL reconciler seed: capped at %d entries; some completed transactions may be re-published",
					maxSeedEntries,
				)

				capped = true

				return
			}

			txID := tryExtractCompletedTxID(record)
			if txID == "" {
				return
			}

			r.completedMu.Lock()
			r.completedSet[txID] = now
			r.completedMu.Unlock()

			seeded++
		})

		if recordCount == 0 || capped {
			break // partition drained or cap reached
		}
	}

	r.logger.Infof("Authorizer WAL reconciler seeded completed set: %d transactions", seeded)
}

// reconcile scans the WAL for stale cross-shard entries and re-publishes commit
// intents for any that are not in the completed set.
func (r *walReconciler) reconcile(ctx context.Context) {
	if r == nil || r.service == nil {
		return
	}

	entries, err := wal.Replay(r.walPath, r.walHMACKeys, r.observer)
	if err != nil {
		r.logger.Warnf("Authorizer WAL reconciler replay failed: %v", err)
		return
	}

	now := time.Now()
	lookbackCutoff := now.Add(-r.lookback)
	graceCutoff := now.Add(-r.grace)
	republished := 0
	scanned := 0

	for i := range entries {
		if ctx.Err() != nil {
			return
		}

		entry := &entries[i]

		scanned++

		if !entry.CrossShard || len(entry.Participants) == 0 {
			continue
		}

		// Skip entries outside the [lookback, grace] window.
		if entry.CreatedAt.Before(lookbackCutoff) || entry.CreatedAt.After(graceCutoff) {
			continue
		}

		txID := strings.TrimSpace(entry.TransactionID)
		if txID == "" {
			continue
		}

		if r.isCompleted(txID) {
			continue
		}

		// Reconstruct a PREPARED commit intent from the WAL entry.
		intent := r.buildIntentFromWALEntry(entry)
		if intent == nil {
			continue
		}

		if err := r.service.publishCommitIntent(ctx, intent); err != nil {
			r.logger.Warnf(
				"Authorizer WAL reconciler publish failed: transaction_id=%s err=%v",
				txID, err,
			)

			continue
		}

		r.markCompleted(txID)

		republished++

		r.logger.Infof(
			"Authorizer WAL reconciler re-published commit intent: transaction_id=%s participants=%d",
			txID, len(intent.Participants),
		)
	}

	// Prune expired entries from the completed set.
	r.pruneCompletedSet(now)

	r.logger.Infof("Authorizer WAL reconciler cycle complete: scanned=%d republished=%d", scanned, republished)
}

// buildIntentFromWALEntry reconstructs a commitIntent from a WAL entry. The
// local participant is marked as Committed=true because the WAL entry's existence
// proves the local engine committed successfully.
func (r *walReconciler) buildIntentFromWALEntry(entry *wal.Entry) *commitIntent {
	if entry == nil || len(entry.Participants) == 0 {
		return nil
	}

	participants := make([]commitParticipant, 0, len(entry.Participants))

	for _, wp := range entry.Participants {
		p := commitParticipant{
			InstanceAddr: wp.InstanceAddr,
			PreparedTxID: wp.PreparedTxID,
			IsLocal:      wp.IsLocal,
			Committed:    wp.IsLocal, // Local is proven committed by WAL entry
		}

		participants = append(participants, p)
	}

	return &commitIntent{
		TransactionID:  entry.TransactionID,
		OrganizationID: entry.OrganizationID,
		LedgerID:       entry.LedgerID,
		Status:         commitIntentStatusPrepared,
		Participants:   participants,
		CreatedAt:      entry.CreatedAt,
	}
}

func (r *walReconciler) isCompleted(txID string) bool {
	r.completedMu.RLock()
	defer r.completedMu.RUnlock()

	_, exists := r.completedSet[txID]

	return exists
}

func (r *walReconciler) markCompleted(txID string) {
	r.completedMu.Lock()
	defer r.completedMu.Unlock()

	r.completedSet[txID] = time.Now()
}

func (r *walReconciler) pruneCompletedSet(now time.Time) {
	r.completedMu.Lock()
	defer r.completedMu.Unlock()

	for txID, addedAt := range r.completedSet {
		if now.Sub(addedAt) > r.completedTTL {
			delete(r.completedSet, txID)
		}
	}
}
