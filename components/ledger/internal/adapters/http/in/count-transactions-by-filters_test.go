// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestTransactionHandler_CountTransactionsByFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID)
		expectedStatus int
		expectedCount  string
	}{
		{
			name:        "success with all filters",
			queryParams: "route=payment&status=APPROVED&start_date=2025-01-01T00:00:00Z&end_date=2025-01-31T23:59:59Z",
			setupMocks: func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRepo.EXPECT().
					CountByFilters(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(int64(100), nil).
					Times(1)
			},
			expectedStatus: 204,
			expectedCount:  "100",
		},
		{
			name:        "success with no filters (defaults)",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRepo.EXPECT().
					CountByFilters(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(int64(5), nil).
					Times(1)
			},
			expectedStatus: 204,
			expectedCount:  "5",
		},
		{
			name:        "success with route only",
			queryParams: "route=transfer",
			setupMocks: func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRepo.EXPECT().
					CountByFilters(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(int64(7), nil).
					Times(1)
			},
			expectedStatus: 204,
			expectedCount:  "7",
		},
		{
			name:        "success with status only",
			queryParams: "status=PENDING",
			setupMocks: func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRepo.EXPECT().
					CountByFilters(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(int64(3), nil).
					Times(1)
			},
			expectedStatus: 204,
			expectedCount:  "3",
		},
		{
			name:        "invalid status returns 400",
			queryParams: "status=INVALID",
			setupMocks: func(_ *transaction.MockRepository, _, _ uuid.UUID) {
				// No repo call expected
			},
			expectedStatus: 400,
		},
		{
			name:        "invalid start_date returns 400",
			queryParams: "start_date=not-a-date",
			setupMocks: func(_ *transaction.MockRepository, _, _ uuid.UUID) {
				// No repo call expected
			},
			expectedStatus: 400,
		},
		{
			name:        "invalid end_date returns 400",
			queryParams: "end_date=not-a-date",
			setupMocks: func(_ *transaction.MockRepository, _, _ uuid.UUID) {
				// No repo call expected
			},
			expectedStatus: 400,
		},
		{
			name:        "start_date after end_date returns 400",
			queryParams: "start_date=2025-12-31T00:00:00Z&end_date=2025-01-01T00:00:00Z",
			setupMocks: func(_ *transaction.MockRepository, _, _ uuid.UUID) {
				// No repo call expected
			},
			expectedStatus: 400,
		},
		{
			name:        "service error returns 500",
			queryParams: "",
			setupMocks: func(transactionRepo *transaction.MockRepository, orgID, ledgerID uuid.UUID) {
				transactionRepo.EXPECT().
					CountByFilters(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(int64(0), pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New()
			ledgerID := uuid.New()

			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			tt.setupMocks(mockTransactionRepo, orgID, ledgerID)

			queryUC := &query.UseCase{
				TransactionRepo: mockTransactionRepo,
			}
			handler := &TransactionHandler{Query: queryUC}

			app := fiber.New()
			app.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgID)
					c.Locals("ledger_id", ledgerID)
					return c.Next()
				},
				handler.CountTransactionsByFilters,
			)

			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/metrics/count"
			if tt.queryParams != "" {
				url += "?" + tt.queryParams
			}

			req := httptest.NewRequest("HEAD", url, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == 204 {
				totalCount := resp.Header.Get(cn.XTotalCount)
				assert.Equal(t, tt.expectedCount, totalCount, "X-Total-Count header should contain the count")

				contentLength := resp.Header.Get(cn.ContentLength)
				assert.Equal(t, "0", contentLength, "Content-Length should be 0")
			}
		})
	}
}
