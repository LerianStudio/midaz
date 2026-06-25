// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"fmt"
	"strings"
)

// FieldContext contains the contextual information required for field-level encryption.
// All fields are required and used to construct canonical associated data (AAD)
// that binds the ciphertext to its originating context.
type FieldContext struct {
	TenantID       string // Tenant identifier from JWT/context
	OrganizationID string // Organization owning the record
	RecordID       string // Unique ID of the holder/alias record
	FieldName      string // Name of the field being encrypted (e.g., "tax_id", "document_number")
}

// Validate checks that all required fields are present and non-empty.
func (fc FieldContext) Validate() error {
	if strings.TrimSpace(fc.TenantID) == "" {
		return fmt.Errorf("field_context: tenant_id is required")
	}

	if strings.TrimSpace(fc.OrganizationID) == "" {
		return fmt.Errorf("field_context: organization_id is required")
	}

	if strings.TrimSpace(fc.RecordID) == "" {
		return fmt.Errorf("field_context: record_id is required")
	}

	if strings.TrimSpace(fc.FieldName) == "" {
		return fmt.Errorf("field_context: field_name is required")
	}

	return nil
}

// CanonicalAAD returns the canonical associated data string for AEAD encryption.
// Format: "tenant:{tenantID}:org:{orgID}:record:{recordID}:field:{fieldName}"
func (fc FieldContext) CanonicalAAD() []byte {
	return fmt.Appendf(nil, "tenant:%s:org:%s:record:%s:field:%s",
		fc.TenantID, fc.OrganizationID, fc.RecordID, fc.FieldName)
}
