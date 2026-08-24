// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// This file holds the response envelopes and per-operation security metadata for the
// fee-package surface. The shells that construct these envelopes live in
// fees_v2_handler.go; the auth ("midaz","packages",verb) + tenant +
// ParseUUIDPathParameters("packages") middleware chain is attached on the /v2 Fiber
// group BEFORE the Huma terminal (see fees_v2_register.go), so the Security metadata
// here is SPEC metadata only.

// secPackageBearer advertises that each package operation accepts a JWT bearer token
// (Bearer-only, matching the Fiber guard chain). SPEC metadata only;
// runtime auth is the Fiber guard chain.
var secPackageBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// CreatePackageResponse pins 201.
type CreatePackageResponse struct {
	Status int
	Body   *pack.Package
}

// ListPackagesResponse carries the pagination envelope verbatim.
type ListPackagesResponse struct {
	Status int
	Body   model.Pagination
}

// GetPackageResponse carries the package verbatim.
type GetPackageResponse struct {
	Status int
	Body   *pack.Package
}

// UpdatePackageResponse carries the updated package at 200.
type UpdatePackageResponse struct {
	Status int
	Body   *pack.Package
}

// DeletePackageResponse has NO Body field: paired with DefaultStatus 204 it makes
// Huma emit a bodiless 204.
type DeletePackageResponse struct{}
