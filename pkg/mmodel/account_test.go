package mmodel

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAccount_ToProto(t *testing.T) {
	tm := time.Now()

	var timeDel *time.Time = &tm

	tests := []struct {
		name    string
		account *Account
	}{
		{
			name: "normal case",
			account: &Account{
				ID:             "1",
				Name:           "Account 1",
				AssetCode:      "USD",
				OrganizationID: "org-123",
				LedgerID:       "ledger-456",
				Status: Status{
					Code:        "1",
					Description: new(string),
				},
				Type:      "type-1",
				UpdatedAt: time.Now(),
				CreatedAt: time.Now(),
			},
		},
		{
			name: "normal case is not nil",
			account: &Account{
				ID:              "1",
				Name:            "Account 1",
				AssetCode:       "USD",
				OrganizationID:  "org-123",
				ParentAccountID: utils.StringPtr("parent"),
				LedgerID:        "ledger-456",
				Status: Status{
					Code:        "1",
					Description: new(string),
				},
				Type:        "type-1",
				UpdatedAt:   time.Now(),
				CreatedAt:   time.Now(),
				DeletedAt:   timeDel,
				EntityID:    utils.StringPtr("EntityID"),
				PortfolioID: utils.StringPtr("PortfolioID"),
				SegmentID:   utils.StringPtr("SegmentID"),
				Alias:       utils.StringPtr("Alias"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account
			t.Log(result)
		})
	}
}

func TestAccount_IDtoUUID(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    uuid.UUID
	}{
		{
			name: "valid UUID",
			account: &Account{
				ID: "123e4567-e89b-12d3-a456-426614174000",
			},
			want: uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		},
		{
			name: "valid UUID with different format",
			account: &Account{
				ID: "123E4567E89B12D3A456426614174000",
			},
			want: uuid.MustParse("123E4567E89B12D3A456426614174000"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.IDtoUUID()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewAccount_ValidInputs(t *testing.T) {
	id := uuid.New().String()
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	account := NewAccount(id, orgID, ledgerID, "USD", "deposit")

	assert.Equal(t, id, account.ID)
	assert.Equal(t, orgID, account.OrganizationID)
	assert.Equal(t, ledgerID, account.LedgerID)
	assert.Equal(t, "USD", account.AssetCode)
	assert.Equal(t, "deposit", account.Type)
	assert.Equal(t, AccountStatusActive, account.Status.Code)
	assert.False(t, account.CreatedAt.IsZero())
	assert.False(t, account.UpdatedAt.IsZero())
}

func TestNewAccount_InvalidID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount("invalid-uuid", uuid.New().String(), uuid.New().String(), "USD", "deposit")
	}, "should panic with invalid ID")
}

func TestNewAccount_InvalidOrganizationID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount(uuid.New().String(), "invalid-uuid", uuid.New().String(), "USD", "deposit")
	}, "should panic with invalid organization ID")
}

func TestNewAccount_InvalidLedgerID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount(uuid.New().String(), uuid.New().String(), "invalid-uuid", "USD", "deposit")
	}, "should panic with invalid ledger ID")
}

func TestNewAccount_EmptyAssetCode_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount(uuid.New().String(), uuid.New().String(), uuid.New().String(), "", "deposit")
	}, "should panic with empty asset code")
}

func TestNewAccount_EmptyAccountType_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount(uuid.New().String(), uuid.New().String(), uuid.New().String(), "USD", "")
	}, "should panic with empty account type")
}
