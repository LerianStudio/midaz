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

	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	midazhttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubHolderAccountsReader is a hand-written stub for HolderAccountsReader.
// It captures the org ID and holder filter the handler forwards, and returns a
// canned account slice or error.
type stubHolderAccountsReader struct {
	accounts []*mmodel.Account
	err      error

	gotOrganizationID string
	gotHolderID       uuid.UUID
	gotHolderFilter   *string
}

func (s *stubHolderAccountsReader) ListAccountsByHolder(_ context.Context, organizationID string, holderID uuid.UUID, filter midazhttp.QueryHeader) ([]*mmodel.Account, error) {
	s.gotOrganizationID = organizationID
	s.gotHolderID = holderID
	s.gotHolderFilter = filter.HolderID

	return s.accounts, s.err
}

func TestHolderAccountsHandler_GetAccountsByHolder(t *testing.T) {
	tests := []struct {
		name           string
		reader         *stubHolderAccountsReader
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with matching accounts",
			reader: &stubHolderAccountsReader{
				accounts: []*mmodel.Account{
					{ID: uuid.New().String(), Name: "Wallet"},
				},
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				require.NoError(t, json.Unmarshal(body, &result))

				assert.Contains(t, result, "items", "response should contain items")
				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 1)
			},
		},
		{
			name: "empty result returns 200 with empty list",
			reader: &stubHolderAccountsReader{
				accounts: []*mmodel.Account{},
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				require.NoError(t, json.Unmarshal(body, &result))

				assert.Contains(t, result, "items", "response should contain items")
			},
		},
		{
			name: "not found returns 404",
			reader: &stubHolderAccountsReader{
				err: pkg.ValidateBusinessError(cn.ErrNoAccountsFound, cn.EntityAccount),
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(body, &errResp))

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrNoAccountsFound.Error(), errResp["code"])
			},
		},
		{
			name: "reader error returns 500",
			reader: &stubHolderAccountsReader{
				err: pkg.InternalServerError{
					Code:    "0046",
					Title:   "Internal Server Error",
					Message: "Database connection failed",
				},
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(body, &errResp))

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orgID := uuid.New().String()
			holderID := uuid.New()

			handler := &HolderAccountsHandler{Reader: tt.reader}

			app := fiber.New()
			app.Get("/v1/holders/:id/accounts",
				func(c *fiber.Ctx) error {
					c.Locals("id", holderID)
					c.Request().Header.Set("X-Organization-Id", orgID)

					return c.Next()
				},
				handler.GetAccountsByHolder,
			)

			req := httptest.NewRequest("GET", "/v1/holders/"+holderID.String()+"/accounts", nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// The handler must forward the path holder ID and the org header to
			// the org-scoped reader, and stamp the holder filter from the path.
			assert.Equal(t, orgID, tt.reader.gotOrganizationID)
			assert.Equal(t, holderID, tt.reader.gotHolderID)
			require.NotNil(t, tt.reader.gotHolderFilter, "handler should set the holder filter")
			assert.Equal(t, holderID.String(), *tt.reader.gotHolderFilter)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}
