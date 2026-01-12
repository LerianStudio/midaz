package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// RegulatoryFields contains regulatory-specific fields for an alias.
//
// swagger:model RegulatoryFields
// @Description RegulatoryFields object
type RegulatoryFields struct {
	// Document of the participant (identifies which financial-group entity owns the relationship)
	ParticipantDocument *string `json:"participantDocument,omitempty" example:"12345678912345"`
} // @name RegulatoryFields

// RelatedParty represents a party related to an alias.
//
// swagger:model RelatedParty
// @Description RelatedParty object
type RelatedParty struct {
	// Unique identifier of the related party.
	ID *uuid.UUID `json:"id,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	// Document of the related party.
	Document string `json:"document" validate:"required" example:"12345678900"`
	// Name of the related party.
	Name string `json:"name" validate:"required" example:"John Smith"`
	// Role of the related party (PRIMARY_HOLDER, LEGAL_REPRESENTATIVE, RESPONSIBLE_PARTY).
	Role string `json:"role" validate:"required,oneof=PRIMARY_HOLDER LEGAL_REPRESENTATIVE RESPONSIBLE_PARTY" example:"PRIMARY_HOLDER"`
	// Start date of the relationship. Accepts both "2025-01-01" and "2025-01-01T00:00:00Z" formats.
	StartDate Date `json:"startDate" validate:"required" example:"2025-01-01"`
	// End date of the relationship (optional). Accepts both "2025-01-01" and "2025-01-01T00:00:00Z" formats.
	EndDate *Date `json:"endDate,omitempty" example:"2026-01-01"`
} // @name RelatedParty

// CreateAliasInput is a struct designed to encapsulate request create payload data.
//
// swagger:model CreateAliasInput
// @Description CreateAliasRequest payload
type CreateAliasInput struct {
	// Unique identifier of the ledger of the related account.
	LedgerID string `json:"ledgerId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	// Unique identifier of the related account on ledger.
	AccountID string `json:"accountId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	// An object containing key-value pairs to add as metadata, where the field name is the key and the field value is the value.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	// Object with banking information of the related account.
	BankingDetails *BankingDetails `json:"bankingDetails"`
	// Object with regulatory fields.
	RegulatoryFields *RegulatoryFields `json:"regulatoryFields,omitempty"`
	// List of related parties to add at creation.
	RelatedParties []*RelatedParty `json:"relatedParties,omitempty"`
} // @name CreateAliasRequest

// UpdateAliasInput is a struct designed to encapsulate request update payload data.
//
// swagger:model UpdateAliasInput
// @Description UpdateAliasRequest payload
type UpdateAliasInput struct {
	// An object containing key-value pairs to add as metadata, where the field name is the key and the field value is the value.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// Object with banking information of the related account.
	BankingDetails *BankingDetails `json:"bankingDetails"`
	// Object with regulatory fields.
	RegulatoryFields *RegulatoryFields `json:"regulatoryFields,omitempty"`
	// List of related parties to add (appends to existing).
	RelatedParties []*RelatedParty `json:"relatedParties,omitempty"`
} // @name UpdateAliasRequest

// Alias is a struct designed to store account data.
//
// swagger:model Alias
// @Description AliasResponse payload
type Alias struct {
	ID               *uuid.UUID        `json:"id,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Document         *string           `json:"document,omitempty" example:"91315026015"`
	Type             *string           `json:"type,omitempty" example:"LEGAL_PERSON"`
	LedgerID         *string           `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	AccountID        *string           `json:"accountId" example:"00000000-0000-0000-0000-000000000000"`
	HolderID         *uuid.UUID        `json:"holderId" example:"00000000-0000-0000-0000-000000000000"`
	Metadata         map[string]any    `json:"metadata,omitempty"`
	BankingDetails   *BankingDetails   `json:"bankingDetails,omitempty"`
	RegulatoryFields *RegulatoryFields `json:"regulatoryFields,omitempty"`
	RelatedParties   []*RelatedParty   `json:"relatedParties,omitempty"`
	CreatedAt        time.Time         `json:"createdAt" example:"2025-01-01T00:00:00Z"`
	UpdatedAt        time.Time         `json:"updatedAt" example:"2025-01-01T00:00:00Z"`
	DeletedAt        *time.Time        `json:"deletedAt" example:"2025-01-01T00:00:00Z"`
} // @name AliasResponse

// BankingDetails is a struct designed to store account banking details data.
//
// swagger:model BankingDetails
// @Description BankingDetails object
type BankingDetails struct {
	// The branch number or code.
	Branch *string `json:"branch,omitempty" example:"0001"`
	// The account code or number.
	Account *string `json:"account,omitempty" example:"123450"`
	// Type of account.
	Type *string `json:"type,omitempty" example:"CACC"`
	// The date the account was opened.
	OpeningDate *string `json:"openingDate,omitempty" example:"2025-01-01"`
	// The date the account was closed.
	ClosingDate *Date `json:"closingDate,omitempty" example:"2025-12-31"`
	// The International Bank Account Number.
	IBAN *string `json:"iban,omitempty" example:"US12345678901234567890"`
	// The country code where the bank is located.
	CountryCode *string `json:"countryCode,omitempty" example:"US"`
	// The code or identifier for correlation with the bank holding the account.
	BankID *string `json:"bankId,omitempty" example:"12345"`
} // @name BankingDetails
