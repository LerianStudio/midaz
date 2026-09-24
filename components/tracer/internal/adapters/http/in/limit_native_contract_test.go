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
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNativeLimitCreateContract(t *testing.T) {
	for _, code := range []string{"wBTC", "POINTS", "BTC", strings.Repeat("x", 256), " BTC", strings.Repeat("é", 129)} {
		t.Run(code, func(t *testing.T) {
			service := NewMockLimitService(gomock.NewController(t))
			valid := code != " BTC" && len(code) <= 256
			if valid {
				service.EXPECT().CreateLimit(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *command.CreateLimitInput) (*model.Limit, error) {
					require.Equal(t, code, input.Asset)
					limit := validLimit(testutil.MustDeterministicUUID(89010))
					limit.Asset = code
					return limit, nil
				})
			}
			app := buildHumaLimitApp(t, service, "tenant")
			var body map[string]any
			require.NoError(t, json.Unmarshal(validCreateLimitBody(), &body))
			body["asset"] = code
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "/v1/limits", bytes.NewReader(raw))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()
			if valid {
				require.Equal(t, http.StatusCreated, response.StatusCode)
			} else {
				require.Equal(t, http.StatusBadRequest, response.StatusCode)
			}
		})
	}
}

func TestNativeLimitJSONSchema(t *testing.T) {
	app := fiber.New()
	api := openapi.New(app, app.Group("/v1"), openapi.Config{Title: "test", Version: "test"})
	RegisterLimitRoutes(api, NewLimitHandler(NewMockLimitService(gomock.NewController(t))))
	schema := api.OpenAPI().Paths["/limits"].Post.RequestBody.Content["application/json"].Schema
	require.Equal(t, "#/components/schemas/CreateLimitInput", schema.Ref)
	document := api.OpenAPI().Components.Schemas.Map()["CreateLimitInput"]
	require.NotNil(t, document)
	require.Equal(t, 1, *document.Properties["asset"].MinLength)
	require.Equal(t, 256, *document.Properties["asset"].MaxLength)
	require.Equal(t, "string", document.Properties["maxAmount"].Type)
}
