// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// This file holds the shared response envelopes and per-operation security metadata for
// the billing-package surface. The ledger-scoped handlers that construct these envelopes
// live in fees_v2_handler.go; the auth ("midaz","billing-packages",verb) + tenant +
// ParseUUIDPathParameters("billing-packages") middleware chain is attached on the /v2
// Fiber group BEFORE the Huma terminal (see fees_v2_register.go), so the Security
// metadata here is SPEC metadata only.

// secBillingBearer advertises that each billing-package operation accepts a JWT bearer
// token (Bearer-only, matching the Fiber guard chain). SPEC metadata
// only; runtime auth is the Fiber guard chain.
var secBillingBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// CreateBillingPackageResponse pins 201 Created.
type CreateBillingPackageResponse struct {
	Status int
	Body   *model.BillingPackage
}

// ListBillingPackagesResponse carries the pagination envelope verbatim.
type ListBillingPackagesResponse struct {
	Status int
	Body   model.Pagination
}

// GetBillingPackageResponse carries the billing package verbatim.
type GetBillingPackageResponse struct {
	Status int
	Body   *model.BillingPackage
}

// UpdateBillingPackageResponse carries the updated package with 200 OK.
type UpdateBillingPackageResponse struct {
	Status int
	Body   *model.BillingPackage
}

// DeleteBillingPackageResponse has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204.
type DeleteBillingPackageResponse struct{}
