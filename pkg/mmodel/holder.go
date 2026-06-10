// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// CreateHolderInput is a struct designed to encapsulate request create payload data.
//
// swagger:model CreateHolderInput
//
//	@Description	Request payload for creating a new holder. A holder represents an identified party (individual or legal entity) that can own accounts within the ledger system. The type field controls which person-type sub-object is applicable.
//
//	@example		{
//	  "externalId": "G4K7N8M2",
//	  "type": "NATURAL_PERSON",
//	  "name": "John Doe",
//	  "document": "91315026015",
//	  "addresses": {
//	    "primary": {
//	      "line1": "123 Main St",
//	      "city": "New York",
//	      "state": "NY",
//	      "country": "US",
//	      "zipCode": "10001"
//	    }
//	  },
//	  "contact": {
//	    "primaryEmail": "john.doe@example.com",
//	    "mobilePhone": "+15555550100"
//	  },
//	  "naturalPerson": {
//	    "birthDate": "1990-01-01",
//	    "nationality": "American"
//	  },
//	  "metadata": {
//	    "source": "onboarding",
//	    "region": "us-east"
//	  }
//	}
type CreateHolderInput struct {
	// Optional client-supplied correlation key for idempotency and external system linking.
	// required: false
	// example: G4K7N8M2
	// maxLength: 256
	ExternalID *string `json:"externalId" example:"G4K7N8M2" maxLength:"256"`

	// Classification of the holder: NATURAL_PERSON for individuals, LEGAL_PERSON for companies.
	// required: true
	// example: NATURAL_PERSON
	// maxLength: 100
	Type *string `json:"type" validate:"required" example:"NATURAL_PERSON" enums:"NATURAL_PERSON,LEGAL_PERSON" maxLength:"100"`

	// Full legal name of the holder. For LEGAL_PERSON this must be the registered company name.
	// required: true
	// example: John Doe
	// maxLength: 256
	Name string `json:"name" validate:"required" example:"John Doe" maxLength:"256"`

	// National or tax identification document number of the holder.
	// required: true
	// example: 91315026015
	// maxLength: 100
	Document string `json:"document" validate:"required" example:"91315026015" maxLength:"100"`

	// Physical addresses associated with the holder (primary + up to two additional).
	// required: false
	Addresses *Addresses `json:"addresses"`

	// Contact details (email addresses and phone numbers) for the holder.
	// required: false
	Contact *Contact `json:"contact"`

	// Individual-specific biographical fields; populate only when type is NATURAL_PERSON.
	// required: false
	NaturalPerson *NaturalPerson `json:"naturalPerson"`

	// Company-specific registration fields; populate only when type is LEGAL_PERSON.
	// required: false
	LegalPerson *LegalPerson `json:"legalPerson"`

	// Custom key-value pairs for extending the holder information (flat map, max 100-char keys, max 2000-char values).
	// required: false
	// example: {"source": "onboarding", "region": "us-east"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateHolderRequest

// UpdateHolderInput is a struct designed to encapsulate update request data.
//
// swagger:model UpdateHolderInput
//
//	@Description	Request payload for updating an existing holder. All fields are optional — only provided fields are applied. Omitted fields remain unchanged, enabling partial updates to name, addresses, contact, person details, and metadata.
//
//	@example		{
//	  "name": "Jonathan Doe",
//	  "contact": {
//	    "primaryEmail": "jonathan.doe@example.com",
//	    "mobilePhone": "+15555550199"
//	  },
//	  "metadata": {
//	    "source": "profile-update",
//	    "region": "us-west"
//	  }
//	}
type UpdateHolderInput struct {
	// Updated client-supplied correlation key.
	// required: false
	// example: G4K7N8M
	// maxLength: 256
	ExternalID *string `json:"externalId" example:"G4K7N8M" maxLength:"256"`

	// Updated full legal name of the holder.
	// required: false
	// example: Jonathan Doe
	// maxLength: 256
	Name *string `json:"name" example:"Jonathan Doe" maxLength:"256"`

	// Updated physical addresses for the holder.
	// required: false
	Addresses *Addresses `json:"addresses"`

	// Updated contact details for the holder.
	// required: false
	Contact *Contact `json:"contact"`

	// Updated individual-specific biographical fields.
	// required: false
	NaturalPerson *NaturalPerson `json:"naturalPerson"`

	// Updated company-specific registration fields.
	// required: false
	LegalPerson *LegalPerson `json:"legalPerson"`

	// Updated custom key-value pairs for extending the holder information.
	// required: false
	// example: {"source": "profile-update", "region": "us-west"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateHolderRequest

// Holder is a struct designed to store holder data.
//
// swagger:model Holder
//
//	@Description	Complete holder entity returned by create, update, and get operations. Contains all holder fields including system-generated ID, person-type sub-objects, and audit timestamps.
//
//	@example		{
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "externalId": "G4K7N8M2",
//	  "type": "NATURAL_PERSON",
//	  "name": "John Doe",
//	  "document": "91315026015",
//	  "addresses": {
//	    "primary": {
//	      "line1": "123 Main St",
//	      "city": "New York",
//	      "state": "NY",
//	      "country": "US",
//	      "zipCode": "10001"
//	    }
//	  },
//	  "contact": {
//	    "primaryEmail": "john.doe@example.com",
//	    "mobilePhone": "+15555550100"
//	  },
//	  "naturalPerson": {
//	    "birthDate": "1990-01-01",
//	    "nationality": "American"
//	  },
//	  "metadata": {
//	    "source": "onboarding"
//	  },
//	  "createdAt": "2025-01-01T00:00:00Z",
//	  "updatedAt": "2025-01-01T00:00:00Z",
//	  "deletedAt": null
//	}
type Holder struct {
	// Unique system-generated identifier for the holder (UUID format).
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID *uuid.UUID `json:"id,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Client-supplied external correlation key.
	// example: G4K7N8M2
	// maxLength: 256
	ExternalID *string `json:"externalId,omitempty" example:"G4K7N8M2" maxLength:"256"`

	// Classification of the holder: NATURAL_PERSON or LEGAL_PERSON.
	// example: NATURAL_PERSON
	// maxLength: 100
	Type *string `json:"type,omitempty" example:"NATURAL_PERSON" enums:"NATURAL_PERSON,LEGAL_PERSON" maxLength:"100"`

	// Full legal name of the holder.
	// example: John Doe
	// maxLength: 256
	Name *string `json:"name,omitempty" example:"John Doe" maxLength:"256"`

	// National or tax identification document number of the holder.
	// example: 91315026015
	// maxLength: 100
	Document *string `json:"document,omitempty" example:"91315026015" maxLength:"100"`

	// Physical addresses associated with the holder.
	Addresses *Addresses `json:"addresses,omitempty"`

	// Contact details (email and phone) for the holder.
	Contact *Contact `json:"contact,omitempty"`

	// Individual-specific biographical fields; present when type is NATURAL_PERSON.
	NaturalPerson *NaturalPerson `json:"naturalPerson,omitempty"`

	// Company-specific registration fields; present when type is LEGAL_PERSON.
	LegalPerson *LegalPerson `json:"legalPerson,omitempty"`

	// Custom key-value pairs for extending the holder information.
	// example: {"source": "onboarding"}
	Metadata map[string]any `json:"metadata,omitempty"`

	// Timestamp when the holder was created (RFC3339 format).
	// example: 2025-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2025-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the holder was last updated (RFC3339 format).
	// example: 2025-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2025-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the holder was soft-deleted; null if the holder is active (RFC3339 format).
	// example: null
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2025-01-01T00:00:00Z" format:"date-time"`
} // @name HolderResponse

// Addresses is a struct designed to store addresses data.
//
// swagger:model Addresses
//
//	@Description	Physical address collection for a holder, supporting one primary address and up to two additional addresses.
type Addresses struct {
	// Primary registered address of the holder.
	Primary *Address `json:"primary,omitempty"`

	// First supplementary address (e.g. mailing address).
	Additional1 *Address `json:"additional1,omitempty"`

	// Second supplementary address (e.g. branch or alternate location).
	Additional2 *Address `json:"additional2,omitempty"`
} // @name Addresses

// Contact is a struct designed to store contact data.
//
// swagger:model Contact
//
//	@Description	Communication contact details for a holder, including email addresses and phone numbers.
type Contact struct {
	// The primary email address of the holder.
	// example: john.doe@example.com
	// maxLength: 256
	PrimaryEmail *string `json:"primaryEmail,omitempty" example:"john.doe@example.com" maxLength:"256"`

	// The secondary email address of the holder.
	// example: john.doe@example.com
	// maxLength: 256
	SecondaryEmail *string `json:"secondaryEmail,omitempty" example:"john.doe@example.com" maxLength:"256"`

	// The mobile phone number of the holder, including country code.
	// example: +1555555555
	// maxLength: 32
	MobilePhone *string `json:"mobilePhone,omitempty" example:"+1555555555" maxLength:"32"`

	// Any additional phone number of the holder.
	// example: +1555555555
	// maxLength: 32
	OtherPhone *string `json:"otherPhone,omitempty" example:"+1555555555" maxLength:"32"`
} // @name Contact

// NaturalPerson is a struct designed to store natural person data.
//
// swagger:model NaturalPerson
//
//	@Description	Individual (natural person) biographical and demographic details, used in both request and response payloads when the holder type is NATURAL_PERSON.
type NaturalPerson struct {
	// The person's nickname or preferred name.
	// example: John
	// maxLength: 256
	FavoriteName *string `json:"favoriteName,omitempty" example:"John" maxLength:"256"`

	// The social name or alternate name used by the person, if applicable.
	// example: John Doe
	// maxLength: 256
	SocialName *string `json:"socialName,omitempty" example:"John Doe" maxLength:"256"`

	// Person's gender.
	// example: Male
	// maxLength: 100
	Gender *string `json:"gender,omitempty" example:"Male" maxLength:"100"`

	// Person's birth date, formatted as YYYY-MM-DD.
	// example: 1990-01-01
	// format: date
	BirthDate *string `json:"birthDate,omitempty" example:"1990-01-01" format:"date"`

	// Person's civil status, for example: "Single", "Married", or "Divorced".
	// example: Single
	// maxLength: 100
	CivilStatus *string `json:"civilStatus,omitempty" example:"Single" maxLength:"100"`

	// The nationality of the person, for example, "Brazilian".
	// example: Brazilian
	// maxLength: 100
	Nationality *string `json:"nationality,omitempty" example:"Brazilian" maxLength:"100"`

	// The name of the person's mother.
	// example: Jane Doe
	// maxLength: 256
	MotherName *string `json:"motherName,omitempty" example:"Jane Doe" maxLength:"256"`

	// The name of the person's father.
	// example: John Doe
	// maxLength: 256
	FatherName *string `json:"fatherName,omitempty" example:"John Doe" maxLength:"256"`

	// The current status of the individual.
	// example: Active
	// maxLength: 100
	Status *string `json:"status,omitempty" example:"Active" maxLength:"100"`
} // @name NaturalPerson

// LegalPerson is a struct designed to store legal person data.
//
// swagger:model LegalPerson
//
//	@Description	Legal entity (company) details for an organizational holder, used in both request and response payloads when the holder type is LEGAL_PERSON.
type LegalPerson struct {
	// The registered business name of the company, if applicable.
	// example: Lerian Studio
	// maxLength: 256
	TradeName *string `json:"tradeName,omitempty" example:"Lerian Studio" maxLength:"256"`

	// The type of business or activity the company engages in.
	// example: Electronic devices development
	// maxLength: 256
	Activity *string `json:"activity,omitempty" example:"Electronic devices development" maxLength:"256"`

	// The legal structure of the company.
	// example: Limited Liability
	// maxLength: 100
	Type *string `json:"type,omitempty" example:"Limited Liability" maxLength:"100"`

	// The date when the company was established (YYYY-MM-DD format).
	// example: 2025-01-01
	// format: date
	FoundingDate *string `json:"foundingDate,omitempty" example:"2025-01-01" format:"date"`

	// The size classification of the company (e.g. Small, Medium, Large).
	// example: Medium
	// maxLength: 100
	Size *string `json:"size,omitempty" example:"Medium" maxLength:"100"`

	// The current status of the legal entity.
	// example: Active
	// maxLength: 100
	Status *string `json:"status,omitempty" example:"Active" maxLength:"100"`

	// Details of the company's legal representative.
	Representative *Representative `json:"representative,omitempty"`
} // @name LegalPerson

// Representative is a struct designed to store legal person representative data.
//
// swagger:model Representative
//
//	@Description	Legal representative details for a company-type holder, identifying the individual authorized to act on behalf of the legal entity.
type Representative struct {
	// The legal representative's full name.
	// example: John Doe
	// maxLength: 256
	Name *string `json:"name,omitempty" example:"John Doe" maxLength:"256"`

	// The identification document number of the legal representative.
	// example: 91315026015
	// maxLength: 100
	Document *string `json:"document,omitempty" example:"91315026015" maxLength:"100"`

	// The email address of the legal representative.
	// example: john.doe@example.com
	// maxLength: 256
	Email *string `json:"email,omitempty" example:"john.doe@example.com" maxLength:"256"`

	// The role of the legal representative within the company.
	// example: CFO
	// maxLength: 100
	Role *string `json:"role,omitempty" example:"CFO" maxLength:"100"`
} // @name Representative
