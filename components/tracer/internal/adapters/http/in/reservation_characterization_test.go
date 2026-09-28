// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

// characterizationLegacyReserveBody is the pre-contract reserve body: a single
// account reference plus segment/portfolio/merchant hints and no
// contractRevision. The reservation surface must refuse it without admitting.
const characterizationLegacyReserveBody = `{
  "transactionId": "11111111-1111-4111-8111-111111111111",
  "requestId": "22222222-2222-4222-8222-222222222222",
  "amount": "2500.125",
  "asset": "BRL",
  "account": {"accountId": "33333333-3333-4333-8333-333333333333"},
  "segmentId": "44444444-4444-4444-8444-444444444444",
  "portfolioId": "55555555-5555-4555-8555-555555555555",
  "merchantId": "66666666-6666-4666-8666-666666666666",
  "transactionType": "PIX",
  "transactionTimestamp": "2026-09-24T12:00:00Z",
  "longLived": true
}`

const characterizationMaxBodyBytes = 65536

func characterizationBounds() tracercontract.Limits {
	return tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
}

func characterizationReserveFixture(t *testing.T) ([]byte, tracercontract.ReserveRequest) {
	t.Helper()

	raw, err := os.ReadFile("../../../../../../pkg/tracercontract/testdata/reserve_request.json")
	require.NoError(t, err)

	request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, characterizationMaxBodyBytes, characterizationBounds())
	require.NoError(t, err)

	return raw, request
}

// characterizationReserveApp mounts the reservation operations behind a stub
// that plays the already-authenticated producer, so the tests observe only the
// transport contract.
func characterizationReserveApp(t *testing.T, admission ContextReserveAdmitter, completion ContextReserveCompleter, byID ContextReserveIDCompleter) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	app.Use(func(c fiber.Ctx) error {
		c.SetContext(contextutil.WithIntegrationIdentity(c.Context(), contextutil.IntegrationIdentity{ID: "producer"}))
		return c.Next()
	})
	libProblem.Install()

	api := openapi.New(app, app.Group("/v1"), openapi.Config{Title: "reservation-characterization", Version: "test"})
	handler, err := NewContextReservationHandler(admission, completion, byID, characterizationBounds(), characterizationMaxBodyBytes, 100)
	require.NoError(t, err)
	RegisterContextReservationRoutes(api, handler)

	return app
}

func postCharacterization(t *testing.T, app *fiber.App, path string, raw []byte) (int, errorResponse) {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")

	response, err := app.Test(request)
	require.NoError(t, err)

	defer func() { require.NoError(t, response.Body.Close()) }()

	var problem errorResponse
	if response.StatusCode >= http.StatusBadRequest {
		require.Contains(t, response.Header.Get("Content-Type"), "application/problem+json")
		require.NoError(t, json.NewDecoder(response.Body).Decode(&problem))
	}

	return response.StatusCode, problem
}

// Both transports pass identical native facts to the same command. BTC, native
// account vocabulary and sub-cent amounts are preserved verbatim.
func TestContextReserveTransportEquivalence(t *testing.T) {
	raw, expected := characterizationReserveFixture(t)
	bounds := characterizationBounds()
	ctrl := gomock.NewController(t)
	admission := mocks.NewMockContextReserveAdmitter(ctrl)
	completion := mocks.NewMockContextReserveCompleter(ctrl)
	byID := mocks.NewMockContextReserveIDCompleter(ctrl)

	var inputs []tracercontract.ReserveRequest

	outcome := &tracercontract.ReserveResult{ContractRevision: expected.ContractRevision, TransactionID: expected.TransactionID, EvaluationID: testutil.MustDeterministicUUID(88201), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}
	admission.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
		inputs = append(inputs, input)
		return outcome, nil
	}).Times(2)

	app := characterizationReserveApp(t, admission, completion, byID)
	request := httptest.NewRequest(http.MethodPost, "/v1/reservations", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	require.NoError(t, err)

	defer func() { require.NoError(t, response.Body.Close()) }()

	require.Equal(t, http.StatusCreated, response.StatusCode)

	var rest tracercontract.ReserveResult
	require.NoError(t, json.NewDecoder(response.Body).Decode(&rest))

	server, err := grpcin.NewContextReservationServer(admission, completion, byID, grpcin.ContextReservationConfig{Bounds: bounds, MaxBodyBytes: characterizationMaxBodyBytes, MaxReservations: 100})
	require.NoError(t, err)
	wireRequest, err := contractpb.EncodeReserve(t.Context(), expected, bounds)
	require.NoError(t, err)
	wire, err := server.Reserve(contextutil.WithIntegrationIdentity(t.Context(), contextutil.IntegrationIdentity{ID: "producer"}), wireRequest)
	require.NoError(t, err)
	rpc, err := contractpb.DecodeResult(wire, 100)
	require.NoError(t, err)

	require.Equal(t, rest, *rpc)
	require.Len(t, inputs, 2)
	require.Equal(t, expected, inputs[0])
	require.Equal(t, inputs[0], inputs[1])
}

// The pre-contract body is not a second dialect of the route: its unknown
// fields reject it before admission, whatever credentials reached the handler.
func TestReserveCharacterizationRejectsLegacyBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	app := characterizationReserveApp(t, mocks.NewMockContextReserveAdmitter(ctrl), mocks.NewMockContextReserveCompleter(ctrl), mocks.NewMockContextReserveIDCompleter(ctrl))

	status, problem := postCharacterization(t, app, "/v1/reservations", []byte(characterizationLegacyReserveBody))
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, constant.ErrInvalidRequestBody.Error(), problem.Code)
}

// A completion always names the contract revision. An absent body addresses
// nothing and never reaches a completion command: the required-body guard
// refuses it before the handler, and the strict decoder refuses every body that
// does not carry the revision.
func TestReserveCharacterizationCompletionRequiresRevisionBody(t *testing.T) {
	id := testutil.MustDeterministicUUID(88202).String()

	for _, path := range []string{
		"/v1/reservations/transaction/" + id + "/confirm",
		"/v1/reservations/transaction/" + id + "/release",
		"/v1/reservations/" + id + "/confirm",
		"/v1/reservations/" + id + "/release",
	} {
		t.Run(path, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			app := characterizationReserveApp(t, mocks.NewMockContextReserveAdmitter(ctrl), mocks.NewMockContextReserveCompleter(ctrl), mocks.NewMockContextReserveIDCompleter(ctrl))

			status, _ := postCharacterization(t, app, path, nil)
			require.Equal(t, http.StatusBadRequest, status, "absent body")

			for name, body := range map[string][]byte{
				"whitespace":       []byte("  \n"),
				"missing revision": []byte(`{}`),
				"unknown revision": []byte(`{"contractRevision":"unsupported"}`),
			} {
				t.Run(name, func(t *testing.T) {
					status, problem := postCharacterization(t, app, path, body)
					require.Equal(t, http.StatusBadRequest, status)
					require.Equal(t, constant.ErrInvalidRequestBody.Error(), problem.Code)
				})
			}
		})
	}
}

// The operation's body limit runs before decoding, so an oversized body need
// not be JSON and never reaches a command.
func TestReserveCharacterizationPayloadTooLarge(t *testing.T) {
	ctrl := gomock.NewController(t)
	app := characterizationReserveApp(t, mocks.NewMockContextReserveAdmitter(ctrl), mocks.NewMockContextReserveCompleter(ctrl), mocks.NewMockContextReserveIDCompleter(ctrl))
	oversized := bytes.Repeat([]byte("x"), characterizationMaxBodyBytes+1)
	id := testutil.MustDeterministicUUID(88203).String()

	for _, path := range []string{"/v1/reservations", "/v1/reservations/transaction/" + id + "/confirm", "/v1/reservations/" + id + "/release"} {
		status, _ := postCharacterization(t, app, path, oversized)
		require.Equal(t, http.StatusRequestEntityTooLarge, status, path)
	}
}
