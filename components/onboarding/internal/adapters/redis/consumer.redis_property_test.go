// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

// =============================================================================
// PROPERTY-BASED TESTS — Redis Key Namespacing (T-002)
//
// These tests verify that the domain invariants of tmvalkey.GetKey and
// tmvalkey.StripTenantPrefix hold across hundreds of automatically-generated
// (tenantID, key) pairs.
//
// Run with:
//
//	go test -run TestProperty -v -count=1 \
//	    ./components/onboarding/internal/adapters/redis/
//
// Each TestProperty_* function uses testing/quick.Check and will report the
// counterexample that falsified the property if any violation is found.
// =============================================================================

import (
	"strings"
	"testing"
	"testing/quick"

	tmvalkey "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/valkey"
	"github.com/stretchr/testify/require"
)

// sanitizeQuickStringOnboarding trims generated strings to reasonable lengths so
// that quick.Check does not produce unbounded inputs that cause memory pressure.
// Bounding is required by the property-testing standard.
func sanitizeQuickStringOnboarding(s string, maxLen int) string {
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
// (or no-tenant) deployments — backwards-compatibility invariant for Set, Get,
// and Del in the onboarding Redis consumer.
func TestProperty_KeyNamespacing_Identity(t *testing.T) {
	property := func(key string) bool {
		key = sanitizeQuickStringOnboarding(key, 512)
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
// This guarantees that the namespaced key used by Set, Get, and Del is
// deterministically constructed and that the original key is always recoverable.
func TestProperty_KeyNamespacing_PrefixStructure(t *testing.T) {
	property := func(tenantID, key string) bool {
		tenantID = sanitizeQuickStringOnboarding(tenantID, 256)
		key = sanitizeQuickStringOnboarding(key, 512)

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
// This guarantees that every namespaced key used by Set/Get/Del can be reversed
// to recover the original key — critical for audit tracing and cache-miss
// detection in the onboarding consumer.
func TestProperty_KeyNamespacing_Reversibility(t *testing.T) {
	property := func(tenantID, key string) bool {
		tenantID = sanitizeQuickStringOnboarding(tenantID, 256)
		key = sanitizeQuickStringOnboarding(key, 512)

		namespaced := tmvalkey.GetKey(tenantID, key)
		recovered := tmvalkey.StripTenantPrefix(tenantID, namespaced)

		return recovered == key
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err,
		"Reversibility property violated: StripTenantPrefix(tenantID, GetKey(tenantID, key)) must equal key for all inputs")
}
