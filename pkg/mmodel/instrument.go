// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// RegulatoryFields contains regulatory-specific fields for an instrument.
//
// swagger:model RegulatoryFields
//
//	@Description	Regulatory metadata for an instrument, carrying the participant document that identifies which financial-group entity owns the regulatory relationship.
type RegulatoryFields struct {
	// Document of the participant (identifies which financial-group entity owns the relationship).
	// example: 12345678912345
	// maxLength: 100
	ParticipantDocument *string `json:"participantDocument,omitempty" example:"12345678912345" maxLength:"100"`
} // @name RegulatoryFields

// RelatedParty represents a party related to an instrument.
//
// swagger:model RelatedParty
//
//	@Description	A party associated with an instrument, defining the role (PRIMARY_HOLDER, LEGAL_REPRESENTATIVE, or RESPONSIBLE_PARTY) and the time range during which the relationship is active.
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
} // @name RelatedParty

// CreateInstrumentInput is a struct designed to encapsulate request create payload data.
//
// swagger:model CreateInstrumentInput
//
//	@Description	Request payload for creating a new instrument that links a holder to a specific ledger account. An instrument captures banking details, regulatory fields, and the related parties authorized on the account.
//
//	@example		{
//	  "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//	  "accountId": "d4e5f6a1-b2c3-7890-defg-4567890123ef",
//	  "bankingDetails": {
//	    "branch": "0001",
//	    "account": "123450",
//	    "type": "CACC",
//	    "openingDate": "2025-01-01",
//	    "countryCode": "US"
//	  },
//	  "regulatoryFields": {
//	    "participantDocument": "12345678912345"
//	  },
//	  "relatedParties": [
//	    {
//	      "document": "12345678900",
//	      "name": "John Smith",
//	      "role": "PRIMARY_HOLDER",
//	      "startDate": "2025-01-01"
//	    }
//	  ],
//	  "metadata": {
//	    "product": "checking",
//	    "region": "us-east"
//	  }
//	}
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
} // @name CreateInstrumentRequest

// UpdateInstrumentInput is a struct designed to encapsulate request update payload data.
//
// swagger:model UpdateInstrumentInput
//
//	@Description	Request payload for updating an existing instrument. All fields are optional — only provided fields are applied. RelatedParties are appended to the existing list; existing entries are not removed.
//
//	@example		{
//	  "bankingDetails": {
//	    "branch": "0002",
//	    "account": "654321",
//	    "type": "SVGS"
//	  },
//	  "regulatoryFields": {
//	    "participantDocument": "98765432198765"
//	  },
//	  "metadata": {
//	    "product": "savings",
//	    "region": "us-west"
//	  }
//	}
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
} // @name UpdateInstrumentRequest

// Instrument is a struct designed to store account data.
//
// swagger:model Instrument
//
//	@Description	Complete instrument entity returned by create, update, and get operations. Captures the link between a holder and a ledger account, together with banking details, regulatory fields, related parties, and audit timestamps.
//
//	@example		{
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "document": "91315026015",
//	  "type": "NATURAL_PERSON",
//	  "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//	  "accountId": "d4e5f6a1-b2c3-7890-defg-4567890123ef",
//	  "holderId": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	  "bankingDetails": {
//	    "branch": "0001",
//	    "account": "123450",
//	    "type": "CACC",
//	    "openingDate": "2025-01-01",
//	    "countryCode": "US"
//	  },
//	  "relatedParties": [
//	    {
//	      "document": "12345678900",
//	      "name": "John Smith",
//	      "role": "PRIMARY_HOLDER",
//	      "startDate": "2025-01-01"
//	    }
//	  ],
//	  "metadata": {
//	    "product": "checking"
//	  },
//	  "createdAt": "2025-01-01T00:00:00Z",
//	  "updatedAt": "2025-01-01T00:00:00Z",
//	  "deletedAt": null
//	}
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
} // @name InstrumentResponse

// BankingDetails is a struct designed to store account banking details data.
//
// swagger:model BankingDetails
//
//	@Description	Banking account details for an instrument, capturing branch, account number, account type, dates, and international identifiers such as IBAN and country code.
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
} // @name BankingDetails
