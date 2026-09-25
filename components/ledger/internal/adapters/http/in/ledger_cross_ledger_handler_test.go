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
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestUpdateLedgerSettings_CrossLedgerEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	repo := ledger.NewMockRepository(ctrl)
	repo.EXPECT().UpdateSettingsAtomic(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ any, _, _ uuid.UUID, merge func(map[string]any) (map[string]any, error)) (map[string]any, error) {
			return merge(map[string]any{"accounting": map[string]any{"validateRoutes": true}})
		})

	app := buildHumaLedgerApp(t, &LedgerHandler{Command: &command.UseCase{LedgerRepo: repo}}, true)
	body := []byte(`{"crossLedger":{"enabled":true}}`)
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+organizationID.String()+"/ledgers/"+ledgerID.String()+"/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got))
	assert.Equal(t, true, got["crossLedger"].(map[string]any)["enabled"])
	assert.Equal(t, true, got["accounting"].(map[string]any)["validateRoutes"])
}

func TestCreateLedger_CrossLedgerEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	repo := ledger.NewMockRepository(ctrl)
	repo.EXPECT().FindByName(gomock.Any(), organizationID, "Cross-ledger enabled").Return(false, nil)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, created *mmodel.Ledger) (*mmodel.Ledger, error) {
		assert.True(t, created.Settings.CrossLedger.Enabled)
		created.ID = ledgerID.String()
		created.CreatedAt = fixedTestTime
		created.UpdatedAt = fixedTestTime

		return created, nil
	})
	metadataRepo := mongodb.NewMockRepository(ctrl)
	metadataRepo.EXPECT().Create(gomock.Any(), constant.EntityLedger, gomock.Any()).Return(nil)

	app := buildHumaLedgerApp(t, &LedgerHandler{Command: &command.UseCase{
		LedgerRepo:             repo,
		OnboardingMetadataRepo: metadataRepo,
	}}, true)
	body := []byte(`{"name":"Cross-ledger enabled","settings":{"crossLedger":{"enabled":true}}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+organizationID.String()+"/ledgers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got))
	assert.Equal(t, true, got["settings"].(map[string]any)["crossLedger"].(map[string]any)["enabled"])
}

func TestUpdateLedgerSettings_CrossLedgerWrongTypeIsCanonical400(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	app := buildHumaLedgerApp(t, &LedgerHandler{Command: &command.UseCase{LedgerRepo: ledger.NewMockRepository(ctrl)}}, true)
	body := []byte(`{"crossLedger":{"enabled":"yes"}}`)
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+organizationID.String()+"/ledgers/"+ledgerID.String()+"/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got))
	assert.Equal(t, constant.ErrInvalidSettingsFieldType.Error(), got["code"])
}
