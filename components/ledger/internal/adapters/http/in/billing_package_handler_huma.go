// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// This file holds the shared response envelopes and per-operation security metadata for
// the billing-package surface. The ledger-scoped handlers that construct these envelopes
// live in fees_v2_handler.go; the auth ("plugin-fees","billing-packages",verb) + tenant +
// ParseUUIDPathParameters("billing-packages") middleware chain is attached on the /v2
// Fiber group BEFORE the Huma terminal (see fees_v2_register.go), so the Security
// metadata here is SPEC metadata only.

// secBillingBearer advertises that each billing-package operation accepts a JWT bearer
// token (Bearer-only, matching the Fiber guard chain). SPEC metadata
// only; runtime auth is the Fiber guard chain.
var secBillingBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// CreateBillingPackageOutputHuma pins 201 (matching the Fiber fiber.StatusCreated).
type CreateBillingPackageOutputHuma struct {
	Status int
	Body   *model.BillingPackage
}

// ListBillingPackagesOutputHuma carries the pagination envelope verbatim.
type ListBillingPackagesOutputHuma struct {
	Status int
	Body   model.Pagination
}

// GetBillingPackageOutputHuma carries the billing package verbatim.
type GetBillingPackageOutputHuma struct {
	Status int
	Body   *model.BillingPackage
}

// UpdateBillingPackageOutputHuma carries the updated package (200, matching the Fiber
// fiber.StatusOK).
type UpdateBillingPackageOutputHuma struct {
	Status int
	Body   *model.BillingPackage
}

// DeleteBillingPackageOutputHuma has NO Body field: paired with DefaultStatus 204 it
// makes Huma emit a bodiless 204, matching the Fiber fiber.StatusNoContent path.
type DeleteBillingPackageOutputHuma struct{}
