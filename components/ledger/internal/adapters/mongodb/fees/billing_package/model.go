// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"fmt"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// EventFilterModel represents the MongoDB model for an event filter.
type EventFilterModel struct {
	TransactionRoute string `bson:"transaction_route"`
	Status           string `bson:"status"`
}

// PricingTierModel represents the MongoDB model for a pricing tier.
type PricingTierModel struct {
	MinQuantity int64  `bson:"min_quantity"`
	MaxQuantity *int64 `bson:"max_quantity,omitempty"`
	UnitPrice   string `bson:"unit_price"`
}

// DiscountTierModel represents the MongoDB model for a discount tier.
type DiscountTierModel struct {
	MinQuantity        int64  `bson:"min_quantity"`
	DiscountPercentage string `bson:"discount_percentage"`
}

// AccountTargetModel represents the MongoDB model for an account target.
type AccountTargetModel struct {
	SegmentID   *string  `bson:"segment_id,omitempty"`
	PortfolioID *string  `bson:"portfolio_id,omitempty"`
	Aliases     []string `bson:"aliases,omitempty"`
}

// BillingPackageMongoDBModel represents the MongoDB document for a billing package.
type BillingPackageMongoDBModel struct {
	ID                       string              `bson:"_id"`
	OrganizationID           string              `bson:"organization_id"`
	LedgerID                 string              `bson:"ledger_id"`
	Label                    string              `bson:"label"`
	Description              *string             `bson:"description,omitempty"`
	Type                     string              `bson:"type"`
	Enable                   bool                `bson:"enable"`
	EventFilter              *EventFilterModel   `bson:"event_filter,omitempty"`
	PricingModel             *string             `bson:"pricing_model,omitempty"`
	Tiers                    []PricingTierModel  `bson:"tiers,omitempty"`
	FreeQuota                *int                `bson:"free_quota,omitempty"`
	DiscountTiers            []DiscountTierModel `bson:"discount_tiers,omitempty"`
	CountMode                *string             `bson:"count_mode,omitempty"`
	AssetCode                *string             `bson:"asset_code,omitempty"`
	DebitAccountAlias        *string             `bson:"debit_account_alias,omitempty"`
	CreditAccountAlias       *string             `bson:"credit_account_alias,omitempty"`
	FeeAmount                *string             `bson:"fee_amount,omitempty"`
	MaintenanceCreditAccount *string             `bson:"maintenance_credit_account,omitempty"`
	AccountTarget            *AccountTargetModel `bson:"account_target,omitempty"`
	CreatedAt                string              `bson:"created_at"`
	UpdatedAt                string              `bson:"updated_at"`
	DeletedAt                *string             `bson:"deleted_at,omitempty"`
}

// ToEntity converts BillingPackageMongoDBModel to model.BillingPackage.
// Returns an error if any stored decimal or UUID string cannot be parsed,
// which would indicate data corruption in the database.
func (m *BillingPackageMongoDBModel) ToEntity() (*model.BillingPackage, error) {
	bp := &model.BillingPackage{
		ID:                       m.ID,
		OrganizationID:           m.OrganizationID,
		LedgerID:                 m.LedgerID,
		Label:                    m.Label,
		Description:              m.Description,
		Type:                     m.Type,
		Enable:                   func() *bool { v := m.Enable; return &v }(),
		PricingModel:             m.PricingModel,
		FreeQuota:                m.FreeQuota,
		CountMode:                m.CountMode,
		AssetCode:                m.AssetCode,
		DebitAccountAlias:        m.DebitAccountAlias,
		CreditAccountAlias:       m.CreditAccountAlias,
		MaintenanceCreditAccount: m.MaintenanceCreditAccount,
		CreatedAt:                m.CreatedAt,
		UpdatedAt:                m.UpdatedAt,
		DeletedAt:                m.DeletedAt,
	}

	if m.EventFilter != nil {
		bp.EventFilter = &model.EventFilter{
			TransactionRoute: m.EventFilter.TransactionRoute,
			Status:           m.EventFilter.Status,
		}
	}

	if len(m.Tiers) > 0 {
		bp.Tiers = make([]model.PricingTier, 0, len(m.Tiers))

		for i, tier := range m.Tiers {
			unitPrice, err := decimal.NewFromString(tier.UnitPrice)
			if err != nil {
				return nil, fmt.Errorf("billing_package %s: invalid tiers[%d].unit_price %q: %w", m.ID, i, tier.UnitPrice, err)
			}

			bp.Tiers = append(bp.Tiers, model.PricingTier{
				MinQuantity: tier.MinQuantity,
				MaxQuantity: tier.MaxQuantity,
				UnitPrice:   unitPrice,
			})
		}
	}

	if len(m.DiscountTiers) > 0 {
		bp.DiscountTiers = make([]model.DiscountTier, 0, len(m.DiscountTiers))

		for i, dt := range m.DiscountTiers {
			pct, err := decimal.NewFromString(dt.DiscountPercentage)
			if err != nil {
				return nil, fmt.Errorf("billing_package %s: invalid discount_tiers[%d].discount_percentage %q: %w", m.ID, i, dt.DiscountPercentage, err)
			}

			bp.DiscountTiers = append(bp.DiscountTiers, model.DiscountTier{
				MinQuantity:        dt.MinQuantity,
				DiscountPercentage: pct,
			})
		}
	}

	if m.FeeAmount != nil {
		amt, err := decimal.NewFromString(*m.FeeAmount)
		if err != nil {
			return nil, fmt.Errorf("billing_package %s: invalid fee_amount %q: %w", m.ID, *m.FeeAmount, err)
		}

		bp.FeeAmount = &amt
	}

	if m.AccountTarget != nil {
		at := &model.AccountTarget{
			Aliases: m.AccountTarget.Aliases,
		}

		if m.AccountTarget.SegmentID != nil {
			segID, err := uuid.Parse(*m.AccountTarget.SegmentID)
			if err != nil {
				return nil, fmt.Errorf("billing_package %s: invalid account_target.segment_id %q: %w", m.ID, *m.AccountTarget.SegmentID, err)
			}

			at.SegmentID = &segID
		}

		if m.AccountTarget.PortfolioID != nil {
			portID, err := uuid.Parse(*m.AccountTarget.PortfolioID)
			if err != nil {
				return nil, fmt.Errorf("billing_package %s: invalid account_target.portfolio_id %q: %w", m.ID, *m.AccountTarget.PortfolioID, err)
			}

			at.PortfolioID = &portID
		}

		bp.AccountTarget = at
	}

	return bp, nil
}

// FromEntity converts model.BillingPackage to BillingPackageMongoDBModel.
func (m *BillingPackageMongoDBModel) FromEntity(bp *model.BillingPackage) {
	m.ID = bp.ID
	m.OrganizationID = bp.OrganizationID
	m.LedgerID = bp.LedgerID
	m.Label = bp.Label
	m.Description = bp.Description

	m.Type = bp.Type

	// Default Enable to true when nil so that direct callers of FromEntity
	// (tests, migrations, etc.) do not silently store false via the zero value.
	if bp.Enable != nil {
		m.Enable = *bp.Enable
	} else {
		m.Enable = true
	}

	m.PricingModel = bp.PricingModel
	m.FreeQuota = bp.FreeQuota
	m.CountMode = bp.CountMode
	m.AssetCode = bp.AssetCode
	m.DebitAccountAlias = bp.DebitAccountAlias
	m.CreditAccountAlias = bp.CreditAccountAlias
	m.MaintenanceCreditAccount = bp.MaintenanceCreditAccount
	m.CreatedAt = bp.CreatedAt
	m.UpdatedAt = bp.UpdatedAt
	m.DeletedAt = bp.DeletedAt

	if bp.EventFilter != nil {
		m.EventFilter = &EventFilterModel{
			TransactionRoute: bp.EventFilter.TransactionRoute,
			Status:           bp.EventFilter.Status,
		}
	}

	if len(bp.Tiers) > 0 {
		m.Tiers = make([]PricingTierModel, 0, len(bp.Tiers))

		for _, tier := range bp.Tiers {
			m.Tiers = append(m.Tiers, PricingTierModel{
				MinQuantity: tier.MinQuantity,
				MaxQuantity: tier.MaxQuantity,
				UnitPrice:   tier.UnitPrice.String(),
			})
		}
	}

	if len(bp.DiscountTiers) > 0 {
		m.DiscountTiers = make([]DiscountTierModel, 0, len(bp.DiscountTiers))

		for _, dt := range bp.DiscountTiers {
			m.DiscountTiers = append(m.DiscountTiers, DiscountTierModel{
				MinQuantity:        dt.MinQuantity,
				DiscountPercentage: dt.DiscountPercentage.String(),
			})
		}
	}

	if bp.FeeAmount != nil {
		s := bp.FeeAmount.String()
		m.FeeAmount = &s
	}

	if bp.AccountTarget != nil {
		at := &AccountTargetModel{
			Aliases: bp.AccountTarget.Aliases,
		}

		if bp.AccountTarget.SegmentID != nil {
			s := bp.AccountTarget.SegmentID.String()
			at.SegmentID = &s
		}

		if bp.AccountTarget.PortfolioID != nil {
			s := bp.AccountTarget.PortfolioID.String()
			at.PortfolioID = &s
		}

		m.AccountTarget = at
	}
}
