// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Re-export errors from constant package for backward compatibility.
// Callers should migrate to using constant.ErrKeyset* directly.
var (
	ErrKeysetNotFound         = constant.ErrKeysetNotFound
	ErrKeysetAlreadyExists    = constant.ErrKeysetAlreadyExists
	ErrKeysetRevisionConflict = constant.ErrKeysetRevisionConflict
)

// OrganizationKeyset stores wrapped keyset metadata for an organization.
// Wrapped keysets are encrypted by a KEK in the KMS provider.
type OrganizationKeyset struct {
	TenantID          string
	OrganizationID    string
	Version           int
	KEKPath           string
	KEKMountPath      string
	WrappedKeyset     string
	KeysetInfo        KeysetInfo
	WrappedHMACKeyset string
	HMACKeysetInfo    KeysetInfo
	Revision          int64
	CreatedAt         time.Time
	RotatedAt         *time.Time
}

// KeysetInfo contains metadata about a Tink keyset without exposing key material.
type KeysetInfo struct {
	PrimaryKeyID uint32
	Keys         []KeyInfo
}

// KeyInfo describes a single key within a keyset.
type KeyInfo struct {
	KeyID     uint32
	Status    string
	Type      string
	IsPrimary bool
}

// Validate checks that required fields are present.
func (k *OrganizationKeyset) Validate() error {
	if k.OrganizationID == "" {
		return fmt.Errorf("organization_id is required")
	}

	if k.Version < 1 {
		return fmt.Errorf("version must be >= 1")
	}

	if k.KEKPath == "" {
		return fmt.Errorf("kek_path is required")
	}

	// Required on write (Save/Update call Validate); the read/unwrap path tolerates
	// legacy records lacking it via fallback. A legacy record must backfill this before re-save.
	if k.KEKMountPath == "" {
		return fmt.Errorf("kek_mount_path is required")
	}

	if k.WrappedKeyset == "" {
		return fmt.Errorf("wrapped_keyset is required")
	}

	if k.KeysetInfo.PrimaryKeyID == 0 {
		return fmt.Errorf("keyset_info.primary_key_id is required")
	}

	// If HMAC keyset is provided, validate its info
	if k.WrappedHMACKeyset != "" && k.HMACKeysetInfo.PrimaryKeyID == 0 {
		return fmt.Errorf("hmac_keyset_info.primary_key_id is required when wrapped_hmac_keyset is provided")
	}

	return nil
}

// SafeView returns a copy with wrapped keysets redacted for logging/API responses.
func (k *OrganizationKeyset) SafeView() OrganizationKeyset {
	return OrganizationKeyset{
		TenantID:          k.TenantID,
		OrganizationID:    k.OrganizationID,
		Version:           k.Version,
		KEKPath:           k.KEKPath,
		KEKMountPath:      k.KEKMountPath,
		WrappedKeyset:     "[REDACTED]",
		KeysetInfo:        k.KeysetInfo,
		WrappedHMACKeyset: "[REDACTED]",
		HMACKeysetInfo:    k.HMACKeysetInfo,
		Revision:          k.Revision,
		CreatedAt:         k.CreatedAt,
		RotatedAt:         k.RotatedAt,
	}
}
