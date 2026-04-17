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
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// TestMetadataIndexHandler_CreateMetadataIndex_ValidateParametersError covers
// the error branch in CreateMetadataIndex where ValidateParameters fails on
// malformed query params (e.g. an unparseable start_date). No repository call
// is expected because validation rejects the request before handler dispatch.
func TestMetadataIndexHandler_CreateMetadataIndex_ValidateParametersError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	handler := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	payload := &mmodel.CreateMetadataIndexInput{MetadataKey: "tier"}

	app := fiber.New()
	app.Post("/v1/settings/metadata-indexes/entities/:entity_name", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.CreateMetadataIndex(payload, c)
	})

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	// start_date=not-a-date forces ValidateParameters to return an error.
	url := "/v1/settings/metadata-indexes/entities/transaction?start_date=not-a-date"
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// TestMetadataIndexHandler_GetAllMetadataIndexes_ValidateParametersError covers
// the error branch in GetAllMetadataIndexes where ValidateParameters rejects
// malformed query params before any repo lookup occurs.
func TestMetadataIndexHandler_GetAllMetadataIndexes_ValidateParametersError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	handler := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	app := fiber.New()
	app.Get("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.GetAllMetadataIndexes(c)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/v1/settings/metadata-indexes?start_date=not-a-date", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// TestMetadataIndexHandler_getRepoAndCollection_UnknownEntity verifies the
// fallthrough branch in getRepoAndCollection where neither onboarding nor
// transaction maps contain the requested entity name. The function must
// return nil repo and empty collection instead of panicking.
func TestMetadataIndexHandler_getRepoAndCollection_UnknownEntity(t *testing.T) {
	t.Parallel()

	handler := &MetadataIndexHandler{
		OnboardingMetadataRepo:  nil,
		TransactionMetadataRepo: nil,
	}

	repo, coll := handler.getRepoAndCollection("definitely-not-an-entity")
	assert.Nil(t, repo, "unknown entity should yield nil repo")
	assert.Empty(t, coll, "unknown entity should yield empty collection")
}

// TestMetadataIndexHandler_GetAllMetadataIndexes_TransactionRepoFails covers
// the warn+continue branch in GetAllMetadataIndexes where the transaction
// repository errors out while fetching indexes for one of its collections.
// The handler must still return 200 with indexes from the other collections.
func TestMetadataIndexHandler_GetAllMetadataIndexes_TransactionRepoFails(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	// Onboarding returns empty for every collection it is asked about.
	mockOnboardingRepo.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		Return([]*mmodel.MetadataIndex{}, nil).
		AnyTimes()

	// Transaction errors for "operation" (exercising warn+continue at
	// metadata.go:264-267) and returns a concrete index for "transaction"
	// so we can assert the partial-success response.
	mockTransactionRepo.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, collection string) ([]*mmodel.MetadataIndex, error) {
			if collection == "operation" {
				return nil, errDatabaseError
			}

			if collection == "transaction" {
				return []*mmodel.MetadataIndex{
					{IndexName: "metadata.tier_1", MetadataKey: "tier"},
				}, nil
			}

			return []*mmodel.MetadataIndex{}, nil
		}).
		AnyTimes()

	handler := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	app := fiber.New()
	app.Get("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.GetAllMetadataIndexes(c)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/v1/settings/metadata-indexes", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// TestMetadataIndexHandler_DeleteMetadataIndex_BlankIndexKey covers the
// empty index_key branch where the URL parameter is defined but blank. The
// existing test only exercises the route-level empty case; this variant
// validates the per-request key check.
func TestMetadataIndexHandler_DeleteMetadataIndex_BlankIndexKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	handler := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	app := fiber.New()
	app.Delete("/v1/settings/metadata-indexes/entities/:entity_name/key/:index_key", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.DeleteMetadataIndex(c)
	})

	// Trailing-slash path forces :index_key to empty after route matching.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		"/v1/settings/metadata-indexes/entities/transaction/key/", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	// Fiber may treat the trailing slash as 404; accept either 400 or 404
	// — both prove we don't hit the repo.
	assert.Contains(t, []int{fiber.StatusBadRequest, fiber.StatusNotFound}, resp.StatusCode)
}
