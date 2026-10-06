// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mbootstrap

import (
	"fmt"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
)

// WireManifestScope teaches a component's route authorization client the scope
// section of its embedded manifest, so a partner's authorization question
// carries the dimensions its route names. Partner scope is its own feature, not
// part of RI declaration: it is wired at every boot — single- or multi-tenant,
// IDP_DECLARATION_ENABLED on or off — and depends on no IdP setting and on no
// publication. Without it a partner restricted to a ledger is refused on that
// ledger. Wiring it again from the same manifest, as declaration.New does with
// the flag on, replaces the catalog with an identical one.
//
// authClient is anything the bootstrap holds as a token minter; only an
// *middleware.AuthClient authorizes routes, so anything else, nil included, has
// no scope to learn. A manifest the scope cannot be wired from is a build
// defect, so the error is meant to fail the boot.
func WireManifestScope(authClient declaration.TokenMinter, manifest []byte) error {
	auth, ok := authClient.(*middleware.AuthClient)
	if !ok || auth == nil {
		return nil
	}

	if err := declaration.WireScope(auth, manifest); err != nil {
		return fmt.Errorf("wire manifest scope: %w", err)
	}

	return nil
}
