// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAccountCreator satisfies composition.AccountCreator for handler tests.
type stubAccountCreator struct {
	account *mmodel.Account
	err     error
}

func (s stubAccountCreator) CreateAccount(_ context.Context, _, _ uuid.UUID, _ *mmodel.CreateAccountInput, _ string) (*mmodel.Account, error) {
	return s.account, s.err
}

// stubInstrumentCreator satisfies composition.InstrumentCreator for handler tests.
type stubInstrumentCreator struct {
	instrument *mmodel.Instrument
	err        error
}

func (s stubInstrumentCreator) CreateInstrument(_ context.Context, _ string, _ uuid.UUID, _ *mmodel.CreateInstrumentInput) (*mmodel.Instrument, error) {
	return s.instrument, s.err
}

func TestCompositionHandler_CreateHolderAccount(t *testing.T) {
	holderID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()

	createdAccount := &mmodel.Account{ID: uuid.New().String(), Name: "Composite Account"}

	instrumentID := uuid.New()
	createdInstrument := &mmodel.Instrument{ID: &instrumentID}

	bankingDetails := &mmodel.BankingDetails{}

	tests := []struct {
		name           string
		payload        *mmodel.CreateHolderAccountInput
		accountCreator stubAccountCreator
		instrCreator   stubInstrumentCreator
		setHeaders     func(req *nethttp.Request)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:           "account-only success returns 201 with null instrument",
			payload:        &mmodel.CreateHolderAccountInput{Name: "Composite Account", AssetCode: "USD", Type: "deposit"},
			accountCreator: stubAccountCreator{account: createdAccount},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var resp mmodel.HolderAccountResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				require.NotNil(t, resp.Account)
				assert.Equal(t, createdAccount.ID, resp.Account.ID)
				assert.Nil(t, resp.Instrument)
				assert.Nil(t, resp.InstrumentError)
			},
		},
		{
			name:           "happy path with instrument returns 201 with instrument",
			payload:        &mmodel.CreateHolderAccountInput{Name: "Composite Account", AssetCode: "USD", Type: "deposit", BankingDetails: bankingDetails},
			accountCreator: stubAccountCreator{account: createdAccount},
			instrCreator:   stubInstrumentCreator{instrument: createdInstrument},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var resp mmodel.HolderAccountResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				require.NotNil(t, resp.Account)
				require.NotNil(t, resp.Instrument)
				assert.Nil(t, resp.InstrumentError)
			},
		},
		{
			name:           "instrument failure after account commit returns 201 with typed failure block",
			payload:        &mmodel.CreateHolderAccountInput{Name: "Composite Account", AssetCode: "USD", Type: "deposit", BankingDetails: bankingDetails},
			accountCreator: stubAccountCreator{account: createdAccount},
			instrCreator:   stubInstrumentCreator{err: pkg.ValidateBusinessError(cn.ErrEntityNotFound, "Holder")},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var resp mmodel.HolderAccountResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				require.NotNil(t, resp.Account, "account remains persisted on instrument failure")
				assert.Nil(t, resp.Instrument)
				require.NotNil(t, resp.InstrumentError, "typed failure block surfaced")
				assert.Equal(t, "FAILED", resp.InstrumentError.Status)
				assert.Equal(t, cn.ErrEntityNotFound.Error(), resp.InstrumentError.Reason)
			},
		},
		{
			name:           "account create error returns the mapped business status",
			payload:        &mmodel.CreateHolderAccountInput{Name: "Composite Account", AssetCode: "UNKNOWN", Type: "deposit"},
			accountCreator: stubAccountCreator{err: pkg.ValidateBusinessError(cn.ErrAssetCodeNotFound, "Account")},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(body, &errResp))
				assert.Equal(t, cn.ErrAssetCodeNotFound.Error(), errResp["code"])
			},
		},
		{
			name:           "missing X-Ledger-Id header returns 400",
			payload:        &mmodel.CreateHolderAccountInput{Name: "Composite Account", AssetCode: "USD", Type: "deposit"},
			accountCreator: stubAccountCreator{account: createdAccount},
			setHeaders: func(req *nethttp.Request) {
				req.Header.Del(ledgerIDHeader)
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(body, &errResp))
				assert.Equal(t, cn.ErrMissingFieldsInRequest.Error(), errResp["code"])
			},
		},
		{
			name:           "invalid X-Organization-Id header returns 400",
			payload:        &mmodel.CreateHolderAccountInput{Name: "Composite Account", AssetCode: "USD", Type: "deposit"},
			accountCreator: stubAccountCreator{account: createdAccount},
			setHeaders: func(req *nethttp.Request) {
				req.Header.Set(organizationIDHeader, "not-a-uuid")
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(body, &errResp))
				assert.Equal(t, cn.ErrInvalidPathParameter.Error(), errResp["code"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &CompositionHandler{
				Service: composition.NewService(tt.accountCreator, tt.instrCreator),
			}

			app := fiber.New()
			app.Post("/v1/holders/:id/accounts",
				func(c *fiber.Ctx) error {
					c.Locals("id", holderID)
					return c.Next()
				},
				func(c *fiber.Ctx) error {
					return handler.CreateHolderAccount(tt.payload, c)
				},
			)

			req := httptest.NewRequest("POST", "/v1/holders/"+holderID.String()+"/accounts", nil)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set(organizationIDHeader, orgID.String())
			req.Header.Set(ledgerIDHeader, ledgerID.String())
			if tt.setHeaders != nil {
				tt.setHeaders(req)
			}

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestCompositionHandler_CreateHolderAccount_PayloadAssertion(t *testing.T) {
	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	holderID := uuid.New()

	app := fiber.New()
	app.Post("/v1/holders/:id/accounts",
		func(c *fiber.Ctx) error {
			c.Locals("id", holderID)
			return c.Next()
		},
		func(c *fiber.Ctx) error {
			// Wrong payload type forces the type-assertion guard.
			return handler.CreateHolderAccount(&mmodel.CreateAccountInput{}, c)
		},
	)

	req := httptest.NewRequest("POST", "/v1/holders/"+holderID.String()+"/accounts", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	// The nil-guard delegates to http.WithError(c, cn.ErrInternalServer), which
	// maps to an internal-server error (HTTP 500). The JSON envelope shape is the
	// production FiberErrorHandler's job; this bare-app test asserts the status
	// contract only.
	assert.Equal(t, 500, resp.StatusCode)
}
