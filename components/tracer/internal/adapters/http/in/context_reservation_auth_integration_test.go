// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextReservationNativeProducerRoutes(t *testing.T) {
	for _, scenario := range []string{"reserve", "confirm", "unknown producer", "unbound completion", "legacy asset reference", "legacy reserve body", "absent completion body", "by-id does not downgrade", "by-id complete"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			admission := mocks.NewMockContextReserveAdmitter(ctrl)
			completion := mocks.NewMockContextReserveCompleter(ctrl)
			completionByID := mocks.NewMockContextReserveIDCompleter(ctrl)
			bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
			guard := producerAuthGuard(startAccessManagerFake(t), false)
			handler, err := NewContextReservationHandler(admission, completion, completionByID, bounds, 65536, 100)
			require.NoError(t, err)
			app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
			problem.Install()
			routes := app.Group("/v1")
			api := openapi.New(app, routes, openapi.Config{Title: "context reserve auth", Version: "test"})
			registerReservationTransportRoutes(routes, api, tracerHumaHandlers{ContextReservation: handler, Guard: guard, ProducerAuth: NewProducerAuthMiddleware(guard, testProducerRegistry(t)), ReservationTenant: []fiber.Handler{func(c fiber.Ctx) error { return c.Next() }}})
			token := producerToken(t, nil)
			if scenario == "unknown producer" || scenario == "unbound completion" {
				token = producerToken(t, func(claims jwt.MapClaims) { claims["azp"] = "unknown-producer" })
			}
			endpoint, client := serveProducerTLS(t, app)
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
					require.Equal(t, "ledger", identity.ID)
					return &tracercontract.ReserveResult{ContractRevision: input.ContractRevision, TransactionID: input.TransactionID, EvaluationID: testutil.MustDeterministicUUID(88901), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}, nil
				})
			case "confirm":
				path += "/transaction/" + request.TransactionID.String() + "/confirm"
				raw = []byte(`{"contractRevision":"context-reserve-1"}`)
				expected = http.StatusOK
				completion.EXPECT().ExecuteReport(gomock.Any(), request.TransactionID, model.OperationConfirmed).Return(&tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: request.TransactionID, Status: "CONFIRMED"}, nil)
			case "absent completion body":
				path += "/transaction/" + request.TransactionID.String() + "/confirm"
				raw = nil
				expected = http.StatusBadRequest
			case "legacy reserve body":
				raw = []byte(characterizationLegacyReserveBody)
				expected = http.StatusBadRequest
			case "unknown producer", "unbound completion":
				if scenario == "unbound completion" {
					path += "/transaction/" + request.TransactionID.String() + "/release"
					raw = []byte(`{"contractRevision":"context-reserve-1"}`)
				}
				expected = http.StatusForbidden
			case "legacy asset reference":
				var document map[string]any
				require.NoError(t, json.Unmarshal(raw, &document))
				document["asset"] = map[string]any{"namespace": "forged", "id": "asset", "code": "BTC"}
				raw, err = json.Marshal(document)
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
			call.Header.Set("Authorization", "Bearer "+token)
			response, err := client.Do(call)
			require.NoError(t, err)
			defer func() { _ = response.Body.Close() }()
			require.Equal(t, expected, response.StatusCode)
		})
	}
}

// serveProducerTLS serves app over HTTPS the way the tracer HTTP listener does
// in mtls mode: no client certificate is requested, so producers authenticate
// by token alone.
func serveProducerTLS(t *testing.T, app *fiber.App) (string, *http.Client) {
	t.Helper()
	fixture := testutil.GenerateMTLSFixture(t)
	serverCert, err := tls.X509KeyPair(fixture.ServerCertPEM, fixture.ServerKeyPEM)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(fixture.CACertPEM))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverTLS := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.NoClientCert}
	done := make(chan error, 1)
	go func() {
		done <- app.Listener(tls.NewListener(listener, serverTLS), fiber.ListenConfig{DisableStartupMessage: true})
	}()
	t.Cleanup(func() { require.NoError(t, app.Shutdown()); require.NoError(t, <-done) })
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "localhost"}}
	t.Cleanup(transport.CloseIdleConnections)
	return "https://" + listener.Addr().String(), &http.Client{Transport: transport, Timeout: 5 * time.Second}
}
