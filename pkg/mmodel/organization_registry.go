// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

type RegistryStatus string

type ProtectionModel string

const (
	// RegistryStatusActive indicates the organization is provisioned for envelope encryption.
	// New encryptions use envelope mode. Old data may still be readable via LegacyReadable flag.
	RegistryStatusActive RegistryStatus = "active"
)

const (
	ProtectionModelLegacy   ProtectionModel = "legacy"
	ProtectionModelEnvelope ProtectionModel = "envelope"
)

// Re-export errors from constant package for backward compatibility.
// Callers should migrate to using constant.ErrRegistry* directly.
var (
	ErrRegistryRevisionConflict = constant.ErrRegistryRevisionConflict
	ErrRegistryNotFound         = constant.ErrRegistryNotFound
	ErrRegistryAlreadyExists    = constant.ErrRegistryAlreadyExists
)

// OrganizationRegistryRecord tracks the encryption state of an organization.
type OrganizationRegistryRecord struct {
	TenantID             string
	OrganizationID       string
	Status               RegistryStatus
	ProtectionModel      ProtectionModel
	CurrentVersion       int
	ReadableVersions     []int
	Revision             int64
	LegacyReadable       bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	CreatedBy            string
	UpdatedBy            string
	LastTransitionReason string
}

// NewOrganizationRegistryRecord creates a new registry record with initial state.
func NewOrganizationRegistryRecord(tenantID, organizationID, actor, reason string) (*OrganizationRegistryRecord, error) {
	tenantID = strings.TrimSpace(tenantID)
	organizationID = strings.TrimSpace(organizationID)
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)

	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	if organizationID == "" {
		return nil, fmt.Errorf("organization_id is required")
	}

	if actor == "" {
		return nil, fmt.Errorf("actor is required")
	}

	if reason == "" {
		return nil, fmt.Errorf("reason is required")
	}

	now := time.Now().UTC()

	return &OrganizationRegistryRecord{
		TenantID:             tenantID,
		OrganizationID:       organizationID,
		Status:               RegistryStatusActive,
		ProtectionModel:      ProtectionModelEnvelope,
		CurrentVersion:       1,
		ReadableVersions:     []int{1},
		Revision:             1,
		LegacyReadable:       false,
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            actor,
		UpdatedBy:            actor,
		LastTransitionReason: reason,
	}, nil
}
