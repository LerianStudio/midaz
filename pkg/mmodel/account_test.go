package mmodel

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	proto "github.com/LerianStudio/midaz/pkg/mgrpc/account"
)

func TestAccount_ToProto(t *testing.T) {
	tm := time.Now()

	var timeDel *time.Time = &tm

	tests := []struct {
		name     string
		account  *Account
		expected *proto.Account
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
			expected: &proto.Account{
				Id:             "1",
				Name:           "Account 1",
				AssetCode:      "USD",
				OrganizationId: "org-123",
				LedgerId:       "ledger-456",
				Balance: &proto.Balance{
					Available: 100,
					OnHold:    200,
					Scale:     2,
				},
				Status: &proto.Status{
					Code:        "1",
					Description: "Some description",
				},
				AllowSending:   true,
				AllowReceiving: true,
				Type:           "type-1",
			},
		},
		{
			name: "normal case is not nil",
			account: &Account{
				ID:              "1",
				Name:            "Account 1",
				AssetCode:       "USD",
				OrganizationID:  "org-123",
				ParentAccountID: ptr.StringPtr("parent"),
				LedgerID:        "ledger-456",
				Status: Status{
					Code:        "1",
					Description: new(string),
				},
				Type:        "type-1",
				UpdatedAt:   time.Now(),
				CreatedAt:   time.Now(),
				DeletedAt:   timeDel,
				EntityID:    ptr.StringPtr("EntityID"),
				PortfolioID: ptr.StringPtr("PortfolioID"),
				SegmentID:   ptr.StringPtr("SegmentID"),
				Alias:       ptr.StringPtr("Alias"),
			},
			expected: &proto.Account{
				Id:             "1",
				Name:           "Account 1",
				AssetCode:      "USD",
				OrganizationId: "org-123",
				LedgerId:       "ledger-456",
				Balance: &proto.Balance{
					Available: 100,
					OnHold:    200,
					Scale:     2,
				},
				Status: &proto.Status{
					Code:        "1",
					Description: "Some description",
				},
				AllowSending:   true,
				AllowReceiving: true,
				Type:           "type-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.ToProto()
			t.Log(result)
		})
	}
}

func TestBalance_IsEmpty(t *testing.T) {
	type fields struct {
		Available *float64
		OnHold    *float64
		Scale     *float64
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "case 01",
			fields: fields{
				Available: nil,
				OnHold:    nil,
				Scale:     nil,
			},
			want: true,
		},
		{
			name: "case 02",
			fields: fields{
				Available: ptr.Float64Ptr(1),
				OnHold:    nil,
				Scale:     nil,
			},
			want: false,
		},
		{
			name: "case 03",
			fields: fields{
				Available: ptr.Float64Ptr(1),
				OnHold:    ptr.Float64Ptr(2),
				Scale:     nil,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := Balance{
				Available: tt.fields.Available,
				OnHold:    tt.fields.OnHold,
				Scale:     tt.fields.Scale,
			}
			if got := b.IsEmpty(); got != tt.want {
				t.Errorf("Balance.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
