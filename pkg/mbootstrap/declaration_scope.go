// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mbootstrap

import (
	"fmt"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
)

// WireScopeWithoutDeclaration teaches a component's route authorization client
// the scope section of its embedded manifest when RI declaration is off and no
// publisher is built, so a partner's authorization question carries the
// dimensions its route names. With the flag on, declaration.New does the same.
// A deployment runs with the flag off when publication is someone else's job —
// the tenant manager's, multi-tenant — and its partners must still be asked
// with their scope, or one restricted to a ledger is refused on that ledger.
//
// authClient is anything the bootstrap holds as a token minter; only an
// *middleware.AuthClient authorizes routes, so anything else, nil included, has
// no scope to learn. A manifest the scope cannot be wired from is a build
// defect, so the error is meant to fail the boot.
func WireScopeWithoutDeclaration(authClient declaration.TokenMinter, manifest []byte) error {
	auth, ok := authClient.(*middleware.AuthClient)
	if !ok || auth == nil {
		return nil
	}

	if err := declaration.WireScope(auth, manifest); err != nil {
		return fmt.Errorf("wire manifest scope without RI declaration: %w", err)
	}

	return nil
}
