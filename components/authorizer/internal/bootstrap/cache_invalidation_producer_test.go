// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	crossshardevents "github.com/LerianStudio/midaz/v3/pkg/crossshard/events"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// TestBuildCacheInvalidationTargets_FromOperations asserts the producer-side
// derivation that runs at authorizeCrossShard time. This is the primary path
// the recovery runner replays against (intent.Aliases carried through the
// commits topic).
func TestBuildCacheInvalidationTargets_FromOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ops  []*authorizerv1.BalanceOperation
		want []cacheInvalidationTarget
	}{
		{
			name: "empty",
			ops:  nil,
			want: nil,
		},
		{
			name: "single operation",
			ops: []*authorizerv1.BalanceOperation{
				{AccountAlias: "@alice", BalanceKey: "default"},
			},
			want: []cacheInvalidationTarget{{Alias: "@alice", BalanceKey: "default"}},
		},
		{
			name: "deduplicates identical alias+key",
			ops: []*authorizerv1.BalanceOperation{
				{AccountAlias: "@alice", BalanceKey: "default"},
				{AccountAlias: "@alice", BalanceKey: "default"},
				{AccountAlias: "@bob", BalanceKey: "savings"},
			},
			want: []cacheInvalidationTarget{
				{Alias: "@alice", BalanceKey: "default"},
				{Alias: "@bob", BalanceKey: "savings"},
			},
		},
		{
			name: "distinguishes same alias with different balance keys",
			ops: []*authorizerv1.BalanceOperation{
				{AccountAlias: "@alice", BalanceKey: "default"},
				{AccountAlias: "@alice", BalanceKey: "savings"},
			},
			want: []cacheInvalidationTarget{
				{Alias: "@alice", BalanceKey: "default"},
				{Alias: "@alice", BalanceKey: "savings"},
			},
		},
		{
			name: "skips nil and empty-alias entries",
			ops: []*authorizerv1.BalanceOperation{
				nil,
				{AccountAlias: "", BalanceKey: "default"},
				{AccountAlias: "@only", BalanceKey: "default"},
			},
			want: []cacheInvalidationTarget{{Alias: "@only", BalanceKey: "default"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildCacheInvalidationTargets(tt.ops)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestAliasTargetsFromMutations_WALReconcile asserts the WAL reconciler path
// where the producer rebuilds the intent from a persisted WAL entry.
func TestAliasTargetsFromMutations_WALReconcile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []wal.BalanceMutation
		want []cacheInvalidationTarget
	}{
		{
			name: "empty mutations returns nil",
			in:   nil,
			want: nil,
		},
		{
			name: "deduplicates identical alias+key across pre/post entries",
			in: []wal.BalanceMutation{
				{AccountAlias: "@alice", BalanceKey: "default"},
				{AccountAlias: "@alice", BalanceKey: "default"},
				{AccountAlias: "@bob", BalanceKey: "savings"},
			},
			want: []cacheInvalidationTarget{
				{Alias: "@alice", BalanceKey: "default"},
				{Alias: "@bob", BalanceKey: "savings"},
			},
		},
		{
			name: "skips mutation with empty alias",
			in: []wal.BalanceMutation{
				{AccountAlias: "", BalanceKey: "default"},
				{AccountAlias: "@a", BalanceKey: "default"},
			},
			want: []cacheInvalidationTarget{{Alias: "@a", BalanceKey: "default"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := aliasTargetsFromMutations(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestPublishCacheInvalidation_PopulatesAliasesFromIntent asserts that the
// producer's published payload carries the Aliases that were captured on the
// commit intent. This is the handshake the transaction-side consumer relies
// on to DEL targeted keys.
func TestPublishCacheInvalidation_PopulatesAliasesFromIntent(t *testing.T) {
	t.Parallel()

	pub := &capturingPublisher{}
	svc := &authorizerService{pub: pub}

	intent := &commitIntent{
		TransactionID:  "tx-alias-test",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		Status:         commitIntentStatusCompleted,
		Aliases: []cacheInvalidationTarget{
			{Alias: "@alice", BalanceKey: "default"},
			{Alias: "@bob", BalanceKey: "savings"},
		},
	}

	require.NoError(t, svc.publishCacheInvalidation(context.Background(), intent))

	require.Len(t, pub.messages, 1)
	assert.Equal(t, crossShardCacheInvalidationTopic, pub.messages[0].Topic)

	var decoded crossshardevents.CacheInvalidationEvent
	require.NoError(t, json.Unmarshal(pub.messages[0].Payload, &decoded))

	assert.Equal(t, "tx-alias-test", decoded.TransactionID)
	assert.Equal(t, "org-1", decoded.OrganizationID)
	assert.Equal(t, "ledger-1", decoded.LedgerID)
	assert.Equal(t, crossshardevents.ReasonRecoveryDriveToCompletion, decoded.Reason)
	assert.Equal(t, intent.Aliases, decoded.Aliases)
	assert.False(t, decoded.EmittedAt.IsZero(), "EmittedAt must be populated")
}

// TestPublishCacheInvalidation_EmptyAliasesFallback documents the safety-net
// behaviour: when the intent does not carry Aliases (e.g. legacy WAL replay
// without Mutations), the event still publishes with an empty slice so the
// consumer can choose to scan / log.
func TestPublishCacheInvalidation_EmptyAliasesFallback(t *testing.T) {
	t.Parallel()

	pub := &capturingPublisher{}
	svc := &authorizerService{pub: pub}

	intent := &commitIntent{
		TransactionID:  "tx-no-aliases",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		Status:         commitIntentStatusCompleted,
		Aliases:        nil,
	}

	require.NoError(t, svc.publishCacheInvalidation(context.Background(), intent))

	require.Len(t, pub.messages, 1)

	var decoded crossshardevents.CacheInvalidationEvent
	require.NoError(t, json.Unmarshal(pub.messages[0].Payload, &decoded))

	assert.Empty(t, decoded.Aliases, "nil Aliases must serialize to empty slice for consumer safety-net")
}
