// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package shard

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouter_DefaultShardCount(t *testing.T) {
	r := NewRouter(0)
	assert.Equal(t, DefaultShardCount, r.ShardCount(), "NewRouter(0) should use DefaultShardCount")

	r = NewRouter(-1)
	assert.Equal(t, DefaultShardCount, r.ShardCount(), "NewRouter(-1) should use DefaultShardCount")
}

func TestNewRouter_CustomShardCount(t *testing.T) {
	for _, count := range []int{1, 4, 8, 16, 32} {
		r := NewRouter(count)
		assert.Equal(t, count, r.ShardCount(), "NewRouter(%d) should have %d shards", count, count)
	}
}

func TestResolve_Deterministic(t *testing.T) {
	r := NewRouter(8)
	alias := "@user_abc123"

	shard1 := r.Resolve(alias)
	shard2 := r.Resolve(alias)
	shard3 := r.Resolve(alias)

	assert.Equal(t, shard1, shard2, "Resolve not deterministic: first and second call differ")
	assert.Equal(t, shard2, shard3, "Resolve not deterministic: second and third call differ")
}

func TestResolve_InRange(t *testing.T) {
	for _, shardCount := range []int{1, 4, 8, 16} {
		r := NewRouter(shardCount)

		for i := 0; i < 1000; i++ {
			alias := fmt.Sprintf("@user_%d", i)
			shard := r.Resolve(alias)

			assert.GreaterOrEqual(t, shard, 0, "Resolve(%q) = %d, expected shard >= 0", alias, shard)
			assert.Less(t, shard, shardCount, "Resolve(%q) = %d, expected shard < %d", alias, shard, shardCount)
		}
	}
}

func TestResolve_Distribution(t *testing.T) {
	// Verify FNV-1a gives reasonably even distribution across 8 shards.
	// With 10000 accounts, each shard should get ~1250 ± 15% (very loose).
	r := NewRouter(8)
	counts := make([]int, 8)

	const n = 10000

	for i := 0; i < n; i++ {
		alias := fmt.Sprintf("@user_%d", i)
		counts[r.Resolve(alias)]++
	}

	expected := float64(n) / float64(r.ShardCount())
	tolerance := 0.20 // 20% tolerance

	for shard, count := range counts {
		deviation := math.Abs(float64(count)-expected) / expected
		assert.LessOrEqual(t, deviation, tolerance,
			"Shard %d: %d accounts (expected ~%.0f, deviation %.1f%% > %.0f%%)",
			shard, count, expected, deviation*100, tolerance*100)
	}
}

func TestResolve_DifferentAliasesDifferentShards(t *testing.T) {
	// With 8 shards and different aliases, we should see at least 2 distinct shards
	r := NewRouter(8)
	seen := make(map[int]bool)

	aliases := []string{"@alice", "@bob", "@charlie", "@dave", "@eve", "@frank", "@grace", "@heidi"}
	for _, alias := range aliases {
		seen[r.Resolve(alias)] = true
	}

	assert.GreaterOrEqual(t, len(seen), 2, "Expected at least 2 distinct shards for 8 aliases, got %d", len(seen))
}

func TestResolve_SingleShard(t *testing.T) {
	// With 1 shard, everything maps to shard 0
	r := NewRouter(1)

	for i := 0; i < 100; i++ {
		shard := r.Resolve(fmt.Sprintf("@user_%d", i))
		assert.Equal(t, 0, shard, "Single shard: Resolve returned %d, want 0", shard)
	}
}

func TestResolveBalance(t *testing.T) {
	r := NewRouter(8)

	t.Run("regular account uses alias hash", func(t *testing.T) {
		alias := "@user_123"
		shardID := r.ResolveBalance(alias, "default")

		assert.Equal(t, r.Resolve(alias), shardID, "ResolveBalance regular mismatch")
	})

	t.Run("external with pre-split key uses key shard", func(t *testing.T) {
		shardID := r.ResolveBalance("@external/USD", "shard_3")

		assert.Equal(t, 3, shardID, "ResolveBalance external key mismatch")
	})

	t.Run("external with invalid key falls back to alias hash", func(t *testing.T) {
		alias := "@external/USD"
		shardID := r.ResolveBalance(alias, "default")

		assert.Equal(t, r.Resolve(alias), shardID, "ResolveBalance fallback mismatch")
	})
}

func TestResolveExternalBalanceKey(t *testing.T) {
	r := NewRouter(8)

	alias := "@user_abc"
	key := r.ResolveExternalBalanceKey(alias)
	want := ExternalBalanceKey(r.Resolve(alias))

	assert.Equal(t, want, key, "ResolveExternalBalanceKey(%q) = %q, want %q", alias, key, want)
}

func TestParseExternalBalanceShardID(t *testing.T) {
	tests := []struct {
		key      string
		wantID   int
		wantOkay bool
	}{
		{key: "shard_0", wantID: 0, wantOkay: true},
		{key: "shard_7", wantID: 7, wantOkay: true},
		{key: "shard_15", wantID: 15, wantOkay: true},
		{key: "default", wantID: 0, wantOkay: false},
		{key: "shard_", wantID: 0, wantOkay: false},
		{key: "shard_-1", wantID: 0, wantOkay: false},
		{key: "shard_abc", wantID: 0, wantOkay: false},
	}

	for _, tt := range tests {
		gotID, gotOkay := ParseExternalBalanceShardID(tt.key)

		assert.Equal(t, tt.wantOkay, gotOkay, "ParseExternalBalanceShardID(%q) ok mismatch", tt.key)
		assert.Equal(t, tt.wantID, gotID, "ParseExternalBalanceShardID(%q) id mismatch", tt.key)

		assert.Equal(t, tt.wantOkay, IsExternalBalanceKey(tt.key), "IsExternalBalanceKey(%q) mismatch", tt.key)
	}
}

func TestIsExternal(t *testing.T) {
	tests := []struct {
		alias    string
		expected bool
	}{
		{"@external/USD", true},
		{"@external/BTC", true},
		{"@external/EUR", true},
		{"@external/", true}, // edge: prefix with no asset
		{"@user_123", false},
		{"@alice", false},
		{"external/USD", false},  // missing @
		{"@External/USD", false}, // wrong case
		{"@external_USD", false}, // underscore not slash
		{"", false},              // empty
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, IsExternal(tt.alias), "IsExternal(%q) mismatch", tt.alias)
	}
}

func TestHashTag(t *testing.T) {
	tests := []struct {
		shardID  int
		expected string
	}{
		{0, "{shard_0}"},
		{1, "{shard_1}"},
		{7, "{shard_7}"},
		{15, "{shard_15}"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, HashTag(tt.shardID), "HashTag(%d) mismatch", tt.shardID)
	}
}

func TestExtractAccountAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"@user_123#default", "@user_123"},
		{"@external/USD#default", "@external/USD"},
		{"@alice#asset-freeze", "@alice"},
		{"@user_123", "@user_123"},         // no # suffix
		{"@external/BTC", "@external/BTC"}, // no # suffix
		{"#default", ""},                   // edge: starts with #
		{"", ""},                           // empty
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, ExtractAccountAlias(tt.input), "ExtractAccountAlias(%q) mismatch", tt.input)
	}
}

func TestResolveTransaction_InternalTransfer(t *testing.T) {
	// Two regular accounts on potentially different shards
	r := NewRouter(8)
	aliases := []string{"@user_A", "@user_B"}

	result := r.ResolveTransaction(aliases)

	require.Len(t, result, 2, "Expected 2 entries in result map")

	assert.Equal(t, r.Resolve("@user_A"), result["@user_A"], "@user_A shard mismatch")
	assert.Equal(t, r.Resolve("@user_B"), result["@user_B"], "@user_B shard mismatch")
}

func TestResolveTransaction_Inflow(t *testing.T) {
	// Inflow: @external/USD (debit) → @user_123 (credit)
	// External uses deterministic alias hashing.
	r := NewRouter(8)
	aliases := []string{"@external/USD", "@user_123"}

	result := r.ResolveTransaction(aliases)

	externalShard := r.Resolve("@external/USD")
	userShard := r.Resolve("@user_123")

	assert.Equal(t, externalShard, result["@external/USD"], "External should route to deterministic shard %d", externalShard)
	assert.Equal(t, userShard, result["@user_123"], "User should route to own shard %d", userShard)
}

func TestResolveTransaction_Outflow(t *testing.T) {
	// Outflow: @user_456 (debit) → @external/USD (credit)
	// External uses deterministic alias hashing.
	r := NewRouter(8)
	aliases := []string{"@user_456", "@external/USD"}

	result := r.ResolveTransaction(aliases)

	externalShard := r.Resolve("@external/USD")

	assert.Equal(t, externalShard, result["@external/USD"], "External should route to deterministic shard %d", externalShard)
}

func TestResolveTransaction_AllExternal(t *testing.T) {
	// All-external transactions still use deterministic routing.
	r := NewRouter(8)
	aliases := []string{"@external/USD", "@external/BTC"}

	result := r.ResolveTransaction(aliases)

	for alias, shard := range result {
		expected := r.Resolve(alias)
		assert.Equal(t, expected, shard, "All-external: %s should be deterministic shard %d, got %d", alias, expected, shard)
	}
}

func TestResolveTransaction_MultipleNonExternal(t *testing.T) {
	// Complex: multiple non-externals + external
	// Every alias resolves deterministically.
	r := NewRouter(8)
	aliases := []string{"@user_A", "@external/USD", "@user_B"}

	result := r.ResolveTransaction(aliases)

	externalShard := r.Resolve("@external/USD")

	assert.Equal(t, externalShard, result["@external/USD"], "External should route to deterministic shard %d", externalShard)

	// Non-externals get their own shards
	assert.Equal(t, r.Resolve("@user_A"), result["@user_A"], "@user_A shard mismatch")
	assert.Equal(t, r.Resolve("@user_B"), result["@user_B"], "@user_B shard mismatch")
}

func TestResolveTransaction_Empty(t *testing.T) {
	r := NewRouter(8)
	result := r.ResolveTransaction(nil)

	assert.Empty(t, result, "Expected empty map for nil aliases")

	result = r.ResolveTransaction([]string{})
	assert.Empty(t, result, "Expected empty map for empty aliases")
}

func BenchmarkResolve(b *testing.B) {
	r := NewRouter(8)
	alias := "@user_benchmark_12345"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Resolve(alias)
	}
}

func TestSplitAliasAndBalanceKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantAlias string
		wantKey   string
	}{
		{
			name:      "standard alias with default key",
			input:     "@user#default",
			wantAlias: "@user",
			wantKey:   "default",
		},
		{
			name:      "multiple hash chars preserves everything after first hash",
			input:     "@user#key#extra",
			wantAlias: "@user",
			wantKey:   "key#extra",
		},
		{
			name:      "empty key after hash falls back to default",
			input:     "@user#",
			wantAlias: "@user",
			wantKey:   "default",
		},
		{
			name:      "no separator falls back to default",
			input:     "@user",
			wantAlias: "@user",
			wantKey:   "default",
		},
		{
			name:      "external alias with shard key",
			input:     "@external/USD#shard_3",
			wantAlias: "@external/USD",
			wantKey:   "shard_3",
		},
		{
			name:      "empty string returns empty alias and default key",
			input:     "",
			wantAlias: "",
			wantKey:   "default",
		},
		{
			name:      "hash only returns empty alias and default key",
			input:     "#",
			wantAlias: "",
			wantKey:   "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotAlias, gotKey := SplitAliasAndBalanceKey(tt.input)

			require.Equal(t, tt.wantAlias, gotAlias,
				"SplitAliasAndBalanceKey(%q) alias mismatch", tt.input)
			require.Equal(t, tt.wantKey, gotKey,
				"SplitAliasAndBalanceKey(%q) balanceKey mismatch", tt.input)
		})
	}
}

func BenchmarkResolveTransaction(b *testing.B) {
	r := NewRouter(8)
	aliases := []string{"@external/USD", "@user_A", "@user_B"}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.ResolveTransaction(aliases)
	}
}
