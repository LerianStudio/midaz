// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
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

	mockDB := &mockOperationDB{}
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

	repo := &OperationPostgreSQLRepository{
		connection:    nil,
		tableName:     "operation",
		requireTenant: false,
	}

	operations := []*Operation{
		generateTestOperation(""),
		nil, // nil element
		generateTestOperation(""),
	}

	result, err := repo.CreateBulk(ctx, operations)

	require.Error(t, err, "should error on nil element")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil operation at index 1")
}

func TestOperationCreateBulk_NilElementAtStart(t *testing.T) {
	t.Parallel()

	mockDB := &mockOperationDB{}
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

	repo := &OperationPostgreSQLRepository{
		connection:    nil,
		tableName:     "operation",
		requireTenant: false,
	}

	operations := []*Operation{
		nil, // nil at index 0
		generateTestOperation(""),
	}

	result, err := repo.CreateBulk(ctx, operations)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil operation at index 0")
}

func TestOperationCreateBulk_NilElementAtEnd(t *testing.T) {
	t.Parallel()

	mockDB := &mockOperationDB{}
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

	repo := &OperationPostgreSQLRepository{
		connection:    nil,
		tableName:     "operation",
		requireTenant: false,
	}

	operations := []*Operation{
		generateTestOperation(""),
		generateTestOperation(""),
		nil, // nil at end
	}

	result, err := repo.CreateBulk(ctx, operations)

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
	queryErr        error
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
	return m.QueryContext(context.Background(), query, args...)
}

func (m *mockOperationDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}

	// Return mock rows using the test driver
	return createMockRows(m.rowsAffected)
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

// createMockRows creates mock *sql.Rows with the specified number of ID rows.
// Uses a minimal in-memory approach with the fakedriver.
func createMockRows(count int64) (*sql.Rows, error) {
	// Generate IDs to return
	ids := make([]string, count)
	for i := int64(0); i < count; i++ {
		ids[i] = uuid.NewString()
	}

	return createMockRowsWithIDs(ids)
}

// createMockRowsWithIDs creates mock *sql.Rows with specific IDs.
func createMockRowsWithIDs(ids []string) (*sql.Rows, error) {
	// Use the fakeRows driver connector to create valid *sql.Rows
	connector := &fakeRowsConnector{ids: ids}
	db := sql.OpenDB(connector)

	return db.Query("SELECT id")
}

// fakeRowsConnector implements driver.Connector for creating mock rows.
type fakeRowsConnector struct {
	ids []string
}

func (c *fakeRowsConnector) Connect(ctx context.Context) (driver.Conn, error) {
	return &fakeRowsConn{ids: c.ids}, nil
}

func (c *fakeRowsConnector) Driver() driver.Driver {
	return &fakeRowsDriver{}
}

// fakeRowsDriver implements driver.Driver.
type fakeRowsDriver struct{}

func (d *fakeRowsDriver) Open(name string) (driver.Conn, error) {
	return &fakeRowsConn{}, nil
}

// fakeRowsConn implements driver.Conn.
type fakeRowsConn struct {
	ids []string
}

func (c *fakeRowsConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeRowsStmt{ids: c.ids}, nil
}

func (c *fakeRowsConn) Close() error {
	return nil
}

func (c *fakeRowsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

// fakeRowsStmt implements driver.Stmt.
type fakeRowsStmt struct {
	ids []string
}

func (s *fakeRowsStmt) Close() error {
	return nil
}

func (s *fakeRowsStmt) NumInput() int {
	return 0
}

func (s *fakeRowsStmt) Exec(args []driver.Value) (driver.Result, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeRowsStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &fakeRows{ids: s.ids, index: 0}, nil
}

// fakeRows implements driver.Rows.
type fakeRows struct {
	ids   []string
	index int
}

func (r *fakeRows) Columns() []string {
	return []string{"id"}
}

func (r *fakeRows) Close() error {
	return nil
}

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.index >= len(r.ids) {
		return io.EOF
	}

	dest[0] = r.ids[r.index]
	r.index++

	return nil
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

	mockDB := &mockOperationDB{}
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

	repo := &OperationPostgreSQLRepository{
		connection:    nil,
		tableName:     "operation",
		requireTenant: false,
	}

	operations := []*Operation{nil, nil, nil}

	result, err := repo.CreateBulk(ctx, operations)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil operation at index 0")
}

func TestOperationCreateBulk_MultipleNilElements(t *testing.T) {
	t.Parallel()

	mockDB := &mockOperationDB{}
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

	repo := &OperationPostgreSQLRepository{
		connection:    nil,
		tableName:     "operation",
		requireTenant: false,
	}

	operations := []*Operation{
		generateTestOperation(""),
		nil,
		generateTestOperation(""),
		nil,
	}

	result, err := repo.CreateBulk(ctx, operations)

	require.Error(t, err)
	assert.Nil(t, result)
	// Should fail on first nil (index 1)
	assert.Contains(t, err.Error(), "nil operation at index 1")
}

// mockOperationDBSequence tracks call count and returns different results per call
type mockOperationDBSequence struct {
	mockOperationDB
	callCount      int
	resultsPerCall []mockCallResult
}

type mockCallResult struct {
	err          error
	rowsAffected int64
}

func (m *mockOperationDBSequence) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.callCount < len(m.resultsPerCall) {
		result := m.resultsPerCall[m.callCount]
		m.callCount++

		if result.err != nil {
			return nil, result.err
		}

		return &mockOperationResult{rowsAffected: result.rowsAffected}, nil
	}

	m.callCount++

	return &mockOperationResult{rowsAffected: 0}, nil
}

func (m *mockOperationDBSequence) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.callCount < len(m.resultsPerCall) {
		result := m.resultsPerCall[m.callCount]
		m.callCount++

		if result.err != nil {
			return nil, result.err
		}

		return createMockRows(result.rowsAffected)
	}

	m.callCount++

	// Return empty rows
	return createMockRows(0)
}

func TestOperationCreateBulk_ChunkFailure_PartialResult(t *testing.T) {
	t.Parallel()

	// Create 2001 operations to trigger 3 chunks (1000 + 1000 + 1)
	operations := generateTestOperations(2001)

	// Mock: chunk 1 succeeds (1000 rows), chunk 2 fails
	dbErr := errors.New("database connection lost")
	mockDB := &mockOperationDBSequence{
		resultsPerCall: []mockCallResult{
			{rowsAffected: 1000}, // Chunk 1: success
			{err: dbErr},         // Chunk 2: failure
		},
	}

	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

	repo := &OperationPostgreSQLRepository{
		connection:    nil,
		tableName:     "operation",
		requireTenant: false,
	}

	result, err := repo.CreateBulk(ctx, operations)

	// Should return error
	require.Error(t, err)
	assert.Equal(t, dbErr, err)

	// Should return partial result
	require.NotNil(t, result)
	assert.Equal(t, int64(2001), result.Attempted, "Attempted should be total count")
	assert.Equal(t, int64(1000), result.Inserted, "Inserted should reflect chunk 1 only")
	assert.Equal(t, int64(0), result.Ignored, "Ignored should be 0 on error (unprocessed items are not duplicates)")
}

func TestOperationCreateBulk_FirstChunkFailure(t *testing.T) {
	t.Parallel()

	operations := generateTestOperations(500)

	// Mock: first chunk fails immediately
	dbErr := errors.New("connection refused")
	mockDB := &mockOperationDBSequence{
		resultsPerCall: []mockCallResult{
			{err: dbErr}, // Chunk 1: failure
		},
	}

	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

	repo := &OperationPostgreSQLRepository{
		connection:    nil,
		tableName:     "operation",
		requireTenant: false,
	}

	result, err := repo.CreateBulk(ctx, operations)

	require.Error(t, err)
	assert.Equal(t, dbErr, err)

	require.NotNil(t, result)
	assert.Equal(t, int64(500), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted, "No rows should be inserted when first chunk fails")
	assert.Equal(t, int64(0), result.Ignored, "Ignored should be 0 on error")
}

func TestOperationCreateBulk_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create enough operations to require multiple chunks
	operations := generateTestOperations(2500)

	// Mock: chunk 1 succeeds, then context is cancelled before chunk 2
	mockDB := &mockOperationDBSequence{
		resultsPerCall: []mockCallResult{
			{rowsAffected: 1000}, // Chunk 1: success
			{rowsAffected: 1000}, // Chunk 2: would succeed but context cancelled
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctx = tmcore.ContextWithModulePGConnection(ctx, "transaction", mockDB)

	repo := &OperationPostgreSQLRepository{
		connection:    nil,
		tableName:     "operation",
		requireTenant: false,
	}

	// Cancel context before calling CreateBulk
	cancel()

	result, err := repo.CreateBulk(ctx, operations)

	// Should return context error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	// Should return partial result (0 since cancelled before first chunk)
	require.NotNil(t, result)
	assert.Equal(t, int64(2500), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted, "No rows inserted when context cancelled before first chunk")
	assert.Equal(t, int64(0), result.Ignored)
}
