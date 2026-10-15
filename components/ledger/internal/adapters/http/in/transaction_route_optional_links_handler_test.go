// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// optionalLinksHTTP is a transaction route linking a required source, a required
// destination and an optional fee destination, served on one contract version.
type optionalLinksHTTP struct {
	orgID, ledgerID, routeID uuid.UUID
	source, destination, fee uuid.UUID
	transactionRoutes        *transactionroute.MockRepository
	operationRoutes          *operationroute.MockRepository
	app                      *fiber.App
	version                  string
}

func newOptionalLinksHTTP(t *testing.T, version string) *optionalLinksHTTP {
	t.Helper()

	ctrl := gomock.NewController(t)
	h := &optionalLinksHTTP{
		orgID: uuid.New(), ledgerID: uuid.New(), routeID: uuid.New(),
		source: uuid.New(), destination: uuid.New(), fee: uuid.New(),
		transactionRoutes: transactionroute.NewMockRepository(ctrl),
		operationRoutes:   operationroute.NewMockRepository(ctrl),
		version:           version,
	}

	metadata := mongodb.NewMockRepository(ctrl)
	metadata.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	cache := redis.NewMockRedisRepository(ctrl)
	cache.EXPECT().SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	cache.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	h.operationRoutes.EXPECT().FindByIDs(gomock.Any(), h.orgID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, ids []uuid.UUID) ([]*mmodel.OperationRoute, error) {
			routes := make([]*mmodel.OperationRoute, 0, len(ids))
			for _, id := range ids {
				operationType := "destination"
				if id == h.source {
					operationType = "source"
				}

				routes = append(routes, &mmodel.OperationRoute{ID: id, OperationType: operationType})
			}

			return routes, nil
		}).AnyTimes()

	handler := &TransactionRouteHandler{
		Command: &command.UseCase{TransactionRouteRepo: h.transactionRoutes, OperationRouteRepo: h.operationRoutes, TransactionMetadataRepo: metadata, TransactionRedisRepo: cache},
		Query:   &query.UseCase{TransactionRouteRepo: h.transactionRoutes, OperationRouteRepo: h.operationRoutes, TransactionMetadataRepo: metadata, TransactionRedisRepo: cache},
	}

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	libProblem.Install()
	f.Use(ledgerMiddleware.ErrorEnvelope())

	group := f.Group("/" + version)
	hAPI := openapi.New(f, group, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/" + version}})

	parse := pkgHTTP.ParseUUIDPathParameters("transaction_route")
	base := "/organizations/:organization_id/ledgers/:ledger_id/transaction-routes"
	group.Post(base, parse)
	group.Get(base, parse)
	group.Get(base+"/:transaction_route_id", parse)
	group.Patch(base+"/:transaction_route_id", parse)

	suffix := v1OpSuffix
	if version == "v2" {
		suffix = v2OpSuffix
	}

	RegisterTransactionRouteRoutes(hAPI, handler, suffix)

	h.app = f

	return h
}

// stored is the transaction route the repository holds.
func (h *optionalLinksHTTP) stored() *mmodel.TransactionRoute {
	return &mmodel.TransactionRoute{
		ID: h.routeID, OrganizationID: h.orgID, LedgerID: &h.ledgerID, Title: "Transfer",
		OperationRoutes: []mmodel.OperationRoute{
			{ID: h.source, OperationType: "source"}, {ID: h.destination, OperationType: "destination"}, {ID: h.fee, OperationType: "destination"},
		},
		OptionalOperationRouteIDs: []uuid.UUID{h.fee},
	}
}

func (h *optionalLinksHTTP) do(t *testing.T, method, suffix, body string) (int, map[string]any) {
	t.Helper()

	path := "/" + h.version + "/organizations/" + h.orgID.String() + "/ledgers/" + h.ledgerID.String() + "/transaction-routes" + suffix

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var got map[string]any
	if len(raw) > 0 {
		require.NoErrorf(t, json.Unmarshal(raw, &got), "body: %s", string(raw))
	}

	return resp.StatusCode, got
}

func linkedIDs(t *testing.T, value any) []string {
	t.Helper()

	items, ok := value.([]any)
	require.Truef(t, ok, "expected a list of operation routes, got %T", value)

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.(map[string]any)["id"].(string))
	}

	return ids
}

// /v1 does not know optionalOperationRoutes: a value for it is an unknown field.
func TestTransactionRouteV1_RefusesOptionalLinks(t *testing.T) {
	h := newOptionalLinksHTTP(t, "v1")
	body := `{"title":"Transfer","operationRoutes":["` + h.source.String() + `","` + h.destination.String() + `"],"optionalOperationRoutes":["` + h.fee.String() + `"]}`

	status, got := h.do(t, http.MethodPost, "", body)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "0053", got["code"])

	status, got = h.do(t, http.MethodPatch, "/"+h.routeID.String(), `{"optionalOperationRoutes":["`+h.fee.String()+`"]}`)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "0053", got["code"])
}

// /v1 keeps listing every link in operationRoutes, so a /v1 client that reads
// and sends the list back does not drop the optional links it cannot see.
func TestTransactionRouteV1_ReadListsOptionalLinksAsOperationRoutes(t *testing.T) {
	h := newOptionalLinksHTTP(t, "v1")
	h.transactionRoutes.EXPECT().FindByID(gomock.Any(), h.orgID, h.routeID).Return(h.stored(), nil)

	status, got := h.do(t, http.MethodGet, "/"+h.routeID.String(), "")
	require.Equal(t, http.StatusOK, status)

	assert.ElementsMatch(t, []string{h.source.String(), h.destination.String(), h.fee.String()}, linkedIDs(t, got["operationRoutes"]))
	assert.NotContains(t, got, "optionalOperationRoutes")
}

func TestTransactionRouteV2_CreateTakesAndAnswersBothLists(t *testing.T) {
	h := newOptionalLinksHTTP(t, "v2")

	var created *mmodel.TransactionRoute

	h.transactionRoutes.EXPECT().Create(gomock.Any(), h.orgID, &h.ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, tr *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
			created = tr
			return tr, nil
		})

	status, got := h.do(t, http.MethodPost, "", `{"title":"Transfer","operationRoutes":["`+h.source.String()+`","`+h.destination.String()+`"],"optionalOperationRoutes":["`+h.fee.String()+`"]}`)
	require.Equalf(t, http.StatusCreated, status, "body: %v", got)

	assert.Equal(t, []uuid.UUID{h.fee}, created.OptionalOperationRouteIDs)
	assert.ElementsMatch(t, []string{h.source.String(), h.destination.String()}, linkedIDs(t, got["operationRoutes"]))
	assert.Equal(t, []string{h.fee.String()}, linkedIDs(t, got["optionalOperationRoutes"]))
}

// An empty optional list is a list, not an unknown field.
func TestTransactionRouteV2_CreateAcceptsAnEmptyOptionalList(t *testing.T) {
	h := newOptionalLinksHTTP(t, "v2")
	h.transactionRoutes.EXPECT().Create(gomock.Any(), h.orgID, &h.ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, tr *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
			return tr, nil
		})

	status, got := h.do(t, http.MethodPost, "", `{"title":"Transfer","operationRoutes":["`+h.source.String()+`","`+h.destination.String()+`"],"optionalOperationRoutes":[]}`)
	require.Equalf(t, http.StatusCreated, status, "body: %v", got)
	assert.Empty(t, linkedIDs(t, got["optionalOperationRoutes"]))
}

func TestTransactionRouteV2_ReadSplitsTheLinks(t *testing.T) {
	h := newOptionalLinksHTTP(t, "v2")
	h.transactionRoutes.EXPECT().FindByID(gomock.Any(), h.orgID, h.routeID).Return(h.stored(), nil)

	status, got := h.do(t, http.MethodGet, "/"+h.routeID.String(), "")
	require.Equal(t, http.StatusOK, status)

	assert.ElementsMatch(t, []string{h.source.String(), h.destination.String()}, linkedIDs(t, got["operationRoutes"]))
	assert.Equal(t, []string{h.fee.String()}, linkedIDs(t, got["optionalOperationRoutes"]))
}

// Each list of a /v2 PATCH is a merge patch of its own: an empty list removes
// the links, null and absence keep them.
func TestTransactionRouteV2_PatchAppliesEachListAsAMergePatch(t *testing.T) {
	tests := []struct {
		name               string
		body               string
		removesOptional    bool
		movesFeeToRequired bool
	}{
		{name: "an empty optional list removes the optional links", body: `{"optionalOperationRoutes":[]}`, removesOptional: true},
		{name: "a null optional list keeps them", body: `{"optionalOperationRoutes":null}`},
		{name: "a null required list keeps them", body: `{"operationRoutes":null}`},
		{name: "both lists move the fee route to the required ones", body: "", movesFeeToRequired: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newOptionalLinksHTTP(t, "v2")

			var want transactionroute.LinkChanges

			body := tc.body

			if tc.movesFeeToRequired {
				body = `{"operationRoutes":["` + h.source.String() + `","` + h.destination.String() + `","` + h.fee.String() + `"],"optionalOperationRoutes":[]}`
				want = transactionroute.LinkChanges{Retag: []transactionroute.OperationRouteLink{{OperationRouteID: h.fee}}}

				h.transactionRoutes.EXPECT().FindByID(gomock.Any(), h.orgID, h.routeID).Return(h.stored(), nil)
			} else if tc.removesOptional {
				want = transactionroute.LinkChanges{Remove: []uuid.UUID{h.fee}}

				h.transactionRoutes.EXPECT().FindByID(gomock.Any(), h.orgID, h.routeID).Return(h.stored(), nil)
			} else {
				h.transactionRoutes.EXPECT().FindOperationRouteLinksByTransactionRouteIDs(gomock.Any(), []uuid.UUID{h.routeID}).
					Return(map[uuid.UUID][]transactionroute.OperationRouteLink{h.routeID: {
						{OperationRouteID: h.source}, {OperationRouteID: h.destination}, {OperationRouteID: h.fee, Optional: true},
					}}, nil)
			}

			h.transactionRoutes.EXPECT().Update(gomock.Any(), h.orgID, h.routeID, gomock.Any(), want).
				Return(&mmodel.TransactionRoute{ID: h.routeID, OrganizationID: h.orgID}, nil)

			status, got := h.do(t, http.MethodPatch, "/"+h.routeID.String(), body)
			require.Equalf(t, http.StatusOK, status, "body: %v", got)
		})
	}
}
