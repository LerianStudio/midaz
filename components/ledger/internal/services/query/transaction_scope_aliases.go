// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// scopeAccountAliases returns the aliases of the live accounts scope allows, for
// a transaction list that also confines pending transactions by the legs their
// body names. It reads nothing when scope confines no account.
func (uc *UseCase) scopeAccountAliases(ctx context.Context, organizationID, ledgerID uuid.UUID, scope http.ScopeConfinement) ([]string, error) {
	ids, confined := scope.IDs("accountId")
	if !confined || len(ids) == 0 {
		return nil, nil
	}

	accounts, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, ids)
	if err != nil {
		return nil, err
	}

	aliases := make([]string, 0, len(accounts))

	for _, acc := range accounts {
		if acc != nil && acc.Alias != nil && *acc.Alias != "" {
			aliases = append(aliases, *acc.Alias)
		}
	}

	return aliases, nil
}
