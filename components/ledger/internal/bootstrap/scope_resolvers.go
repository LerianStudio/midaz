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
	resolverHolderLedgers       = "holderLedgers"
	resolverAccountPortfolio    = "accountPortfolio"
	resolverAccountSegment      = "accountSegment"
	resolverInstrumentLedger    = "instrumentLedger"
)

// errScopeResolverNoOrganization refuses a holder lookup the request does not
// confine to one organization: holders are unique only inside one.
var errScopeResolverNoOrganization = errors.New("scope resolution needs exactly one organization named by the request")

// errScopeResolverNoHolder refuses an instrument lookup the request does not
// confine to one holder: instruments are read under the holder that owns them.
var errScopeResolverNoHolder = errors.New("scope resolution needs exactly one holder named by the request")

// errScopeInstrumentsUnavailable refuses an instrument lookup when no instrument
// reader is configured: answering nothing would ask the question without the
// instrument's ledger.
var errScopeInstrumentsUnavailable = errors.New("scope resolution of instruments has no instrument reader configured")

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
	// instruments reads the ledger of a holder's instruments from the CRM.
	instruments scopeInstruments
}

// instrumentLedgerReader reads the ledger each of a holder's instruments
// belongs to.
type instrumentLedgerReader interface {
	LedgerIDsByIDs(ctx context.Context, organizationID string, holderID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// scopeInstruments is the CRM side of the scope resolvers: the instrument reader
// and, in multi-tenant mode, the attachment of the tenant's CRM database.
type scopeInstruments struct {
	reader instrumentLedgerReader
	tenant scopeTenant
}

// registerScopeResolvers registers every resolver the manifest names. It must
// run before declaration.WireScope, which refuses a manifest naming a resolver
// that is not registered.
func registerScopeResolvers(auth *middleware.AuthClient, resolver query.ScopeResolver, tenant scopeTenant, instruments scopeInstruments) error {
	set := scopeResolvers{resolver: resolver, tenant: tenant, instruments: instruments}

	for name, fn := range map[string]middleware.ScopeResolver{
		resolverAccountByAlias:      set.accountByAlias,
		resolverExternalAccount:     set.externalAccount,
		resolverTransactionAccounts: set.transactionAccounts,
		resolverBalanceAccount:      set.balanceAccount,
		resolverHolderLedgers:       set.holderLedgers,
		resolverAccountPortfolio:    set.accountPortfolio,
		resolverAccountSegment:      set.accountSegment,
		resolverInstrumentLedger:    set.instrumentLedger,
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

// holderLedgers answers, for each holder, the ledgers it owns a live account in,
// read for every holder of the request at once. A holder without one, or a value
// that is not a uuid, names nothing.
func (s scopeResolvers) holderLedgers(ctx context.Context, in middleware.ResolveInput) ([][]string, error) {
	organization, named := single(nil, in.Known, "organizationId")
	if !named {
		return nil, errScopeResolverNoOrganization
	}

	out := make([][]string, len(in.Items))

	organizationID, ok := parseUUID(organization)
	if !ok {
		return out, nil
	}

	holders := make([]uuid.UUID, 0, len(in.Items))

	for _, item := range in.Items {
		if id, isUUID := parseUUID(item.Value); isUUID {
			holders = append(holders, id)
		}
	}

	if len(holders) == 0 {
		return out, nil
	}

	ctx, err := s.attachTenant(ctx)
	if err != nil {
		return nil, err
	}

	ledgers, err := s.resolver.LedgerIDsOfHolders(ctx, organizationID, holders)
	if err != nil {
		return nil, fmt.Errorf("resolve %s of holders: %w", in.Dimension, err)
	}

	for i, item := range in.Items {
		id, isUUID := parseUUID(item.Value)
		if !isUUID {
			continue
		}

		for _, ledgerID := range ledgers[id] {
			out[i] = append(out[i], ledgerID.String())
		}
	}

	return out, nil
}

// accountPortfolio answers the portfolio of each account; an account in none
// names nothing.
func (s scopeResolvers) accountPortfolio(ctx context.Context, in middleware.ResolveInput) ([][]string, error) {
	return s.accountPlacement(ctx, in, func(p query.AccountPlacement) *uuid.UUID { return p.PortfolioID })
}

// accountSegment answers the segment of each account; an account in none names
// nothing.
func (s scopeResolvers) accountSegment(ctx context.Context, in middleware.ResolveInput) ([][]string, error) {
	return s.accountPlacement(ctx, in, func(p query.AccountPlacement) *uuid.UUID { return p.SegmentID })
}

// accountPlacement reads the placement of the accounts in one read per
// organization and ledger they are confined to, and answers the part pick
// selects. A value that is not a uuid names nothing.
func (s scopeResolvers) accountPlacement(ctx context.Context, in middleware.ResolveInput, pick func(query.AccountPlacement) *uuid.UUID) ([][]string, error) {
	out := make([][]string, len(in.Items))
	accountOf := make([]uuid.UUID, len(in.Items))
	scopeOf := make([]ledgerScope, len(in.Items))
	resolvable := make([]bool, len(in.Items))

	var order []ledgerScope

	asked := make(map[ledgerScope][]uuid.UUID)

	for i, item := range in.Items {
		organizationID, ledgerID, ok, err := confinement(item, in.Known)
		if err != nil {
			return nil, err
		}

		id, isUUID := parseUUID(item.Value)
		if !ok || !isUUID {
			continue
		}

		scope := ledgerScope{organizationID: organizationID, ledgerID: ledgerID}
		accountOf[i], scopeOf[i], resolvable[i] = id, scope, true

		if _, known := asked[scope]; !known {
			order = append(order, scope)
		}

		asked[scope] = append(asked[scope], id)
	}

	if len(order) == 0 {
		return out, nil
	}

	ctx, err := s.attachTenant(ctx)
	if err != nil {
		return nil, err
	}

	placed := make(map[ledgerScope]map[uuid.UUID]query.AccountPlacement, len(order))

	for _, scope := range order {
		placements, err := s.resolver.PlacementOfAccounts(ctx, scope.organizationID, scope.ledgerID, asked[scope])
		if err != nil {
			return nil, fmt.Errorf("resolve %s of accounts: %w", in.Dimension, err)
		}

		placed[scope] = placements
	}

	for i := range in.Items {
		if !resolvable[i] {
			continue
		}

		if id := pick(placed[scopeOf[i]][accountOf[i]]); id != nil {
			out[i] = []string{id.String()}
		}
	}

	return out, nil
}

// instrumentLedger answers the ledger each instrument of the path's holder
// belongs to, read for every instrument of the request at once. An instrument
// that names no ledger, or a value that is not a uuid, names nothing.
func (s scopeResolvers) instrumentLedger(ctx context.Context, in middleware.ResolveInput) ([][]string, error) {
	if s.instruments.reader == nil {
		return nil, errScopeInstrumentsUnavailable
	}

	organization, named := single(nil, in.Known, "organizationId")
	if !named {
		return nil, errScopeResolverNoOrganization
	}

	holder, named := single(nil, in.Known, "holderId")
	if !named {
		return nil, errScopeResolverNoHolder
	}

	out := make([][]string, len(in.Items))

	holderID, ok := parseUUID(holder)
	if !ok {
		return out, nil
	}

	ids := make([]uuid.UUID, 0, len(in.Items))

	for _, item := range in.Items {
		if id, isUUID := parseUUID(item.Value); isUUID {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return out, nil
	}

	if s.instruments.tenant != nil {
		var err error

		ctx, err = s.instruments.tenant.attach(ctx)
		if err != nil {
			return nil, err
		}
	}

	ledgers, err := s.instruments.reader.LedgerIDsByIDs(ctx, organization, holderID, ids)
	if err != nil {
		return nil, fmt.Errorf("resolve %s of instruments: %w", in.Dimension, err)
	}

	for i, item := range in.Items {
		id, isUUID := parseUUID(item.Value)
		if !isUUID {
			continue
		}

		if ledger := ledgers[id]; ledger != "" {
			out[i] = []string{ledger}
		}
	}

	return out, nil
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
