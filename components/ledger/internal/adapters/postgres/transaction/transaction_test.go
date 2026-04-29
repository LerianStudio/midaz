// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"database/sql"
	"testing"
	"time"

	constant "github.com/LerianStudio/lib-commons/v4/commons/constants"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	pkgConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ptr is a helper function that returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

func TestStatus_IsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   Status
		expected bool
	}{
		{
			name:     "empty code and nil description",
			status:   Status{Code: "", Description: nil},
			expected: true,
		},
		{
			name:     "non-empty code and nil description",
			status:   Status{Code: "ACTIVE", Description: nil},
			expected: false,
		},
		{
			name:     "empty code and non-nil description",
			status:   Status{Code: "", Description: ptr("Some description")},
			expected: false,
		},
		{
			name:     "non-empty code and non-nil description",
			status:   Status{Code: "PENDING", Description: ptr("Pending approval")},
			expected: false,
		},
		{
			name:     "empty code and empty string description",
			status:   Status{Code: "", Description: ptr("")},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.status.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransactionPostgreSQLModel_ToEntity(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	transactionID := uuid.New().String()
	parentID := uuid.New().String()
	ledgerID := uuid.New().String()
	organizationID := uuid.New().String()
	amount := decimal.NewFromInt(1000)

	tests := []struct {
		name  string
		model TransactionPostgreSQLModel
	}{
		{
			name: "minimal transaction",
			model: TransactionPostgreSQLModel{
				ID:             transactionID,
				Description:    "Test transaction",
				Status:         "ACTIVE",
				AssetCode:      "USD",
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		{
			name: "transaction with parent ID",
			model: TransactionPostgreSQLModel{
				ID:                  transactionID,
				ParentTransactionID: &parentID,
				Description:         "Child transaction",
				Status:              "PENDING",
				AssetCode:           "EUR",
				LedgerID:            ledgerID,
				OrganizationID:      organizationID,
				CreatedAt:           now,
				UpdatedAt:           now,
			},
		},
		{
			name: "transaction with amount and status description",
			model: TransactionPostgreSQLModel{
				ID:                transactionID,
				Description:       "Transaction with amount",
				Status:            "COMPLETED",
				StatusDescription: ptr("Successfully completed"),
				Amount:            &amount,
				AssetCode:         "BRL",
				LedgerID:          ledgerID,
				OrganizationID:    organizationID,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		},
		{
			name: "transaction with route",
			model: TransactionPostgreSQLModel{
				ID:             transactionID,
				Description:    "Routed transaction",
				Status:         "ACTIVE",
				AssetCode:      "USD",
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				Route:          ptr("route-123"),
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		{
			name: "transaction with deleted at",
			model: TransactionPostgreSQLModel{
				ID:             transactionID,
				Description:    "Deleted transaction",
				Status:         "DELETED",
				AssetCode:      "USD",
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      sql.NullTime{Time: now, Valid: true},
			},
		},
		{
			name: "transaction with body",
			model: TransactionPostgreSQLModel{
				ID:             transactionID,
				Description:    "Transaction with body",
				Status:         "ACTIVE",
				AssetCode:      "USD",
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				Body: &mtransaction.Transaction{
					ChartOfAccountsGroupName: "FUNDING",
					Description:              "Body description",
					Code:                     "TX-001",
					Send: mtransaction.Send{
						Asset: "USD",
						Value: decimal.NewFromInt(500),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		{
			name: "transaction with chart of accounts group name",
			model: TransactionPostgreSQLModel{
				ID:                       transactionID,
				Description:              "Transaction with COA",
				Status:                   "ACTIVE",
				AssetCode:                "USD",
				ChartOfAccountsGroupName: "REVENUE",
				LedgerID:                 ledgerID,
				OrganizationID:           organizationID,
				CreatedAt:                now,
				UpdatedAt:                now,
			},
		},
		{
			name: "transaction with all fields",
			model: TransactionPostgreSQLModel{
				ID:                       transactionID,
				ParentTransactionID:      &parentID,
				Description:              "Full transaction",
				Status:                   "COMPLETED",
				StatusDescription:        ptr("All fields populated"),
				Amount:                   &amount,
				AssetCode:                "USD",
				ChartOfAccountsGroupName: "EXPENSES",
				LedgerID:                 ledgerID,
				OrganizationID:           organizationID,
				Body: &mtransaction.Transaction{
					ChartOfAccountsGroupName: "EXPENSES",
					Description:              "Full body",
					Send: mtransaction.Send{
						Asset: "USD",
						Value: decimal.NewFromInt(1000),
					},
				},
				Route:     ptr("route-456"),
				CreatedAt: now,
				UpdatedAt: now,
				DeletedAt: sql.NullTime{Time: now, Valid: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entity := tt.model.ToEntity()

			require.NotNil(t, entity)
			assert.Equal(t, tt.model.ID, entity.ID)
			assert.Equal(t, tt.model.ParentTransactionID, entity.ParentTransactionID)
			assert.Equal(t, tt.model.Description, entity.Description)
			assert.Equal(t, tt.model.Status, entity.Status.Code)
			assert.Equal(t, tt.model.StatusDescription, entity.Status.Description)
			assert.Equal(t, tt.model.Amount, entity.Amount)
			assert.Equal(t, tt.model.AssetCode, entity.AssetCode)
			assert.Equal(t, tt.model.ChartOfAccountsGroupName, entity.ChartOfAccountsGroupName)
			assert.Equal(t, tt.model.LedgerID, entity.LedgerID)
			assert.Equal(t, tt.model.OrganizationID, entity.OrganizationID)
			assert.Equal(t, tt.model.CreatedAt, entity.CreatedAt)
			assert.Equal(t, tt.model.UpdatedAt, entity.UpdatedAt)

			// Check Route
			if tt.model.Route != nil {
				assert.Equal(t, *tt.model.Route, entity.Route)
			} else {
				assert.Empty(t, entity.Route)
			}

			// Check DeletedAt
			if tt.model.DeletedAt.Valid && !tt.model.DeletedAt.Time.IsZero() {
				require.NotNil(t, entity.DeletedAt)
				assert.Equal(t, tt.model.DeletedAt.Time, *entity.DeletedAt)
			} else {
				assert.Nil(t, entity.DeletedAt)
			}

			// Check Body
			if tt.model.Body != nil && !tt.model.Body.IsEmpty() {
				assert.False(t, entity.Body.IsEmpty())
				assert.Equal(t, tt.model.Body.Description, entity.Body.Description)
			}
		})
	}
}

func TestTransactionPostgreSQLModel_FromEntity(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	existingID := uuid.New().String()
	parentID := uuid.New().String()
	ledgerID := uuid.New().String()
	organizationID := uuid.New().String()
	amount := decimal.NewFromInt(1500)

	tests := []struct {
		name               string
		entity             *Transaction
		expectGeneratedID  bool
		expectedIDOverride string
	}{
		{
			name: "minimal entity",
			entity: &Transaction{
				Description:    "Minimal",
				Status:         Status{Code: "ACTIVE"},
				AssetCode:      "USD",
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			expectGeneratedID: true,
		},
		{
			name: "entity with existing ID",
			entity: &Transaction{
				ID:             existingID,
				Description:    "With ID",
				Status:         Status{Code: "PENDING"},
				AssetCode:      "EUR",
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			expectGeneratedID:  false,
			expectedIDOverride: existingID,
		},
		{
			name: "entity with parent transaction ID",
			entity: &Transaction{
				ID:                  existingID,
				ParentTransactionID: &parentID,
				Description:         "Child entity",
				Status:              Status{Code: "ACTIVE"},
				AssetCode:           "USD",
				LedgerID:            ledgerID,
				OrganizationID:      organizationID,
				CreatedAt:           now,
				UpdatedAt:           now,
			},
			expectGeneratedID:  false,
			expectedIDOverride: existingID,
		},
		{
			name: "entity with amount and status description",
			entity: &Transaction{
				ID:          existingID,
				Description: "With amount",
				Status: Status{
					Code:        "COMPLETED",
					Description: ptr("Done"),
				},
				Amount:         &amount,
				AssetCode:      "BRL",
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			expectGeneratedID:  false,
			expectedIDOverride: existingID,
		},
		{
			name: "entity with body",
			entity: &Transaction{
				ID:          existingID,
				Description: "With body",
				Status:      Status{Code: "ACTIVE"},
				AssetCode:   "USD",
				Body: mtransaction.Transaction{
					ChartOfAccountsGroupName: "FUNDING",
					Description:              "Body content",
					Send: mtransaction.Send{
						Asset: "USD",
						Value: decimal.NewFromInt(100),
					},
				},
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			expectGeneratedID:  false,
			expectedIDOverride: existingID,
		},
		{
			name: "entity with route",
			entity: &Transaction{
				ID:             existingID,
				Description:    "With route",
				Status:         Status{Code: "ACTIVE"},
				AssetCode:      "USD",
				Route:          "route-789",
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			expectGeneratedID:  false,
			expectedIDOverride: existingID,
		},
		{
			name: "entity with deleted at",
			entity: &Transaction{
				ID:             existingID,
				Description:    "Deleted entity",
				Status:         Status{Code: "DELETED"},
				AssetCode:      "USD",
				LedgerID:       ledgerID,
				OrganizationID: organizationID,
				CreatedAt:      now,
				UpdatedAt:      now,
				DeletedAt:      &now,
			},
			expectGeneratedID:  false,
			expectedIDOverride: existingID,
		},
		{
			name: "entity with chart of accounts group name",
			entity: &Transaction{
				ID:                       existingID,
				Description:              "With COA",
				Status:                   Status{Code: "ACTIVE"},
				AssetCode:                "USD",
				ChartOfAccountsGroupName: "LIABILITIES",
				LedgerID:                 ledgerID,
				OrganizationID:           organizationID,
				CreatedAt:                now,
				UpdatedAt:                now,
			},
			expectGeneratedID:  false,
			expectedIDOverride: existingID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var model TransactionPostgreSQLModel
			model.FromEntity(tt.entity)

			if tt.expectGeneratedID {
				assert.NotEmpty(t, model.ID)
				_, err := uuid.Parse(model.ID)
				assert.NoError(t, err, "Generated ID should be valid UUID")
			} else {
				assert.Equal(t, tt.expectedIDOverride, model.ID)
			}

			assert.Equal(t, tt.entity.ParentTransactionID, model.ParentTransactionID)
			assert.Equal(t, tt.entity.Description, model.Description)
			assert.Equal(t, tt.entity.Status.Code, model.Status)
			assert.Equal(t, tt.entity.Status.Description, model.StatusDescription)
			assert.Equal(t, tt.entity.Amount, model.Amount)
			assert.Equal(t, tt.entity.AssetCode, model.AssetCode)
			assert.Equal(t, tt.entity.ChartOfAccountsGroupName, model.ChartOfAccountsGroupName)
			assert.Equal(t, tt.entity.LedgerID, model.LedgerID)
			assert.Equal(t, tt.entity.OrganizationID, model.OrganizationID)
			assert.Equal(t, tt.entity.CreatedAt, model.CreatedAt)
			assert.Equal(t, tt.entity.UpdatedAt, model.UpdatedAt)

			// Check Route
			if tt.entity.Route != "" {
				require.NotNil(t, model.Route)
				assert.Equal(t, tt.entity.Route, *model.Route)
			} else {
				assert.Nil(t, model.Route)
			}

			// Check DeletedAt
			if tt.entity.DeletedAt != nil {
				assert.True(t, model.DeletedAt.Valid)
				assert.Equal(t, *tt.entity.DeletedAt, model.DeletedAt.Time)
			} else {
				assert.False(t, model.DeletedAt.Valid)
			}

			// Check Body
			if !tt.entity.Body.IsEmpty() {
				require.NotNil(t, model.Body)
				assert.Equal(t, tt.entity.Body.Description, model.Body.Description)
			}
		})
	}
}

func TestTransactionPostgreSQLModel_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	transactionID := uuid.New().String()
	parentID := uuid.New().String()
	ledgerID := uuid.New().String()
	organizationID := uuid.New().String()
	amount := decimal.NewFromInt(2500)

	originalEntity := &Transaction{
		ID:                       transactionID,
		ParentTransactionID:      &parentID,
		Description:              "Round trip test",
		Status:                   Status{Code: "ACTIVE", Description: ptr("Active transaction")},
		Amount:                   &amount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "FUNDING",
		LedgerID:                 ledgerID,
		OrganizationID:           organizationID,
		Route:                    "route-roundtrip",
		Body: mtransaction.Transaction{
			ChartOfAccountsGroupName: "FUNDING",
			Description:              "Round trip body",
			Code:                     "RT-001",
			Send: mtransaction.Send{
				Asset: "USD",
				Value: decimal.NewFromInt(2500),
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
		DeletedAt: &now,
	}

	// Convert Entity -> Model
	var model TransactionPostgreSQLModel
	model.FromEntity(originalEntity)

	// Convert Model -> Entity
	resultEntity := model.ToEntity()

	// Verify round-trip integrity
	assert.Equal(t, originalEntity.ID, resultEntity.ID)
	assert.Equal(t, originalEntity.ParentTransactionID, resultEntity.ParentTransactionID)
	assert.Equal(t, originalEntity.Description, resultEntity.Description)
	assert.Equal(t, originalEntity.Status.Code, resultEntity.Status.Code)
	assert.Equal(t, originalEntity.Status.Description, resultEntity.Status.Description)
	assert.Equal(t, originalEntity.Amount, resultEntity.Amount)
	assert.Equal(t, originalEntity.AssetCode, resultEntity.AssetCode)
	assert.Equal(t, originalEntity.ChartOfAccountsGroupName, resultEntity.ChartOfAccountsGroupName)
	assert.Equal(t, originalEntity.LedgerID, resultEntity.LedgerID)
	assert.Equal(t, originalEntity.OrganizationID, resultEntity.OrganizationID)
	assert.Equal(t, originalEntity.Route, resultEntity.Route)
	assert.Equal(t, originalEntity.CreatedAt, resultEntity.CreatedAt)
	assert.Equal(t, originalEntity.UpdatedAt, resultEntity.UpdatedAt)

	require.NotNil(t, resultEntity.DeletedAt)
	assert.Equal(t, *originalEntity.DeletedAt, *resultEntity.DeletedAt)

	// Body comparison
	assert.Equal(t, originalEntity.Body.ChartOfAccountsGroupName, resultEntity.Body.ChartOfAccountsGroupName)
	assert.Equal(t, originalEntity.Body.Description, resultEntity.Body.Description)
	assert.Equal(t, originalEntity.Body.Code, resultEntity.Body.Code)
}

func TestTransaction_IDtoUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		transactionID string
		expectedUUID  uuid.UUID
	}{
		{
			name:          "valid UUID",
			transactionID: "550e8400-e29b-41d4-a716-446655440000",
			expectedUUID:  uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		},
		{
			name:          "another valid UUID",
			transactionID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			expectedUUID:  uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		},
		{
			name:          "nil UUID",
			transactionID: "00000000-0000-0000-0000-000000000000",
			expectedUUID:  uuid.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			transaction := Transaction{ID: tt.transactionID}
			result := transaction.IDtoUUID()

			assert.Equal(t, tt.expectedUUID, result)
		})
	}
}

func TestTransaction_TransactionRevert(t *testing.T) {
	t.Parallel()

	amount100 := decimal.NewFromInt(100)
	amount200 := decimal.NewFromInt(200)
	amount300 := decimal.NewFromInt(300)
	totalAmount := decimal.NewFromInt(300)

	tests := []struct {
		name        string
		transaction Transaction
		validate    func(t *testing.T, result mtransaction.Transaction)
	}{
		{
			name: "revert transaction with credit operations becomes from",
			transaction: Transaction{
				Description:              "Original credit transaction",
				AssetCode:                "USD",
				Amount:                   &amount100,
				ChartOfAccountsGroupName: "REVENUE",
				Metadata:                 map[string]any{"original": true},
				Route:                    "route-original",
				Operations: []*operation.Operation{
					{
						Type:            constant.CREDIT,
						AccountAlias:    "@receiver",
						AssetCode:       "USD",
						Amount:          operation.Amount{Value: &amount100},
						Description:     "Credit operation",
						ChartOfAccounts: "1001",
						Metadata:        map[string]any{"op": "credit"},
						Route:           "op-route-1",
					},
				},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				assert.Equal(t, "Original credit transaction", result.Description)
				assert.Equal(t, "REVENUE", result.ChartOfAccountsGroupName)
				assert.False(t, result.Pending, "Reversal should not be pending")
				assert.Equal(t, map[string]any{"original": true}, result.Metadata)
				assert.Equal(t, "route-original", result.Route)

				// CREDIT operations become From (source) in reversal
				require.Len(t, result.Send.Source.From, 1)
				assert.Equal(t, "@receiver", result.Send.Source.From[0].AccountAlias)
				assert.True(t, result.Send.Source.From[0].IsFrom)
				assert.Equal(t, "Credit operation", result.Send.Source.From[0].Description)
				assert.Equal(t, "1001", result.Send.Source.From[0].ChartOfAccounts)
				assert.Equal(t, "op-route-1", result.Send.Source.From[0].Route)

				// No To entries since there were no DEBIT operations
				assert.Empty(t, result.Send.Distribute.To)
			},
		},
		{
			name: "revert transaction with debit operations becomes to",
			transaction: Transaction{
				Description:              "Original debit transaction",
				AssetCode:                "USD",
				Amount:                   &amount200,
				ChartOfAccountsGroupName: "EXPENSES",
				Operations: []*operation.Operation{
					{
						Type:            constant.DEBIT,
						AccountAlias:    "@sender",
						AssetCode:       "USD",
						Amount:          operation.Amount{Value: &amount200},
						Description:     "Debit operation",
						ChartOfAccounts: "2001",
						Metadata:        map[string]any{"op": "debit"},
						Route:           "op-route-2",
					},
				},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				assert.Equal(t, "Original debit transaction", result.Description)
				assert.False(t, result.Pending)

				// DEBIT operations become To (distribute) in reversal
				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, "@sender", result.Send.Distribute.To[0].AccountAlias)
				assert.False(t, result.Send.Distribute.To[0].IsFrom)
				assert.Equal(t, "Debit operation", result.Send.Distribute.To[0].Description)
				assert.Equal(t, "2001", result.Send.Distribute.To[0].ChartOfAccounts)
				assert.Equal(t, "op-route-2", result.Send.Distribute.To[0].Route)

				// No From entries since there were no CREDIT operations
				assert.Empty(t, result.Send.Source.From)
			},
		},
		{
			name: "revert transaction with mixed operations",
			transaction: Transaction{
				Description:              "Mixed operations transaction",
				AssetCode:                "BRL",
				Amount:                   &totalAmount,
				ChartOfAccountsGroupName: "TRANSFER",
				Route:                    "transfer-route",
				Metadata:                 map[string]any{"type": "transfer"},
				Operations: []*operation.Operation{
					{
						Type:            constant.DEBIT,
						AccountAlias:    "@account1",
						AssetCode:       "BRL",
						Amount:          operation.Amount{Value: &amount100},
						Description:     "Debit from account1",
						ChartOfAccounts: "3001",
					},
					{
						Type:            constant.DEBIT,
						AccountAlias:    "@account2",
						AssetCode:       "BRL",
						Amount:          operation.Amount{Value: &amount200},
						Description:     "Debit from account2",
						ChartOfAccounts: "3002",
					},
					{
						Type:            constant.CREDIT,
						AccountAlias:    "@account3",
						AssetCode:       "BRL",
						Amount:          operation.Amount{Value: &amount300},
						Description:     "Credit to account3",
						ChartOfAccounts: "3003",
					},
				},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				assert.Equal(t, "Mixed operations transaction", result.Description)
				assert.Equal(t, "BRL", result.Send.Asset)
				assert.True(t, result.Send.Value.Equal(totalAmount))
				assert.False(t, result.Pending)
				assert.Equal(t, "transfer-route", result.Route)
				assert.Equal(t, map[string]any{"type": "transfer"}, result.Metadata)

				// CREDIT becomes From in reversal
				require.Len(t, result.Send.Source.From, 1)
				assert.Equal(t, "@account3", result.Send.Source.From[0].AccountAlias)
				assert.True(t, result.Send.Source.From[0].IsFrom)

				// DEBIT becomes To in reversal
				require.Len(t, result.Send.Distribute.To, 2)

				// Find the specific To entries
				toAliases := make(map[string]mtransaction.FromTo)
				for _, to := range result.Send.Distribute.To {
					toAliases[to.AccountAlias] = to
				}

				assert.Contains(t, toAliases, "@account1")
				assert.Contains(t, toAliases, "@account2")
				assert.False(t, toAliases["@account1"].IsFrom)
				assert.False(t, toAliases["@account2"].IsFrom)
			},
		},
		{
			name: "revert transaction with no operations",
			transaction: Transaction{
				Description: "Empty operations",
				AssetCode:   "EUR",
				Amount:      &amount100,
				Operations:  []*operation.Operation{},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				assert.Equal(t, "Empty operations", result.Description)
				assert.Equal(t, "EUR", result.Send.Asset)
				assert.Empty(t, result.Send.Source.From)
				assert.Empty(t, result.Send.Distribute.To)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.transaction.TransactionRevert()

			tt.validate(t, result)
		})
	}
}

func TestTransactionRevert_NilAmount(t *testing.T) {
	t.Parallel()

	txn := Transaction{
		Description: "Transaction with nil amount",
		AssetCode:   "BRL",
		Amount:      nil,
		Operations: []*operation.Operation{
			{
				Type:         constant.CREDIT,
				AccountAlias: "@receiver",
				AssetCode:    "BRL",
			},
		},
	}

	result := txn.TransactionRevert()

	assert.Empty(t, result.Send.Asset, "should return empty transaction when Amount is nil")
	assert.Empty(t, result.Send.Source.From, "should return empty froms when Amount is nil")
	assert.Empty(t, result.Send.Distribute.To, "should return empty tos when Amount is nil")
}

func TestTransactionRevert_PreservesBalanceKey(t *testing.T) {
	t.Parallel()

	amount := decimal.NewFromInt(40)
	txn := Transaction{
		Description: "additional balance transfer",
		AssetCode:   "BRL",
		Amount:      &amount,
		Operations: []*operation.Operation{
			{
				Type:         constant.CREDIT,
				AccountAlias: "@receiver",
				BalanceKey:   "voucher",
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount},
			},
			{
				Type:         constant.DEBIT,
				AccountAlias: "@sender",
				BalanceKey:   "reserve",
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount},
			},
		},
	}

	reverted := txn.TransactionRevert()

	require.Len(t, reverted.Send.Source.From, 1)
	assert.Equal(t, "voucher", reverted.Send.Source.From[0].BalanceKey)
	require.Len(t, reverted.Send.Distribute.To, 1)
	assert.Equal(t, "reserve", reverted.Send.Distribute.To[0].BalanceKey)
}

func TestTransactionRevert_FoldsOverdraftCompanionIntoDefaultLeg(t *testing.T) {
	t.Parallel()

	amount100 := decimal.NewFromInt(100)
	amount50 := decimal.NewFromInt(50)
	txn := Transaction{
		Description: "overdraft transfer",
		AssetCode:   "BRL",
		Amount:      &amount100,
		Operations: []*operation.Operation{
			{
				Type:         constant.DEBIT,
				AccountAlias: "@source",
				BalanceKey:   pkgConstant.DefaultBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount50},
			},
			{
				Type:         pkgConstant.OVERDRAFT,
				Direction:    pkgConstant.DirectionDebit,
				AccountAlias: "@source",
				BalanceKey:   pkgConstant.OverdraftBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount50},
			},
			{
				Type:         constant.CREDIT,
				AccountAlias: "@destination",
				BalanceKey:   pkgConstant.DefaultBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount100},
			},
		},
	}

	reverted := txn.TransactionRevert()

	require.Len(t, reverted.Send.Source.From, 1)
	assert.Equal(t, "@destination", reverted.Send.Source.From[0].AccountAlias)
	assert.Equal(t, pkgConstant.DefaultBalanceKey, reverted.Send.Source.From[0].BalanceKey)
	assert.True(t, reverted.Send.Source.From[0].Amount.Value.Equal(amount100))

	require.Len(t, reverted.Send.Distribute.To, 1,
		"internal overdraft companion must not be targeted directly by public revert transaction")
	assert.Equal(t, "@source", reverted.Send.Distribute.To[0].AccountAlias)
	assert.Equal(t, pkgConstant.DefaultBalanceKey, reverted.Send.Distribute.To[0].BalanceKey)
	assert.True(t, reverted.Send.Distribute.To[0].Amount.Value.Equal(amount100),
		"default reverse credit must include the default movement plus overdraft companion amount")
}

func TestTransactionRevert_FoldsOverdraftCompanionRegardlessOfOperationOrder(t *testing.T) {
	t.Parallel()

	amount100 := decimal.NewFromInt(100)
	amount50 := decimal.NewFromInt(50)
	txn := Transaction{
		Description: "overdraft transfer with companion before default",
		AssetCode:   "BRL",
		Amount:      &amount100,
		Operations: []*operation.Operation{
			{
				Type:         pkgConstant.OVERDRAFT,
				Direction:    pkgConstant.DirectionDebit,
				AccountAlias: "@source",
				BalanceKey:   pkgConstant.OverdraftBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount50},
			},
			{
				Type:         constant.DEBIT,
				AccountAlias: "@source",
				BalanceKey:   pkgConstant.DefaultBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount50},
			},
			{
				Type:         constant.CREDIT,
				AccountAlias: "@destination",
				BalanceKey:   pkgConstant.DefaultBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount100},
			},
		},
	}

	reverted := txn.TransactionRevert()

	require.Len(t, reverted.Send.Source.From, 1)
	assert.Equal(t, "@destination", reverted.Send.Source.From[0].AccountAlias)
	assert.True(t, reverted.Send.Source.From[0].Amount.Value.Equal(amount100))
	require.Len(t, reverted.Send.Distribute.To, 1)
	assert.Equal(t, "@source", reverted.Send.Distribute.To[0].AccountAlias)
	assert.Equal(t, pkgConstant.DefaultBalanceKey, reverted.Send.Distribute.To[0].BalanceKey)
	assert.True(t, reverted.Send.Distribute.To[0].Amount.Value.Equal(amount100),
		"companion amount must be folded even when PostgreSQL returns it before the default DEBIT")
}

func TestTransactionRevert_DoesNotDoubleCountCommittedPendingOverdraftCompanion(t *testing.T) {
	t.Parallel()

	amount100 := decimal.NewFromInt(100)
	amount50 := decimal.NewFromInt(50)
	txn := Transaction{
		Description: "committed pending overdraft transfer",
		AssetCode:   "BRL",
		Amount:      &amount100,
		Operations: []*operation.Operation{
			{
				Type:         constant.ONHOLD,
				Direction:    pkgConstant.DirectionDebit,
				AccountAlias: "@source",
				BalanceKey:   pkgConstant.DefaultBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount100},
			},
			{
				Type:         pkgConstant.OVERDRAFT,
				Direction:    pkgConstant.DirectionDebit,
				AccountAlias: "@source",
				BalanceKey:   pkgConstant.OverdraftBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount50},
			},
			{
				Type:         constant.CREDIT,
				AccountAlias: "@destination",
				BalanceKey:   pkgConstant.DefaultBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount100},
			},
			{
				Type:         constant.DEBIT,
				AccountAlias: "@source",
				BalanceKey:   pkgConstant.DefaultBalanceKey,
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount100},
			},
		},
	}

	reverted := txn.TransactionRevert()

	require.Len(t, reverted.Send.Source.From, 1)
	assert.Equal(t, "@destination", reverted.Send.Source.From[0].AccountAlias)
	assert.True(t, reverted.Send.Source.From[0].Amount.Value.Equal(amount100))
	require.Len(t, reverted.Send.Distribute.To, 1)
	assert.Equal(t, "@source", reverted.Send.Distribute.To[0].AccountAlias)
	assert.Equal(t, pkgConstant.DefaultBalanceKey, reverted.Send.Distribute.To[0].BalanceKey)
	assert.True(t, reverted.Send.Distribute.To[0].Amount.Value.Equal(amount100),
		"committed pending DEBIT already carries the full amount; companion must not inflate revert to 150")
}

func TestTransactionRevert_DirectionNotSet(t *testing.T) {
	t.Parallel()

	// TransactionRevert intentionally omits Direction because CalculateTotal
	// re-derives it via DetermineOperation based on IsFrom.
	amount100 := decimal.NewFromInt(100)
	amount200 := decimal.NewFromInt(200)

	txn := Transaction{
		Description: "Mixed transaction",
		AssetCode:   "BRL",
		Amount:      &amount200,
		Operations: []*operation.Operation{
			{
				Type:         constant.CREDIT,
				AccountAlias: "@receiver",
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount100},
			},
			{
				Type:         constant.DEBIT,
				AccountAlias: "@sender",
				AssetCode:    "BRL",
				Amount:       operation.Amount{Value: &amount200},
			},
		},
	}

	result := txn.TransactionRevert()

	require.Len(t, result.Send.Source.From, 1)
	from := result.Send.Source.From[0]
	require.NotNil(t, from.Amount)
	assert.Empty(t, from.Amount.Direction, "Direction should be empty — derived downstream by DetermineOperation")

	require.Len(t, result.Send.Distribute.To, 1)
	to := result.Send.Distribute.To[0]
	require.NotNil(t, to.Amount)
	assert.Empty(t, to.Amount.Direction, "Direction should be empty — derived downstream by DetermineOperation")
}

func TestTransactionRevert_RoutePreservation(t *testing.T) {
	t.Parallel()

	amount100 := decimal.NewFromInt(100)
	routeID := "route-" + uuid.New().String()

	tests := []struct {
		name        string
		transaction Transaction
		validate    func(t *testing.T, result mtransaction.Transaction)
	}{
		{
			name: "route from CREDIT operation preserved in reversed FromTo",
			transaction: Transaction{
				Description: "Transaction with route on credit",
				AssetCode:   "USD",
				Amount:      &amount100,
				Operations: []*operation.Operation{
					{
						Type:         constant.CREDIT,
						AccountAlias: "@receiver",
						AssetCode:    "USD",
						Amount:       operation.Amount{Value: &amount100},
						Route:        routeID,
					},
				},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				require.Len(t, result.Send.Source.From, 1)
				assert.Equal(t, routeID, result.Send.Source.From[0].Route,
					"Route should be preserved in reversed FromTo")
			},
		},
		{
			name: "route from DEBIT operation preserved in reversed FromTo",
			transaction: Transaction{
				Description: "Transaction with route on debit",
				AssetCode:   "USD",
				Amount:      &amount100,
				Operations: []*operation.Operation{
					{
						Type:         constant.DEBIT,
						AccountAlias: "@sender",
						AssetCode:    "USD",
						Amount:       operation.Amount{Value: &amount100},
						Route:        routeID,
					},
				},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, routeID, result.Send.Distribute.To[0].Route,
					"Route should be preserved in reversed FromTo")
			},
		},
		{
			name: "operations without route revert normally",
			transaction: Transaction{
				Description: "Transaction without routes",
				AssetCode:   "USD",
				Amount:      &amount100,
				Operations: []*operation.Operation{
					{
						Type:         constant.CREDIT,
						AccountAlias: "@receiver",
						AssetCode:    "USD",
						Amount:       operation.Amount{Value: &amount100},
					},
				},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				require.Len(t, result.Send.Source.From, 1)
				assert.Empty(t, result.Send.Source.From[0].Route,
					"Route should be empty when not set on original operation")
			},
		},
		{
			name: "RouteID from CREDIT operation preserved in reversed FromTo",
			transaction: Transaction{
				Description: "Transaction with RouteID on credit",
				AssetCode:   "USD",
				Amount:      &amount100,
				Operations: []*operation.Operation{
					{
						Type:         constant.CREDIT,
						AccountAlias: "@receiver",
						AssetCode:    "USD",
						Amount:       operation.Amount{Value: &amount100},
						RouteID:      &routeID,
					},
				},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				require.Len(t, result.Send.Source.From, 1)
				require.NotNil(t, result.Send.Source.From[0].RouteID,
					"RouteID should not be nil")
				assert.Equal(t, routeID, *result.Send.Source.From[0].RouteID,
					"RouteID should be preserved in reversed FromTo")
			},
		},
		{
			name: "RouteID from DEBIT operation preserved in reversed FromTo",
			transaction: Transaction{
				Description: "Transaction with RouteID on debit",
				AssetCode:   "USD",
				Amount:      &amount100,
				Operations: []*operation.Operation{
					{
						Type:         constant.DEBIT,
						AccountAlias: "@sender",
						AssetCode:    "USD",
						Amount:       operation.Amount{Value: &amount100},
						RouteID:      &routeID,
					},
				},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				require.Len(t, result.Send.Distribute.To, 1)
				require.NotNil(t, result.Send.Distribute.To[0].RouteID,
					"RouteID should not be nil")
				assert.Equal(t, routeID, *result.Send.Distribute.To[0].RouteID,
					"RouteID should be preserved in reversed FromTo")
			},
		},
		{
			name: "operations without RouteID have nil RouteID in result",
			transaction: Transaction{
				Description: "Transaction without RouteID",
				AssetCode:   "USD",
				Amount:      &amount100,
				Operations: []*operation.Operation{
					{
						Type:         constant.CREDIT,
						AccountAlias: "@receiver",
						AssetCode:    "USD",
						Amount:       operation.Amount{Value: &amount100},
					},
				},
			},
			validate: func(t *testing.T, result mtransaction.Transaction) {
				require.Len(t, result.Send.Source.From, 1)
				assert.Nil(t, result.Send.Source.From[0].RouteID,
					"RouteID should be nil when not set on original operation")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.transaction.TransactionRevert()

			tt.validate(t, result)
		})
	}
}

// TestOperationColumnListPrefixed_IncludesSnapshot is a compile-time-safe guard
// that prevents column-list drift between the canonical operation.operationColumnList
// (which includes "snapshot") and the prefixed copy used by FindWithOperations /
// FindOrListAllWithOperations.
func TestOperationColumnListPrefixed_IncludesSnapshot(t *testing.T) {
	t.Parallel()

	found := false

	for _, col := range operationColumnListPrefixed {
		if col == "o.snapshot" {
			found = true

			break
		}
	}

	require.True(t, found,
		"operationColumnListPrefixed must include 'o.snapshot'; "+
			"without it, FindWithOperations and FindOrListAllWithOperations "+
			"return operations with nil Snapshot, causing ToEntity to default "+
			"all overdraft fields to zero regardless of stored values")

	// Also verify total count matches operation.operationColumnList (31 columns).
	// This catches additions to the canonical list that aren't mirrored here.
	assert.Len(t, operationColumnListPrefixed, operation.ExportedOperationColumnListLen(),
		"operationColumnListPrefixed column count must match operation.operationColumnList; "+
			"a column was added to the canonical list but not to the prefixed copy")
}
