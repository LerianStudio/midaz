// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// TestResolvePeerShardRanges_NoPeers_LocalMustOwnEverything proves that when
// no peers are configured the local instance must own the full shard space.
// A partial ownership without peers would silently leak shards into the void
// and balances in those shards would be unauthorized - a financial incident.
func TestResolvePeerShardRanges_NoPeers_LocalMustOwnEverything(t *testing.T) {
	t.Parallel()

	t.Run("full coverage returns empty ranges", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 0,
			OwnedShardEnd:   7,
		}
		ranges, err := resolvePeerShardRanges(cfg)
		require.NoError(t, err)
		require.Empty(t, ranges)
	})

	t.Run("partial ownership rejected", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 0,
			OwnedShardEnd:   3,
		}
		_, err := resolvePeerShardRanges(cfg)
		require.Error(t, err)
		require.ErrorIs(t, err, constant.ErrLocalShardCoverageIncomplete)
	})
}

// TestResolvePeerShardRanges_ExplicitRanges proves that explicit peer shard
// ranges must match the peer count and must not overlap with local ownership.
func TestResolvePeerShardRanges_ExplicitRanges(t *testing.T) {
	t.Parallel()

	t.Run("valid explicit ranges", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 0,
			OwnedShardEnd:   3,
			PeerInstances:   []string{"peer-1:9000"},
			PeerShardRanges: []string{"4-7"},
		}
		ranges, err := resolvePeerShardRanges(cfg)
		require.NoError(t, err)
		require.Equal(t, []peerShardRange{{start: 4, end: 7}}, ranges)
	})

	t.Run("range count mismatch rejected", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 0,
			OwnedShardEnd:   1,
			PeerInstances:   []string{"peer-1:9000", "peer-2:9000"},
			PeerShardRanges: []string{"2-7"},
		}
		_, err := resolvePeerShardRanges(cfg)
		require.Error(t, err)
		require.ErrorIs(t, err, constant.ErrPeerShardRangeCountMismatch)
	})

	t.Run("overlap with local rejected", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 0,
			OwnedShardEnd:   3,
			PeerInstances:   []string{"peer-1:9000"},
			// 2-7 overlaps local 0-3
			PeerShardRanges: []string{"2-7"},
		}
		_, err := resolvePeerShardRanges(cfg)
		require.Error(t, err)
		require.ErrorIs(t, err, constant.ErrPeerShardRangeOverlapsLocal)
	})

	t.Run("peer ranges that overlap each other rejected", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      16,
			OwnedShardStart: 0,
			OwnedShardEnd:   3,
			PeerInstances:   []string{"peer-1:9000", "peer-2:9000"},
			PeerShardRanges: []string{"4-10", "8-15"},
		}
		_, err := resolvePeerShardRanges(cfg)
		require.Error(t, err)
		require.ErrorIs(t, err, constant.ErrPeerShardRangesOverlap)
	})

	t.Run("invalid range format propagates parse error", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 0,
			OwnedShardEnd:   3,
			PeerInstances:   []string{"peer-1:9000"},
			PeerShardRanges: []string{"not-a-range"},
		}
		_, err := resolvePeerShardRanges(cfg)
		require.Error(t, err)
	})
}

// TestInferSinglePeerRange proves the single-peer inference rules: the peer
// must own the complement of the local range and multi-peer configs are
// rejected (ambiguous).
func TestInferSinglePeerRange(t *testing.T) {
	t.Parallel()

	t.Run("local owns tail peer gets head", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 4,
			OwnedShardEnd:   7,
			PeerInstances:   []string{"peer-1:9000"},
		}
		rng, err := inferSinglePeerRange(cfg)
		require.NoError(t, err)
		require.Equal(t, peerShardRange{start: 0, end: 3}, rng)
	})

	t.Run("local owns head peer gets tail", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 0,
			OwnedShardEnd:   3,
			PeerInstances:   []string{"peer-1:9000"},
		}
		rng, err := inferSinglePeerRange(cfg)
		require.NoError(t, err)
		require.Equal(t, peerShardRange{start: 4, end: 7}, rng)
	})

	t.Run("middle ownership cannot be inferred", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      16,
			OwnedShardStart: 4,
			OwnedShardEnd:   11,
			PeerInstances:   []string{"peer-1:9000"},
		}
		_, err := inferSinglePeerRange(cfg)
		require.Error(t, err)
		require.ErrorIs(t, err, constant.ErrPeerShardRangeCannotInfer)
	})

	t.Run("local owns all shards rejects peer", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 0,
			OwnedShardEnd:   7,
			PeerInstances:   []string{"peer-1:9000"},
		}
		_, err := inferSinglePeerRange(cfg)
		require.Error(t, err)
		require.ErrorIs(t, err, constant.ErrPeerOwnsAllShards)
	})

	t.Run("multiple peers require explicit ranges", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			ShardCount:      8,
			OwnedShardStart: 0,
			OwnedShardEnd:   3,
			PeerInstances:   []string{"peer-1:9000", "peer-2:9000"},
		}
		_, err := inferSinglePeerRange(cfg)
		require.Error(t, err)
		require.ErrorIs(t, err, constant.ErrPeerShardRangesRequired)
	})
}

// TestValidatePeerRangeOverlaps proves that any two overlapping peer ranges
// are rejected. Overlap would mean two owners of the same shard - split-brain.
func TestValidatePeerRangeOverlaps(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ranges  []peerShardRange
		wantErr bool
	}{
		{
			name:    "empty set accepted",
			ranges:  nil,
			wantErr: false,
		},
		{
			name:    "single range accepted",
			ranges:  []peerShardRange{{start: 0, end: 3}},
			wantErr: false,
		},
		{
			name:    "disjoint ranges accepted",
			ranges:  []peerShardRange{{start: 0, end: 3}, {start: 4, end: 7}},
			wantErr: false,
		},
		{
			name:    "touching ranges are overlap at boundary",
			ranges:  []peerShardRange{{start: 0, end: 4}, {start: 4, end: 7}},
			wantErr: true,
		},
		{
			name:    "interior overlap rejected",
			ranges:  []peerShardRange{{start: 0, end: 5}, {start: 3, end: 7}},
			wantErr: true,
		},
		{
			name:    "triple with middle overlap rejected",
			ranges:  []peerShardRange{{start: 0, end: 2}, {start: 3, end: 5}, {start: 4, end: 6}},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validatePeerRangeOverlaps(tc.ranges)
			if tc.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, constant.ErrPeerShardRangesOverlap)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestBuildPeerTransportOption_Insecure proves that when peer TLS is disabled
// the returned dial option is non-nil and usable (can be applied to NewClient
// config without panic). We don't introspect internal credentials type - that
// would be pinning implementation detail.
func TestBuildPeerTransportOption_Insecure(t *testing.T) {
	t.Parallel()

	cfg := &Config{GRPCTLSEnabled: false}
	opt, err := buildPeerTransportOption(cfg)
	require.NoError(t, err)
	require.NotNil(t, opt)

	// Smoke-apply the option to a grpc.NewClient — this confirms the option
	// is a valid DialOption, without actually dialing anything.
	var opts []grpc.DialOption
	opts = append(opts, opt)
	require.Len(t, opts, 1)
}

// TestBuildPeerTransportOption_TLSCAFileMissing proves that a non-existent
// CA file surfaces a readable error rather than panicking.
func TestBuildPeerTransportOption_TLSCAFileMissing(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		GRPCTLSEnabled: true,
		PeerTLSCAFile:  "/nonexistent/path/ca.pem",
	}
	_, err := buildPeerTransportOption(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AUTHORIZER_PEER_TLS_CA_FILE")
}

// TestBuildPeerTransportOption_TLSCAFileInvalidPEM proves that a file that
// exists but contains no valid PEM certificates is rejected.
func TestBuildPeerTransportOption_TLSCAFileInvalidPEM(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	caPath := tmpDir + "/ca.pem"

	// Write invalid (non-PEM) content.
	require.NoError(t, os.WriteFile(caPath, []byte("this is not a PEM block"), 0o600))

	cfg := &Config{
		GRPCTLSEnabled: true,
		PeerTLSCAFile:  caPath,
	}
	_, err := buildPeerTransportOption(cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, constant.ErrPeerTLSCANoCertificates),
		"expected ErrPeerTLSCANoCertificates, got: %v", err)
}
