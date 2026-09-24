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
	"time"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

func scopedReserveFixture(t *testing.T) ([]byte, *ReserveRequest, time.Time) {
	t.Helper()
	// Preserve the legacy REST baseline during the coordinated rollout.
	raw, err := os.ReadFile("../../../../../ledger/internal/adapters/tracer/testdata/reserve_scoped.json")
	require.NoError(t, err)
	request := &ReserveRequest{}
	require.NoError(t, json.Unmarshal(raw, request))
	now := request.TransactionTimestamp
	return raw, request, now
}

func characterizationReserveApp(t *testing.T, service ReservationService, now time.Time) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	libProblem.Install()
	api := openapi.New(app, app.Group("/v1"), openapi.Config{Title: "reservation-characterization", Version: "test"})
	handler, err := NewReservationHandler(service, clock.NewFixedClock(now))
	require.NoError(t, err)
	RegisterReservationRoutes(api, handler)
	return app
}

func postCharacterizationReserve(t *testing.T, app *fiber.App, raw []byte) *http.Response {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/reservations", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

// Both replacement transports pass identical native facts to the same command.
// BTC, native account vocabulary and sub-cent amounts are preserved verbatim.
func TestContextReserveTransportEquivalence(t *testing.T) {
	raw, err := os.ReadFile("../../../../../../pkg/tracercontract/testdata/reserve_request.json")
	require.NoError(t, err)
	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	expected, err := tracercontract.DecodeReserveJSON(t.Context(), raw, 65536, bounds)
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	admission := mocks.NewMockContextReserveAdmitter(ctrl)
	completion := mocks.NewMockContextReserveCompleter(ctrl)
	legacy := mocks.NewMockReservationService(ctrl)
	var inputs []tracercontract.ReserveRequest
	outcome := &tracercontract.ReserveResult{ContractRevision: expected.ContractRevision, TransactionID: expected.TransactionID, EvaluationID: testutil.MustDeterministicUUID(88201), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}
	admission.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
		inputs = append(inputs, input)
		return outcome, nil
	}).Times(2)
	identity := contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "origin-a"}
	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	app.Use(func(c fiber.Ctx) error {
		c.SetContext(contextutil.WithIntegrationIdentity(c.Context(), identity))
		return c.Next()
	})
	libProblem.Install()
	api := openapi.New(app, app.Group("/v1"), openapi.Config{Title: "context reservation", Version: "test"})
	handler, err := NewContextReservationHandler(admission, completion, mocks.NewMockContextReserveIDCompleter(ctrl), bounds, 65536, 100)
	require.NoError(t, err)
	RegisterContextReservationRoutes(api, handler, nil)
	response := postCharacterizationReserve(t, app, raw)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var rest tracercontract.ReserveResult
	require.NoError(t, json.NewDecoder(response.Body).Decode(&rest))
	server, err := grpcin.NewContextReservationServer(legacy, clock.NewFixedClock(testutil.FixedTime()), admission, completion, mocks.NewMockContextReserveIDCompleter(ctrl), grpcin.ContextReservationConfig{Bounds: bounds, MaxBodyBytes: 65536, MaxReservations: 100})
	require.NoError(t, err)
	request, err := contractpb.EncodeReserve(t.Context(), expected, identity.AssetNamespace, bounds)
	require.NoError(t, err)
	wire, err := server.Reserve(contextutil.WithIntegrationIdentity(t.Context(), identity), request)
	require.NoError(t, err)
	rpc, err := contractpb.DecodeResult(wire, 100)
	require.NoError(t, err)
	require.Equal(t, rest, *rpc)
	require.Len(t, inputs, 2)
	require.Equal(t, expected, inputs[0])
	require.Equal(t, inputs[0], inputs[1])
}

func TestReserveCharacterizationNativeAccountVocabulary(t *testing.T) {
	for _, tc := range []struct {
		name, accountType, accountStatus string
		want                             error
	}{
		{"legacy account", "checking", "active", nil},
		{"native deposit", "deposit", "active", constant.ErrValidationInvalidAccountType},
		{"native asset account", "current_assets", "active", constant.ErrValidationInvalidAccountType},
		{"native inactive", "checking", "INACTIVE", constant.ErrValidationInvalidAccountStatus},
		{"native blocked", "checking", "BLOCKED", constant.ErrValidationInvalidAccountStatus},
		{"native active is case sensitive", "checking", "ACTIVE", constant.ErrValidationInvalidAccountStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _, now := scopedReserveFixture(t)
			var request ReserveRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			request.Account.Type, request.Account.Status = tc.accountType, tc.accountStatus
			err := request.NormalizeAndReserveValidate(now)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
		})
	}
}

func TestReserveCharacterizationNativeAssetLimit(t *testing.T) {
	_, request, now := scopedReserveFixture(t)
	accountID := request.Account.ID
	for _, asset := range []string{"BRL", "BTC"} {
		t.Run(asset, func(t *testing.T) {
			limit, err := model.NewLimit("account daily limit", model.LimitTypeDaily, decimal.NewFromInt(100), asset,
				[]model.Scope{{AccountID: &accountID}}, nil, now)
			// Limit administration now accepts native codes. The old Reserve
			// transports are still characterized separately until replacement.
			require.NoError(t, err)
			require.NotNil(t, limit)
			require.Equal(t, asset, limit.Asset)
		})
	}
}

func TestReserveCharacterizationTypeOptionalOnlyOnReserve(t *testing.T) {
	raw, _, now := scopedReserveFixture(t)
	var request ReserveRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	request.TransactionType = ""
	request.Account = model.AccountContext{}
	require.NoError(t, request.NormalizeAndReserveValidate(now))
	require.ErrorIs(t, request.ValidationRequest.Validate(now), constant.ErrValidationInvalidTransactionType)
}
