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
	// tenant attaches the tenant's databases in multi-tenant mode; nil in
	// single-tenant mode, where the repositories use their static connection.
	tenant scopeTenant
}

// registerScopeResolvers registers every resolver the manifest names. It must
// run before declaration.WireScope, which refuses a manifest naming a resolver
// that is not registered.
func registerScopeResolvers(auth *middleware.AuthClient, resolver query.ScopeResolver, tenant scopeTenant) error {
	set := scopeResolvers{resolver: resolver, tenant: tenant}

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
func (s scopeResolvers) accountByAlias(ctx context.Context, in middleware.ResolveInput) ([][]string, error) {
	return s.aliasesToAccounts(ctx, in, mtransaction.BareAlias)
}

// externalAccount answers the external account of each asset code.
func (s scopeResolvers) externalAccount(ctx context.Context, in middleware.ResolveInput) ([][]string, error) {
	return s.aliasesToAccounts(ctx, in, func(code string) string {
		return constant.DefaultExternalAccountAliasPrefix + code
	})
}

// ledgerScope is one organization and ledger a lookup is confined to.
type ledgerScope struct {
	organizationID, ledgerID uuid.UUID
}

// aliasesToAccounts resolves the items in one read per organization and ledger
// they are confined to.
func (s scopeResolvers) aliasesToAccounts(ctx context.Context, in middleware.ResolveInput, toAlias func(string) string) ([][]string, error) {
	ctx, err := s.attachTenant(ctx)
	if err != nil {
		return nil, err
	}

	out := make([][]string, len(in.Items))
	aliasOf := make([]string, len(in.Items))
	scopeOf := make([]ledgerScope, len(in.Items))
	resolvable := make([]bool, len(in.Items))

	var order []ledgerScope

	asked := make(map[ledgerScope][]string)
	seen := make(map[ledgerScope]map[string]struct{})

	for i, item := range in.Items {
		organizationID, ledgerID, ok, err := confinement(item, in.Known)
		if err != nil {
			return nil, err
		}

		if !ok {
			continue
		}

		scope := ledgerScope{organizationID: organizationID, ledgerID: ledgerID}
		alias := toAlias(item.Value)

		aliasOf[i], scopeOf[i], resolvable[i] = alias, scope, true

		if _, known := seen[scope]; !known {
			seen[scope] = make(map[string]struct{})
			order = append(order, scope)
		}

		if _, dup := seen[scope][alias]; !dup {
			seen[scope][alias] = struct{}{}
			asked[scope] = append(asked[scope], alias)
		}
	}

	resolved := make(map[ledgerScope]map[string]uuid.UUID, len(order))

	for _, scope := range order {
		resolution, err := s.resolver.AccountIDsByAlias(ctx, scope.organizationID, scope.ledgerID, asked[scope])
		if err != nil {
			return nil, err
		}

		resolved[scope] = resolution.AccountIDs
	}

	for i := range in.Items {
		if !resolvable[i] {
			continue
		}

		if id, found := resolved[scopeOf[i]][aliasOf[i]]; found {
			out[i] = []string{id.String()}
		}
	}

	return out, nil
}

// transactionAccounts answers the accounts of every leg of each transaction.
func (s scopeResolvers) transactionAccounts(ctx context.Context, in middleware.ResolveInput) ([][]string, error) {
	return s.byID(ctx, in, func(ctx context.Context, organizationID, ledgerID, id uuid.UUID) ([]uuid.UUID, bool, error) {
		return s.resolver.AccountIDsOfTransaction(ctx, organizationID, ledgerID, id)
	})
}

// balanceAccount answers the account that owns each balance.
func (s scopeResolvers) balanceAccount(ctx context.Context, in middleware.ResolveInput) ([][]string, error) {
	return s.byID(ctx, in, func(ctx context.Context, organizationID, ledgerID, id uuid.UUID) ([]uuid.UUID, bool, error) {
		accountID, found, err := s.resolver.AccountIDOfBalance(ctx, organizationID, ledgerID, id)

		return []uuid.UUID{accountID}, found, err
	})
}

type idLookup func(ctx context.Context, organizationID, ledgerID, id uuid.UUID) ([]uuid.UUID, bool, error)

// byID resolves each item that is a uuid; one that is not names nothing.
func (s scopeResolvers) byID(ctx context.Context, in middleware.ResolveInput, lookup idLookup) ([][]string, error) {
	ctx, err := s.attachTenant(ctx)
	if err != nil {
		return nil, err
	}

	out := make([][]string, len(in.Items))

	for i, item := range in.Items {
		organizationID, ledgerID, ok, err := confinement(item, in.Known)
		if err != nil {
			return nil, err
		}

		id, isUUID := parseUUID(item.Value)
		if !ok || !isUUID {
			continue
		}

		accountIDs, found, err := lookup(ctx, organizationID, ledgerID, id)
		if err != nil {
			return nil, fmt.Errorf("resolve %s %s: %w", in.Dimension, item.Value, err)
		}

		if !found {
			continue
		}

		resolved := make([]string, 0, len(accountIDs))
		for _, accountID := range accountIDs {
			resolved = append(resolved, accountID.String())
		}

		out[i] = resolved
	}

	return out, nil
}

// attachTenant attaches the tenant's databases when the deployment is
// multi-tenant, and leaves the context as it is otherwise.
func (s scopeResolvers) attachTenant(ctx context.Context) (context.Context, error) {
	if s.tenant == nil {
		return ctx, nil
	}

	return s.tenant.attach(ctx)
}

// confinement reads the one organization and one ledger an item is looked up in:
// those of its own body element first, else the ones the request names directly.
// ok is false when they are named but are not uuids: no record can live there, so
// the item resolves to nothing.
func confinement(item middleware.ResolveItem, known map[string][]string) (organizationID, ledgerID uuid.UUID, ok bool, err error) {
	organization, hasOrganization := single(item.Siblings, known, "organizationId")
	ledger, hasLedger := single(item.Siblings, known, "ledgerId")

	if !hasOrganization || !hasLedger {
		return uuid.Nil, uuid.Nil, false, errScopeResolverUnconfined
	}

	organizationID, orgOK := parseUUID(organization)
	ledgerID, ledgerOK := parseUUID(ledger)

	return organizationID, ledgerID, orgOK && ledgerOK, nil
}

// single is the one value of a dimension an item names, from its siblings or
// else from the request; false when there is none or more than one.
func single(siblings map[string]string, known map[string][]string, dimension string) (string, bool) {
	if value, named := siblings[dimension]; named {
		return value, true
	}

	if values := known[dimension]; len(values) == 1 {
		return values[0], true
	}

	return "", false
}

func parseUUID(value string) (uuid.UUID, bool) {
	id, err := uuid.Parse(value)

	return id, err == nil
}
