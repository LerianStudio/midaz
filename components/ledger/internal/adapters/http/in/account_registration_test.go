// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	crmhttp "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/crm/http"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accountregistration"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newTestApp(t *testing.T, orgID, ledgerID uuid.UUID, regRepo *accountregistration.MockRepository, crm *crmhttp.MockCRMAccountRelationshipPort) *fiber.App {
	t.Helper()

	cmdUC := &command.UseCase{
		AccountRegistrationRepo: regRepo,
		CRMClient:               crm,
	}
	qryUC := &query.UseCase{
		AccountRegistrationRepo: regRepo,
	}

	handler := &AccountRegistrationHandler{Command: cmdUC, Query: qryUC}

	app := fiber.New()
	app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/account-registrations",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			return c.Next()
		},
		http.WithBody(new(mmodel.CreateAccountRegistrationInput), handler.CreateAccountRegistration),
	)
	app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-registrations/:id",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgID)
			c.Locals("ledger_id", ledgerID)
			if raw := c.Params("id"); raw != "" {
				if parsed, err := uuid.Parse(raw); err == nil {
					c.Locals("id", parsed)
				}
			}
			return c.Next()
		},
		handler.GetAccountRegistration,
	)

	return app
}

func validPayload() *mmodel.CreateAccountRegistrationInput {
	return &mmodel.CreateAccountRegistrationInput{
		HolderID: uuid.New(),
		Account: mmodel.CreateAccountInput{
			Name:      "John Checking",
			Type:      "deposit",
			AssetCode: "USD",
		},
		CRMAlias: mmodel.CreateAliasInput{
			LedgerID:  uuid.New().String(),
			AccountID: uuid.New().String(),
		},
	}
}

// TestAccountRegistrationHandler_Create_MissingIdempotencyKey returns 400 when the
// Idempotency-Key header is missing.
func TestAccountRegistrationHandler_Create_MissingIdempotencyKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()

	repo := accountregistration.NewMockRepository(ctrl)
	crm := crmhttp.NewMockCRMAccountRelationshipPort(ctrl)

	app := newTestApp(t, orgID, ledgerID, repo, crm)

	body, err := json.Marshal(validPayload())
	require.NoError(t, err)

	req := httptest.NewRequest(nethttp.MethodPost,
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-registrations",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Deliberately omit Idempotency-Key.

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, nethttp.StatusBadRequest, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)

	var errResp map[string]any
	_ = json.Unmarshal(respBody, &errResp)
	assert.Equal(t, constant.ErrIdempotencyKeyRequired.Error(), errResp["code"])
}

// TestAccountRegistrationHandler_Create_PropagatesTerminalFailure verifies the handler
// surfaces a terminal saga error (holder not found) as an HTTP error mapped by
// pkg.ValidateBusinessError → the libHTTP FiberErrorHandler. The test does NOT
// simulate the full saga; it stubs the repo so that UpsertByIdempotencyKey succeeds
// and then makes the CRM client return ErrHolderNotFound so the saga short-circuits
// quickly.
func TestAccountRegistrationHandler_Create_HolderNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()

	repo := accountregistration.NewMockRepository(ctrl)
	crm := crmhttp.NewMockCRMAccountRelationshipPort(ctrl)

	app := newTestApp(t, orgID, ledgerID, repo, crm)

	regID := uuid.New()

	repo.EXPECT().
		UpsertByIdempotencyKey(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, seed *mmodel.AccountRegistration) (*mmodel.AccountRegistration, bool, error) {
			seed.ID = regID
			return seed, true, nil
		}).
		Times(1)

	crm.EXPECT().
		GetHolder(gomock.Any(), orgID.String(), gomock.Any(), gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrHolderNotFound, constant.EntityAccountRegistration)).
		Times(1)

	repo.EXPECT().
		MarkFailed(gomock.Any(), regID, mmodel.AccountRegistrationFailedTerminal, "HOLDER_NOT_FOUND", gomock.Any()).
		Return(nil).
		Times(1)

	body, err := json.Marshal(validPayload())
	require.NoError(t, err)

	req := httptest.NewRequest(nethttp.MethodPost,
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-registrations",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency", "key-1")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := app.Test(req)
	require.NoError(t, err)

	respBody, _ := io.ReadAll(resp.Body)

	var errResp map[string]any
	_ = json.Unmarshal(respBody, &errResp)
	assert.Equal(t, constant.ErrHolderNotFound.Error(), errResp["code"],
		"expected CRM-0006 error code, got body %s", string(respBody))
}

// TestAccountRegistrationHandler_Get_ReturnsOK verifies GET returns the registration.
func TestAccountRegistrationHandler_Get_ReturnsOK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	regID := uuid.New()

	repo := accountregistration.NewMockRepository(ctrl)
	crm := crmhttp.NewMockCRMAccountRelationshipPort(ctrl)

	stored := &mmodel.AccountRegistration{
		ID:             regID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status:         mmodel.AccountRegistrationCompleted,
	}

	repo.EXPECT().
		FindByID(gomock.Any(), orgID, ledgerID, regID).
		Return(stored, nil).
		Times(1)

	app := newTestApp(t, orgID, ledgerID, repo, crm)

	req := httptest.NewRequest(nethttp.MethodGet,
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-registrations/"+regID.String(),
		nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, nethttp.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)

	var got mmodel.AccountRegistration
	require.NoError(t, json.Unmarshal(respBody, &got))
	assert.Equal(t, regID, got.ID)
	assert.Equal(t, mmodel.AccountRegistrationCompleted, got.Status)
}

// TestAccountRegistrationHandler_Get_PropagatesNotFound maps the repository's
// not-found business error to HTTP 404.
func TestAccountRegistrationHandler_Get_PropagatesNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	regID := uuid.New()

	repo := accountregistration.NewMockRepository(ctrl)
	crm := crmhttp.NewMockCRMAccountRelationshipPort(ctrl)

	repo.EXPECT().
		FindByID(gomock.Any(), orgID, ledgerID, regID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrAccountRegistrationNotFound, constant.EntityAccountRegistration)).
		Times(1)

	app := newTestApp(t, orgID, ledgerID, repo, crm)

	req := httptest.NewRequest(nethttp.MethodGet,
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-registrations/"+regID.String(),
		nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, nethttp.StatusNotFound, resp.StatusCode)
}
