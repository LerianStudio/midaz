// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func bridgeRubrics() *mmodel.AccountingEntry {
	return &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1900", Description: "Arriving from another ledger"},
		Credit: &mmodel.AccountingRubric{Code: "2900", Description: "Leaving to another ledger"},
	}
}

func TestFindUnknownAccountingEntryKeys_AcceptsCrossLedger(t *testing.T) {
	t.Parallel()

	assert.Nil(t, findUnknownAccountingEntryKeys(json.RawMessage(`{"crossLedger":{"debit":{"code":"1900","description":"In"},"credit":{"code":"2900","description":"Out"}}}`)))
	assert.Contains(t, findUnknownAccountingEntryKeys(json.RawMessage(`{"crossLedger":{},"cross_ledger":{}}`)), "cross_ledger")
}

func TestOperationRouteHandler_CrossLedgerEntryRules(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}
	direct := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1000", Description: "Direct debit"},
		Credit: &mmodel.AccountingRubric{Code: "2000", Description: "Direct credit"},
	}

	tests := []struct {
		name          string
		operationType string
		entries       *mmodel.AccountingEntries
		wantCode      string
	}{
		{
			name:          "bidirectional route with only the crossLedger entry is accepted without a direct entry",
			operationType: constant.OperationRouteTypeBidirectional,
			entries:       &mmodel.AccountingEntries{CrossLedger: bridgeRubrics()},
		},
		{
			name:          "crossLedger without the credit rubric is rejected",
			operationType: constant.OperationRouteTypeBidirectional,
			entries:       &mmodel.AccountingEntries{CrossLedger: &mmodel.AccountingEntry{Debit: bridgeRubrics().Debit}},
			wantCode:      constant.ErrAccountingEntryFieldRequired.Error(),
		},
		{
			name:          "crossLedger without the debit rubric is rejected",
			operationType: constant.OperationRouteTypeBidirectional,
			entries:       &mmodel.AccountingEntries{CrossLedger: &mmodel.AccountingEntry{Credit: bridgeRubrics().Credit}},
			wantCode:      constant.ErrAccountingEntryFieldRequired.Error(),
		},
		{
			name:          "crossLedger on a source route is rejected",
			operationType: constant.OperationRouteTypeSource,
			entries:       &mmodel.AccountingEntries{CrossLedger: bridgeRubrics()},
			wantCode:      constant.ErrScenarioNotAllowedForDirection.Error(),
		},
		{
			name:          "crossLedger on a destination route is rejected",
			operationType: constant.OperationRouteTypeDestination,
			entries:       &mmodel.AccountingEntries{CrossLedger: bridgeRubrics()},
			wantCode:      constant.ErrScenarioNotAllowedForDirection.Error(),
		},
		{
			name:          "crossLedger combined with another entry is rejected",
			operationType: constant.OperationRouteTypeBidirectional,
			entries:       &mmodel.AccountingEntries{Direct: direct, CrossLedger: bridgeRubrics()},
			wantCode:      constant.ErrInvalidCrossLedgerRoute.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			err := handler.validateAccountingEntries(ctx, tt.entries)
			if err == nil {
				err = handler.validateAccountingRulesMatrix(ctx, tt.operationType, tt.entries)
			}

			if tt.wantCode == "" {
				require.NoError(t, err)
				return
			}

			var businessErr pkg.UnprocessableOperationError
			require.ErrorAs(t, err, &businessErr)
			assert.Equal(t, tt.wantCode, businessErr.Code)
		})
	}
}

func TestOperationRouteHandler_CrossLedgerEmptyEntryIsStructurallyInvalid(t *testing.T) {
	t.Parallel()

	err := (&OperationRouteHandler{}).validateAccountingEntries(context.Background(), &mmodel.AccountingEntries{CrossLedger: &mmodel.AccountingEntry{}})

	var businessErr pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &businessErr)
	assert.Equal(t, constant.ErrAccountingEntryFieldRequired.Error(), businessErr.Code)
	assert.Contains(t, businessErr.Message, "accountingEntries.crossLedger")
}

func TestMergeAccountingEntries_CarriesCrossLedger(t *testing.T) {
	t.Parallel()

	existing := &mmodel.AccountingEntries{CrossLedger: bridgeRubrics()}
	direct := &mmodel.AccountingEntry{Debit: &mmodel.AccountingRubric{Code: "1000", Description: "Direct"}}

	t.Run("an absent crossLedger key keeps the stored entry", func(t *testing.T) {
		t.Parallel()

		merged := mergeAccountingEntries(existing, &mmodel.AccountingEntries{Direct: direct}, json.RawMessage(`{"direct":{"debit":{"code":"1000","description":"Direct"}}}`))
		require.NotNil(t, merged)
		assert.Equal(t, existing.CrossLedger, merged.CrossLedger)
		assert.Equal(t, direct, merged.Direct)
	})

	t.Run("an explicit null removes the stored entry", func(t *testing.T) {
		t.Parallel()

		merged := mergeAccountingEntries(existing, &mmodel.AccountingEntries{}, json.RawMessage(`{"crossLedger":null}`))
		assert.Nil(t, merged)
	})

	t.Run("an incoming entry is added to a route that had none", func(t *testing.T) {
		t.Parallel()

		incoming := &mmodel.AccountingEntries{CrossLedger: bridgeRubrics()}
		merged := mergeAccountingEntries(&mmodel.AccountingEntries{Direct: direct}, incoming, json.RawMessage(`{"crossLedger":{}}`))
		require.NotNil(t, merged)
		assert.Equal(t, incoming.CrossLedger, merged.CrossLedger)
		assert.Equal(t, direct, merged.Direct)
	})

	t.Run("a stored block entry stays in the view that adding crossLedger is validated against", func(t *testing.T) {
		t.Parallel()

		stored := &mmodel.AccountingEntries{Block: &mmodel.AccountingEntry{Debit: direct.Debit, Credit: direct.Debit}}
		merged := mergeAccountingEntries(stored, &mmodel.AccountingEntries{CrossLedger: bridgeRubrics()}, json.RawMessage(`{"crossLedger":{}}`))

		err := (&OperationRouteHandler{}).validateAccountingRulesMatrix(context.Background(), constant.OperationRouteTypeBidirectional, merged)

		var businessErr pkg.UnprocessableOperationError
		require.ErrorAs(t, err, &businessErr)
		assert.Equal(t, constant.ErrInvalidCrossLedgerRoute.Error(), businessErr.Code)
	})

	t.Run("the fallback merge keeps both sides", func(t *testing.T) {
		t.Parallel()

		merged := mergeAccountingEntries(existing, &mmodel.AccountingEntries{Direct: direct}, nil)
		require.NotNil(t, merged)
		assert.Equal(t, existing.CrossLedger, merged.CrossLedger)
	})
}

func TestCreateOperationRoute_CrossLedgerEntry(t *testing.T) {
	// NOT parallel: buildHumaOperationRouteApp mutates process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	path := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/operation-routes"
	bridge := map[string]any{
		"debit":  map[string]any{"code": "1900", "description": "Arriving from another ledger"},
		"credit": map[string]any{"code": "2900", "description": "Leaving to another ledger"},
	}

	post := func(t *testing.T, handler *OperationRouteHandler, payload map[string]any) (int, map[string]any) {
		t.Helper()

		body, err := json.Marshal(payload)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := buildHumaOperationRouteApp(t, handler, true).Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(raw, &decoded), "body: %s", string(raw))

		return resp.StatusCode, decoded
	}

	t.Run("a bidirectional bridge route is created with its crossLedger entry", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		orRepo := operationroute.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		orRepo.EXPECT().Create(gomock.Any(), orgID, &ledgerID, gomock.Any()).
			DoAndReturn(func(_ any, oID uuid.UUID, lID *uuid.UUID, or *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
				require.NotNil(t, or.AccountingEntries)
				require.NotNil(t, or.AccountingEntries.CrossLedger)
				or.ID = uuid.Must(libCommons.GenerateUUIDv7())
				or.OrganizationID = oID
				or.LedgerID = lID
				or.CreatedAt = fixedTestTime
				or.UpdatedAt = fixedTestTime

				return or, nil
			})
		metaRepo.EXPECT().Create(gomock.Any(), constant.EntityOperationRoute, gomock.Any()).Return(nil)

		status, body := post(t, &OperationRouteHandler{Command: &command.UseCase{OperationRouteRepo: orRepo, TransactionMetadataRepo: metaRepo}}, map[string]any{
			"title": "Cross-ledger bridge", "operationType": "bidirectional",
			"accountingEntries": map[string]any{"crossLedger": bridge},
		})

		require.Equal(t, http.StatusCreated, status, "body: %v", body)
		entries, ok := body["accountingEntries"].(map[string]any)
		require.True(t, ok, "body: %v", body)
		assert.Contains(t, entries, "crossLedger")
	})

	t.Run("a source route cannot carry the crossLedger entry", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		status, body := post(t, &OperationRouteHandler{Command: &command.UseCase{
			OperationRouteRepo:      operationroute.NewMockRepository(ctrl),
			TransactionMetadataRepo: mongodb.NewMockRepository(ctrl),
		}}, map[string]any{
			"title": "Cross-ledger bridge", "operationType": "source",
			"accountingEntries": map[string]any{"crossLedger": bridge},
		})

		assert.Equal(t, http.StatusUnprocessableEntity, status)
		assert.Equal(t, constant.ErrScenarioNotAllowedForDirection.Error(), body["code"])
	})
}
