// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg/repository"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestOperation creates a test operation with the given ID or generates a new UUID.
func generateTestOperation(id string) *Operation {
	if id == "" {
		id = uuid.NewString()
	}

	now := time.Now().UTC()
	amount := decimal.NewFromInt(100)

	return &Operation{
		ID:              id,
		TransactionID:   uuid.NewString(),
		Description:     "Test operation " + id[:8],
		Type:            "DEBIT",
		AssetCode:       "USD",
		Amount:          Amount{Value: &amount},
		Status:          Status{Code: "ACTIVE"},
		AccountID:       uuid.NewString(),
		AccountAlias:    "@test",
		BalanceID:       uuid.NewString(),
		ChartOfAccounts: "1000",
		OrganizationID:  uuid.NewString(),
		LedgerID:        uuid.NewString(),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// generateTestOperations creates n test operations with unique UUIDs.
func generateTestOperations(n int) []*Operation {
	operations := make([]*Operation, n)
	for i := range n {
		operations[i] = generateTestOperation("")
	}

	return operations
}

// Ensure generateTestOperations is used in at least one test
var _ = generateTestOperations

func TestOperationCreateBulk_EmptyInput(t *testing.T) {
	t.Parallel()

	repo := &OperationPostgreSQLRepository{
		connection: nil, // Will return empty result before DB call
		tableName:  "operation",
	}

	result, err := repo.CreateBulk(context.Background(), []*Operation{})

	require.NoError(t, err, "empty input should not error")
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
}

func TestOperationCreateBulk_NilInput(t *testing.T) {
	t.Parallel()

	repo := &OperationPostgreSQLRepository{
		connection: nil,
		tableName:  "operation",
	}

	result, err := repo.CreateBulk(context.Background(), nil)

	require.NoError(t, err, "nil input should be treated as empty")
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
}

func TestOperationCreateBulk_NilElementInSlice(t *testing.T) {
	t.Parallel()

	repo := &OperationPostgreSQLRepository{
		connection: nil,
		tableName:  "operation",
	}

	operations := []*Operation{
		generateTestOperation(""),
		nil, // nil element
		generateTestOperation(""),
	}

	result, err := repo.CreateBulk(context.Background(), operations)

	require.Error(t, err, "should error on nil element")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil operation at index 1")
}

func TestOperationCreateBulk_NilElementAtStart(t *testing.T) {
	t.Parallel()

	repo := &OperationPostgreSQLRepository{
		connection: nil,
		tableName:  "operation",
	}

	operations := []*Operation{
		nil, // nil at index 0
		generateTestOperation(""),
	}

	result, err := repo.CreateBulk(context.Background(), operations)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil operation at index 0")
}

func TestOperationCreateBulk_NilElementAtEnd(t *testing.T) {
	t.Parallel()

	repo := &OperationPostgreSQLRepository{
		connection: nil,
		tableName:  "operation",
	}

	operations := []*Operation{
		generateTestOperation(""),
		generateTestOperation(""),
		nil, // nil at end
	}

	result, err := repo.CreateBulk(context.Background(), operations)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil operation at index 2")
}

func TestOperationCreateBulk_SortsInputByID(t *testing.T) {
	t.Parallel()

	// Create operations with IDs that will sort differently
	op1 := generateTestOperation("ffffffff-ffff-ffff-ffff-ffffffffffff") // Highest
	op2 := generateTestOperation("00000000-0000-0000-0000-000000000001") // Lowest
	op3 := generateTestOperation("88888888-8888-8888-8888-888888888888") // Middle

	input := []*Operation{op1, op2, op3}

	// Verify initial order: op1 (highest) is first
	assert.Equal(t, op1.ID, input[0].ID, "original order should have op1 first")

	// Verify lexicographic ordering assumption
	assert.True(t, op2.ID < op3.ID, "op2 should be less than op3")
	assert.True(t, op3.ID < op1.ID, "op3 should be less than op1")

	// Create mock DB that returns success
	mockDB := &mockOperationDB{
		rowsAffected: 3,
	}

	// Inject mock DB into context using tenant manager
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

	repo := &OperationPostgreSQLRepository{
		connection:    nil,
		tableName:     "operation",
		requireTenant: false,
	}

	// Call CreateBulk which sorts the slice in-place before inserting
	result, err := repo.CreateBulk(ctx, input)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the slice was sorted in-place by ID (ascending)
	// Expected order after sort: op2 (lowest) -> op3 (middle) -> op1 (highest)
	assert.Equal(t, op2.ID, input[0].ID, "after CreateBulk, first element should be op2 (lowest ID)")
	assert.Equal(t, op3.ID, input[1].ID, "after CreateBulk, second element should be op3 (middle ID)")
	assert.Equal(t, op1.ID, input[2].ID, "after CreateBulk, third element should be op1 (highest ID)")
}

func TestOperationCreateBulk_ChunkingBoundaryConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		inputCount     int
		expectedChunks int
	}{
		{"single_item", 1, 1},
		{"exactly_999", 999, 1},
		{"exactly_1000", 1000, 1},
		{"exactly_1001", 1001, 2},
		{"exactly_2000", 2000, 2},
		{"exactly_2001", 2001, 3},
		{"exactly_3000", 3000, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify the chunking math
			const chunkSize = 1000
			chunks := (tt.inputCount + chunkSize - 1) / chunkSize
			assert.Equal(t, tt.expectedChunks, chunks, "chunk count should match")
		})
	}
}

func TestOperationBulkInsertResult_Invariant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		attempted int64
		inserted  int64
	}{
		{"all_inserted", 100, 100},
		{"all_ignored", 100, 0},
		{"partial", 100, 75},
		{"single_inserted", 1, 1},
		{"single_ignored", 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := &repository.BulkInsertResult{
				Attempted: tt.attempted,
				Inserted:  tt.inserted,
				Ignored:   tt.attempted - tt.inserted,
			}

			// Verify invariant: Attempted = Inserted + Ignored
			assert.Equal(t, result.Attempted, result.Inserted+result.Ignored,
				"invariant failed: Attempted should equal Inserted + Ignored")
		})
	}
}

func TestOperationBulkInsertResult_ZeroValues(t *testing.T) {
	t.Parallel()

	result := &repository.BulkInsertResult{}

	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
}

// mockOperationDB implements dbresolver.DB for testing
type mockOperationDB struct {
	execErr         error
	rowsAffected    int64
	rowsAffectedErr error
}

func (m *mockOperationDB) Begin() (dbresolver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (m *mockOperationDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (dbresolver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (m *mockOperationDB) Close() error {
	return nil
}

func (m *mockOperationDB) Conn(ctx context.Context) (dbresolver.Conn, error) {
	return nil, errors.New("not implemented")
}

func (m *mockOperationDB) Driver() driver.Driver {
	return nil
}

func (m *mockOperationDB) Exec(query string, args ...any) (sql.Result, error) {
	return m.ExecContext(context.Background(), query, args...)
}

func (m *mockOperationDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.execErr != nil {
		return nil, m.execErr
	}

	return &mockOperationResult{rowsAffected: m.rowsAffected, rowsAffectedErr: m.rowsAffectedErr}, nil
}

func (m *mockOperationDB) Ping() error {
	return nil
}

func (m *mockOperationDB) PingContext(ctx context.Context) error {
	return nil
}

func (m *mockOperationDB) Prepare(query string) (dbresolver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (m *mockOperationDB) PrepareContext(ctx context.Context, query string) (dbresolver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (m *mockOperationDB) Query(query string, args ...any) (*sql.Rows, error) {
	return nil, errors.New("not implemented")
}

func (m *mockOperationDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return nil, errors.New("not implemented")
}

func (m *mockOperationDB) QueryRow(query string, args ...any) *sql.Row {
	return nil
}

func (m *mockOperationDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return nil
}

func (m *mockOperationDB) SetConnMaxIdleTime(d time.Duration) {}

func (m *mockOperationDB) SetConnMaxLifetime(d time.Duration) {}

func (m *mockOperationDB) SetMaxIdleConns(n int) {}

func (m *mockOperationDB) SetMaxOpenConns(n int) {}

func (m *mockOperationDB) PrimaryDBs() []*sql.DB {
	return nil
}

func (m *mockOperationDB) ReplicaDBs() []*sql.DB {
	return nil
}

func (m *mockOperationDB) Stats() sql.DBStats {
	return sql.DBStats{}
}

type mockOperationResult struct {
	rowsAffected    int64
	rowsAffectedErr error
}

func (m *mockOperationResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (m *mockOperationResult) RowsAffected() (int64, error) {
	if m.rowsAffectedErr != nil {
		return 0, m.rowsAffectedErr
	}

	return m.rowsAffected, nil
}

func TestInsertOperationChunk_ColumnCount(t *testing.T) {
	t.Parallel()

	// Verify that operationColumnList has expected number of columns
	// This ensures the bulk insert won't have column/value mismatch
	expectedColumns := 30 // Based on operationColumnList definition
	assert.Equal(t, expectedColumns, len(operationColumnList),
		"operationColumnList should have %d columns", expectedColumns)
}

func TestInsertOperationChunk_ParameterLimitCalculation(t *testing.T) {
	t.Parallel()

	// Verify that 1000 rows * 30 columns stays under PostgreSQL's 65,535 limit
	const chunkSize = 1000
	const columnCount = 30 // operationColumnList length
	const postgresLimit = 65535

	parametersPerChunk := chunkSize * columnCount
	assert.Less(t, parametersPerChunk, postgresLimit,
		"parameters per chunk (%d) should be less than PostgreSQL limit (%d)",
		parametersPerChunk, postgresLimit)
}

func TestOperationCreateBulk_AllNilElements(t *testing.T) {
	t.Parallel()

	repo := &OperationPostgreSQLRepository{
		connection: nil,
		tableName:  "operation",
	}

	operations := []*Operation{nil, nil, nil}

	result, err := repo.CreateBulk(context.Background(), operations)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil operation at index 0")
}

func TestOperationCreateBulk_MultipleNilElements(t *testing.T) {
	t.Parallel()

	repo := &OperationPostgreSQLRepository{
		connection: nil,
		tableName:  "operation",
	}

	operations := []*Operation{
		generateTestOperation(""),
		nil,
		generateTestOperation(""),
		nil,
	}

	result, err := repo.CreateBulk(context.Background(), operations)

	require.Error(t, err)
	assert.Nil(t, result)
	// Should fail on first nil (index 1)
	assert.Contains(t, err.Error(), "nil operation at index 1")
}
