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

	"github.com/LerianStudio/reporter/components/manager/internal/services"
	"github.com/LerianStudio/reporter/pkg/template_builder"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestTemplateBuilderHandler_ValidateBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           interface{}
		expectedStatus int
		validate       func(t *testing.T, body []byte)
	}{
		{
			name: "Success - valid blocks returns 200 with valid=true",
			body: template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: "Hello World"},
				},
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response template_builder.ValidateBlocksResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err, "response must be valid JSON")
				assert.True(t, response.Valid)
				assert.Empty(t, response.Errors)
			},
		},
		{
			name: "Success - invalid blocks returns 200 with valid=false and errors",
			body: template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: ""},
				},
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response template_builder.ValidateBlocksResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err, "response must be valid JSON")
				assert.False(t, response.Valid)
				require.NotEmpty(t, response.Errors)
				assert.Equal(t, "content", response.Errors[0].Field)
				assert.Equal(t, "Texto deve ter conteudo", response.Errors[0].Message)
			},
		},
		{
			name: "Success - multiple invalid blocks returns all errors",
			body: template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: ""},
					{Type: "variable", Variable: ""},
				},
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response template_builder.ValidateBlocksResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.False(t, response.Valid)
				assert.Len(t, response.Errors, 2)
			},
		},
		{
			name: "Success - empty blocks returns 200 with structured validation failure",
			body: template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{},
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response template_builder.ValidateBlocksResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.False(t, response.Valid)
				require.Len(t, response.Errors, 1)
				assert.Equal(t, "blocks", response.Errors[0].Field)
				assert.Equal(t, "blocks must not be empty", response.Errors[0].Message)
			},
		},
		{
			name:           "Error - invalid JSON body returns 400",
			body:           "not json",
			expectedStatus: http.StatusBadRequest,
			validate:       func(t *testing.T, body []byte) { t.Helper() },
		},
		{
			name: "Success - blockId is present in errors",
			body: template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{BlockID: "my-block", Type: "text", Content: ""},
				},
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response template_builder.ValidateBlocksResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.False(t, response.Valid)
				require.NotEmpty(t, response.Errors)
				assert.Equal(t, "my-block", response.Errors[0].BlockID)
			},
		},
		{
			name: "Success - generated blockId when not provided",
			body: template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: ""},
				},
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()

				var response template_builder.ValidateBlocksResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.False(t, response.Valid)
				require.NotEmpty(t, response.Errors)
				assert.Equal(t, "block-0", response.Errors[0].BlockID)
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
			app.Post("/v1/templates/validate", handler.ValidateBlocks)

			var bodyBytes []byte

			switch v := tt.body.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				bodyBytes, err = json.Marshal(v)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/templates/validate", bytes.NewReader(bodyBytes))
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
