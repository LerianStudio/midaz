// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextReservationNativeProducerRoutes(t *testing.T) {
	for _, scenario := range []string{"reserve", "admin only", "admin completion", "confirm", "unknown producer", "forged namespace", "legacy requires guard", "legacy authorized", "by-id does not downgrade", "by-id complete"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			admission := mocks.NewMockContextReserveAdmitter(ctrl)
			completion := mocks.NewMockContextReserveCompleter(ctrl)
			completionByID := mocks.NewMockContextReserveIDCompleter(ctrl)
			legacyService := mocks.NewMockReservationService(ctrl)
			bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
			purpose := seamidentity.PurposeReserve
			if scenario == "admin only" || scenario == "admin completion" {
				purpose = seamidentity.PurposeAssetAdmin
			}
			resolver, err := seamidentity.NewResolver([]seamidentity.Binding{{URI: "spiffe://example.test/ledger", IntegrationID: "producer", AssetNamespace: "origin-a", Purposes: []seamidentity.Purpose{purpose}}}, 256)
			require.NoError(t, err)
			handler, err := NewContextReservationHandler(admission, completion, completionByID, bounds, 65536, 100)
			require.NoError(t, err)
			legacy, err := NewReservationHandler(legacyService, testutil.NewDefaultMockClock())
			require.NoError(t, err)
			guard := middleware.NewAuthGuard(middleware.AuthGuardConfig{APIKeyEnabled: true, APIKey: "legacy-key"}, nil)
			app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
			problem.Install()
			routes := app.Group("/v1")
			api := openapi.New(app, routes, openapi.Config{Title: "context reserve auth", Version: "test"})
			registerReservationTransportRoutes(routes, api, tracerHumaHandlers{Guard: guard, Reservation: legacy, ContextReservation: handler, ContextReservationIdentity: resolver, ResTenantMW: func(c fiber.Ctx) error { return c.Next() }})
			uri := "spiffe://example.test/ledger"
			if scenario == "unknown producer" {
				uri = "spiffe://example.test/unknown"
			}
			endpoint, client := serveLimitAssetTLS(t, app, uri)
			raw, err := os.ReadFile("../../../../../../pkg/tracercontract/testdata/reserve_request.json")
			require.NoError(t, err)
			request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, 65536, bounds)
			require.NoError(t, err)
			path := "/v1/reservations"
			expected := http.StatusCreated
			switch scenario {
			case "reserve":
				admission.EXPECT().Execute(gomock.Any(), request).DoAndReturn(func(ctx context.Context, input tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
					identity, ok := contextutil.GetIntegrationIdentity(ctx)
					require.True(t, ok)
					require.Equal(t, "producer", identity.ID)
					return &tracercontract.ReserveResult{ContractRevision: input.ContractRevision, TransactionID: input.TransactionID, EvaluationID: testutil.MustDeterministicUUID(88901), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}, nil
				})
			case "confirm", "legacy requires guard", "legacy authorized":
				path += "/transaction/" + request.TransactionID.String() + "/confirm"
				raw = []byte(`{"contractRevision":"context-reserve-1"}`)
				expected = http.StatusOK
				if scenario == "confirm" {
					completion.EXPECT().ExecuteReport(gomock.Any(), request.TransactionID, model.OperationConfirmed).Return(&tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: request.TransactionID, Status: "CONFIRMED"}, nil)
				} else {
					raw = nil
				}
				if scenario == "legacy requires guard" {
					expected = http.StatusUnauthorized
				}
				if scenario == "legacy authorized" {
					legacyService.EXPECT().ConfirmByTransaction(gomock.Any(), request.TransactionID).Return(0, nil)
				}
			case "unknown producer", "admin only", "admin completion":
				if scenario == "admin completion" {
					path += "/transaction/" + request.TransactionID.String() + "/release"
					raw = []byte(`{"contractRevision":"context-reserve-1"}`)
				}
				expected = http.StatusForbidden
			case "forged namespace":
				request.Asset.Namespace = "forged"
				raw, err = json.Marshal(request)
				require.NoError(t, err)
				expected = http.StatusBadRequest
			case "by-id complete":
				path += "/" + request.RequestID.String() + "/confirm"
				raw = []byte(`{"contractRevision":"context-reserve-1"}`)
				expected = http.StatusOK
				evaluation := testutil.MustDeterministicUUID(88902)
				completionByID.EXPECT().Execute(gomock.Any(), request.RequestID, model.OperationConfirmed).Return(&tracercontract.ReservationCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: request.TransactionID, ReservationID: request.RequestID, Status: "CONFIRMED", EvaluationID: &evaluation}, nil)
			case "by-id does not downgrade":
				path += "/" + request.TransactionID.String() + "/confirm"
				raw = []byte(`{"contractRevision":"unsupported"}`)
				expected = http.StatusBadRequest
			}
			call, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint+path, bytes.NewReader(raw))
			require.NoError(t, err)
			call.Header.Set("Content-Type", "application/json")
			if scenario == "legacy authorized" {
				call.Header.Set("X-API-Key", "legacy-key")
			}
			response, err := client.Do(call)
			require.NoError(t, err)
			defer func() { _ = response.Body.Close() }()
			require.Equal(t, expected, response.StatusCode)
		})
	}
}
