// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"errors"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/bsondecimal"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/google/uuid"
	"github.com/iancoleman/strcase"
	"github.com/shopspring/decimal"
)

var (
	// ErrMissingOrganizationID is returned when OrganizationID is missing.
	ErrMissingOrganizationID = errors.New("organizationID is required")

	// ErrMissingLedgerID is returned when LedgerID is missing.
	ErrMissingLedgerID = errors.New("ledgerID is required")

	// ErrMissingName is returned when FeeGroupLabel (Name) is missing.
	ErrMissingName = errors.New("feeGroupLabel (name) is required")
)

// Calculation represents the calculation details for a fee
type Calculation struct {
	Type  string              `bson:"type"`
	Value bsondecimal.Decimal `bson:"value"`
}

// CalculationModel represents the model used to calculate fees
type CalculationModel struct {
	ApplicationRule string        `bson:"application_rule"`
	Calculations    []Calculation `bson:"calculations"`
}

// Fee represents an individual fee in the fees array
type Fee struct {
	FeeLabel         string           `bson:"fee_label"`
	CalculationModel CalculationModel `bson:"calculation_model"`
	ReferenceAmount  string           `bson:"reference_amount"`
	Priority         int              `bson:"priority"`
	IsDeductibleFrom *bool            `bson:"is_deductible_from"`
	CreditAccount    string           `bson:"credit_account"`
	RouteFrom        *string          `bson:"route_from"`
	RouteTo          *string          `bson:"route_to"`
}

// PackageMongoDBModel represents the MongoDB model for a pack
type PackageMongoDBModel struct {
	ID               uuid.UUID           `bson:"_id"`
	FeeGroupLabel    string              `bson:"fee_group_label"`
	Description      *string             `bson:"description"`
	OrganizationID   uuid.UUID           `bson:"organization_id"`
	SegmentID        *uuid.UUID          `bson:"segment_id"`
	LedgerID         uuid.UUID           `bson:"ledger_id"`
	TransactionRoute *string             `bson:"transaction_route"`
	MinimumAmount    bsondecimal.Decimal `bson:"minimum_amount"`
	MaximumAmount    bsondecimal.Decimal `bson:"maximum_amount"`
	WaivedAccounts   *[]string           `bson:"waived_accounts"`
	Fees             map[string]Fee      `bson:"fees"`
	Enable           *bool               `bson:"enable"`
	CreatedAt        time.Time           `bson:"created_at"`
	UpdatedAt        time.Time           `bson:"updated_at"`
	DeletedAt        *time.Time          `bson:"deleted_at"`
}

// Package represents the entity model for a pack
type Package struct {
	ID               uuid.UUID            `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	FeeGroupLabel    string               `json:"feeGroupLabel" example:"Pacote Padrão"`
	Description      *string              `json:"description" example:"Pacote de taxas administrativas padrão"`
	SegmentID        *uuid.UUID           `json:"segmentId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID         uuid.UUID            `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	TransactionRoute *string              `json:"transactionRoute" example:"debitoted"`
	MinimumAmount    decimal.Decimal      `json:"minimumAmount" example:"100" minimum:"0"`
	MaximumAmount    decimal.Decimal      `json:"maximumAmount" example:"2" minimum:"0"`
	WaivedAccounts   *[]string            `json:"waivedAccounts" example:"acc001,acc002"`
	Fees             map[string]model.Fee `json:"fees"`
	Enable           *bool                `json:"enable"`
	CreatedAt        time.Time            `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt        time.Time            `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt        *time.Time           `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
}

// NewPackage creates a new Package with validation of required fields.
// Use this constructor when creating a new package from user input.
// It validates that OrganizationID, LedgerID, and FeeGroupLabel are provided.
func NewPackage(
	organizationID uuid.UUID,
	ledgerID uuid.UUID,
	feeGroupLabel string,
	minAmount decimal.Decimal,
	maxAmount decimal.Decimal,
	fees map[string]model.Fee,
	enable *bool,
) (*Package, error) {
	if organizationID == uuid.Nil {
		return nil, ErrMissingOrganizationID
	}

	if ledgerID == uuid.Nil {
		return nil, ErrMissingLedgerID
	}

	if feeGroupLabel == "" {
		return nil, ErrMissingName
	}

	now := time.Now()

	id, err := commons.GenerateUUIDv7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}

	return &Package{
		ID:            id,
		FeeGroupLabel: feeGroupLabel,
		LedgerID:      ledgerID,
		MinimumAmount: minAmount,
		MaximumAmount: maxAmount,
		Fees:          fees,
		Enable:        enable,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// ReconstructPackage creates a Package from existing data (e.g., database load).
// No validation is performed since the data was already validated at creation time.
// Use this function when loading from the database or deserializing existing data.
func ReconstructPackage(
	id uuid.UUID,
	feeGroupLabel string,
	description *string,
	segmentID *uuid.UUID,
	ledgerID uuid.UUID,
	transactionRoute *string,
	minAmount decimal.Decimal,
	maxAmount decimal.Decimal,
	waivedAccounts *[]string,
	fees map[string]model.Fee,
	enable *bool,
	createdAt time.Time,
	updatedAt time.Time,
	deletedAt *time.Time,
) *Package {
	return &Package{
		ID:               id,
		FeeGroupLabel:    feeGroupLabel,
		Description:      description,
		SegmentID:        segmentID,
		LedgerID:         ledgerID,
		TransactionRoute: transactionRoute,
		MinimumAmount:    minAmount,
		MaximumAmount:    maxAmount,
		WaivedAccounts:   waivedAccounts,
		Fees:             fees,
		Enable:           enable,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		DeletedAt:        deletedAt,
	}
}

func (p *Package) GetSegmentID() uuid.UUID {
	if p.SegmentID == nil {
		return uuid.Nil
	}

	return *p.SegmentID
}

func (p *Package) GetTransactionRoute() string {
	if p.TransactionRoute == nil {
		return ""
	}

	return *p.TransactionRoute
}

// ToEntity converts PackageMongoDBModel to Package
func (pmm *PackageMongoDBModel) ToEntity() *Package {
	return &Package{
		ID:               pmm.ID,
		FeeGroupLabel:    pmm.FeeGroupLabel,
		Description:      pmm.Description,
		LedgerID:         pmm.LedgerID,
		SegmentID:        pmm.SegmentID,
		TransactionRoute: pmm.TransactionRoute,
		MinimumAmount:    pmm.MinimumAmount.Decimal,
		MaximumAmount:    pmm.MaximumAmount.Decimal,
		WaivedAccounts:   pmm.WaivedAccounts,
		Fees:             ToEntityFeeMap(pmm.Fees),
		CreatedAt:        pmm.CreatedAt,
		UpdatedAt:        pmm.UpdatedAt,
		DeletedAt:        pmm.DeletedAt,
		Enable:           pmm.Enable,
	}
}

// FromEntity converts Package to PackageMongoDBModel
func (pmm *PackageMongoDBModel) FromEntity(p *Package, organizationID uuid.UUID) error {
	fees, err := FromEntityFeeMap(p.Fees)
	if err != nil {
		return fmt.Errorf("failed to convert fees: %w", err)
	}

	pmm.ID = p.ID
	pmm.FeeGroupLabel = p.FeeGroupLabel
	pmm.Description = p.Description
	pmm.TransactionRoute = p.TransactionRoute
	pmm.SegmentID = p.SegmentID
	pmm.OrganizationID = organizationID
	pmm.LedgerID = p.LedgerID
	pmm.MinimumAmount = bsondecimal.Decimal{Decimal: p.MinimumAmount}
	pmm.MaximumAmount = bsondecimal.Decimal{Decimal: p.MaximumAmount}
	pmm.WaivedAccounts = p.WaivedAccounts
	pmm.Fees = fees
	pmm.Enable = p.Enable
	pmm.CreatedAt = p.CreatedAt
	pmm.UpdatedAt = p.UpdatedAt

	return nil
}

// ToEntityFeeMap converts array of Fee to array of FeeModel
func ToEntityFeeMap(fees map[string]Fee) map[string]model.Fee {
	feesModel := make(map[string]model.Fee)

	for key, fee := range fees {
		calcModelDB := &model.CalculationModel{
			ApplicationRule: fee.CalculationModel.ApplicationRule,
			Calculations:    ToEntityCalculationArray(fee.CalculationModel.Calculations),
		}

		feesModel[key] = model.Fee{
			FeeLabel:         fee.FeeLabel,
			CalculationModel: calcModelDB,
			ReferenceAmount:  fee.ReferenceAmount,
			Priority:         fee.Priority,
			IsDeductibleFrom: fee.IsDeductibleFrom,
			CreditAccount:    fee.CreditAccount,
			RouteTo:          fee.RouteTo,
			RouteFrom:        fee.RouteFrom,
		}
	}

	return feesModel
}

// ToEntityCalculationArray converts array of Calculation to array of CalculationModel
func ToEntityCalculationArray(calculations []Calculation) []model.Calculation {
	calculationsModel := make([]model.Calculation, 0, len(calculations))
	for _, calc := range calculations {
		calculationsModel = append(calculationsModel, model.Calculation{
			Type:  calc.Type,
			Value: calc.Value.String(),
		})
	}

	return calculationsModel
}

// FromEntityFeeMap transform an array of Fee to FeeMongoDBModel
func FromEntityFeeMap(fees map[string]model.Fee) (map[string]Fee, error) {
	feesDBModel := make(map[string]Fee)

	for key, fee := range fees {
		if fee.CalculationModel == nil {
			return nil, fmt.Errorf("fee %q: calculationModel is required", key)
		}

		calculations, err := FromEntityCalculationArray(fee.CalculationModel.Calculations)
		if err != nil {
			return nil, fmt.Errorf("fee %q: %w", key, err)
		}

		calcModelDB := CalculationModel{
			ApplicationRule: fee.CalculationModel.ApplicationRule,
			Calculations:    calculations,
		}

		if commons.IsNilOrEmpty(fee.RouteFrom) {
			fee.RouteFrom = nil
		}

		if commons.IsNilOrEmpty(fee.RouteTo) {
			fee.RouteTo = nil
		}

		feesDBModel[strcase.ToLowerCamel(key)] = Fee{
			FeeLabel:         fee.FeeLabel,
			CalculationModel: calcModelDB,
			ReferenceAmount:  fee.ReferenceAmount,
			Priority:         fee.Priority,
			IsDeductibleFrom: fee.IsDeductibleFrom,
			CreditAccount:    fee.CreditAccount,
			RouteTo:          fee.RouteTo,
			RouteFrom:        fee.RouteFrom,
		}
	}

	return feesDBModel, nil
}

// FromEntityCalculationArray transform an array of Calculation to CalculationDB
func FromEntityCalculationArray(calculations []model.Calculation) ([]Calculation, error) {
	calculationsDBModel := make([]Calculation, 0, len(calculations))

	for _, calc := range calculations {
		value, err := decimal.NewFromString(calc.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid decimal value %q for calculation type %q: %w", calc.Value, calc.Type, err)
		}

		calculationsDBModel = append(calculationsDBModel, Calculation{
			Type:  calc.Type,
			Value: bsondecimal.Decimal{Decimal: value},
		})
	}

	return calculationsDBModel, nil
}
