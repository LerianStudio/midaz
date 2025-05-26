package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
	
	_ "github.com/lib/pq"
)

// TestDB wraps a test database connection
type TestDB struct {
	DB       *sql.DB
	Cleanup  func()
	TestName string
}

// SetupTestDB creates a test database for integration tests
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()
	
	// Use test-specific database name
	dbName := fmt.Sprintf("test_%s_%d", t.Name(), time.Now().Unix())
	
	// Connect to postgres to create test database
	db, err := sql.Open("postgres", "postgres://postgres:postgres@localhost/postgres?sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to connect to postgres: %v", err)
	}
	
	// Create test database
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	db.Close()
	
	// Connect to test database
	testDB, err := sql.Open("postgres", fmt.Sprintf("postgres://postgres:postgres@localhost/%s?sslmode=disable", dbName))
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}
	
	return &TestDB{
		DB:       testDB,
		TestName: dbName,
		Cleanup: func() {
			testDB.Close()
			// Drop test database
			db, _ := sql.Open("postgres", "postgres://postgres:postgres@localhost/postgres?sslmode=disable")
			db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			db.Close()
		},
	}
}

// RunMigrations executes database migrations for tests
func (tdb *TestDB) RunMigrations(migrationPath string) error {
	// This would integrate with your migration tool
	// For now, we'll create basic test tables
	queries := []string{
		`CREATE TABLE IF NOT EXISTS transactions (
			id UUID PRIMARY KEY,
			description TEXT,
			amount BIGINT,
			currency VARCHAR(3),
			status VARCHAR(50),
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS operations (
			id UUID PRIMARY KEY,
			transaction_id UUID REFERENCES transactions(id),
			account_id UUID,
			type VARCHAR(50),
			amount BIGINT,
			created_at TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX idx_operations_transaction_id ON operations(transaction_id)`,
		`CREATE INDEX idx_transactions_status ON transactions(status)`,
		`CREATE INDEX idx_transactions_created_at ON transactions(created_at)`,
	}
	
	for _, query := range queries {
		if _, err := tdb.DB.Exec(query); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	
	return nil
}

// TruncateTables clears all data from test tables
func (tdb *TestDB) TruncateTables(tables ...string) error {
	for _, table := range tables {
		if _, err := tdb.DB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)); err != nil {
			return fmt.Errorf("failed to truncate %s: %w", table, err)
		}
	}
	return nil
}

// AssertRowCount verifies the number of rows in a table
func (tdb *TestDB) AssertRowCount(t *testing.T, table string, expected int) {
	t.Helper()
	
	var count int
	err := tdb.DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows: %v", err)
	}
	
	if count != expected {
		t.Errorf("Expected %d rows in %s, got %d", expected, table, count)
	}
}

// WithTransaction runs a function within a database transaction
func (tdb *TestDB) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := tdb.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	
	return tx.Commit()
}