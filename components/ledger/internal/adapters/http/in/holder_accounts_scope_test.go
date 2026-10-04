// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestGetAccountsByHolder_ConfinedToThePartnerLedgers pins the holder's account
// list to the ledgers the decision allows a partner: only those reach the reader.
func TestGetAccountsByHolder_ConfinedToThePartnerLedgers(t *testing.T) {
	orgID, holderID, ownLedger := uuid.New(), uuid.New(), uuid.New()

	t.Run("a ledger partner reads only the accounts of its ledgers", func(t *testing.T) {
		reader := &stubHolderAccountsReader{}
		handler := &HolderAccountsHandler{Reader: reader}

		ctx := partnerScopedContext(t, `,"allowed":{"ledgerId":["`+ownLedger.String()+`"]}`, scopeDimensionLedger)

		_, err := handler.getAccountsByHolder(ctx, orgID, holderID, map[string]string{})
		require.NoError(t, err)
		assert.Equal(t, pkgHTTP.ScopeConfinement{"ledgerId": {ownLedger}}, reader.gotScope)
	})

	t.Run("an empty allowed list reads nothing", func(t *testing.T) {
		reader := &stubHolderAccountsReader{}
		handler := &HolderAccountsHandler{Reader: reader}

		ctx := partnerScopedContext(t, `,"allowed":{"ledgerId":[]}`, scopeDimensionLedger)

		_, err := handler.getAccountsByHolder(ctx, orgID, holderID, map[string]string{})
		require.NoError(t, err)
		assert.True(t, reader.gotScope.ListsNothing())
	})

	t.Run("a caller bound to no partner is not confined", func(t *testing.T) {
		reader := &stubHolderAccountsReader{}
		handler := &HolderAccountsHandler{Reader: reader}

		_, err := handler.getAccountsByHolder(context.Background(), orgID, holderID, map[string]string{})
		require.NoError(t, err)
		assert.Nil(t, reader.gotScope)
	})
}
