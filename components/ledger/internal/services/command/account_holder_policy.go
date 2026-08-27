// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import "github.com/LerianStudio/midaz/v4/pkg/mmodel"

// RouteHolderPolicy states whether the route version that received the request
// contracts the account holder seam.
//
// It is an alias of mmodel.HolderPolicy: the account repository shapes its SQL
// projection on the same signal, and the repository owns the interface this
// package depends on, so the type has to live outside both. The alias keeps the
// command-layer name the seam is documented under.
//
// It is the command-layer sibling of the transaction cores' routeVersionPolicy: the
// use case is transport-agnostic and cannot read the request path, so the version
// signal has to travel as an argument. They stay separate types because they sit on
// opposite sides of the dependency direction — the fee and tracer seams live in the
// transaction handler, the holder seam in the account and organization use cases.
type RouteHolderPolicy = mmodel.HolderPolicy

const (
	// HolderOffV1 is the /v1 account contract. It shipped before the holder seam
	// existed, so a /v1 create must not acquire a holder link, a holder-skip
	// rejection, or a required-holder rejection from a version upgrade. A holderId
	// or skip.holder in a /v1 body is inert: the /v1 response withholds both holder
	// keys (see AccountV1), so the field has no readable effect on this contract.
	HolderOffV1 = mmodel.HolderOffV1

	// HolderOnV2 is the /v2 account contract, which includes the holder seam:
	// the requireHolder gate, the two-key per-call skip, and the self-holder
	// default. The holder-account composition route contracts it too — it exists
	// to link a holder — and is served on /v2 only.
	HolderOnV2 = mmodel.HolderOnV2
)
