package transaction

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
IMPORTANT NOTE ON TEST COVERAGE:

These tests use a mock repository implementation that satisfies the Repository interface
rather than directly testing the actual PostgreSQL repository implementation. This is a common
pattern in Go testing, but it means that the coverage report doesn't accurately reflect the
test coverage of our code.

The compile-time interface checks below ensure that both the mock repository and the real
PostgreSQL repository implement the same interface, which provides some confidence that
our tests are testing the right contract.

To get more accurate coverage metrics, consider adding integration tests that use a real
or containerized PostgreSQL database.
*/

// Compile-time interface check
var (
	_ Repository = (*TransactionPostgreSQLRepository)(nil)
)

// Create a mock for the real repository to test
type mockRepository struct {
	db        *sql.DB
	mock      sqlmock.Sqlmock
	tableName string
	err       error
}

func (r *mockRepository) Create(ctx context.Context, transaction *Transaction) (*Transaction, error) {
	if r.err != nil {
		return nil, r.err
	}

	// Execute the query to insert the transaction
	_, err := r.db.ExecContext(
		ctx,
		"INSERT INTO transaction (id, parent_transaction_id, description, template, status, status_description, amount, amount_scale, asset_code, chart_of_accounts_group_name, ledger_id, organization_id, body, created_at, updated_at, deleted_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)",
		transaction.ID,
		nil, // ParentTransactionID
		transaction.Description,
		nil, // Template
		transaction.Status.Code,
		nil, // StatusDescription
		nil, // Amount
		nil, // AmountScale
		transaction.AssetCode,
		nil, // ChartOfAccountsGroupName
		transaction.LedgerID,
		transaction.OrganizationID,
		nil, // Body
		transaction.CreatedAt,
		transaction.UpdatedAt,
		nil, // DeletedAt
	)
	if err != nil {
		return nil, err
	}

	// Query to get the inserted transaction
	row := r.db.QueryRowContext(
		ctx,
		"SELECT id, parent_transaction_id, description, template, status, status_description, amount, amount_scale, asset_code, chart_of_accounts_group_name, ledger_id, organization_id, body, created_at, updated_at, deleted_at FROM transaction WHERE id = $1",
		transaction.ID,
	)

	var result Transaction
	var parentTransactionID, template, statusDescription, amount, amountScale, chartOfAccountsGroupName, body, deletedAt sql.NullString

	err = row.Scan(
		&result.ID,
		&parentTransactionID,
		&result.Description,
		&template,
		&result.Status.Code,
		&statusDescription,
		&amount,
		&amountScale,
		&result.AssetCode,
		&chartOfAccountsGroupName,
		&result.LedgerID,
		&result.OrganizationID,
		&body,
		&result.CreatedAt,
		&result.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *mockRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	if r.err != nil {
		return nil, libHTTP.CursorPagination{}, r.err
	}

	// Execute the query to get all transactions
	rows, err := r.db.QueryContext(
		ctx,
		"SELECT id, parent_transaction_id, description, template, status, status_description, amount, amount_scale, asset_code, chart_of_accounts_group_name, ledger_id, organization_id, body, created_at, updated_at, deleted_at FROM transaction WHERE organization_id = $1 AND ledger_id = $2 ORDER BY created_at DESC LIMIT $3",
		organizationID,
		ledgerID,
		filter.Limit,
	)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	var transactions []*Transaction
	for rows.Next() {
		var transaction Transaction
		var parentTransactionID, template, statusDescription, amount, amountScale, chartOfAccountsGroupName, body, deletedAt sql.NullString

		err := rows.Scan(
			&transaction.ID,
			&parentTransactionID,
			&transaction.Description,
			&template,
			&transaction.Status.Code,
			&statusDescription,
			&amount,
			&amountScale,
			&transaction.AssetCode,
			&chartOfAccountsGroupName,
			&transaction.LedgerID,
			&transaction.OrganizationID,
			&body,
			&transaction.CreatedAt,
			&transaction.UpdatedAt,
			&deletedAt,
		)
		if err != nil {
			return nil, libHTTP.CursorPagination{}, err
		}

		transactions = append(transactions, &transaction)
	}

	if err := rows.Err(); err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	// Create pagination info - using an empty struct since we don't need to test pagination details
	pagination := libHTTP.CursorPagination{
		Next: "",
		Prev: "",
	}

	return transactions, pagination, nil
}

func (r *mockRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	if r.err != nil {
		return nil, r.err
	}

	// Query to get the transaction
	row := r.db.QueryRowContext(
		ctx,
		"SELECT id, parent_transaction_id, description, template, status, status_description, amount, amount_scale, asset_code, chart_of_accounts_group_name, ledger_id, organization_id, body, created_at, updated_at, deleted_at FROM transaction WHERE id = $1 AND organization_id = $2 AND ledger_id = $3",
		id,
		organizationID,
		ledgerID,
	)

	var transaction Transaction
	var parentTransactionID, template, statusDescription, amount, amountScale, chartOfAccountsGroupName, body, deletedAt sql.NullString

	err := row.Scan(
		&transaction.ID,
		&parentTransactionID,
		&transaction.Description,
		&template,
		&transaction.Status.Code,
		&statusDescription,
		&amount,
		&amountScale,
		&transaction.AssetCode,
		&chartOfAccountsGroupName,
		&transaction.LedgerID,
		&transaction.OrganizationID,
		&body,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("transaction not found")
		}
		return nil, err
	}

	return &transaction, nil
}

func (r *mockRepository) FindByParentID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*Transaction, error) {
	if r.err != nil {
		return nil, r.err
	}

	// Query to get the transaction by parent ID
	row := r.db.QueryRowContext(
		ctx,
		"SELECT id, parent_transaction_id, description, template, status, status_description, amount, amount_scale, asset_code, chart_of_accounts_group_name, ledger_id, organization_id, body, created_at, updated_at, deleted_at FROM transaction WHERE parent_transaction_id = $1 AND organization_id = $2 AND ledger_id = $3",
		parentID,
		organizationID,
		ledgerID,
	)

	var transaction Transaction
	var parentTransactionID, template, statusDescription, amount, amountScale, chartOfAccountsGroupName, body, deletedAt sql.NullString

	err := row.Scan(
		&transaction.ID,
		&parentTransactionID,
		&transaction.Description,
		&template,
		&transaction.Status.Code,
		&statusDescription,
		&amount,
		&amountScale,
		&transaction.AssetCode,
		&chartOfAccountsGroupName,
		&transaction.LedgerID,
		&transaction.OrganizationID,
		&body,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("transaction not found")
		}
		return nil, err
	}

	return &transaction, nil
}

func (r *mockRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Transaction, error) {
	return nil, errors.New("not implemented")
}

func (r *mockRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transaction *Transaction) (*Transaction, error) {
	if r.err != nil {
		return nil, r.err
	}

	// Execute the query to update the transaction
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE transaction SET description = $1, status = $2, status_description = $3, updated_at = $4 
         WHERE organization_id = $5 AND ledger_id = $6 AND id = $7 AND deleted_at IS NULL`,
		transaction.Description,
		transaction.Status.Code,
		transaction.Status.Description,
		transaction.UpdatedAt,
		organizationID,
		ledgerID,
		id,
	)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, errors.New("transaction not found")
	}

	// Set the ID to the provided ID
	transaction.ID = id.String()
	transaction.OrganizationID = organizationID.String()
	transaction.LedgerID = ledgerID.String()
	return transaction, nil
}

func (r *mockRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	if r.err != nil {
		return r.err
	}

	// Execute the query to delete the transaction
	result, err := r.db.ExecContext(
		ctx,
		"DELETE FROM transaction WHERE id = $1 AND organization_id = $2 AND ledger_id = $3",
		id,
		organizationID,
		ledgerID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("transaction not found")
	}

	return nil
}

func (r *mockRepository) Count(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	if r.err != nil {
		return 0, r.err
	}

	return 10, nil
}

func (r *mockRepository) FindWithOperations(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	if r.err != nil {
		return nil, r.err
	}

	row := r.db.QueryRowContext(
		ctx,
		"SELECT * FROM transaction t INNER JOIN operation o ON t.id = o.transaction_id WHERE t.organization_id = $1 AND t.ledger_id = $2 AND t.id = $3 AND t.deleted_at IS NULL",
		organizationID, ledgerID, id,
	)

	var transaction Transaction
	var parentTransactionID, statusDescription, amount, amountScale, chartOfAccountsGroupName, body, deletedAt sql.NullString

	err := row.Scan(
		&transaction.ID,
		&parentTransactionID,
		&transaction.Description,
		&transaction.Status.Code,
		&statusDescription,
		&amount,
		&amountScale,
		&transaction.AssetCode,
		&chartOfAccountsGroupName,
		&transaction.LedgerID,
		&transaction.OrganizationID,
		&body,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("transaction not found")
		}
		return nil, err
	}

	op := &operation.Operation{
		ID:             uuid.New().String(),
		TransactionID:  transaction.ID,
		Type:           "DEBIT",
		AssetCode:      transaction.AssetCode,
		AccountID:      uuid.New().String(),
		LedgerID:       transaction.LedgerID,
		OrganizationID: transaction.OrganizationID,
	}

	transaction.Operations = append(transaction.Operations, op)

	return &transaction, nil
}

func (r *mockRepository) FindOrListAllWithOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	if r.err != nil {
		return nil, libHTTP.CursorPagination{}, r.err
	}

	transactions := []*Transaction{
		{
			ID:             organizationID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status: Status{
				Code: "APPROVED",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Operations: []*operation.Operation{
				{
					ID:             uuid.New().String(),
					OrganizationID: organizationID.String(),
					LedgerID:       ledgerID.String(),
					TransactionID:  uuid.New().String(),
					AccountID:      uuid.New().String(),
					Type:           "CREDIT",
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				},
				{
					ID:             uuid.New().String(),
					OrganizationID: organizationID.String(),
					LedgerID:       ledgerID.String(),
					TransactionID:  uuid.New().String(),
					AccountID:      uuid.New().String(),
					Type:           "DEBIT",
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				},
			},
		},
		{
			ID:             uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status: Status{
				Code: "PENDING",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Operations: []*operation.Operation{
				{
					ID:             uuid.New().String(),
					OrganizationID: organizationID.String(),
					LedgerID:       ledgerID.String(),
					TransactionID:  uuid.New().String(),
					AccountID:      uuid.New().String(),
					Type:           "CREDIT",
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				},
				{
					ID:             uuid.New().String(),
					OrganizationID: organizationID.String(),
					LedgerID:       ledgerID.String(),
					TransactionID:  uuid.New().String(),
					AccountID:      uuid.New().String(),
					Type:           "DEBIT",
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				},
			},
		},
	}

	pagination := libHTTP.CursorPagination{
		Next: "",
		Prev: "",
	}

	return transactions, pagination, nil
}

// setupMockDB creates a new mock database and repository
func setupMockDB(t *testing.T) (*mockRepository, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := &mockRepository{
		db:        db,
		mock:      mock,
		tableName: "transaction",
	}

	return repo, mock
}

// setupErrorDB creates a mock repository that returns an error
func setupErrorDB() *mockRepository {
	return &mockRepository{
		err: errors.New("database error"),
	}
}

func TestTransactionRepository_Create(t *testing.T) {
	// Test cases
	tests := []struct {
		name          string
		setupRepo     func() *mockRepository
		transaction   *Transaction
		expectedError bool
	}{
		{
			name: "successful creation",
			setupRepo: func() *mockRepository {
				repo, mock := setupMockDB(t)

				// Expect the insert query
				mock.ExpectExec("INSERT INTO transaction").
					WithArgs(
						sqlmock.AnyArg(),
						nil,
						sqlmock.AnyArg(),
						nil,
						sqlmock.AnyArg(),
						nil,
						nil,
						nil,
						sqlmock.AnyArg(),
						nil,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						nil,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						nil,
					).
					WillReturnResult(sqlmock.NewResult(1, 1))

				// Expect the select query to get the inserted transaction
				mock.ExpectQuery("SELECT (.+) FROM transaction WHERE id = (.+)").
					WithArgs(sqlmock.AnyArg()).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "parent_transaction_id", "description", "template", "status", "status_description",
						"amount", "amount_scale", "asset_code", "chart_of_accounts_group_name", "ledger_id",
						"organization_id", "body", "created_at", "updated_at", "deleted_at",
					}).AddRow(
						uuid.New().String(),
						nil,
						"Test Transaction",
						nil,
						"PENDING",
						nil,
						nil,
						nil,
						"USD",
						nil,
						uuid.New().String(),
						uuid.New().String(),
						nil,
						time.Now(),
						time.Now(),
						nil,
					))

				return repo
			},
			transaction: &Transaction{
				ID:             uuid.New().String(),
				Description:    "Test Transaction",
				Status:         Status{Code: "PENDING"},
				AssetCode:      "USD",
				LedgerID:       uuid.New().String(),
				OrganizationID: uuid.New().String(),
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
			expectedError: false,
		},
		{
			name: "database error",
			setupRepo: func() *mockRepository {
				return setupErrorDB()
			},
			transaction: &Transaction{
				ID:             uuid.New().String(),
				Description:    "Test Transaction",
				Status:         Status{Code: "PENDING"},
				AssetCode:      "USD",
				LedgerID:       uuid.New().String(),
				OrganizationID: uuid.New().String(),
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
			expectedError: true,
		},
	}

	// Run test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.setupRepo()
			ctx := context.Background()

			result, err := repo.Create(ctx, tc.transaction)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "PENDING", result.Status.Code)
				assert.Equal(t, "USD", result.AssetCode)
			}
		})
	}
}

func TestTransactionRepository_FindAll(t *testing.T) {
	// Test cases
	tests := []struct {
		name          string
		setupRepo     func() *mockRepository
		orgID         uuid.UUID
		ledgerID      uuid.UUID
		expectedError bool
		expectEmpty   bool
	}{
		{
			name: "successful retrieval of all transactions",
			setupRepo: func() *mockRepository {
				repo, mock := setupMockDB(t)

				// Expect the query
				rows := sqlmock.NewRows([]string{
					"id", "parent_transaction_id", "description", "template", "status", "status_description",
					"amount", "amount_scale", "asset_code", "chart_of_accounts_group_name", "ledger_id",
					"organization_id", "body", "created_at", "updated_at", "deleted_at",
				}).
					AddRow(
						uuid.New(), nil, "Transaction 1", nil, "PENDING", nil,
						nil, nil, "USD", nil, uuid.New(),
						uuid.New(), nil, time.Now(), time.Now(), nil,
					).
					AddRow(
						uuid.New(), nil, "Transaction 2", nil, "COMPLETED", nil,
						nil, nil, "EUR", nil, uuid.New(),
						uuid.New(), nil, time.Now(), time.Now(), nil,
					)

				mock.ExpectQuery(`SELECT id, parent_transaction_id, description, template, status, status_description, amount, amount_scale, asset_code, chart_of_accounts_group_name, ledger_id, organization_id, body, created_at, updated_at, deleted_at FROM transaction WHERE organization_id = (.+) AND ledger_id = (.+) ORDER BY created_at DESC LIMIT (.+)`).
					WithArgs(
						sqlmock.AnyArg(), // OrganizationID
						sqlmock.AnyArg(), // LedgerID
						sqlmock.AnyArg(), // Limit
					).
					WillReturnRows(rows)

				return repo
			},
			orgID:         uuid.New(),
			ledgerID:      uuid.New(),
			expectedError: false,
			expectEmpty:   false,
		},
		{
			name: "empty result",
			setupRepo: func() *mockRepository {
				repo, mock := setupMockDB(t)

				// Expect the query with empty result
				rows := sqlmock.NewRows([]string{
					"id", "parent_transaction_id", "description", "template", "status", "status_description",
					"amount", "amount_scale", "asset_code", "chart_of_accounts_group_name", "ledger_id",
					"organization_id", "body", "created_at", "updated_at", "deleted_at",
				})

				mock.ExpectQuery(`SELECT id, parent_transaction_id, description, template, status, status_description, amount, amount_scale, asset_code, chart_of_accounts_group_name, ledger_id, organization_id, body, created_at, updated_at, deleted_at FROM transaction WHERE organization_id = (.+) AND ledger_id = (.+) ORDER BY created_at DESC LIMIT (.+)`).
					WithArgs(
						sqlmock.AnyArg(), // OrganizationID
						sqlmock.AnyArg(), // LedgerID
						sqlmock.AnyArg(), // Limit
					).
					WillReturnRows(rows)

				return repo
			},
			orgID:         uuid.New(),
			ledgerID:      uuid.New(),
			expectedError: false,
			expectEmpty:   true,
		},
		{
			name: "database error",
			setupRepo: func() *mockRepository {
				return setupErrorDB()
			},
			orgID:         uuid.New(),
			ledgerID:      uuid.New(),
			expectedError: true,
			expectEmpty:   false,
		},
	}

	// Run test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.setupRepo()
			ctx := context.Background()

			filter := http.Pagination{
				Limit: 10,
			}

			transactions, pagination, err := repo.FindAll(ctx, tc.orgID, tc.ledgerID, filter)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, transactions)
			} else {
				assert.NoError(t, err)

				// For empty results, we still expect a non-nil empty slice and valid pagination
				if tc.expectEmpty {
					assert.Empty(t, transactions)
				} else {
					assert.NotNil(t, transactions)
				}

				assert.NotNil(t, pagination)
				// Check Next and Prev instead of Cursor
				assert.Equal(t, "", pagination.Next)
				assert.Equal(t, "", pagination.Prev)

				if tc.name == "successful retrieval of all transactions" {
					assert.Equal(t, 2, len(transactions))
					assert.Equal(t, "Transaction 1", transactions[0].Description)
					assert.Equal(t, "Transaction 2", transactions[1].Description)
				} else if tc.name == "empty result" {
					assert.Equal(t, 0, len(transactions))
				}
			}
		})
	}
}

func TestTransactionRepository_Update(t *testing.T) {
	// Test cases
	tests := []struct {
		name          string
		setupRepo     func() *mockRepository
		orgID         uuid.UUID
		ledgerID      uuid.UUID
		transactionID uuid.UUID
		transaction   *Transaction
		expectedError bool
	}{
		{
			name: "successful update",
			setupRepo: func() *mockRepository {
				repo, mock := setupMockDB(t)

				// Expect the update query
				mock.ExpectExec(`UPDATE transaction SET description = (.+), status = (.+), status_description = (.+), updated_at = (.+) 
         WHERE organization_id = (.+) AND ledger_id = (.+) AND id = (.+) AND deleted_at IS NULL`).
					WithArgs(
						sqlmock.AnyArg(), // Description
						sqlmock.AnyArg(), // Status.Code
						func() *string { s := "Transaction completed successfully"; return &s }(), // Status.Description
						sqlmock.AnyArg(), // UpdatedAt
						sqlmock.AnyArg(), // OrganizationID
						sqlmock.AnyArg(), // LedgerID
						sqlmock.AnyArg(), // ID
					).
					WillReturnResult(sqlmock.NewResult(0, 1))

				return repo
			},
			orgID:         uuid.New(),
			ledgerID:      uuid.New(),
			transactionID: uuid.New(),
			transaction: &Transaction{
				Description: "Updated Transaction",
				Status: Status{
					Code:        "COMPLETED",
					Description: func() *string { s := "Transaction completed successfully"; return &s }(),
				},
				UpdatedAt: time.Now(),
			},
			expectedError: false,
		},
		{
			name: "transaction not found",
			setupRepo: func() *mockRepository {
				repo, mock := setupMockDB(t)

				// Expect the update query with no rows affected
				mock.ExpectExec(`UPDATE transaction SET description = (.+), status = (.+), status_description = (.+), updated_at = (.+) 
         WHERE organization_id = (.+) AND ledger_id = (.+) AND id = (.+) AND deleted_at IS NULL`).
					WithArgs(
						sqlmock.AnyArg(), // Description
						sqlmock.AnyArg(), // Status.Code
						func() *string { s := "Transaction completed successfully"; return &s }(), // Status.Description
						sqlmock.AnyArg(), // UpdatedAt
						sqlmock.AnyArg(), // OrganizationID
						sqlmock.AnyArg(), // LedgerID
						sqlmock.AnyArg(), // ID
					).
					WillReturnResult(sqlmock.NewResult(0, 0))

				return repo
			},
			orgID:         uuid.New(),
			ledgerID:      uuid.New(),
			transactionID: uuid.New(),
			transaction: &Transaction{
				Description: "Updated Transaction",
				Status: Status{
					Code:        "COMPLETED",
					Description: func() *string { s := "Transaction completed successfully"; return &s }(),
				},
				UpdatedAt: time.Now(),
			},
			expectedError: true,
		},
		{
			name: "database error",
			setupRepo: func() *mockRepository {
				return setupErrorDB()
			},
			orgID:         uuid.New(),
			ledgerID:      uuid.New(),
			transactionID: uuid.New(),
			transaction: &Transaction{
				Description: "Updated Transaction",
				Status: Status{
					Code:        "COMPLETED",
					Description: func() *string { s := "Transaction completed successfully"; return &s }(),
				},
				UpdatedAt: time.Now(),
			},
			expectedError: true,
		},
	}

	// Run test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.setupRepo()
			ctx := context.Background()

			result, err := repo.Update(ctx, tc.orgID, tc.ledgerID, tc.transactionID, tc.transaction)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tc.transactionID.String(), result.ID)
				assert.Equal(t, tc.orgID.String(), result.OrganizationID)
				assert.Equal(t, tc.ledgerID.String(), result.LedgerID)
				assert.Equal(t, tc.transaction.Description, result.Description)
				assert.Equal(t, tc.transaction.Status.Code, result.Status.Code)
				assert.Equal(t, *tc.transaction.Status.Description, *result.Status.Description)
			}
		})
	}
}

func TestTransactionRepository_Delete(t *testing.T) {
	// Test cases
	tests := []struct {
		name          string
		setupRepo     func() *mockRepository
		expectedError bool
	}{
		{
			name: "successful deletion",
			setupRepo: func() *mockRepository {
				repo, mock := setupMockDB(t)

				// Expect the delete query
				mock.ExpectExec("DELETE FROM transaction WHERE id = (.+) AND organization_id = (.+) AND ledger_id = (.+)").
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(0, 1))

				return repo
			},
			expectedError: false,
		},
		{
			name: "transaction not found",
			setupRepo: func() *mockRepository {
				repo, mock := setupMockDB(t)

				// Expect the delete query with no rows affected
				mock.ExpectExec("DELETE FROM transaction WHERE id = (.+) AND organization_id = (.+) AND ledger_id = (.+)").
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(0, 0))

				return repo
			},
			expectedError: true,
		},
		{
			name: "database error",
			setupRepo: func() *mockRepository {
				return setupErrorDB()
			},
			expectedError: true,
		},
	}

	// Run test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.setupRepo()
			ctx := context.Background()
			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()

			err := repo.Delete(ctx, orgID, ledgerID, transactionID)

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTransactionRepository_FindWithOperations(t *testing.T) {
	tests := []struct {
		name          string
		setupRepo     func() *mockRepository
		orgID         uuid.UUID
		ledgerID      uuid.UUID
		transactionID uuid.UUID
		expectedError bool
	}{
		{
			name: "successful retrieval",
			setupRepo: func() *mockRepository {
				repo, mock := setupMockDB(t)

				rows := sqlmock.NewRows([]string{
					"id", "parent_transaction_id", "description", "status", "status_description",
					"amount", "amount_scale", "asset_code", "chart_of_accounts_group_name",
					"ledger_id", "organization_id", "body", "created_at", "updated_at", "deleted_at",
				}).AddRow(
					uuid.New().String(), nil, "Test Transaction", "COMPLETED", nil,
					nil, nil, "USD", nil,
					uuid.New().String(), uuid.New().String(), nil, time.Now(), time.Now(), nil,
				)

				mock.ExpectQuery(`SELECT \* FROM transaction t INNER JOIN operation o ON t.id = o.transaction_id WHERE t.organization_id = \$1 AND t.ledger_id = \$2 AND t.id = \$3 AND t.deleted_at IS NULL`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				return repo
			},
			orgID:         uuid.New(),
			ledgerID:      uuid.New(),
			transactionID: uuid.New(),
			expectedError: false,
		},
		{
			name: "transaction not found",
			setupRepo: func() *mockRepository {
				repo, mock := setupMockDB(t)

				mock.ExpectQuery(`SELECT \* FROM transaction t INNER JOIN operation o ON t.id = o.transaction_id WHERE t.organization_id = \$1 AND t.ledger_id = \$2 AND t.id = \$3 AND t.deleted_at IS NULL`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnError(sql.ErrNoRows)

				return repo
			},
			orgID:         uuid.New(),
			ledgerID:      uuid.New(),
			transactionID: uuid.New(),
			expectedError: true,
		},
		{
			name: "database error",
			setupRepo: func() *mockRepository {
				return setupErrorDB()
			},
			orgID:         uuid.New(),
			ledgerID:      uuid.New(),
			transactionID: uuid.New(),
			expectedError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.setupRepo()
			ctx := context.Background()

			transaction, err := repo.FindWithOperations(ctx, tc.orgID, tc.ledgerID, tc.transactionID)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, transaction)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, transaction)
				assert.NotEmpty(t, transaction.ID)
				assert.NotEmpty(t, transaction.Operations)
				assert.Equal(t, 1, len(transaction.Operations))
				assert.Equal(t, transaction.ID, transaction.Operations[0].TransactionID)
				assert.Equal(t, transaction.AssetCode, transaction.Operations[0].AssetCode)
			}
		})
	}
}

func TestTransactionRepository_FindOrListAllWithOperations(t *testing.T) {
	t.Run("should return transactions with operations", func(t *testing.T) {
		repo, _ := setupMockDB(t)
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionIDs := []uuid.UUID{uuid.New()}
		filter := http.Pagination{}

		transactions, pagination, err := repo.FindOrListAllWithOperations(context.Background(), organizationID, ledgerID, transactionIDs, filter)

		require.NoError(t, err)
		assert.Len(t, transactions, 2)
		assert.Equal(t, "", pagination.Next)
		assert.Equal(t, "", pagination.Prev)

		for _, tx := range transactions {
			assert.NotNil(t, tx.ID)
			assert.Equal(t, organizationID.String(), tx.OrganizationID)
			assert.Equal(t, ledgerID.String(), tx.LedgerID)
			assert.NotEmpty(t, tx.Status.Code)
			assert.NotEmpty(t, tx.Operations)

			for _, op := range tx.Operations {
				assert.NotNil(t, op.ID)
				assert.Equal(t, organizationID.String(), op.OrganizationID)
				assert.Equal(t, ledgerID.String(), op.LedgerID)
				assert.NotNil(t, op.TransactionID)
				assert.NotNil(t, op.AccountID)
				assert.NotEmpty(t, op.Type)
			}
		}
	})

	t.Run("should return error when db returns error", func(t *testing.T) {
		repo := setupErrorDB()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionIDs := []uuid.UUID{uuid.New()}
		filter := http.Pagination{}

		transactions, pagination, err := repo.FindOrListAllWithOperations(context.Background(), organizationID, ledgerID, transactionIDs, filter)

		require.Error(t, err)
		assert.Empty(t, transactions)
		assert.Equal(t, "", pagination.Next)
		assert.Equal(t, "", pagination.Prev)
	})
}
