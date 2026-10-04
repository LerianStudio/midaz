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
var manifestScopeResolverNames = []string{"accountByAlias", "externalAccount", "transactionAccounts", "balanceAccount", "holderLedgers", "accountPortfolio", "accountSegment"}

// wireManifestScope wires the embedded manifest as the boot does. accountByAlias
// answers legAccount for the ledger each alias's own element names, and the account
// placement resolvers answer an account in no portfolio and no segment; every other
// resolver fails the test when called.
func wireManifestScope(t *testing.T, auth *middleware.AuthClient) {
	t.Helper()

	for _, name := range manifestScopeResolverNames {
		resolver := func(_ context.Context, in middleware.ResolveInput) ([][]string, error) {
			t.Errorf("scope resolver %s called for %v on a route these tests do not resolve", in.Resolver, in.Items)

			return nil, nil
		}

		if name == "accountPortfolio" || name == "accountSegment" {
			resolver = func(_ context.Context, in middleware.ResolveInput) ([][]string, error) {
				return make([][]string, len(in.Items)), nil
			}
		}

		if name == "accountByAlias" {
			resolver = func(_ context.Context, in middleware.ResolveInput) ([][]string, error) {
				out := make([][]string, len(in.Items))
				for i, item := range in.Items {
					out[i] = []string{legAccount(item.Siblings["ledgerId"], item.Value)}
				}

				return out, nil
			}
		}

		require.NoError(t, auth.RegisterScopeResolver(name, resolver))
	}

	require.NoError(t, declaration.WireScope(auth, ledgerembed.MidazManifest))
}

// legAccount is the account the test resolver answers for alias within ledgerID, so
// the same alias in two ledgers names two accounts.
func legAccount(ledgerID, alias string) string {
	return "account:" + ledgerID + ":" + alias
}
