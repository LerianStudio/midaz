//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountregistration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// businessErrorCode extracts the numeric error code from a business error returned by
// pkg.ValidateBusinessError. The underlying error types expose a Code field but not all
// of them echo it in Error(), so we use type assertion to assert on the sentinel-derived
// code in a uniform way.
func businessErrorCode(t *testing.T, err error) string {
	t.Helper()

	var notFound pkg.EntityNotFoundError
	if errors.As(err, &notFound) {
		return notFound.Code
	}

	var conflict pkg.EntityConflictError
	if errors.As(err, &conflict) {
		return conflict.Code
	}

	var unproc pkg.UnprocessableOperationError
	if errors.As(err, &unproc) {
		return unproc.Code
	}

	var validation pkg.ValidationError
	if errors.As(err, &validation) {
		return validation.Code
	}

	t.Fatalf("error is not a known business error type: %T (%v)", err, err)

	return ""
}

// createRepository wires a Postgres-backed repository against the test container. The
// migrations path points at the onboarding migrations directory (where 000011 lives).
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *AccountRegistrationPostgreSQLRepository {
	t.Helper()

	migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, container.Config.DBName, migrationsPath)

	return NewAccountRegistrationPostgreSQLRepository(conn)
}

// newRegistration returns a fresh, consistent AccountRegistration instance with the given
// idempotency key and request hash. Caller may override any field afterwards.
func newRegistration(idempotencyKey, requestHash string) *mmodel.AccountRegistration {
	now := time.Now().UTC().Truncate(time.Microsecond)

	return &mmodel.AccountRegistration{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()),
		OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()),
		LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()),
		HolderID:       uuid.Must(libCommons.GenerateUUIDv7()),
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Status:         mmodel.AccountRegistrationReceived,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// ============================================================================
// UpsertByIdempotencyKey
// ============================================================================

func TestIntegration_AccountRegistration_Upsert_CreatesNewRow(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	reg := newRegistration("idem-new-"+uuid.NewString(), "hash-abc")

	ctx := context.Background()

	stored, created, err := repo.UpsertByIdempotencyKey(ctx, reg)
	require.NoError(t, err, "first upsert must succeed")
	require.True(t, created, "first upsert must report wasCreated=true")
	require.NotNil(t, stored)

	assert.Equal(t, reg.ID, stored.ID)
	assert.Equal(t, reg.RequestHash, stored.RequestHash)
	assert.Equal(t, mmodel.AccountRegistrationReceived, stored.Status)
}

func TestIntegration_AccountRegistration_Upsert_ReplaysSameHash(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	idempotencyKey := "idem-replay-" + uuid.NewString()

	first := newRegistration(idempotencyKey, "hash-same")

	ctx := context.Background()

	original, created, err := repo.UpsertByIdempotencyKey(ctx, first)
	require.NoError(t, err)
	require.True(t, created, "initial insert must create the row")

	// Submit a second registration with a fresh ID but the same hash — replay scenario.
	replay := newRegistration(idempotencyKey, "hash-same")
	replay.OrganizationID = first.OrganizationID
	replay.LedgerID = first.LedgerID
	replay.HolderID = first.HolderID

	got, created, err := repo.UpsertByIdempotencyKey(ctx, replay)
	require.NoError(t, err, "replay with same hash must succeed")
	assert.False(t, created, "replay must report wasCreated=false")
	assert.Equal(t, original.ID, got.ID, "replay must return the original row's ID, not the new attempted ID")
	assert.Equal(t, original.RequestHash, got.RequestHash)
}

func TestIntegration_AccountRegistration_Upsert_RejectsConflictOnDifferentHash(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	idempotencyKey := "idem-conflict-" + uuid.NewString()

	first := newRegistration(idempotencyKey, "hash-first")

	ctx := context.Background()

	_, created, err := repo.UpsertByIdempotencyKey(ctx, first)
	require.NoError(t, err)
	require.True(t, created)

	conflict := newRegistration(idempotencyKey, "hash-different")
	conflict.OrganizationID = first.OrganizationID
	conflict.LedgerID = first.LedgerID

	_, created, err = repo.UpsertByIdempotencyKey(ctx, conflict)
	require.Error(t, err, "different hash on the same idempotency key must fail")
	assert.False(t, created)

	// ValidateBusinessError surfaces a typed business error carrying the sentinel code
	// in its Code field. Not all business-error types echo the code in Error(), so we
	// inspect the Code field directly.
	assert.Equal(t, constant.ErrAccountRegistrationIdempotencyConflict.Error(), businessErrorCode(t, err),
		"conflict error must carry code 0168")
}

// ============================================================================
// Status transitions
// ============================================================================

func TestIntegration_AccountRegistration_StatusTransitions(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	reg := newRegistration("idem-transitions-"+uuid.NewString(), "hash-transitions")

	ctx := context.Background()

	_, _, err := repo.UpsertByIdempotencyKey(ctx, reg)
	require.NoError(t, err)

	// RECEIVED -> HOLDER_VALIDATED
	require.NoError(t, repo.UpdateStatus(ctx, reg.ID, mmodel.AccountRegistrationHolderValidated))

	got, err := repo.FindByID(ctx, reg.OrganizationID, reg.LedgerID, reg.ID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.AccountRegistrationHolderValidated, got.Status)

	// Attach account then advance
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	require.NoError(t, repo.AttachAccount(ctx, reg.ID, accountID))
	require.NoError(t, repo.UpdateStatus(ctx, reg.ID, mmodel.AccountRegistrationLedgerAccountCreated))

	got, err = repo.FindByID(ctx, reg.OrganizationID, reg.LedgerID, reg.ID)
	require.NoError(t, err)
	require.NotNil(t, got.AccountID)
	assert.Equal(t, accountID, *got.AccountID)
	assert.Equal(t, mmodel.AccountRegistrationLedgerAccountCreated, got.Status)

	// Attach alias then advance
	aliasID := uuid.Must(libCommons.GenerateUUIDv7())

	require.NoError(t, repo.AttachCRMAlias(ctx, reg.ID, aliasID))
	require.NoError(t, repo.UpdateStatus(ctx, reg.ID, mmodel.AccountRegistrationCRMAliasCreated))
	require.NoError(t, repo.UpdateStatus(ctx, reg.ID, mmodel.AccountRegistrationAccountActivated))

	got, err = repo.FindByID(ctx, reg.OrganizationID, reg.LedgerID, reg.ID)
	require.NoError(t, err)
	require.NotNil(t, got.CRMAliasID)
	assert.Equal(t, aliasID, *got.CRMAliasID)

	// Mark completed — sets completed_at
	completedAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, repo.MarkCompleted(ctx, reg.ID, completedAt))

	got, err = repo.FindByID(ctx, reg.OrganizationID, reg.LedgerID, reg.ID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.AccountRegistrationCompleted, got.Status)
	require.NotNil(t, got.CompletedAt)
	assert.WithinDuration(t, completedAt, *got.CompletedAt, time.Second)
}

func TestIntegration_AccountRegistration_MarkFailed_SetsCodeAndMessage(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	reg := newRegistration("idem-fail-"+uuid.NewString(), "hash-fail")

	ctx := context.Background()

	_, _, err := repo.UpsertByIdempotencyKey(ctx, reg)
	require.NoError(t, err)

	require.NoError(t, repo.MarkFailed(ctx, reg.ID, mmodel.AccountRegistrationFailedTerminal, "0165", "Holder not found"))

	got, err := repo.FindByID(ctx, reg.OrganizationID, reg.LedgerID, reg.ID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.AccountRegistrationFailedTerminal, got.Status)
	require.NotNil(t, got.FailureCode)
	assert.Equal(t, "0165", *got.FailureCode)
	require.NotNil(t, got.FailureMessage)
	assert.Equal(t, "Holder not found", *got.FailureMessage)
}

// ============================================================================
// FindByID
// ============================================================================

func TestIntegration_AccountRegistration_FindByID_ReturnsNotFound(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	ctx := context.Background()

	_, err := repo.FindByID(ctx,
		uuid.Must(libCommons.GenerateUUIDv7()),
		uuid.Must(libCommons.GenerateUUIDv7()),
		uuid.Must(libCommons.GenerateUUIDv7()),
	)
	require.Error(t, err)
	assert.Equal(t, constant.ErrAccountRegistrationNotFound.Error(), businessErrorCode(t, err),
		"absent registration must surface code 0167")
}

// ============================================================================
// Concurrent claim via FOR UPDATE SKIP LOCKED
//
// The repository exposes saga-transition methods rather than a Claim() method
// (the recovery worker and its claim semantics are Phase 5). This test verifies
// that FOR UPDATE SKIP LOCKED against the account_registration table behaves as
// expected at the SQL level — two concurrent transactions attempting to lock the
// same RECEIVED row must produce exactly one winner per SKIP LOCKED iteration.
// ============================================================================

func TestIntegration_AccountRegistration_ConcurrentClaim_WithForUpdateSkipLocked(t *testing.T) {
	container := pgtestutil.SetupContainer(t)

	repo := createRepository(t, container)

	ctx := context.Background()

	// Seed a batch of RECEIVED rows with distinct idempotency keys.
	const numRows = 5

	var ids []uuid.UUID
	for i := 0; i < numRows; i++ {
		reg := newRegistration("idem-claim-"+uuid.NewString(), "hash-claim")

		_, _, err := repo.UpsertByIdempotencyKey(ctx, reg)
		require.NoError(t, err)

		ids = append(ids, reg.ID)
	}

	db := container.DB

	// Run two concurrent "claimers" that each try to lock RECEIVED rows using
	// FOR UPDATE SKIP LOCKED. The union of their claimed IDs must equal numRows
	// and the intersection must be empty.
	var wg sync.WaitGroup

	claimAttempt := func(workerID string) []uuid.UUID {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err, "%s: begin tx", workerID)

		// Rollback at the end — we do not mutate state, we only verify locking.
		defer func() { _ = tx.Rollback() }()

		query, args, err := squirrel.Select("id").
			From(tableName).
			Where(squirrel.Eq{"status": string(mmodel.AccountRegistrationReceived)}).
			OrderBy("created_at").
			Limit(uint64(numRows)).
			Suffix("FOR UPDATE SKIP LOCKED").
			PlaceholderFormat(squirrel.Dollar).
			ToSql()
		require.NoError(t, err, "%s: build query", workerID)

		rows, err := tx.QueryContext(ctx, query, args...)
		require.NoError(t, err, "%s: query", workerID)
		defer rows.Close()

		var claimed []uuid.UUID

		for rows.Next() {
			var id uuid.UUID

			require.NoError(t, rows.Scan(&id), "%s: scan", workerID)

			claimed = append(claimed, id)
		}
		require.NoError(t, rows.Err(), "%s: rows err", workerID)

		// Hold the lock long enough for the other worker to run.
		time.Sleep(200 * time.Millisecond)

		return claimed
	}

	resultsCh := make(chan []uuid.UUID, 2)

	wg.Add(2)

	// Worker A starts first, acquires some rows, sleeps. Worker B runs 50ms later
	// so that SKIP LOCKED forces it to step around A's locks.
	go func() {
		defer wg.Done()

		resultsCh <- claimAttempt("A")
	}()

	time.Sleep(50 * time.Millisecond)

	go func() {
		defer wg.Done()

		resultsCh <- claimAttempt("B")
	}()

	wg.Wait()
	close(resultsCh)

	var all []uuid.UUID

	seen := make(map[uuid.UUID]int)

	for claimed := range resultsCh {
		for _, id := range claimed {
			seen[id]++

			all = append(all, id)
		}
	}

	// SKIP LOCKED guarantee: no row appears in both workers' result sets.
	for id, count := range seen {
		assert.Equalf(t, 1, count, "row %s must be claimed by exactly one worker (got %d)", id, count)
	}

	// The union of claims must exactly equal the seeded set.
	require.Len(t, all, numRows, "the two workers combined must observe every seeded row once")
}

// sanityCheckSentinelPresence guards against accidental removal of the
// ErrAccountRegistrationIdempotencyConflict and ErrAccountRegistrationNotFound
// sentinels — those are load-bearing for the error assertions in this file.
func TestIntegration_AccountRegistration_SentinelSanity(t *testing.T) {
	require.True(t, errors.Is(constant.ErrAccountRegistrationIdempotencyConflict, constant.ErrAccountRegistrationIdempotencyConflict))
	require.True(t, errors.Is(constant.ErrAccountRegistrationNotFound, constant.ErrAccountRegistrationNotFound))
	assert.Equal(t, "0168", constant.ErrAccountRegistrationIdempotencyConflict.Error())
	assert.Equal(t, "0167", constant.ErrAccountRegistrationNotFound.Error())
}
