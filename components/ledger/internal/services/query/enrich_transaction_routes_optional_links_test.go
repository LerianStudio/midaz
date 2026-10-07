// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// The listing hydrates each transaction route with every linked operation
// route and names the optional ones per transaction route: the same operation
// route can be optional in one and required in another.
func TestEnrichTransactionRoutes_CarriesOptionalLinksPerTransactionRoute(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	organizationID := uuid.New()
	withOptionalFee, requiredOnly := uuid.New(), uuid.New()
	source, fee := uuid.New(), uuid.New()

	transactionRoutes := transactionroute.NewMockRepository(ctrl)
	transactionRoutes.EXPECT().
		FindOperationRouteLinksByTransactionRouteIDs(gomock.Any(), gomock.InAnyOrder([]uuid.UUID{withOptionalFee, requiredOnly})).
		Return(map[uuid.UUID][]transactionroute.OperationRouteLink{
			withOptionalFee: {{OperationRouteID: source}, {OperationRouteID: fee, Optional: true}},
			requiredOnly:    {{OperationRouteID: source}, {OperationRouteID: fee}},
		}, nil)

	operationRoutes := operationroute.NewMockRepository(ctrl)
	operationRoutes.EXPECT().
		FindByIDs(gomock.Any(), organizationID, gomock.InAnyOrder([]uuid.UUID{source, fee})).
		Return([]*mmodel.OperationRoute{{ID: source, OperationType: "source"}, {ID: fee, OperationType: "destination"}}, nil)

	uc := &UseCase{TransactionRouteRepo: transactionRoutes, OperationRouteRepo: operationRoutes}

	routes := []*mmodel.TransactionRoute{
		{ID: withOptionalFee, OrganizationID: organizationID},
		{ID: requiredOnly, OrganizationID: organizationID},
	}

	require.NoError(t, uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), routes))

	assert.Len(t, routes[0].OperationRoutes, 2)
	assert.Equal(t, []uuid.UUID{fee}, routes[0].OptionalOperationRouteIDs)
	assert.Len(t, routes[1].OperationRoutes, 2)
	assert.Empty(t, routes[1].OptionalOperationRouteIDs)
}
