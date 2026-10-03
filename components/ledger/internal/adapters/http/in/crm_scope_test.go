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
	"go.uber.org/mock/gomock"

	holderrepo "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	instrumentrepo "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

type holderScopeRecorder struct {
	asked pkgHTTP.ScopeConfinement
	ids   []uuid.UUID
}

func (r *holderScopeRecorder) HolderIDsInScope(_ context.Context, _ uuid.UUID, scope pkgHTTP.ScopeConfinement) ([]uuid.UUID, error) {
	r.asked = scope

	return r.ids, nil
}

func TestGetAllHolders_ConfinedToThePartnerScope(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	org, ledgerID, account, holderID := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	repo := holderrepo.NewMockRepository(ctrl)
	recorder := &holderScopeRecorder{ids: []uuid.UUID{holderID}}
	handler := &HolderHandler{Service: &services.UseCase{HolderRepo: repo, HolderScope: recorder}}

	repo.EXPECT().FindAll(gomock.Any(), org.String(), gomock.Cond(func(q pkgHTTP.QueryHeader) bool {
		return assert.Equal(t, pkgHTTP.ScopeConfinement{"holderId": {holderID}}, q.Scope)
	}), false).Return([]*mmodel.Holder{}, nil)

	ctx := partnerScopedContext(t, `,"allowed":{"ledgerId":["`+ledgerID.String()+`"],"accountId":["`+account.String()+`"]}`,
		scopeDimensionLedger, scopeDimensionAccount)

	_, err := handler.GetAllHolders(ctx, &ListHoldersRequest{OrganizationID: org.String()})
	require.NoError(t, err)
	assert.Equal(t, pkgHTTP.ScopeConfinement{"ledgerId": {ledgerID}, "accountId": {account}}, recorder.asked)
}

func TestGetAllInstruments_ConfinedToThePartnerScope(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	org, ledgerID := uuid.New(), uuid.New()

	repo := instrumentrepo.NewMockRepository(ctrl)
	handler := &InstrumentHandler{Service: &services.UseCase{InstrumentRepo: repo}}

	repo.EXPECT().FindAll(gomock.Any(), org.String(), uuid.Nil, gomock.Cond(func(q pkgHTTP.QueryHeader) bool {
		return assert.Equal(t, pkgHTTP.ScopeConfinement{"ledgerId": {ledgerID}, "accountId": {}}, q.Scope)
	}), false).Return([]*mmodel.Instrument{}, nil)

	ctx := partnerScopedContext(t, `,"allowed":{"ledgerId":["`+ledgerID.String()+`"],"accountId":[]}`,
		scopeDimensionLedger, scopeDimensionAccount)

	_, err := handler.GetAllInstruments(ctx, &ListInstrumentsRequest{OrganizationID: org.String()})
	require.NoError(t, err)
}
