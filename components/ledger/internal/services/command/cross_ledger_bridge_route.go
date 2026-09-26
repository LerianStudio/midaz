// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// TransactionRouteCacheReader is the cross-ledger extension implemented by the
// production query use case. It stays separate from TransactionReader so the
// singular transaction readers and their test doubles do not gain it.
type TransactionRouteCacheReader interface {
	// GetOrCreateTransactionRouteCache returns the cached accounting view of a
	// transaction route, loading and caching it on a miss.
	GetOrCreateTransactionRouteCache(ctx context.Context, organizationID, transactionRouteID uuid.UUID) (mmodel.TransactionRouteCache, error)
}

// routeCrossLedgerBridgeLegs gives the bridge leg of every part whose ledger
// validates accounting routes the bridge route of the request's transaction
// route: the one operation route carrying a crossLedger entry. Parts in ledgers
// that do not validate routes, net-zero parts and requests naming no
// transaction route are left untouched, and nothing is read for them.
func (uc *UseCase) routeCrossLedgerBridgeLegs(ctx context.Context, transaction mtransaction.Transaction, parts []decomposedCrossLedgerPart) error {
	transactionRoute := crossLedgerTransactionRoute(transaction)
	if transactionRoute == "" {
		return nil
	}

	routed, err := uc.crossLedgerPartsRequiringBridgeRoute(ctx, parts)
	if err != nil || len(routed) == 0 {
		return err
	}

	transactionRouteID, err := uuid.Parse(transactionRoute)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidTransactionRouteID, constant.EntityTransactionRoute)
	}

	reader, ok := uc.TransactionReader.(TransactionRouteCacheReader)
	if !ok {
		return errors.New("cross-ledger transaction route reader is not configured")
	}

	primary := parts[routed[0]].ledgerRef

	cache, err := reader.GetOrCreateTransactionRouteCache(ctx, primary.organizationID, transactionRouteID)
	if err != nil {
		return err
	}

	bridgeRouteID, err := crossLedgerBridgeRouteID(cache)
	if err != nil {
		return err
	}

	for _, index := range routed {
		parts[index].assignBridgeRoute(bridgeRouteID)
	}

	return nil
}

// crossLedgerPartsRequiringBridgeRoute returns, in part order, the parts that
// carry a bridge leg in a ledger that validates accounting routes. A
// participant that has not opted into cross-ledger, and a group that spans
// organizations with route validation, are refused here exactly as the group
// execution would refuse them, so those refusals keep their precedence over the
// route lookup.
func (uc *UseCase) crossLedgerPartsRequiringBridgeRoute(ctx context.Context, parts []decomposedCrossLedgerPart) ([]int, error) {
	if uc.TransactionReader == nil {
		return nil, errors.New("cross-ledger transaction reader is not configured")
	}

	settingsByRef := make(map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings, len(parts))

	for index := range parts {
		ref := parts[index].ledgerRef
		if _, ok := settingsByRef[ref]; ok {
			continue
		}

		settings, err := uc.TransactionReader.GetParsedLedgerSettings(ctx, ref.organizationID, ref.ledgerID)
		if err != nil {
			return nil, fmt.Errorf("get cross-ledger participant settings: %w", err)
		}

		if !settings.CrossLedger.Enabled {
			return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerNotEnabled, constant.EntityLedger, ref.ledgerID.String())
		}

		settingsByRef[ref] = settings
	}

	if err := refuseCrossOrganizationRouteValidation(settingsByRef); err != nil {
		return nil, err
	}

	routed := make([]int, 0, len(parts))

	for index := range parts {
		if parts[index].bridge != nil && settingsByRef[parts[index].ledgerRef].Accounting.ValidateRoutes {
			routed = append(routed, index)
		}
	}

	return routed, nil
}

// crossLedgerBridgeRouteID returns the single operation route of the
// transaction route that carries a crossLedger entry.
func crossLedgerBridgeRouteID(cache mmodel.TransactionRouteCache) (string, error) {
	bridges := cache.Actions[constant.ActionCrossLedger]

	ids := make([]string, 0, len(bridges.Bidirectional))
	for _, routes := range []map[string]mmodel.OperationRouteCache{bridges.Source, bridges.Destination, bridges.Bidirectional} {
		for id := range routes {
			ids = append(ids, id)
		}
	}

	switch len(ids) {
	case 0:
		return "", pkg.ValidateBusinessError(constant.ErrCrossLedgerRouteNotConfigured, constant.EntityTransactionRoute)
	case 1:
		return ids[0], nil
	default:
		sort.Strings(ids)

		return "", pkg.ValidateBusinessError(constant.ErrInvalidCrossLedgerRoute, constant.EntityTransactionRoute,
			"The transaction route links more than one operation route with a crossLedger entry: "+strings.Join(ids, ", ")+".")
	}
}

// crossLedgerTransactionRoute returns the transaction route the request names,
// preferring routeId over the deprecated route field as route validation does.
func crossLedgerTransactionRoute(transaction mtransaction.Transaction) string {
	if transaction.RouteID != nil && strings.TrimSpace(*transaction.RouteID) != "" {
		return *transaction.RouteID
	}

	return strings.TrimSpace(transaction.Route) //nolint:staticcheck // route validation still honors the legacy field when routeId is absent
}

// assignBridgeRoute sets routeID on the bridge leg the decomposition appended.
func (part *decomposedCrossLedgerPart) assignBridgeRoute(routeID string) {
	if part.bridge == nil {
		return
	}

	legs := part.transaction.Send.Distribute.To
	if part.bridge.isFrom {
		legs = part.transaction.Send.Source.From
	}

	route := routeID
	legs[part.bridge.index].RouteID = &route
}
