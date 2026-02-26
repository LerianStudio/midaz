// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/valkey"
)

// =============================================================================
// FUZZ TESTS — Redis Key Namespacing (T-001)
//
// These fuzz tests exercise tmvalkey.GetKeyFromContext with arbitrary key and
// tenantID inputs to verify:
//   1. No panic under any input (including Unicode, null bytes, very long strings,
//      colons, empty strings, and strings that already look like namespaced keys).
//   2. Namespacing invariants are preserved for all inputs.
//
// Run each function with:
//
//	go test -run=^$ -fuzz=FuzzKeyNamespacing_SimpleKey  -fuzztime=30s \
//	    ./components/transaction/internal/adapters/redis/
//
// =============================================================================

// FuzzKeyNamespacing_SimpleKey fuzzes the single-key namespacing path used by
// Set, SetNX, Get, Del, Incr, SetBytes, and GetBytes.
//
// Invariants verified for every (key, tenantID) pair:
//   - No panic
//   - Non-empty tenantID → result has prefix "tenant:{tenantID}:"
//   - Empty tenantID      → result is identical to the input key
//   - Determinism         → two calls with the same inputs return the same value
func FuzzKeyNamespacing_SimpleKey(f *testing.F) {
	// Seed corpus: 7 entries covering the required categories.
	// Each entry is (key string, tenantID string).

	// Empty key, no tenant
	f.Add("", "")
	// Simple key, non-empty tenant
	f.Add("my:key", "tenant-abc")
	// Colon-heavy key, non-empty tenant
	f.Add("org:ledger:account:balance", "tenant-xyz")
	// Unicode key, non-empty tenant
	f.Add("キー:テスト", "unicode-tenant")
	// Very long key (256 chars), no tenant
	f.Add(strings.Repeat("k", 256), "")
	// Key that already looks like a namespaced key (could confuse prefix logic)
	f.Add("tenant:fake:key", "real-tenant")
	// Null byte in key, non-empty tenant
	f.Add("key\x00value", "tenant-null")

	f.Fuzz(func(t *testing.T, key, tenantID string) {
		// Bound inputs to prevent resource exhaustion (OOM / extremely slow runs).
		if len(key) > 512 {
			key = key[:512]
		}

		if len(tenantID) > 256 {
			tenantID = tenantID[:256]
		}

		// Build context with or without a tenant ID.
		var ctx context.Context
		if tenantID != "" {
			ctx = tmcore.SetTenantIDInContext(context.Background(), tenantID)
		} else {
			ctx = context.Background()
		}

		// Call the function under test — must not panic.
		result := tmvalkey.GetKeyFromContext(ctx, key)

		// Invariant 1: non-empty tenantID → prefix is applied.
		if tenantID != "" {
			expectedPrefix := "tenant:" + tenantID + ":"
			if !strings.HasPrefix(result, expectedPrefix) {
				t.Errorf(
					"FuzzKeyNamespacing_SimpleKey: expected result to have prefix %q, got %q (key=%q, tenantID=%q)",
					expectedPrefix, result, key, tenantID,
				)
			}
		}

		// Invariant 2: empty tenantID → key is returned unchanged.
		if tenantID == "" && result != key {
			t.Errorf(
				"FuzzKeyNamespacing_SimpleKey: expected key unchanged for empty tenantID, got %q (key=%q)",
				result, key,
			)
		}

		// Invariant 3: determinism — calling again with the same context and key
		// must return the same value.
		result2 := tmvalkey.GetKeyFromContext(ctx, key)
		if result != result2 {
			t.Errorf(
				"FuzzKeyNamespacing_SimpleKey: non-deterministic result: first=%q second=%q (key=%q, tenantID=%q)",
				result, result2, key, tenantID,
			)
		}
	})
}

// FuzzKeyNamespacing_MGet fuzzes the multi-key namespacing path used by MGet.
//
// Because f.Add only accepts primitives, the slice of keys is derived from a
// keyCount (0–10) and a keyPrefix string. This produces predictable but varied
// slices while still exercising the boundary conditions.
//
// Invariants verified for every (keyPrefix, keyCount, tenantID) combination:
//   - No panic
//   - Result map is keyed by ORIGINAL (non-prefixed) keys
//   - No namespaced keys appear as result map keys
//   - len(result) <= keyCount
func FuzzKeyNamespacing_MGet(f *testing.F) {
	// Seed corpus: 5 entries — (keyPrefix, keyCount, tenantID).

	// Empty key, zero count, no tenant
	f.Add("", 0, "")
	// Simple key prefix, single key, with tenant
	f.Add("cache:key:", 1, "t1")
	// Colon-heavy key prefix, 3 keys, with tenant
	f.Add("org:ledger:account:", 3, "multi-tenant")
	// Unicode key prefix, 2 keys, with tenant
	f.Add("キー:", 2, "unicode-t")
	// Long prefix, max count, no tenant
	f.Add(strings.Repeat("x", 64)+":", 10, "")
	// Key that looks like a namespaced key, some count, non-empty tenant
	f.Add("tenant:existing:prefix:", 5, "new-tenant")
	// Null byte prefix, 1 key, with tenant
	f.Add("key\x00:", 1, "null-t")

	f.Fuzz(func(t *testing.T, keyPrefix string, keyCount int, tenantID string) {
		// Bound inputs.
		if len(keyPrefix) > 128 {
			keyPrefix = keyPrefix[:128]
		}

		if keyCount < 0 {
			keyCount = 0
		}

		if keyCount > 10 {
			keyCount = 10
		}

		if len(tenantID) > 256 {
			tenantID = tenantID[:256]
		}

		// Build the key slice.
		keys := make([]string, keyCount)
		for i := range keys {
			keys[i] = keyPrefix + strings.Repeat("k", i+1)
		}

		// Build context.
		var ctx context.Context
		if tenantID != "" {
			ctx = tmcore.SetTenantIDInContext(context.Background(), tenantID)
		} else {
			ctx = context.Background()
		}

		// Use a fresh recording client — no panic must occur.
		conn, recorder := newRecordingConnection(t)
		repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

		if keyCount == 0 {
			// Empty slice path — MGet returns early with an empty map.
			result, err := repo.MGet(ctx, keys)
			if err != nil {
				t.Errorf("FuzzKeyNamespacing_MGet: unexpected error for empty keys: %v", err)
			}

			if len(result) != 0 {
				t.Errorf("FuzzKeyNamespacing_MGet: expected empty result for 0 keys, got %d entries", len(result))
			}

			return
		}

		result, err := repo.MGet(ctx, keys)
		if err != nil {
			t.Errorf("FuzzKeyNamespacing_MGet: unexpected error: %v", err)
			return
		}

		// Invariant 1: result count never exceeds input key count.
		if len(result) > keyCount {
			t.Errorf(
				"FuzzKeyNamespacing_MGet: result has %d entries but only %d keys were requested",
				len(result), keyCount,
			)
		}

		// Invariant 2: result map keys are the ORIGINAL keys (not namespaced).
		for resultKey := range result {
			found := false

			for _, origKey := range keys {
				if resultKey == origKey {
					found = true
					break
				}
			}

			if !found {
				t.Errorf(
					"FuzzKeyNamespacing_MGet: result map contains unexpected key %q (not in original keys)",
					resultKey,
				)
			}
		}

		// Invariant 3: no namespaced key leaks into the result map when tenant is set.
		if tenantID != "" {
			prefix := "tenant:" + tenantID + ":"

			for resultKey := range result {
				if strings.HasPrefix(resultKey, prefix) {
					t.Errorf(
						"FuzzKeyNamespacing_MGet: result map contains namespaced key %q (tenantID=%q)",
						resultKey, tenantID,
					)
				}
			}
		}

		// Invariant 4: the keys that were actually sent to Redis (in the recorder)
		// must be namespaced when tenant is set, and unchanged when not.
		if len(recorder.mgetCalls) > 0 {
			sentKeys := recorder.mgetCalls[0]

			if tenantID != "" {
				expectedPrefix := "tenant:" + tenantID + ":"

				for i, sent := range sentKeys {
					if !strings.HasPrefix(sent, expectedPrefix) {
						t.Errorf(
							"FuzzKeyNamespacing_MGet: key sent to Redis at index %d (%q) missing prefix %q",
							i, sent, expectedPrefix,
						)
					}
				}
			} else {
				for i, sent := range sentKeys {
					if sent != keys[i] {
						t.Errorf(
							"FuzzKeyNamespacing_MGet: key sent to Redis at index %d (%q) != original key (%q) with no tenant",
							i, sent, keys[i],
						)
					}
				}
			}
		}
	})
}

// FuzzKeyNamespacing_QueueKey fuzzes the queue operation namespacing path used
// by AddMessageToQueue, ReadMessageFromQueue, and RemoveMessageFromQueue.
//
// Invariants verified for every (msgKey, tenantID) pair:
//   - No panic
//   - Non-empty tenantID → queue hash key starts with "tenant:{tenantID}:"
//   - Non-empty tenantID → message field key starts with "tenant:{tenantID}:"
//   - Empty tenantID      → queue hash key equals TransactionBackupQueue
//   - Empty tenantID      → message field key equals the input msgKey
func FuzzKeyNamespacing_QueueKey(f *testing.F) {
	// Seed corpus: 7 entries covering the required categories.
	// Each entry is (msgKey string, tenantID string).

	// Empty message key, no tenant
	f.Add("", "")
	// Simple message key, with tenant
	f.Add("tx:abc-123", "tenant-abc")
	// Colon-heavy key, with tenant
	f.Add("org:ledger:account:balance", "tenant-xyz")
	// Unicode key, with tenant
	f.Add("キー:テスト", "unicode-tenant")
	// Very long key (256 chars), no tenant
	f.Add(strings.Repeat("m", 256), "")
	// Key that already looks like a namespaced key
	f.Add("tenant:fake:key", "real-tenant")
	// Null byte in key, with tenant
	f.Add("msg\x00data", "tenant-null")

	f.Fuzz(func(t *testing.T, msgKey, tenantID string) {
		// Bound inputs.
		if len(msgKey) > 512 {
			msgKey = msgKey[:512]
		}

		if len(tenantID) > 256 {
			tenantID = tenantID[:256]
		}

		// Build context.
		var ctx context.Context
		if tenantID != "" {
			ctx = tmcore.SetTenantIDInContext(context.Background(), tenantID)
		} else {
			ctx = context.Background()
		}

		conn, recorder := newRecordingConnection(t)
		repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

		// Exercise AddMessageToQueue — must not panic.
		err := repo.AddMessageToQueue(ctx, msgKey, []byte("fuzz-payload"))
		if err != nil {
			t.Errorf("FuzzKeyNamespacing_QueueKey: AddMessageToQueue returned unexpected error: %v", err)
			return
		}

		if len(recorder.hsetCalls) == 0 {
			t.Error("FuzzKeyNamespacing_QueueKey: expected at least one HSet call")
			return
		}

		sentQueueKey := recorder.hsetCalls[0].Key

		var sentMsgKey string
		if len(recorder.hsetCalls[0].Values) >= 1 {
			if s, ok := recorder.hsetCalls[0].Values[0].(string); ok {
				sentMsgKey = s
			}
		}

		if tenantID != "" {
			expectedPrefix := "tenant:" + tenantID + ":"

			// Invariant 1: queue hash key is namespaced.
			if !strings.HasPrefix(sentQueueKey, expectedPrefix) {
				t.Errorf(
					"FuzzKeyNamespacing_QueueKey: queue key %q missing prefix %q (tenantID=%q)",
					sentQueueKey, expectedPrefix, tenantID,
				)
			}

			// Invariant 2: message field key is namespaced.
			if !strings.HasPrefix(sentMsgKey, expectedPrefix) {
				t.Errorf(
					"FuzzKeyNamespacing_QueueKey: msg field key %q missing prefix %q (tenantID=%q, msgKey=%q)",
					sentMsgKey, expectedPrefix, tenantID, msgKey,
				)
			}
		} else {
			// Invariant 3: without tenant, queue key equals the constant.
			if sentQueueKey != TransactionBackupQueue {
				t.Errorf(
					"FuzzKeyNamespacing_QueueKey: expected queue key %q, got %q (no tenant)",
					TransactionBackupQueue, sentQueueKey,
				)
			}

			// Invariant 4: without tenant, message field key is unchanged.
			if sentMsgKey != msgKey {
				t.Errorf(
					"FuzzKeyNamespacing_QueueKey: expected msg field key %q, got %q (no tenant)",
					msgKey, sentMsgKey,
				)
			}
		}
	})
}
