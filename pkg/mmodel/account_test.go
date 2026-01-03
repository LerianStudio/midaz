package mmodel

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAccount_ToProto(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			result := tt.account
			t.Log(result)
		})
	}
}

func TestAccount_IDtoUUID(t *testing.T) {
	t.Parallel()
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
			t.Parallel()

			got := tt.account.IDtoUUID()
			assert.Equal(t, tt.want, got)
		})
	}
}
