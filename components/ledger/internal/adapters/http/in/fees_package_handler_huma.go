// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// This file holds the shared response envelopes and per-operation security metadata for
// the fee-package surface. The ledger-scoped handlers that construct these envelopes
// live in fees_v2_handler.go; the auth ("plugin-fees","packages",verb) + tenant +
// ParseUUIDPathParameters("packages") middleware chain is attached on the /v2 Fiber
// group BEFORE the Huma terminal (see fees_v2_register.go), so the Security metadata
// here is SPEC metadata only.

// secPackageBearer advertises that each package operation accepts a JWT bearer token
// (Bearer-only, matching the Fiber guard chain). SPEC metadata only;
// runtime auth is the Fiber guard chain.
var secPackageBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// CreatePackageOutputHuma pins 201 (matching the Fiber fiber.StatusCreated).
type CreatePackageOutputHuma struct {
	Status int
	Body   *pack.Package
}

// ListPackagesOutputHuma carries the pagination envelope verbatim.
type ListPackagesOutputHuma struct {
	Status int
	Body   model.Pagination
}

// GetPackageOutputHuma carries the package verbatim.
type GetPackageOutputHuma struct {
	Status int
	Body   *pack.Package
}

// UpdatePackageOutputHuma carries the updated package (200, matching the Fiber
// fiber.StatusOK).
type UpdatePackageOutputHuma struct {
	Status int
	Body   *pack.Package
}

// DeletePackageOutputHuma has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204, matching the Fiber fiber.StatusNoContent path.
type DeletePackageOutputHuma struct{}
