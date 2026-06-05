// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// assertErr is a generic technical failure used to drive the 500 path.
var assertErr = errors.New("boom: reservation service failure")

// newValidReserveRequest builds a reserve body inside the timestamp window so the
// embedded validation-request validation passes against the real clock.
func newValidReserveRequest() ReserveRequest {
	return ReserveRequest{
		TransactionID: testutil.MustDeterministicUUID(1),
		ValidationRequest: model.ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(2),
			TransactionType:      model.TransactionTypeCard,
			Amount:               decimal.RequireFromString("100"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account: model.AccountContext{
				ID: testutil.MustDeterministicUUID(3),
			},
		},
	}
}

func TestReservationHandler_Reserve(t *testing.T) {
	reservationID := testutil.MustDeterministicUUID(10)

	tests := []struct {
		name           string
		requestBody    any
		mockSetup      func(ctrl *gomock.Controller) *mocks.MockReservationService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success - capacity reserved returns 201 with reservation ids",
			requestBody: newValidReserveRequest(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				m := mocks.NewMockReservationService(ctrl)
				m.EXPECT().
					Reserve(gomock.Any(), testutil.MustDeterministicUUID(1), gomock.Any(), false).
					Return(&services.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}, nil)
				return m
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var resp ReserveResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.False(t, resp.Denied)
				require.Len(t, resp.ReservationIDs, 1)
				assert.Equal(t, reservationID, resp.ReservationIDs[0])
				assert.Equal(t, testutil.MustDeterministicUUID(1), resp.TransactionID)
			},
		},
		{
			// A PENDING-transaction reserve sets longLived=true on the wire; the
			// handler must forward that hint to the service so the reservation gets
			// the long-lived TTL (R18). The matcher asserts true reaches the service.
			name: "long-lived - longLived=true is forwarded to the service",
			requestBody: func() any {
				r := newValidReserveRequest()
				r.LongLived = true
				return r
			}(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				m := mocks.NewMockReservationService(ctrl)
				m.EXPECT().
					Reserve(gomock.Any(), testutil.MustDeterministicUUID(1), gomock.Any(), true).
					Return(&services.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}, nil)
				return m
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var resp ReserveResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.False(t, resp.Denied)
				require.Len(t, resp.ReservationIDs, 1)
			},
		},
		{
			name:        "denied - limit exceeded returns 201 with denied=true and empty ids",
			requestBody: newValidReserveRequest(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				m := mocks.NewMockReservationService(ctrl)
				m.EXPECT().
					Reserve(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&services.ReserveResult{Denied: true}, nil)
				return m
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var resp ReserveResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.True(t, resp.Denied)
				assert.NotNil(t, resp.ReservationIDs)
				assert.Empty(t, resp.ReservationIDs)
				// reservationIds must serialize as [] not null
				assert.Contains(t, string(body), `"reservationIds":[]`)
			},
		},
		{
			name: "bad input - missing transactionId returns 400, service not called",
			requestBody: func() any {
				r := newValidReserveRequest()
				r.TransactionID = uuid.Nil
				return r
			}(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				return mocks.NewMockReservationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "transactionId")
			},
		},
		{
			// The reserve path RELAXES the account requirement (the ledger may
			// reserve for an external-only source with no internal account UUID).
			// A reserve with no account is accepted and reaches the service, which
			// matches non-account-scoped limits.
			name: "relaxed - missing account is accepted (reserve path), service called",
			requestBody: func() any {
				r := newValidReserveRequest()
				r.Account = model.AccountContext{}
				return r
			}(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				m := mocks.NewMockReservationService(ctrl)
				m.EXPECT().
					Reserve(gomock.Any(), testutil.MustDeterministicUUID(1), gomock.Any(), false).
					Return(&services.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}, nil)
				return m
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var resp ReserveResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.False(t, resp.Denied)
			},
		},
		{
			// The reserve path RELAXES the transactionType requirement (the ledger
			// has no card-rail nature). An EMPTY transactionType is accepted; this
			// is exactly the shape the ledger anchor sends.
			name: "relaxed - empty transactionType is accepted (reserve path), service called",
			requestBody: func() any {
				r := newValidReserveRequest()
				r.TransactionType = ""
				r.Account = model.AccountContext{} // ledger-shaped: no account either
				return r
			}(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				m := mocks.NewMockReservationService(ctrl)
				m.EXPECT().
					Reserve(gomock.Any(), testutil.MustDeterministicUUID(1), gomock.Any(), false).
					Return(&services.ReserveResult{}, nil)
				return m
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var resp ReserveResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.False(t, resp.Denied)
			},
		},
		{
			// An INVALID (non-empty, non-enum) transactionType is still rejected on
			// the reserve path — relaxation makes it optional, not unvalidated.
			name: "bad input - invalid transactionType returns 400, service not called",
			requestBody: func() any {
				r := newValidReserveRequest()
				r.TransactionType = model.TransactionType("PIXIE")
				return r
			}(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				return mocks.NewMockReservationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "transactionType")
			},
		},
		{
			name: "bad input - invalid currency returns 400, service not called",
			requestBody: func() any {
				r := newValidReserveRequest()
				r.Currency = "usd" // lowercase rejected by strict ISO 4217 check
				return r
			}(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				return mocks.NewMockReservationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "currency")
			},
		},
		{
			name:        "bad input - malformed JSON body returns 400, service not called",
			requestBody: `{"transactionId": "not-a-uuid"`,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				return mocks.NewMockReservationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), constant.CodeBadRequest)
			},
		},
		{
			name:        "service failure returns 500",
			requestBody: newValidReserveRequest(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				m := mocks.NewMockReservationService(ctrl)
				m.EXPECT().
					Reserve(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, assertErr)
				return m
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), constant.CodeInternalServer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockService := tt.mockSetup(ctrl)

			handler, err := NewReservationHandler(mockService, clock.New())
			require.NoError(t, err)

			app := fiber.New()
			app.Post("/v1/reservations", handler.Reserve)

			body := marshalRequestBody(t, tt.requestBody)

			req := httptest.NewRequest(http.MethodPost, "/v1/reservations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.expectedBody != nil {
				tt.expectedBody(t, respBody)
			}
		})
	}
}

func TestReservationHandler_ConfirmRelease(t *testing.T) {
	validID := testutil.MustDeterministicUUID(20)

	tests := []struct {
		name               string
		path               string
		idParam            string
		mockSetup          func(ctrl *gomock.Controller) *mocks.MockReservationService
		expectedStatus     int
		expectedStatusBody string
	}{
		{
			name:    "confirm success returns 200 CONFIRMED",
			path:    "confirm",
			idParam: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				m := mocks.NewMockReservationService(ctrl)
				m.EXPECT().Confirm(gomock.Any(), validID).Return(nil)
				return m
			},
			expectedStatus:     http.StatusOK,
			expectedStatusBody: string(model.StatusConfirmed),
		},
		{
			name:    "release success returns 200 RELEASED",
			path:    "release",
			idParam: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				m := mocks.NewMockReservationService(ctrl)
				m.EXPECT().Release(gomock.Any(), validID).Return(nil)
				return m
			},
			expectedStatus:     http.StatusOK,
			expectedStatusBody: string(model.StatusReleased),
		},
		{
			name:    "confirm with bad uuid returns 400, service not called",
			path:    "confirm",
			idParam: "not-a-uuid",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				return mocks.NewMockReservationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:    "release with bad uuid returns 400, service not called",
			path:    "release",
			idParam: "not-a-uuid",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				return mocks.NewMockReservationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:    "confirm not-found maps to 404",
			path:    "confirm",
			idParam: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockReservationService {
				m := mocks.NewMockReservationService(ctrl)
				m.EXPECT().Confirm(gomock.Any(), validID).Return(constant.ErrReservationNotFound)
				return m
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockService := tt.mockSetup(ctrl)

			handler, err := NewReservationHandler(mockService, clock.New())
			require.NoError(t, err)

			app := fiber.New()
			app.Post("/v1/reservations/:id/confirm", handler.Confirm)
			app.Post("/v1/reservations/:id/release", handler.Release)

			url := "/v1/reservations/" + tt.idParam + "/" + tt.path
			req := httptest.NewRequest(http.MethodPost, url, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatusBody != "" {
				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var action ReservationActionResponse
				require.NoError(t, json.Unmarshal(respBody, &action))
				assert.Equal(t, tt.idParam, action.ReservationID.String())
				assert.Equal(t, tt.expectedStatusBody, action.Status)
			}
		})
	}
}

func TestNewReservationHandler_NilDeps(t *testing.T) {
	_, err := NewReservationHandler(nil, clock.New())
	require.Error(t, err)

	ctrl := gomock.NewController(t)
	_, err = NewReservationHandler(mocks.NewMockReservationService(ctrl), nil)
	require.Error(t, err)
}

// marshalRequestBody renders a test request body: strings pass through as raw
// payloads (for malformed-JSON cases), everything else is JSON-marshaled.
func marshalRequestBody(t *testing.T, requestBody any) []byte {
	t.Helper()

	if s, ok := requestBody.(string); ok {
		return []byte(s)
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	return body
}
