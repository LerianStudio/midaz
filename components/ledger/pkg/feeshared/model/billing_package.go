// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"math"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BillingPackageType constants define the supported billing package types.
const (
	BillingPackageTypeVolume      = "volume"
	BillingPackageTypeMaintenance = "maintenance"
)

// BillingPeriod is a string in "YYYY-MM" format (e.g., "2026-01").
type BillingPeriod = string

// PricingModel constants define the supported pricing model types.
const (
	PricingModelTiered = "tiered"
	PricingModelFixed  = "fixed"
)

// CountMode constants define how transactions are counted for billing purposes.
const (
	CountModePerRoute   = "perRoute"
	CountModePerAccount = "perAccount"
)

// maxAliasesCount is the maximum number of aliases allowed in an AccountTarget.
const maxAliasesCount = 100

// PricingTier defines a quantity range and the unit price that applies within it.
//
// swagger:model PricingTier
//
//	@Description	PricingTier defines a quantity range and the unit price that applies within it.
type PricingTier struct {
	MinQuantity int64           `json:"minQuantity" bson:"min_quantity" example:"0"`
	MaxQuantity *int64          `json:"maxQuantity,omitempty" bson:"max_quantity,omitempty" example:"999"`
	UnitPrice   decimal.Decimal `json:"unitPrice" bson:"unit_price" swaggertype:"string" example:"1.50"`
} //	@name	PricingTier

// DiscountTier defines a quantity threshold above which a discount percentage applies.
//
// swagger:model DiscountTier
//
//	@Description	DiscountTier defines a quantity threshold above which a discount percentage applies.
type DiscountTier struct {
	MinQuantity        int64           `json:"minQuantity" bson:"min_quantity" example:"1000"`
	DiscountPercentage decimal.Decimal `json:"discountPercentage" bson:"discount_percentage" swaggertype:"string" example:"10.00"`
} //	@name	DiscountTier

// AccountTarget identifies which accounts a maintenance billing package targets.
// Exactly one of SegmentID, PortfolioID, or Aliases must be set.
//
// swagger:model AccountTarget
//
//	@Description	AccountTarget identifies which accounts a maintenance billing package targets. Exactly one of segmentId, portfolioId, or aliases must be set.
type AccountTarget struct {
	SegmentID   *uuid.UUID `json:"segmentId,omitempty" bson:"segment_id,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	PortfolioID *uuid.UUID `json:"portfolioId,omitempty" bson:"portfolio_id,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Aliases     []string   `json:"aliases,omitempty" bson:"aliases,omitempty" example:"account_alpha,account_beta"`
} //	@name	AccountTarget

// Validate ensures exactly one targeting field is set, aliases do not exceed the allowed maximum,
// and no alias contains an empty or whitespace-only string.
func (a *AccountTarget) Validate() error {
	fieldsSet := 0

	if a.SegmentID != nil {
		if *a.SegmentID == uuid.Nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidAccountTarget, "",
				"segmentId must not be a nil UUID")
		}

		fieldsSet++
	}

	if a.PortfolioID != nil {
		if *a.PortfolioID == uuid.Nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidAccountTarget, "",
				"portfolioId must not be a nil UUID")
		}

		fieldsSet++
	}

	if len(a.Aliases) > 0 {
		fieldsSet++
	}

	if fieldsSet != 1 {
		return pkg.ValidateBusinessError(constant.ErrInvalidAccountTarget, "")
	}

	if len(a.Aliases) > maxAliasesCount {
		return pkg.ValidateBusinessError(constant.ErrInvalidAccountTarget, "")
	}

	for _, alias := range a.Aliases {
		if strings.TrimSpace(alias) == "" {
			return pkg.ValidateBusinessError(constant.ErrInvalidAccountTarget, "")
		}
	}

	return nil
}

// EventFilter identifies the transaction route and status used to match billing events.
//
// swagger:model EventFilter
//
//	@Description	EventFilter identifies the transaction route and status used to match billing events.
type EventFilter struct {
	TransactionRoute string `json:"transactionRoute" bson:"transaction_route" example:"payment_route"`
	Status           string `json:"status" bson:"status" example:"APPROVED" enums:"CREATED,APPROVED,PENDING,CANCELED,NOTED"`
} //	@name	EventFilter

// Validate checks that EventFilter has a non-blank route and a non-blank status.
func (ef *EventFilter) Validate() error {
	if strings.TrimSpace(ef.TransactionRoute) == "" {
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "BillingPackage", "eventFilter.transactionRoute is required")
	}

	if strings.TrimSpace(ef.Status) == "" {
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "BillingPackage", "eventFilter.status is required")
	}

	return nil
}

// BillingPackage is the main domain model for the billing package CRUD feature.
//
// swagger:model BillingPackage
//
//	@Description	BillingPackage is the full representation of a billing package, covering both volume and maintenance types.
type BillingPackage struct {
	ID             string  `json:"id" bson:"_id" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string  `json:"organizationId" bson:"organization_id" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID       string  `json:"ledgerId" bson:"ledger_id" example:"00000000-0000-0000-0000-000000000000"`
	Label          string  `json:"label" bson:"label" example:"Monthly Volume Billing"`
	Description    *string `json:"description,omitempty" bson:"description,omitempty" example:"Charges per completed transaction route"`
	Type           string  `json:"type" bson:"type" example:"volume" enums:"volume,maintenance"`
	Enable         *bool   `json:"enable" bson:"enable" example:"true"`

	// Volume-specific fields.
	EventFilter        *EventFilter   `json:"eventFilter,omitempty" bson:"event_filter,omitempty"`
	PricingModel       *string        `json:"pricingModel,omitempty" bson:"pricing_model,omitempty" example:"tiered" enums:"tiered,fixed"`
	Tiers              []PricingTier  `json:"tiers,omitempty" bson:"tiers,omitempty"`
	FreeQuota          *int           `json:"freeQuota,omitempty" bson:"free_quota,omitempty" example:"100"`
	DiscountTiers      []DiscountTier `json:"discountTiers,omitempty" bson:"discount_tiers,omitempty"`
	CountMode          *string        `json:"countMode,omitempty" bson:"count_mode,omitempty" example:"perRoute" enums:"perRoute,perAccount"`
	AssetCode          *string        `json:"assetCode,omitempty" bson:"asset_code,omitempty" example:"BRL"`
	DebitAccountAlias  *string        `json:"debitAccountAlias,omitempty" bson:"debit_account_alias,omitempty" example:"account_fees_debit"`
	CreditAccountAlias *string        `json:"creditAccountAlias,omitempty" bson:"credit_account_alias,omitempty" example:"account_fees_credit"`

	// Maintenance-specific fields.
	FeeAmount                *decimal.Decimal `json:"feeAmount,omitempty" bson:"fee_amount,omitempty" swaggertype:"string" example:"50.00"`
	MaintenanceCreditAccount *string          `json:"maintenanceCreditAccount,omitempty" bson:"maintenance_credit_account,omitempty" example:"account_maintenance_credit"`
	AccountTarget            *AccountTarget   `json:"accountTarget,omitempty" bson:"account_target,omitempty"`

	// Timestamps.
	CreatedAt string  `json:"createdAt" bson:"created_at" example:"2026-01-01T00:00:00Z"`
	UpdatedAt string  `json:"updatedAt" bson:"updated_at" example:"2026-01-01T00:00:00Z"`
	DeletedAt *string `json:"deletedAt,omitempty" bson:"deleted_at,omitempty" example:"2026-06-01T00:00:00Z"`
} //	@name	BillingPackage

// Validate checks the BillingPackage fields for consistency and correctness.
func (bp *BillingPackage) Validate() error {
	if err := bp.validateCommonFields(); err != nil {
		return err
	}

	if err := bp.validateType(); err != nil {
		return err
	}

	switch bp.Type {
	case BillingPackageTypeVolume:
		return bp.validateVolumeFields()
	case BillingPackageTypeMaintenance:
		return bp.validateMaintenanceFields()
	}

	return nil
}

// validateCommonFields checks that required top-level fields are present and non-blank.
func (bp *BillingPackage) validateCommonFields() error {
	if strings.TrimSpace(bp.Label) == "" {
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "BillingPackage", "label is required")
	}

	if strings.TrimSpace(bp.LedgerID) == "" {
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "BillingPackage", "ledgerId is required")
	}

	return nil
}

// validateType checks that the billing package type is one of the allowed values.
func (bp *BillingPackage) validateType() error {
	if bp.Type != BillingPackageTypeVolume && bp.Type != BillingPackageTypeMaintenance {
		return pkg.ValidateBusinessError(constant.ErrInvalidBillingPackageType, "")
	}

	return nil
}

// isNonBlankString returns true when the pointer is non-nil and contains a non-whitespace value.
func isNonBlankString(s *string) bool {
	return s != nil && strings.TrimSpace(*s) != ""
}

// validateVolumeFields checks that all required fields for a volume package are present and valid.
func (bp *BillingPackage) validateVolumeFields() error {
	// Reject maintenance-specific fields on volume packages.
	if bp.FeeAmount != nil || bp.MaintenanceCreditAccount != nil || bp.AccountTarget != nil {
		return pkg.ValidationError{
			EntityType: "BillingPackage",
			Code:       constant.ErrUnexpectedFieldsInTheRequest.Error(),
			Title:      "Unexpected Fields in the Request",
			Message:    "The request body contains fields that are not allowed for this type of billing package. feeAmount, maintenanceCreditAccount, and accountTarget are not allowed for volume packages",
		}
	}

	if bp.EventFilter == nil ||
		bp.PricingModel == nil ||
		len(bp.Tiers) == 0 ||
		!isNonBlankString(bp.AssetCode) ||
		!isNonBlankString(bp.DebitAccountAlias) ||
		!isNonBlankString(bp.CreditAccountAlias) {
		return pkg.ValidateBusinessError(constant.ErrMissingVolumeFields, "")
	}

	if err := bp.EventFilter.Validate(); err != nil {
		return err
	}

	if err := bp.validatePricingModel(); err != nil {
		return err
	}

	if err := bp.validatePricingTiers(); err != nil {
		return err
	}

	if err := bp.validateFreeQuota(); err != nil {
		return err
	}

	if err := bp.validateDiscountTiers(); err != nil {
		return err
	}

	if err := bp.validateCountMode(); err != nil {
		return err
	}

	return nil
}

// validateMaintenanceFields checks that all required fields for a maintenance package are present and valid.
func (bp *BillingPackage) validateMaintenanceFields() error {
	// Reject volume-specific fields on maintenance packages.
	if bp.PricingModel != nil || len(bp.Tiers) > 0 || bp.EventFilter != nil ||
		bp.FreeQuota != nil || len(bp.DiscountTiers) > 0 || bp.CountMode != nil ||
		bp.DebitAccountAlias != nil || bp.CreditAccountAlias != nil {
		return pkg.ValidationError{
			EntityType: "BillingPackage",
			Code:       constant.ErrUnexpectedFieldsInTheRequest.Error(),
			Title:      "Unexpected Fields in the Request",
			Message:    "The request body contains fields that are not allowed for this type of billing package. pricingModel, tiers, eventFilter, freeQuota, discountTiers, countMode, debitAccountAlias, and creditAccountAlias are not allowed for maintenance packages",
		}
	}

	if bp.FeeAmount == nil ||
		!isNonBlankString(bp.AssetCode) ||
		!isNonBlankString(bp.MaintenanceCreditAccount) ||
		bp.AccountTarget == nil {
		return pkg.ValidateBusinessError(constant.ErrMissingMaintenanceFields, "")
	}

	if !bp.FeeAmount.IsPositive() {
		return pkg.ValidateBusinessError(constant.ErrInvalidFeeAmount, "BillingPackage", "feeAmount must be positive")
	}

	return bp.AccountTarget.Validate()
}

// validatePricingModel checks that the pricing model value is one of the allowed values.
func (bp *BillingPackage) validatePricingModel() error {
	if *bp.PricingModel != PricingModelTiered && *bp.PricingModel != PricingModelFixed {
		return pkg.ValidateBusinessError(constant.ErrInvalidPricingModel, "")
	}

	return nil
}

// maxPricingTiers is the maximum number of pricing tiers allowed per billing package.
const maxPricingTiers = 50

// validatePricingTiers checks each tier for valid values and ensures no ranges overlap.
func (bp *BillingPackage) validatePricingTiers() error {
	if len(bp.Tiers) > maxPricingTiers {
		return pkg.ValidateBusinessError(constant.ErrInvalidPricingTier, "BillingPackage",
			fmt.Sprintf("too many pricing tiers: %d (max %d)", len(bp.Tiers), maxPricingTiers))
	}

	for _, tier := range bp.Tiers {
		if err := validateSinglePricingTier(tier); err != nil {
			return err
		}
	}

	return validateTiersNoOverlap(bp.Tiers)
}

// validateSinglePricingTier checks a single tier's MinQuantity, MaxQuantity, and UnitPrice.
func validateSinglePricingTier(tier PricingTier) error {
	if tier.MinQuantity < 0 {
		return pkg.ValidateBusinessError(constant.ErrInvalidPricingTier, "BillingPackage", "minQuantity must be >= 0")
	}

	if tier.MaxQuantity != nil && *tier.MaxQuantity <= tier.MinQuantity {
		return pkg.ValidateBusinessError(constant.ErrInvalidPricingTier, "BillingPackage",
			fmt.Sprintf("maxQuantity (%d) must be greater than minQuantity (%d)", *tier.MaxQuantity, tier.MinQuantity))
	}

	if !tier.UnitPrice.IsPositive() {
		return pkg.ValidateBusinessError(constant.ErrInvalidPricingTier, "BillingPackage", "unitPrice must be positive")
	}

	return nil
}

// validateTiersNoOverlap ensures that no two tiers have overlapping quantity ranges
// and that tiers form a contiguous coverage with no gaps between them.
func validateTiersNoOverlap(tiers []PricingTier) error {
	for i := 0; i < len(tiers); i++ {
		for j := i + 1; j < len(tiers); j++ {
			if tiersOverlap(tiers[i], tiers[j]) {
				return pkg.ValidateBusinessError(constant.ErrInvalidPricingTier, "BillingPackage",
					fmt.Sprintf("tiers %d and %d have overlapping quantity ranges", i+1, j+1))
			}
		}
	}

	return validateTiersNoGap(tiers)
}

// validateTiersNoGap ensures that sorted tiers are contiguous — i.e. each tier's
// minQuantity is exactly the previous tier's maxQuantity + 1, leaving no uncovered
// range where a transaction count would match no tier and cause a runtime error.
func validateTiersNoGap(tiers []PricingTier) error {
	if len(tiers) <= 1 {
		return nil
	}

	// Sort a local copy by MinQuantity to detect gaps regardless of input order.
	sorted := make([]PricingTier, len(tiers))
	copy(sorted, tiers)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].MinQuantity < sorted[i].MinQuantity {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	for i := 0; i < len(sorted)-1; i++ {
		current := sorted[i]

		// A tier without MaxQuantity is unbounded — it covers everything above
		// MinQuantity, so no gap is possible after it.
		if current.MaxQuantity == nil {
			return nil
		}

		next := sorted[i+1]

		// Contiguous means: next.MinQuantity == current.MaxQuantity + 1.
		if next.MinQuantity != *current.MaxQuantity+1 {
			return pkg.ValidateBusinessError(constant.ErrInvalidPricingTier, "BillingPackage",
				fmt.Sprintf("tiers have a gap: range %d-%d is not covered by any tier",
					*current.MaxQuantity+1, next.MinQuantity-1))
		}
	}

	// The last tier must be unbounded (MaxQuantity == nil) to ensure all quantities
	// above its MinQuantity are covered. A bounded last tier leaves a range uncovered
	// that would cause a runtime error during billing calculation.
	last := sorted[len(sorted)-1]
	if last.MaxQuantity != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidPricingTier, "BillingPackage",
			fmt.Sprintf("last tier must be unbounded (no maxQuantity): quantities above %d are not covered",
				*last.MaxQuantity))
	}

	return nil
}

// tiersOverlap returns true when two PricingTier ranges intersect.
func tiersOverlap(a, b PricingTier) bool {
	aMax := a.MaxQuantity
	bMax := b.MaxQuantity

	// Determine effective max for tier a: nil means unbounded (max int64).
	var aEffMax int64
	if aMax == nil {
		aEffMax = math.MaxInt64
	} else {
		aEffMax = *aMax
	}

	// Determine effective max for tier b: nil means unbounded (max int64).
	var bEffMax int64
	if bMax == nil {
		bEffMax = math.MaxInt64
	} else {
		bEffMax = *bMax
	}

	return a.MinQuantity <= bEffMax && b.MinQuantity <= aEffMax
}

// validateFreeQuota checks that freeQuota, when set, is not negative.
func (bp *BillingPackage) validateFreeQuota() error {
	if bp.FreeQuota != nil && *bp.FreeQuota < 0 {
		return pkg.ValidateBusinessError(constant.ErrInvalidFreeQuota, "")
	}

	return nil
}

// validateDiscountTiers checks that each discount tier's percentage is in the [0, 100] range.
func (bp *BillingPackage) validateDiscountTiers() error {
	zero := decimal.Zero
	oneHundred := decimal.NewFromInt(100)

	for _, dt := range bp.DiscountTiers {
		if dt.MinQuantity < 0 {
			return pkg.ValidateBusinessError(constant.ErrInvalidDiscountTier, "BillingPackage", "discount tier minQuantity must be >= 0")
		}

		if dt.DiscountPercentage.LessThan(zero) || dt.DiscountPercentage.GreaterThan(oneHundred) {
			return pkg.ValidateBusinessError(constant.ErrInvalidDiscountTier, "BillingPackage")
		}
	}

	return nil
}

// validateCountMode checks that the countMode, when set, is one of the allowed values.
func (bp *BillingPackage) validateCountMode() error {
	if bp.CountMode == nil {
		return nil
	}

	if *bp.CountMode != CountModePerRoute && *bp.CountMode != CountModePerAccount {
		return pkg.ValidateBusinessError(constant.ErrInvalidCountMode, "")
	}

	return nil
}

// BillingPackageUpdate is the DTO for partial updates to a BillingPackage.
// Pointer fields allow distinguishing between "not provided" (nil) and "set to zero value".
//
// swagger:model BillingPackageUpdate
//
//	@Description	BillingPackageUpdate is the request payload for partial updates to a billing package. Only provided fields are applied.
type BillingPackageUpdate struct {
	Label       *string `json:"label,omitempty" example:"Updated Billing Label"`
	Description *string `json:"description,omitempty" example:"Updated description for the billing package"`
	Enable      *bool   `json:"enable,omitempty" example:"false"`
} //	@name	BillingPackageUpdate

// Validate checks that provided update fields contain valid values.
// Label, when provided, must not be empty or whitespace-only.
func (u *BillingPackageUpdate) Validate() error {
	if u.Label != nil && strings.TrimSpace(*u.Label) == "" {
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "BillingPackage", "label must not be empty")
	}

	return nil
}

// ToMap converts the BillingPackageUpdate to a map containing only the fields that were provided.
func (u *BillingPackageUpdate) ToMap() map[string]any {
	updates := make(map[string]any)

	if u.Label != nil {
		updates["label"] = *u.Label
	}

	if u.Description != nil {
		updates["description"] = *u.Description
	}

	if u.Enable != nil {
		updates["enable"] = *u.Enable
	}

	return updates
}
