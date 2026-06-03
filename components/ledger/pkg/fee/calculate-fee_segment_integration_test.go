// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// integUUID returns a deterministic UUID for segment integration tests.
func integUUID(s string) uuid.UUID {
	return uuid.MustParse(s)
}

// ─── End-to-End: resolveSegmentWaivedAccounts + isAccountExemptWithSegments ──

// TestSegmentExemption_EndToEnd_SegmentMatch verifies that an account belonging to
// a waived segment is correctly identified as exempt through the combined flow.
func TestSegmentExemption_EndToEnd_SegmentMatch(t *testing.T) {
	t.Parallel()

	seg1 := integUUID("aaaaaaaa-0000-0000-0000-000000000001")

	tests := []struct {
		name          string
		waivedAccts   []string
		account       string
		accountSegID  *uuid.UUID
		wantExempt    bool
		wantCallCount int
	}{
		{
			name:          "account in waived segment is exempt",
			waivedAccts:   []string{"segment:aaaaaaaa-0000-0000-0000-000000000001"},
			account:       "acc1",
			accountSegID:  &seg1,
			wantExempt:    true,
			wantCallCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			segID := tt.accountSegID
			client := &mockSegmentMidazClient{
				getAccountDetailsFn: func(_ context.Context, _, _, _ string) (*pkg.Account, error) {
					return &pkg.Account{
						ID:        "acc1",
						Alias:     tt.account,
						SegmentID: segID,
					}, nil
				},
			}

			directAliases, segmentIDs, resolveErr := resolveSegmentWaivedAccounts(tt.waivedAccts)
			assert.NoError(t, resolveErr)

			got, gotErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&directAliases,
				segmentIDs,
				client,
				"org-1", "led-1",
			)

			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantExempt, got)
			assert.Equal(t, tt.wantCallCount, client.callCount)
		})
	}
}

// TestSegmentExemption_EndToEnd_SegmentNoMatch verifies that an account belonging to
// a different segment than the waived one is not exempt.
func TestSegmentExemption_EndToEnd_SegmentNoMatch(t *testing.T) {
	t.Parallel()

	differentSeg := integUUID("bbbbbbbb-0000-0000-0000-000000000002")

	tests := []struct {
		name         string
		waivedAccts  []string
		account      string
		accountSegID *uuid.UUID
		wantExempt   bool
	}{
		{
			name:         "account in different segment is not exempt",
			waivedAccts:  []string{"segment:aaaaaaaa-0000-0000-0000-000000000001"},
			account:      "acc1",
			accountSegID: &differentSeg,
			wantExempt:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			segID := tt.accountSegID
			client := &mockSegmentMidazClient{
				getAccountDetailsFn: func(_ context.Context, _, _, _ string) (*pkg.Account, error) {
					return &pkg.Account{
						ID:        "acc1",
						Alias:     tt.account,
						SegmentID: segID,
					}, nil
				},
			}

			directAliases, segmentIDs, resolveErr := resolveSegmentWaivedAccounts(tt.waivedAccts)
			assert.NoError(t, resolveErr)

			got, gotErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&directAliases,
				segmentIDs,
				client,
				"org-1", "led-1",
			)

			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantExempt, got)
		})
	}
}

// TestSegmentExemption_EndToEnd_NilSegmentOnAccount verifies that an account with
// a nil SegmentID is not exempt even when segment-based waivers are configured.
func TestSegmentExemption_EndToEnd_NilSegmentOnAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		waivedAccts []string
		account     string
		wantExempt  bool
	}{
		{
			name:        "account with nil segmentID is not exempt via segment waiver",
			waivedAccts: []string{"segment:aaaaaaaa-0000-0000-0000-000000000001"},
			account:     "acc1",
			wantExempt:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &mockSegmentMidazClient{
				getAccountDetailsFn: func(_ context.Context, _, _, _ string) (*pkg.Account, error) {
					return &pkg.Account{
						ID:        "acc1",
						Alias:     tt.account,
						SegmentID: nil,
					}, nil
				},
			}

			directAliases, segmentIDs, resolveErr := resolveSegmentWaivedAccounts(tt.waivedAccts)
			assert.NoError(t, resolveErr)

			got, gotErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&directAliases,
				segmentIDs,
				client,
				"org-1", "led-1",
			)

			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantExempt, got)
		})
	}
}

// TestSegmentExemption_EndToEnd_DirectAliasStillWorks verifies that a direct alias
// waiver resolves without any MidazClient call (fast path preserved end-to-end).
func TestSegmentExemption_EndToEnd_DirectAliasStillWorks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		waivedAccts   []string
		account       string
		wantExempt    bool
		wantCallCount int
	}{
		{
			name:          "direct alias waiver bypasses MidazClient entirely",
			waivedAccts:   []string{"acc1"},
			account:       "acc1",
			wantExempt:    true,
			wantCallCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &mockSegmentMidazClient{}

			directAliases, segmentIDs, resolveErr := resolveSegmentWaivedAccounts(tt.waivedAccts)
			assert.NoError(t, resolveErr)

			got, gotErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&directAliases,
				segmentIDs,
				client,
				"org-1", "led-1",
			)

			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantExempt, got)
			assert.Equal(t, tt.wantCallCount, client.callCount,
				"MidazClient must not be called when direct alias match resolves the check")
		})
	}
}

// TestSegmentExemption_EndToEnd_ExternalError_FEE0062 verifies that external errors
// when MidazClient returns an error (simulating FEE-0062 external service failure).
func TestSegmentExemption_EndToEnd_ExternalError_FEE0062(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		waivedAccts []string
		account     string
		clientErr   error
		wantExempt  bool
	}{
		{
			name:        "external client error returns error instead of silently charging exempt accounts",
			waivedAccts: []string{"segment:aaaaaaaa-0000-0000-0000-000000000001"},
			account:     "acc1",
			clientErr:   errors.New("FEE-0062: external service unavailable"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clientErr := tt.clientErr
			client := &mockSegmentMidazClient{
				getAccountDetailsFn: func(_ context.Context, _, _, _ string) (*pkg.Account, error) {
					return nil, clientErr
				},
			}

			directAliases, segmentIDs, resolveErr := resolveSegmentWaivedAccounts(tt.waivedAccts)
			assert.NoError(t, resolveErr)

			got, gotErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&directAliases,
				segmentIDs,
				client,
				"org-1", "led-1",
			)

			assert.Error(t, gotErr)
			assert.Contains(t, gotErr.Error(), "segment resolution failed")
			assert.False(t, got)
		})
	}
}

// TestSegmentExemption_EndToEnd_MixedWaivedAccounts verifies that a mixed
// waivedAccounts slice correctly exempts accounts via direct alias or segment match,
// while leaving unmatched accounts non-exempt.
func TestSegmentExemption_EndToEnd_MixedWaivedAccounts(t *testing.T) {
	t.Parallel()

	seg1 := integUUID("aaaaaaaa-0000-0000-0000-000000000001")

	tests := []struct {
		name          string
		queryAccount  string
		accountSegID  *uuid.UUID
		wantExempt    bool
		wantCallCount int
	}{
		{
			name:          "acc1 is exempt via direct alias match",
			queryAccount:  "acc1",
			accountSegID:  nil,
			wantExempt:    true,
			wantCallCount: 0,
		},
		{
			name:          "acc3 with waived segmentID is exempt via segment match",
			queryAccount:  "acc3",
			accountSegID:  &seg1,
			wantExempt:    true,
			wantCallCount: 1,
		},
		{
			name:          "acc4 with no segment match and no direct alias is not exempt",
			queryAccount:  "acc4",
			accountSegID:  nil,
			wantExempt:    false,
			wantCallCount: 1,
		},
	}

	waivedAccts := []string{"acc1", "segment:aaaaaaaa-0000-0000-0000-000000000001", "acc2"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			segID := tt.accountSegID
			client := &mockSegmentMidazClient{
				getAccountDetailsFn: func(_ context.Context, _, _, alias string) (*pkg.Account, error) {
					return &pkg.Account{
						ID:        alias,
						Alias:     alias,
						SegmentID: segID,
					}, nil
				},
			}

			directAliases, segmentIDs, resolveErr := resolveSegmentWaivedAccounts(waivedAccts)
			assert.NoError(t, resolveErr)

			got, gotErr := isAccountExemptWithSegments(
				context.Background(),
				tt.queryAccount,
				&directAliases,
				segmentIDs,
				client,
				"org-1", "led-1",
			)

			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantExempt, got)
			assert.Equal(t, tt.wantCallCount, client.callCount)
		})
	}
}

// TestSegmentExemption_EndToEnd_EmptyWaivedAccounts verifies that no account is
// exempt when waivedAccounts is empty — no MidazClient calls are made.
func TestSegmentExemption_EndToEnd_EmptyWaivedAccounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		waivedAccts   []string
		account       string
		wantExempt    bool
		wantCallCount int
	}{
		{
			name:          "empty waivedAccounts: no account is exempt",
			waivedAccts:   []string{},
			account:       "acc1",
			wantExempt:    false,
			wantCallCount: 0,
		},
		{
			name:          "nil waivedAccounts: no account is exempt",
			waivedAccts:   nil,
			account:       "acc1",
			wantExempt:    false,
			wantCallCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &mockSegmentMidazClient{}

			directAliases, segmentIDs, resolveErr := resolveSegmentWaivedAccounts(tt.waivedAccts)
			assert.NoError(t, resolveErr)

			got, gotErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&directAliases,
				segmentIDs,
				client,
				"org-1", "led-1",
			)

			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantExempt, got)
			assert.Equal(t, tt.wantCallCount, client.callCount,
				"MidazClient must not be called when waivedAccounts is empty")
		})
	}
}

// TestSegmentExemption_EndToEnd_CacheHitBehavior verifies that calling
// isAccountExemptWithSegments twice for the same account invokes MidazClient
// on each call (cache is outside the scope of these pure functions; the
// production CachedPackageRepository wraps at a higher layer).
func TestSegmentExemption_EndToEnd_CacheHitBehavior(t *testing.T) {
	t.Parallel()

	seg1 := integUUID("aaaaaaaa-0000-0000-0000-000000000001")

	tests := []struct {
		name               string
		waivedAccts        []string
		account            string
		accountSegID       *uuid.UUID
		wantExempt         bool
		wantCallCountAfter int
	}{
		{
			name:               "second call for same account invokes MidazClient again (no in-function cache)",
			waivedAccts:        []string{"segment:aaaaaaaa-0000-0000-0000-000000000001"},
			account:            "acc1",
			accountSegID:       &seg1,
			wantExempt:         true,
			wantCallCountAfter: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			segID := tt.accountSegID
			client := &mockSegmentMidazClient{
				getAccountDetailsFn: func(_ context.Context, _, _, _ string) (*pkg.Account, error) {
					return &pkg.Account{
						ID:        "acc1",
						Alias:     tt.account,
						SegmentID: segID,
					}, nil
				},
			}

			directAliases, segmentIDs, resolveErr := resolveSegmentWaivedAccounts(tt.waivedAccts)
			assert.NoError(t, resolveErr)

			// First call.
			firstResult, firstErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&directAliases,
				segmentIDs,
				client,
				"org-1", "led-1",
			)

			assert.NoError(t, firstErr)
			assert.Equal(t, tt.wantExempt, firstResult, "first call must return expected exemption result")

			// Second call — same inputs, function has no internal cache.
			secondResult, secondErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&directAliases,
				segmentIDs,
				client,
				"org-1", "led-1",
			)

			assert.NoError(t, secondErr)
			assert.Equal(t, tt.wantExempt, secondResult, "second call must return same exemption result")
			assert.Equal(t, tt.wantCallCountAfter, client.callCount,
				"each call must invoke MidazClient once since caching is not in-function")
		})
	}
}
