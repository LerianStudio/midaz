// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package revertclaim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func setupRevertClaimRepository(t *testing.T) (*PostgreSQLRepository, *postgrestestutil.ContainerResult) {
	t.Helper()
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	return setupRevertClaimRepositoryWithContainer(t, postgrestestutil.SetupMigratedContainer(t, "transaction"))
}

func setupExclusiveRevertClaimRepository(t *testing.T) (*PostgreSQLRepository, *postgrestestutil.ContainerResult) {
	t.Helper()
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	return setupRevertClaimRepositoryWithContainer(t, postgrestestutil.SetupContainer(t))
}

func setupRevertClaimRepositoryWithContainer(
	t *testing.T,
	container *postgrestestutil.ContainerResult,
) (*PostgreSQLRepository, *postgrestestutil.ContainerResult) {
	t.Helper()

	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	dsn := postgrestestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	client := postgrestestutil.CreatePostgresClient(t, dsn, dsn, container.Config.DBName, migrationsPath)

	return NewPostgreSQLRepository(client), container
}

func TestIntegration_RevertClaim_ConcurrentOriginHasSingleReservedReverse(t *testing.T) {
	repo, _ := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()

	const racers = 24
	claims := make([]*Claim, racers)
	acquired := make([]bool, racers)
	errs := make([]error, racers)
	legacyKeys := make([]string, racers)
	rolloutMode := "bridge"
	rolloutToken := "origin-generation-token"
	redisGeneration := "financial-dataset-generation"
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			<-start
			legacyKeys[slot] = "legacy-fence-" + uuid.NewString()
			reverseID := uuid.New()
			legacyOwner := reverseID.String()
			claims[slot], acquired[slot], errs[slot] = repo.Claim(ctx, organizationID, ledgerID, originID,
				reverseID, &legacyKeys[slot], &legacyOwner, &rolloutMode, &rolloutToken, &redisGeneration)
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	reservedID := uuid.Nil
	winnerLegacyKey := ""
	for i := range racers {
		require.NoError(t, errs[i])
		require.NotNil(t, claims[i])
		if acquired[i] {
			winners++
			reservedID = claims[i].ReverseTransactionID
			winnerLegacyKey = legacyKeys[i]
		}
	}
	require.Equal(t, 1, winners)
	for _, claim := range claims {
		assert.Equal(t, reservedID, claim.ReverseTransactionID)
		require.NotNil(t, claim.LegacyFenceKey)
		assert.Equal(t, winnerLegacyKey, *claim.LegacyFenceKey,
			"every loser must recover the immutable fence chosen by the winning claim")
		require.NotNil(t, claim.LegacyFenceOwner)
		assert.Equal(t, reservedID.String(), *claim.LegacyFenceOwner,
			"every loser must recover the immutable owner chosen by the winning claim")
		require.NotNil(t, claim.RolloutMode)
		assert.Equal(t, rolloutMode, *claim.RolloutMode)
		require.NotNil(t, claim.RolloutToken)
		assert.Equal(t, rolloutToken, *claim.RolloutToken,
			"every loser must recover the exact rollout generation chosen by the winning claim")
		require.NotNil(t, claim.RedisGeneration)
		assert.Equal(t, redisGeneration, *claim.RedisGeneration)
	}
}

func TestIntegration_RevertClaim_ReverseIDIsUniqueAndReleaseIsPreMutationOnly(t *testing.T) {
	repo, _ := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	reverseID := uuid.New()

	first, acquired, err := repo.Claim(ctx, organizationID, ledgerID, uuid.New(), reverseID,
		nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)
	byReverseID, err := repo.GetByReverseID(ctx, organizationID, ledgerID, reverseID)
	require.NoError(t, err)
	require.NotNil(t, byReverseID)
	assert.Equal(t, first.OriginTransactionID, byReverseID.OriginTransactionID)
	missing, err := repo.GetByReverseID(ctx, organizationID, ledgerID, uuid.New())
	require.NoError(t, err)
	assert.Nil(t, missing)

	_, _, err = repo.Claim(ctx, organizationID, ledgerID, uuid.New(), reverseID,
		nil, nil, nil, nil, nil)
	require.Error(t, err, "one reverse ID cannot be reserved for two origins")

	require.NoError(t, repo.Arm(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID,
		first.ReverseTransactionID, first.ReverseTransactionID.String()))
	require.NoError(t, repo.Transition(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID,
		first.ReverseTransactionID, StateMutated, nil))
	released, err := repo.Release(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID, first.ReverseTransactionID)
	require.NoError(t, err)
	assert.False(t, released, "a post-mutation claim must never be released")

	require.NoError(t, repo.Transition(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID,
		first.ReverseTransactionID, StateCompleted, nil))
	reason := "legacy_revert_fence_completion_failed"
	require.NoError(t, repo.Transition(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID,
		first.ReverseTransactionID, StateReconciliationRequired, &reason))
	completed, err := repo.Get(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID)
	require.NoError(t, err)
	require.NotNil(t, completed)
	assert.Equal(t, StateCompleted, completed.State,
		"a concurrent retry must never downgrade an already-completed money-path claim")
	assert.Nil(t, completed.FailureReason)
	released, err = repo.Release(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID, first.ReverseTransactionID)
	require.NoError(t, err)
	assert.False(t, released, "reconciliation may never reopen the money path")
}

func TestIntegration_RevertClaim_RolloutGenerationIsAllOrNothing(t *testing.T) {
	repo, container := setupRevertClaimRepository(t)
	ctx := context.Background()

	mode := "bridge"
	_, _, err := repo.Claim(ctx, uuid.New(), uuid.New(), uuid.New(), uuid.New(),
		nil, nil, &mode, nil, nil)
	require.ErrorContains(t, err, "must be provided together")

	_, err = container.DB.ExecContext(ctx, `
		INSERT INTO transaction_revert_claim (
			organization_id, ledger_id, origin_transaction_id, reverse_transaction_id, rollout_mode
		) VALUES ($1, $2, $3, $4, 'bridge')`, uuid.New(), uuid.New(), uuid.New(), uuid.New())
	require.Error(t, err, "the database must reject a rollout generation without its exact token")

	redisGeneration := "financial-dataset-generation"
	claim, acquired, err := repo.Claim(ctx, uuid.New(), uuid.New(), uuid.New(), uuid.New(),
		nil, nil, nil, nil, &redisGeneration)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, claim.RedisGeneration)
	assert.Equal(t, redisGeneration, *claim.RedisGeneration,
		"final mode must durably bind a claim to the financial dataset without a legacy rollout token")
}

func TestIntegration_RevertRolloutInitializationBirthCertificateIsOneShot(t *testing.T) {
	repo, container := setupRevertClaimRepository(t)
	ctx := context.Background()
	generation := uuid.New()
	requestID := uuid.New()
	exists, storedGeneration, state, err := repo.InspectRolloutInitialization(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, uuid.Nil, storedGeneration)
	assert.Empty(t, state)

	prepared, created, err := repo.BeginRolloutInitialization(ctx, generation, requestID)
	require.NoError(t, err)
	require.True(t, created)
	assert.False(t, prepared)
	state = ""
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT state FROM transaction_revert_rollout_initialization WHERE singleton = TRUE`).Scan(&state))
	assert.Equal(t, "PREPARING", state)
	exists, storedGeneration, state, err = repo.InspectRolloutInitialization(ctx)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, generation, storedGeneration)
	assert.Equal(t, "PREPARING", state)

	prepared, created, err = repo.BeginRolloutInitialization(ctx, generation, requestID)
	require.NoError(t, err)
	assert.False(t, created, "retry of the exact initialization request must reuse its birth certificate")
	assert.False(t, prepared)

	_, _, err = repo.BeginRolloutInitialization(ctx, generation, uuid.New())
	require.ErrorContains(t, err, "initialization request differs")
	_, _, err = repo.BeginRolloutInitialization(ctx, uuid.New(), requestID)
	require.ErrorContains(t, err, "dataset generation differs")

	require.NoError(t, repo.CompleteRolloutInitialization(ctx, generation, requestID))
	require.NoError(t, repo.CompleteRolloutInitialization(ctx, generation, requestID),
		"lost completion response retry must be idempotent")
	require.NoError(t, repo.ValidatePreparedRollout(ctx, generation))
	exists, storedGeneration, state, err = repo.InspectRolloutInitialization(ctx)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, generation, storedGeneration)
	assert.Equal(t, "PREPARED", state)

	prepared, created, err = repo.BeginRolloutInitialization(ctx, generation, requestID)
	require.NoError(t, err)
	assert.False(t, created)
	assert.True(t, prepared)
}

func TestIntegration_RevertRolloutInitializationMigration38MissingTableFailsLoudly(t *testing.T) {
	repo, container := setupExclusiveRevertClaimRepository(t)
	ctx := context.Background()
	_, err := container.DB.ExecContext(ctx, `UPDATE schema_migrations SET version = 38, dirty = FALSE`)
	require.NoError(t, err,
		"simulate migration metadata published at 38 even though the test harness also knows later migrations")
	var migrationVersion int
	var dirty bool
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT version, dirty FROM schema_migrations`).Scan(&migrationVersion, &dirty))
	assert.Equal(t, 38, migrationVersion)
	assert.False(t, dirty)
	_, err = container.DB.ExecContext(ctx, `DROP TABLE transaction_revert_rollout_initialization`)
	require.NoError(t, err)

	_, _, err = repo.BeginRolloutInitialization(ctx, uuid.New(), uuid.New())
	require.ErrorContains(t, err, "transaction_revert_rollout_initialization",
		"migration metadata at 38 must never make a missing control table look initialized")
}

func TestIntegration_RevertRolloutInitializationConcurrentRequestsChooseOneIdentity(t *testing.T) {
	repo, _ := setupRevertClaimRepository(t)
	ctx := context.Background()
	generation := uuid.New()
	requestIDs := []uuid.UUID{uuid.New(), uuid.New()}

	type result struct {
		prepared bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, len(requestIDs))
	for _, requestID := range requestIDs {
		requestID := requestID
		go func() {
			<-start
			prepared, _, err := repo.BeginRolloutInitialization(ctx, generation, requestID)
			results <- result{prepared: prepared, err: err}
		}()
	}
	close(start)

	var successes int
	var conflicts int
	for range requestIDs {
		result := <-results
		if result.err != nil {
			require.ErrorContains(t, result.err, "initialization request differs")
			conflicts++
			continue
		}
		assert.False(t, result.prepared)
		successes++
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
}

func TestIntegration_RevertRolloutInitializationReconcilesLostPostgresCommitResponse(t *testing.T) {
	repo, _ := setupRevertClaimRepository(t)
	ctx := context.Background()
	generation := uuid.New()
	requestID := uuid.New()
	lostResponse := true
	repo.commitRolloutInitialization = func(tx dbresolver.Tx) error {
		err := tx.Commit()
		if err != nil {
			return err
		}
		if lostResponse {
			lostResponse = false

			return errors.New("lost PostgreSQL commit response")
		}

		return nil
	}

	prepared, created, err := repo.BeginRolloutInitialization(ctx, generation, requestID)
	require.NoError(t, err, "exact primary reread must prove an ambiguously committed birth certificate")
	require.True(t, created)
	assert.False(t, prepared)

	lostResponse = true
	require.NoError(t, repo.CompleteRolloutInitialization(ctx, generation, requestID),
		"exact primary reread must prove an ambiguously committed PREPARED promotion")
	require.NoError(t, repo.ValidatePreparedRollout(ctx, generation))
}

func TestIntegration_RevertClaim_UpgradePublished000036Through000039(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	container := postgrestestutil.SetupContainer(t)
	ctx := context.Background()
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	oldUp, err := os.ReadFile(filepath.Join(migrationsPath, "000036_create_revert_claim.up.sql"))
	require.NoError(t, err)
	rolloutUp, err := os.ReadFile(filepath.Join(migrationsPath, "000037_add_revert_rollout_generation.up.sql"))
	require.NoError(t, err)
	rolloutDown, err := os.ReadFile(filepath.Join(migrationsPath, "000037_add_revert_rollout_generation.down.sql"))
	require.NoError(t, err)
	initializationUp, err := os.ReadFile(filepath.Join(migrationsPath, "000038_create_revert_rollout_initialization.up.sql"))
	require.NoError(t, err)
	initializationDown, err := os.ReadFile(filepath.Join(migrationsPath, "000038_create_revert_rollout_initialization.down.sql"))
	require.NoError(t, err)
	armUp, err := os.ReadFile(filepath.Join(migrationsPath, "000039_arm_revert_claim.up.sql"))
	require.NoError(t, err)
	armDown, err := os.ReadFile(filepath.Join(migrationsPath, "000039_arm_revert_claim.down.sql"))
	require.NoError(t, err)

	_, err = container.DB.ExecContext(ctx, string(oldUp))
	require.NoError(t, err)
	var rolloutColumns int
	require.NoError(t, container.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'transaction_revert_claim'
		  AND column_name IN ('rollout_mode', 'rollout_token', 'redis_generation')`).Scan(&rolloutColumns))
	assert.Zero(t, rolloutColumns, "the published 000036 contract must remain unchanged")

	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	_, err = container.DB.ExecContext(ctx, `
		INSERT INTO transaction_revert_claim (
			organization_id, ledger_id, origin_transaction_id, reverse_transaction_id
		) VALUES ($1, $2, $3, $4)`, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)

	_, err = container.DB.ExecContext(ctx, string(rolloutUp))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(rolloutUp))
	require.NoError(t, err, "000037 up must be idempotent after a partially or fully applied deployment")
	require.NoError(t, container.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'transaction_revert_claim'
		  AND column_name IN ('rollout_mode', 'rollout_token', 'redis_generation')`).Scan(&rolloutColumns))
	assert.Equal(t, 3, rolloutColumns)
	var rolloutInitializationTable bool
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT to_regclass('public.transaction_revert_rollout_initialization') IS NOT NULL`).Scan(&rolloutInitializationTable))
	assert.False(t, rolloutInitializationTable,
		"the published 000037 migration must remain byte-for-byte unchanged")

	_, err = container.DB.ExecContext(ctx, string(initializationUp))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(initializationUp))
	require.NoError(t, err, "000038 up must validate an existing complete table idempotently")
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT to_regclass('public.transaction_revert_rollout_initialization') IS NOT NULL`).Scan(&rolloutInitializationTable))
	assert.True(t, rolloutInitializationTable, "000038 must add the durable deployment-scoped dataset birth certificate")
	_, err = container.DB.ExecContext(ctx,
		`ALTER TABLE transaction_revert_claim DROP CONSTRAINT transaction_revert_claim_state_check`)
	require.NoError(t, err, "simulate a partially applied deployment with the state constraint missing")
	_, err = container.DB.ExecContext(ctx, string(armUp))
	require.NoError(t, err, "000039 must restore a missing state constraint rather than silently doing nothing")
	_, err = container.DB.ExecContext(ctx, string(armUp))
	require.NoError(t, err, "000039 up must not arm claims created after its first application")

	var mode, token, generation *string
	require.NoError(t, container.DB.QueryRowContext(ctx, `
		SELECT rollout_mode, rollout_token, redis_generation
		FROM transaction_revert_claim
		WHERE organization_id = $1 AND ledger_id = $2 AND origin_transaction_id = $3`,
		organizationID, ledgerID, originID).Scan(&mode, &token, &generation))
	assert.Nil(t, mode)
	assert.Nil(t, token)
	assert.Nil(t, generation, "a legacy 000036 claim must survive the upgrade without fabricated Redis proof")
	var upgradedState State
	require.NoError(t, container.DB.QueryRowContext(ctx, `
		SELECT state
		FROM transaction_revert_claim
		WHERE organization_id = $1 AND ledger_id = $2 AND origin_transaction_id = $3`,
		organizationID, ledgerID, originID).Scan(&upgradedState))
	assert.Equal(t, StateArmed, upgradedState,
		"a pre-phase claim cannot be proven pre-movement and must upgrade conservatively")

	newOriginID := uuid.New()
	_, err = container.DB.ExecContext(ctx, `
		INSERT INTO transaction_revert_claim (
			organization_id, ledger_id, origin_transaction_id, reverse_transaction_id
		) VALUES ($1, $2, $3, $4)`, organizationID, ledgerID, newOriginID, uuid.New())
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(armUp))
	require.NoError(t, err)
	var newState State
	require.NoError(t, container.DB.QueryRowContext(ctx, `
		SELECT state FROM transaction_revert_claim
		WHERE organization_id = $1 AND ledger_id = $2 AND origin_transaction_id = $3`,
		organizationID, ledgerID, newOriginID).Scan(&newState))
	assert.Equal(t, StateClaimed, newState, "an idempotent migration retry must not arm a new request")

	_, err = container.DB.ExecContext(ctx, string(armDown))
	require.ErrorContains(t, err, "rollback requires an empty claim table")
	_, err = container.DB.ExecContext(ctx, string(rolloutDown))
	require.ErrorContains(t, err, "rollback requires an empty claim table")
	_, err = container.DB.ExecContext(ctx, `
		INSERT INTO transaction_revert_rollout_initialization (
			singleton, redis_generation, initialization_request_id, state
		) VALUES (TRUE, $1, $2, 'PREPARED')`, uuid.New(), uuid.New())
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(initializationDown))
	require.ErrorContains(t, err, "rollback requires an uninitialized rollout")
	_, err = container.DB.ExecContext(ctx, `DELETE FROM transaction_revert_rollout_initialization`)
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(initializationDown))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(rolloutDown))
	require.ErrorContains(t, err, "rollback requires an empty claim table")
	_, err = container.DB.ExecContext(ctx, `DELETE FROM transaction_revert_claim`)
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(armDown))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(rolloutDown))
	require.NoError(t, err)
	require.NoError(t, container.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'transaction_revert_claim'
		  AND column_name IN ('rollout_mode', 'rollout_token', 'redis_generation')`).Scan(&rolloutColumns))
	assert.Zero(t, rolloutColumns)
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT to_regclass('public.transaction_revert_rollout_initialization') IS NOT NULL`).Scan(&rolloutInitializationTable))
	assert.False(t, rolloutInitializationTable)
}

func TestIntegration_RevertClaim_ArmIsMonotonicAndLostCommitResponseUsesPrimaryProof(t *testing.T) {
	repo, _ := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()

	_, acquired, err := repo.Claim(ctx, organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Error(t, repo.Arm(ctx, organizationID, ledgerID, originID, reverseID, uuid.NewString()),
		"only the exact reserved reverse may own the balance attempt")

	repo.commitClaimArm = func(tx dbresolver.Tx) error {
		require.NoError(t, tx.Commit())

		return errors.New("lost arm commit response")
	}
	require.NoError(t, repo.Arm(ctx, organizationID, ledgerID, originID, reverseID, reverseID.String()),
		"the primary exact reread must reconcile a lost commit response")
	repo.commitClaimArm = nil

	claim, err := repo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, StateArmed, claim.State)

	recoveryOwner, err := repo.BeginPreMutationRecovery(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.False(t, recoveryOwner, "an armed claim can never return to automatic pre-movement recovery")
	released, err := repo.Release(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.False(t, released, "an armed claim can never be deleted by the pre-movement release path")
	released, err = repo.ReleaseRejectedArm(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.True(t, released, "only the explicit definitively-rejected path may remove an armed claim")
}

func TestIntegration_RevertClaim_ConcurrentArmAndRecoveryChooseOnePhase(t *testing.T) {
	repo, _ := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	_, acquired, err := repo.Claim(ctx, organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)

	start := make(chan struct{})
	armResult := make(chan error, 1)
	recoveryResult := make(chan struct {
		owner bool
		err   error
	}, 1)
	go func() {
		<-start
		armResult <- repo.Arm(ctx, organizationID, ledgerID, originID, reverseID, reverseID.String())
	}()
	go func() {
		<-start
		owner, recoverErr := repo.BeginPreMutationRecovery(ctx, organizationID, ledgerID, originID, reverseID)
		recoveryResult <- struct {
			owner bool
			err   error
		}{owner: owner, err: recoverErr}
	}()
	close(start)
	armErr := <-armResult
	recovered := <-recoveryResult
	require.NoError(t, recovered.err)

	claim, err := repo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	if armErr == nil {
		assert.False(t, recovered.owner)
		assert.Equal(t, StateArmed, claim.State)
	} else {
		assert.True(t, recovered.owner)
		assert.Equal(t, StateRecovering, claim.State)
	}
}

func TestIntegration_RevertClaim_PreMutationRecoveryElectsOneOwner(t *testing.T) {
	repo, container := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()

	_, acquired, err := repo.Claim(ctx, organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)

	first, err := repo.BeginPreMutationRecovery(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.True(t, first)
	second, err := repo.BeginPreMutationRecovery(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.False(t, second, "only one process may clean Redis fences")

	_, err = container.DB.ExecContext(ctx, `
		UPDATE transaction_revert_claim
		SET updated_at = NOW() - INTERVAL '31 seconds'
		WHERE organization_id = $1 AND ledger_id = $2 AND origin_transaction_id = $3`,
		organizationID, ledgerID, originID)
	require.NoError(t, err)
	resumed, err := repo.BeginPreMutationRecovery(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.True(t, resumed, "a crashed RECOVERING owner must be re-electable after its cleanup lease expires")

	claim, err := repo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, StateRecovering, claim.State)

	released, err := repo.Release(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.True(t, released, "the elected recovery owner releases PostgreSQL last")
}

func TestIntegration_RevertClaim_ReadsIgnoreReplicaLag(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	ctx := context.Background()
	primary := postgrestestutil.SetupContainer(t)
	replica := postgrestestutil.SetupContainer(t)
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	primaryDSN := postgrestestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config)
	replicaDSN := postgrestestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config)
	migrateRevertClaimSchema(t, primaryDSN, primary.Config.DBName, migrationsPath)
	migrateRevertClaimSchema(t, replicaDSN, replica.Config.DBName, migrationsPath)

	client, err := libPostgres.New(libPostgres.Config{PrimaryDSN: primaryDSN, ReplicaDSN: replicaDSN})
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	repo := NewPostgreSQLRepository(client)
	tenantClient, err := libPostgres.New(libPostgres.Config{PrimaryDSN: replicaDSN, ReplicaDSN: replicaDSN})
	require.NoError(t, err)
	require.NoError(t, tenantClient.Connect(ctx))
	t.Cleanup(func() { require.NoError(t, tenantClient.Close()) })
	tenantDB, err := tenantClient.Resolver(ctx)
	require.NoError(t, err)
	tenantConnectionCtx := tmcore.ContextWithPG(ctx, tenantDB)
	tenantBContext := tmcore.ContextWithTenantID(tenantConnectionCtx, "rollout-tenant-b")
	tenantAContext := tmcore.ContextWithTenantID(tenantConnectionCtx, "rollout-tenant-a")
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	_, acquired, err := repo.Claim(ctx, organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, repo.Arm(ctx, organizationID, ledgerID, originID, reverseID, reverseID.String()),
		"the balance arm must use the primary even while the replica is empty")

	byOrigin, err := repo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, byOrigin, "claim replay must see primary-only state")
	assert.Equal(t, reverseID, byOrigin.ReverseTransactionID)
	assert.Equal(t, StateArmed, byOrigin.State)
	byReverse, err := repo.GetByReverseID(ctx, organizationID, ledgerID, reverseID)
	require.NoError(t, err)
	require.NotNil(t, byReverse, "backup recovery must see primary-only state")
	assert.Equal(t, originID, byReverse.OriginTransactionID)
	rolloutGeneration := uuid.New()
	initializationRequestID := uuid.New()
	_, created, err := repo.BeginRolloutInitialization(tenantBContext, rolloutGeneration, initializationRequestID)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, repo.CompleteRolloutInitialization(tenantAContext, rolloutGeneration, initializationRequestID))
	require.NoError(t, repo.ValidatePreparedRollout(tenantBContext, rolloutGeneration),
		"rollout admission must see the primary birth certificate while the replica remains empty")
	exists, inspectedGeneration, inspectedState, err := repo.InspectRolloutInitialization(tenantBContext)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, rolloutGeneration, inspectedGeneration)
	assert.Equal(t, "PREPARED", inspectedState,
		"target-empty admission must inspect the deployment primary, never the empty tenant replica")
	require.NoError(t, tenantClient.Close(), "simulated tenant removal must close only the tenant-local database")
	require.NoError(t, repo.ValidatePreparedRollout(tenantAContext, rolloutGeneration),
		"removing a tenant must not remove or disconnect the deployment-scoped birth certificate")

	var replicaClaims int
	require.NoError(t, replica.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM transaction_revert_claim`).Scan(&replicaClaims))
	assert.Zero(t, replicaClaims, "the deterministic replica remains delayed throughout the proof")
	var replicaRolloutInitializations int
	require.NoError(t, replica.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transaction_revert_rollout_initialization`).Scan(&replicaRolloutInitializations))
	assert.Zero(t, replicaRolloutInitializations)
}

func migrateRevertClaimSchema(t *testing.T, dsn, databaseName, migrationsPath string) {
	t.Helper()
	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN: dsn, DatabaseName: databaseName, MigrationsPath: migrationsPath,
	})
	require.NoError(t, err)
	require.NoError(t, migrator.Up(context.Background()))
}

func TestIntegration_RevertClaim_MigrationDownAndUp(t *testing.T) {
	repo, container := setupExclusiveRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()

	_, acquired, err := repo.Claim(ctx, organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)

	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	armDown, err := os.ReadFile(filepath.Join(migrationsPath, "000039_arm_revert_claim.down.sql"))
	require.NoError(t, err)
	initializationDown, err := os.ReadFile(filepath.Join(migrationsPath, "000038_create_revert_rollout_initialization.down.sql"))
	require.NoError(t, err)
	rolloutDown, err := os.ReadFile(filepath.Join(migrationsPath, "000037_add_revert_rollout_generation.down.sql"))
	require.NoError(t, err)
	tableDown, err := os.ReadFile(filepath.Join(migrationsPath, "000036_create_revert_claim.down.sql"))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(armDown))
	require.ErrorContains(t, err, "rollback requires an empty claim table")
	_, err = container.DB.ExecContext(ctx, string(initializationDown))
	require.NoError(t, err, "empty pre-initialization 000038 must be safely removable before 000037")
	_, err = container.DB.ExecContext(ctx, string(rolloutDown))
	require.ErrorContains(t, err, "rollback requires an empty claim table")
	_, err = container.DB.ExecContext(ctx, string(tableDown))
	require.ErrorContains(t, err, "cannot remove transaction_revert_claim while reversal claims exist")

	var exists bool
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT to_regclass('public.transaction_revert_claim') IS NOT NULL`).Scan(&exists))
	assert.True(t, exists, "failed rollback must preserve the money-path fence")

	released, err := repo.Release(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	require.True(t, released)

	_, err = container.DB.ExecContext(ctx, string(armDown))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(rolloutDown))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(tableDown))
	require.NoError(t, err)
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT to_regclass('public.transaction_revert_claim') IS NOT NULL`).Scan(&exists))
	assert.False(t, exists)

	up, err := os.ReadFile(filepath.Join(migrationsPath, "000036_create_revert_claim.up.sql"))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(up))
	require.NoError(t, err)
	rolloutUp, err := os.ReadFile(filepath.Join(migrationsPath, "000037_add_revert_rollout_generation.up.sql"))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(rolloutUp))
	require.NoError(t, err)
	initializationUp, err := os.ReadFile(filepath.Join(migrationsPath, "000038_create_revert_rollout_initialization.up.sql"))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(initializationUp))
	require.NoError(t, err)
	armUp, err := os.ReadFile(filepath.Join(migrationsPath, "000039_arm_revert_claim.up.sql"))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(armUp))
	require.NoError(t, err)

	_, acquired, err = repo.Claim(ctx, uuid.New(), uuid.New(), uuid.New(), uuid.New(),
		nil, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.True(t, acquired)
}

func TestIntegration_RevertClaim_MigrationDownCannotRaceConcurrentClaim(t *testing.T) {
	_, container := setupExclusiveRevertClaimRepository(t)
	ctx := context.Background()
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	down, err := os.ReadFile(filepath.Join(migrationsPath, "000037_add_revert_rollout_generation.down.sql"))
	require.NoError(t, err)

	tx, err := container.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO transaction_revert_claim (
			organization_id, ledger_id, origin_transaction_id, reverse_transaction_id
		) VALUES ($1, $2, $3, $4)`, uuid.New(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)

	downResult := make(chan error, 1)
	go func() {
		_, execErr := container.DB.ExecContext(ctx, string(down))
		downResult <- execErr
	}()

	select {
	case earlyErr := <-downResult:
		require.Failf(t, "down migration did not wait for concurrent claim", "returned early: %v", earlyErr)
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, tx.Commit())
	downErr := <-downResult
	require.ErrorContains(t, downErr, "rollback requires an empty claim table")

	var exists bool
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT to_regclass('public.transaction_revert_claim') IS NOT NULL`).Scan(&exists))
	assert.True(t, exists, "the linearized down guard must preserve a concurrently-created claim")
	var generationColumn bool
	require.NoError(t, container.DB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'transaction_revert_claim'
			  AND column_name = 'redis_generation'
		)`).Scan(&generationColumn))
	assert.True(t, generationColumn, "failed 000037 rollback must preserve the witness contract")
}
