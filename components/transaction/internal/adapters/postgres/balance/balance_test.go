package balance

import (
	"database/sql"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestBalancePostgreSQLModel_ToEntity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		model    BalancePostgreSQLModel
		expected *mmodel.Balance
	}{
		{
			name: "converts all fields correctly",
			model: BalancePostgreSQLModel{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@test-alias",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(100),
				Version:        5,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: false,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				DeletedAt:      sql.NullTime{},
			},
			expected: &mmodel.Balance{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@test-alias",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(100),
				Version:        5,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: false,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "handles zero values",
			model: BalancePostgreSQLModel{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "",
				Key:            "",
				AssetCode:      "",
				Available:      decimal.Zero,
				OnHold:         decimal.Zero,
				Version:        0,
				AccountType:    "",
				AllowSending:   false,
				AllowReceiving: false,
				CreatedAt:      time.Time{},
				UpdatedAt:      time.Time{},
				DeletedAt:      sql.NullTime{},
			},
			expected: &mmodel.Balance{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "",
				Key:            "",
				AssetCode:      "",
				Available:      decimal.Zero,
				OnHold:         decimal.Zero,
				Version:        0,
				AccountType:    "",
				AllowSending:   false,
				AllowReceiving: false,
				CreatedAt:      time.Time{},
				UpdatedAt:      time.Time{},
			},
		},
		{
			name: "handles large decimal values",
			model: BalancePostgreSQLModel{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@large-values",
				Key:            "default",
				AssetCode:      "BTC",
				Available:      decimal.RequireFromString("123456789012345678901234567890.123456789012345678901234567890"),
				OnHold:         decimal.RequireFromString("987654321098765432109876543210.987654321098765432109876543210"),
				Version:        1,
				AccountType:    "crypto",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				DeletedAt:      sql.NullTime{},
			},
			expected: &mmodel.Balance{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@large-values",
				Key:            "default",
				AssetCode:      "BTC",
				Available:      decimal.RequireFromString("123456789012345678901234567890.123456789012345678901234567890"),
				OnHold:         decimal.RequireFromString("987654321098765432109876543210.987654321098765432109876543210"),
				Version:        1,
				AccountType:    "crypto",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.model.ToEntity()

			assert.Equal(t, tt.expected.ID, result.ID)
			assert.Equal(t, tt.expected.OrganizationID, result.OrganizationID)
			assert.Equal(t, tt.expected.LedgerID, result.LedgerID)
			assert.Equal(t, tt.expected.AccountID, result.AccountID)
			assert.Equal(t, tt.expected.Alias, result.Alias)
			assert.Equal(t, tt.expected.Key, result.Key)
			assert.Equal(t, tt.expected.AssetCode, result.AssetCode)
			assert.True(t, tt.expected.Available.Equal(result.Available), "Available mismatch: expected %s, got %s", tt.expected.Available, result.Available)
			assert.True(t, tt.expected.OnHold.Equal(result.OnHold), "OnHold mismatch: expected %s, got %s", tt.expected.OnHold, result.OnHold)
			assert.Equal(t, tt.expected.Version, result.Version)
			assert.Equal(t, tt.expected.AccountType, result.AccountType)
			assert.Equal(t, tt.expected.AllowSending, result.AllowSending)
			assert.Equal(t, tt.expected.AllowReceiving, result.AllowReceiving)
			assert.Equal(t, tt.expected.CreatedAt, result.CreatedAt)
			assert.Equal(t, tt.expected.UpdatedAt, result.UpdatedAt)
			assert.Nil(t, result.DeletedAt, "DeletedAt should be nil when model.DeletedAt is not valid")
		})
	}
}

func TestBalancePostgreSQLModel_FromEntity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		balance  *mmodel.Balance
		expected BalancePostgreSQLModel
	}{
		{
			name: "converts all fields correctly",
			balance: &mmodel.Balance{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@test-alias",
				Key:            "custom-key",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(100),
				Version:        5,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: false,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			expected: BalancePostgreSQLModel{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@test-alias",
				Key:            "custom-key",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(100),
				Version:        5,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: false,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				DeletedAt:      sql.NullTime{},
			},
		},
		{
			name: "sets key to default when empty",
			balance: &mmodel.Balance{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@test",
				Key:            "", // empty key
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(100),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			expected: BalancePostgreSQLModel{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@test",
				Key:            "default", // should be set to "default"
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(100),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				DeletedAt:      sql.NullTime{},
			},
		},
		{
			name: "handles deleted_at when set",
			balance: &mmodel.Balance{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@deleted",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(0),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   false,
				AllowReceiving: false,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				DeletedAt:      timePtr(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)),
			},
			expected: BalancePostgreSQLModel{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@deleted",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(0),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   false,
				AllowReceiving: false,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				DeletedAt:      sql.NullTime{Time: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			},
		},
		{
			name: "handles large decimal values",
			balance: &mmodel.Balance{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@large",
				Key:            "default",
				AssetCode:      "BTC",
				Available:      decimal.RequireFromString("123456789012345678901234567890.123456789012345678901234567890"),
				OnHold:         decimal.RequireFromString("987654321098765432109876543210.987654321098765432109876543210"),
				Version:        1,
				AccountType:    "crypto",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			expected: BalancePostgreSQLModel{
				ID:             "00000000-0000-0000-0000-000000000001",
				OrganizationID: "00000000-0000-0000-0000-000000000002",
				LedgerID:       "00000000-0000-0000-0000-000000000003",
				AccountID:      "00000000-0000-0000-0000-000000000004",
				Alias:          "@large",
				Key:            "default",
				AssetCode:      "BTC",
				Available:      decimal.RequireFromString("123456789012345678901234567890.123456789012345678901234567890"),
				OnHold:         decimal.RequireFromString("987654321098765432109876543210.987654321098765432109876543210"),
				Version:        1,
				AccountType:    "crypto",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				DeletedAt:      sql.NullTime{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := &BalancePostgreSQLModel{}
			model.FromEntity(tt.balance)

			assert.Equal(t, tt.expected.ID, model.ID)
			assert.Equal(t, tt.expected.OrganizationID, model.OrganizationID)
			assert.Equal(t, tt.expected.LedgerID, model.LedgerID)
			assert.Equal(t, tt.expected.AccountID, model.AccountID)
			assert.Equal(t, tt.expected.Alias, model.Alias)
			assert.Equal(t, tt.expected.Key, model.Key)
			assert.Equal(t, tt.expected.AssetCode, model.AssetCode)
			assert.True(t, tt.expected.Available.Equal(model.Available), "Available mismatch: expected %s, got %s", tt.expected.Available, model.Available)
			assert.True(t, tt.expected.OnHold.Equal(model.OnHold), "OnHold mismatch: expected %s, got %s", tt.expected.OnHold, model.OnHold)
			assert.Equal(t, tt.expected.Version, model.Version)
			assert.Equal(t, tt.expected.AccountType, model.AccountType)
			assert.Equal(t, tt.expected.AllowSending, model.AllowSending)
			assert.Equal(t, tt.expected.AllowReceiving, model.AllowReceiving)
			assert.Equal(t, tt.expected.CreatedAt, model.CreatedAt)
			assert.Equal(t, tt.expected.UpdatedAt, model.UpdatedAt)
			assert.Equal(t, tt.expected.DeletedAt, model.DeletedAt)
		})
	}
}

// TestBalancePostgreSQLModel_FromEntity_NilKey tests the specific branch where Key is nil/empty
func TestBalancePostgreSQLModel_FromEntity_NilKey(t *testing.T) {
	t.Parallel()

	// Test with empty string key
	balance := &mmodel.Balance{
		ID:             "00000000-0000-0000-0000-000000000001",
		OrganizationID: "00000000-0000-0000-0000-000000000002",
		LedgerID:       "00000000-0000-0000-0000-000000000003",
		AccountID:      "00000000-0000-0000-0000-000000000004",
		Alias:          "@test",
		Key:            "",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	model := &BalancePostgreSQLModel{}
	model.FromEntity(balance)

	assert.Equal(t, "default", model.Key, "Key should be set to 'default' when empty")
}

// TestBalancePostgreSQLModel_RoundTrip tests that ToEntity and FromEntity are inverses
func TestBalancePostgreSQLModel_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Microsecond)

	original := &mmodel.Balance{
		ID:             "00000000-0000-0000-0000-000000000001",
		OrganizationID: "00000000-0000-0000-0000-000000000002",
		LedgerID:       "00000000-0000-0000-0000-000000000003",
		AccountID:      "00000000-0000-0000-0000-000000000004",
		Alias:          "@roundtrip",
		Key:            "custom-key",
		AssetCode:      "EUR",
		Available:      decimal.NewFromInt(5000),
		OnHold:         decimal.NewFromInt(250),
		Version:        10,
		AccountType:    "savings",
		AllowSending:   false,
		AllowReceiving: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Convert to model
	model := &BalancePostgreSQLModel{}
	model.FromEntity(original)

	// Convert back to entity
	result := model.ToEntity()

	// Verify all fields match
	assert.Equal(t, original.ID, result.ID)
	assert.Equal(t, original.OrganizationID, result.OrganizationID)
	assert.Equal(t, original.LedgerID, result.LedgerID)
	assert.Equal(t, original.AccountID, result.AccountID)
	assert.Equal(t, original.Alias, result.Alias)
	assert.Equal(t, original.Key, result.Key)
	assert.Equal(t, original.AssetCode, result.AssetCode)
	assert.True(t, original.Available.Equal(result.Available))
	assert.True(t, original.OnHold.Equal(result.OnHold))
	assert.Equal(t, original.Version, result.Version)
	assert.Equal(t, original.AccountType, result.AccountType)
	assert.Equal(t, original.AllowSending, result.AllowSending)
	assert.Equal(t, original.AllowReceiving, result.AllowReceiving)
	assert.Equal(t, original.CreatedAt, result.CreatedAt)
	assert.Equal(t, original.UpdatedAt, result.UpdatedAt)
}

// timePtr is a helper to get a pointer to a time.Time
func timePtr(t time.Time) *time.Time {
	return &t
}
