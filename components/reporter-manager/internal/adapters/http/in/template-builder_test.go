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

	"github.com/LerianStudio/midaz/v3/components/reporter-manager/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/pongo"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/template_builder"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestNewTemplateBuilderHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		service   *services.UseCase
		expectErr bool
	}{
		{
			name:      "Success - creates handler with valid service",
			service:   &services.UseCase{},
			expectErr: false,
		},
		{
			name:      "Error - nil service returns error",
			service:   nil,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, err := NewTemplateBuilderHandler(tt.service)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, handler)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, handler)
			}
		})
	}
}

func TestTemplateBuilderHandler_GetBlocksConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		expectedStatus int
		validate       func(t *testing.T, body []byte)
	}{
		{
			name:           "Success - returns 200 with blocks config",
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response pongo.BlocksConfigResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err, "response must be valid JSON")
				assert.Len(t, response.Blocks, 13, "must return exactly 13 block types")
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &services.UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			handler, err := NewTemplateBuilderHandler(svc)
			require.NoError(t, err)

			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Get("/v1/templates/blocks-config", handler.GetBlocksConfig)

			req := httptest.NewRequest(http.MethodGet, "/v1/templates/blocks-config", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			tt.validate(t, body)
		})
	}
}

func TestTemplateBuilderHandler_GetFiltersConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		expectedStatus int
		validate       func(t *testing.T, body []byte)
	}{
		{
			name:           "Success - returns 200 with filters including DIMP",
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response pongo.FiltersResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err, "response must be valid JSON")

				filterNames := make([]string, len(response.Filters))
				for i, f := range response.Filters {
					filterNames[i] = f.Name
				}

				dimpFilters := []string{"replace", "where", "sum", "count"}
				for _, df := range dimpFilters {
					assert.Contains(t, filterNames, df, "must contain DIMP filter: %s", df)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &services.UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			handler, err := NewTemplateBuilderHandler(svc)
			require.NoError(t, err)

			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Get("/v1/templates/filters", handler.GetFiltersConfig)

			req := httptest.NewRequest(http.MethodGet, "/v1/templates/filters", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			tt.validate(t, body)
		})
	}
}

func TestTemplateBuilderHandler_GenerateCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           interface{}
		expectedStatus int
		validate       func(t *testing.T, body []byte)
	}{
		{
			name: "Success - generates code for text block",
			body: template_builder.GenerateCodeInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: "Hello World"},
				},
				Format: "",
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response template_builder.GenerateCodeResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err, "response must be valid JSON")
				assert.Equal(t, "Hello World\n", response.Code)
				assert.NotNil(t, response.MappedFields)
			},
		},
		{
			name: "Success - generates code with variable and extracts mapped fields",
			body: template_builder.GenerateCodeInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "variable", Variable: "orders.total"},
				},
				Format: "",
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response template_builder.GenerateCodeResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Contains(t, response.Code, "{{ orders.total }}")
				require.Contains(t, response.MappedFields, "default")
				require.Contains(t, response.MappedFields["default"], "orders")
				assert.Contains(t, response.MappedFields["default"]["orders"], "total")
			},
		},
		{
			name: "Success - generates HTML wrapped code",
			body: template_builder.GenerateCodeInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: "Report"},
				},
				Format: "html",
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response template_builder.GenerateCodeResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Contains(t, response.Code, "<!DOCTYPE html>")
				assert.Contains(t, response.Code, "Report")
			},
		},
		{
			name: "Error - empty blocks returns 400",
			body: template_builder.GenerateCodeInput{
				Blocks: []template_builder.TemplateBlock{},
				Format: "",
			},
			expectedStatus: http.StatusBadRequest,
			validate:       func(t *testing.T, body []byte) { t.Helper() },
		},
		{
			name:           "Error - invalid JSON body returns 400",
			body:           "not json",
			expectedStatus: http.StatusBadRequest,
			validate:       func(t *testing.T, body []byte) { t.Helper() },
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &services.UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			handler, err := NewTemplateBuilderHandler(svc)
			require.NoError(t, err)

			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Post("/v1/templates/generate-code", handler.GenerateCode)

			var bodyBytes []byte

			switch v := tt.body.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				bodyBytes, err = json.Marshal(v)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/templates/generate-code", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			tt.validate(t, respBody)
		})
	}
}
