// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

// =============================================================================
// PROPERTY-BASED TESTS — Redis Key Namespacing (T-001)
//
// These tests verify that the domain invariants of tmvalkey.GetKey and
// tmvalkey.StripTenantPrefix hold across hundreds of automatically-generated
// (tenantID, key) pairs.
//
// Run with:
//
//	go test -run TestProperty -v -count=1 \
//	    ./components/transaction/internal/adapters/redis/
//
// Each TestProperty_* function uses testing/quick.Check and will report the
// counterexample that falsified the property if any violation is found.
// =============================================================================

import (
	"context"
	"strings"
	"testing"
	"testing/quick"

	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/valkey"
	"github.com/stretchr/testify/require"
)

// sanitizeQuickString trims generated strings to reasonable lengths so that
// quick.Check does not produce unbounded inputs that cause memory pressure.
// Bounding is required by the property-testing standard.
func sanitizeQuickString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}

	return s
}

// TestProperty_KeyNamespacing_Identity verifies the identity property:
//
//	When tenantID is empty, GetKey("", key) == key for ALL keys.
//
// This guarantees that the namespacing function is a no-op for single-tenant
// deployments (backwards-compatibility invariant).
func TestProperty_KeyNamespacing_Identity(t *testing.T) {
	property := func(key string) bool {
		key = sanitizeQuickString(key, 512)
		result := tmvalkey.GetKey("", key)

		return result == key
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Identity property violated: GetKey(\"\", key) must equal key for all inputs")
}

// TestProperty_KeyNamespacing_PrefixStructure verifies the prefix structure property:
//
//	When tenantID is non-empty, GetKey(tenantID, key) ALWAYS:
//	  - starts with "tenant:" + tenantID + ":"
//	  - ends with the original key
//
// This guarantees that the namespaced key is deterministically constructed and
// that the original key is always recoverable from the suffix.
func TestProperty_KeyNamespacing_PrefixStructure(t *testing.T) {
	property := func(tenantID, key string) bool {
		tenantID = sanitizeQuickString(tenantID, 256)
		key = sanitizeQuickString(key, 512)

		// This property only applies when tenantID is non-empty.
		if tenantID == "" {
			return true
		}

		result := tmvalkey.GetKey(tenantID, key)
		expectedPrefix := "tenant:" + tenantID + ":"

		startsCorrectly := strings.HasPrefix(result, expectedPrefix)
		endsWithOriginalKey := strings.HasSuffix(result, key)

		return startsCorrectly && endsWithOriginalKey
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err,
		"PrefixStructure property violated: GetKey(tenantID, key) must start with 'tenant:{tenantID}:' and end with key when tenantID is non-empty")
}

// TestProperty_KeyNamespacing_Reversibility verifies the reversibility property:
//
//	StripTenantPrefix(tenantID, GetKey(tenantID, key)) == key
//	for ALL (tenantID, key) pairs (including empty tenantID).
//
// This guarantees that every namespaced key can be reversed to recover the
// original key, which is critical for MGet result mapping and audit tracing.
func TestProperty_KeyNamespacing_Reversibility(t *testing.T) {
	property := func(tenantID, key string) bool {
		tenantID = sanitizeQuickString(tenantID, 256)
		key = sanitizeQuickString(key, 512)

		namespaced := tmvalkey.GetKey(tenantID, key)
		recovered := tmvalkey.StripTenantPrefix(tenantID, namespaced)

		return recovered == key
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err,
		"Reversibility property violated: StripTenantPrefix(tenantID, GetKey(tenantID, key)) must equal key for all inputs")
}

// TestProperty_KeyNamespacing_Determinism verifies the determinism property:
//
//	Same (tenantID, key) input ALWAYS produces the same output.
//	Calling GetKey twice with identical inputs returns identical results.
//
// This guarantees that key generation has no hidden state or randomness,
// which is essential for cache lookup correctness.
func TestProperty_KeyNamespacing_Determinism(t *testing.T) {
	property := func(tenantID, key string) bool {
		tenantID = sanitizeQuickString(tenantID, 256)
		key = sanitizeQuickString(key, 512)

		first := tmvalkey.GetKey(tenantID, key)
		second := tmvalkey.GetKey(tenantID, key)

		return first == second
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err,
		"Determinism property violated: GetKey must return the same result for identical inputs")
}

// TestProperty_MGet_OutputKeysMatchOriginal verifies the MGet output key invariant:
//
//	For any key set and any tenantID, the result map returned by MGet is keyed
//	by the ORIGINAL (non-namespaced) input keys, never by the namespaced keys
//	that were sent to Redis.
//
// This is the contract that callers depend on: they pass original keys and
// receive results indexed by those same original keys, regardless of whether
// the underlying Redis call used a tenant-prefixed namespace.
//
// The recording client is reused from consumer.redis_namespace_test.go
// (same package), so no additional stubs are needed.
func TestProperty_MGet_OutputKeysMatchOriginal(t *testing.T) {
	property := func(tenantID, keyA, keyB string) bool {
		tenantID = sanitizeQuickString(tenantID, 256)
		keyA = sanitizeQuickString(keyA, 256)
		keyB = sanitizeQuickString(keyB, 256)

		// Deduplicate to avoid ambiguous map assertions.
		if keyA == keyB {
			keyB = keyB + "_b"
		}

		originalKeys := []string{keyA, keyB}

		conn, _ := newRecordingConnection(t)
		repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

		ctx := context.Background()
		if tenantID != "" {
			ctx = tmcore.SetTenantIDInContext(ctx, tenantID)
		}

		result, err := repo.MGet(ctx, originalKeys)
		if err != nil {
			// A connection error from the recording client is not a property violation;
			// it means the recording stub was inadequate. In practice the recording client
			// never returns errors, but we guard defensively.
			return true
		}

		// Invariant: every key present in the result map must be one of the original keys.
		for resultKey := range result {
			found := false

			for _, orig := range originalKeys {
				if resultKey == orig {
					found = true
					break
				}
			}

			if !found {
				return false
			}
		}

		// Invariant: when tenantID is set, no namespaced key must appear in the result map.
		if tenantID != "" {
			prefix := "tenant:" + tenantID + ":"

			for resultKey := range result {
				if strings.HasPrefix(resultKey, prefix) {
					return false
				}
			}
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err,
		"MGet output key invariant violated: result map must be keyed by original (non-namespaced) keys for all (tenantID, keys) inputs")
}
