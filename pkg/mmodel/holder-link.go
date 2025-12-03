package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// LinkType represents the type of relationship between Holder and Alias (TpVinc)
type LinkType string

const (
	// LinkTypePrimaryHolder represents TpVinc = 1 (Primary Holder / Titular Principal)
	LinkTypePrimaryHolder LinkType = "PRIMARY_HOLDER"
	// LinkTypeLegalRepresentative represents TpVinc = 2 (Legal Representative / Proxy)
	LinkTypeLegalRepresentative LinkType = "LEGAL_REPRESENTATIVE"
	// LinkTypeResponsibleParty represents TpVinc = 3 (Responsible Party)
	LinkTypeResponsibleParty LinkType = "RESPONSIBLE_PARTY"
)

// TpVincMapping maps LinkType to its numeric TpVinc value
var ValidLinkTypeMapping = map[LinkType]bool{
	LinkTypePrimaryHolder:       true,
	LinkTypeLegalRepresentative: true,
	LinkTypeResponsibleParty:    true,
}


// IsValidLinkType checks if a string is a valid LinkType.
func IsValidLinkType(linkTypeStr string) bool {
	linkType := LinkType(linkTypeStr)
	return ValidLinkTypeMapping[linkType]
}

// GetValidLinkTypes returns a slice of all valid LinkType strings.
func GetValidLinkTypes() []string {
	validTypes := make([]string, 0, len(ValidLinkTypeMapping))
	for linkType := range ValidLinkTypeMapping {
		validTypes = append(validTypes, string(linkType))
	}
	return validTypes
}

// CreateHolderLinkInput is a struct designed to encapsulate request create payload data.
//
// swagger:model CreateHolderLinkInput
// @Description CreateHolderLinkRequest payload
type CreateHolderLinkInput struct {
	// Unique identifier of the alias to be linked.
	AliasID string `json:"aliasId" validate:"required,uuid" example:"00000000-0000-0000-0000-000000000000"`
	// Type of relationship between the holder and the alias (TpVinc).
	// * PRIMARY_HOLDER (TpVinc=1) - Primary account holder
	// * LEGAL_REPRESENTATIVE (TpVinc=2) - Legal Representative or Proxy
	// * RESPONSIBLE_PARTY (TpVinc=3) - Responsible Party
	LinkType string `json:"linkType" validate:"required,oneof=PRIMARY_HOLDER LEGAL_REPRESENTATIVE RESPONSIBLE_PARTY" example:"PRIMARY_HOLDER"`
	// An object containing key-value pairs to add as metadata.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateHolderLinkRequest

// UpdateHolderLinkInput is a struct designed to encapsulate request update payload data.
//
// swagger:model UpdateHolderLinkInput
// @Description UpdateHolderLinkRequest payload
type UpdateHolderLinkInput struct {
	// Updated type of relationship (TpVinc).
	LinkType *string `json:"linkType" validate:"omitempty,oneof=PRIMARY_HOLDER LEGAL_REPRESENTATIVE RESPONSIBLE_PARTY" example:"LEGAL_REPRESENTATIVE"`
	// Updated metadata.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateHolderLinkRequest

// HolderLink is a struct designed to store the relationship between Holder and Alias.
//
// swagger:model HolderLink
// @Description HolderLinkResponse payload - represents the relationship (TpVinc) between a Holder and an Alias
type HolderLink struct {
	ID        *uuid.UUID     `json:"id,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	HolderID  *uuid.UUID     `json:"holderId" example:"00000000-0000-0000-0000-000000000000"`
	AliasID   *uuid.UUID     `json:"aliasId" example:"00000000-0000-0000-0000-000000000000"`
	LinkType  *string        `json:"linkType" example:"PRIMARY_HOLDER" enums:"PRIMARY_HOLDER,LEGAL_REPRESENTATIVE,RESPONSIBLE_PARTY"`
	TpVinc    *int           `json:"tpVinc,omitempty" example:"1" description:"Numeric TpVinc value (1=PRIMARY_HOLDER, 2=LEGAL_REPRESENTATIVE, 3=RESPONSIBLE_PARTY)"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"createdAt" example:"2025-01-01T00:00:00Z"`
	UpdatedAt time.Time      `json:"updatedAt" example:"2025-01-01T00:00:00Z"`
	DeletedAt *time.Time     `json:"deletedAt" example:"2025-01-01T00:00:00Z"`
} // @name HolderLinkResponse
