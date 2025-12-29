package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestMetadataIndexHandler_CreateMetadataIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mbootstrap.NewMockMetadataIndexPort(ctrl)

	handler := &MetadataIndexHandler{
		MetadataIndexPort: mockPort,
	}

	t.Run("success", func(t *testing.T) {
		app := fiber.New()

		app.Post("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())

			var input mmodel.CreateMetadataIndexInput
			if err := c.BodyParser(&input); err != nil {
				return err
			}

			return handler.CreateMetadataIndex(&input, c)
		})

		input := mmodel.CreateMetadataIndexInput{
			EntityName:  "transaction",
			MetadataKey: "tier",
			Unique:      false,
		}

		expectedResult := &mmodel.MetadataIndex{
			IndexName:   "metadata.tier_1",
			EntityName:  "transaction",
			MetadataKey: "tier",
			Unique:      false,
			Sparse:      true,
		}

		mockPort.EXPECT().
			CreateMetadataIndex(gomock.Any(), gomock.Any()).
			Return(expectedResult, nil)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/v1/settings/metadata-indexes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
	})

	t.Run("error - port failure", func(t *testing.T) {
		app := fiber.New()

		app.Post("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())

			var input mmodel.CreateMetadataIndexInput
			if err := c.BodyParser(&input); err != nil {
				return err
			}

			return handler.CreateMetadataIndex(&input, c)
		})

		input := mmodel.CreateMetadataIndexInput{
			EntityName:  "transaction",
			MetadataKey: "tier",
			Unique:      false,
		}

		mockPort.EXPECT().
			CreateMetadataIndex(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("index already exists"))

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/v1/settings/metadata-indexes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("error - invalid payload type", func(t *testing.T) {
		app := fiber.New()

		app.Post("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())

			invalidPayload := "invalid"

			return handler.CreateMetadataIndex(invalidPayload, c)
		})

		req := httptest.NewRequest("POST", "/v1/settings/metadata-indexes", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})

	t.Run("error - nil payload", func(t *testing.T) {
		app := fiber.New()

		app.Post("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())

			return handler.CreateMetadataIndex(nil, c)
		})

		req := httptest.NewRequest("POST", "/v1/settings/metadata-indexes", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})
}

func TestMetadataIndexHandler_GetAllMetadataIndexes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mbootstrap.NewMockMetadataIndexPort(ctrl)

	handler := &MetadataIndexHandler{
		MetadataIndexPort: mockPort,
	}

	t.Run("success", func(t *testing.T) {
		app := fiber.New()

		app.Get("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())

			return handler.GetAllMetadataIndexes(c)
		})

		expectedResult := []*mmodel.MetadataIndex{
			{
				IndexName:   "metadata.tier_1",
				EntityName:  "transaction",
				MetadataKey: "tier",
				Unique:      false,
				Sparse:      true,
			},
		}

		mockPort.EXPECT().
			GetAllMetadataIndexes(gomock.Any(), gomock.Any()).
			Return(expectedResult, nil)

		req := httptest.NewRequest("GET", "/v1/settings/metadata-indexes", nil)

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		respBody, _ := io.ReadAll(resp.Body)
		var result []*mmodel.MetadataIndex
		json.Unmarshal(respBody, &result)

		assert.Len(t, result, 1)
		assert.Equal(t, "metadata.tier_1", result[0].IndexName)
	})

	t.Run("success - with entity filter", func(t *testing.T) {
		app := fiber.New()

		app.Get("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())

			return handler.GetAllMetadataIndexes(c)
		})

		expectedResult := []*mmodel.MetadataIndex{
			{
				IndexName:   "metadata.tier_1",
				EntityName:  "operation",
				MetadataKey: "tier",
				Unique:      false,
				Sparse:      true,
			},
		}

		mockPort.EXPECT().
			GetAllMetadataIndexes(gomock.Any(), gomock.Any()).
			Return(expectedResult, nil)

		req := httptest.NewRequest("GET", "/v1/settings/metadata-indexes?entity_name=operation", nil)

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	})

	t.Run("error - port failure", func(t *testing.T) {
		app := fiber.New()

		app.Get("/v1/settings/metadata-indexes", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())

			return handler.GetAllMetadataIndexes(c)
		})

		mockPort.EXPECT().
			GetAllMetadataIndexes(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("database error"))

		req := httptest.NewRequest("GET", "/v1/settings/metadata-indexes", nil)

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	})
}

func TestMetadataIndexHandler_DeleteMetadataIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mbootstrap.NewMockMetadataIndexPort(ctrl)

	handler := &MetadataIndexHandler{
		MetadataIndexPort: mockPort,
	}

	t.Run("success", func(t *testing.T) {
		app := fiber.New()

		app.Delete("/v1/settings/metadata-indexes/:index_name", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())
			c.Locals("index_name", c.Params("index_name"))

			return handler.DeleteMetadataIndex(c)
		})

		mockPort.EXPECT().
			DeleteMetadataIndex(gomock.Any(), "transaction", "metadata.tier_1").
			Return(nil)

		req := httptest.NewRequest("DELETE", "/v1/settings/metadata-indexes/metadata.tier_1?entity_name=transaction", nil)

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
	})

	t.Run("error - missing entity_name", func(t *testing.T) {
		app := fiber.New()

		app.Delete("/v1/settings/metadata-indexes/:index_name", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())
			c.Locals("index_name", c.Params("index_name"))

			return handler.DeleteMetadataIndex(c)
		})

		req := httptest.NewRequest("DELETE", "/v1/settings/metadata-indexes/metadata.tier_1", nil)

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.NotEqual(t, fiber.StatusNoContent, resp.StatusCode)
	})

	t.Run("error - invalid entity_name", func(t *testing.T) {
		app := fiber.New()

		app.Delete("/v1/settings/metadata-indexes/:index_name", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())
			c.Locals("index_name", c.Params("index_name"))

			return handler.DeleteMetadataIndex(c)
		})

		req := httptest.NewRequest("DELETE", "/v1/settings/metadata-indexes/metadata.tier_1?entity_name=invalid", nil)

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.NotEqual(t, fiber.StatusNoContent, resp.StatusCode)
	})

	t.Run("error - port failure", func(t *testing.T) {
		app := fiber.New()

		app.Delete("/v1/settings/metadata-indexes/:index_name", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())
			c.Locals("index_name", c.Params("index_name"))

			return handler.DeleteMetadataIndex(c)
		})

		mockPort.EXPECT().
			DeleteMetadataIndex(gomock.Any(), "transaction", "metadata.tier_1").
			Return(errors.New("index not found"))

		req := httptest.NewRequest("DELETE", "/v1/settings/metadata-indexes/metadata.tier_1?entity_name=transaction", nil)

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("error - index_name not set in locals", func(t *testing.T) {
		app := fiber.New()

		app.Delete("/v1/settings/metadata-indexes/:index_name", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())

			return handler.DeleteMetadataIndex(c)
		})

		req := httptest.NewRequest("DELETE", "/v1/settings/metadata-indexes/metadata.tier_1?entity_name=transaction", nil)

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})

	t.Run("error - index_name wrong type in locals", func(t *testing.T) {
		app := fiber.New()

		app.Delete("/v1/settings/metadata-indexes/:index_name", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())
			c.Locals("index_name", 12345)

			return handler.DeleteMetadataIndex(c)
		})

		req := httptest.NewRequest("DELETE", "/v1/settings/metadata-indexes/metadata.tier_1?entity_name=transaction", nil)

		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})
}

// Dummy for http.QueryHeader used in tests
var _ http.QueryHeader
