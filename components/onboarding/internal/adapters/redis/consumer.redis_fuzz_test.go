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
// FUZZ TESTS — Redis Key Namespacing (T-002)
//
// These fuzz tests exercise tmvalkey.GetKeyFromContext with arbitrary key and
// tenantID inputs to verify:
//   1. No panic under any input (including Unicode, null bytes, very long strings,
//      colons, empty strings, and strings that already look like namespaced keys).
//   2. Namespacing invariants are preserved for all inputs.
//
// Run with:
//
//	go test -run=^$ -fuzz=FuzzKeyNamespacing_SimpleKey -fuzztime=30s \
//	    ./components/onboarding/internal/adapters/redis/
//
// =============================================================================

// FuzzKeyNamespacing_SimpleKey fuzzes the single-key namespacing path used by
// Set, Get, and Del in the onboarding Redis consumer.
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
	// Simple key with tenant
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
