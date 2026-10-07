// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// optionalLinkRoutes is an organization's operation routes: a source, two
// destinations, a fee destination and a cross-ledger bridge.
type optionalLinkRoutes struct {
	source, destination, fee, other, bridge uuid.UUID
}

func newOptionalLinkRoutes() optionalLinkRoutes {
	return optionalLinkRoutes{
		source:      uuid.MustParse("0199b600-0000-7000-8000-000000000001"),
		destination: uuid.MustParse("0199b600-0000-7000-8000-000000000002"),
		fee:         uuid.MustParse("0199b600-0000-7000-8000-000000000003"),
		other:       uuid.MustParse("0199b600-0000-7000-8000-000000000004"),
		bridge:      uuid.MustParse("0199b600-0000-7000-8000-000000000005"),
	}
}

func (r optionalLinkRoutes) route(id uuid.UUID) *mmodel.OperationRoute {
	switch id {
	case r.source:
		return &mmodel.OperationRoute{ID: id, OperationType: constant.OperationRouteTypeSource}
	case r.bridge:
		return bridgeOperationRoute(id)
	default:
		return &mmodel.OperationRoute{ID: id, OperationType: constant.OperationRouteTypeDestination}
	}
}

// findByIDs answers OperationRouteRepo.FindByIDs from the organization's routes.
func (r optionalLinkRoutes) findByIDs(_ context.Context, _ uuid.UUID, ids []uuid.UUID) ([]*mmodel.OperationRoute, error) {
	routes := make([]*mmodel.OperationRoute, 0, len(ids))
	for _, id := range ids {
		routes = append(routes, r.route(id))
	}

	return routes, nil
}

// current is the stored transaction route: source and destination required,
// fee optional.
func (r optionalLinkRoutes) current(id uuid.UUID) *mmodel.TransactionRoute {
	return &mmodel.TransactionRoute{
		ID:                        id,
		OperationRoutes:           []mmodel.OperationRoute{*r.route(r.source), *r.route(r.destination), *r.route(r.fee)},
		OptionalOperationRouteIDs: []uuid.UUID{r.fee},
	}
}

// noMetadataRepository answers a route that has no metadata document.
func noMetadataRepository(ctrl *gomock.Controller) *mongodb.MockRepository {
	metadata := mongodb.NewMockRepository(ctrl)
	metadata.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransactionRoute, gomock.Any()).Return(nil, nil).AnyTimes()

	return metadata
}

func requireBusinessCode(t *testing.T, err error, want error) {
	t.Helper()

	require.Error(t, err)

	var validation pkg.ValidationError
	if errors.As(err, &validation) {
		assert.Equal(t, want.Error(), validation.Code, validation.Message)
		return
	}

	var unprocessable pkg.UnprocessableOperationError
	if errors.As(err, &unprocessable) {
		assert.Equal(t, want.Error(), unprocessable.Code, unprocessable.Message)
		return
	}

	require.Failf(t, "unexpected error type", "%T: %v", err, err)
}

func TestCreateTransactionRoute_OptionalLinks(t *testing.T) {
	r := newOptionalLinkRoutes()
	organizationID, ledgerID := uuid.New(), uuid.New()

	tests := []struct {
		name               string
		required, optional []uuid.UUID
		want               error
	}{
		{name: "an optional fee route is linked as optional", required: []uuid.UUID{r.source, r.destination}, optional: []uuid.UUID{r.fee}},
		{name: "a route in both lists is refused", required: []uuid.UUID{r.source, r.destination, r.fee}, optional: []uuid.UUID{r.fee}, want: constant.ErrOperationRouteBothRequiredAndOptional},
		{name: "the only destination cannot be optional", required: []uuid.UUID{r.source}, optional: []uuid.UUID{r.destination, r.fee}, want: constant.ErrNoDestinationForAction},
		{name: "the only source cannot be optional", required: []uuid.UUID{r.destination}, optional: []uuid.UUID{r.source}, want: constant.ErrNoSourceForAction},
		{name: "a bridge route cannot be optional", required: []uuid.UUID{r.source, r.destination}, optional: []uuid.UUID{r.bridge}, want: constant.ErrInvalidCrossLedgerRoute},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			operationRoutes := operationroute.NewMockRepository(ctrl)
			transactionRoutes := transactionroute.NewMockRepository(ctrl)

			operationRoutes.EXPECT().FindByIDs(gomock.Any(), organizationID, gomock.Any()).DoAndReturn(r.findByIDs).AnyTimes()

			var created *mmodel.TransactionRoute

			if tc.want == nil {
				transactionRoutes.EXPECT().Create(gomock.Any(), organizationID, &ledgerID, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, tr *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
						created = tr
						return tr, nil
					})
			}

			uc := &UseCase{OperationRouteRepo: operationRoutes, TransactionRouteRepo: transactionRoutes}

			result, err := uc.CreateTransactionRoute(context.Background(), organizationID, &ledgerID, &mmodel.CreateTransactionRouteInput{
				Title: "Transfer", OperationRoutes: tc.required, OptionalOperationRoutes: tc.optional,
			})
			if tc.want != nil {
				requireBusinessCode(t, err, tc.want)
				return
			}

			require.NoError(t, err)
			assert.Len(t, created.OperationRoutes, 3, "every link is persisted")
			assert.True(t, created.IsOptional(r.fee))
			assert.False(t, created.IsOptional(r.source))
			assert.Equal(t, []uuid.UUID{r.fee}, result.OptionalOperationRouteIDs)
		})
	}
}

func TestUpdateTransactionRoute_OptionalLinks(t *testing.T) {
	r := newOptionalLinkRoutes()
	organizationID, transactionRouteID := uuid.New(), uuid.New()

	ids := func(values ...uuid.UUID) *[]uuid.UUID { return &values }

	tests := []struct {
		name               string
		policy             TransactionRouteLinkPolicy
		required, optional *[]uuid.UUID
		wantChanges        transactionroute.LinkChanges
		wantOptional       []uuid.UUID
		want               error
	}{
		{
			name:         "moving routes between the lists retags them in place",
			policy:       LinksMergePatchV2,
			required:     ids(r.source, r.fee),
			optional:     ids(r.destination),
			wantChanges:  transactionroute.LinkChanges{Retag: []transactionroute.OperationRouteLink{{OperationRouteID: r.destination, Optional: true}, {OperationRouteID: r.fee}}},
			wantOptional: []uuid.UUID{r.destination},
		},
		{
			name:         "sending only the required list keeps the optional links",
			policy:       LinksMergePatchV2,
			required:     ids(r.source, r.destination),
			wantOptional: []uuid.UUID{r.fee},
		},
		{
			name:     "sending only the required list with an optional route in it is refused",
			policy:   LinksMergePatchV2,
			required: ids(r.source, r.destination, r.fee),
			want:     constant.ErrOperationRouteBothRequiredAndOptional,
		},
		{
			name:        "an empty optional list removes the optional links",
			policy:      LinksMergePatchV2,
			optional:    ids(),
			wantChanges: transactionroute.LinkChanges{Remove: []uuid.UUID{r.fee}},
		},
		{
			name:     "an empty required list leaves too few links",
			policy:   LinksMergePatchV2,
			required: ids(),
			want:     constant.ErrMissingOperationRoutes,
		},
		{
			name:         "a single optional route replaces the optional links",
			policy:       LinksMergePatchV2,
			optional:     ids(r.other),
			wantChanges:  transactionroute.LinkChanges{Add: []transactionroute.OperationRouteLink{{OperationRouteID: r.other, Optional: true}}, Remove: []uuid.UUID{r.fee}},
			wantOptional: []uuid.UUID{r.other},
		},
		{
			name:     "a bridge route cannot become optional",
			policy:   LinksMergePatchV2,
			optional: ids(r.bridge),
			want:     constant.ErrInvalidCrossLedgerRoute,
		},
		{
			name:         "the full-set contract keeps the optionality of the links it keeps",
			policy:       LinksFullSetV1,
			required:     ids(r.source, r.fee, r.other),
			wantChanges:  transactionroute.LinkChanges{Add: []transactionroute.OperationRouteLink{{OperationRouteID: r.other}}, Remove: []uuid.UUID{r.destination}},
			wantOptional: []uuid.UUID{r.fee},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			operationRoutes := operationroute.NewMockRepository(ctrl)
			transactionRoutes := transactionroute.NewMockRepository(ctrl)

			transactionRoutes.EXPECT().FindByID(gomock.Any(), organizationID, transactionRouteID).Return(r.current(transactionRouteID), nil).AnyTimes()
			operationRoutes.EXPECT().FindByIDs(gomock.Any(), organizationID, gomock.Any()).DoAndReturn(r.findByIDs).AnyTimes()

			var applied *transactionroute.LinkChanges

			if tc.want == nil {
				transactionRoutes.EXPECT().Update(gomock.Any(), organizationID, transactionRouteID, gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _, id uuid.UUID, _ *mmodel.TransactionRoute, links transactionroute.LinkChanges) (*mmodel.TransactionRoute, error) {
						applied = &links
						return &mmodel.TransactionRoute{ID: id}, nil
					})
			}

			uc := &UseCase{OperationRouteRepo: operationRoutes, TransactionRouteRepo: transactionRoutes, TransactionMetadataRepo: noMetadataRepository(ctrl)}

			result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, transactionRouteID, &mmodel.UpdateTransactionRouteInput{
				OperationRoutes: tc.required, OptionalOperationRoutes: tc.optional,
			}, tc.policy)
			if tc.want != nil {
				requireBusinessCode(t, err, tc.want)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, applied)
			assert.ElementsMatch(t, tc.wantChanges.Add, applied.Add)
			assert.ElementsMatch(t, tc.wantChanges.Remove, applied.Remove)
			assert.ElementsMatch(t, tc.wantChanges.Retag, applied.Retag)
			assert.ElementsMatch(t, tc.wantOptional, result.OptionalOperationRouteIDs, "the returned route feeds the cache rewrite")
		})
	}
}

// A PATCH that leaves the links alone still returns their optionality, because
// the handler rewrites the cache from the returned route.
func TestUpdateTransactionRoute_TitleOnlyKeepsOptionalLinks(t *testing.T) {
	r := newOptionalLinkRoutes()
	organizationID, transactionRouteID := uuid.New(), uuid.New()

	ctrl := gomock.NewController(t)
	operationRoutes := operationroute.NewMockRepository(ctrl)
	transactionRoutes := transactionroute.NewMockRepository(ctrl)

	transactionRoutes.EXPECT().Update(gomock.Any(), organizationID, transactionRouteID, gomock.Any(), transactionroute.LinkChanges{}).
		Return(&mmodel.TransactionRoute{ID: transactionRouteID, Title: "Renamed"}, nil)
	transactionRoutes.EXPECT().FindOperationRouteLinksByTransactionRouteIDs(gomock.Any(), []uuid.UUID{transactionRouteID}).
		Return(map[uuid.UUID][]transactionroute.OperationRouteLink{transactionRouteID: {
			{OperationRouteID: r.source}, {OperationRouteID: r.destination}, {OperationRouteID: r.fee, Optional: true},
		}}, nil)
	operationRoutes.EXPECT().FindByIDs(gomock.Any(), organizationID, gomock.Any()).DoAndReturn(r.findByIDs)

	uc := &UseCase{OperationRouteRepo: operationRoutes, TransactionRouteRepo: transactionRoutes, TransactionMetadataRepo: noMetadataRepository(ctrl)}

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, transactionRouteID, &mmodel.UpdateTransactionRouteInput{Title: "Renamed"}, LinksMergePatchV2)
	require.NoError(t, err)

	assert.Len(t, result.OperationRoutes, 3)
	assert.Equal(t, []uuid.UUID{r.fee}, result.OptionalOperationRouteIDs)
}

// An operation route linked as optional cannot become a bridge route: the
// bridge is never a client leg, so it can never be optional.
func TestUpdateOperationRoute_BridgeEntryOnAnOptionalLinkIsRefused(t *testing.T) {
	organizationID, operationRouteID, transactionRouteID := uuid.New(), uuid.New(), uuid.New()
	patch := &mmodel.UpdateOperationRouteInput{AccountingEntries: bridgeOperationRoute(operationRouteID).AccountingEntries}

	ctrl := gomock.NewController(t)
	operationRoutes := operationroute.NewMockRepository(ctrl)
	transactionRoutes := transactionroute.NewMockRepository(ctrl)

	operationRoutes.EXPECT().FindTransactionRouteIDs(gomock.Any(), operationRouteID).Return([]uuid.UUID{transactionRouteID}, nil)
	transactionRoutes.EXPECT().FindOperationRouteLinksByTransactionRouteIDs(gomock.Any(), []uuid.UUID{transactionRouteID}).
		Return(map[uuid.UUID][]transactionroute.OperationRouteLink{transactionRouteID: {{OperationRouteID: operationRouteID, Optional: true}}}, nil)

	uc := &UseCase{OperationRouteRepo: operationRoutes, TransactionRouteRepo: transactionRoutes}

	result, err := uc.UpdateOperationRoute(context.Background(), organizationID, operationRouteID, patch)

	assert.Nil(t, result)
	requireBusinessCode(t, err, constant.ErrInvalidCrossLedgerRoute)
}

// Updating an operation route rewrites the cache of every transaction route
// that links it; the rewrite keeps the optional links optional.
func TestReloadOperationRouteCache_KeepsOptionalLinks(t *testing.T) {
	r := newOptionalLinkRoutes()
	organizationID, transactionRouteID := uuid.New(), uuid.New()

	ctrl := gomock.NewController(t)
	operationRoutes := operationroute.NewMockRepository(ctrl)
	transactionRoutes := transactionroute.NewMockRepository(ctrl)
	cache := redis.NewMockRedisRepository(ctrl)

	stored := r.current(transactionRouteID)
	stored.OrganizationID = organizationID

	for i := range stored.OperationRoutes {
		stored.OperationRoutes[i].AccountingEntries = &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}}
	}

	operationRoutes.EXPECT().FindTransactionRouteIDs(gomock.Any(), r.fee).Return([]uuid.UUID{transactionRouteID}, nil)
	transactionRoutes.EXPECT().FindByID(gomock.Any(), organizationID, transactionRouteID).Return(stored, nil)

	var written []byte

	cache.EXPECT().SetBytes(gomock.Any(), utils.AccountingRoutesInternalKey(organizationID, transactionRouteID), gomock.Any(), time.Duration(0)).
		DoAndReturn(func(_ context.Context, _ string, value []byte, _ time.Duration) error {
			written = value
			return nil
		})

	uc := &UseCase{OperationRouteRepo: operationRoutes, TransactionRouteRepo: transactionRoutes, TransactionRedisRepo: cache}

	require.NoError(t, uc.ReloadOperationRouteCache(context.Background(), organizationID, r.fee))

	var decoded mmodel.TransactionRouteCache
	require.NoError(t, decoded.FromMsgpack(written))

	assert.True(t, decoded.Actions["direct"].Destination[r.fee.String()].Optional)
	assert.False(t, decoded.Actions["direct"].Destination[r.destination.String()].Optional)
}

// Deleting a transaction route removes every link, the optional ones too, so
// the operation routes they named can be deleted afterwards.
func TestDeleteTransactionRoute_RemovesOptionalLinksToo(t *testing.T) {
	r := newOptionalLinkRoutes()
	organizationID, transactionRouteID := uuid.New(), uuid.New()

	ctrl := gomock.NewController(t)
	transactionRoutes := transactionroute.NewMockRepository(ctrl)
	cache := redis.NewMockRedisRepository(ctrl)

	stored := r.current(transactionRouteID)
	stored.OrganizationID = organizationID

	transactionRoutes.EXPECT().FindByID(gomock.Any(), organizationID, transactionRouteID).Return(stored, nil)
	transactionRoutes.EXPECT().Delete(gomock.Any(), organizationID, transactionRouteID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, toRemove []uuid.UUID) error {
			assert.ElementsMatch(t, []uuid.UUID{r.source, r.destination, r.fee}, toRemove)
			return nil
		})
	cache.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	uc := &UseCase{TransactionRouteRepo: transactionRoutes, TransactionRedisRepo: cache}

	require.NoError(t, uc.DeleteTransactionRouteByID(context.Background(), organizationID, transactionRouteID))
}
