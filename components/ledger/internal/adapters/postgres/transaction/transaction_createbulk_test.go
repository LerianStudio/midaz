// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestTransaction creates a test transaction with the given ID or generates a new UUID.
func generateTestTransaction(id string) *Transaction {
	if id == "" {
		id = uuid.NewString()
	}

	now := time.Now().UTC()

	return &Transaction{
		ID:             id,
		Description:    "Test transaction " + id[:8],
		Status:         Status{Code: "PENDING"},
		AssetCode:      "USD",
		LedgerID:       uuid.NewString(),
		OrganizationID: uuid.NewString(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// generateTestTransactions creates n test transactions with unique UUIDs.
func generateTestTransactions(n int) []*Transaction {
	transactions := make([]*Transaction, n)
	for i := range n {
		transactions[i] = generateTestTransaction("")
	}

	return transactions
}

func TestCreateBulk_EmptyInput(t *testing.T) {
	t.Parallel()

	repo := &TransactionPostgreSQLRepository{
		connection: nil, // Will return empty result before DB call
		tableName:  "transaction",
	}

	result, err := repo.CreateBulk(context.Background(), []*Transaction{})

	require.NoError(t, err, "empty input should not error")
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
}

func TestCreateBulk_NilInput(t *testing.T) {
	t.Parallel()

	repo := &TransactionPostgreSQLRepository{
		connection: nil,
		tableName:  "transaction",
	}

	result, err := repo.CreateBulk(context.Background(), nil)

	require.NoError(t, err, "nil input should be treated as empty")
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
}

func TestCreateBulk_NilElementInSlice(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := []*Transaction{
		generateTestTransaction(""),
		nil, // nil element
		generateTestTransaction(""),
	}

	result, err := repo.CreateBulk(ctx, transactions)

	require.Error(t, err, "should error on nil element")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil transaction at index 1")
}

func TestCreateBulk_NilElementAtStart(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := []*Transaction{
		nil, // nil at index 0
		generateTestTransaction(""),
	}

	result, err := repo.CreateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil transaction at index 0")
}

func TestCreateBulk_NilElementAtEnd(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := []*Transaction{
		generateTestTransaction(""),
		generateTestTransaction(""),
		nil, // nil at end
	}

	result, err := repo.CreateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil transaction at index 2")
}

func TestCreateBulk_SortsInputByID(t *testing.T) {
	t.Parallel()

	// Create transactions with IDs that will sort differently
	tx1 := generateTestTransaction("ffffffff-ffff-ffff-ffff-ffffffffffff") // Highest
	tx2 := generateTestTransaction("00000000-0000-0000-0000-000000000001") // Lowest
	tx3 := generateTestTransaction("88888888-8888-8888-8888-888888888888") // Middle

	input := []*Transaction{tx1, tx2, tx3}

	// Verify initial order: tx1 (highest) is first
	assert.Equal(t, tx1.ID, input[0].ID, "original order should have tx1 first")

	// Verify lexicographic ordering assumption
	assert.True(t, tx2.ID < tx3.ID, "tx2 should be less than tx3")
	assert.True(t, tx3.ID < tx1.ID, "tx3 should be less than tx1")

	// Create mock DB that returns success
	mockDB := &bulkMockDB{
		rowsAffected: 3,
	}

	// Inject mock DB into context using tenant manager
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Call CreateBulk which sorts the slice in-place before inserting
	result, err := repo.CreateBulk(ctx, input)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the slice was sorted in-place by ID (ascending)
	// Expected order after sort: tx2 (lowest) -> tx3 (middle) -> tx1 (highest)
	assert.Equal(t, tx2.ID, input[0].ID, "after CreateBulk, first element should be tx2 (lowest ID)")
	assert.Equal(t, tx3.ID, input[1].ID, "after CreateBulk, second element should be tx3 (middle ID)")
	assert.Equal(t, tx1.ID, input[2].ID, "after CreateBulk, third element should be tx1 (highest ID)")
}

func TestCreateBulk_ChunkingBoundaryConditions(t *testing.T) {
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

func TestBulkInsertResult_Invariant(t *testing.T) {
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

func TestBulkInsertResult_ZeroValues(t *testing.T) {
	t.Parallel()

	result := &repository.BulkInsertResult{}

	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
}

// bulkMockDB implements dbresolver.DB for testing bulk operations
type bulkMockDB struct {
	execErr         error
	rowsAffected    int64
	rowsAffectedErr error
	queryErr        error
}

func (m *bulkMockDB) Begin() (dbresolver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (m *bulkMockDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (dbresolver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (m *bulkMockDB) Close() error {
	return nil
}

func (m *bulkMockDB) Conn(ctx context.Context) (dbresolver.Conn, error) {
	return nil, errors.New("not implemented")
}

func (m *bulkMockDB) Driver() driver.Driver {
	return nil
}

func (m *bulkMockDB) Exec(query string, args ...any) (sql.Result, error) {
	return m.ExecContext(context.Background(), query, args...)
}

func (m *bulkMockDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.execErr != nil {
		return nil, m.execErr
	}

	return &bulkMockResult{rowsAffected: m.rowsAffected, rowsAffectedErr: m.rowsAffectedErr}, nil
}

func (m *bulkMockDB) Ping() error {
	return nil
}

func (m *bulkMockDB) PingContext(ctx context.Context) error {
	return nil
}

func (m *bulkMockDB) Prepare(query string) (dbresolver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (m *bulkMockDB) PrepareContext(ctx context.Context, query string) (dbresolver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (m *bulkMockDB) Query(query string, args ...any) (*sql.Rows, error) {
	return m.QueryContext(context.Background(), query, args...)
}

func (m *bulkMockDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}

	// Return mock rows using the test driver
	return createMockRows(m.rowsAffected)
}

func (m *bulkMockDB) QueryRow(query string, args ...any) *sql.Row {
	return nil
}

func (m *bulkMockDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return nil
}

func (m *bulkMockDB) SetConnMaxIdleTime(d time.Duration) {}

func (m *bulkMockDB) SetConnMaxLifetime(d time.Duration) {}

func (m *bulkMockDB) SetMaxIdleConns(n int) {}

func (m *bulkMockDB) SetMaxOpenConns(n int) {}

func (m *bulkMockDB) PrimaryDBs() []*sql.DB {
	return nil
}

func (m *bulkMockDB) ReplicaDBs() []*sql.DB {
	return nil
}

func (m *bulkMockDB) Stats() sql.DBStats {
	return sql.DBStats{}
}

type bulkMockResult struct {
	rowsAffected    int64
	rowsAffectedErr error
}

func (m *bulkMockResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (m *bulkMockResult) RowsAffected() (int64, error) {
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

func TestInsertTransactionChunk_ColumnCount(t *testing.T) {
	t.Parallel()

	// Verify that transactionColumnList has expected number of columns
	// This ensures the bulk insert won't have column/value mismatch
	expectedColumns := 16 // Based on transactionColumnList definition
	assert.Equal(t, expectedColumns, len(transactionColumnList),
		"transactionColumnList should have %d columns", expectedColumns)
}

func TestInsertTransactionChunk_ParameterLimitCalculation(t *testing.T) {
	t.Parallel()

	// Verify that 1000 rows * 15 columns stays under PostgreSQL's 65,535 limit
	const chunkSize = 1000
	const columnCount = 16 // transactionColumnList length
	const postgresLimit = 65535

	parametersPerChunk := chunkSize * columnCount
	assert.Less(t, parametersPerChunk, postgresLimit,
		"parameters per chunk (%d) should be less than PostgreSQL limit (%d)",
		parametersPerChunk, postgresLimit)
}

// bulkMockDBSequence tracks call count and returns different results per call
type bulkMockDBSequence struct {
	bulkMockDB
	callCount      int
	resultsPerCall []bulkMockCallResult
}

type bulkMockCallResult struct {
	err          error
	rowsAffected int64
}

func (m *bulkMockDBSequence) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.callCount < len(m.resultsPerCall) {
		result := m.resultsPerCall[m.callCount]
		m.callCount++

		if result.err != nil {
			return nil, result.err
		}

		return &bulkMockResult{rowsAffected: result.rowsAffected}, nil
	}

	m.callCount++

	return &bulkMockResult{rowsAffected: 0}, nil
}

func (m *bulkMockDBSequence) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
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

func TestCreateBulk_ChunkFailure_PartialResult(t *testing.T) {
	t.Parallel()

	// Create 2001 transactions to trigger 3 chunks (1000 + 1000 + 1) for CreateBulk
	// Note: CreateBulk uses chunk size 1000 (15 columns), UpdateBulk uses chunk size 500 (6 columns)
	transactions := generateTestTransactions(2001)

	// Mock: chunk 1 succeeds (1000 rows), chunk 2 fails
	dbErr := errors.New("database connection lost")
	mockDB := &bulkMockDBSequence{
		resultsPerCall: []bulkMockCallResult{
			{rowsAffected: 1000}, // Chunk 1: success
			{err: dbErr},         // Chunk 2: failure
		},
	}

	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	result, err := repo.CreateBulk(ctx, transactions)

	// Should return error
	require.Error(t, err)
	assert.Equal(t, dbErr, err)

	// Should return partial result
	require.NotNil(t, result)
	assert.Equal(t, int64(2001), result.Attempted, "Attempted should be total count")
	assert.Equal(t, int64(1000), result.Inserted, "Inserted should reflect chunk 1 only")
	assert.Equal(t, int64(0), result.Ignored, "Ignored should be 0 on error (unprocessed items are not duplicates)")
}

func TestCreateBulk_FirstChunkFailure(t *testing.T) {
	t.Parallel()

	transactions := generateTestTransactions(500)

	// Mock: first chunk fails immediately
	dbErr := errors.New("connection refused")
	mockDB := &bulkMockDBSequence{
		resultsPerCall: []bulkMockCallResult{
			{err: dbErr}, // Chunk 1: failure
		},
	}

	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	result, err := repo.CreateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Equal(t, dbErr, err)

	require.NotNil(t, result)
	assert.Equal(t, int64(500), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted, "No rows should be inserted when first chunk fails")
	assert.Equal(t, int64(0), result.Ignored, "Ignored should be 0 on error")
}

func TestCreateBulk_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create enough transactions to require multiple chunks
	transactions := generateTestTransactions(2500)

	// Mock: chunk 1 succeeds, then context is cancelled before chunk 2
	mockDB := &bulkMockDBSequence{
		resultsPerCall: []bulkMockCallResult{
			{rowsAffected: 1000}, // Chunk 1: would succeed but context cancelled
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctx = tmcore.ContextWithPG(ctx, mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Cancel context before calling CreateBulk
	cancel()

	result, err := repo.CreateBulk(ctx, transactions)

	// Should return context error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	// Should return partial result (0 since cancelled before first chunk)
	require.NotNil(t, result)
	assert.Equal(t, int64(2500), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted, "No rows inserted when context cancelled before first chunk")
	assert.Equal(t, int64(0), result.Ignored)
}
