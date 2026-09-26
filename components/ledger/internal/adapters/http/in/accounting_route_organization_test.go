// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// Accounting routes belong to the organization. The organization-level surface is
// published on /v2 only and must authorize exactly like the ledger-level surface it
// sits beside: same ("midaz", resource, verb) tuples, same guard chain.
//
// NOT parallel: libProblem.Install swaps a process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools; concurrent builds cross-contaminate.

// accountingRouteOp is one operation of an accounting-route surface, spelled once for
// the ledger path and once for the organization path.
type accountingRouteOp struct {
	resource    string
	action      string
	verb        string
	method      string
	ledgerPath  string
	orgPath     string
	operationID string
	body        string
}

func accountingRouteOps() []accountingRouteOp {
	const (
		trLedger = "/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes"
		trOrg    = "/organizations/{organization_id}/transaction-routes"
		orLedger = "/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes"
		orOrg    = "/organizations/{organization_id}/operation-routes"
	)

	trBody := `{"title":"Settlement","operationRoutes":["` + uuid.NewString() + `"]}`
	orBody := `{"title":"Cash in","operationType":"source"}`
	patchBody := `{"title":"Renamed"}`

	return []accountingRouteOp{
		{"transaction-routes", "create", "post", http.MethodPost, trLedger, trOrg, "createOrganizationTransactionRouteV2", trBody},
		{"transaction-routes", "list", "get", http.MethodGet, trLedger, trOrg, "listOrganizationTransactionRoutesV2", ""},
		{"transaction-routes", "getByID", "get", http.MethodGet, trLedger + "/{transaction_route_id}", trOrg + "/{transaction_route_id}", "getOrganizationTransactionRouteByIDV2", ""},
		{"transaction-routes", "update", "patch", http.MethodPatch, trLedger + "/{transaction_route_id}", trOrg + "/{transaction_route_id}", "updateOrganizationTransactionRouteV2", patchBody},
		{"transaction-routes", "delete", "delete", http.MethodDelete, trLedger + "/{transaction_route_id}", trOrg + "/{transaction_route_id}", "deleteOrganizationTransactionRouteV2", ""},
		{"operation-routes", "create", "post", http.MethodPost, orLedger, orOrg, "createOrganizationOperationRouteV2", orBody},
		{"operation-routes", "list", "get", http.MethodGet, orLedger, orOrg, "listOrganizationOperationRoutesV2", ""},
		{"operation-routes", "getByID", "get", http.MethodGet, orLedger + "/{operation_route_id}", orOrg + "/{operation_route_id}", "getOrganizationOperationRouteByIDV2", ""},
		{"operation-routes", "update", "patch", http.MethodPatch, orLedger + "/{operation_route_id}", orOrg + "/{operation_route_id}", "updateOrganizationOperationRouteV2", patchBody},
		{"operation-routes", "delete", "delete", http.MethodDelete, orLedger + "/{operation_route_id}", orOrg + "/{operation_route_id}", "deleteOrganizationOperationRouteV2", ""},
	}
}

// concretePath substitutes fixed identifiers for the Huma path parameters.
func (op accountingRouteOp) concretePath(template string, orgID, ledgerID, routeID uuid.UUID) string {
	replacer := map[string]string{
		"{organization_id}":      orgID.String(),
		"{ledger_id}":            ledgerID.String(),
		"{transaction_route_id}": routeID.String(),
		"{operation_route_id}":   routeID.String(),
	}

	out := template
	for placeholder, value := range replacer {
		out = strings.ReplaceAll(out, placeholder, value)
	}

	return "/v2" + out
}

// TestOrganizationAccountingRoutes_PublishedOnV2Only locks the contract decision: the
// organization paths are served on /v2 alone, with their own operation IDs, while the
// ledger paths stay on both contracts.
func TestOrganizationAccountingRoutes_PublishedOnV2Only(t *testing.T) {
	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range accountingRouteOps() {
		t.Run(op.resource+"/"+op.action, func(t *testing.T) {
			item, ok := paths["/v2"+op.orgPath]
			require.Truef(t, ok, "/v2 must publish %s", op.orgPath)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %s must be published", op.method, op.orgPath)
			assert.Equal(t, op.operationID, operation.OperationID)

			if v1Item, onV1 := paths["/v1"+op.orgPath]; onV1 {
				assert.Nilf(t, operationForMethod(v1Item, op.method), "/v1 must not publish %s %s", op.method, op.orgPath)
			}

			for _, prefix := range []string{"/v1", "/v2"} {
				ledgerItem, ok := paths[prefix+op.ledgerPath]
				require.Truef(t, ok, "%s must keep publishing %s", prefix, op.ledgerPath)
				assert.NotNilf(t, operationForMethod(ledgerItem, op.method), "%s %s%s must stay published", op.method, prefix, op.ledgerPath)
			}
		})
	}
}

// TestOrganizationAccountingRoutes_V1PathIsNotServed proves the served surface agrees
// with the contract: the organization path answers 404 on /v1.
func TestOrganizationAccountingRoutes_V1PathIsNotServed(t *testing.T) {
	app, _ := buildUnifiedHumaAPI()

	for _, resource := range []string{"transaction-routes", "operation-routes"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+uuid.NewString()+"/"+resource, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		_ = resp.Body.Close()

		assert.Equalf(t, http.StatusNotFound, resp.StatusCode, "POST /v1/organizations/{id}/%s must not be routed", resource)
	}
}

// accountingRouteTestHandlers wires both handlers to mocked repositories that answer
// every lookup with "not found" and every listing with an empty page, so an allowed
// request reaches the terminal and answers deterministically.
func accountingRouteTestHandlers(t *testing.T) (*TransactionRouteHandler, *OperationRouteHandler) {
	t.Helper()

	ctrl := gomock.NewController(t)

	trRepo := transactionroute.NewMockRepository(ctrl)
	orRepo := operationroute.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	trRepo.EXPECT().FindByID(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, services.ErrDatabaseItemNotFound).AnyTimes()
	trRepo.EXPECT().FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, libHTTP.CursorPagination{}, nil).AnyTimes()
	trRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, services.ErrDatabaseItemNotFound).AnyTimes()
	orRepo.EXPECT().FindByID(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, services.ErrDatabaseItemNotFound).AnyTimes()
	orRepo.EXPECT().FindByIDs(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, services.ErrDatabaseItemNotFound).AnyTimes()
	orRepo.EXPECT().FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, libHTTP.CursorPagination{}, nil).AnyTimes()
	orRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, services.ErrDatabaseItemNotFound).AnyTimes()
	orRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, services.ErrDatabaseItemNotFound).AnyTimes()

	commandUC := &command.UseCase{
		TransactionRouteRepo:    trRepo,
		OperationRouteRepo:      orRepo,
		TransactionMetadataRepo: metadataRepo,
		TransactionRedisRepo:    redisRepo,
	}
	queryUC := &query.UseCase{
		TransactionRouteRepo:    trRepo,
		OperationRouteRepo:      orRepo,
		TransactionMetadataRepo: metadataRepo,
		TransactionRedisRepo:    redisRepo,
	}

	return &TransactionRouteHandler{Command: commandUC, Query: queryUC},
		&OperationRouteHandler{Command: commandUC, Query: queryUC}
}

// mountAccountingRouteSurfaces mounts the PRODUCTION registrars of both accounting-route
// resources on one /v2 group — the ledger paths and the organization paths — the way
// MountV2 does, with the ledger's ErrorEnvelope on the app root.
func mountAccountingRouteSurfaces(t *testing.T, auth *middleware.AuthClient, trh *TransactionRouteHandler, orh *OperationRouteHandler) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	libProblem.Install()
	f.Use(ledgerMiddleware.ErrorEnvelope())

	group := f.Group("/v2")
	api := openapi.New(f, group, openapi.Config{Title: "accounting-route-authz", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(api)
	openapi.DeclareBearerAuth(api)

	RegisterTransactionRouteV2RoutesToApp(group, api, auth, trh, nil)
	RegisterOrganizationTransactionRouteV2RoutesToApp(group, api, auth, trh, nil)
	RegisterOperationRouteV2RoutesToApp(group, api, auth, orh, nil)
	RegisterOrganizationOperationRouteV2RoutesToApp(group, api, auth, orh, nil)

	return f
}

// TestOrganizationAccountingRoutes_AuthorizeLikeTheLedgerPaths is the permission parity
// gate: for every resource and verb, the organization path forwards the same
// ("midaz", resource, verb) tuple as the ledger path, is denied with the same 403 when
// the grant is missing, and reaches the same terminal answer when it is granted.
func TestOrganizationAccountingRoutes_AuthorizeLikeTheLedgerPaths(t *testing.T) {
	orgID, ledgerID, routeID := uuid.New(), uuid.New(), uuid.New()

	for _, op := range accountingRouteOps() {
		for _, granted := range []bool{false, true} {
			decision := "denied"
			if granted {
				decision = "granted"
			}

			t.Run(op.resource+"/"+op.action+"/"+decision, func(t *testing.T) {
				var call authzCall

				srv := newAuthzTupleCapture(t, &call, granted)
				defer srv.Close()

				trh, orh := accountingRouteTestHandlers(t)
				app := mountAccountingRouteSurfaces(t, &middleware.AuthClient{Address: srv.URL, Enabled: true}, trh, orh)

				statusByPath := map[string]int{}

				for _, template := range []string{op.ledgerPath, op.orgPath} {
					call = authzCall{}

					var body io.Reader
					if op.body != "" {
						body = bytes.NewBufferString(op.body)
					}

					req := httptest.NewRequest(op.method, op.concretePath(template, orgID, ledgerID, routeID), body)
					req.Header.Set("Authorization", "Bearer "+guardBearerToken(t))
					req.Header.Set("Content-Type", "application/json")

					resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
					require.NoError(t, err)

					_ = resp.Body.Close()

					statusByPath[template] = resp.StatusCode

					assert.Equalf(t, authzCall{product: midazName, resource: op.resource, action: op.verb}, call,
						"%s %s must authorize as (midaz, %s, %s)", op.method, template, op.resource, op.verb)
				}

				assert.Equalf(t, statusByPath[op.ledgerPath], statusByPath[op.orgPath],
					"the organization path must answer like the ledger path (ledger=%d, organization=%d)",
					statusByPath[op.ledgerPath], statusByPath[op.orgPath])

				if granted {
					assert.NotContainsf(t, []int{http.StatusUnauthorized, http.StatusForbidden}, statusByPath[op.orgPath],
						"a granted caller must reach the terminal")
				} else {
					assert.Equal(t, http.StatusForbidden, statusByPath[op.orgPath])
				}
			})
		}
	}
}

// organizationRouteApp mounts both organization-level registrars on /v2 with auth
// disabled, for the handler behavior tests.
func organizationRouteApp(t *testing.T, trh *TransactionRouteHandler, orh *OperationRouteHandler) *fiber.App {
	t.Helper()

	return mountAccountingRouteSurfaces(t, &middleware.AuthClient{Enabled: false}, trh, orh)
}

func doJSON(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var decoded map[string]any
	if len(raw) > 0 {
		require.NoErrorf(t, json.Unmarshal(raw, &decoded), "body: %s", string(raw))
	}

	return resp.StatusCode, decoded
}

// TestCreateOrganizationTransactionRoute_HasNoLedger proves a transaction route created
// at organization level is stored without a ledger and answers without ledgerId, while
// linking operation routes found by organization.
func TestCreateOrganizationTransactionRoute_HasNoLedger(t *testing.T) {
	ctrl := gomock.NewController(t)

	orgID := uuid.New()
	ledgerA, ledgerB := uuid.New(), uuid.New()
	sourceID, destinationID := uuid.New(), uuid.New()

	trRepo := transactionroute.NewMockRepository(ctrl)
	orRepo := operationroute.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	orRepo.EXPECT().FindByIDs(gomock.Any(), orgID, []uuid.UUID{sourceID, destinationID}).
		Return([]*mmodel.OperationRoute{
			{ID: sourceID, OrganizationID: orgID, LedgerID: &ledgerA, OperationType: "source", Title: "Source"},
			{ID: destinationID, OrganizationID: orgID, LedgerID: &ledgerB, OperationType: "destination", Title: "Destination"},
		}, nil)
	trRepo.EXPECT().Create(gomock.Any(), orgID, gomock.Nil(), gomock.Any()).
		DoAndReturn(func(_ any, org uuid.UUID, ledger *uuid.UUID, tr *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
			tr.OrganizationID = org
			tr.LedgerID = ledger

			return tr, nil
		})
	metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	trh := &TransactionRouteHandler{Command: &command.UseCase{
		TransactionRouteRepo:    trRepo,
		OperationRouteRepo:      orRepo,
		TransactionMetadataRepo: metadataRepo,
		TransactionRedisRepo:    redisRepo,
	}}

	app := organizationRouteApp(t, trh, &OperationRouteHandler{})

	body := `{"title":"Organization settlement","operationRoutes":["` + sourceID.String() + `","` + destinationID.String() + `"]}`
	status, got := doJSON(t, app, http.MethodPost, "/v2/organizations/"+orgID.String()+"/transaction-routes", body)

	require.Equalf(t, http.StatusCreated, status, "body: %v", got)
	assert.NotContains(t, got, "ledgerId", "a route created at organization level has no ledger")
	assert.Equal(t, orgID.String(), got["organizationId"])
	assert.Len(t, got["operationRoutes"], 2)
}

// TestCreateOrganizationOperationRoute_HasNoLedger proves an operation route created at
// organization level is stored without a ledger and answers without ledgerId.
func TestCreateOrganizationOperationRoute_HasNoLedger(t *testing.T) {
	ctrl := gomock.NewController(t)

	orgID := uuid.New()

	orRepo := operationroute.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	orRepo.EXPECT().Create(gomock.Any(), orgID, gomock.Nil(), gomock.Any()).
		DoAndReturn(func(_ any, org uuid.UUID, ledger *uuid.UUID, or *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			or.OrganizationID = org
			or.LedgerID = ledger

			return or, nil
		})
	metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	orh := &OperationRouteHandler{Command: &command.UseCase{
		OperationRouteRepo:      orRepo,
		TransactionMetadataRepo: metadataRepo,
	}}

	app := organizationRouteApp(t, &TransactionRouteHandler{}, orh)

	status, got := doJSON(t, app, http.MethodPost, "/v2/organizations/"+orgID.String()+"/operation-routes",
		`{"title":"Organization cash in","operationType":"source"}`)

	require.Equalf(t, http.StatusCreated, status, "body: %v", got)
	assert.NotContains(t, got, "ledgerId", "a route created at organization level has no ledger")
	assert.Equal(t, orgID.String(), got["organizationId"])
}

// TestListOrganizationAccountingRoutes_ReachEveryRouteOfTheOrganization proves the
// organization listings apply no ledger filter.
func TestListOrganizationAccountingRoutes_ReachEveryRouteOfTheOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)

	orgID := uuid.New()

	trRepo := transactionroute.NewMockRepository(ctrl)
	orRepo := operationroute.NewMockRepository(ctrl)

	trRepo.EXPECT().FindAll(gomock.Any(), orgID, gomock.Nil(), gomock.Any()).Return(nil, libHTTP.CursorPagination{}, nil)
	orRepo.EXPECT().FindAll(gomock.Any(), orgID, gomock.Nil(), gomock.Any()).Return(nil, libHTTP.CursorPagination{}, nil)

	app := organizationRouteApp(t,
		&TransactionRouteHandler{Query: &query.UseCase{TransactionRouteRepo: trRepo}},
		&OperationRouteHandler{Query: &query.UseCase{OperationRouteRepo: orRepo}})

	for _, resource := range []string{"transaction-routes", "operation-routes"} {
		status, got := doJSON(t, app, http.MethodGet, "/v2/organizations/"+orgID.String()+"/"+resource+"?limit=10", "")
		assert.Equalf(t, http.StatusOK, status, "%s body: %v", resource, got)
		assert.Containsf(t, got, "items", "%s must answer the pagination envelope", resource)
	}
}

// TestOrganizationAccountingRoutes_RejectMalformedIdentifiers proves the organization
// paths keep the UUID path validation of the guard chain.
func TestOrganizationAccountingRoutes_RejectMalformedIdentifiers(t *testing.T) {
	app := organizationRouteApp(t, &TransactionRouteHandler{}, &OperationRouteHandler{})

	for _, path := range []string{
		"/v2/organizations/not-a-uuid/transaction-routes",
		"/v2/organizations/" + uuid.NewString() + "/transaction-routes/not-a-uuid",
		"/v2/organizations/not-a-uuid/operation-routes",
		"/v2/organizations/" + uuid.NewString() + "/operation-routes/not-a-uuid",
	} {
		status, got := doJSON(t, app, http.MethodGet, path, "")
		assert.Equalf(t, http.StatusBadRequest, status, "%s body: %v", path, got)
	}
}
