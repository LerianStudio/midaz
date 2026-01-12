package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// CreateHolderInput is a struct designed to encapsulate request create payload data.
//
// swagger:model CreateHolderInput
// @Description CreateHolderRequest payload
type CreateHolderInput struct {
	// Optional field for an external identifier to client correlation purposes.
	ExternalID *string `json:"externalId" example:"G4K7N8M2"`
	// Type of person.
	// * NATURAL_PERSON - Individual
	// * LEGAL_PERSON - Company
	Type *string `json:"type" validate:"required" example:"NATURAL_PERSON" enums:"NATURAL_PERSON,LEGAL_PERSON"`
	// Holders name.
	// **Notes:** If the person type is LEGAL_PERSON, this must be the full legal name. If the person type is NATURAL_PERSON, this should be the individuals full name.
	Name string `json:"name" validate:"required" example:"John Doe"`
	// The holder’s identification document.
	Document string `json:"document" validate:"required,cpfcnpj" example:"91315026015"`
	// Object of addresses.
	Addresses *Addresses `json:"addresses"`
	// Object with contact information.
	Contact *Contact `json:"contact"`
	// Object with natural person information.
	NaturalPerson *NaturalPerson `json:"naturalPerson"`
	// Object with legal person information.
	LegalPerson *LegalPerson `json:"legalPerson"`
	// An object containing key-value pairs to add as metadata, where the field name is the key and the field value is the value.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateHolderRequest

// UpdateHolderInput is a struct designed to encapsulate update request data
//
// swagger:model UpdateHolderInput
// @Description UpdateHolderRequest payload
type UpdateHolderInput struct {
	// Optional field for an external identifier to client correlation purposes.
	ExternalID *string `json:"externalId" example:"G4K7N8M"`
	// Holders name.
	Name *string `json:"name" example:"John Doe"`
	// Object of addresses.
	Addresses *Addresses `json:"addresses"`
	// Object with contact information.
	Contact *Contact `json:"contact"`
	// Object with natural person information.
	NaturalPerson *NaturalPerson `json:"naturalPerson"`
	// Object with legal person information.
	LegalPerson *LegalPerson `json:"legalPerson"`
	// An object containing key-value pairs to add as metadata, where the field name is the key and the field value is the value.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateHolderRequest

// Holder is a struct designed to store holder data.
//
// swagger:model Holder
// @Description HolderResponse payload
type Holder struct {
	ID            *uuid.UUID     `json:"id,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	ExternalID    *string        `json:"externalId,omitempty" example:"G4K7N8M2"`
	Type          *string        `json:"type,omitempty" example:"NATURAL_PERSON" enums:"NATURAL_PERSON,LEGAL_PERSON"`
	Name          *string        `json:"name,omitempty" example:"John Doe"`
	Document      *string        `json:"document,omitempty" example:"91315026015"`
	Addresses     *Addresses     `json:"addresses,omitempty"`
	Contact       *Contact       `json:"contact,omitempty"`
	NaturalPerson *NaturalPerson `json:"naturalPerson,omitempty"`
	LegalPerson   *LegalPerson   `json:"legalPerson,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     time.Time      `json:"createdAt" example:"2025-01-01T00:00:00Z"`
	UpdatedAt     time.Time      `json:"updatedAt" example:"2025-01-01T00:00:00Z"`
	DeletedAt     *time.Time     `json:"deletedAt" example:"2025-01-01T00:00:00Z"`
} // @name HolderResponse

// Addresses is a struct designed to store addresses data.
//
// swagger:model Addresses
// @Description Addresses object
type Addresses struct {
	Primary     *Address `json:"primary,omitempty"`
	Additional1 *Address `json:"additional1,omitempty"`
	Additional2 *Address `json:"additional2,omitempty"`
} // @name Addresses

// Contact is a struct designed to store contact data.
//
// swagger:model Contact
// @Description Contact object
type Contact struct {
	// The primary email address of the holder.
	PrimaryEmail *string `json:"primaryEmail,omitempty" example:"john.doe@example.com"`
	// The secondary email address of the holder.
	SecondaryEmail *string `json:"secondaryEmail,omitempty" example:"john.doe@example.com"`
	// The mobile phone number of the holder, including country code.
	MobilePhone *string `json:"mobilePhone,omitempty" example:"+1555555555"`
	// Any additional phone number of the holder.
	OtherPhone *string `json:"otherPhone,omitempty" example:"+1555555555"`
} // @name Contact

// NaturalPerson is a struct designed to store natural person data.
//
// swagger:model NaturalPerson
// @Description NaturalPerson object
type NaturalPerson struct {
	// The person's nickname or preferred name.
	FavoriteName *string `json:"favoriteName,omitempty" example:"John"`
	// The social name or alternate name used by the person, if applicable.
	SocialName *string `json:"socialName,omitempty" example:"John Doe"`
	// Person's gender.
	Gender *string `json:"gender,omitempty" example:"Male"`
	// Person's birth date, formatted as YYYY-MM-DD.
	BirthDate *string `json:"birthDate,omitempty" example:"1990-01-01"`
	// Person's civil status, for example: "Single", "Married", or "Divorced".
	CivilStatus *string `json:"civilStatus,omitempty" example:"Single"`
	// The nationality of the person, for example, "Brazilian".
	Nationality *string `json:"nationality,omitempty" example:"Brazilian"`
	// The name of the person's mother.
	MotherName *string `json:"motherName,omitempty" example:"Jane Doe"`
	// The name of the person's father.
	FatherName *string `json:"fatherName,omitempty" example:"John Doe"`
	// The current status of the individual.
	Status *string `json:"status,omitempty" example:"Active"`
} // @name NaturalPerson

// LegalPerson is a struct designed to store legal person data.
//
// swagger:model LegalPerson
// @Description LegalPerson is a struct designed to encapsulate response payload data.
type LegalPerson struct {
	// The registered business name of the company, if applicable.
	TradeName *string `json:"tradeName,omitempty" example:"Lerian Studio"`
	// The type of business or activity the company engages in.
	Activity *string `json:"activity,omitempty" example:"Electronic devices development"`
	// The legal structure of the company.
	Type *string `json:"type,omitempty" example:"Limited Liability"`
	// The date when the company was established.
	FoundingDate *string `json:"foundingDate,omitempty" example:"2025-01-01"`
	// The size classification of the company.
	Size *string `json:"size,omitempty" example:"Medium"`
	// The current status of the legal entity.
	Status *string `json:"status,omitempty" example:"Closed"`
	// Object with details of the company's legal representative.
	Representative *Representative `json:"representative,omitempty"`
} // @name LegalPerson

// Representative is a struct designed to store legal person representative data.
//
// swagger:model Representative
// @Description Representative object from LegalPerson
type Representative struct {
	// The legal representative’s name.
	Name *string `json:"name,omitempty" example:"John Doe"`
	// The document number of the legal representative.
	Document *string `json:"document,omitempty" example:"91315026015"`
	// The email address of the legal representative.
	Email *string `json:"email,omitempty" example:"john.doe@example.com"`
	// The role of the legal representative within the company.
	Role *string `json:"role,omitempty" example:"CFO"`
} // @name Representative
