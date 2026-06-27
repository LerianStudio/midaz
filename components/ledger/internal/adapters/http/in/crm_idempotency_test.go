// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// fakeCRMIdempotencyRepo is an in-memory IdempotencyRepo with SetNX semantics,
// shared across the two requests in a replay test.
type fakeCRMIdempotencyRepo struct {
	store map[string]string
}

func newFakeCRMIdempotencyRepo() *fakeCRMIdempotencyRepo {
	return &fakeCRMIdempotencyRepo{store: make(map[string]string)}
}

func (f *fakeCRMIdempotencyRepo) SetNX(_ context.Context, key, value string, _ time.Duration) (bool, error) {
	if _, ok := f.store[key]; ok {
		return false, nil
	}

	f.store[key] = value

	return true, nil
}

func (f *fakeCRMIdempotencyRepo) Get(_ context.Context, key string) (string, error) {
	value, ok := f.store[key]
	if !ok {
		return "", redis.Nil
	}

	return value, nil
}

func (f *fakeCRMIdempotencyRepo) Set(_ context.Context, key, value string, _ time.Duration) error {
	f.store[key] = value

	return nil
}

func TestHolderHandler_CreateHolder_IdempotentReplay(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgUUID := uuid.New()
	orgID := orgUUID.String()
	holderID := uuid.New()
	name := "John Doe"
	document := "91315026015"
	holderType := "NATURAL_PERSON"

	mockHolderRepo := holder.NewMockRepository(ctrl)
	// The Mongo create MUST run exactly once across the two identical requests.
	mockHolderRepo.EXPECT().
		Create(gomock.Any(), orgID, gomock.Any()).
		Return(&mmodel.Holder{ID: &holderID, Name: &name, Document: &document, Type: &holderType}, nil).
		Times(1)

	uc := &services.UseCase{
		HolderRepo:  mockHolderRepo,
		Idempotency: newFakeCRMIdempotencyRepo(),
	}
	handler := &HolderHandler{Service: uc}

	app := fiber.New()
	app.Post("/v1/organizations/:organization_id/holders",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgUUID)
			return c.Next()
		},
		http.WithBody(new(mmodel.CreateHolderInput), handler.CreateHolder),
	)

	body := `{"type":"NATURAL_PERSON","name":"John Doe","document":"91315026015"}`

	doRequest := func() (int, string, []byte) {
		req := httptest.NewRequest("POST", "/v1/organizations/"+orgID+"/holders", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(libConstants.IdempotencyKey, "holder-key-1")

		resp, err := app.Test(req)
		require.NoError(t, err)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return resp.StatusCode, resp.Header.Get(libConstants.IdempotencyReplayed), respBody
	}

	// First request: real create, not a replay.
	status1, replayed1, body1 := doRequest()
	assert.Equal(t, 201, status1)
	assert.Equal(t, "false", replayed1)

	var first map[string]any
	require.NoError(t, json.Unmarshal(body1, &first))

	// Second identical request: replays the original entity, no second create.
	status2, replayed2, body2 := doRequest()
	assert.Equal(t, 201, status2)
	assert.Equal(t, "true", replayed2)

	var second map[string]any
	require.NoError(t, json.Unmarshal(body2, &second))

	assert.Equal(t, first["id"], second["id"])
	assert.Equal(t, first["name"], second["name"])
}

func TestInstrumentHandler_CreateInstrument_IdempotentReplay(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgUUID := uuid.New()
	orgID := orgUUID.String()
	holderID := uuid.New()
	instrumentID := uuid.New()
	document := "12345678901"
	holderType := "individual"

	mockInstrumentRepo := instrument.NewMockRepository(ctrl)
	mockHolderRepo := holder.NewMockRepository(ctrl)

	// GetHolderByID runs on every non-replay create; here only the first request
	// reaches the create path, so Find is expected exactly once.
	mockHolderRepo.EXPECT().
		Find(gomock.Any(), orgID, holderID, false).
		Return(&mmodel.Holder{ID: &holderID, Document: &document, Type: &holderType}, nil).
		Times(1)

	// The Mongo create MUST run exactly once across the two identical requests.
	mockInstrumentRepo.EXPECT().
		Create(gomock.Any(), orgID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *mmodel.Instrument) (*mmodel.Instrument, error) {
			a.ID = &instrumentID
			return a, nil
		}).
		Times(1)

	uc := &services.UseCase{
		InstrumentRepo: mockInstrumentRepo,
		HolderRepo:     mockHolderRepo,
		Idempotency:    newFakeCRMIdempotencyRepo(),
		LedgerAccounts: stubInstrumentLedgerAccountReader{ledgerExists: true, accountExists: true},
	}
	handler := &InstrumentHandler{Service: uc}

	app := fiber.New()
	app.Post("/v1/organizations/:organization_id/holders/:holder_id/instruments",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", orgUUID)
			c.Locals("holder_id", holderID)
			return c.Next()
		},
		http.WithBody(new(mmodel.CreateInstrumentInput), handler.CreateInstrument),
	)

	body := `{"ledgerId":"00000000-0000-0000-0000-000000000001","accountId":"00000000-0000-0000-0000-000000000002"}`

	doRequest := func() (int, string, []byte) {
		req := httptest.NewRequest("POST", "/v1/organizations/"+orgID+"/holders/"+holderID.String()+"/instruments", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(libConstants.IdempotencyKey, "instrument-key-1")

		resp, err := app.Test(req)
		require.NoError(t, err)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return resp.StatusCode, resp.Header.Get(libConstants.IdempotencyReplayed), respBody
	}

	status1, replayed1, body1 := doRequest()
	assert.Equal(t, 201, status1)
	assert.Equal(t, "false", replayed1)

	var first map[string]any
	require.NoError(t, json.Unmarshal(body1, &first))

	status2, replayed2, body2 := doRequest()
	assert.Equal(t, 201, status2)
	assert.Equal(t, "true", replayed2)

	var second map[string]any
	require.NoError(t, json.Unmarshal(body2, &second))

	assert.Equal(t, first["id"], second["id"])
	assert.Equal(t, first["ledgerId"], second["ledgerId"])
}
