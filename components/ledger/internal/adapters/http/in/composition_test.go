// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
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
	// Fixed UUIDs keep the suite deterministic (no time.Now()/random seeds).
	holderID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	ledgerID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	createdAccount := &mmodel.Account{ID: "44444444-4444-4444-4444-444444444444", Name: "Composite Account"}

	instrumentID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	createdInstrument := &mmodel.Instrument{ID: &instrumentID}

	bankingDetails := &mmodel.BankingDetails{}

	// validPath is the happy-path target: org, ledger, and holder are all valid
	// UUID path segments. Org and ledger are path-scoped now (no scoping headers).
	validPath := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/holders/" + holderID.String() + "/accounts"

	tests := []struct {
		name           string
		payload        *mmodel.CreateHolderAccountInput
		accountCreator stubAccountCreator
		instrCreator   stubInstrumentCreator
		// targetPath overrides validPath for the malformed-segment cases that must
		// drive the real ParseUUIDPathParameters validator. Empty means validPath.
		targetPath     string
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
			// Replaces the former "missing ledger header returns 400" case. Ledger is
			// now a path segment validated by ParseUUIDPathParameters. A genuinely
			// MISSING segment can no longer be expressed — the route would not match
			// and Fiber returns 404 — so the "missing" semantics become "malformed":
			// a non-UUID ledger_id segment is rejected with 400 (ErrInvalidPathParameter).
			name:           "non-UUID ledger_id path segment returns 400",
			payload:        &mmodel.CreateHolderAccountInput{Name: "Composite Account", AssetCode: "USD", Type: "deposit"},
			accountCreator: stubAccountCreator{account: createdAccount},
			targetPath:     "/v1/organizations/" + orgID.String() + "/ledgers/not-a-uuid/holders/" + holderID.String() + "/accounts",
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(body, &errResp))
				assert.Equal(t, cn.ErrInvalidPathParameter.Error(), errResp["code"])
			},
		},
		{
			// A non-UUID organization_id path segment is rejected by the
			// ParseUUIDPathParameters chain with 400 (ErrInvalidPathParameter).
			name:           "non-UUID organization_id path segment returns 400",
			payload:        &mmodel.CreateHolderAccountInput{Name: "Composite Account", AssetCode: "USD", Type: "deposit"},
			accountCreator: stubAccountCreator{account: createdAccount},
			targetPath:     "/v1/organizations/not-a-uuid/ledgers/" + ledgerID.String() + "/holders/" + holderID.String() + "/accounts",
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

			// Drive the REAL path-scoped route through ParseUUIDPathParameters, which
			// validates organization_id, ledger_id, and id (holder) as UUIDs and
			// stores them in locals — the same chain the production router runs. The
			// handler reads org/ledger/holder from those locals; no scoping headers.
			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts",
				http.ParseUUIDPathParameters("holder"),
				func(c *fiber.Ctx) error {
					return handler.CreateHolderAccount(tt.payload, c)
				},
			)

			target := tt.targetPath
			if target == "" {
				target = validPath
			}

			req := httptest.NewRequest(fiber.MethodPost, target, nil)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")

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
	orgID := uuid.New()
	ledgerID := uuid.New()

	app := fiber.New()
	app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts",
		http.ParseUUIDPathParameters("holder"),
		func(c *fiber.Ctx) error {
			// Wrong payload type forces the type-assertion guard.
			return handler.CreateHolderAccount(&mmodel.CreateAccountInput{}, c)
		},
	)

	req := httptest.NewRequest(fiber.MethodPost,
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/holders/"+holderID.String()+"/accounts", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	// The nil-guard delegates to http.WithError(c, cn.ErrInternalServer), which
	// maps to an internal-server error (HTTP 500). The JSON envelope shape is the
	// production FiberErrorHandler's job; this bare-app test asserts the status
	// contract only.
	assert.Equal(t, 500, resp.StatusCode)
}
