// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

var groupRouteRubric = &mmodel.AccountingRubric{Code: "1000", Description: "Rubric"}

// groupRouteSet is a transaction route of client routes plus the cross-ledger
// bridge route, which carries only its crossLedger entry.
type groupRouteSet struct {
	transactionRouteID uuid.UUID
	source             uuid.UUID
	secondSource       uuid.UUID
	destination        uuid.UUID
	bidirectional      uuid.UUID
	bridge             uuid.UUID
}

func newGroupRouteSet() groupRouteSet {
	return groupRouteSet{
		transactionRouteID: uuid.MustParse("0199b500-0000-7000-8000-000000000001"),
		source:             uuid.MustParse("0199b500-0000-7000-8000-000000000002"),
		secondSource:       uuid.MustParse("0199b500-0000-7000-8000-000000000003"),
		destination:        uuid.MustParse("0199b500-0000-7000-8000-000000000004"),
		bidirectional:      uuid.MustParse("0199b500-0000-7000-8000-000000000005"),
		bridge:             uuid.MustParse("0199b500-0000-7000-8000-000000000006"),
	}
}

func (set groupRouteSet) sourceRoute() mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: set.source, OperationType: constant.OperationRouteTypeSource, AccountingEntries: &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{Debit: groupRouteRubric},
		Hold:   &mmodel.AccountingEntry{Debit: groupRouteRubric, Credit: groupRouteRubric},
		Commit: &mmodel.AccountingEntry{Debit: groupRouteRubric},
		Cancel: &mmodel.AccountingEntry{Debit: groupRouteRubric, Credit: groupRouteRubric},
		Revert: &mmodel.AccountingEntry{Debit: groupRouteRubric},
	}}
}

func (set groupRouteSet) secondSourceRoute() mmodel.OperationRoute {
	route := set.sourceRoute()
	route.ID = set.secondSource

	return route
}

func (set groupRouteSet) destinationRoute() mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: set.destination, OperationType: constant.OperationRouteTypeDestination, AccountingEntries: &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{Credit: groupRouteRubric},
		Commit: &mmodel.AccountingEntry{Credit: groupRouteRubric},
		Revert: &mmodel.AccountingEntry{Credit: groupRouteRubric},
	}}
}

// commitOnlyDestinationRoute is a destination route that only exists for the
// commit of a pending transaction.
func (set groupRouteSet) commitOnlyDestinationRoute() mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: set.destination, OperationType: constant.OperationRouteTypeDestination, AccountingEntries: &mmodel.AccountingEntries{
		Commit: &mmodel.AccountingEntry{Credit: groupRouteRubric},
	}}
}

func (set groupRouteSet) bidirectionalRoute() mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: set.bidirectional, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{Debit: groupRouteRubric, Credit: groupRouteRubric},
	}}
}

func (set groupRouteSet) bridgeRoute(debit, credit *mmodel.AccountingRubric) mmodel.OperationRoute {
	return mmodel.OperationRoute{ID: set.bridge, OperationType: constant.OperationRouteTypeBidirectional, AccountingEntries: &mmodel.AccountingEntries{
		CrossLedger: &mmodel.AccountingEntry{Debit: debit, Credit: credit},
	}}
}

// groupRouteUseCase answers route validation with validateRoutes as given and
// the transaction route of set built from operationRoutes.
func groupRouteUseCase(t *testing.T, set groupRouteSet, validateRoutes bool, operationRoutes ...mmodel.OperationRoute) *UseCase {
	t.Helper()

	ctrl := gomock.NewController(t)

	ledgers := ledger.NewMockRepository(ctrl)
	ledgers.EXPECT().GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]any{"accounting": map[string]any{"validateRoutes": validateRoutes}}, nil).AnyTimes()

	cache, err := (&mmodel.TransactionRoute{ID: set.transactionRouteID, OperationRoutes: operationRoutes}).ToCache().ToMsgpack()
	require.NoError(t, err)

	cacheRepo := redis.NewMockRedisRepository(ctrl)
	if validateRoutes {
		cacheRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(cache, nil).AnyTimes()
	}

	return &UseCase{TransactionRedisRepo: cacheRepo, LedgerRepo: ledgers}
}

// groupPart is one ledger part as the per-part validation sees it: its
// operations and the validated legs with their operation routes.
type groupPart struct {
	operations []mmodel.BalanceOperation
	validate   *mtransaction.Responses
}

type groupLeg struct {
	alias     string
	routeID   uuid.UUID
	operation string
	direction string
}

func newGroupPart(set groupRouteSet, from, to []groupLeg) groupPart {
	routeID := set.transactionRouteID.String()
	part := groupPart{validate: &mtransaction.Responses{
		TransactionRouteID:  &routeID,
		From:                map[string]mtransaction.Amount{},
		To:                  map[string]mtransaction.Amount{},
		OperationRoutesFrom: map[string]string{},
		OperationRoutesTo:   map[string]string{},
	}}

	for _, leg := range from {
		amount := mtransaction.Amount{Operation: leg.operation, Direction: leg.direction}
		part.validate.From[leg.alias] = amount
		part.validate.OperationRoutesFrom[leg.alias] = leg.route()
		part.operations = append(part.operations, mmodel.BalanceOperation{Alias: leg.alias, Balance: &mmodel.Balance{AccountType: "deposit"}, Amount: amount})
	}

	for _, leg := range to {
		amount := mtransaction.Amount{Operation: leg.operation, Direction: leg.direction}
		part.validate.To[leg.alias] = amount
		part.validate.OperationRoutesTo[leg.alias] = leg.route()
		part.operations = append(part.operations, mmodel.BalanceOperation{Alias: leg.alias, Balance: &mmodel.Balance{AccountType: "deposit"}, Amount: amount})
	}

	return part
}

// route is the leg's operation route ID, empty for a leg that names none.
func (leg groupLeg) route() string {
	if leg.routeID == uuid.Nil {
		return ""
	}

	return leg.routeID.String()
}

func debitLeg(alias string, routeID uuid.UUID) groupLeg {
	return groupLeg{alias: alias, routeID: routeID, operation: constant.DEBIT, direction: constant.DirectionDebit}
}

func creditLeg(alias string, routeID uuid.UUID) groupLeg {
	return groupLeg{alias: alias, routeID: routeID, operation: constant.CREDIT, direction: constant.DirectionCredit}
}

func requireBusinessCode(t *testing.T, err error, want error) {
	t.Helper()

	require.Error(t, err)

	var unprocessable pkg.UnprocessableOperationError
	if errors.As(err, &unprocessable) {
		assert.Equal(t, want.Error(), unprocessable.Code, unprocessable.Message)
		return
	}

	var validation pkg.ValidationError
	if errors.As(err, &validation) {
		assert.Equal(t, want.Error(), validation.Code, validation.Message)
		return
	}

	require.Failf(t, "unexpected error type", "%T: %v", err, err)
}

// The origin part of a direct group moves the client source and credits its
// bridge. Validated alone, as a single-ledger transaction, it cannot satisfy the
// template; validated as a group part it passes, because the template is
// checked over the whole group and the bridge is checked by its own rule.
func TestValidateGroupPartAccountingRules_ValidatesTheOriginPartLegByLeg(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	routes := []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), set.bridgeRoute(groupRouteRubric, groupRouteRubric)}
	origin := newGroupPart(set,
		[]groupLeg{debitLeg("0#@alice#default", set.source)},
		[]groupLeg{creditLeg("0#@external/BRL#default", set.bridge)})

	uc := groupRouteUseCase(t, set, true, routes...)

	cache, err := uc.ValidateGroupPartAccountingRules(context.Background(), uuid.New(), uuid.New(), origin.operations, origin.validate, constant.ActionDirect)
	require.NoError(t, err)
	require.NotNil(t, cache, "a validated part returns the route cache used for its rubrics")

	_, singleLedgerErr := uc.ValidateAccountingRules(context.Background(), uuid.New(), uuid.New(), origin.operations, origin.validate, constant.ActionDirect)
	require.Error(t, singleLedgerErr, "the same part validated as a single-ledger transaction keeps today's checks")
}

func TestValidateGroupPartAccountingRules_BridgeRubricFollowsThePostedDirection(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	origin := newGroupPart(set,
		[]groupLeg{debitLeg("0#@alice#default", set.source)},
		[]groupLeg{creditLeg("0#@external/BRL#default", set.bridge)})
	destination := newGroupPart(set,
		[]groupLeg{debitLeg("0#@external/BRL#default", set.bridge)},
		[]groupLeg{creditLeg("0#@bob#default", set.destination)})

	tests := []struct {
		name   string
		part   groupPart
		debit  *mmodel.AccountingRubric
		credit *mmodel.AccountingRubric
		want   error
	}{
		{name: "origin credits the bridge with the credit rubric", part: origin, credit: groupRouteRubric},
		{name: "origin without a credit rubric", part: origin, debit: groupRouteRubric, want: constant.ErrCrossLedgerRouteNotConfigured},
		{name: "origin with an empty credit rubric code", part: origin, debit: groupRouteRubric, credit: &mmodel.AccountingRubric{}, want: constant.ErrCrossLedgerRouteNotConfigured},
		{name: "destination debits the bridge with the debit rubric", part: destination, debit: groupRouteRubric},
		{name: "destination without a debit rubric", part: destination, credit: groupRouteRubric, want: constant.ErrCrossLedgerRouteNotConfigured},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			uc := groupRouteUseCase(t, set, true, set.sourceRoute(), set.destinationRoute(), set.bridgeRoute(tc.debit, tc.credit))

			_, err := uc.ValidateGroupPartAccountingRules(context.Background(), uuid.New(), uuid.New(), tc.part.operations, tc.part.validate, constant.ActionDirect)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireBusinessCode(t, err, tc.want)
		})
	}
}

// Everything a part is checked on today, other than the count and the
// counterparts, still applies to its client legs.
func TestValidateGroupPartAccountingRules_KeepsThePerLegChecksOfClientLegs(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	unknownRoute := uuid.MustParse("0199b500-0000-7000-8000-0000000000ff")
	routes := []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), set.bridgeRoute(groupRouteRubric, groupRouteRubric)}

	tests := []struct {
		name string
		part groupPart
		want error
	}{
		{
			name: "client route outside the template",
			part: newGroupPart(set,
				[]groupLeg{debitLeg("0#@alice#default", unknownRoute)},
				[]groupLeg{creditLeg("0#@external/BRL#default", set.bridge)}),
			want: constant.ErrAccountingRouteNotFound,
		},
		{
			name: "client leg without a route",
			part: newGroupPart(set,
				[]groupLeg{{alias: "0#@alice#default", operation: constant.DEBIT, direction: constant.DirectionDebit}},
				[]groupLeg{creditLeg("0#@external/BRL#default", set.bridge)}),
			want: constant.ErrAccountingRouteNotFound,
		},
		{
			name: "client source route credited",
			part: newGroupPart(set,
				[]groupLeg{debitLeg("0#@external/BRL#default", set.bridge)},
				[]groupLeg{creditLeg("0#@bob#default", set.source)}),
			want: constant.ErrAccountingRouteNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			uc := groupRouteUseCase(t, set, true, routes...)

			_, err := uc.ValidateGroupPartAccountingRules(context.Background(), uuid.New(), uuid.New(), tc.part.operations, tc.part.validate, constant.ActionDirect)
			requireBusinessCode(t, err, tc.want)
		})
	}
}

func TestValidateGroupPartAccountingRules_LedgerWithoutRouteValidationIsSkipped(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	unrouted := newGroupPart(set,
		[]groupLeg{{alias: "0#@alice#default", operation: constant.DEBIT, direction: constant.DirectionDebit}},
		[]groupLeg{{alias: "0#@external/BRL#default", operation: constant.CREDIT, direction: constant.DirectionCredit}})

	uc := groupRouteUseCase(t, set, false)

	cache, err := uc.ValidateGroupPartAccountingRules(context.Background(), uuid.New(), uuid.New(), unrouted.operations, unrouted.validate, constant.ActionDirect)
	require.NoError(t, err)
	assert.Nil(t, cache)
}

func routeUse(alias string, routeID uuid.UUID, source bool, direction string) mmodel.AccountingRouteUse {
	return mmodel.AccountingRouteUse{Alias: alias, RouteID: routeID.String(), Source: source, Direction: direction}
}

func TestValidateGroupAccountingRoutes_CoverageOverTheGroup(t *testing.T) {
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

	tests := []struct {
		name   string
		routes []mmodel.OperationRoute
		uses   []mmodel.AccountingRouteUse
		action string
		want   error
	}{
		{
			name:   "the parts together use the whole template and the bridge is not counted",
			routes: []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), bridge},
			uses:   append(append([]mmodel.AccountingRouteUse(nil), originUses...), destinationUses...),
			action: constant.ActionDirect,
		},
		{
			name:   "one part alone does not cover the template",
			routes: []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), bridge},
			uses:   originUses,
			action: constant.ActionDirect,
			want:   constant.ErrAccountingRouteCountMismatch,
		},
		{
			name:   "a template route no part uses",
			routes: []mmodel.OperationRoute{set.sourceRoute(), set.secondSourceRoute(), set.destinationRoute(), bridge},
			uses:   append(append([]mmodel.AccountingRouteUse(nil), originUses...), destinationUses...),
			action: constant.ActionDirect,
			want:   constant.ErrAccountingRouteCountMismatch,
		},
		{
			name:   "a leg route outside the template",
			routes: []mmodel.OperationRoute{set.sourceRoute(), bridge},
			uses:   append(append([]mmodel.AccountingRouteUse(nil), originUses...), destinationUses...),
			action: constant.ActionDirect,
			want:   constant.ErrAccountingRouteNotFound,
		},
		{
			name:   "a leg without a route",
			routes: []mmodel.OperationRoute{set.sourceRoute(), set.destinationRoute(), bridge},
			uses: append(append([]mmodel.AccountingRouteUse(nil), originUses...),
				mmodel.AccountingRouteUse{Alias: "0#@bob#default", Direction: constant.DirectionCredit}),
			action: constant.ActionDirect,
			want:   constant.ErrAccountingRouteNotFound,
		},
		{
			name:   "cancel is source-only and has no group-wide rule",
			routes: []mmodel.OperationRoute{set.sourceRoute(), set.secondSourceRoute(), bridge},
			uses:   originUses[:1],
			action: constant.ActionCancel,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			uc := groupRouteUseCase(t, set, true, tc.routes...)

			err := uc.ValidateGroupAccountingRoutes(context.Background(), uuid.New(), set.transactionRouteID.String(), tc.uses, tc.action)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireBusinessCode(t, err, tc.want)
		})
	}
}

// A bidirectional client route used on both sides across the parts needs a
// debit and a credit somewhere in the group.
func TestValidateGroupAccountingRoutes_CounterpartsAcrossTheGroup(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	routes := []mmodel.OperationRoute{set.bidirectionalRoute(), set.bridgeRoute(groupRouteRubric, groupRouteRubric)}

	tests := []struct {
		name string
		uses []mmodel.AccountingRouteUse
		want error
	}{
		{
			name: "debited in one part and credited in the other",
			uses: []mmodel.AccountingRouteUse{
				routeUse("0#@alice#default", set.bidirectional, true, constant.DirectionDebit),
				routeUse("0#@external/BRL#default", set.bridge, false, constant.DirectionCredit),
				routeUse("0#@external/BRL#default", set.bridge, true, constant.DirectionDebit),
				routeUse("0#@bob#default", set.bidirectional, false, constant.DirectionCredit),
			},
		},
		{
			name: "only debited across the group",
			uses: []mmodel.AccountingRouteUse{
				routeUse("0#@alice#default", set.bidirectional, true, constant.DirectionDebit),
				routeUse("0#@bob#default", set.bidirectional, false, constant.DirectionDebit),
			},
			want: constant.ErrMissingCounterpart,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			uc := groupRouteUseCase(t, set, true, routes...)

			err := uc.ValidateGroupAccountingRoutes(context.Background(), uuid.New(), set.transactionRouteID.String(), tc.uses, constant.ActionDirect)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireBusinessCode(t, err, tc.want)
		})
	}
}

// At hold, a destination part is not executed yet: its client routes come from
// the persisted intent and are checked against the destination side of the hold
// template, which is the commit's.
func TestValidateGroupAccountingRoutes_HoldTakesTheDestinationSideFromCommit(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	uses := []mmodel.AccountingRouteUse{
		routeUse("0#@alice#default", set.source, true, constant.DirectionDebit),
		routeUse("0#@alice#default", set.source, true, constant.DirectionCredit),
		routeUse("0#@external/BRL#default", set.bridge, false, constant.DirectionCredit),
		routeUse("@external/BRL", set.bridge, true, constant.DirectionDebit),
		routeUse("@bob", set.destination, false, constant.DirectionCredit),
	}

	t.Run("destination route configured for commit", func(t *testing.T) {
		t.Parallel()

		uc := groupRouteUseCase(t, set, true, set.sourceRoute(), set.commitOnlyDestinationRoute(), set.bridgeRoute(groupRouteRubric, groupRouteRubric))

		require.NoError(t, uc.ValidateGroupAccountingRoutes(context.Background(), uuid.New(), set.transactionRouteID.String(), uses, constant.ActionHold))
	})

	t.Run("destination route without a commit entry", func(t *testing.T) {
		t.Parallel()

		destination := set.destinationRoute()
		destination.AccountingEntries.Commit = nil
		uc := groupRouteUseCase(t, set, true, set.sourceRoute(), destination, set.bridgeRoute(groupRouteRubric, groupRouteRubric))

		err := uc.ValidateGroupAccountingRoutes(context.Background(), uuid.New(), set.transactionRouteID.String(), uses, constant.ActionHold)
		requireBusinessCode(t, err, constant.ErrAccountingRouteNotFound)
	})
}

func TestValidateGroupAccountingRoutes_RequiresTheTransactionRoute(t *testing.T) {
	t.Parallel()

	set := newGroupRouteSet()
	uc := groupRouteUseCase(t, set, false)

	err := uc.ValidateGroupAccountingRoutes(context.Background(), uuid.New(), "", nil, constant.ActionDirect)
	requireBusinessCode(t, err, constant.ErrTransactionRouteNotInformed)

	err = uc.ValidateGroupAccountingRoutes(context.Background(), uuid.New(), "not-a-uuid", nil, constant.ActionDirect)
	requireBusinessCode(t, err, constant.ErrInvalidTransactionRouteID)
}
