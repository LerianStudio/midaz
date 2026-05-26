// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// KeysetMongoDBModel is the MongoDB representation of OrganizationKeyset.
type KeysetMongoDBModel struct {
	TenantID              string          `bson:"tenant_id,omitempty"`
	OrganizationID        string          `bson:"organization_id"`
	KEKPath               string          `bson:"kek_path"`
	WrappedKeyset         string          `bson:"wrapped_keyset"`
	KeysetInfo            KeysetInfoModel `bson:"keyset_info"`
	LegacyKeyImported     bool            `bson:"legacy_key_imported"`
	WrappedHMACKeyset     string          `bson:"wrapped_hmac_keyset,omitempty"`
	HMACKeysetInfo        KeysetInfoModel `bson:"hmac_keyset_info,omitempty"`
	LegacyHMACKeyImported bool            `bson:"legacy_hmac_key_imported"`
	Revision              int64           `bson:"revision"`
	CreatedAt             time.Time       `bson:"created_at"`
	RotatedAt             *time.Time      `bson:"rotated_at,omitempty"`
}

// KeysetInfoModel is the MongoDB representation of KeysetInfo.
type KeysetInfoModel struct {
	PrimaryKeyID uint32         `bson:"primary_key_id"`
	Keys         []KeyInfoModel `bson:"keys"`
}

// KeyInfoModel is the MongoDB representation of KeyInfo.
type KeyInfoModel struct {
	KeyID     uint32 `bson:"key_id"`
	Status    string `bson:"status"`
	Type      string `bson:"type"`
	IsPrimary bool   `bson:"is_primary"`
}

// KeysetFromEntity converts a domain OrganizationKeyset to MongoDB model.
func KeysetFromEntity(k *mmodel.OrganizationKeyset) *KeysetMongoDBModel {
	if k == nil {
		return nil
	}

	return &KeysetMongoDBModel{
		TenantID:              k.TenantID,
		OrganizationID:        k.OrganizationID,
		KEKPath:               k.KEKPath,
		WrappedKeyset:         k.WrappedKeyset,
		KeysetInfo:            keysetInfoFromEntity(k.KeysetInfo),
		LegacyKeyImported:     k.LegacyKeyImported,
		WrappedHMACKeyset:     k.WrappedHMACKeyset,
		HMACKeysetInfo:        keysetInfoFromEntity(k.HMACKeysetInfo),
		LegacyHMACKeyImported: k.LegacyHMACKeyImported,
		Revision:              k.Revision,
		CreatedAt:             k.CreatedAt,
		RotatedAt:             k.RotatedAt,
	}
}

// ToEntity converts the MongoDB model to a domain OrganizationKeyset.
func (m *KeysetMongoDBModel) ToEntity() *mmodel.OrganizationKeyset {
	if m == nil {
		return nil
	}

	return &mmodel.OrganizationKeyset{
		TenantID:              m.TenantID,
		OrganizationID:        m.OrganizationID,
		KEKPath:               m.KEKPath,
		WrappedKeyset:         m.WrappedKeyset,
		KeysetInfo:            m.KeysetInfo.toEntity(),
		LegacyKeyImported:     m.LegacyKeyImported,
		WrappedHMACKeyset:     m.WrappedHMACKeyset,
		HMACKeysetInfo:        m.HMACKeysetInfo.toEntity(),
		LegacyHMACKeyImported: m.LegacyHMACKeyImported,
		Revision:              m.Revision,
		CreatedAt:             m.CreatedAt,
		RotatedAt:             m.RotatedAt,
	}
}

func keysetInfoFromEntity(info mmodel.KeysetInfo) KeysetInfoModel {
	keys := make([]KeyInfoModel, len(info.Keys))
	for i, k := range info.Keys {
		keys[i] = KeyInfoModel{
			KeyID:     k.KeyID,
			Status:    k.Status,
			Type:      k.Type,
			IsPrimary: k.IsPrimary,
		}
	}

	return KeysetInfoModel{
		PrimaryKeyID: info.PrimaryKeyID,
		Keys:         keys,
	}
}

func (m *KeysetInfoModel) toEntity() mmodel.KeysetInfo {
	keys := make([]mmodel.KeyInfo, len(m.Keys))
	for i, k := range m.Keys {
		keys[i] = mmodel.KeyInfo{
			KeyID:     k.KeyID,
			Status:    k.Status,
			Type:      k.Type,
			IsPrimary: k.IsPrimary,
		}
	}

	return mmodel.KeysetInfo{
		PrimaryKeyID: m.PrimaryKeyID,
		Keys:         keys,
	}
}
