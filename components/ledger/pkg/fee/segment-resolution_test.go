// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"context"
	"errors"
	"testing"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// testOrgID and testLedgerID are fixed IDs used by the segment-resolution tests.
var (
	testOrgID    = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testLedgerID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

// mockSegmentResolver is a hand-rolled test double for pkg.MidazResolver,
// exercising only the GetAccountByAlias path used by isAccountExemptWithSegments.
type mockSegmentResolver struct {
	getAccountFn func(ctx context.Context, orgID, ledgerID uuid.UUID, alias string) (*pkg.Account, error)
	callCount    int
}

func (m *mockSegmentResolver) GetAccountByAlias(
	ctx context.Context,
	orgID, ledgerID uuid.UUID,
	alias string,
) (*pkg.Account, error) {
	m.callCount++
	if m.getAccountFn != nil {
		return m.getAccountFn(ctx, orgID, ledgerID, alias)
	}

	return nil, nil
}

func (m *mockSegmentResolver) AccountExistsByAlias(
	_ context.Context,
	_, _ uuid.UUID,
	_ string,
) error {
	return nil
}

func (m *mockSegmentResolver) ListAccounts(
	_ context.Context,
	_, _ uuid.UUID,
	_, _ *uuid.UUID,
) ([]pkg.Account, error) {
	return nil, nil
}

func (m *mockSegmentResolver) CountTransactionsByRoute(
	_ context.Context,
	_, _ uuid.UUID,
	_, _ string,
	_, _ time.Time,
) (int64, error) {
	return 0, nil
}

// Compile-time assertion: mockSegmentResolver implements pkg.MidazResolver.
var _ pkg.MidazResolver = (*mockSegmentResolver)(nil)

// segmentUUID returns a fixed, deterministic UUID for use in test cases.
func segmentUUID(s string) uuid.UUID {
	return uuid.MustParse(s)
}

// ─── resolveSegmentWaivedAccounts ────────────────────────────────────────────

func TestResolveSegmentWaivedAccounts_EmptyInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		waivedAccts  []string
		wantAliases  []string
		wantSegments []uuid.UUID
	}{
		{
			name:         "nil waivedAccounts returns empty slices",
			waivedAccts:  nil,
			wantAliases:  nil,
			wantSegments: nil,
		},
		{
			name:         "empty waivedAccounts returns empty slices",
			waivedAccts:  []string{},
			wantAliases:  nil,
			wantSegments: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotAliases, gotSegments, gotErr := resolveSegmentWaivedAccounts(tt.waivedAccts)

			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantAliases, gotAliases)
			assert.Equal(t, tt.wantSegments, gotSegments)
		})
	}
}

func TestResolveSegmentWaivedAccounts_OnlyDirectAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		waivedAccts []string
		wantAliases []string
	}{
		{
			name:        "single direct alias passes through unchanged",
			waivedAccts: []string{"account-alias-1"},
			wantAliases: []string{"account-alias-1"},
		},
		{
			name:        "multiple direct aliases all pass through unchanged",
			waivedAccts: []string{"account-alias-1", "account-alias-2", "account-alias-3"},
			wantAliases: []string{"account-alias-1", "account-alias-2", "account-alias-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotAliases, gotSegments, gotErr := resolveSegmentWaivedAccounts(tt.waivedAccts)

			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantAliases, gotAliases)
			assert.Empty(t, gotSegments, "no segment IDs expected when all entries are direct aliases")
		})
	}
}

func TestResolveSegmentWaivedAccounts_OnlySegmentReferences(t *testing.T) {
	t.Parallel()

	seg1 := segmentUUID("550e8400-e29b-41d4-a716-446655440001")
	seg2 := segmentUUID("550e8400-e29b-41d4-a716-446655440002")

	tests := []struct {
		name         string
		waivedAccts  []string
		wantSegments []uuid.UUID
	}{
		{
			name:         "single segment reference is parsed into segmentIDs",
			waivedAccts:  []string{"segment:550e8400-e29b-41d4-a716-446655440001"},
			wantSegments: []uuid.UUID{seg1},
		},
		{
			name: "multiple segment references are all parsed into segmentIDs",
			waivedAccts: []string{
				"segment:550e8400-e29b-41d4-a716-446655440001",
				"segment:550e8400-e29b-41d4-a716-446655440002",
			},
			wantSegments: []uuid.UUID{seg1, seg2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotAliases, gotSegments, gotErr := resolveSegmentWaivedAccounts(tt.waivedAccts)

			assert.NoError(t, gotErr)
			assert.Empty(t, gotAliases, "no direct aliases expected when all entries are segment references")
			assert.Equal(t, tt.wantSegments, gotSegments)
		})
	}
}

func TestResolveSegmentWaivedAccounts_MixedEntries(t *testing.T) {
	t.Parallel()

	seg1 := segmentUUID("550e8400-e29b-41d4-a716-446655440001")

	tests := []struct {
		name         string
		waivedAccts  []string
		wantAliases  []string
		wantSegments []uuid.UUID
	}{
		{
			name: "mixed entries split correctly into aliases and segment IDs",
			waivedAccts: []string{
				"account-alias-1",
				"segment:550e8400-e29b-41d4-a716-446655440001",
				"account-alias-2",
			},
			wantAliases:  []string{"account-alias-1", "account-alias-2"},
			wantSegments: []uuid.UUID{seg1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotAliases, gotSegments, gotErr := resolveSegmentWaivedAccounts(tt.waivedAccts)

			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantAliases, gotAliases)
			assert.Equal(t, tt.wantSegments, gotSegments)
		})
	}
}

func TestResolveSegmentWaivedAccounts_MalformedSegmentUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		waivedAccts []string
		errContains string
	}{
		{
			name:        "invalid UUID after segment prefix returns error",
			waivedAccts: []string{"account-alias-1", "segment:not-a-valid-uuid"},
			errContains: "malformed segment waiver",
		},
		{
			name:        "empty UUID after segment prefix returns error",
			waivedAccts: []string{"segment:"},
			errContains: "malformed segment waiver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotAliases, gotSegments, gotErr := resolveSegmentWaivedAccounts(tt.waivedAccts)

			assert.Error(t, gotErr)
			assert.Contains(t, gotErr.Error(), tt.errContains)
			assert.Nil(t, gotAliases)
			assert.Nil(t, gotSegments)
		})
	}
}

// ─── isAccountExemptWithSegments ─────────────────────────────────────────────

func TestIsAccountExemptWithSegments_DirectAliasMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		account       string
		directAliases []string
		wantExempt    bool
		wantCalls     int
	}{
		{
			name:          "account matching direct alias returns true without calling MidazClient",
			account:       "account-alias-1",
			directAliases: []string{"account-alias-1", "account-alias-2"},
			wantExempt:    true,
			wantCalls:     0,
		},
		{
			name:          "account not in direct aliases with no segment IDs returns false",
			account:       "unknown-account",
			directAliases: []string{"account-alias-1"},
			wantExempt:    false,
			wantCalls:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := &mockSegmentResolver{}
			aliases := tt.directAliases

			got, gotExemptErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&aliases,
				nil,
				resolver,
				testOrgID, testLedgerID,
			)

			assert.NoError(t, gotExemptErr)
			assert.Equal(t, tt.wantExempt, got)
			assert.Equal(t, tt.wantCalls, resolver.callCount,
				"MidazClient must not be called when direct alias match resolves the check")
		})
	}
}

func TestIsAccountExemptWithSegments_NoSegmentIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		account    string
		wantCalls  int
		wantExempt bool
	}{
		{
			name:       "no segment IDs and no direct match returns false without calling MidazClient",
			account:    "unknown-account",
			wantCalls:  0,
			wantExempt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := &mockSegmentResolver{}
			aliases := []string{"other-account"}

			got, gotExemptErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&aliases,
				[]uuid.UUID{},
				resolver,
				testOrgID, testLedgerID,
			)

			assert.NoError(t, gotExemptErr)
			assert.Equal(t, tt.wantExempt, got)
			assert.Equal(t, tt.wantCalls, resolver.callCount)
		})
	}
}

func TestIsAccountExemptWithSegments_SegmentMatch(t *testing.T) {
	t.Parallel()

	seg1 := segmentUUID("550e8400-e29b-41d4-a716-446655440001")

	tests := []struct {
		name       string
		account    string
		segmentIDs []uuid.UUID
		accountFn  func(ctx context.Context, orgID, ledgerID uuid.UUID, alias string) (*pkg.Account, error)
		wantExempt bool
		wantCalls  int
	}{
		{
			name:       "account whose segmentID matches a waived segment returns true",
			account:    "target-account",
			segmentIDs: []uuid.UUID{seg1},
			accountFn: func(_ context.Context, _, _ uuid.UUID, _ string) (*pkg.Account, error) {
				return &pkg.Account{
					ID:        "acc-001",
					Alias:     "target-account",
					SegmentID: &seg1,
				}, nil
			},
			wantExempt: true,
			wantCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := &mockSegmentResolver{getAccountFn: tt.accountFn}
			aliases := []string{}

			got, gotExemptErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&aliases,
				tt.segmentIDs,
				resolver,
				testOrgID, testLedgerID,
			)

			assert.NoError(t, gotExemptErr)
			assert.Equal(t, tt.wantExempt, got)
			assert.Equal(t, tt.wantCalls, resolver.callCount)
		})
	}
}

func TestIsAccountExemptWithSegments_SegmentNoMatch(t *testing.T) {
	t.Parallel()

	waivedSeg := segmentUUID("550e8400-e29b-41d4-a716-446655440001")
	accountSeg := segmentUUID("550e8400-e29b-41d4-a716-446655440099")

	tests := []struct {
		name       string
		account    string
		segmentIDs []uuid.UUID
		accountFn  func(ctx context.Context, orgID, ledgerID uuid.UUID, alias string) (*pkg.Account, error)
		wantExempt bool
	}{
		{
			name:       "account whose segmentID does not match any waived segment returns false",
			account:    "target-account",
			segmentIDs: []uuid.UUID{waivedSeg},
			accountFn: func(_ context.Context, _, _ uuid.UUID, _ string) (*pkg.Account, error) {
				return &pkg.Account{
					ID:        "acc-002",
					Alias:     "target-account",
					SegmentID: &accountSeg,
				}, nil
			},
			wantExempt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := &mockSegmentResolver{getAccountFn: tt.accountFn}
			aliases := []string{}

			got, gotExemptErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&aliases,
				tt.segmentIDs,
				resolver,
				testOrgID, testLedgerID,
			)

			assert.NoError(t, gotExemptErr)
			assert.Equal(t, tt.wantExempt, got)
		})
	}
}

func TestIsAccountExemptWithSegments_MidazClientError(t *testing.T) {
	t.Parallel()

	seg1 := segmentUUID("550e8400-e29b-41d4-a716-446655440001")

	tests := []struct {
		name       string
		account    string
		segmentIDs []uuid.UUID
		clientErr  error
	}{
		{
			name:       "MidazClient error returns error instead of silently charging exempt accounts",
			account:    "target-account",
			segmentIDs: []uuid.UUID{seg1},
			clientErr:  errors.New("midaz unavailable"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := &mockSegmentResolver{
				getAccountFn: func(_ context.Context, _, _ uuid.UUID, _ string) (*pkg.Account, error) {
					return nil, tt.clientErr
				},
			}
			aliases := []string{}

			got, gotExemptErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&aliases,
				tt.segmentIDs,
				resolver,
				testOrgID, testLedgerID,
			)

			assert.Error(t, gotExemptErr)
			assert.Contains(t, gotExemptErr.Error(), "segment resolution failed")
			assert.False(t, got)
		})
	}
}

func TestIsAccountExemptWithSegments_NilMidazClient(t *testing.T) {
	t.Parallel()

	seg1 := segmentUUID("550e8400-e29b-41d4-a716-446655440001")

	tests := []struct {
		name       string
		account    string
		segmentIDs []uuid.UUID
		wantExempt bool
	}{
		{
			name:       "nil MidazClient with segment IDs returns false without panic",
			account:    "target-account",
			segmentIDs: []uuid.UUID{seg1},
			wantExempt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			aliases := []string{}

			got, gotExemptErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&aliases,
				tt.segmentIDs,
				nil,
				testOrgID, testLedgerID,
			)

			assert.NoError(t, gotExemptErr)
			assert.Equal(t, tt.wantExempt, got)
		})
	}
}

func TestIsAccountExemptWithSegments_AccountHasNilSegmentID(t *testing.T) {
	t.Parallel()

	seg1 := segmentUUID("550e8400-e29b-41d4-a716-446655440001")

	tests := []struct {
		name       string
		account    string
		segmentIDs []uuid.UUID
		wantExempt bool
	}{
		{
			name:       "account with nil segmentID is not exempt even when segment IDs are waived",
			account:    "target-account",
			segmentIDs: []uuid.UUID{seg1},
			wantExempt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := &mockSegmentResolver{
				getAccountFn: func(_ context.Context, _, _ uuid.UUID, _ string) (*pkg.Account, error) {
					return &pkg.Account{
						ID:        "acc-003",
						Alias:     "target-account",
						SegmentID: nil,
					}, nil
				},
			}
			aliases := []string{}

			got, gotExemptErr := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&aliases,
				tt.segmentIDs,
				resolver,
				testOrgID, testLedgerID,
			)

			assert.NoError(t, gotExemptErr)
			assert.Equal(t, tt.wantExempt, got)
		})
	}
}

func TestIsAccountExemptWithSegments_ExternalAccount(t *testing.T) {
	t.Parallel()

	segID := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")

	tests := []struct {
		name          string
		account       string
		directAliases []string
		wantExempt    bool
		wantCalls     int
	}{
		{
			name:          "@external/BRL returns false without calling MidazClient",
			account:       "@external/BRL",
			directAliases: []string{},
			wantExempt:    false,
			wantCalls:     0,
		},
		{
			name:          "@external/USD returns false without calling MidazClient",
			account:       "@external/USD",
			directAliases: []string{},
			wantExempt:    false,
			wantCalls:     0,
		},
		{
			name:          "@external/BRL explicitly in directAliases still returns true",
			account:       "@external/BRL",
			directAliases: []string{"@external/BRL"},
			wantExempt:    true,
			wantCalls:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := &mockSegmentResolver{}
			aliases := tt.directAliases

			got, err := isAccountExemptWithSegments(
				context.Background(),
				tt.account,
				&aliases,
				[]uuid.UUID{segID},
				resolver,
				testOrgID, testLedgerID,
			)

			assert.NoError(t, err)
			assert.Equal(t, tt.wantExempt, got)
			assert.Equal(t, tt.wantCalls, resolver.callCount,
				"MidazClient must not be called for @external/* accounts")
		})
	}
}
