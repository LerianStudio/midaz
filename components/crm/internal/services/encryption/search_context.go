// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"fmt"
	"strings"
)

// SearchTokenContext contains the contextual information for generating search tokens.
// Note: RecordID is intentionally excluded so tokens can be computed at query time
// without knowing the record ID.
type SearchTokenContext struct {
	TenantID       string // Tenant identifier from JWT/context
	OrganizationID string // Organization owning the record
	FieldName      string // Name of the searchable field
}

// Validate checks that all required fields are present and non-empty.
func (stc SearchTokenContext) Validate() error {
	if strings.TrimSpace(stc.TenantID) == "" {
		return fmt.Errorf("search_token_context: tenant_id is required")
	}

	if strings.TrimSpace(stc.OrganizationID) == "" {
		return fmt.Errorf("search_token_context: organization_id is required")
	}

	if strings.TrimSpace(stc.FieldName) == "" {
		return fmt.Errorf("search_token_context: field_name is required")
	}

	return nil
}

// CanonicalInput returns the canonical input for MAC computation.
// Format: "tenant:{tenantID}:org:{orgID}:field:{fieldName}:{normalizedValue}"
func (stc SearchTokenContext) CanonicalInput(normalizedValue string) []byte {
	return fmt.Appendf(nil, "tenant:%s:org:%s:field:%s:%s",
		stc.TenantID, stc.OrganizationID, stc.FieldName, normalizedValue)
}
