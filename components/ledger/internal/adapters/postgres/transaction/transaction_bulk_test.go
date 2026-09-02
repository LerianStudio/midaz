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

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/repository"
)

// fixedBulkTestTime stamps every generated fixture. No assertion reads it, so a fixed
// value costs nothing and keeps the fixtures reproducible.
var fixedBulkTestTime = time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)

// generateTestTransaction creates a test transaction with the given ID or generates a new UUID.
func generateTestTransaction(id string) *Transaction {
	if id == "" {
		id = uuid.NewString()
	}

	now := fixedBulkTestTime

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

// TestCreateBulk_ChunkingBoundaryConditions drives CreateBulk at the boundaries of
// createBulkChunkSize and counts the statements the DB actually received, so the
// assertion is on production's chunk loop rather than on arithmetic the test repeats.
// Input sizes are expressed relative to the constant: a change to it moves the
// boundaries here with it instead of leaving the table describing the old value.
func TestCreateBulk_ChunkingBoundaryConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		inputCount     int
		expectedChunks int
	}{
		{"single_item", 1, 1},
		{"one_below_chunk", createBulkChunkSize - 1, 1},
		{"exactly_one_chunk", createBulkChunkSize, 1},
		{"one_above_chunk", createBulkChunkSize + 1, 2},
		{"exactly_two_chunks", createBulkChunkSize * 2, 2},
		{"one_above_two_chunks", createBulkChunkSize*2 + 1, 3},
		{"exactly_three_chunks", createBulkChunkSize * 3, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockDB := &bulkMockDBSequence{}
			ctx := tmcore.ContextWithPG(context.Background(), mockDB)

			repo := &TransactionPostgreSQLRepository{
				connection:    nil,
				tableName:     "transaction",
				requireTenant: false,
			}

			result, err := repo.CreateBulk(ctx, generateTestTransactions(tt.inputCount))
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, tt.expectedChunks, mockDB.callCount,
				"CreateBulk must issue one statement per chunk")
			assert.Equal(t, int64(tt.inputCount), result.Attempted,
				"every input must be attempted regardless of chunking")
		})
	}
}

// TestBulkInsertResult_Invariant asserts Attempted == Inserted + Ignored on results
// CreateBulk actually produced, across the outcomes that reach different arms of its
// accounting: every row inserted, every row a duplicate, and a partial insert. The
// identity is the caller's basis for reporting how many rows were skipped, and only
// CreateBulk decides it — a result the test assembles itself proves nothing.
func TestBulkInsertResult_Invariant(t *testing.T) {
	t.Parallel()

	const attempted = 100

	tests := []struct {
		name         string
		rowsReturned int64
	}{
		{"all_inserted", attempted},
		{"all_ignored", 0},
		{"partial", 75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockDB := &bulkMockDBSequence{
				resultsPerCall: []bulkMockCallResult{{rowsAffected: tt.rowsReturned}},
			}
			ctx := tmcore.ContextWithPG(context.Background(), mockDB)

			repo := &TransactionPostgreSQLRepository{
				connection:    nil,
				tableName:     "transaction",
				requireTenant: false,
			}

			result, err := repo.CreateBulk(ctx, generateTestTransactions(attempted))
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, int64(attempted), result.Attempted)
			assert.Equal(t, tt.rowsReturned, result.Inserted)
			assert.Equal(t, result.Attempted, result.Inserted+result.Ignored,
				"Attempted must equal Inserted + Ignored")
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
	expectedColumns := 19 // Based on transactionColumnList definition
	assert.Equal(t, expectedColumns, len(transactionColumnList),
		"transactionColumnList should have %d columns", expectedColumns)
}

// TestInsertTransactionChunk_ParameterLimitCalculation pins the headroom a full chunk
// leaves under PostgreSQL's placeholder ceiling. Both factors come from production —
// createBulkChunkSize and the length of transactionColumnList, the same list
// insertTransactionChunk passes to squirrel's Columns — so adding a column or raising
// the chunk size is what has to clear this bound, which a hardcoded 18 could not see.
func TestInsertTransactionChunk_ParameterLimitCalculation(t *testing.T) {
	t.Parallel()

	// PostgreSQL's wire protocol caps bind parameters per statement at 65,535.
	const postgresParameterLimit = 65535

	parametersPerChunk := createBulkChunkSize * len(transactionColumnList)

	assert.Less(t, parametersPerChunk, postgresParameterLimit,
		"a full chunk binds %d parameters, over PostgreSQL's %d limit: lower createBulkChunkSize",
		parametersPerChunk, postgresParameterLimit)
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
	// Note: CreateBulk uses chunk size 1000 (19 columns, matching insertTransactionChunk),
	// UpdateBulk uses chunk size 500 (6 columns)
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

// =============================================================================
// UpdateBulk
// =============================================================================

func TestUpdateBulk_EmptyInput(t *testing.T) {
	t.Parallel()

	repo := &TransactionPostgreSQLRepository{
		connection: nil, // Will return empty result before DB call
		tableName:  "transaction",
	}

	result, err := repo.UpdateBulk(context.Background(), []*Transaction{})

	require.NoError(t, err, "empty input should not error")
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulk_NilInput(t *testing.T) {
	t.Parallel()

	repo := &TransactionPostgreSQLRepository{
		connection: nil,
		tableName:  "transaction",
	}

	result, err := repo.UpdateBulk(context.Background(), nil)

	require.NoError(t, err, "nil input should be treated as empty")
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulk_NilElementInSlice(t *testing.T) {
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

	result, err := repo.UpdateBulk(ctx, transactions)

	require.Error(t, err, "should error on nil element")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil transaction at index 1")
}

func TestUpdateBulk_NilElementAtStart(t *testing.T) {
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

	result, err := repo.UpdateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil transaction at index 0")
}

func TestUpdateBulk_SingleTransaction_AllUpdated(t *testing.T) {
	t.Parallel()

	// With batched update, a single transaction results in a single ExecContext call
	// rowsAffected = 1 means the transaction was updated
	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := []*Transaction{
		generateTestTransaction(""),
	}
	transactions[0].Status = Status{Code: "APPROVED"}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(1), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulk_SingleTransaction_Unchanged(t *testing.T) {
	t.Parallel()

	// rowsAffected = 0 means status already matches (no update needed)
	mockDB := &bulkMockDB{rowsAffected: 0}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := []*Transaction{
		generateTestTransaction(""),
	}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
	assert.Equal(t, int64(1), result.Unchanged)
}

func TestUpdateBulk_MultipleTransactions_MixedResults(t *testing.T) {
	t.Parallel()

	// With batched update, all transactions in a chunk are updated in a single ExecContext call.
	// The rowsAffected reflects how many rows were actually updated (status changed).
	// For 3 transactions where 2 have status changes and 1 doesn't, rowsAffected = 2.
	mockDB := &bulkMockDB{rowsAffected: 2}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := generateTestTransactions(3)

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Attempted)
	assert.Equal(t, int64(2), result.Updated)
	assert.Equal(t, int64(1), result.Unchanged)
}

func TestUpdateBulk_SortsByID(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create transactions with IDs in reverse order
	transactions := []*Transaction{
		generateTestTransaction("zzz00000-0000-0000-0000-000000000003"),
		generateTestTransaction("aaa00000-0000-0000-0000-000000000001"),
		generateTestTransaction("mmm00000-0000-0000-0000-000000000002"),
	}

	_, err := repo.UpdateBulk(ctx, transactions)
	require.NoError(t, err)

	// Verify transactions were sorted in-place
	assert.Equal(t, "aaa00000-0000-0000-0000-000000000001", transactions[0].ID)
	assert.Equal(t, "mmm00000-0000-0000-0000-000000000002", transactions[1].ID)
	assert.Equal(t, "zzz00000-0000-0000-0000-000000000003", transactions[2].ID)
}

func TestUpdateBulk_DatabaseError(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("database connection lost")
	mockDB := &bulkMockDB{execErr: dbErr}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := generateTestTransactions(2)

	result, err := repo.UpdateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Equal(t, dbErr, err)
	// Partial result should be returned
	assert.Equal(t, int64(2), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
}

func TestUpdateBulkTx_NilExecutor(t *testing.T) {
	t.Parallel()

	repo := &TransactionPostgreSQLRepository{
		connection: nil,
		tableName:  "transaction",
	}

	transactions := generateTestTransactions(1)

	result, err := repo.UpdateBulkTx(context.Background(), nil, transactions)

	require.Error(t, err)
	assert.Equal(t, repository.ErrNilDBExecutor, err)
	assert.Nil(t, result)
}

func TestUpdateBulkTx_EmptyInput(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{}

	repo := &TransactionPostgreSQLRepository{
		connection: nil,
		tableName:  "transaction",
	}

	result, err := repo.UpdateBulkTx(context.Background(), mockDB, []*Transaction{})

	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulkTx_Success(t *testing.T) {
	t.Parallel()

	// With batched update, all transactions are updated in a single ExecContext call.
	// rowsAffected should equal the number of transactions that were actually updated.
	mockDB := &bulkMockDB{rowsAffected: 2}

	repo := &TransactionPostgreSQLRepository{
		connection: nil,
		tableName:  "transaction",
	}

	transactions := generateTestTransactions(2)

	result, err := repo.UpdateBulkTx(context.Background(), mockDB, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Attempted)
	assert.Equal(t, int64(2), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulk_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create 1001 transactions to trigger chunking (500 + 500 + 1 = 3 chunks)
	transactions := generateTestTransactions(1001)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx = tmcore.ContextWithPG(ctx, mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	// When context is cancelled before any chunk is submitted,
	// Attempted reflects only rows actually submitted (0 in this case)
	assert.Equal(t, int64(0), result.Attempted)
}

func TestUpdateBulk_StatusTransition_PendingToApproved(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create transaction with status transition
	tx := generateTestTransaction("")
	tx.Status = Status{Code: "APPROVED", Description: ptr("Transaction approved")}

	transactions := []*Transaction{tx}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(1), result.Updated)
}

func TestUpdateBulk_StatusTransition_PendingToCanceled(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create transaction with status transition
	tx := generateTestTransaction("")
	tx.Status = Status{Code: "CANCELED", Description: ptr("Transaction canceled by user")}

	transactions := []*Transaction{tx}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(1), result.Updated)
}

// updateBulkQueryCaptureMock captures the query and args for verification
type updateBulkQueryCaptureMock struct {
	bulkMockDB
	capturedQuery string
	capturedArgs  []any
	callCount     int
}

func (m *updateBulkQueryCaptureMock) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	m.capturedQuery = query
	m.capturedArgs = args
	m.callCount++

	return &bulkMockResult{rowsAffected: m.rowsAffected}, nil
}

func TestUpdateBulk_BatchedQuery_SingleExecPerChunk(t *testing.T) {
	t.Parallel()

	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 5},
	}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create 5 transactions - should be processed in a single batch (under chunk size of 500)
	transactions := generateTestTransactions(5)
	for i, tx := range transactions {
		tx.Status = Status{Code: "APPROVED", Description: ptr("Approved " + string(rune('A'+i)))}
	}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(5), result.Attempted)
	assert.Equal(t, int64(5), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)

	// Verify single ExecContext call for all 5 transactions (batched update)
	assert.Equal(t, 1, mockDB.callCount, "batched update should make exactly 1 ExecContext call for chunk")

	// Verify query uses UPDATE...FROM (VALUES...) pattern with org/ledger scoping
	assert.Contains(t, mockDB.capturedQuery, "UPDATE transaction t")
	assert.Contains(t, mockDB.capturedQuery, "FROM (VALUES")
	assert.Contains(t, mockDB.capturedQuery, "t.status != v.new_status")
	assert.Contains(t, mockDB.capturedQuery, "t.deleted_at IS NULL")
	assert.Contains(t, mockDB.capturedQuery, "t.organization_id = v.organization_id")
	assert.Contains(t, mockDB.capturedQuery, "t.ledger_id = v.ledger_id")

	// Verify parameter count: 5 transactions * 6 parameters each = 30 parameters
	// Parameters: id, organization_id, ledger_id, status, status_description, updated_at
	assert.Equal(t, 30, len(mockDB.capturedArgs), "should have 6 parameters per transaction")
}

func TestUpdateBulk_BatchedQuery_ParameterStructure(t *testing.T) {
	t.Parallel()

	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 2},
	}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create 2 transactions with specific statuses
	tx1 := generateTestTransaction("11111111-1111-1111-1111-111111111111")
	tx1.Status = Status{Code: "APPROVED", Description: ptr("First approved")}

	tx2 := generateTestTransaction("22222222-2222-2222-2222-222222222222")
	tx2.Status = Status{Code: "CANCELED", Description: ptr("Second canceled")}

	transactions := []*Transaction{tx1, tx2}

	_, err := repo.UpdateBulk(ctx, transactions)
	require.NoError(t, err)

	// Verify parameters are in correct order (sorted by ID)
	// After sorting: tx1 (11111111...) comes before tx2 (22222222...)
	// Each transaction contributes: id, organization_id, ledger_id, status, status_description, updated_at
	require.Equal(t, 12, len(mockDB.capturedArgs), "should have 12 parameters for 2 transactions (6 each)")

	// First transaction parameters (indices 0-5)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", mockDB.capturedArgs[0]) // id
	assert.NotEmpty(t, mockDB.capturedArgs[1])                                      // organization_id
	assert.NotEmpty(t, mockDB.capturedArgs[2])                                      // ledger_id
	assert.Equal(t, "APPROVED", mockDB.capturedArgs[3])                             // status
	assert.NotNil(t, mockDB.capturedArgs[4])                                        // status_description pointer
	// mockDB.capturedArgs[5] is updated_at timestamp

	// Second transaction parameters (indices 6-11)
	assert.Equal(t, "22222222-2222-2222-2222-222222222222", mockDB.capturedArgs[6]) // id
	assert.NotEmpty(t, mockDB.capturedArgs[7])                                      // organization_id
	assert.NotEmpty(t, mockDB.capturedArgs[8])                                      // ledger_id
	assert.Equal(t, "CANCELED", mockDB.capturedArgs[9])                             // status
	assert.NotNil(t, mockDB.capturedArgs[10])                                       // status_description pointer
	// mockDB.capturedArgs[11] is updated_at timestamp
}

func TestUpdateBulk_BatchedQuery_MultipleChunks(t *testing.T) {
	t.Parallel()

	// Use a sequence mock to handle multiple chunks
	// With chunk size of 500 and 1001 transactions, we get 3 chunks: 500 + 500 + 1
	sequenceMock := &bulkMockDBSequence{
		resultsPerCall: []bulkMockCallResult{
			{rowsAffected: 500}, // First chunk
			{rowsAffected: 500}, // Second chunk
			{rowsAffected: 1},   // Third chunk (1 transaction)
		},
	}
	ctx := tmcore.ContextWithPG(context.Background(), sequenceMock)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create 1001 transactions to trigger 3 chunks (500 + 500 + 1)
	transactions := generateTestTransactions(1001)

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1001), result.Attempted)
	assert.Equal(t, int64(1001), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)

	// Verify 3 ExecContext calls (one per chunk)
	assert.Equal(t, 3, sequenceMock.callCount, "should make 3 ExecContext calls for 3 chunks")
}

func TestUpdateBulk_BatchedQuery_EmptyChunk(t *testing.T) {
	t.Parallel()

	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 0},
	}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Empty input should not call ExecContext
	result, err := repo.UpdateBulk(ctx, []*Transaction{})

	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, 0, mockDB.callCount, "should not call ExecContext for empty input")
}

func TestUpdateBulk_BatchedQuery_ParameterLimitCalculation(t *testing.T) {
	t.Parallel()

	// Verify that 500 rows * 6 columns stays under PostgreSQL's 65,535 limit
	// Columns: id, organization_id, ledger_id, status, status_description, updated_at
	const chunkSize = 500
	const columnCount = 6
	const postgresLimit = 65535

	parametersPerChunk := chunkSize * columnCount
	assert.Less(t, parametersPerChunk, postgresLimit,
		"parameters per chunk (%d) should be less than PostgreSQL limit (%d)",
		parametersPerChunk, postgresLimit)
}

func TestUpdateBulk_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	// This test verifies that the bulk update WHERE clause correctly includes
	// organization_id and ledger_id, preventing cross-tenant updates.
	// When the org/ledger IDs in the VALUES don't match the database record,
	// the update should affect 0 rows (no security breach).

	// Mock returns 0 rows affected - simulating org/ledger mismatch
	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 0}, // No rows updated due to WHERE mismatch
	}
	ctx := tmcore.ContextWithPG(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create transaction with specific org/ledger
	tx := generateTestTransaction("")
	tx.OrganizationID = "org-attempt-cross-tenant"
	tx.LedgerID = "ledger-attempt-cross-tenant"
	tx.Status = Status{Code: "APPROVED", Description: ptr("Attempted cross-tenant update")}

	transactions := []*Transaction{tx}

	result, err := repo.UpdateBulk(ctx, transactions)

	// Should NOT return error (query executed successfully, just matched 0 rows)
	require.NoError(t, err)

	// CRITICAL: Verify result shows 0 updates (cross-tenant blocked by WHERE clause)
	assert.Equal(t, int64(1), result.Attempted, "Should attempt 1 transaction")
	assert.Equal(t, int64(0), result.Updated, "Should update 0 rows due to org/ledger mismatch in WHERE")
	assert.Equal(t, int64(1), result.Unchanged, "Transaction should be marked as unchanged")

	// Verify the query contains the tenant isolation conditions
	assert.Contains(t, mockDB.capturedQuery, "t.organization_id = v.organization_id",
		"Query MUST include organization_id in WHERE clause for tenant isolation")
	assert.Contains(t, mockDB.capturedQuery, "t.ledger_id = v.ledger_id",
		"Query MUST include ledger_id in WHERE clause for tenant isolation")

	// Verify org/ledger IDs are in the parameters
	// Parameters order: id, organization_id, ledger_id, status, status_description, updated_at
	require.GreaterOrEqual(t, len(mockDB.capturedArgs), 6, "Should have at least 6 parameters")
	assert.Equal(t, "org-attempt-cross-tenant", mockDB.capturedArgs[1],
		"organization_id should be in parameters")
	assert.Equal(t, "ledger-attempt-cross-tenant", mockDB.capturedArgs[2],
		"ledger_id should be in parameters")
}
