// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stringPtr is a tiny helper to take the address of a string literal in
// table-driven test data. The settings.OverdraftLimit field is *string.
func stringPtr(s string) *string { return &s }

// decimalEqual is a small assertion helper that prefers `decimal.Equal`
// over `reflect.DeepEqual` for decimal.Decimal values, because two
// decimals with the same numeric value can have different internal
// big.Int representations after arithmetic.
func decimalEqual(t *testing.T, want, got decimal.Decimal, msgAndArgs ...any) {
	t.Helper()
	assert.Truef(t, want.Equal(got), "want %s, got %s%v", want.String(), got.String(), msgAndArgs)
}

// ============================================================================
// ComputePosition — Available
// ============================================================================

func TestComputePosition_Available_PositiveNormalBalance(t *testing.T) {
	t.Parallel()

	b := Balance{
		Available:     decimal.NewFromInt(1000),
		OnHold:        decimal.NewFromInt(50),
		OverdraftUsed: decimal.Zero,
	}

	pos := b.ComputePosition()

	decimalEqual(t, decimal.NewFromInt(1000), pos.Available, "available with no overdraft used")
}

func TestComputePosition_Available_ZeroAtBoundary(t *testing.T) {
	t.Parallel()

	b := Balance{
		Available:     decimal.NewFromInt(50),
		OnHold:        decimal.Zero,
		OverdraftUsed: decimal.NewFromInt(50),
	}

	pos := b.ComputePosition()

	decimalEqual(t, decimal.Zero, pos.Available, "available - overdraftUsed = 0 at boundary")
}

func TestComputePosition_Available_NegativeWhenOverdrafted(t *testing.T) {
	t.Parallel()

	b := Balance{
		Available:     decimal.Zero,
		OnHold:        decimal.Zero,
		OverdraftUsed: decimal.NewFromInt(50),
	}

	pos := b.ComputePosition()

	decimalEqual(t, decimal.NewFromInt(-50), pos.Available, "negative position when fully overdrafted")
}

// ============================================================================
// ComputePosition — OnHold
// ============================================================================

func TestComputePosition_OnHold_MirroredVerbatim(t *testing.T) {
	t.Parallel()

	cases := []decimal.Decimal{
		decimal.Zero,
		decimal.NewFromInt(100),
		decimal.NewFromFloat(3.14159),
		decimal.NewFromInt(999_999_999),
	}

	for _, want := range cases {
		b := Balance{
			Available: decimal.NewFromInt(1000),
			OnHold:    want,
		}

		pos := b.ComputePosition()

		decimalEqual(t, want, pos.OnHold, "onHold should mirror verbatim")
	}
}

// ============================================================================
// ComputePosition — AvailableOverdraftLimit (all four cases)
// ============================================================================

func TestComputePosition_AvailableOverdraftLimit_Cases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		settings        *BalanceSettings
		overdraftUsed   decimal.Decimal
		wantHeadroomNil bool
		wantHeadroom    decimal.Decimal
	}{
		{
			name:            "Settings nil (legacy balance) → headroom = 0",
			settings:        nil,
			overdraftUsed:   decimal.Zero,
			wantHeadroomNil: false,
			wantHeadroom:    decimal.Zero,
		},
		{
			name: "AllowOverdraft=false → headroom = 0",
			settings: &BalanceSettings{
				AllowOverdraft:        false,
				OverdraftLimitEnabled: false,
				OverdraftLimit:        nil,
			},
			overdraftUsed:   decimal.Zero,
			wantHeadroomNil: false,
			wantHeadroom:    decimal.Zero,
		},
		{
			name: "AllowOverdraft=true unlimited → headroom = nil (omitted)",
			settings: &BalanceSettings{
				AllowOverdraft:        true,
				OverdraftLimitEnabled: false,
				OverdraftLimit:        nil,
			},
			overdraftUsed:   decimal.NewFromInt(123),
			wantHeadroomNil: true,
		},
		{
			name: "AllowOverdraft=true limited unused → headroom = limit",
			settings: &BalanceSettings{
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        stringPtr("500"),
			},
			overdraftUsed:   decimal.Zero,
			wantHeadroomNil: false,
			wantHeadroom:    decimal.NewFromInt(500),
		},
		{
			name: "AllowOverdraft=true limited partially used → headroom = limit - used",
			settings: &BalanceSettings{
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        stringPtr("500"),
			},
			overdraftUsed:   decimal.NewFromInt(200),
			wantHeadroomNil: false,
			wantHeadroom:    decimal.NewFromInt(300),
		},
		{
			name: "AllowOverdraft=true limited exactly at limit → headroom = 0",
			settings: &BalanceSettings{
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        stringPtr("500"),
			},
			overdraftUsed:   decimal.NewFromInt(500),
			wantHeadroomNil: false,
			wantHeadroom:    decimal.Zero,
		},
		{
			name: "AllowOverdraft=true limited with malformed limit → falls back to zero",
			settings: &BalanceSettings{
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        stringPtr("not-a-number"),
			},
			overdraftUsed:   decimal.NewFromInt(50),
			wantHeadroomNil: false,
			wantHeadroom:    decimal.NewFromInt(-50), // 0 - 50
		},
		{
			name: "AllowOverdraft=true OverdraftLimitEnabled=true but Limit nil → zero",
			settings: &BalanceSettings{
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        nil,
			},
			overdraftUsed:   decimal.Zero,
			wantHeadroomNil: false,
			wantHeadroom:    decimal.Zero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := Balance{
				Available:     decimal.NewFromInt(1000),
				OnHold:        decimal.Zero,
				OverdraftUsed: tt.overdraftUsed,
				Settings:      tt.settings,
			}

			pos := b.ComputePosition()

			if tt.wantHeadroomNil {
				assert.Nil(t, pos.AvailableOverdraftLimit, "headroom should be nil for unlimited overdraft")
			} else {
				require.NotNil(t, pos.AvailableOverdraftLimit, "headroom should be present")
				decimalEqual(t, tt.wantHeadroom, *pos.AvailableOverdraftLimit, "headroom value")
			}
		})
	}
}

// ============================================================================
// MarshalJSON — Balance
// ============================================================================

func TestBalance_MarshalJSON_IncludesPositionBlock(t *testing.T) {
	t.Parallel()

	b := Balance{
		ID:            "11111111-1111-1111-1111-111111111111",
		Alias:         "@alice",
		Available:     decimal.NewFromInt(1000),
		OnHold:        decimal.NewFromInt(100),
		OverdraftUsed: decimal.Zero,
		Settings: &BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        stringPtr("500"),
		},
	}

	raw, err := json.Marshal(b)
	require.NoError(t, err)

	// Decode into a generic map to assert structure.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	pos, ok := decoded["position"].(map[string]any)
	require.True(t, ok, "position must be present and be a JSON object")

	assert.Equal(t, "1000", pos["available"], "position.available should equal Available - OverdraftUsed (1000 - 0)")
	assert.Equal(t, "100", pos["onHold"], "position.onHold should mirror OnHold")
	assert.Equal(t, "500", pos["availableOverdraftLimit"], "position.availableOverdraftLimit should equal limit - used (500 - 0)")
}

func TestBalance_MarshalJSON_OmitsAvailableOverdraftLimit_WhenUnlimited(t *testing.T) {
	t.Parallel()

	b := Balance{
		Available:     decimal.NewFromInt(1000),
		OnHold:        decimal.Zero,
		OverdraftUsed: decimal.Zero,
		Settings: &BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: false, // unlimited
			OverdraftLimit:        nil,
		},
	}

	raw, err := json.Marshal(b)
	require.NoError(t, err)

	// Assert the key is literally absent from the JSON (omitempty on nil pointer).
	assert.NotContains(t, string(raw), `"availableOverdraftLimit"`,
		"availableOverdraftLimit must be omitted when overdraft is unlimited")

	// Position block is still present with available + onHold.
	assert.Contains(t, string(raw), `"position"`)
	assert.Contains(t, string(raw), `"available"`)
	assert.Contains(t, string(raw), `"onHold"`)
}

func TestBalance_MarshalJSON_NegativeAvailable_OnOverdraftedAccount(t *testing.T) {
	t.Parallel()

	b := Balance{
		Available:     decimal.Zero,
		OnHold:        decimal.Zero,
		OverdraftUsed: decimal.NewFromInt(75),
		Settings: &BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        stringPtr("100"),
		},
	}

	raw, err := json.Marshal(b)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	pos := decoded["position"].(map[string]any)
	assert.Equal(t, "-75", pos["available"], "position.available is negative when fully overdrafted")
	assert.Equal(t, "25", pos["availableOverdraftLimit"], "position.availableOverdraftLimit = 100 - 75 = 25")
}

func TestBalance_MarshalJSON_NoInfiniteRecursion(t *testing.T) {
	t.Parallel()

	// If MarshalJSON is incorrectly implemented (e.g. calls json.Marshal on
	// the receiver type rather than the alias type), it recurses forever
	// and crashes with a stack-overflow / "encountered a cycle" error.
	// This sanity test calls Marshal on a populated Balance and just
	// asserts the call returns without panicking.
	b := Balance{
		ID:            "deadbeef-dead-beef-dead-beefdeadbeef",
		Alias:         "@recursion-guard",
		Available:     decimal.NewFromInt(1),
		OnHold:        decimal.Zero,
		OverdraftUsed: decimal.Zero,
	}

	require.NotPanics(t, func() {
		_, err := json.Marshal(b)
		require.NoError(t, err)
	})
}

// ============================================================================
// MarshalJSON — Balance preserves all original fields
// ============================================================================

func TestBalance_MarshalJSON_PreservesOriginalFields(t *testing.T) {
	t.Parallel()

	b := Balance{
		ID:             "22222222-2222-2222-2222-222222222222",
		OrganizationID: "org-id",
		LedgerID:       "ledger-id",
		AccountID:      "account-id",
		Alias:          "@bob",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(500),
		OnHold:         decimal.NewFromInt(25),
		Version:        7,
		AccountType:    "creditCard",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      "credit",
		OverdraftUsed:  decimal.Zero,
		Metadata:       map[string]any{"foo": "bar"},
	}

	raw, err := json.Marshal(b)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	// Spot-check the original fields are still emitted.
	assert.Equal(t, "22222222-2222-2222-2222-222222222222", decoded["id"])
	assert.Equal(t, "@bob", decoded["alias"])
	assert.Equal(t, "default", decoded["key"])
	assert.Equal(t, "USD", decoded["assetCode"])
	assert.Equal(t, "500", decoded["available"])
	assert.Equal(t, "25", decoded["onHold"])
	assert.Equal(t, float64(7), decoded["version"])
	assert.Equal(t, "creditCard", decoded["accountType"])
	assert.Equal(t, true, decoded["allowSending"])
	assert.Equal(t, "credit", decoded["direction"])
	assert.Equal(t, "0", decoded["overdraftUsed"])

	// Plus the new position block.
	require.NotNil(t, decoded["position"])
}

// ============================================================================
// MarshalJSON — BalanceHistory
// ============================================================================

func TestBalanceHistory_MarshalJSON_IncludesPositionBlock(t *testing.T) {
	t.Parallel()

	bh := BalanceHistory{
		ID:            "33333333-3333-3333-3333-333333333333",
		Alias:         "@charlie",
		Available:     decimal.NewFromInt(2000),
		OnHold:        decimal.NewFromInt(500),
		OverdraftUsed: decimal.NewFromInt(100),
		Settings: &BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        stringPtr("300"),
		},
	}

	raw, err := json.Marshal(bh)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	pos, ok := decoded["position"].(map[string]any)
	require.True(t, ok, "position must be present on BalanceHistory")

	assert.Equal(t, "1900", pos["available"], "position.available = 2000 - 100")
	assert.Equal(t, "500", pos["onHold"])
	assert.Equal(t, "200", pos["availableOverdraftLimit"], "headroom = 300 - 100")
}

func TestBalanceHistory_ComputePosition_LegacySettingsNil(t *testing.T) {
	t.Parallel()

	bh := BalanceHistory{
		Available:     decimal.NewFromInt(100),
		OnHold:        decimal.Zero,
		OverdraftUsed: decimal.Zero,
		Settings:      nil,
	}

	pos := bh.ComputePosition()

	require.NotNil(t, pos.AvailableOverdraftLimit)
	decimalEqual(t, decimal.Zero, *pos.AvailableOverdraftLimit, "legacy → zero headroom")
}

// ============================================================================
// Non-pollution: BalanceRedis JSON shape unaffected
// ============================================================================

// TestBalanceRedis_MarshalJSON_NoPositionBlock asserts that adding
// MarshalJSON on Balance does NOT leak the position block into the
// BalanceRedis cache envelope. BalanceRedis is a separate type with its
// own (default) JSON marshaling; the Balance.MarshalJSON receiver only
// fires for Balance values.
func TestBalanceRedis_MarshalJSON_NoPositionBlock(t *testing.T) {
	t.Parallel()

	br := BalanceRedis{
		ID:                    "redis-id",
		Alias:                 "@cache",
		Key:                   "default",
		AccountID:             "account-id",
		AssetCode:             "USD",
		Available:             decimal.NewFromInt(100),
		OnHold:                decimal.Zero,
		Version:               1,
		AccountType:           "deposit",
		AllowSending:          1,
		AllowReceiving:        1,
		Direction:             "credit",
		OverdraftUsed:         "0",
		AllowOverdraft:        0,
		OverdraftLimitEnabled: 0,
		OverdraftLimit:        "0",
		BalanceScope:          "transactional",
	}

	raw, err := json.Marshal(br)
	require.NoError(t, err)

	// The cache envelope must NOT carry a position block.
	assert.NotContains(t, string(raw), `"position"`,
		"BalanceRedis JSON envelope must not include a position block")
	assert.NotContains(t, string(raw), `"availableOverdraftLimit"`)
}

// ============================================================================
// Non-pollution: Balance round-trip through marshal + custom unmarshal
// ============================================================================

// TestBalance_MarshalJSON_RoundTrip verifies that a marshal-then-unmarshal
// cycle preserves the source fields. The Position block injected by
// MarshalJSON is read-only on the wire — Go has no symmetrical
// UnmarshalJSON on Balance, so the unmarshal side will silently drop the
// `position` key (consumers don't need to round-trip it because it's
// always derivable from the source fields).
func TestBalance_MarshalJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	original := Balance{
		ID:            "44444444-4444-4444-4444-444444444444",
		Alias:         "@round-trip",
		Available:     decimal.NewFromInt(123),
		OnHold:        decimal.NewFromInt(45),
		OverdraftUsed: decimal.NewFromInt(7),
		Direction:     "credit",
		Settings: &BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        stringPtr("100"),
		},
	}

	raw, err := json.Marshal(original)
	require.NoError(t, err)

	// Sanity: position is in the wire.
	require.Contains(t, string(raw), `"position"`)

	// Unmarshal back into a fresh Balance. The default decoder ignores
	// the unknown `position` key (Go's json package skips fields that
	// don't match any struct field — Balance has no `Position` member).
	var restored Balance
	require.NoError(t, json.Unmarshal(raw, &restored))

	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.Alias, restored.Alias)
	decimalEqual(t, original.Available, restored.Available)
	decimalEqual(t, original.OnHold, restored.OnHold)
	decimalEqual(t, original.OverdraftUsed, restored.OverdraftUsed)
	assert.Equal(t, original.Direction, restored.Direction)
	require.NotNil(t, restored.Settings)
	assert.Equal(t, original.Settings.AllowOverdraft, restored.Settings.AllowOverdraft)
}

// ============================================================================
// Wire-format spot check
// ============================================================================

// TestBalance_MarshalJSON_PositionBlockShape locks the exact JSON shape of
// the position block — three keys, in the order produced by the Go
// encoder. Useful as a documentation anchor: a contributor reading this
// test sees the wire-format contract at a glance.
func TestBalance_MarshalJSON_PositionBlockShape(t *testing.T) {
	t.Parallel()

	b := Balance{
		Available:     decimal.NewFromInt(1000),
		OnHold:        decimal.NewFromInt(50),
		OverdraftUsed: decimal.NewFromInt(20),
		Settings: &BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        stringPtr("100"),
		},
	}

	raw, err := json.Marshal(b)
	require.NoError(t, err)

	// Substring check (order is what Go's encoder produces by struct field
	// order in the alias type).
	assert.True(t,
		strings.Contains(string(raw), `"position":{"available":"980","onHold":"50","availableOverdraftLimit":"80"}`),
		"position block shape: %s", string(raw))
}
