// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package events exposes the cross-shard event contracts published by the
// authorizer and consumed by downstream services (currently the transaction
// Redis cache invalidator). Keeping these contracts in a stand-alone package
// means producer (components/authorizer/...) and consumer
// (components/transaction/...) import the same struct + topic constant, so a
// contract drift is a compile error rather than a runtime parse failure.
package events

import "time"

// TopicCacheInvalidation is the Redpanda topic on which the authorizer
// publishes CacheInvalidationEvent payloads. Consumers subscribe here to
// purge stale balance cache entries after the 2PC recovery runner drives an
// incomplete commit to completion.
//
// The string MUST match the authorizer-side constant historically named
// crossShardCacheInvalidationTopic — it is aliased from that file now.
const TopicCacheInvalidation = "authorizer.cross-shard.cache-invalidation"

// Reason constants classify why a cache-invalidation event was emitted.
// They are stable identifiers carried on the event payload so consumers and
// operators can filter / alert without re-parsing upstream logs.
const (
	// ReasonRecoveryDriveToCompletion signals that the authorizer recovery
	// runner drove an incomplete cross-shard commit to COMPLETED and the
	// downstream Redis cache may hold pre-recovery balances.
	ReasonRecoveryDriveToCompletion = "recovery_drive_to_completion"
)

// CacheInvalidationTarget identifies a single (alias, balance_key) tuple whose
// cached Redis entry must be purged. The tuple is exactly what
// pkg/utils.BalanceInternalKey / BalanceShardKey hash into the cache key, so
// a consumer can reconstruct both the non-sharded and shard-aware key shapes
// from these two strings plus the event-level org / ledger fields.
//
// BalanceKey is the balance's Key (e.g. "default", "available"), NOT the
// #-joined composite used inside the cache value. The consumer is responsible
// for assembling "{alias}#{balance_key}" before passing to the key helper.
type CacheInvalidationTarget struct {
	Alias      string `json:"alias"`
	BalanceKey string `json:"balance_key"`
}

// CacheInvalidationEvent is the payload written to TopicCacheInvalidation.
//
// Fields:
//   - TransactionID: the 2PC transaction whose completion triggered the
//     invalidation. Used by consumers for correlation and dedup.
//   - OrganizationID / LedgerID: namespace that bounds the affected cache
//     keys. Exactly the (org, ledger) pair the authorizer committed against.
//   - Aliases: per-balance targets. Empty slice is a documented safety-net
//     fallback: when the producer cannot derive the affected aliases (rare,
//     recovery-only edge case) it still publishes an event so the consumer
//     can choose to fall back to a ledger-scoped cache scan. The primary
//     path ALWAYS populates Aliases.
//   - Reason: classification label (see Reason* constants).
//   - EmittedAt: producer wall-clock at publish time (UTC). Consumers MUST
//     treat it as advisory; broker append timestamp remains the authoritative
//     ordering marker.
type CacheInvalidationEvent struct {
	TransactionID  string                    `json:"transaction_id"`
	OrganizationID string                    `json:"organization_id"`
	LedgerID       string                    `json:"ledger_id"`
	Aliases        []CacheInvalidationTarget `json:"aliases"`
	Reason         string                    `json:"reason"`
	EmittedAt      time.Time                 `json:"emitted_at"`
}
