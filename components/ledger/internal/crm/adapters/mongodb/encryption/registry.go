// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// RegistryMongoDBModel is the MongoDB representation of OrganizationRegistryRecord.
type RegistryMongoDBModel struct {
	TenantID             string                 `bson:"tenant_id,omitempty"`
	OrganizationID       string                 `bson:"organization_id"`
	Status               mmodel.RegistryStatus  `bson:"status"`
	ProtectionModel      mmodel.ProtectionModel `bson:"protection_model"`
	CurrentVersion       int                    `bson:"current_version"`
	ReadableVersions     []int                  `bson:"readable_versions"`
	Revision             int64                  `bson:"revision"`
	LegacyReadable       bool                   `bson:"legacy_readable"`
	CreatedAt            time.Time              `bson:"created_at"`
	UpdatedAt            time.Time              `bson:"updated_at"`
	CreatedBy            string                 `bson:"created_by"`
	UpdatedBy            string                 `bson:"updated_by"`
	LastTransitionReason string                 `bson:"last_transition_reason"`
}

// RegistryFromEntity converts a domain OrganizationRegistryRecord to MongoDB model.
func RegistryFromEntity(r *mmodel.OrganizationRegistryRecord) *RegistryMongoDBModel {
	if r == nil {
		return nil
	}

	readableVersions := make([]int, len(r.ReadableVersions))
	copy(readableVersions, r.ReadableVersions)

	return &RegistryMongoDBModel{
		TenantID:             r.TenantID,
		OrganizationID:       r.OrganizationID,
		Status:               r.Status,
		ProtectionModel:      r.ProtectionModel,
		CurrentVersion:       r.CurrentVersion,
		ReadableVersions:     readableVersions,
		Revision:             r.Revision,
		LegacyReadable:       r.LegacyReadable,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
		CreatedBy:            r.CreatedBy,
		UpdatedBy:            r.UpdatedBy,
		LastTransitionReason: r.LastTransitionReason,
	}
}

// ToEntity converts the MongoDB model to a domain OrganizationRegistryRecord.
func (m *RegistryMongoDBModel) ToEntity() *mmodel.OrganizationRegistryRecord {
	if m == nil {
		return nil
	}

	readableVersions := make([]int, len(m.ReadableVersions))
	copy(readableVersions, m.ReadableVersions)

	return &mmodel.OrganizationRegistryRecord{
		TenantID:             m.TenantID,
		OrganizationID:       m.OrganizationID,
		Status:               m.Status,
		ProtectionModel:      m.ProtectionModel,
		CurrentVersion:       m.CurrentVersion,
		ReadableVersions:     readableVersions,
		Revision:             m.Revision,
		LegacyReadable:       m.LegacyReadable,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
		CreatedBy:            m.CreatedBy,
		UpdatedBy:            m.UpdatedBy,
		LastTransitionReason: m.LastTransitionReason,
	}
}
