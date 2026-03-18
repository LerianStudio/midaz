// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestTransactionHandler_CountTransactionsByRoute(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success returns 200 with count",
			queryParams: "?route=550e8400-e29b-41d4-a716-446655440010&status=APPROVED&start_date=2026-01-01T00:00:00Z&end_date=2026-02-01T00:00:00Z",
			setupMocks: func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRepo.EXPECT().
					CountByRoute(gomock.Any(), orgID, ledgerID, "550e8400-e29b-41d4-a716-446655440010", "APPROVED", gomock.Any(), gomock.Any()).
					Return(int64(773), nil).
					Times(1)
			},
			expectedStatus: nethttp.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "totalCount")
				assert.Contains(t, result, "route")
				assert.Contains(t, result, "status")
				assert.Contains(t, result, "period")
				assert.Equal(t, float64(773), result["totalCount"])
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440010", result["route"])
				assert.Equal(t, "APPROVED", result["status"])
			},
		},
		{
			name:           "missing route returns 400",
			queryParams:    "?status=APPROVED&start_date=2026-01-01T00:00:00Z&end_date=2026-02-01T00:00:00Z",
			setupMocks:     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {},
			expectedStatus: nethttp.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp, "code")
			},
		},
		{
			name:           "missing status returns 400",
			queryParams:    "?route=550e8400-e29b-41d4-a716-446655440010&start_date=2026-01-01T00:00:00Z&end_date=2026-02-01T00:00:00Z",
			setupMocks:     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {},
			expectedStatus: nethttp.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp, "code")
			},
		},
		{
			name:           "missing start_date returns 400",
			queryParams:    "?route=550e8400-e29b-41d4-a716-446655440010&status=APPROVED&end_date=2026-02-01T00:00:00Z",
			setupMocks:     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {},
			expectedStatus: nethttp.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp, "code")
			},
		},
		{
			name:           "missing end_date returns 400",
			queryParams:    "?route=550e8400-e29b-41d4-a716-446655440010&status=APPROVED&start_date=2026-01-01T00:00:00Z",
			setupMocks:     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {},
			expectedStatus: nethttp.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp, "code")
			},
		},
		{
			name:           "invalid route UUID returns 400",
			queryParams:    "?route=not-a-uuid&status=APPROVED&start_date=2026-01-01T00:00:00Z&end_date=2026-02-01T00:00:00Z",
			setupMocks:     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {},
			expectedStatus: nethttp.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp, "code")
			},
		},
		{
			name:           "invalid start_date format returns 400",
			queryParams:    "?route=550e8400-e29b-41d4-a716-446655440010&status=APPROVED&start_date=bad-date&end_date=2026-02-01T00:00:00Z",
			setupMocks:     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {},
			expectedStatus: nethttp.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp, "code")
			},
		},
		{
			name:           "invalid end_date format returns 400",
			queryParams:    "?route=550e8400-e29b-41d4-a716-446655440010&status=APPROVED&start_date=2026-01-01T00:00:00Z&end_date=bad-date",
			setupMocks:     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {},
			expectedStatus: nethttp.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp, "code")
			},
		},
		{
			name:           "start_date after end_date returns 400",
			queryParams:    "?route=550e8400-e29b-41d4-a716-446655440010&status=APPROVED&start_date=2026-03-01T00:00:00Z&end_date=2026-01-01T00:00:00Z",
			setupMocks:     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {},
			expectedStatus: nethttp.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp, "code")
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "?route=550e8400-e29b-41d4-a716-446655440010&status=APPROVED&start_date=2026-01-01T00:00:00Z&end_date=2026-02-01T00:00:00Z",
			setupMocks: func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRepo.EXPECT().
					CountByRoute(gomock.Any(), orgID, ledgerID, "550e8400-e29b-41d4-a716-446655440010", "APPROVED", gomock.Any(), gomock.Any()).
					Return(int64(0), pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "The server encountered an unexpected error.",
					}).
					Times(1)
			},
			expectedStatus: nethttp.StatusInternalServerError,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp, "code")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New()
			ledgerID := uuid.New()

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			tt.setupMocks(mockTransactionRepo, orgID, ledgerID)

			uc := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
			}
			handler := &TransactionHandler{Query: uc}

			app := fiber.New()
			app.Get("/test/:organization_id/:ledger_id/transactions/count",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.CountTransactionsByRoute,
			)

			req := httptest.NewRequest("GET",
				"/test/"+orgID.String()+"/"+ledgerID.String()+"/transactions/count"+tt.queryParams,
				nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}
