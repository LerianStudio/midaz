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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

func scopedReserveFixture(t *testing.T) ([]byte, *reservationv1.ReserveRequest, time.Time) {
	t.Helper()
	// One fixture locks the actual Ledger encoder and both Tracer adapters.
	raw, err := os.ReadFile("../../../../../ledger/internal/adapters/tracer/testdata/reserve_scoped.json")
	require.NoError(t, err)
	request := &reservationv1.ReserveRequest{}
	require.NoError(t, protojson.Unmarshal(raw, request))
	now, err := time.Parse(time.RFC3339, request.TransactionTimestamp)
	require.NoError(t, err)
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

// Known defect baseline: the same Ledger JSON becomes full scopes over protobuf
// and silently loses the three flat scope IDs over REST. This is not the desired
// future behavior; migration must replace this assertion with equality.
func TestReserveCharacterizationTransportScopeLoss(t *testing.T) {
	raw, request, now := scopedReserveFixture(t)
	service := mocks.NewMockReservationService(gomock.NewController(t))
	txID := uuid.MustParse(request.TransactionId)
	var inputs []*model.CheckLimitsInput
	service.EXPECT().Reserve(gomock.Any(), txID, gomock.Any(), true).DoAndReturn(
		func(_ context.Context, _ uuid.UUID, input *model.CheckLimitsInput, _ bool) (*services.ReserveResult, error) {
			inputs = append(inputs, input)
			return &services.ReserveResult{}, nil
		},
	).Times(2)
	response := postCharacterizationReserve(t, characterizationReserveApp(t, service, now), raw)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	server, err := grpcin.NewReservationServer(service, clock.NewFixedClock(now))
	require.NoError(t, err)
	_, err = server.Reserve(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, inputs, 2)
	rest, rpc := inputs[0], inputs[1]
	require.Nil(t, rest.SegmentID)
	require.Nil(t, rest.PortfolioID)
	require.Nil(t, rest.MerchantID)
	require.NotNil(t, rpc.SegmentID)
	require.NotNil(t, rpc.PortfolioID)
	require.NotNil(t, rpc.MerchantID)
	require.Equal(t, request.SegmentId, rpc.SegmentID.String())
	require.Equal(t, request.PortfolioId, rpc.PortfolioID.String())
	require.Equal(t, request.MerchantId, rpc.MerchantID.String())
	rpc.SegmentID, rpc.PortfolioID, rpc.MerchantID = nil, nil, nil
	require.Equal(t, rest, rpc, "all other normalized inputs must agree, including the exact fractional amount")
}

func TestReserveCharacterizationNativeAssetRejectedByBothTransports(t *testing.T) {
	raw, request, now := scopedReserveFixture(t)
	raw = bytes.Replace(raw, []byte(`"BRL"`), []byte(`"BTC"`), 1)
	request.Asset = "BTC"
	service := mocks.NewMockReservationService(gomock.NewController(t))
	response := postCharacterizationReserve(t, characterizationReserveApp(t, service, now), raw)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	var problem struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&problem))
	require.Equal(t, constant.ErrValidationInvalidCurrency.Error(), problem.Code)
	server, err := grpcin.NewReservationServer(service, clock.NewFixedClock(now))
	require.NoError(t, err)
	_, err = server.Reserve(context.Background(), request)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, constant.ErrValidationInvalidCurrency.Error(), status.Convert(err).Message())
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
	accountID := uuid.MustParse(request.Account.AccountId)
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
