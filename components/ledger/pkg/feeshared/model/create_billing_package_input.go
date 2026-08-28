// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"github.com/shopspring/decimal"
)

// CreateBillingPackageInput is the dedicated request DTO for creating a billing
// package. It mirrors BillingPackage's client-supplied fields exactly, with a single
// difference: it carries no ledgerId. The ledger a create acts within is named by the
// path (ledger_id) and stamped by the handler, so a body carrying ledgerId is rejected
// as an unknown field. Every other field BillingPackage exposes is kept — including the
// server-managed id/organizationId/timestamps — so a body carrying them stays tolerated
// as before (the create path overwrites/sets them); only ledgerId leaves the wire.
type CreateBillingPackageInput struct {
	ID             string  `json:"id,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string  `json:"organizationId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Label          string  `json:"label" example:"Monthly Volume Billing"`
	Description    *string `json:"description,omitempty" example:"Charges per completed transaction route"`
	Type           string  `json:"type" example:"volume" enums:"volume,maintenance"`
	Enable         *bool   `json:"enable" example:"true"`

	// Volume-specific fields.
	EventFilter        *EventFilter   `json:"eventFilter,omitempty"`
	PricingModel       *string        `json:"pricingModel,omitempty" example:"tiered" enums:"tiered,fixed"`
	Tiers              []PricingTier  `json:"tiers,omitempty"`
	FreeQuota          *int           `json:"freeQuota,omitempty" example:"100"`
	DiscountTiers      []DiscountTier `json:"discountTiers,omitempty"`
	CountMode          *string        `json:"countMode,omitempty" example:"perRoute" enums:"perRoute,perAccount"`
	AssetCode          *string        `json:"assetCode,omitempty" example:"BRL"`
	DebitAccountAlias  *string        `json:"debitAccountAlias,omitempty" example:"account_fees_debit"`
	CreditAccountAlias *string        `json:"creditAccountAlias,omitempty" example:"account_fees_credit"`

	// Maintenance-specific fields.
	FeeAmount                *decimal.Decimal `json:"feeAmount,omitempty" swaggertype:"string" example:"50.00"`
	MaintenanceCreditAccount *string          `json:"maintenanceCreditAccount,omitempty" example:"account_maintenance_credit"`
	AccountTarget            *AccountTarget   `json:"accountTarget,omitempty"`

	// Timestamps.
	CreatedAt string  `json:"createdAt,omitempty" example:"2026-01-01T00:00:00Z"`
	UpdatedAt string  `json:"updatedAt,omitempty" example:"2026-01-01T00:00:00Z"`
	DeletedAt *string `json:"deletedAt,omitempty" example:"2026-06-01T00:00:00Z"`
}

// ToBillingPackage builds the domain BillingPackage from the request DTO, copying every
// client-supplied field verbatim. LedgerID is left blank: the create core stamps it from
// the path, which is the sole authority on the ledger. OrganizationID is copied as sent
// but is likewise overwritten from the path by the create core.
func (in *CreateBillingPackageInput) ToBillingPackage() *BillingPackage {
	return &BillingPackage{
		ID:                       in.ID,
		OrganizationID:           in.OrganizationID,
		Label:                    in.Label,
		Description:              in.Description,
		Type:                     in.Type,
		Enable:                   in.Enable,
		EventFilter:              in.EventFilter,
		PricingModel:             in.PricingModel,
		Tiers:                    in.Tiers,
		FreeQuota:                in.FreeQuota,
		DiscountTiers:            in.DiscountTiers,
		CountMode:                in.CountMode,
		AssetCode:                in.AssetCode,
		DebitAccountAlias:        in.DebitAccountAlias,
		CreditAccountAlias:       in.CreditAccountAlias,
		FeeAmount:                in.FeeAmount,
		MaintenanceCreditAccount: in.MaintenanceCreditAccount,
		AccountTarget:            in.AccountTarget,
		CreatedAt:                in.CreatedAt,
		UpdatedAt:                in.UpdatedAt,
		DeletedAt:                in.DeletedAt,
	}
}
