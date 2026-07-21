//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transactionquarantine

// =============================================================================
// INTEGRATION TESTS — Quarantine Insert against a real PostgreSQL container.
//
// These tests exist to lock the column-type invariant that the unit suite
// cannot reach: the payload column MUST store raw, opaque bytes so the ONE
// poison record that matters — the "unmarshal_failure" record, whose payload
// is non-JSON by definition — can actually be quarantined. With a JSONB column
// Postgres rejected those bytes (SQLSTATE 22P02) and the record retried in
// Redis forever; with BYTEA the Insert succeeds and the financial copy lands.
//
// Run with:
//
//	go test -tags integration -run TestIntegration_Quarantine -v -count=1 \
//	    ./components/ledger/internal/adapters/postgres/transactionquarantine/
//
// =============================================================================

import (
	"context"
	"database/sql"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupQuarantineRepo starts a PostgreSQL container, applies the transaction
// migrations (which include 000034), and returns the repository plus the raw
// *sql.DB used to read rows back.
func setupQuarantineRepo(t *testing.T) (*QuarantinePostgreSQLRepository, *sql.DB) {
	t.Helper()

	pgContainer := pgtestutil.SetupContainer(t)

	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)

	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, pgContainer.Config.DBName, migrationsPath)

	return NewQuarantinePostgreSQLRepository(conn), pgContainer.DB
}

// TestIntegration_Quarantine_Insert_NonJSONPayload is the regression test for
// the data-loss merge-blocker (Q1). It inserts a record whose Payload is NOT
// valid JSON and asserts the Insert SUCCEEDS and the bytes round-trip verbatim.
// On the old JSONB column this Insert failed with SQLSTATE 22P02, so the only
// poison record that is non-JSON by definition could never be quarantined.
func TestIntegration_Quarantine_Insert_NonJSONPayload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repo, db := setupQuarantineRepo(t)
	ctx := context.Background()

	// Deliberately non-JSON bytes: a NUL byte, a control byte, then a dangling
	// opening brace. JSONB would reject this with invalid_text_representation.
	nonJSON := []byte("\x00\x01 not json {")

	record := &QuarantineRecord{
		OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()),
		LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()),
		TransactionID:  uuid.Must(libCommons.GenerateUUIDv7()),
		RedisKey:       "backup_queue:{transactions}:" + uuid.NewString(),
		Payload:        nonJSON,
		FailureReason:  "unmarshal_failure",
		Attempts:       3,
		FirstFailedAt:  time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
	}

	err := repo.Insert(ctx, record)
	require.NoError(t, err, "Insert of a non-JSON payload must succeed against the BYTEA column")

	// Round-trip: the stored bytes must equal what we wrote, verbatim.
	var got []byte
	queryErr := db.QueryRowContext(ctx,
		`SELECT payload FROM transaction_backup_quarantine WHERE redis_key = $1`,
		record.RedisKey,
	).Scan(&got)
	require.NoError(t, queryErr, "quarantined row must be readable")
	assert.Equal(t, nonJSON, got, "stored payload must round-trip the raw non-JSON bytes verbatim")
}

// TestIntegration_Quarantine_Insert_Idempotent verifies the ON CONFLICT
// (redis_key) DO NOTHING contract: re-inserting the same redis_key is a
// successful no-op so the caller may safely delete the Redis copy.
func TestIntegration_Quarantine_Insert_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repo, db := setupQuarantineRepo(t)
	ctx := context.Background()

	redisKey := "backup_queue:{transactions}:" + uuid.NewString()
	payload := []byte("\x00 still not json")

	newRecord := func() *QuarantineRecord {
		return &QuarantineRecord{
			OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()),
			LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()),
			TransactionID:  uuid.Must(libCommons.GenerateUUIDv7()),
			RedisKey:       redisKey,
			Payload:        payload,
			FailureReason:  "unmarshal_failure",
		}
	}

	require.NoError(t, repo.Insert(ctx, newRecord()), "first insert must succeed")
	require.NoError(t, repo.Insert(ctx, newRecord()), "conflicting re-insert must be a successful no-op")

	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transaction_backup_quarantine WHERE redis_key = $1`,
		redisKey,
	).Scan(&count))
	assert.Equal(t, 1, count, "record must land exactly once despite two inserts")
}
