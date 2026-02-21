// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package shard provides account-based routing for Redis Cluster sharding.
//
// The core problem: Redis executes Lua scripts single-threaded. A single Redis
// instance caps at ~5K TPS for atomic balance operations. By distributing
// accounts across N shards (each a separate Redis Cluster hash slot group),
// we get N× throughput.
//
// Key design decisions:
//   - Shard by account alias, not by ledger (ledger cardinality is 1-3, useless)
//   - External accounts (@external/ASSET) are pre-split: each shard has its own
//     independent balance for the external account, so inflow/outflow transactions
//     (55-75% of traffic) stay same-shard
//   - FNV-1a hash for deterministic, well-distributed routing with zero I/O
//   - Hash tags ({shard_N}) force Redis Cluster key-to-slot mapping so all keys
//     for a shard land on the same node, enabling atomic Lua EVAL
package shard

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

const (
	// DefaultShardCount is the initial target for Redis Cluster shards.
	// 8 shards = 8× Lua throughput = ~160-240K Lua calls/sec.
	// Each shard owns 2048 of the 16384 Redis Cluster hash slots.
	DefaultShardCount = 8

	// ExternalPrefix is the alias prefix for external accounts.
	// External accounts are auto-created at asset creation time (create-asset.go:118-198)
	// and participate in ALL inflow/outflow transactions.
	ExternalPrefix = "@external/"

	// ExternalBalanceKeyPrefix identifies pre-split external balance keys.
	// Example: shard_3
	ExternalBalanceKeyPrefix = "shard_"
)

// Router maps account aliases to Redis Cluster shard IDs using deterministic
// FNV-1a hashing. It is stateless, goroutine-safe, and does zero I/O.
//
// The Router handles account aliases with deterministic hashing:
//   - Regular accounts: hash(alias) % shardCount
//   - External accounts: hash(alias) % shardCount
//
// NOTE:
// External accounts are currently resolved deterministically to avoid divergent
// shard assignment between read and write paths before full per-shard external
// persistence semantics are enforced end-to-end.
type Router struct {
	shardCount int
}

// NewRouter creates a Router with the given shard count.
// If shardCount <= 0, defaults to DefaultShardCount (8).
func NewRouter(shardCount int) *Router {
	if shardCount <= 0 {
		shardCount = DefaultShardCount
	}

	return &Router{shardCount: shardCount}
}

// ShardCount returns the number of shards this router distributes across.
func (r *Router) ShardCount() int {
	return r.shardCount
}

// Resolve returns the shard ID (0..shardCount-1) for a given account alias.
// Uses FNV-1a for fast, well-distributed hashing.
func (r *Router) Resolve(alias string) int {
	if r == nil || r.shardCount <= 0 {
		return 0
	}

	h := fnv.New32a()
	// hash.Hash.Write never returns an error for in-memory hashes
	_, _ = h.Write([]byte(alias))

	return int(h.Sum32()) % r.shardCount
}

// ResolveBalance resolves the target shard for a balance alias/key pair.
//
// For regular accounts, shard is derived from alias hash.
// For external accounts, a pre-split key (e.g., shard_3) takes precedence,
// forcing routing to the encoded shard.
func (r *Router) ResolveBalance(alias, balanceKey string) int {
	if r == nil || r.shardCount <= 0 {
		return 0
	}

	if IsExternal(alias) {
		if shardID, ok := ParseExternalBalanceShardID(balanceKey); ok {
			if shardID >= 0 && shardID < r.shardCount {
				return shardID
			}
		}
	}

	return r.Resolve(alias)
}

// ResolveExternalBalanceKey deterministically maps a counterpart alias to an
// external pre-split balance key (e.g., shard_3).
func (r *Router) ResolveExternalBalanceKey(counterpartyAlias string) string {
	return ExternalBalanceKey(r.Resolve(counterpartyAlias))
}

// ResolveTransaction assigns shard IDs to all accounts in a transaction.
// Returns a map of alias → shardID.
//
// Rules:
//   - Every account alias (including external aliases) is resolved by
//     deterministic hash(alias) % shardCount.
//
// The aliases parameter uses raw account aliases (e.g., "@user_123"),
// NOT the alias#key format (e.g., "@user_123#default").
func (r *Router) ResolveTransaction(aliases []string) map[string]int {
	result := make(map[string]int, len(aliases))

	for _, alias := range aliases {
		result[alias] = r.Resolve(alias)
	}

	return result
}

// IsExternal returns true if the alias represents an external account.
// External accounts have the format "@external/{ASSET}" (e.g., "@external/USD").
func IsExternal(alias string) bool {
	return strings.HasPrefix(alias, ExternalPrefix)
}

// ExternalBalanceKey formats an external pre-split balance key for a shard.
func ExternalBalanceKey(shardID int) string {
	return fmt.Sprintf("%s%d", ExternalBalanceKeyPrefix, shardID)
}

// IsExternalBalanceKey returns true when key matches "shard_<N>" format.
func IsExternalBalanceKey(key string) bool {
	_, ok := ParseExternalBalanceShardID(key)

	return ok
}

// ParseExternalBalanceShardID parses shard ID from "shard_<N>" keys.
func ParseExternalBalanceShardID(key string) (int, bool) {
	if !strings.HasPrefix(key, ExternalBalanceKeyPrefix) {
		return 0, false
	}

	rawShardID := strings.TrimPrefix(key, ExternalBalanceKeyPrefix)
	if rawShardID == "" {
		return 0, false
	}

	shardID, err := strconv.Atoi(rawShardID)
	if err != nil || shardID < 0 {
		return 0, false
	}

	return shardID, true
}

// HashTag returns the Redis Cluster hash tag for a given shard ID.
// All Redis keys for a shard MUST include this tag to ensure they
// map to the same hash slot, enabling atomic Lua EVAL across them.
//
// Example: HashTag(3) → "{shard_3}"
func HashTag(shardID int) string {
	return fmt.Sprintf("{shard_%d}", shardID)
}

// ExtractAccountAlias strips the balance key suffix from an alias#key string.
// Input:  "@user_123#default" → "@user_123"
// Input:  "@external/USD#default" → "@external/USD"
// Input:  "@user_123" → "@user_123" (no-op if no #)
func ExtractAccountAlias(aliasKey string) string {
	if idx := strings.IndexByte(aliasKey, '#'); idx >= 0 {
		return aliasKey[:idx]
	}

	return aliasKey
}

// SplitAliasAndBalanceKey splits an "alias#balanceKey" string into its two components.
// If the key part is absent or empty, constant.DefaultBalanceKey ("default") is returned
// as the balance key. This is the canonical shared helper used by any package that
// needs to parse the alias#key wire format.
//
// The expected input is a plain "alias#balanceKey" string WITHOUT a numeric index
// prefix. This is the format returned by transaction.SplitAliasWithKey after it
// strips the index prefix from the ConcatAlias format ("index#alias#balanceKey").
//
// Examples:
//
//	"@user_123#default"     → ("@user_123",     "default")
//	"@external/USD#shard_3" → ("@external/USD", "shard_3")
//	"@alice#"               → ("@alice",        "default")   // empty key → fallback
//	"@alice"                → ("@alice",        "default")   // no separator → fallback
//
// NOTE: This function differs from transaction.SplitAliasWithKey, which expects
// the "index#alias#balanceKey" format (with a numeric index prefix) and returns
// a single string "alias#balanceKey" rather than splitting into two values.
func SplitAliasAndBalanceKey(aliasWithKey string) (alias, balanceKey string) {
	parts := strings.SplitN(aliasWithKey, "#", 2)
	if len(parts) == 2 {
		key := parts[1]
		if key == "" {
			key = constant.DefaultBalanceKey
		}

		return parts[0], key
	}

	return aliasWithKey, constant.DefaultBalanceKey
}
