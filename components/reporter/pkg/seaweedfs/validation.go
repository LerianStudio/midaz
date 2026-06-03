// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seaweedfs

import (
	"context"
	"fmt"
	"strings"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
)

// ValidateKeyForTenant verifies that the resolved S3 key starts with the
// authenticated tenant's ID prefix when in multi-tenant mode.
//
// In multi-tenant mode (tenant ID present in context), the key MUST start with
// "{tenantID}/". If it does not, this function returns an error indicating a
// tenant key prefix mismatch, which prevents cross-tenant object access.
//
// In single-tenant mode (no tenant ID in context or nil context), this function
// is a no-op and always returns nil.
//
// This function does NOT guard against path traversal within the tenant prefix
// (e.g., "tenantA/../tenantB/..."). Path traversal prevention is the responsibility
// of the S3-compatible object storage layer.
func ValidateKeyForTenant(ctx context.Context, key string) error {
	if ctx == nil {
		return nil
	}

	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		// Single-tenant mode: no validation needed
		return nil
	}

	expectedPrefix := strings.Trim(tenantID, "/") + "/"

	if !strings.HasPrefix(key, expectedPrefix) {
		return fmt.Errorf("tenant key prefix mismatch: key %q does not start with expected prefix %q for tenant %s",
			key, expectedPrefix, tenantID)
	}

	return nil
}
