package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// Settings is a struct designed to store Settings data.
//
// swagger:model Settings
// @Description Settings object
type Settings struct {
	// The unique identifier of the Settings.
	ID uuid.UUID `json:"id,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// The unique identifier of the Organization.
	OrganizationID uuid.UUID `json:"organizationId,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// The unique identifier of the Ledger.
	LedgerID uuid.UUID `json:"ledgerId,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// The key identifier for the setting.
	Key string `json:"key,omitempty" example:"accounting_validation_enabled"`
	// The value of the setting.
	Value string `json:"value,omitempty" example:"true"`
	// A description for the setting.
	Description string `json:"description,omitempty" example:"Controls whether strict accounting validation rules are enforced for transactions"`
	// The timestamp when the setting was created.
	CreatedAt time.Time `json:"createdAt" example:"2025-01-01T00:00:00Z"`
	// The timestamp when the setting was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2025-01-01T00:00:00Z"`
	// The timestamp when the setting was deleted.
	DeletedAt *time.Time `json:"deletedAt" example:"2025-01-01T00:00:00Z"`
} // @name Settings

// CreateSettingsInput is a struct designed to store CreateSettingsInput data.
//
// swagger:model CreateSettingsInput
// @Description CreateSettingsInput payload
type CreateSettingsInput struct {
	// The key identifier for the setting.
	Key string `json:"key" validate:"required,max=100" example:"accounting_validation_enabled"`
	// The value of the setting.
	Value string `json:"value" validate:"required,max=100" example:"true"`
	// A description for the setting.
	Description string `json:"description,omitempty" validate:"max=255" example:"Controls whether strict accounting validation rules are enforced for transactions"`
} // @name CreateSettingsInput

// UpdateSettingsInput is a struct designed to store settings update data.
//
// swagger:model UpdateSettingsInput
// @Description UpdateSettingsInput payload
type UpdateSettingsInput struct {
	// The value of the setting.
	Value string `json:"value,omitempty" validate:"max=100" example:"false"`
	// A description for the setting.
	Description string `json:"description,omitempty" validate:"max=255" example:"Controls whether strict accounting validation rules are enforced for transactions"`
} // @name UpdateSettingsInput
