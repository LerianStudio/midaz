// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// RegulatoryFields contains regulatory-specific fields for an instrument.
type RegulatoryFields struct {
	// Document of the participant (identifies which financial-group entity owns the relationship).
	// example: 12345678912345
	// maxLength: 100
	ParticipantDocument *string `json:"participantDocument,omitempty" example:"12345678912345" maxLength:"100"`
}

// RelatedParty represents a party related to an instrument.
type RelatedParty struct {
	// Unique system-generated identifier of the related party (UUID format).
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID *uuid.UUID `json:"id,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// National or tax identification document of the related party.
	// required: true
	// example: 12345678900
	// maxLength: 100
	Document string `json:"document" validate:"required" example:"12345678900" maxLength:"100"`

	// Full legal name of the related party.
	// required: true
	// example: John Smith
	// maxLength: 256
	Name string `json:"name" validate:"required" example:"John Smith" maxLength:"256"`

	// Role of the related party in the instrument relationship.
	// required: true
	// example: PRIMARY_HOLDER
	// maxLength: 100
	Role string `json:"role" validate:"required,oneof=PRIMARY_HOLDER LEGAL_REPRESENTATIVE RESPONSIBLE_PARTY" example:"PRIMARY_HOLDER" maxLength:"100"`

	// Start date of the relationship. Accepts both "2025-01-01" and "2025-01-01T00:00:00Z" formats.
	// required: true
	// format: date
	StartDate Date `json:"startDate" validate:"required" swaggertype:"string" format:"date" example:"2025-01-01"`

	// End date of the relationship (optional). Accepts both "2025-01-01" and "2025-01-01T00:00:00Z" formats.
	// required: false
	// format: date
	EndDate *Date `json:"endDate,omitempty" swaggertype:"string" format:"date" example:"2026-01-01"`
}

// CreateInstrumentInput is a struct designed to encapsulate request create payload data.
type CreateInstrumentInput struct {
	// Unique identifier of the ledger that contains the related account (UUID format).
	// required: true
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" validate:"required" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Unique identifier of the ledger account this instrument is linked to (UUID format).
	// required: true
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	AccountID string `json:"accountId" validate:"required" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Custom key-value pairs for extending the instrument information (flat map, max 100-char keys, max 2000-char values).
	// required: false
	// example: {"product": "checking", "region": "us-east"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`

	// Banking details for the account linked by this instrument.
	// required: false
	BankingDetails *BankingDetails `json:"bankingDetails"`

	// Regulatory metadata identifying the participant entity.
	// required: false
	RegulatoryFields *RegulatoryFields `json:"regulatoryFields,omitempty"`

	// List of related parties to associate at instrument creation.
	// required: false
	RelatedParties []*RelatedParty `json:"relatedParties,omitempty"`
}

// UpdateInstrumentInput is a struct designed to encapsulate request update payload data.
type UpdateInstrumentInput struct {
	// Updated custom key-value pairs for extending the instrument information.
	// required: false
	// example: {"product": "savings", "region": "us-west"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Updated banking details for the linked account.
	// required: false
	BankingDetails *BankingDetails `json:"bankingDetails"`

	// Updated regulatory metadata identifying the participant entity.
	// required: false
	RegulatoryFields *RegulatoryFields `json:"regulatoryFields,omitempty"`

	// Additional related parties to append to the instrument (existing entries are not removed).
	// required: false
	RelatedParties []*RelatedParty `json:"relatedParties,omitempty"`
}

// Instrument is a struct designed to store account data.
type Instrument struct {
	// Unique system-generated identifier for the instrument (UUID format).
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID *uuid.UUID `json:"id,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// National or tax identification document of the holder linked to this instrument.
	// example: 91315026015
	// maxLength: 100
	Document *string `json:"document,omitempty" example:"91315026015" maxLength:"100"`

	// Holder type (NATURAL_PERSON or LEGAL_PERSON), derived from the associated holder.
	// example: NATURAL_PERSON
	// maxLength: 100
	Type *string `json:"type,omitempty" example:"NATURAL_PERSON" maxLength:"100"`

	// Unique identifier of the ledger that contains the linked account (UUID format).
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID *string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Unique identifier of the ledger account this instrument is linked to (UUID format).
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	AccountID *string `json:"accountId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Unique identifier of the holder that owns this instrument (UUID format).
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	HolderID *uuid.UUID `json:"holderId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Custom key-value pairs for extending the instrument information.
	// example: {"product": "checking"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// Banking details for the account linked by this instrument.
	BankingDetails *BankingDetails `json:"bankingDetails,omitempty"`

	// Regulatory metadata identifying the participant entity.
	RegulatoryFields *RegulatoryFields `json:"regulatoryFields,omitempty"`

	// List of parties associated with this instrument and their roles.
	RelatedParties []*RelatedParty `json:"relatedParties,omitempty"`

	// Timestamp when the instrument was created (RFC3339 format).
	// example: 2025-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2025-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the instrument was last updated (RFC3339 format).
	// example: 2025-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2025-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the instrument was soft-deleted; null if the instrument is active (RFC3339 format).
	// example: null
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2025-01-01T00:00:00Z" format:"date-time"`
}

// BankingDetails is a struct designed to store account banking details data.
type BankingDetails struct {
	// The branch number or code at the holding bank.
	// example: 0001
	// maxLength: 50
	Branch *string `json:"branch,omitempty" example:"0001" maxLength:"50"`

	// The account code or number at the holding bank.
	// example: 123450
	// maxLength: 50
	Account *string `json:"account,omitempty" example:"123450" maxLength:"50"`

	// ISO 20022 account type code (e.g. CACC for current account, SVGS for savings).
	// example: CACC
	// maxLength: 10
	Type *string `json:"type,omitempty" example:"CACC" maxLength:"10"`

	// The date the account was opened (YYYY-MM-DD format).
	// example: 2025-01-01
	// format: date
	OpeningDate *string `json:"openingDate,omitempty" example:"2025-01-01" format:"date"`

	// The date the account was closed.
	// example: 2025-12-31
	// format: date
	ClosingDate *Date `json:"closingDate,omitempty" example:"2025-12-31" format:"date"`

	// The International Bank Account Number.
	// example: US12345678901234567890
	// maxLength: 34
	IBAN *string `json:"iban,omitempty" example:"US12345678901234567890" maxLength:"34"`

	// The ISO 3166-1 alpha-2 country code where the bank is located.
	// example: US
	// maxLength: 2
	CountryCode *string `json:"countryCode,omitempty" example:"US" maxLength:"2"`

	// The code or identifier for correlation with the bank holding the account.
	// example: 12345
	// maxLength: 50
	BankID *string `json:"bankId,omitempty" example:"12345" maxLength:"50"`
}
