// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

var (
	optionalFeeRouteID      = uuid.MustParse("0199b500-0000-7000-8000-0000000000f1")
	unlinkedRouteID         = uuid.MustParse("0199b500-0000-7000-8000-0000000000f2")
	optionalBidirectionalID = uuid.MustParse("0199b500-0000-7000-8000-0000000000f3")
)

// feeRoute is the destination route a fee credits. It has the direct, commit and
// revert entries, so it belongs to those actions' templates.
func feeRoute() mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: optionalFeeRouteID, OperationType: constant.OperationRouteTypeDestination, AccountingEntries: &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{Credit: groupRouteRubric},
		Commit: &mmodel.AccountingEntry{Credit: groupRouteRubric},
		Revert: &mmodel.AccountingEntry{Credit: groupRouteRubric},
	}}
}

func directOnlyFeeRoute() mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: optionalFeeRouteID, OperationType: constant.OperationRouteTypeDestination, AccountingEntries: &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{Credit: groupRouteRubric},
	}}
}

func optionalBidirectionalRoute() mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: optionalBidirectionalID, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{Debit: groupRouteRubric, Credit: groupRouteRubric},
	}}
}

// optionalRouteUseCase answers route validation for a ledger that validates
// routes, with the transaction route of set built from routes and the given
// links marked optional.
func optionalRouteUseCase(t *testing.T, set groupRouteSet, optional []uuid.UUID, routes ...mmodel.OperationRoute) *UseCase {
	t.Helper()

	ctrl := gomock.NewController(t)

	ledgers := ledger.NewMockRepository(ctrl)
	ledgers.EXPECT().GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]any{"accounting": map[string]any{"validateRoutes": true}}, nil).AnyTimes()

	route := &mmodel.TransactionRoute{ID: set.transactionRouteID, OperationRoutes: routes, OptionalOperationRouteIDs: optional}

	cache, err := route.ToCache().ToMsgpack()
	require.NoError(t, err)

	cacheRepo := redis.NewMockRedisRepository(ctrl)
	cacheRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(cache, nil).AnyTimes()

	return &UseCase{TransactionRedisRepo: cacheRepo, LedgerRepo: ledgers}
}

func TestValidateAccountingRules_OptionalRoutes(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	template := []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), feeRoute()}
	withOptionalBidirectional := []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), optionalBidirectionalRoute()}

	tests := []struct {
		name     string
		routes   []mmodel.OperationRoute
		optional []uuid.UUID
		action   string
		from, to []groupLeg
		want     error
	}{
		{
			name:     "an optional route no leg uses is accepted",
			routes:   template,
			optional: []uuid.UUID{optionalFeeRouteID},
			action:   constant.ActionDirect,
			from:     []groupLeg{debitLeg("@payer", set.source)},
			to:       []groupLeg{creditLeg("@payee", set.destination)},
		},
		{
			name:     "an optional route a leg uses is accepted",
			routes:   template,
			optional: []uuid.UUID{optionalFeeRouteID},
			action:   constant.ActionDirect,
			from:     []groupLeg{debitLeg("@payer", set.source)},
			to:       []groupLeg{creditLeg("@payee", set.destination), creditLeg("@fees", optionalFeeRouteID)},
		},
		{
			name:     "a missing required route is refused even when the optional one is used",
			routes:   template,
			optional: []uuid.UUID{optionalFeeRouteID},
			action:   constant.ActionDirect,
			from:     []groupLeg{debitLeg("@payer", set.source)},
			to:       []groupLeg{creditLeg("@fees", optionalFeeRouteID)},
			want:     constant.ErrAccountingRouteCountMismatch,
		},
		{
			name:     "a route outside the template in place of a required one is not found",
			routes:   template,
			optional: []uuid.UUID{optionalFeeRouteID},
			action:   constant.ActionDirect,
			from:     []groupLeg{debitLeg("@payer", set.source)},
			to:       []groupLeg{creditLeg("@payee", unlinkedRouteID)},
			want:     constant.ErrAccountingRouteNotFound,
		},
		{
			name:     "a route outside the template next to the required ones breaks the count",
			routes:   template,
			optional: []uuid.UUID{optionalFeeRouteID},
			action:   constant.ActionDirect,
			from:     []groupLeg{debitLeg("@payer", set.source)},
			to:       []groupLeg{creditLeg("@payee", set.destination), creditLeg("@other", unlinkedRouteID)},
			want:     constant.ErrAccountingRouteCountMismatch,
		},
		{
			name:     "an optional destination route on a source leg is not found",
			routes:   template,
			optional: []uuid.UUID{optionalFeeRouteID},
			action:   constant.ActionDirect,
			from:     []groupLeg{debitLeg("@payer", set.source), debitLeg("@fees", optionalFeeRouteID)},
			to:       []groupLeg{creditLeg("@payee", set.destination)},
			want:     constant.ErrAccountingRouteNotFound,
		},
		{
			name:     "a hold leaves the optional commit destination unused",
			routes:   template,
			optional: []uuid.UUID{optionalFeeRouteID},
			action:   constant.ActionHold,
			from:     []groupLeg{debitLeg("@payer", set.source)},
			to:       []groupLeg{creditLeg("@payee", set.destination)},
		},
		{
			name:     "a commit leaves the optional destination unused",
			routes:   template,
			optional: []uuid.UUID{optionalFeeRouteID},
			action:   constant.ActionCommit,
			from:     []groupLeg{debitLeg("@payer", set.source)},
			to:       []groupLeg{creditLeg("@payee", set.destination)},
		},
		{
			name:     "a revert leaves the optional route unused",
			routes:   template,
			optional: []uuid.UUID{optionalFeeRouteID},
			action:   constant.ActionRevert,
			from:     []groupLeg{debitLeg("@payer", set.source)},
			to:       []groupLeg{creditLeg("@payee", set.destination)},
		},
		{
			name:     "an optional bidirectional route used on both sides still needs a debit and a credit",
			routes:   withOptionalBidirectional,
			optional: []uuid.UUID{optionalBidirectionalID},
			action:   constant.ActionDirect,
			from:     []groupLeg{debitLeg("@payer", set.source), debitLeg("@clearing-out", optionalBidirectionalID)},
			to:       []groupLeg{creditLeg("@payee", set.destination), debitLeg("@clearing-in", optionalBidirectionalID)},
			want:     constant.ErrMissingCounterpart,
		},
		{
			name:     "an optional bidirectional route used on one side is accepted",
			routes:   withOptionalBidirectional,
			optional: []uuid.UUID{optionalBidirectionalID},
			action:   constant.ActionDirect,
			from:     []groupLeg{debitLeg("@payer", set.source), debitLeg("@clearing", optionalBidirectionalID)},
			to:       []groupLeg{creditLeg("@payee", set.destination)},
		},
		{
			name:     "an optional bidirectional route left unused is accepted",
			routes:   withOptionalBidirectional,
			optional: []uuid.UUID{optionalBidirectionalID},
			action:   constant.ActionDirect,
			from:     []groupLeg{debitLeg("@payer", set.source)},
			to:       []groupLeg{creditLeg("@payee", set.destination)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			uc := optionalRouteUseCase(t, set, tc.optional, tc.routes...)
			part := newGroupPart(set, tc.from, tc.to)

			_, err := uc.ValidateAccountingRules(context.Background(), uuid.New(), uuid.New(), part.operations, part.validate, tc.action)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireBusinessCode(t, err, tc.want)
		})
	}
}

// Optionality belongs to the link, but only counts for the actions the route
// is part of: a route optional for direct that a revert leg names is outside
// the revert template and refused exactly as if it were required.
func TestValidateAccountingRules_OptionalRouteOutsideTheActionIsRefusedAsBefore(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	routes := []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), directOnlyFeeRoute()}
	part := newGroupPart(set,
		[]groupLeg{debitLeg("@payer", set.source)},
		[]groupLeg{creditLeg("@payee", set.destination), creditLeg("@fees", optionalFeeRouteID)})

	_, required := optionalRouteUseCase(t, set, nil, routes...).
		ValidateAccountingRules(context.Background(), uuid.New(), uuid.New(), part.operations, part.validate, constant.ActionRevert)
	requireBusinessCode(t, required, constant.ErrAccountingRouteCountMismatch)

	_, optional := optionalRouteUseCase(t, set, []uuid.UUID{optionalFeeRouteID}, routes...).
		ValidateAccountingRules(context.Background(), uuid.New(), uuid.New(), part.operations, part.validate, constant.ActionRevert)
	requireBusinessCode(t, optional, constant.ErrAccountingRouteCountMismatch)
}

func TestValidateGroupAccountingRoutes_OptionalRoutes(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	bridge := set.bridgeRoute(groupRouteRubric, groupRouteRubric)

	originUses := []mmodel.AccountingRouteUse{
		routeUse("0#@alice#default", set.source, true, constant.DirectionDebit),
		routeUse("0#@external/BRL#default", set.bridge, false, constant.DirectionCredit),
	}
	destinationUses := []mmodel.AccountingRouteUse{
		routeUse("0#@external/BRL#default", set.bridge, true, constant.DirectionDebit),
		routeUse("0#@bob#default", set.destination, false, constant.DirectionCredit),
	}
	feeOnlyDestinationUses := []mmodel.AccountingRouteUse{
		routeUse("0#@external/BRL#default", set.bridge, true, constant.DirectionDebit),
		routeUse("0#@fees#default", optionalFeeRouteID, false, constant.DirectionCredit),
	}

	tests := []struct {
		name     string
		routes   []mmodel.OperationRoute
		optional []uuid.UUID
		uses     []mmodel.AccountingRouteUse
		want     error
	}{
		{
			name:     "no part uses the optional route",
			routes:   []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), feeRoute(), bridge},
			optional: []uuid.UUID{optionalFeeRouteID},
			uses:     append(append([]mmodel.AccountingRouteUse(nil), originUses...), destinationUses...),
		},
		{
			name:     "the group misses a required route",
			routes:   []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), feeRoute(), bridge},
			optional: []uuid.UUID{optionalFeeRouteID},
			uses:     append(append([]mmodel.AccountingRouteUse(nil), originUses...), feeOnlyDestinationUses...),
			want:     constant.ErrAccountingRouteCountMismatch,
		},
		{
			name:     "an optional bidirectional route used on both sides without a credit",
			routes:   []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), optionalBidirectionalRoute(), bridge},
			optional: []uuid.UUID{optionalBidirectionalID},
			uses: append(append(append([]mmodel.AccountingRouteUse(nil), originUses...), destinationUses...),
				routeUse("0#@clearing-out#default", optionalBidirectionalID, true, constant.DirectionDebit),
				routeUse("0#@clearing-in#default", optionalBidirectionalID, false, constant.DirectionDebit)),
			want: constant.ErrMissingCounterpart,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			uc := optionalRouteUseCase(t, set, tc.optional, tc.routes...)

			err := uc.ValidateGroupAccountingRoutes(context.Background(), uuid.New(), set.transactionRouteID.String(), tc.uses, constant.ActionDirect)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireBusinessCode(t, err, tc.want)
		})
	}
}
