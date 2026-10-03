// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/stretchr/testify/require"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
)

// manifestScopeResolverNames are the resolvers the embedded manifest names; the
// boot registers them before wiring the scope.
var manifestScopeResolverNames = []string{"accountByAlias", "externalAccount", "transactionAccounts", "balanceAccount"}

// wireManifestScope wires the embedded manifest as the boot does. The routes these
// tests drive resolve nothing, so every resolver fails the test when called.
func wireManifestScope(t *testing.T, auth *middleware.AuthClient) {
	t.Helper()

	for _, name := range manifestScopeResolverNames {
		require.NoError(t, auth.RegisterScopeResolver(name, func(_ context.Context, in middleware.ResolveInput) ([][]string, error) {
			t.Errorf("scope resolver %s called for %v on a route these tests do not resolve", in.Resolver, in.Items)

			return nil, nil
		}))
	}

	require.NoError(t, declaration.WireScope(auth, ledgerembed.MidazManifest))
}
