// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// Resolver names the manifest's scope.routes refer to.
const (
	resolverAccountByAlias      = "accountByAlias"
	resolverExternalAccount     = "externalAccount"
	resolverTransactionAccounts = "transactionAccounts"
	resolverBalanceAccount      = "balanceAccount"
)

// errScopeResolverUnconfined refuses a lookup the request does not confine to one
// organization and one ledger: an alias, a transaction or a balance is only
// unique inside a ledger, so resolving it anywhere wider could name another
// ledger's account.
var errScopeResolverUnconfined = errors.New("scope resolution needs exactly one organization and one ledger named by the request")

// scopeResolvers adapts the ledger's ScopeResolver to the authorization
// middleware: each method translates the values one request carries into the
// account ids they stand for. A value that names nothing is left out of the
// answer, which the middleware refuses with 422 naming where it was read; a
// failed lookup is an error, which it refuses with 503.
type scopeResolvers struct {
	resolver query.ScopeResolver
}

// registerScopeResolvers registers every resolver the manifest names. It must
// run before declaration.WireScope, which refuses a manifest naming a resolver
// that is not registered.
func registerScopeResolvers(auth *middleware.AuthClient, resolver query.ScopeResolver) error {
	set := scopeResolvers{resolver: resolver}

	for name, fn := range map[string]middleware.ScopeResolver{
		resolverAccountByAlias:      set.accountByAlias,
		resolverExternalAccount:     set.externalAccount,
		resolverTransactionAccounts: set.transactionAccounts,
		resolverBalanceAccount:      set.balanceAccount,
	} {
		if err := auth.RegisterScopeResolver(name, fn); err != nil {
			return err
		}
	}

	return nil
}

// accountByAlias answers the account of each alias. A balance-key suffix is
// dropped, as the transaction paths do before resolving an alias.
func (s scopeResolvers) accountByAlias(ctx context.Context, in middleware.ResolveInput) (map[string][]string, error) {
	return s.aliasesToAccounts(ctx, in, mtransaction.BareAlias)
}

// externalAccount answers the external account of each asset code.
func (s scopeResolvers) externalAccount(ctx context.Context, in middleware.ResolveInput) (map[string][]string, error) {
	return s.aliasesToAccounts(ctx, in, func(code string) string {
		return constant.DefaultExternalAccountAliasPrefix + code
	})
}

func (s scopeResolvers) aliasesToAccounts(ctx context.Context, in middleware.ResolveInput, toAlias func(string) string) (map[string][]string, error) {
	organizationID, ledgerID, ok, err := confinement(in)
	if err != nil || !ok {
		return map[string][]string{}, err
	}

	aliasOf := make(map[string]string, len(in.Values))
	aliases := make([]string, 0, len(in.Values))
	asked := make(map[string]struct{}, len(in.Values))

	for _, value := range in.Values {
		alias := toAlias(value)
		aliasOf[value] = alias

		if _, dup := asked[alias]; !dup {
			asked[alias] = struct{}{}
			aliases = append(aliases, alias)
		}
	}

	resolution, err := s.resolver.AccountIDsByAlias(ctx, organizationID, ledgerID, aliases)
	if err != nil {
		return nil, err
	}

	out := make(map[string][]string, len(in.Values))

	for _, value := range in.Values {
		if id, found := resolution.AccountIDs[aliasOf[value]]; found {
			out[value] = []string{id.String()}
		}
	}

	return out, nil
}

// transactionAccounts answers the accounts of every leg of each transaction.
func (s scopeResolvers) transactionAccounts(ctx context.Context, in middleware.ResolveInput) (map[string][]string, error) {
	return s.byID(ctx, in, func(ctx context.Context, organizationID, ledgerID, id uuid.UUID) ([]uuid.UUID, bool, error) {
		return s.resolver.AccountIDsOfTransaction(ctx, organizationID, ledgerID, id)
	})
}

// balanceAccount answers the account that owns each balance.
func (s scopeResolvers) balanceAccount(ctx context.Context, in middleware.ResolveInput) (map[string][]string, error) {
	return s.byID(ctx, in, func(ctx context.Context, organizationID, ledgerID, id uuid.UUID) ([]uuid.UUID, bool, error) {
		accountID, found, err := s.resolver.AccountIDOfBalance(ctx, organizationID, ledgerID, id)

		return []uuid.UUID{accountID}, found, err
	})
}

type idLookup func(ctx context.Context, organizationID, ledgerID, id uuid.UUID) ([]uuid.UUID, bool, error)

// byID resolves each value that is a uuid; one that is not names nothing.
func (s scopeResolvers) byID(ctx context.Context, in middleware.ResolveInput, lookup idLookup) (map[string][]string, error) {
	organizationID, ledgerID, ok, err := confinement(in)
	if err != nil || !ok {
		return map[string][]string{}, err
	}

	out := make(map[string][]string, len(in.Values))

	for _, value := range in.Values {
		id, isUUID := parseUUID(value)
		if !isUUID {
			continue
		}

		accountIDs, found, err := lookup(ctx, organizationID, ledgerID, id)
		if err != nil {
			return nil, fmt.Errorf("resolve %s %s: %w", in.Dimension, value, err)
		}

		if !found {
			continue
		}

		resolved := make([]string, 0, len(accountIDs))
		for _, accountID := range accountIDs {
			resolved = append(resolved, accountID.String())
		}

		out[value] = resolved
	}

	return out, nil
}

// confinement reads the one organization and one ledger the request names. ok is
// false when they are named but are not uuids: no record can live there, so
// nothing resolves.
func confinement(in middleware.ResolveInput) (organizationID, ledgerID uuid.UUID, ok bool, err error) {
	organizations, ledgers := in.Known["organizationId"], in.Known["ledgerId"]
	if len(organizations) != 1 || len(ledgers) != 1 {
		return uuid.Nil, uuid.Nil, false, errScopeResolverUnconfined
	}

	organizationID, orgOK := parseUUID(organizations[0])
	ledgerID, ledgerOK := parseUUID(ledgers[0])

	return organizationID, ledgerID, orgOK && ledgerOK, nil
}

func parseUUID(value string) (uuid.UUID, bool) {
	id, err := uuid.Parse(value)

	return id, err == nil
}
