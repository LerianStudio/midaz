// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package multitenant

import (
	"errors"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
)

// Tenant-isolation is a third-rail invariant: the tenant ID is the SOLE
// boundary between tenants, so the empty-and-malformed rejection that gates
// every per-tenant resolution must be defined once. ValidateTenantID is that
// single predicate; the engine resolver and the datasource schema source both
// call it and wrap the outcome in their own error types.

var (
	// ErrTenantIDRequired indicates no tenant ID was supplied where multi-tenant
	// resolution requires one. Callers fail closed rather than fall back to a
	// shared pool.
	ErrTenantIDRequired = errors.New("tenant id is required for multi-tenant resolution")

	// ErrTenantIDInvalid indicates the supplied tenant ID failed the lib-commons
	// shape check, so the reporter's notion of a valid tenant matches the rest of
	// midaz.
	ErrTenantIDInvalid = errors.New("tenant id is invalid")
)

// ValidateTenantID enforces the tenant-isolation predicate: a tenant ID must be
// present and well-formed. It returns ErrTenantIDRequired or ErrTenantIDInvalid
// so callers can wrap the failure in their own error type while sharing one
// definition of "valid tenant".
func ValidateTenantID(tenantID string) error {
	if tenantID == "" {
		return ErrTenantIDRequired
	}

	if !tmcore.IsValidTenantID(tenantID) {
		return ErrTenantIDInvalid
	}

	return nil
}
