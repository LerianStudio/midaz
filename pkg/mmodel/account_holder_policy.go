// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

// HolderPolicy states whether the route version that received the request
// contracts the account holder seam. It is a distinct type so it cannot be
// transposed with the other arguments at a call site.
//
// It lives here, and not in the command or the adapter package, because both
// sides need it: the command layer gates the holder seam on it, and the account
// repository shapes its SQL projection on it. The repository owns the interface
// the command layer depends on, so a type declared in either package would close
// an import cycle with the other.
type HolderPolicy bool

const (
	// HolderOffV1 is the /v1 account contract. It shipped before the holder seam
	// existed, so a /v1 create must not acquire a holder link, a holder-skip
	// rejection, or a required-holder rejection from a version upgrade. A holderId
	// or skip.holder in a /v1 body is inert: the /v1 response withholds both holder
	// keys (see AccountV1), so the field has no readable effect on this contract.
	HolderOffV1 HolderPolicy = false

	// HolderOnV2 is the /v2 account contract, which includes the holder seam:
	// the requireHolder gate, the two-key per-call skip, and the self-holder
	// default. The holder-account composition route contracts it too — it exists
	// to link a holder — and is served on /v2 only.
	HolderOnV2 HolderPolicy = true
)

// ProjectsHolder reports whether a read on this contract must project the real
// holder columns. Only /v2 can observe them, so /v1 reads substitute constants
// and never name the columns — which keeps the /v1 contract servable against a
// schema that predates them.
func (p HolderPolicy) ProjectsHolder() bool {
	return p == HolderOnV2
}
