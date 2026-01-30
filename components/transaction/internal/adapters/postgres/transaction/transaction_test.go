package transaction

import (
	"database/sql"
	"testing"
	"time"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
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
				Body: &pkgTransaction.Transaction{
					ChartOfAccountsGroupName: "FUNDING",
					Description:              "Body description",
					Code:                     "TX-001",
					Send: pkgTransaction.Send{
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
				Body: &pkgTransaction.Transaction{
					ChartOfAccountsGroupName: "EXPENSES",
					Description:              "Full body",
					Send: pkgTransaction.Send{
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
				Body: pkgTransaction.Transaction{
					ChartOfAccountsGroupName: "FUNDING",
					Description:              "Body content",
					Send: pkgTransaction.Send{
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
		Body: pkgTransaction.Transaction{
			ChartOfAccountsGroupName: "FUNDING",
			Description:              "Round trip body",
			Code:                     "RT-001",
			Send: pkgTransaction.Send{
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

func TestCreateTransactionInput_BuildTransaction(t *testing.T) {
	t.Parallel()

	transactionDate := &pkgTransaction.TransactionDate{}

	tests := []struct {
		name     string
		input    CreateTransactionInput
		validate func(t *testing.T, result *pkgTransaction.Transaction)
	}{
		{
			name: "minimal input without send",
			input: CreateTransactionInput{
				Description: "Minimal transaction",
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				assert.Equal(t, "Minimal transaction", result.Description)
				assert.Empty(t, result.ChartOfAccountsGroupName)
				assert.Empty(t, result.Code)
				assert.False(t, result.Pending)
				assert.Nil(t, result.Metadata)
			},
		},
		{
			name: "input with all fields except send",
			input: CreateTransactionInput{
				ChartOfAccountsGroupName: "FUNDING",
				Description:              "Full transaction",
				Code:                     "TX-001",
				Pending:                  true,
				Metadata:                 map[string]any{"key": "value"},
				Route:                    "route-123",
				TransactionDate:          transactionDate,
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				assert.Equal(t, "FUNDING", result.ChartOfAccountsGroupName)
				assert.Equal(t, "Full transaction", result.Description)
				assert.Equal(t, "TX-001", result.Code)
				assert.True(t, result.Pending)
				assert.Equal(t, map[string]any{"key": "value"}, result.Metadata)
				assert.Equal(t, "route-123", result.Route)
				assert.Equal(t, transactionDate, result.TransactionDate)
			},
		},
		{
			name: "input with send and from entries",
			input: CreateTransactionInput{
				Description: "Transaction with send",
				Send: &pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(1000),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: "@sender1",
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(600),
								},
								IsFrom: false, // Should be set to true by FromDSL
							},
							{
								AccountAlias: "@sender2",
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(400),
								},
								IsFrom: false, // Should be set to true by FromDSL
							},
						},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{
							{
								AccountAlias: "@receiver",
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(1000),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				assert.Equal(t, "Transaction with send", result.Description)
				assert.Equal(t, "USD", result.Send.Asset)
				assert.True(t, result.Send.Value.Equal(decimal.NewFromInt(1000)))

				// Verify IsFrom is set to true for all From entries
				require.Len(t, result.Send.Source.From, 2)
				for _, from := range result.Send.Source.From {
					assert.True(t, from.IsFrom, "IsFrom should be true for From entries")
				}

				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, "@receiver", result.Send.Distribute.To[0].AccountAlias)
			},
		},
		{
			name: "input with nil send",
			input: CreateTransactionInput{
				Description: "No send",
				Send:        nil,
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				assert.Equal(t, "No send", result.Description)
				// Send should be empty/zero value
				assert.True(t, result.Send.Value.IsZero())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.input.BuildTransaction()

			require.NotNil(t, result)
			tt.validate(t, result)
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
		validate    func(t *testing.T, result pkgTransaction.Transaction)
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
			validate: func(t *testing.T, result pkgTransaction.Transaction) {
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
			validate: func(t *testing.T, result pkgTransaction.Transaction) {
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
			validate: func(t *testing.T, result pkgTransaction.Transaction) {
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
				toAliases := make(map[string]pkgTransaction.FromTo)
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
			validate: func(t *testing.T, result pkgTransaction.Transaction) {
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

func TestCreateTransactionInflowInput_BuildInflowEntry(t *testing.T) {
	t.Parallel()

	transactionDate := &pkgTransaction.TransactionDate{}

	tests := []struct {
		name     string
		input    CreateTransactionInflowInput
		validate func(t *testing.T, result *pkgTransaction.Transaction)
	}{
		{
			name: "minimal inflow",
			input: CreateTransactionInflowInput{
				Description: "Minimal inflow",
				Send: &SendInflow{
					Asset: "USD",
					Value: decimal.NewFromInt(500),
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{
							{
								AccountAlias: "@receiver",
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(500),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				assert.Equal(t, "Minimal inflow", result.Description)
				assert.Equal(t, "USD", result.Send.Asset)
				assert.True(t, result.Send.Value.Equal(decimal.NewFromInt(500)))

				// Verify external account is created as source
				require.Len(t, result.Send.Source.From, 1)
				from := result.Send.Source.From[0]
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"USD", from.AccountAlias)
				assert.True(t, from.IsFrom)
				require.NotNil(t, from.Amount)
				assert.Equal(t, "USD", from.Amount.Asset)
				assert.True(t, from.Amount.Value.Equal(decimal.NewFromInt(500)))

				// Verify distribute is passed through
				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, "@receiver", result.Send.Distribute.To[0].AccountAlias)
			},
		},
		{
			name: "inflow with all fields",
			input: CreateTransactionInflowInput{
				ChartOfAccountsGroupName: "FUNDING",
				Description:              "Full inflow",
				Code:                     "INF-001",
				Metadata:                 map[string]any{"source": "external"},
				Route:                    "inflow-route",
				TransactionDate:          transactionDate,
				Send: &SendInflow{
					Asset: "BRL",
					Value: decimal.NewFromInt(1000),
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{
							{
								AccountAlias:    "@account1",
								Description:     "Credit to account1",
								ChartOfAccounts: "4001",
								Amount: &pkgTransaction.Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(600),
								},
							},
							{
								AccountAlias:    "@account2",
								Description:     "Credit to account2",
								ChartOfAccounts: "4002",
								Amount: &pkgTransaction.Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(400),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				assert.Equal(t, "FUNDING", result.ChartOfAccountsGroupName)
				assert.Equal(t, "Full inflow", result.Description)
				assert.Equal(t, "INF-001", result.Code)
				assert.Equal(t, map[string]any{"source": "external"}, result.Metadata)
				assert.Equal(t, "inflow-route", result.Route)
				assert.Equal(t, transactionDate, result.TransactionDate)

				// Verify external account prefix
				require.Len(t, result.Send.Source.From, 1)
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"BRL", result.Send.Source.From[0].AccountAlias)

				// Verify multiple To entries
				require.Len(t, result.Send.Distribute.To, 2)
			},
		},
		{
			name: "inflow with different asset",
			input: CreateTransactionInflowInput{
				Description: "EUR inflow",
				Send: &SendInflow{
					Asset: "EUR",
					Value: decimal.NewFromInt(250),
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{
							{
								AccountAlias: "@euro_account",
								Amount: &pkgTransaction.Amount{
									Asset: "EUR",
									Value: decimal.NewFromInt(250),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				// Verify external account uses correct asset
				require.Len(t, result.Send.Source.From, 1)
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"EUR", result.Send.Source.From[0].AccountAlias)
				assert.Equal(t, "EUR", result.Send.Source.From[0].Amount.Asset)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.input.BuildInflowEntry()

			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestCreateTransactionOutflowInput_BuildOutflowEntry(t *testing.T) {
	t.Parallel()

	transactionDate := &pkgTransaction.TransactionDate{}

	tests := []struct {
		name     string
		input    CreateTransactionOutflowInput
		validate func(t *testing.T, result *pkgTransaction.Transaction)
	}{
		{
			name: "minimal outflow",
			input: CreateTransactionOutflowInput{
				Description: "Minimal outflow",
				Send: &SendOutflow{
					Asset: "USD",
					Value: decimal.NewFromInt(500),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: "@sender",
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(500),
								},
								IsFrom: false, // Should be set to true by OutflowFromDSL
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				assert.Equal(t, "Minimal outflow", result.Description)
				assert.Equal(t, "USD", result.Send.Asset)
				assert.True(t, result.Send.Value.Equal(decimal.NewFromInt(500)))

				// Verify external account is created as destination
				require.Len(t, result.Send.Distribute.To, 1)
				to := result.Send.Distribute.To[0]
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"USD", to.AccountAlias)
				assert.False(t, to.IsFrom)
				require.NotNil(t, to.Amount)
				assert.Equal(t, "USD", to.Amount.Asset)

				// Verify source From entries have IsFrom=true
				require.Len(t, result.Send.Source.From, 1)
				assert.True(t, result.Send.Source.From[0].IsFrom, "From entries should have IsFrom=true")
			},
		},
		{
			name: "outflow with all fields",
			input: CreateTransactionOutflowInput{
				ChartOfAccountsGroupName: "WITHDRAWAL",
				Description:              "Full outflow",
				Code:                     "OUT-001",
				Pending:                  true,
				Metadata:                 map[string]any{"destination": "external"},
				Route:                    "outflow-route",
				TransactionDate:          transactionDate,
				Send: &SendOutflow{
					Asset: "BRL",
					Value: decimal.NewFromInt(1000),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias:    "@account1",
								Description:     "Debit from account1",
								ChartOfAccounts: "5001",
								Amount: &pkgTransaction.Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(600),
								},
							},
							{
								AccountAlias:    "@account2",
								Description:     "Debit from account2",
								ChartOfAccounts: "5002",
								Amount: &pkgTransaction.Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(400),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				assert.Equal(t, "WITHDRAWAL", result.ChartOfAccountsGroupName)
				assert.Equal(t, "Full outflow", result.Description)
				assert.Equal(t, "OUT-001", result.Code)
				assert.True(t, result.Pending, "Pending flag should be preserved")
				assert.Equal(t, map[string]any{"destination": "external"}, result.Metadata)
				assert.Equal(t, "outflow-route", result.Route)
				assert.Equal(t, transactionDate, result.TransactionDate)

				// Verify external account prefix in To
				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"BRL", result.Send.Distribute.To[0].AccountAlias)

				// Verify multiple From entries have IsFrom=true
				require.Len(t, result.Send.Source.From, 2)
				for _, from := range result.Send.Source.From {
					assert.True(t, from.IsFrom, "All From entries should have IsFrom=true")
				}
			},
		},
		{
			name: "outflow with different asset",
			input: CreateTransactionOutflowInput{
				Description: "EUR outflow",
				Send: &SendOutflow{
					Asset: "EUR",
					Value: decimal.NewFromInt(250),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: "@euro_account",
								Amount: &pkgTransaction.Amount{
									Asset: "EUR",
									Value: decimal.NewFromInt(250),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				// Verify external account uses correct asset
				require.Len(t, result.Send.Distribute.To, 1)
				assert.Equal(t, cn.DefaultExternalAccountAliasPrefix+"EUR", result.Send.Distribute.To[0].AccountAlias)
				assert.Equal(t, "EUR", result.Send.Distribute.To[0].Amount.Asset)
			},
		},
		{
			name: "outflow not pending",
			input: CreateTransactionOutflowInput{
				Description: "Non-pending outflow",
				Pending:     false,
				Send: &SendOutflow{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: "@account",
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, result *pkgTransaction.Transaction) {
				assert.False(t, result.Pending, "Pending flag should be false")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.input.BuildOutflowEntry()

			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}
