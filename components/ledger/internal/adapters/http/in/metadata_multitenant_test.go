// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/mongo"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestMetadataIndexHandler_MongoManagerSelection(t *testing.T) {
	t.Parallel()

	onboardingManager := &tmmongo.Manager{}
	transactionManager := &tmmongo.Manager{}

	handler := &MetadataIndexHandler{
		OnboardingMongoManager:  onboardingManager,
		TransactionMongoManager: transactionManager,
	}

	assert.Same(t, onboardingManager, handler.getMongoManager("organization"))
	assert.Same(t, onboardingManager, handler.getMongoManager("account"))
	assert.Same(t, transactionManager, handler.getMongoManager("transaction"))
	assert.Same(t, transactionManager, handler.getMongoManager("operation_route"))
	assert.Nil(t, handler.getMongoManager("unknown_entity"))
}

func TestMetadataIndexHandler_ContextHelpers_MissingManager(t *testing.T) {
	t.Parallel()

	handler := &MetadataIndexHandler{}

	type ctxKey string
	ctx := context.WithValue(context.Background(), ctxKey("sentinel"), uuid.NewString())

	ctxNoTenant, err := handler.contextForEntity(ctx, "transaction")
	require.NoError(t, err)
	assert.Equal(t, ctx, ctxNoTenant)

	ctxWithTenant := tmcore.ContextWithTenantID(ctx, "tenant-1")

	ctxEntity, err := handler.contextForEntity(ctxWithTenant, "transaction")
	require.Error(t, err)
	assert.Nil(t, ctxEntity)
	assert.Contains(t, err.Error(), "multi-tenant mongo manager not configured")

	ctxRepoNoTenant, err := handler.contextForRepoGroup(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, ctx, ctxRepoNoTenant)

	ctxRepoOnboarding, err := handler.contextForRepoGroup(ctxWithTenant, true)
	require.Error(t, err)
	assert.Nil(t, ctxRepoOnboarding)
	assert.Contains(t, err.Error(), "onboarding")

	ctxRepoTransaction, err := handler.contextForRepoGroup(ctxWithTenant, false)
	require.Error(t, err)
	assert.Nil(t, ctxRepoTransaction)
	assert.Contains(t, err.Error(), "transaction")
}

func TestMetadataIndexHandler_MultiTenantContextResolutionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		method         string
		url            string
		body           any
		registerRoute  func(*fiber.App, *MetadataIndexHandler, any)
		expectedStatus int
	}{
		{
			name:   "create metadata index returns 500 when tenant mongo manager is missing",
			method: fiber.MethodPost,
			url:    "/v1/settings/metadata-indexes/entities/transaction",
			body: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "tier",
			},
			registerRoute: func(app *fiber.App, handler *MetadataIndexHandler, payload any) {
				app.Post("/v1/settings/metadata-indexes/entities/:entity_name", func(c *fiber.Ctx) error {
					c.SetUserContext(tmcore.ContextWithTenantID(context.Background(), "tenant-1"))

					return handler.CreateMetadataIndex(payload, c)
				})
			},
			expectedStatus: fiber.StatusInternalServerError,
		},
		{
			name:           "get metadata indexes by entity returns 500 when tenant mongo manager is missing",
			method:         fiber.MethodGet,
			url:            "/v1/settings/metadata-indexes?entity_name=transaction",
			registerRoute:  registerGetAllMetadataRoute,
			expectedStatus: fiber.StatusInternalServerError,
		},
		{
			name:           "get all metadata indexes returns 500 when tenant mongo manager is missing",
			method:         fiber.MethodGet,
			url:            "/v1/settings/metadata-indexes",
			registerRoute:  registerGetAllMetadataRoute,
			expectedStatus: fiber.StatusInternalServerError,
		},
		{
			name:           "delete metadata index returns 500 when tenant mongo manager is missing",
			method:         fiber.MethodDelete,
			url:            "/v1/settings/metadata-indexes/entities/transaction/key/tier",
			registerRoute:  registerDeleteMetadataRoute,
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			handler := &MetadataIndexHandler{
				OnboardingMetadataRepo:  mbootstrap.NewMockMetadataIndexRepository(ctrl),
				TransactionMetadataRepo: mbootstrap.NewMockMetadataIndexRepository(ctrl),
			}

			app := newMetadataHandlerTestApp(func(app *fiber.App) {
				tt.registerRoute(app, handler, tt.body)
			})

			var reqBody *bytes.Reader
			if tt.body != nil {
				body, err := json.Marshal(tt.body)
				require.NoError(t, err)
				reqBody = bytes.NewReader(body)
			} else {
				reqBody = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(tt.method, tt.url, reqBody)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			assertJSONErrorResponse(t, resp)
		})
	}
}

func registerGetAllMetadataRoute(app *fiber.App, handler *MetadataIndexHandler, _ any) {
	app.Get("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
		c.SetUserContext(tmcore.ContextWithTenantID(context.Background(), "tenant-1"))

		return handler.GetAllMetadataIndexes(c)
	})
}

func registerDeleteMetadataRoute(app *fiber.App, handler *MetadataIndexHandler, _ any) {
	app.Delete("/v1/settings/metadata-indexes/entities/:entity_name/key/:index_key", func(c *fiber.Ctx) error {
		c.SetUserContext(tmcore.ContextWithTenantID(context.Background(), "tenant-1"))

		return handler.DeleteMetadataIndex(c)
	})
}
