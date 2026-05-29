// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import "errors"

// ProvisionEncryptionInput represents the input for provisioning an organization for envelope encryption.
//
// swagger:model ProvisionEncryptionInput
// @Description ProvisionEncryptionRequest payload
type ProvisionEncryptionInput struct {
	// The actor performing the provisioning operation.
	Actor string `json:"actor" validate:"required" example:"admin@example.com"`
	// The reason for provisioning the organization.
	Reason string `json:"reason" validate:"required" example:"Initial encryption setup"`
} // @name ProvisionEncryptionRequest

// Validate validates the provision encryption input.
func (p *ProvisionEncryptionInput) Validate() error {
	if p.Actor == "" {
		return errors.New("actor is required")
	}

	if p.Reason == "" {
		return errors.New("reason is required")
	}

	return nil
}

// ActivateEncryptionInput represents the input for activating an organization's envelope encryption.
//
// swagger:model ActivateEncryptionInput
// @Description ActivateEncryptionRequest payload
type ActivateEncryptionInput struct {
	// The actor performing the activation operation.
	Actor string `json:"actor" validate:"required" example:"admin@example.com"`
	// The reason for activating the organization's encryption.
	Reason string `json:"reason" validate:"required" example:"Ready for production"`
} // @name ActivateEncryptionRequest

// Validate validates the activate encryption input.
func (a *ActivateEncryptionInput) Validate() error {
	if a.Actor == "" {
		return errors.New("actor is required")
	}

	if a.Reason == "" {
		return errors.New("reason is required")
	}

	return nil
}

// ProvisionEncryptionResponse represents the response for a successful provisioning operation.
//
// swagger:model ProvisionEncryptionResponse
// @Description ProvisionEncryptionResponse payload
type ProvisionEncryptionResponse struct {
	// The unique identifier of the organization.
	OrganizationID string `json:"organization_id" example:"00000000-0000-0000-0000-000000000000"`
	// The path to the Key Encryption Key in Vault.
	KEKPath string `json:"kek_path" example:"transit/keys/org-00000000-0000-0000-0000-000000000000"`
	// The primary key ID for AEAD encryption.
	AEADPrimaryKeyID uint32 `json:"aead_primary_key_id" example:"1"`
	// The primary key ID for MAC operations.
	MACPrimaryKeyID uint32 `json:"mac_primary_key_id" example:"1"`
	// The current provisioning status.
	Status string `json:"status" example:"pending_migration"`
} // @name ProvisionEncryptionResponse

// ActivateEncryptionResponse represents the response for a successful activation operation.
//
// swagger:model ActivateEncryptionResponse
// @Description ActivateEncryptionResponse payload
type ActivateEncryptionResponse struct {
	// The unique identifier of the organization.
	OrganizationID string `json:"organization_id" example:"00000000-0000-0000-0000-000000000000"`
	// The current provisioning status.
	Status string `json:"status" example:"active"`
} // @name ActivateEncryptionResponse

// ProvisioningStatusResponse represents the response for a provisioning status query.
//
// swagger:model ProvisioningStatusResponse
// @Description ProvisioningStatusResponse payload
type ProvisioningStatusResponse struct {
	// The unique identifier of the organization.
	OrganizationID string `json:"organization_id" example:"00000000-0000-0000-0000-000000000000"`
	// The current provisioning status.
	Status string `json:"status,omitempty" example:"active"`
	// Whether the organization has been provisioned for envelope encryption.
	Provisioned bool `json:"provisioned" example:"true"`
} // @name ProvisioningStatusResponse
