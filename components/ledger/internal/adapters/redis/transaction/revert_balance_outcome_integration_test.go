//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

const redisIntegrationDatasetGeneration = "645439df-1837-421e-9607-f60b091542c9"
const redisIntegrationInitializationRequestID = "52c85247-b684-4ff7-a45e-41d8f437e4f1"

func completeRedisEconomicBalance() mmodel.BalanceRedis {
	return mmodel.BalanceRedis{
		ID: uuid.NewString(), Alias: "@economic-proof", Key: constant.DefaultBalanceKey,
		AccountID: uuid.NewString(), AssetCode: "USD", Available: decimal.NewFromInt(900),
		OnHold: decimal.Zero, Version: 2, AccountType: "deposit", AllowSending: 1,
		AllowReceiving: 1, Direction: constant.DirectionDebit, OverdraftUsed: "0",
		OverdraftLimit: "0", BalanceScope: mmodel.BalanceScopeTransactional,
	}
}

func completeRedisEconomicOperation(
	organizationID, ledgerID, transactionID uuid.UUID,
	operationID string,
) mmodel.OperationRedis {
	return mmodel.OperationRedis{
		ID: operationID, TransactionID: transactionID.String(), Type: constant.DEBIT,
		AssetCode: "USD", AmountValue: decimal.NewFromInt(100),
		BalanceAvailable: decimal.NewFromInt(1000), BalanceOnHold: decimal.Zero, BalanceVersion: 1,
		BalanceAfterAvailable: decimal.NewFromInt(900), BalanceAfterOnHold: decimal.Zero,
		BalanceAfterVersion: 2, BalanceID: uuid.NewString(), AccountID: uuid.NewString(),
		BalanceKey: constant.DefaultBalanceKey, OrganizationID: organizationID.String(),
		LedgerID: ledgerID.String(), BalanceAffected: true, Direction: constant.DirectionDebit,
		Snapshot: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
	}
}

func redisEconomicOperationFromMovement(
	organizationID, ledgerID, transactionID uuid.UUID,
	operationID string,
	before, after *mmodel.Balance,
	operationType, direction string,
) mmodel.OperationRedis {
	beforeRedis := before.ToRedis()
	afterRedis := after.ToRedis()
	amount := before.Available.Sub(after.Available).Abs()
	if onHold := before.OnHold.Sub(after.OnHold).Abs(); onHold.GreaterThan(amount) {
		amount = onHold
	}

	return mmodel.OperationRedis{
		ID: operationID, TransactionID: transactionID.String(), Type: operationType, Direction: direction,
		AssetCode: before.AssetCode, AmountValue: amount,
		BalanceAvailable: before.Available, BalanceOnHold: before.OnHold, BalanceVersion: before.Version,
		BalanceAfterAvailable: after.Available, BalanceAfterOnHold: after.OnHold, BalanceAfterVersion: after.Version,
		BalanceID: before.ID, BalanceKey: before.Key, AccountID: before.AccountID, AccountAlias: before.Alias,
		OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), BalanceAffected: true,
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: beforeRedis.OverdraftUsed,
			OverdraftUsedAfter:  afterRedis.OverdraftUsed,
		},
	}
}

func bindFinalEconomicPlan(
	t *testing.T,
	queue *mmodel.TransactionRedisQueue,
	operations []mmodel.BalanceOperation,
	transactionStatus string,
	pending bool,
) []mmodel.BalanceOperation {
	t.Helper()

	bound := append([]mmodel.BalanceOperation(nil), operations...)
	for index := range bound {
		if bound[index].EconomicSide == "" {
			bound[index].EconomicSide = mmodel.EconomicSideSource
		}
		if bound[index].EconomicRole == "" {
			bound[index].EconomicRole = mmodel.EconomicRolePrimary
		}
		if bound[index].Amount.Direction == "" {
			switch bound[index].Amount.Operation {
			case constant.CREDIT, constant.RELEASE:
				bound[index].Amount.Direction = constant.DirectionCredit
			default:
				bound[index].Amount.Direction = constant.DirectionDebit
			}
		}
	}
	plan, err := mmodel.BuildExpectedEconomicPlan(bound, transactionStatus, pending, queue.OperationTypeOverride)
	require.NoError(t, err)
	queue.ExpectedEconomicPlan = plan
	for index := range bound {
		bound[index].ExpectedEconomicPlan = plan
	}

	return bound
}

type rolloutInitializationWitnessStub struct {
	mu               sync.Mutex
	generation       string
	requestID        string
	prepared         bool
	completeFailures int
	inspectFailures  int
}

func (s *rolloutInitializationWitnessStub) BeginRolloutInitialization(
	_ context.Context,
	generation, requestID uuid.UUID,
) (bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation == "" {
		s.generation = generation.String()
		s.requestID = requestID.String()

		return false, true, nil
	}
	if s.generation != generation.String() {
		return false, false, fmt.Errorf("dataset generation differs")
	}
	if s.requestID != requestID.String() {
		return false, false, fmt.Errorf("initialization request differs")
	}

	return s.prepared, false, nil
}

func (s *rolloutInitializationWitnessStub) CompleteRolloutInitialization(
	_ context.Context,
	generation, requestID uuid.UUID,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation != generation.String() || s.requestID != requestID.String() {
		return fmt.Errorf("rollout initialization identity differs")
	}
	if s.completeFailures > 0 {
		s.completeFailures--

		return fmt.Errorf("lost PostgreSQL completion response")
	}
	s.prepared = true

	return nil
}

func (s *rolloutInitializationWitnessStub) ValidatePreparedRollout(
	_ context.Context,
	generation uuid.UUID,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.prepared || s.generation != generation.String() {
		return fmt.Errorf("prepared rollout birth certificate differs")
	}

	return nil
}

func (s *rolloutInitializationWitnessStub) InspectRolloutInitialization(
	_ context.Context,
) (bool, uuid.UUID, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inspectFailures > 0 {
		s.inspectFailures--

		return false, uuid.Nil, "", fmt.Errorf("deployment primary unavailable")
	}
	if s.generation == "" {
		return false, uuid.Nil, "", nil
	}
	generation, err := uuid.Parse(s.generation)
	if err != nil {
		return false, uuid.Nil, "", err
	}
	state := "PREPARING"
	if s.prepared {
		state = "PREPARED"
	}

	return true, generation, state, nil
}

func TestIntegration_BalanceExecutionOutcomeIsImmutableAndExactlyReplayable(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	executionKey := utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID)
	outcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	owner := uuid.NewString()
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: executionKey,
		OutcomeKey:   outcomeKey,
		Owner:        owner,
		Outcome:      mmodel.TransactionOutcomeCommitted,
		Identity:     transactionID,
	}
	acquired, err := infra.repo.AcquireOwnedKey(ctx, executionKey, owner, 300)
	require.NoError(t, err)
	require.True(t, acquired)
	wrongKeyAttempt := attempt
	wrongKeyAttempt.OutcomeKey = utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, uuid.New())
	_, err = infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.APPROVED, false, nil, wrongKeyAttempt)
	require.ErrorContains(t, err, "complete balance execution attempt is required",
		"an attempt cannot bind one transaction identity to another transaction's outcome key")
	operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@shared-outcome", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	queue := mmodel.TransactionRedisQueue{
		TransactionID:     transactionID,
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		AttemptOwner:      owner,
		ExpectedOutcome:   attempt.Outcome,
		TransactionStatus: constant.APPROVED,
	}
	operations = bindFinalEconomicPlan(t, &queue, operations, constant.APPROVED, false)
	seed, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, infra.repo.AddMessageToQueue(ctx,
		utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String()), seed))
	first, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.APPROVED, false, operations, attempt)
	require.NoError(t, err)
	require.Len(t, first.After, 1)
	require.True(t, decimal.NewFromInt(900).Equal(first.After[0].Available))

	rawOutcome, err := infra.repo.Get(ctx, outcomeKey)
	require.NoError(t, err)
	var persisted mmodel.BalanceExecutionOutcome
	require.NoError(t, json.Unmarshal([]byte(rawOutcome), &persisted))
	assert.Equal(t, transactionID, persisted.Identity)
	assert.Equal(t, owner, persisted.Owner)
	assert.Equal(t, mmodel.TransactionOutcomeCommitted, persisted.Outcome)

	replay := attempt
	replay.Owner = uuid.NewString()
	second, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.APPROVED, false, operations, replay)
	require.NoError(t, err, "same outcome must replay before validating a new attempt owner")
	require.Equal(t, first, second, "replay must return the exact original balance snapshots")

	opposite := replay
	opposite.Outcome = mmodel.TransactionOutcomeAborted
	_, err = infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CANCELED, false, operations, opposite)
	require.Error(t, err, "an opposite terminal outcome must conflict before movement")
	var oppositeConflict pkg.EntityConflictError
	require.ErrorAs(t, err, &oppositeConflict)
	assert.Equal(t, constant.ErrCommitTransactionNotPending.Error(), oppositeConflict.Code)

	balance, err := infra.repo.Get(ctx, operations[0].InternalKey)
	require.NoError(t, err)
	assert.Contains(t, balance, `"Available":"900"`, "same replay and opposite conflict must not move funds again")
}

func TestIntegration_BalanceOutcomeAtomicallyBindsFinalEconomicPlan(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	owner := uuid.NewString()
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
		OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
		Owner:        owner, Outcome: mmodel.TransactionOutcomeCommitted, Identity: transactionID,
	}
	require.NoError(t, func() error {
		acquired, err := infra.repo.AcquireOwnedKey(ctx, attempt.ExecutionKey, owner, 300)
		if err != nil || !acquired {
			return fmt.Errorf("acquire attempt: acquired=%t: %w", acquired, err)
		}
		return nil
	}())

	operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@plan-bound", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	operations[0].EconomicSide = mmodel.EconomicSideSource
	operations[0].Amount.Direction = constant.DirectionDebit
	operations[0].Balance.Direction = constant.DirectionCredit
	plan, err := mmodel.BuildExpectedEconomicPlan(operations, constant.CREATED, false, "")
	require.NoError(t, err)
	seed, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		TransactionStatus: constant.CREATED, AttemptOwner: owner, ExpectedOutcome: attempt.Outcome,
		EffectModeVersion: mmodel.TransactionEffectModeVersion, EffectMode: mmodel.TransactionEffectBalanceMutation,
		Action: constant.ActionDirect, ExpectedEconomicPlan: plan,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(100)}},
	})
	require.NoError(t, err)
	require.NoError(t, infra.repo.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID, seed, attempt))

	divergentOperations := append([]mmodel.BalanceOperation(nil), operations...)
	divergentOperations[0].Amount.Value = decimal.NewFromInt(101)
	divergentPlan, err := mmodel.BuildExpectedEconomicPlan(divergentOperations, constant.CREATED, false, "")
	require.NoError(t, err)
	for index := range divergentOperations {
		divergentOperations[index].ExpectedEconomicPlan = divergentPlan
	}
	_, err = infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CREATED, false, divergentOperations, attempt)
	require.ErrorContains(t, err, "EXPECTED_ECONOMIC_PLAN_MISMATCH")
	assert.Empty(t, mustGetRedisString(t, infra.repo, ctx, attempt.OutcomeKey))
	assert.Empty(t, mustGetRedisString(t, infra.repo, ctx, operations[0].InternalKey),
		"a plan mismatch must reject before even seeding a balance")

	for index := range operations {
		operations[index].ExpectedEconomicPlan = plan
	}
	result, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CREATED, false, operations, attempt)
	require.NoError(t, err)
	require.Len(t, result.After, 1)
	rawOutcome := mustGetRedisString(t, infra.repo, ctx, attempt.OutcomeKey)
	persisted := mmodel.BalanceExecutionOutcome{}
	require.NoError(t, json.Unmarshal([]byte(rawOutcome), &persisted))
	assert.Equal(t, fmt.Sprint(plan.Version), persisted.EconomicPlanVersion)
	assert.Equal(t, plan.Digest, persisted.EconomicPlanDigest)

	replay, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CREATED, false, operations, attempt)
	require.NoError(t, err)
	assert.Equal(t, result, replay)
}

func mustGetRedisString(t *testing.T, repo *RedisConsumerRepository, ctx context.Context, key string) string {
	t.Helper()
	value, err := repo.Get(ctx, key)
	require.NoError(t, err)

	return value
}

func TestIntegration_GenerationBoundBalanceOutcomeRemainsImmutableAndExactlyReplayable(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	redisGeneration := uuid.NewString()
	require.NoError(t, infra.redisContainer.Client.Set(ctx, FinancialDatasetGenerationKey, redisGeneration, 0).Err())
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey:    utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
		OutcomeKey:      utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
		Owner:           transactionID.String(),
		Outcome:         mmodel.TransactionOutcomeCommitted,
		Identity:        transactionID,
		RedisGeneration: redisGeneration,
	}
	acquired, err := infra.repo.AcquireOwnedKey(ctx, attempt.ExecutionKey, attempt.Owner, 300)
	require.NoError(t, err)
	require.True(t, acquired)
	operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@generation-bound", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	operations[0].Balance.Direction = constant.DirectionDebit
	queue := mmodel.TransactionRedisQueue{
		TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		TransactionStatus: constant.CREATED, AttemptOwner: attempt.Owner, ExpectedOutcome: attempt.Outcome,
		EffectModeVersion: mmodel.TransactionEffectModeVersion, EffectMode: mmodel.TransactionEffectBalanceMutation,
		RedisGeneration: redisGeneration, Action: constant.ActionCommit,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(100)}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"@generation-bound#default": {Asset: "USD", Value: decimal.NewFromInt(100),
				Operation: constant.DEBIT, Direction: constant.DirectionDebit},
		}},
	}
	operations = bindFinalEconomicPlan(t, &queue, operations, constant.CREATED, false)
	seed, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, infra.repo.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID, seed, attempt))

	first, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CREATED, false, operations, attempt)
	require.NoError(t, err)
	require.Len(t, first.After, 1)
	outcome, err := infra.repo.Get(ctx, attempt.OutcomeKey)
	require.NoError(t, err)
	require.NotEmpty(t, outcome, "the generation-bound Lua path must persist the authoritative outcome")
	execution, err := infra.repo.MGet(ctx, []string{attempt.ExecutionKey, attempt.ExecutionKey + ":owner"})
	require.NoError(t, err)
	assert.Empty(t, execution, "terminal outcome creation must consume the exact execution lease")

	replay := attempt
	replay.Owner = uuid.NewString()
	second, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CREATED, false, operations, replay)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	materialized := []mmodel.OperationRedis{redisEconomicOperationFromMovement(
		organizationID, ledgerID, transactionID, uuid.NewString(), first.Before[0], first.After[0],
		constant.DEBIT, constant.DirectionDebit)}
	economicCtx := mmodel.WithTransactionEconomicContext(ctx, mmodel.TransactionEconomicContext{
		TransactionStatus: constant.CREATED, Action: constant.ActionCommit,
		TransactionAmount: "100", TransactionAssetCode: "USD",
	})
	_, _, _, err = infra.repo.EnrichTransactionBackup(economicCtx, organizationID, ledgerID, transactionID,
		materialized, constant.ActionCommit, &attempt)
	require.NoError(t, err)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, FinancialDatasetGenerationKey, uuid.NewString(), 0).Err())
	_, _, _, err = infra.repo.EnrichTransactionBackup(economicCtx, organizationID, ledgerID, transactionID,
		materialized, constant.ActionCommit, &attempt)
	require.Error(t, err,
		"a delayed consumer must reject the old generation before adopting operations or writing PostgreSQL")
	require.NoError(t, infra.redisContainer.Client.Set(ctx, FinancialDatasetGenerationKey, redisGeneration, 0).Err())
	require.NoError(t, infra.repo.FinalizeTransactionPersistence(economicCtx, organizationID, ledgerID, transactionID,
		attempt, materialized, mmodel.BalancesToRedis(first.After)))
	_, tombstoneBalances, terminal, err := infra.repo.EnrichTransactionBackup(economicCtx, organizationID, ledgerID, transactionID,
		materialized, constant.ActionCommit, &attempt)
	require.NoError(t, err)
	assert.True(t, terminal)
	assert.True(t, mmodel.RedisBalanceSetEconomicEqual(mmodel.BalancesToRedis(first.After), tombstoneBalances))
	require.NoError(t, infra.redisContainer.Client.Set(ctx, FinancialDatasetGenerationKey, uuid.NewString(), 0).Err())
	_, _, _, err = infra.repo.EnrichTransactionBackup(economicCtx, organizationID, ledgerID, transactionID,
		materialized, constant.ActionCommit, &attempt)
	require.Error(t, err, "generation rollover must invalidate even an otherwise exact terminal receipt")
}

func TestIntegration_GlobalFinancialGenerationServesTwoTenantsAndSurvivesTenantRemoval(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	redisGeneration := uuid.NewString()
	require.NoError(t, infra.redisContainer.Client.Set(ctx,
		FinancialDatasetGenerationKey, redisGeneration, 0).Err())

	type tenantExecution struct {
		ctx          context.Context
		tenantID     string
		organization uuid.UUID
		ledger       uuid.UUID
		transaction  uuid.UUID
		attempt      mmodel.BalanceExecutionAttempt
		operation    mmodel.OperationRedis
	}
	execute := func(tenantID string) tenantExecution {
		tenantCtx := mmodel.WithTransactionEconomicContext(tmcore.ContextWithTenantID(ctx, tenantID),
			mmodel.TransactionEconomicContext{TransactionStatus: constant.CREATED, Action: constant.ActionCommit,
				TransactionAmount: "100", TransactionAssetCode: "USD"})
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()
		attempt := mmodel.BalanceExecutionAttempt{
			ExecutionKey:    utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
			OutcomeKey:      utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
			Owner:           uuid.NewString(),
			Outcome:         mmodel.TransactionOutcomeCommitted,
			Identity:        transactionID,
			RedisGeneration: redisGeneration,
		}
		acquired, err := infra.repo.AcquireOwnedKey(tenantCtx, attempt.ExecutionKey, attempt.Owner, 300)
		require.NoError(t, err)
		require.True(t, acquired)
		balanceOperations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
			organizationID, ledgerID, "@global-generation-"+tenantID, "USD", constant.DEBIT,
			decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
		)}
		balanceOperations[0].Balance.Direction = constant.DirectionDebit
		queue := mmodel.TransactionRedisQueue{
			TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
			TransactionStatus: constant.CREATED, AttemptOwner: attempt.Owner, ExpectedOutcome: attempt.Outcome,
			EffectModeVersion: mmodel.TransactionEffectModeVersion, EffectMode: mmodel.TransactionEffectBalanceMutation,
			RedisGeneration: redisGeneration, Action: constant.ActionCommit,
			TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
				Asset: "USD", Value: decimal.NewFromInt(100),
			}},
			Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
				"@global-generation-" + tenantID + "#default": {
					Asset: "USD", Value: decimal.NewFromInt(100),
					Operation: constant.DEBIT, Direction: constant.DirectionDebit,
				},
			}},
		}
		balanceOperations = bindFinalEconomicPlan(t, &queue, balanceOperations, constant.CREATED, false)
		seed, err := json.Marshal(queue)
		require.NoError(t, err)
		require.NoError(t, infra.repo.SeedTransactionBackup(tenantCtx,
			organizationID, ledgerID, transactionID, seed, attempt))
		result, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(tenantCtx,
			organizationID, ledgerID, transactionID, constant.CREATED, false, balanceOperations, attempt)
		require.NoError(t, err)
		operation := redisEconomicOperationFromMovement(organizationID, ledgerID, transactionID, uuid.NewString(),
			result.Before[0], result.After[0], constant.DEBIT, constant.DirectionDebit)
		canonical, _, terminal, err := infra.repo.EnrichTransactionBackup(tenantCtx,
			organizationID, ledgerID, transactionID, []mmodel.OperationRedis{operation}, constant.ActionCommit, &attempt)
		require.NoError(t, err)
		assert.False(t, terminal)
		require.Len(t, canonical, 1)
		assert.Equal(t, operation.ID, canonical[0].ID)
		assert.Equal(t, operation.TransactionID, canonical[0].TransactionID)
		assert.Zero(t, infra.redisContainer.Client.Exists(ctx,
			"tenant:"+tenantID+":"+FinancialDatasetGenerationKey).Val(),
			"a tenant money path must use the deployment-wide generation, never manufacture a tenant witness")

		return tenantExecution{
			ctx: tenantCtx, tenantID: tenantID, organization: organizationID, ledger: ledgerID,
			transaction: transactionID, attempt: attempt, operation: operation,
		}
	}

	// Tenant B deliberately starts before tenant A. Rollout identity must not
	// depend on which tenant reaches the money path first.
	tenantB := execute("generation-tenant-b-" + uuid.NewString())
	tenantA := execute("generation-tenant-a-" + uuid.NewString())

	var tenantAKeys []string
	iterator := infra.redisContainer.Client.Scan(ctx, 0, "tenant:"+tenantA.tenantID+":*", 0).Iterator()
	for iterator.Next(ctx) {
		tenantAKeys = append(tenantAKeys, iterator.Val())
	}
	require.NoError(t, iterator.Err())
	require.NotEmpty(t, tenantAKeys)
	require.NoError(t, infra.redisContainer.Client.Del(ctx, tenantAKeys...).Err())
	assert.Equal(t, redisGeneration,
		infra.redisContainer.Client.Get(ctx, FinancialDatasetGenerationKey).Val(),
		"removing a tenant must not remove the deployment rollout identity")

	canonical, _, terminal, err := infra.repo.EnrichTransactionBackup(tenantB.ctx,
		tenantB.organization, tenantB.ledger, tenantB.transaction,
		[]mmodel.OperationRedis{tenantB.operation}, constant.ActionCommit, &tenantB.attempt)
	require.NoError(t, err)
	assert.False(t, terminal)
	require.Len(t, canonical, 1)
	assert.Equal(t, tenantB.operation.ID, canonical[0].ID,
		"another tenant must keep exact replay after an unrelated tenant is removed")
	evidence, generationMatches, err := infra.repo.TransactionEconomicEvidenceExists(tenantB.ctx,
		tenantB.organization, tenantB.ledger, tenantB.transaction, redisGeneration)
	require.NoError(t, err)
	assert.True(t, evidence)
	assert.True(t, generationMatches)
}

func TestIntegration_TransactionBackupEnrichmentPreservesOutcomeUntilExactDurableCleanup(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
		OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
		Owner:        uuid.NewString(),
		Outcome:      mmodel.TransactionOutcomeCommitted,
		Identity:     transactionID,
	}
	acquired, err := infra.repo.AcquireOwnedKey(ctx, attempt.ExecutionKey, attempt.Owner, 300)
	require.NoError(t, err)
	require.True(t, acquired)
	balanceOperations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@backup-envelope", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	balanceOperations[0].Balance.Direction = constant.DirectionDebit
	queue := mmodel.TransactionRedisQueue{
		TransactionID:       transactionID,
		OrganizationID:      organizationID,
		LedgerID:            ledgerID,
		TransactionStatus:   constant.APPROVED,
		Action:              constant.ActionCommit,
		EffectModeVersion:   mmodel.TransactionEffectModeVersion,
		EffectMode:          mmodel.TransactionEffectBalanceMutation,
		AttemptOwner:        attempt.Owner,
		ExpectedOutcome:     attempt.Outcome,
		ParentTransactionID: nil,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "USD", Value: decimal.NewFromInt(100),
		}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"@backup-envelope#default": {Asset: "USD", Value: decimal.NewFromInt(100),
				Operation: constant.DEBIT, Direction: constant.DirectionDebit},
		}},
	}
	balanceOperations = bindFinalEconomicPlan(t, &queue, balanceOperations, constant.APPROVED, false)
	seed, err := json.Marshal(queue)
	require.NoError(t, err)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	require.NoError(t, infra.repo.AddMessageToQueue(ctx, transactionKey, seed))

	result, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.APPROVED, false, balanceOperations, attempt)
	require.NoError(t, err)
	require.Len(t, result.Before, 1)
	require.Len(t, result.After, 1)

	materialized := []mmodel.OperationRedis{redisEconomicOperationFromMovement(
		organizationID, ledgerID, transactionID, uuid.NewString(), result.Before[0], result.After[0],
		constant.DEBIT, constant.DirectionDebit)}
	validContext := mmodel.WithTransactionEconomicContext(ctx, mmodel.TransactionEconomicContext{
		TransactionStatus: constant.APPROVED, Action: constant.ActionCommit,
		TransactionAmount: "100", TransactionAssetCode: "USD",
	})
	unboundRaw, err := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Bytes()
	require.NoError(t, err)
	for name, rawOperations := range map[string]json.RawMessage{
		"null operation field":  json.RawMessage(`null`),
		"empty operation array": json.RawMessage(`[]`),
	} {
		t.Run(name+" is enriched exactly once", func(t *testing.T) {
			rawEnvelope := map[string]json.RawMessage{}
			require.NoError(t, json.Unmarshal(unboundRaw, &rawEnvelope))
			rawEnvelope["operations"] = rawOperations
			delete(rawEnvelope, "economic_effect_digest")
			candidateRaw, marshalErr := json.Marshal(rawEnvelope)
			require.NoError(t, marshalErr)
			require.NoError(t, infra.redisContainer.Client.HSet(ctx, TransactionBackupQueue, transactionKey, candidateRaw).Err())

			canonical, _, terminal, enrichErr := infra.repo.EnrichTransactionBackup(validContext,
				organizationID, ledgerID, transactionID, materialized, constant.ActionCommit, &attempt)
			require.NoError(t, enrichErr)
			assert.False(t, terminal)
			require.Equal(t, materialized, canonical)
			boundRaw, readErr := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Bytes()
			require.NoError(t, readErr)
			bound := mmodel.TransactionRedisQueue{}
			require.NoError(t, json.Unmarshal(boundRaw, &bound))
			require.Equal(t, materialized, bound.Operations)
			require.NotEmpty(t, bound.EconomicEffectDigest)
		})
	}
	require.NoError(t, infra.redisContainer.Client.HSet(ctx, TransactionBackupQueue, transactionKey, unboundRaw).Err())
	for name, mutate := range map[string]func(*mmodel.OperationRedis){
		"amount":    func(op *mmodel.OperationRedis) { op.AmountValue = decimal.RequireFromString("9007199254740993") },
		"direction": func(op *mmodel.OperationRedis) { op.Direction = constant.DirectionCredit },
		"balance":   func(op *mmodel.OperationRedis) { op.BalanceID = uuid.NewString() },
		"type":      func(op *mmodel.OperationRedis) { op.Type = constant.CREDIT },
	} {
		t.Run("rejects malicious first enrichment "+name, func(t *testing.T) {
			beforeRaw, readErr := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Bytes()
			require.NoError(t, readErr)
			beforeOutcome, readErr := infra.repo.Get(ctx, attempt.OutcomeKey)
			require.NoError(t, readErr)
			candidate := materialized[0]
			mutate(&candidate)
			_, _, _, enrichErr := infra.repo.EnrichTransactionBackup(validContext, organizationID, ledgerID, transactionID,
				[]mmodel.OperationRedis{candidate}, constant.ActionCommit, &attempt)
			require.Error(t, enrichErr)
			afterRaw, readErr := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Bytes()
			require.NoError(t, readErr)
			afterOutcome, readErr := infra.repo.Get(ctx, attempt.OutcomeKey)
			require.NoError(t, readErr)
			assert.Equal(t, beforeRaw, afterRaw, "rejected first enrichment must preserve the authoritative backup byte-for-byte")
			assert.Equal(t, beforeOutcome, afterOutcome, "rejected first enrichment must preserve the immutable outcome")
		})
	}
	t.Run("raw change between read and CAS cannot bind stale candidate", func(t *testing.T) {
		oldRaw, readErr := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Bytes()
		require.NoError(t, readErr)
		envelope := map[string]json.RawMessage{}
		require.NoError(t, json.Unmarshal(oldRaw, &envelope))
		envelope["header_id"] = json.RawMessage(`"concurrent-writer"`)
		changedRaw, marshalErr := json.Marshal(envelope)
		require.NoError(t, marshalErr)
		require.NoError(t, infra.redisContainer.Client.HSet(ctx, TransactionBackupQueue, transactionKey, changedRaw).Err())
		encodedOperations, marshalErr := json.Marshal(materialized)
		require.NoError(t, marshalErr)
		digest, digestErr := mmodel.RedisEconomicEffectDigest("100", "USD", materialized, mmodel.BalancesToRedis(result.After))
		require.NoError(t, digestErr)
		_, bindErr := bindTransactionEconomicDigestScript.Run(ctx, infra.redisContainer.Client,
			[]string{TransactionBackupQueue, transactionKey, attempt.OutcomeKey},
			string(oldRaw), string(encodedOperations), digest, transactionID.String(), "1",
			attempt.Owner, attempt.Outcome, constant.ActionCommit, "").Result()
		require.ErrorContains(t, bindErr, "TRANSACTION_BACKUP_CHANGED")
		retainedRaw, readErr := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Bytes()
		require.NoError(t, readErr)
		retained := mmodel.TransactionRedisQueue{}
		require.NoError(t, json.Unmarshal(retainedRaw, &retained))
		assert.Empty(t, retained.Operations)
		assert.Empty(t, retained.EconomicEffectDigest)
	})
	wrongParent := uuid.New()
	for name, proof := range map[string]mmodel.TransactionEconomicContext{
		"parent": {ParentTransactionID: &wrongParent, TransactionStatus: constant.APPROVED,
			TransactionAmount: "100", TransactionAssetCode: "USD"},
		"status": {TransactionStatus: constant.CANCELED,
			TransactionAmount: "100", TransactionAssetCode: "USD"},
		"transaction amount": {TransactionStatus: constant.APPROVED,
			TransactionAmount: "9007199254740993", TransactionAssetCode: "USD"},
		"transaction asset": {TransactionStatus: constant.APPROVED,
			TransactionAmount: "100", TransactionAssetCode: "EUR"},
	} {
		t.Run("rejects malicious transaction "+name, func(t *testing.T) {
			beforeRaw, readErr := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Bytes()
			require.NoError(t, readErr)
			beforeOutcome, readErr := infra.repo.Get(ctx, attempt.OutcomeKey)
			require.NoError(t, readErr)
			proof.Action = constant.ActionCommit
			wrongContext := mmodel.WithTransactionEconomicContext(ctx, proof)
			_, _, _, enrichErr := infra.repo.EnrichTransactionBackup(wrongContext, organizationID, ledgerID, transactionID,
				materialized, constant.ActionCommit, &attempt)
			require.Error(t, enrichErr, "candidate context cannot relabel the immutable envelope")
			afterRaw, readErr := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Bytes()
			require.NoError(t, readErr)
			afterOutcome, readErr := infra.repo.Get(ctx, attempt.OutcomeKey)
			require.NoError(t, readErr)
			assert.Equal(t, beforeRaw, afterRaw)
			assert.Equal(t, beforeOutcome, afterOutcome)
		})
	}
	canonical, canonicalBalances, terminal, err := infra.repo.EnrichTransactionBackup(validContext, organizationID, ledgerID, transactionID,
		materialized, constant.ActionCommit, &attempt)
	require.NoError(t, err)
	assert.False(t, terminal)
	require.Len(t, canonical, 1)
	require.Equal(t, materialized[0].ID, canonical[0].ID)
	require.NotEmpty(t, canonicalBalances)
	canonicalAfterLostBindResponse, balancesAfterLostBindResponse, terminal, err := infra.repo.EnrichTransactionBackup(
		validContext, organizationID, ledgerID, transactionID, materialized, constant.ActionCommit, &attempt,
	)
	require.NoError(t, err, "retrying the same digest bind after a lost response must be idempotent")
	assert.False(t, terminal)
	assert.Equal(t, canonical, canonicalAfterLostBindResponse)
	assert.True(t, mmodel.RedisBalanceSetEconomicEqual(canonicalBalances, balancesAfterLostBindResponse))
	backup, err := infra.repo.ReadMessageFromQueue(ctx, transactionKey)
	require.NoError(t, err)
	envelope := mmodel.TransactionRedisQueue{}
	require.NoError(t, json.Unmarshal(backup, &envelope))
	assert.Equal(t, attempt.Owner, envelope.AttemptOwner)
	assert.Equal(t, attempt.Outcome, envelope.ExpectedOutcome)
	require.NotEmpty(t, envelope.BalancesAfter, "CAS enrichment must preserve the Lua-authored after state")
	require.Len(t, envelope.Operations, 1)
	assert.Equal(t, materialized[0].ID, envelope.Operations[0].ID)
	require.NotEmpty(t, envelope.EconomicEffectDigest)

	foreign := attempt
	foreign.Owner = uuid.NewString()
	cancelContext := mmodel.WithTransactionEconomicContext(ctx, mmodel.TransactionEconomicContext{
		TransactionStatus: constant.APPROVED, Action: constant.ActionCancel,
		TransactionAmount: "100", TransactionAssetCode: "USD",
	})
	_, _, _, err = infra.repo.EnrichTransactionBackup(cancelContext, organizationID, ledgerID, transactionID,
		[]mmodel.OperationRedis{{ID: uuid.NewString()}}, constant.ActionCancel, &foreign)
	require.Error(t, err)
	require.Error(t, infra.repo.FinalizeTransactionPersistence(validContext, organizationID, ledgerID, transactionID, foreign,
		materialized, canonicalBalances))
	_, err = infra.repo.ReadMessageFromQueue(ctx, transactionKey)
	require.NoError(t, err, "a foreign cleanup must preserve the authoritative backup")
	outcome, err := infra.repo.Get(ctx, attempt.OutcomeKey)
	require.NoError(t, err)
	require.NotEmpty(t, outcome, "a foreign cleanup must preserve the immutable outcome")

	divergentOperation := materialized[0]
	divergentOperation.AmountValue = divergentOperation.AmountValue.Add(decimal.NewFromInt(1))
	require.Error(t, infra.repo.FinalizeTransactionPersistence(validContext, organizationID, ledgerID, transactionID, attempt,
		[]mmodel.OperationRedis{divergentOperation}, canonicalBalances),
		"cleanup must preserve an envelope whose economic operation body does not match PostgreSQL proof")
	divergentBalances := append([]mmodel.BalanceRedis(nil), canonicalBalances...)
	divergentBalances[0].Version++
	require.Error(t, infra.repo.FinalizeTransactionPersistence(validContext, organizationID, ledgerID, transactionID, attempt,
		materialized, divergentBalances),
		"cleanup must preserve an envelope whose complete balance body does not match PostgreSQL proof")
	_, err = infra.repo.ReadMessageFromQueue(ctx, transactionKey)
	require.NoError(t, err, "an economic body mismatch must preserve the authoritative backup")
	outcome, err = infra.repo.Get(ctx, attempt.OutcomeKey)
	require.NoError(t, err)
	require.NotEmpty(t, outcome, "an economic body mismatch must preserve the immutable outcome")
	for name, proof := range map[string]mmodel.TransactionEconomicContext{
		"parent": {ParentTransactionID: &wrongParent, TransactionStatus: constant.APPROVED, Action: constant.ActionCommit,
			TransactionAmount: "100", TransactionAssetCode: "USD"},
		"status": {TransactionStatus: constant.CANCELED, Action: constant.ActionCommit,
			TransactionAmount: "100", TransactionAssetCode: "USD"},
		"transaction amount": {TransactionStatus: constant.APPROVED, Action: constant.ActionCommit,
			TransactionAmount: "101", TransactionAssetCode: "USD"},
		"transaction asset": {TransactionStatus: constant.APPROVED, Action: constant.ActionCommit,
			TransactionAmount: "100", TransactionAssetCode: "EUR"},
	} {
		t.Run("terminal cleanup rejects malicious transaction "+name, func(t *testing.T) {
			wrongContext := mmodel.WithTransactionEconomicContext(ctx, proof)
			require.Error(t, infra.repo.FinalizeTransactionPersistence(wrongContext, organizationID, ledgerID,
				transactionID, attempt, materialized, canonicalBalances))
		})
	}
	require.NoError(t, infra.repo.FinalizeTransactionPersistence(validContext, organizationID, ledgerID, transactionID, attempt,
		materialized, canonicalBalances))
	_, err = infra.repo.ReadMessageFromQueue(ctx, transactionKey)
	require.ErrorIs(t, err, redis.Nil)
	outcome, err = infra.repo.Get(ctx, attempt.OutcomeKey)
	require.NoError(t, err)
	assert.Empty(t, outcome)
	tombstoneKey := utils.TransactionPersistenceTombstoneKey(organizationID, ledgerID, transactionID)
	tombstoneRaw, err := infra.repo.Get(ctx, tombstoneKey)
	require.NoError(t, err)
	require.NotEmpty(t, tombstoneRaw,
		"terminal cleanup must atomically replace live evidence with an append-only receipt")
	tombstone := mmodel.TransactionPersistenceTombstone{}
	require.NoError(t, json.Unmarshal([]byte(tombstoneRaw), &tombstone))
	assert.Equal(t, transactionID, tombstone.Identity)
	assert.Equal(t, attempt.Owner, tombstone.Owner)
	assert.Equal(t, attempt.Outcome, tombstone.Outcome)
	assert.Equal(t, constant.ActionCommit, tombstone.Action)
	assert.Equal(t, "100", tombstone.TransactionAmount)
	assert.Equal(t, "USD", tombstone.TransactionAssetCode)
	require.Equal(t, envelope.EconomicEffectDigest, tombstone.EconomicEffectDigest)
	require.Len(t, tombstone.Operations, 1)
	assert.Equal(t, materialized[0].ID, tombstone.Operations[0].ID)
	assert.Equal(t, materialized[0].TransactionID, tombstone.Operations[0].TransactionID)
	require.NotEmpty(t, tombstone.BalancesAfter)
	require.NoError(t, infra.repo.FinalizeTransactionPersistence(validContext, organizationID, ledgerID, transactionID, attempt,
		materialized, canonicalBalances),
		"a lost successful cleanup response must be exactly replayable")
	canonicalReplay, tombstoneBalances, terminal, err := infra.repo.EnrichTransactionBackup(validContext, organizationID, ledgerID, transactionID,
		materialized, constant.ActionCommit, &attempt)
	require.NoError(t, err)
	assert.True(t, terminal)
	require.Len(t, canonicalReplay, 1)
	assert.Equal(t, materialized[0].ID, canonicalReplay[0].ID)
	assert.Equal(t, materialized[0].TransactionID, canonicalReplay[0].TransactionID)
	assert.True(t, mmodel.RedisBalanceSetEconomicEqual(tombstone.BalancesAfter, tombstoneBalances))
	opposite := attempt
	opposite.Outcome = mmodel.TransactionOutcomeAborted
	_, _, _, err = infra.repo.EnrichTransactionBackup(validContext, organizationID, ledgerID, transactionID,
		materialized, constant.ActionCommit, &opposite)
	require.Error(t, err, "an opposite terminal outcome must conflict with the append-only receipt")
	require.NoError(t, infra.redisContainer.Client.Set(ctx, attempt.OutcomeKey, `{"identity":"foreign"}`, 0).Err())
	_, _, _, err = infra.repo.EnrichTransactionBackup(validContext, organizationID, ledgerID, transactionID,
		materialized, constant.ActionCommit, &attempt)
	require.Error(t, err, "partial Redis restoration must not coexist with a terminal receipt")
	require.NoError(t, infra.redisContainer.Client.Del(ctx, attempt.OutcomeKey, tombstoneKey).Err())
	_, _, _, err = infra.repo.EnrichTransactionBackup(validContext, organizationID, ledgerID, transactionID,
		materialized, constant.ActionCommit, &attempt)
	require.Error(t, err, "missing terminal receipt must never be inferred from absent backup and outcome")
}

func TestIntegration_TransactionBackupOperationIDsAreSingleAssignmentAcrossConsumers(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := mmodel.WithTransactionEconomicContext(context.Background(), mmodel.TransactionEconomicContext{
		TransactionStatus: constant.APPROVED, Action: constant.ActionCommit,
		TransactionAmount: "100", TransactionAssetCode: "USD",
	})
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
		OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
		Owner:        uuid.NewString(),
		Outcome:      mmodel.TransactionOutcomeCommitted,
		Identity:     transactionID,
	}
	acquired, err := infra.repo.AcquireOwnedKey(ctx, attempt.ExecutionKey, attempt.Owner, 300)
	require.NoError(t, err)
	require.True(t, acquired)
	balanceOperations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@single-assignment", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	balanceOperations[0].Balance.Direction = constant.DirectionDebit
	queue := mmodel.TransactionRedisQueue{
		TransactionID:         transactionID,
		OrganizationID:        organizationID,
		LedgerID:              ledgerID,
		TransactionStatus:     constant.APPROVED,
		Action:                constant.ActionCommit,
		EffectModeVersion:     mmodel.TransactionEffectModeVersion,
		EffectMode:            mmodel.TransactionEffectBalanceMutation,
		OperationTypeOverride: constant.BLOCK,
		AttemptOwner:          attempt.Owner,
		ExpectedOutcome:       attempt.Outcome,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "USD", Value: decimal.NewFromInt(100),
		}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"@single-assignment#default": {Asset: "USD", Value: decimal.NewFromInt(100),
				Operation: constant.DEBIT, Direction: constant.DirectionDebit},
		}},
	}
	balanceOperations = bindFinalEconomicPlan(t, &queue, balanceOperations, constant.APPROVED, false)
	seed, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, infra.repo.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID, seed, attempt))
	result, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.APPROVED, false, balanceOperations, attempt)
	require.NoError(t, err)
	require.Len(t, result.Before, 1)
	require.Len(t, result.After, 1)

	first := redisEconomicOperationFromMovement(organizationID, ledgerID, transactionID, uuid.NewString(),
		result.Before[0], result.After[0], constant.BLOCK, constant.DirectionDebit)
	second := first
	second.ID = uuid.NewString()
	candidates := [][]mmodel.OperationRedis{
		{first},
		{second},
	}
	results := make([][]mmodel.OperationRedis, len(candidates))
	errs := make([]error, len(candidates))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range candidates {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], _, _, errs[index] = infra.repo.EnrichTransactionBackup(ctx, organizationID, ledgerID,
				transactionID, candidates[index], constant.ActionCommit, &attempt)
		}(i)
	}
	close(start)
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, results[0], results[1], "both consumers must persist the Redis-selected operation IDs")
	require.Len(t, results[0], 1)

	// Simulate a restart after the winning CAS response was lost. A new set of
	// generated IDs must replay the authoritative selection, never replace it.
	restartedCandidate := candidates[0][0]
	restartedCandidate.ID = uuid.NewString()
	restarted, _, terminal, err := infra.repo.EnrichTransactionBackup(ctx, organizationID, ledgerID, transactionID,
		[]mmodel.OperationRedis{restartedCandidate},
		constant.ActionCommit, &attempt)
	require.NoError(t, err)
	assert.False(t, terminal)
	require.Equal(t, results[0], restarted)

	raw, err := infra.repo.ReadMessageFromQueue(ctx,
		utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String()))
	require.NoError(t, err)
	envelope := mmodel.TransactionRedisQueue{}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.Equal(t, results[0], envelope.Operations)
}

func TestIntegration_AnnotationBackupBindsOnlyProvenNonFinancialRows(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	baseCtx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	newFixture := func(transactionID uuid.UUID) (context.Context, mmodel.TransactionRedisQueue, mmodel.OperationRedis) {
		ctx := mmodel.WithTransactionEconomicContext(baseCtx, mmodel.TransactionEconomicContext{
			TransactionStatus:    constant.NOTED,
			Action:               constant.ActionDirect,
			TransactionAmount:    "1000",
			TransactionAssetCode: "USD",
		})
		queue := mmodel.TransactionRedisQueue{
			TransactionID:     transactionID,
			OrganizationID:    organizationID,
			LedgerID:          ledgerID,
			TransactionStatus: constant.NOTED,
			Action:            constant.ActionDirect,
			EffectModeVersion: mmodel.TransactionEffectModeVersion,
			EffectMode:        mmodel.TransactionEffectAnnotationOnly,
			TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
				Asset: "USD", Value: decimal.NewFromInt(1000),
			}},
			Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
				"0#@annotation#default": {
					Asset: "USD", Value: decimal.NewFromInt(1000), Operation: constant.DEBIT,
					Direction: constant.DirectionDebit, TransactionType: constant.NOTED,
				},
			}},
		}
		operation := mmodel.OperationRedis{
			ID: uuid.NewString(), TransactionID: transactionID.String(), Type: constant.DEBIT,
			AssetCode: "USD", AmountValue: decimal.Zero,
			BalanceAvailable: decimal.Zero, BalanceOnHold: decimal.Zero, BalanceVersion: 0,
			BalanceAfterAvailable: decimal.Zero, BalanceAfterOnHold: decimal.Zero, BalanceAfterVersion: 0,
			BalanceID: uuid.NewString(), AccountID: uuid.NewString(), AccountAlias: "@annotation",
			BalanceKey: constant.DefaultBalanceKey, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			BalanceAffected: false, Direction: constant.DirectionDebit,
			Snapshot: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
		}

		return ctx, queue, operation
	}
	seed := func(ctx context.Context, queue mmodel.TransactionRedisQueue) {
		raw, err := json.Marshal(queue)
		require.NoError(t, err)
		require.NoError(t, infra.repo.AddMessageToQueue(ctx,
			utils.TransactionInternalKey(queue.OrganizationID, queue.LedgerID, queue.TransactionID.String()), raw))
	}

	t.Run("lost response replays typed canonical row", func(t *testing.T) {
		transactionID := uuid.New()
		ctx, queue, operation := newFixture(transactionID)
		seed(ctx, queue)
		canonical, balances, terminal, err := infra.repo.EnrichTransactionBackup(ctx, organizationID, ledgerID,
			transactionID, []mmodel.OperationRedis{operation}, constant.ActionDirect, nil)
		require.NoError(t, err)
		assert.False(t, terminal)
		assert.Empty(t, balances)
		require.Len(t, canonical, 1)
		assert.Equal(t, operation.ID, canonical[0].ID)

		restarted := operation
		restarted.ID = uuid.NewString()
		replayed, balances, terminal, err := infra.repo.EnrichTransactionBackup(ctx, organizationID, ledgerID,
			transactionID, []mmodel.OperationRedis{restarted}, constant.ActionDirect, nil)
		require.NoError(t, err)
		assert.False(t, terminal)
		assert.Empty(t, balances)
		require.Equal(t, canonical, replayed, "a lost CAS response must retain the first annotation row identity")
	})

	t.Run("legacy NOTED envelope uses the explicit compatibility classifier", func(t *testing.T) {
		transactionID := uuid.New()
		ctx, queue, operation := newFixture(transactionID)
		queue.EffectModeVersion = 0
		queue.EffectMode = ""
		seed(ctx, queue)
		canonical, balances, terminal, err := infra.repo.EnrichTransactionBackup(ctx, organizationID, ledgerID,
			transactionID, []mmodel.OperationRedis{operation}, constant.ActionDirect, nil)
		require.NoError(t, err)
		assert.False(t, terminal)
		assert.Empty(t, balances)
		require.True(t, mmodel.RedisOperationSetEconomicEqualIgnoringIDs(
			[]mmodel.OperationRedis{operation}, canonical,
		))
		require.Equal(t, operation.ID, canonical[0].ID)
	})

	for name, mutate := range map[string]func(*mmodel.TransactionRedisQueue, *mmodel.OperationRedis){
		"balance evidence": func(queue *mmodel.TransactionRedisQueue, _ *mmodel.OperationRedis) {
			queue.Balances = []mmodel.BalanceRedis{completeRedisEconomicBalance()}
		},
		"attempt owner": func(queue *mmodel.TransactionRedisQueue, _ *mmodel.OperationRedis) {
			queue.AttemptOwner = uuid.NewString()
		},
		"terminal outcome": func(queue *mmodel.TransactionRedisQueue, _ *mmodel.OperationRedis) {
			queue.ExpectedOutcome = mmodel.TransactionOutcomeCommitted
		},
		"balance mutation": func(_ *mmodel.TransactionRedisQueue, operation *mmodel.OperationRedis) {
			operation.BalanceAffected = true
			operation.BalanceAfterAvailable = decimal.NewFromInt(1)
		},
		"nonzero operation amount": func(_ *mmodel.TransactionRedisQueue, operation *mmodel.OperationRedis) {
			operation.AmountValue = decimal.NewFromInt(1000)
		},
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			transactionID := uuid.New()
			ctx, queue, operation := newFixture(transactionID)
			mutate(&queue, &operation)
			seed(ctx, queue)
			_, _, _, err := infra.repo.EnrichTransactionBackup(ctx, organizationID, ledgerID,
				transactionID, []mmodel.OperationRedis{operation}, constant.ActionDirect, nil)
			require.Error(t, err)
			raw, readErr := infra.repo.ReadMessageFromQueue(ctx,
				utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String()))
			require.NoError(t, readErr, "reconciliation must retain a malicious annotation envelope")
			require.NotEmpty(t, raw)
		})
	}

	t.Run("rejects a surviving outcome key", func(t *testing.T) {
		transactionID := uuid.New()
		ctx, queue, operation := newFixture(transactionID)
		seed(ctx, queue)
		require.NoError(t, infra.repo.Set(ctx, utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
			`{"identity":"foreign"}`, 0))
		_, _, _, err := infra.repo.EnrichTransactionBackup(ctx, organizationID, ledgerID,
			transactionID, []mmodel.OperationRedis{operation}, constant.ActionDirect, nil)
		require.Error(t, err)
	})
}

func TestIntegration_TransactionBackupSeedAndCleanupAreOwnerFenced(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	newAttempt := func(owner string) mmodel.BalanceExecutionAttempt {
		return mmodel.BalanceExecutionAttempt{
			ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
			OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
			Owner:        owner,
			Outcome:      mmodel.TransactionOutcomeCommitted,
			Identity:     transactionID,
		}
	}
	seedFor := func(owner string) []byte {
		raw, marshalErr := json.Marshal(mmodel.TransactionRedisQueue{
			TransactionID:     transactionID,
			OrganizationID:    organizationID,
			LedgerID:          ledgerID,
			TransactionStatus: constant.PENDING,
			AttemptOwner:      owner,
			ExpectedOutcome:   mmodel.TransactionOutcomeCommitted,
		})
		require.NoError(t, marshalErr)
		return raw
	}

	attemptA := newAttempt("owner-a")
	acquired, err := infra.repo.AcquireOwnedKey(ctx, attemptA.ExecutionKey, attemptA.Owner, 300)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, infra.repo.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID,
		seedFor(attemptA.Owner), attemptA))
	removed, err := infra.repo.RemoveMessageFromQueueIfStatus(ctx, transactionKey, constant.PENDING,
		attemptA.Owner, attemptA.Outcome, true)
	require.NoError(t, err)
	require.True(t, removed)
	released, err := infra.repo.ReleaseOwnedKey(ctx, attemptA.ExecutionKey, attemptA.Owner)
	require.NoError(t, err)
	require.True(t, released)

	attemptB := newAttempt("owner-b")
	acquired, err = infra.repo.AcquireOwnedKey(ctx, attemptB.ExecutionKey, attemptB.Owner, 300)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, infra.repo.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID,
		seedFor(attemptB.Owner), attemptB))
	require.Error(t, infra.repo.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID,
		seedFor(attemptA.Owner), attemptA), "a delayed old request must not overwrite its successor's seed")

	raw, err := infra.repo.ReadMessageFromQueue(ctx, transactionKey)
	require.NoError(t, err)
	envelope := mmodel.TransactionRedisQueue{}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.Equal(t, attemptB.Owner, envelope.AttemptOwner)
}

func TestIntegration_PendingCleanupCannotDeleteConcurrentTerminalBackup(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	for i := 0; i < 25; i++ {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()
		transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
		pendingOwner := uuid.NewString()
		pending, err := json.Marshal(mmodel.TransactionRedisQueue{
			TransactionID:     transactionID,
			TransactionStatus: constant.PENDING,
			AttemptOwner:      pendingOwner,
			ExpectedOutcome:   mmodel.TransactionOutcomeAborted,
		})
		require.NoError(t, err)
		require.NoError(t, infra.repo.AddMessageToQueue(ctx, transactionKey, pending))
		terminal, err := json.Marshal(mmodel.TransactionRedisQueue{
			TransactionID:     transactionID,
			TransactionStatus: constant.PENDING,
			AttemptOwner:      pendingOwner,
			ExpectedOutcome:   mmodel.TransactionOutcomeAborted,
			BalancesAfter:     []mmodel.BalanceRedis{{ID: uuid.NewString()}},
		})
		require.NoError(t, err)

		start := make(chan struct{})
		var cleanupErr, terminalErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, cleanupErr = infra.repo.RemoveMessageFromQueueIfStatus(ctx, transactionKey, constant.PENDING,
				pendingOwner, mmodel.TransactionOutcomeAborted, true)
		}()
		go func() {
			defer wg.Done()
			<-start
			terminalErr = infra.redisContainer.Client.HSet(ctx, TransactionBackupQueue, transactionKey, terminal).Err()
		}()
		close(start)
		wg.Wait()
		require.NoError(t, cleanupErr)
		require.NoError(t, terminalErr)

		raw, err := infra.repo.ReadMessageFromQueue(ctx, transactionKey)
		require.NoError(t, err, "the terminal envelope must survive regardless of the Redis command order")
		actual := mmodel.TransactionRedisQueue{}
		require.NoError(t, json.Unmarshal(raw, &actual))
		require.Equal(t, constant.PENDING, actual.TransactionStatus)
		require.NotEmpty(t, actual.BalancesAfter, "pre-movement cleanup must never target a terminal Lua envelope")
	}
}

func TestIntegration_LegacyBackupCleanupRequiresExactDurableIdentity(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	parentID := uuid.New()
	operationIDs := []string{uuid.NewString()}
	completeOperations := []mmodel.OperationRedis{
		completeRedisEconomicOperation(organizationID, ledgerID, transactionID, operationIDs[0]),
	}
	completeAfter := completeRedisEconomicBalance()
	completeAfter.ID = completeOperations[0].BalanceID
	completeAfter.Key = completeOperations[0].BalanceKey
	completeAfter.AccountID = completeOperations[0].AccountID
	completeAfter.AssetCode = completeOperations[0].AssetCode
	completeAfter.Available = completeOperations[0].BalanceAfterAvailable
	completeAfter.OnHold = completeOperations[0].BalanceAfterOnHold
	completeAfter.Version = completeOperations[0].BalanceAfterVersion
	completeBefore := completeAfter
	completeBefore.Available = completeOperations[0].BalanceAvailable
	completeBefore.OnHold = completeOperations[0].BalanceOnHold
	completeBefore.Version = completeOperations[0].BalanceVersion
	completeQueue := mmodel.TransactionRedisQueue{
		TransactionID:       transactionID,
		ParentTransactionID: &parentID,
		OrganizationID:      organizationID,
		LedgerID:            ledgerID,
		TransactionStatus:   constant.CREATED,
		Action:              constant.ActionRevert,
		Balances:            []mmodel.BalanceRedis{completeBefore},
		BalancesAfter:       []mmodel.BalanceRedis{completeAfter},
		Operations:          completeOperations,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "USD", Value: decimal.NewFromInt(100),
		}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"@economic-proof#default": {Asset: "USD", Value: decimal.NewFromInt(100),
				Operation: constant.DEBIT, Direction: constant.DirectionDebit},
		}},
	}
	legacyProof := mmodel.TransactionEconomicContext{
		ParentTransactionID:  &parentID,
		TransactionStatus:    constant.CREATED,
		Action:               constant.ActionRevert,
		Operations:           completeOperations,
		BalancesAfter:        []mmodel.BalanceRedis{completeAfter},
		TransactionAmount:    "100",
		TransactionAssetCode: "USD",
	}
	legacyCtx := mmodel.WithTransactionEconomicContext(ctx, legacyProof)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	missingFields := []struct {
		section string
		field   string
		nested  string
	}{
		{section: "operation", field: "id"},
		{section: "operation", field: "transactionId"},
		{section: "operation", field: "balanceId"},
		{section: "operation", field: "balanceKey"},
		{section: "operation", field: "accountId"},
		{section: "operation", field: "organizationId"},
		{section: "operation", field: "ledgerId"},
		{section: "operation", field: "type"},
		{section: "operation", field: "direction"},
		{section: "operation", field: "assetCode"},
		{section: "operation", field: "amountValue"},
		{section: "operation", field: "balanceAvailable"},
		{section: "operation", field: "balanceOnHold"},
		{section: "operation", field: "balanceVersion"},
		{section: "operation", field: "balanceAfterAvailable"},
		{section: "operation", field: "balanceAfterOnHold"},
		{section: "operation", field: "balanceAfterVersion"},
		{section: "operation", field: "snapshot", nested: "overdraftUsedBefore"},
		{section: "operation", field: "snapshot", nested: "overdraftUsedAfter"},
		{section: "balance", field: "id"},
		{section: "balance", field: "key"},
		{section: "balance", field: "accountId"},
		{section: "balance", field: "assetCode"},
		{section: "balance", field: "available"},
		{section: "balance", field: "onHold"},
		{section: "balance", field: "version"},
		{section: "balance", field: "accountType"},
		{section: "balance", field: "allowSending"},
		{section: "balance", field: "allowReceiving"},
		{section: "balance", field: "direction"},
		{section: "balance", field: "overdraftUsed"},
		{section: "balance", field: "allowOverdraft"},
		{section: "balance", field: "overdraftLimitEnabled"},
		{section: "balance", field: "overdraftLimit"},
		{section: "balance", field: "balanceScope"},
	}
	for _, missing := range missingFields {
		completeSeed, err := json.Marshal(completeQueue)
		require.NoError(t, err)
		incomplete := map[string]any{}
		require.NoError(t, json.Unmarshal(completeSeed, &incomplete))
		var economicBody map[string]any
		if missing.section == "operation" {
			economicBody = incomplete["operations"].([]any)[0].(map[string]any)
		} else {
			economicBody = incomplete["balancesAfter"].([]any)[0].(map[string]any)
		}
		if missing.nested == "" {
			delete(economicBody, missing.field)
		} else {
			delete(economicBody[missing.field].(map[string]any), missing.nested)
		}
		seed, err := json.Marshal(incomplete)
		require.NoError(t, err)
		require.NoError(t, infra.repo.AddMessageToQueue(ctx, transactionKey, seed))
		require.Error(t, infra.repo.FinalizeLegacyTransactionPersistence(legacyCtx, organizationID, ledgerID,
			transactionID, parentID, constant.CREATED, operationIDs),
			"missing %s.%s.%s must never be tombstoned or removed", missing.section, missing.field, missing.nested)
		retained, err := infra.repo.ReadMessageFromQueue(ctx, transactionKey)
		require.NoError(t, err, "reconciliation must retain incomplete phase-zero economic evidence")
		require.JSONEq(t, string(seed), string(retained))
	}
	completeSeed, err := json.Marshal(completeQueue)
	require.NoError(t, err)
	require.NoError(t, infra.repo.AddMessageToQueue(ctx, transactionKey, completeSeed))
	require.Error(t, infra.repo.FinalizeLegacyTransactionPersistence(legacyCtx, organizationID, ledgerID,
		transactionID, uuid.New(), constant.CREATED, operationIDs))
	_, err = infra.repo.ReadMessageFromQueue(ctx, transactionKey)
	require.NoError(t, err, "a foreign cleanup proof must preserve the phase-zero backup")
	divergentProof := legacyProof
	divergentProof.Operations = append([]mmodel.OperationRedis(nil), completeOperations...)
	divergentProof.Operations[0].AmountValue = decimal.NewFromInt(101)
	divergentCtx := mmodel.WithTransactionEconomicContext(ctx, divergentProof)
	require.Error(t, infra.repo.FinalizeLegacyTransactionPersistence(divergentCtx, organizationID, ledgerID,
		transactionID, parentID, constant.CREATED, operationIDs),
		"matching operation IDs cannot hide a different durable amount")
	_, err = infra.repo.ReadMessageFromQueue(ctx, transactionKey)
	require.NoError(t, err, "divergent PostgreSQL proof must preserve the phase-zero backup")
	require.NoError(t, infra.repo.FinalizeLegacyTransactionPersistence(legacyCtx, organizationID, ledgerID,
		transactionID, parentID, constant.CREATED, operationIDs))
	_, err = infra.repo.ReadMessageFromQueue(ctx, transactionKey)
	require.ErrorIs(t, err, redis.Nil)
	tombstoneKey := utils.TransactionPersistenceTombstoneKey(organizationID, ledgerID, transactionID)
	tombstoneRaw, err := infra.repo.Get(ctx, tombstoneKey)
	require.NoError(t, err)
	tombstone := mmodel.TransactionPersistenceTombstone{}
	require.NoError(t, json.Unmarshal([]byte(tombstoneRaw), &tombstone))
	assert.Equal(t, transactionID, tombstone.Identity)
	assert.Equal(t, parentID.String(), tombstone.ParentTransactionID)
	assert.Equal(t, constant.CREATED, tombstone.TransactionStatus)
	assert.Empty(t, tombstone.Owner)
	assert.Empty(t, tombstone.Outcome)
	assert.Empty(t, tombstone.RedisGeneration)
	assert.Equal(t, constant.ActionRevert, tombstone.Action)
	assert.Equal(t, "100", tombstone.TransactionAmount)
	assert.Equal(t, "USD", tombstone.TransactionAssetCode)
	require.NotEmpty(t, tombstone.EconomicEffectDigest)
	assert.Len(t, tombstone.Operations, len(operationIDs))
	assert.NotEmpty(t, tombstone.BalancesAfter)
	require.NoError(t, infra.repo.FinalizeLegacyTransactionPersistence(legacyCtx, organizationID, ledgerID,
		transactionID, parentID, constant.CREATED, operationIDs), "lost cleanup response must be replayable")
	tombstone.Action = constant.ActionCommit
	foreignAction, err := json.Marshal(tombstone)
	require.NoError(t, err)
	require.NoError(t, infra.repo.Set(ctx, tombstoneKey, string(foreignAction), 0))
	require.Error(t, infra.repo.FinalizeLegacyTransactionPersistence(legacyCtx, organizationID, ledgerID,
		transactionID, parentID, constant.CREATED, operationIDs),
		"a terminal receipt for another money-path action must never satisfy revert replay")
	require.NoError(t, infra.redisContainer.Client.Del(ctx, tombstoneKey).Err())
	require.Error(t, infra.repo.FinalizeLegacyTransactionPersistence(legacyCtx, organizationID, ledgerID,
		transactionID, parentID, constant.CREATED, operationIDs),
		"absence without the append-only terminal receipt is data loss, not a successful cleanup replay")
}

// TestIntegration_RevertBalanceOutcomeIsAtomic proves the recovery signal used
// after a lost Lua response is committed with the balance movement itself. A
// successful mutation always leaves its transaction backup marker; a
// script-declared rejection leaves neither marker nor partial balance write.
func TestIntegration_RevertBalanceOutcomeIsAtomic(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	successID := uuid.New()
	successOps := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@revert-outcome-success", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, successID, constant.CREATED, false, successOps)
	require.NoError(t, err)
	require.Len(t, result.After, 1)
	assert.True(t, decimal.NewFromInt(900).Equal(result.After[0].Available))

	outcome, err := infra.repo.ReadMessageFromQueue(ctx, utils.TransactionInternalKey(organizationID, ledgerID, successID.String()))
	require.NoError(t, err)
	assert.NotEmpty(t, outcome, "a committed balance movement must have its atomic recovery marker")

	rejectedID := uuid.New()
	rejectedOps := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@revert-outcome-rejected", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(50), "deposit",
	)}
	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, rejectedID, constant.CREATED, false, rejectedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())

	_, err = infra.repo.ReadMessageFromQueue(ctx, utils.TransactionInternalKey(organizationID, ledgerID, rejectedID.String()))
	assert.ErrorIs(t, err, redis.Nil, "a rolled-back Lua rejection must not publish a recovery marker")
	rejectedBalance, err := infra.repo.Get(ctx, rejectedOps[0].InternalKey)
	require.NoError(t, err)
	assert.Contains(t, rejectedBalance, `"Available":"50"`,
		"a rolled-back Lua rejection may seed the input balance but must preserve its pre-mutation amount")
}

func TestIntegration_BalanceExecutionAttemptIsCheckedAndConsumedWithOutcome(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	executionKey := utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID)
	outcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	owner := uuid.NewString()
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: executionKey,
		OutcomeKey:   outcomeKey,
		Owner:        owner,
		Outcome:      mmodel.TransactionOutcomeCommitted,
		Identity:     transactionID,
	}
	operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@economic-execution-attempt", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	queue := mmodel.TransactionRedisQueue{
		TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		TransactionStatus: constant.CREATED, AttemptOwner: owner, ExpectedOutcome: attempt.Outcome,
		EffectModeVersion: mmodel.TransactionEffectModeVersion, EffectMode: mmodel.TransactionEffectBalanceMutation,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "USD", Value: decimal.NewFromInt(100),
		}},
	}
	operations = bindFinalEconomicPlan(t, &queue, operations, constant.CREATED, false)
	seed, err := json.Marshal(queue)
	require.NoError(t, err)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	require.NoError(t, infra.repo.AddMessageToQueue(ctx, transactionKey, seed))

	_, err = infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CREATED, false, operations, attempt)
	require.Error(t, err)
	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, constant.ErrIdempotencyKey.Error(), conflict.Code)
	retained, err := infra.repo.ReadMessageFromQueue(ctx, transactionKey)
	require.NoError(t, err)
	assert.JSONEq(t, string(seed), string(retained),
		"an absent execution attempt must reject without changing the pre-Lua economic plan")

	acquired, err := infra.repo.AcquireOwnedKey(ctx, executionKey, owner, 300)
	require.NoError(t, err)
	require.True(t, acquired)
	result, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CREATED, false, operations, attempt)
	require.NoError(t, err)
	require.Len(t, result.After, 1)
	assert.True(t, decimal.NewFromInt(900).Equal(result.After[0].Available))

	executionValue, err := infra.repo.Get(ctx, executionKey)
	require.NoError(t, err)
	assert.Empty(t, executionValue, "successful balance Lua must consume the execution attempt atomically")
	ownerValue, err := infra.repo.Get(ctx, executionKey+":owner")
	require.NoError(t, err)
	assert.Empty(t, ownerValue)
	outcome, err := infra.repo.Get(ctx, outcomeKey)
	require.NoError(t, err)
	assert.NotEmpty(t, outcome)
}

func TestIntegration_OwnedLegacyFence_LateOwnerCannotDeleteReacquiredFence(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "idempotency:{owned-legacy-fence}:payload"

	acquired, err := infra.repo.AcquireOwnedKey(ctx, key, "owner-a", 1)
	require.NoError(t, err)
	require.True(t, acquired)

	require.Eventually(t, func() bool {
		acquired, acquireErr := infra.repo.AcquireOwnedKey(ctx, key, "owner-b", 300)
		return acquireErr == nil && acquired
	}, 3*time.Second, 25*time.Millisecond, "owner B must acquire after owner A's TTL expires")

	released, err := infra.repo.ReleaseOwnedKey(ctx, key, "owner-a")
	require.NoError(t, err)
	assert.False(t, released, "expired owner A must not delete owner B's reacquired fence")

	acquired, err = infra.repo.SetNX(ctx, key, "", 300)
	require.NoError(t, err)
	assert.False(t, acquired, "owner B's legacy-compatible empty fence must still exist")

	released, err = infra.repo.ReleaseOwnedKey(ctx, key, "owner-b")
	require.NoError(t, err)
	assert.True(t, released)
}

func TestIntegration_OwnedLegacyFence_DoesNotOverwriteOwnerOnlyState(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "idempotency:{owner-only-legacy-fence}:payload"
	ownerKey := key + ":owner"

	require.NoError(t, infra.redisContainer.Client.Set(ctx, ownerKey, "existing-owner", 0).Err())
	acquired, err := infra.repo.AcquireOwnedKey(ctx, key, "new-owner", 300)
	require.NoError(t, err)
	require.False(t, acquired, "an evicted main key cannot make a surviving owner token acquirable")
	owner, err := infra.repo.Get(ctx, ownerKey)
	require.NoError(t, err)
	require.Equal(t, "existing-owner", owner)
	mainExists, err := infra.redisContainer.Client.Exists(ctx, key).Result()
	require.NoError(t, err)
	require.Zero(t, mainExists)
}

func TestIntegration_OwnedLegacyFence_ExactOwnerCompletesAfterMainEviction(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "idempotency:{owner-only-terminal-fence}:payload"
	ownerKey := key + ":owner"
	replay := `{"id":"reserved-reverse"}`

	require.NoError(t, infra.redisContainer.Client.Set(ctx, ownerKey, "reserved-reverse", 0).Err())
	completed, err := infra.repo.CompleteOwnedKey(ctx, key, "reserved-reverse", replay, 300)
	require.NoError(t, err)
	require.True(t, completed, "the durable exact owner must recover a main key evicted before terminal publication")

	main, err := infra.repo.Get(ctx, key)
	require.NoError(t, err)
	require.JSONEq(t, replay, main)
	ownerExists, err := infra.redisContainer.Client.Exists(ctx, ownerKey).Result()
	require.NoError(t, err)
	require.Zero(t, ownerExists, "terminal publication must retire the recovered owner companion")
}

func TestIntegration_UnownedEmptyFence_ReleaseIsExactAndIdempotent(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "idempotency:{drained-phase-zero-fence}:payload"
	ownerKey := key + ":owner"

	require.NoError(t, infra.repo.Set(ctx, key, "", 0))
	released, err := infra.repo.ReleaseUnownedEmptyKey(ctx, key)
	require.NoError(t, err)
	require.True(t, released)

	released, err = infra.repo.ReleaseUnownedEmptyKey(ctx, key)
	require.NoError(t, err)
	require.True(t, released, "lost cleanup responses must replay exactly")

	require.NoError(t, infra.repo.Set(ctx, key, "", 0))
	require.NoError(t, infra.repo.Set(ctx, ownerKey, "bridge-owner", 0))
	released, err = infra.repo.ReleaseUnownedEmptyKey(ctx, key)
	require.NoError(t, err)
	require.False(t, released, "a successor owner companion must preserve the main fence")

	require.NoError(t, infra.repo.Set(ctx, key, `{"id":"foreign-replay"}`, 0))
	require.NoError(t, infra.redisContainer.Client.Del(ctx, ownerKey).Err())
	released, err = infra.repo.ReleaseUnownedEmptyKey(ctx, key)
	require.NoError(t, err)
	require.False(t, released, "a completed replay is never an abandoned empty fence")
}

func TestIntegration_OwnedLegacyFence_ForeignMainCannotBeReleasedOrCompletedByCompanion(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "idempotency:{foreign-main-owned-fence}:payload"
	ownerKey := key + ":owner"

	acquired, err := infra.repo.AcquireOwnedKey(ctx, key, "bridge-owner", 0)
	require.NoError(t, err)
	require.True(t, acquired)

	foreign := `{"id":"phase-zero-reverse"}`
	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, foreign, 0).Err(),
		"simulate an old writer racing after the companion was acquired")

	released, err := infra.repo.ReleaseOwnedKey(ctx, key, "bridge-owner")
	require.NoError(t, err)
	require.False(t, released, "owner alone must not authorize deleting a foreign main value")

	completed, err := infra.repo.CompleteOwnedKey(ctx, key, "bridge-owner", `{"id":"bridge-reverse"}`, 300)
	require.NoError(t, err)
	require.False(t, completed, "owner alone must not authorize overwriting a foreign main value")

	main, err := infra.repo.Get(ctx, key)
	require.NoError(t, err)
	require.JSONEq(t, foreign, main)
	owner, err := infra.repo.Get(ctx, ownerKey)
	require.NoError(t, err)
	require.Equal(t, "bridge-owner", owner, "unresolved ownership must remain fenced for reconciliation")
}

func TestIntegration_PhaseZeroFence_CompletesOnlyUnownedEmptyMain(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "idempotency:{phase-zero-terminal-fence}:payload"
	ownerKey := key + ":owner"
	replay := `{"id":"phase-zero-reverse"}`

	require.NoError(t, infra.repo.Set(ctx, key, "", 0))
	completed, err := infra.repo.CompleteUnownedKey(ctx, key, replay, 300)
	require.NoError(t, err)
	require.True(t, completed)
	value, err := infra.repo.Get(ctx, key)
	require.NoError(t, err)
	require.JSONEq(t, replay, value)

	require.NoError(t, infra.repo.Set(ctx, key, "", 0))
	require.NoError(t, infra.repo.Set(ctx, ownerKey, "bridge-owner", 0))
	completed, err = infra.repo.CompleteUnownedKey(ctx, key, replay, 300)
	require.NoError(t, err)
	require.False(t, completed, "an owner companion makes phase-zero completion ineligible")

	require.NoError(t, infra.repo.Del(ctx, ownerKey))
	require.NoError(t, infra.repo.Set(ctx, key, `{"id":"foreign-reverse"}`, 0))
	completed, err = infra.repo.CompleteUnownedKey(ctx, key, replay, 300)
	require.NoError(t, err)
	require.False(t, completed, "a non-empty main value can never be overwritten")
	value, err = infra.repo.Get(ctx, key)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"foreign-reverse"}`, value)
}

func TestIntegration_OwnedLegacyFence_PersistentUntilAtomicCompletion(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	key := "idempotency:{persistent-owned-legacy-fence}:payload"
	ownerKey := key + ":owner"

	acquired, err := infra.repo.AcquireOwnedKey(ctx, key, "bridge-owner", 0)
	require.NoError(t, err)
	require.True(t, acquired)

	fenceTTL, err := infra.redisContainer.Client.TTL(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), fenceTTL, "in-flight bridge fence must not expire")
	ownerTTL, err := infra.redisContainer.Client.TTL(ctx, ownerKey).Result()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), ownerTTL, "owner token must live exactly as long as its fence")

	oldPodAcquired, err := infra.repo.SetNX(ctx, key, "", 300)
	require.NoError(t, err)
	assert.False(t, oldPodAcquired, "an old pod cannot pass the bridge fence while the bridge request is paused")

	completed, err := infra.repo.CompleteOwnedKey(ctx, key, "bridge-owner", `{"id":"reserved-reverse"}`, 300)
	require.NoError(t, err)
	require.True(t, completed)

	replay, err := infra.repo.Get(ctx, key)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"reserved-reverse"}`, replay)
	completedTTL, err := infra.redisContainer.Client.TTL(ctx, key).Result()
	require.NoError(t, err)
	assert.Greater(t, completedTTL, time.Duration(0), "completed replay must regain the configured finite TTL")
	assert.LessOrEqual(t, completedTTL, 300*time.Second)
	ownerExists, err := infra.redisContainer.Client.Exists(ctx, ownerKey).Result()
	require.NoError(t, err)
	assert.Zero(t, ownerExists, "atomic completion must remove the owner token")
}

func TestIntegration_OwnedLegacyFence_RedisClusterRejectsCrossSlotButAcceptsCompanionScript(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	ctx := context.Background()
	cluster := startSingleNodeRedisCluster(t, ctx)
	connection := redistestutil.CreateConnection(t, cluster.Addr)
	repo, err := NewConsumerRedis(connection)
	require.NoError(t, err)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "legacy-payload-hash")
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "origin-scoped-hash")

	_, err = cluster.Client.Eval(ctx, "return 1", []string{legacyKey, originKey}).Result()
	require.ErrorContains(t, err, "CROSSSLOT",
		"the cluster must reject any attempted Lua coupling of legacy and origin barriers")

	acquired, err := repo.AcquireOwnedKey(ctx, legacyKey, "reserved-reverse", 0)
	require.NoError(t, err)
	require.True(t, acquired, "the legacy fence and owner companion must execute in one cluster slot")
	completed, err := repo.CompleteOwnedKey(ctx, legacyKey, "reserved-reverse", `{"id":"reserved-reverse"}`, 300)
	require.NoError(t, err)
	assert.True(t, completed)
	phaseZeroLegacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "phase-zero-legacy-payload-hash")
	require.NoError(t, repo.Set(ctx, phaseZeroLegacyKey, "", 0))
	completed, err = repo.CompleteUnownedKey(ctx, phaseZeroLegacyKey, `{"id":"phase-zero-reverse"}`, 300)
	require.NoError(t, err, "phase-zero empty-fence completion must remain inside its companion slot")
	assert.True(t, completed)

	transactionID := uuid.New()
	executionKey := utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID)
	outcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: executionKey,
		OutcomeKey:   outcomeKey,
		Owner:        transactionID.String(),
		Outcome:      mmodel.TransactionOutcomeCommitted,
		Identity:     transactionID,
	}
	leaseAcquired, err := repo.AcquireOwnedKey(ctx, executionKey, attempt.Owner, 300)
	require.NoError(t, err)
	require.True(t, leaseAcquired)
	operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@cluster-fenced-revert", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	operations[0].Balance.Direction = constant.DirectionDebit
	queue := mmodel.TransactionRedisQueue{
		TransactionID:     transactionID,
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		TransactionStatus: constant.CREATED,
		EffectModeVersion: mmodel.TransactionEffectModeVersion,
		EffectMode:        mmodel.TransactionEffectBalanceMutation,
		Action:            constant.ActionCommit,
		AttemptOwner:      attempt.Owner,
		ExpectedOutcome:   attempt.Outcome,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "USD", Value: decimal.NewFromInt(100),
		}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"@cluster-fenced-revert#default": {
				Asset: "USD", Value: decimal.NewFromInt(100),
				Operation: constant.DEBIT, Direction: constant.DirectionDebit,
			},
		}},
	}
	operations = bindFinalEconomicPlan(t, &queue, operations, constant.CREATED, false)
	seed, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, repo.SeedTransactionBackup(ctx, organizationID, ledgerID, transactionID, seed, attempt),
		"owner-fenced backup seeding must remain in the transactions slot")
	evidence, generationMatches, err := repo.TransactionEconomicEvidenceExists(ctx, organizationID, ledgerID, transactionID, "")
	require.NoError(t, err, "atomic rollout evidence inspection must remain in the transactions slot")
	require.True(t, evidence)
	require.True(t, generationMatches)
	result, err := repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CREATED, false, operations, attempt)
	require.NoError(t, err, "the execution lease and balance outcome must share the existing transactions slot")
	require.Len(t, result.After, 1)
	clusterOperationID := uuid.NewString()
	clusterOperation := redisEconomicOperationFromMovement(organizationID, ledgerID, transactionID, clusterOperationID,
		result.Before[0], result.After[0], constant.DEBIT, constant.DirectionDebit)
	clusterEconomicCtx := mmodel.WithTransactionEconomicContext(ctx, mmodel.TransactionEconomicContext{
		TransactionStatus: constant.CREATED, Action: constant.ActionCommit,
		TransactionAmount: "100", TransactionAssetCode: "USD",
	})
	_, _, _, err = repo.EnrichTransactionBackup(clusterEconomicCtx, organizationID, ledgerID, transactionID,
		[]mmodel.OperationRedis{clusterOperation},
		constant.ActionCommit, &attempt)
	require.NoError(t, err, "backup enrichment must remain in the transactions slot")
	clusterFinalizeCtx := mmodel.WithTransactionEconomicContext(ctx, mmodel.TransactionEconomicContext{
		TransactionStatus: constant.CREATED, Action: constant.ActionCommit,
		TransactionAmount: "100", TransactionAssetCode: "USD",
	})
	require.NoError(t, repo.FinalizeTransactionPersistence(clusterFinalizeCtx, organizationID, ledgerID, transactionID, attempt,
		[]mmodel.OperationRedis{clusterOperation}, mmodel.BalancesToRedis(result.After)),
		"exact outcome cleanup must remain in the transactions slot")
	evidence, generationMatches, err = repo.TransactionEconomicEvidenceExists(ctx, organizationID, ledgerID, transactionID, "")
	require.NoError(t, err)
	require.False(t, evidence, "terminal cleanup must remove backup, outcome, attempt, and owner as one drain proof")
	require.True(t, generationMatches)

	phaseZeroID := uuid.New()
	phaseZeroParentID := uuid.New()
	phaseZeroOperationID := uuid.NewString()
	phaseZeroOperation := completeRedisEconomicOperation(
		organizationID, ledgerID, phaseZeroID, phaseZeroOperationID)
	phaseZeroAfter := completeRedisEconomicBalance()
	phaseZeroAfter.ID = phaseZeroOperation.BalanceID
	phaseZeroAfter.Key = phaseZeroOperation.BalanceKey
	phaseZeroAfter.AccountID = phaseZeroOperation.AccountID
	phaseZeroAfter.AssetCode = phaseZeroOperation.AssetCode
	phaseZeroAfter.Available = phaseZeroOperation.BalanceAfterAvailable
	phaseZeroAfter.OnHold = phaseZeroOperation.BalanceAfterOnHold
	phaseZeroAfter.Version = phaseZeroOperation.BalanceAfterVersion
	phaseZeroBefore := phaseZeroAfter
	phaseZeroBefore.Available = phaseZeroOperation.BalanceAvailable
	phaseZeroBefore.OnHold = phaseZeroOperation.BalanceOnHold
	phaseZeroBefore.Version = phaseZeroOperation.BalanceVersion
	phaseZeroBackup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       phaseZeroID,
		ParentTransactionID: &phaseZeroParentID,
		OrganizationID:      organizationID,
		LedgerID:            ledgerID,
		TransactionStatus:   constant.CREATED,
		Action:              constant.ActionRevert,
		Balances:            []mmodel.BalanceRedis{phaseZeroBefore},
		BalancesAfter:       []mmodel.BalanceRedis{phaseZeroAfter},
		Operations:          []mmodel.OperationRedis{phaseZeroOperation},
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "USD", Value: decimal.NewFromInt(100),
		}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"@economic-proof#default": {
				Asset: "USD", Value: decimal.NewFromInt(100),
				Operation: constant.DEBIT, Direction: constant.DirectionDebit,
			},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, repo.AddMessageToQueue(ctx,
		utils.TransactionInternalKey(organizationID, ledgerID, phaseZeroID.String()), phaseZeroBackup))
	phaseZeroCtx := mmodel.WithTransactionEconomicContext(ctx, mmodel.TransactionEconomicContext{
		ParentTransactionID:  &phaseZeroParentID,
		TransactionStatus:    constant.CREATED,
		Action:               constant.ActionRevert,
		Operations:           []mmodel.OperationRedis{phaseZeroOperation},
		BalancesAfter:        []mmodel.BalanceRedis{phaseZeroAfter},
		TransactionAmount:    "100",
		TransactionAssetCode: "USD",
	})
	require.NoError(t, repo.FinalizeLegacyTransactionPersistence(phaseZeroCtx, organizationID, ledgerID,
		phaseZeroID, phaseZeroParentID, constant.CREATED, []string{phaseZeroOperationID}),
		"phase-zero compatibility cleanup must remain in the transactions slot")

	witness := &rolloutInitializationWitnessStub{}
	initializer := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeInitialize,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness,
		redisIntegrationInitializationRequestID)
	require.NoError(t, initializer.FinancialDurability(ctx),
		"every Redis Cluster shard must prove noeviction and healthy AOF before phase zero")
	require.NoError(t, initializer.InitializeFinancialDatasetGeneration(ctx))
	guard := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezePrepared,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness, "")
	tenantCtx := mmodel.WithTransactionEconomicContext(
		tmcore.ContextWithTenantID(ctx, "cluster-generation-tenant"),
		mmodel.TransactionEconomicContext{TransactionStatus: constant.CREATED, Action: constant.ActionCommit,
			TransactionAmount: "100", TransactionAssetCode: "USD"},
	)
	tenantTransactionID := uuid.New()
	tenantAttempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey:    utils.TransactionBalanceExecutionKey(organizationID, ledgerID, tenantTransactionID),
		OutcomeKey:      utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, tenantTransactionID),
		Owner:           uuid.NewString(),
		Outcome:         mmodel.TransactionOutcomeCommitted,
		Identity:        tenantTransactionID,
		RedisGeneration: redisIntegrationDatasetGeneration,
	}
	tenantLease, err := repo.AcquireOwnedKey(tenantCtx, tenantAttempt.ExecutionKey, tenantAttempt.Owner, 300)
	require.NoError(t, err)
	require.True(t, tenantLease)
	tenantBalanceOperations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@cluster-global-generation", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	tenantBalanceOperations[0].Balance.Direction = constant.DirectionDebit
	tenantQueue := mmodel.TransactionRedisQueue{
		TransactionID: tenantTransactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		TransactionStatus: constant.CREATED, AttemptOwner: tenantAttempt.Owner,
		EffectModeVersion: mmodel.TransactionEffectModeVersion, EffectMode: mmodel.TransactionEffectBalanceMutation,
		ExpectedOutcome: tenantAttempt.Outcome, RedisGeneration: redisIntegrationDatasetGeneration,
		Action: constant.ActionCommit,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "USD", Value: decimal.NewFromInt(100),
		}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"@cluster-global-generation#default": {
				Asset: "USD", Value: decimal.NewFromInt(100),
				Operation: constant.DEBIT, Direction: constant.DirectionDebit,
			},
		}},
	}
	tenantBalanceOperations = bindFinalEconomicPlan(t, &tenantQueue, tenantBalanceOperations, constant.CREATED, false)
	tenantSeed, err := json.Marshal(tenantQueue)
	require.NoError(t, err)
	require.NoError(t, repo.SeedTransactionBackup(tenantCtx, organizationID, ledgerID,
		tenantTransactionID, tenantSeed, tenantAttempt),
		"tenant-scoped backup and deployment generation must share the transactions slot")
	tenantResult, err := repo.ProcessOutcomeBalanceAtomicOperation(tenantCtx, organizationID, ledgerID,
		tenantTransactionID, constant.CREATED, false, tenantBalanceOperations, tenantAttempt)
	require.NoError(t, err,
		"tenant balances and the deployment generation must execute without CROSSSLOT")
	tenantOperationID := uuid.NewString()
	require.Len(t, tenantResult.Before, 1)
	require.Len(t, tenantResult.After, 1)
	tenantOperation := redisEconomicOperationFromMovement(organizationID, ledgerID, tenantTransactionID,
		tenantOperationID, tenantResult.Before[0], tenantResult.After[0], constant.DEBIT, constant.DirectionDebit)
	_, _, _, err = repo.EnrichTransactionBackup(tenantCtx, organizationID, ledgerID, tenantTransactionID,
		[]mmodel.OperationRedis{tenantOperation},
		constant.ActionCommit, &tenantAttempt)
	require.NoError(t, err,
		"tenant outcome enrichment and the deployment generation must execute without CROSSSLOT")
	tenantFinalizeCtx := mmodel.WithTransactionEconomicContext(tenantCtx, mmodel.TransactionEconomicContext{
		TransactionStatus: constant.CREATED, Action: constant.ActionCommit,
		TransactionAmount: "100", TransactionAssetCode: "USD",
	})
	require.NoError(t, repo.FinalizeTransactionPersistence(tenantFinalizeCtx, organizationID, ledgerID,
		tenantTransactionID, tenantAttempt, []mmodel.OperationRedis{tenantOperation},
		mmodel.BalancesToRedis(tenantResult.After)),
		"tenant terminal cleanup and the deployment generation must execute without CROSSSLOT")
	admitted, frozen, leaseHeld, err := guard.AcquireApprovedUpdate(ctx, "legacy", "cluster-update")
	require.NoError(t, err, "rollout admission must not issue a multi-slot Lua command")
	assert.True(t, admitted)
	assert.False(t, frozen)
	assert.True(t, leaseHeld)
	require.Error(t, guard.Activate(ctx))
	require.NoError(t, guard.ReleaseApprovedUpdate(ctx, "cluster-update"))
	require.NoError(t, guard.Activate(ctx), "marker transition and lease proof must share one cluster slot")
	admitted, leaseHeld, phase, err := guard.AcquireRevert(ctx, "legacy", "cluster-phase-zero-revert", "cluster-attempt")
	require.NoError(t, err)
	assert.True(t, admitted)
	assert.True(t, leaseHeld)
	assert.Equal(t, RevertUpdateFreezeActive, phase)
	terminal, err := guard.RevertTerminalHandoffComplete(ctx, "legacy", "cluster-phase-zero-revert")
	require.NoError(t, err, "generation proof must remain inside the rollout Cluster slot")
	assert.False(t, terminal)
	require.Error(t, guard.MarkPhaseZeroDrained(ctx))
	require.NoError(t, guard.CompleteRevert(ctx, "legacy", "cluster-phase-zero-revert"))
	terminal, err = guard.RevertTerminalHandoffComplete(ctx, "legacy", "cluster-phase-zero-revert")
	require.NoError(t, err)
	assert.True(t, terminal,
		"terminal proof must atomically observe tombstone plus absence of attempts and active origin")
	require.NoError(t, guard.MarkPhaseZeroDrained(ctx))
}

func TestIntegration_RevertUpdateFreezeMarkerIsSharedPersistentAndFinalizable(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	connection := redistestutil.CreateConnection(t, infra.redisContainer.Addr)
	witness := &rolloutInitializationWitnessStub{}
	initializer := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeInitialize,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness,
		redisIntegrationInitializationRequestID)
	guard := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezePrepared,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness, "")
	activeTargetGuard := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeActive,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness, "")
	releasedLegacy := NewRevertUpdateFreezeGuard(connection).WithRolloutInitializationWitness(witness, "")
	witness.mu.Lock()
	witness.inspectFailures = 1
	witness.mu.Unlock()
	_, err := releasedLegacy.ReadyForMode(ctx, "legacy")
	require.Error(t, err, "target-empty readiness must fail when the deployment primary is unavailable")
	witness.mu.Lock()
	witness.inspectFailures = 1
	witness.mu.Unlock()
	_, _, _, err = releasedLegacy.AcquireRevert(ctx, "legacy", "unverified-old-origin", "unverified-old-attempt")
	require.Error(t, err, "target-empty admission must fail when the deployment primary is unavailable")
	releasedAdmitted, releasedLease, releasedPhase, err := releasedLegacy.AcquireRevert(ctx, "legacy",
		"released-old-origin", "released-old-attempt")
	require.NoError(t, err)
	assert.True(t, releasedAdmitted)
	assert.True(t, releasedLease,
		"phase-zero capable old pods must publish an in-flight attempt before initialization")
	assert.Equal(t, RevertUpdateFreezeUninitialized, releasedPhase,
		"target-empty legacy must remain the released old algorithm without a dataset witness")
	require.Error(t, initializer.FinancialDurability(ctx),
		"the default ephemeral test Redis must not be mistaken for a durable financial trust boundary")
	require.Error(t, initializer.InitializeFinancialDatasetGeneration(ctx),
		"initialization must insert PREPARING before it fails on durability or an in-flight old request")
	_, err = releasedLegacy.ReadyForMode(ctx, "legacy")
	require.Error(t, err,
		"an old request paused before balance must abort once initialization commits PREPARING")
	_, _, _, err = releasedLegacy.AcquireRevert(ctx, "legacy", "late-old-origin", "late-old-attempt")
	require.Error(t, err, "PREPARING must block every new old request")
	require.NoError(t, releasedLegacy.ReleaseRevert(ctx, "legacy", "released-old-origin", "released-old-attempt"))
	phaseBeforeDurability, err := guard.Phase(ctx)
	require.NoError(t, err)
	assert.Empty(t, phaseBeforeDurability, "failed durability preflight must not create the rollout marker")
	configureFinancialRedisDurability(t, ctx, infra.redisContainer.Client)
	require.Eventually(t, func() bool { return guard.FinancialDurability(ctx) == nil },
		10*time.Second, 50*time.Millisecond)
	witness.mu.Lock()
	witness.completeFailures = 1
	witness.mu.Unlock()
	require.Error(t, initializer.InitializeFinancialDatasetGeneration(ctx),
		"crash after exact Redis preparation must leave the PostgreSQL birth certificate PREPARING")
	preparedBeforePostgres, err := guard.Phase(ctx)
	require.NoError(t, err)
	assert.Equal(t, RevertUpdateFreezePrepared, preparedBeforePostgres)
	require.NoError(t, initializer.InitializeFinancialDatasetGeneration(ctx),
		"exact retry must adopt Redis and promote the same PostgreSQL birth certificate")
	require.NoError(t, guard.ValidatePrepared(ctx))
	preparedRestart := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezePrepared,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness, "")
	require.NoError(t, preparedRestart.ValidatePrepared(ctx),
		"prepared startup must validate without rewriting shared state")
	preparedGeneration, err := guard.FinancialDatasetGeneration(ctx)
	require.NoError(t, err)
	tenantCtx := tmcore.ContextWithTenantID(ctx, "rollout-witness-tenant")
	evidence, generationMatches, err := infra.repo.TransactionEconomicEvidenceExists(tenantCtx,
		uuid.New(), uuid.New(), uuid.New(), preparedGeneration)
	require.NoError(t, err)
	assert.False(t, evidence)
	assert.True(t, generationMatches,
		"tenant namespacing must not change the deployment-wide financial dataset witness")
	targetReady, err := activeTargetGuard.ReadyForMode(ctx, "legacy")
	require.NoError(t, err)
	assert.False(t, targetReady,
		"a configured active target must not reinterpret a lost marker as a never-started rollout")
	targetFrozen, targetUpdateReady, err := activeTargetGuard.ApprovedUpdatePolicy(ctx, "legacy")
	require.NoError(t, err)
	assert.False(t, targetFrozen)
	assert.False(t, targetUpdateReady,
		"APPROVED update preflight must fail closed instead of treating the missing marker as unfrozen")
	targetAdmitted, _, _, err := activeTargetGuard.AcquireRevert(ctx, "legacy",
		"lost-marker-origin", "lost-marker-attempt")
	require.NoError(t, err)
	assert.False(t, targetAdmitted, "atomic admission must fail closed with target active and marker absent")

	active, err := guard.Active(ctx)
	require.NoError(t, err)
	assert.False(t, active)
	legacyReady, err := guard.ReadyForMode(ctx, "legacy")
	require.NoError(t, err)
	assert.True(t, legacyReady, "phase zero must be readiness-verifiable before activation")
	frozen, bridgeUpdateReady, err := guard.ApprovedUpdatePolicy(ctx, "bridge")
	require.NoError(t, err)
	assert.False(t, frozen)
	assert.False(t, bridgeUpdateReady, "one absent-marker snapshot must not admit bridge updates")

	updateAdmitted, updateFrozen, updateLease, err := guard.AcquireApprovedUpdate(ctx, "legacy", "update-before-activation")
	require.NoError(t, err)
	assert.True(t, updateAdmitted)
	assert.False(t, updateFrozen)
	assert.True(t, updateLease)
	require.Error(t, guard.Activate(ctx), "activation must wait for every admitted APPROVED update to finish")
	require.NoError(t, guard.ReleaseApprovedUpdate(ctx, "update-before-activation"))
	revertAdmitted, revertLease, revertPhase, err := guard.AcquireRevert(ctx, "legacy", "revert-before-activation", "lost-response-attempt")
	require.NoError(t, err)
	assert.True(t, revertAdmitted)
	assert.True(t, revertLease)
	assert.Equal(t, RevertUpdateFreezePrepared, revertPhase)
	require.Error(t, guard.Activate(ctx),
		"activation must wait until every pre-activation reverse is durably persisted or proved pre-movement")
	retryAdmitted, retryLease, _, err := guard.AcquireRevert(ctx, "legacy", "revert-before-activation", "lost-response-attempt")
	require.NoError(t, err)
	require.True(t, retryAdmitted)
	require.True(t, retryLease)
	require.NoError(t, guard.ReleaseRevert(ctx, "legacy", "revert-before-activation", "lost-response-attempt"))
	require.Error(t, guard.MarkPhaseZeroDrained(ctx), "drain cannot manufacture proof that the freeze was active")
	require.Error(t, guard.Finalize(ctx), "finalization cannot skip the drain proof")
	require.NoError(t, guard.Activate(ctx),
		"retrying one lost-response attempt must not inflate its durable admission")
	require.NoError(t, activeTargetGuard.Activate(ctx),
		"active startup must be an exact idempotent replay")
	targetReady, err = activeTargetGuard.ReadyForMode(ctx, "legacy")
	require.NoError(t, err)
	assert.True(t, targetReady)
	active, err = guard.Active(ctx)
	require.NoError(t, err)
	assert.True(t, active)
	bridgeReady, err := guard.ReadyForMode(ctx, "bridge")
	require.NoError(t, err)
	assert.True(t, bridgeReady)
	finalReady, err := guard.ReadyForMode(ctx, "final")
	require.NoError(t, err)
	assert.False(t, finalReady, "final must not coexist directly with phase zero while the marker is active")
	frozen, bridgeUpdateReady, err = guard.ApprovedUpdatePolicy(ctx, "bridge")
	require.NoError(t, err)
	assert.True(t, frozen)
	assert.True(t, bridgeUpdateReady)
	updateAdmitted, updateFrozen, updateLease, err = guard.AcquireApprovedUpdate(ctx, "legacy", "blocked-update")
	require.NoError(t, err)
	assert.False(t, updateAdmitted)
	assert.True(t, updateFrozen)
	assert.False(t, updateLease)
	markerTTL, err := infra.redisContainer.Client.TTL(ctx, RevertUpdateFreezeKey).Result()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), markerTTL)

	legacyRevertAdmitted, legacyRevertLease, legacyRevertPhase, err := guard.AcquireRevert(ctx, "legacy", "phase-zero-revert", "phase-zero-attempt-a")
	require.NoError(t, err)
	assert.True(t, legacyRevertAdmitted)
	assert.True(t, legacyRevertLease)
	assert.Equal(t, RevertUpdateFreezeActive, legacyRevertPhase)
	retryLegacyAdmitted, retryLegacyLease, _, err := guard.AcquireRevert(ctx, "legacy", "phase-zero-revert", "phase-zero-attempt-a")
	require.NoError(t, err)
	require.True(t, retryLegacyAdmitted)
	require.True(t, retryLegacyLease)
	concurrentLegacyAdmitted, concurrentLegacyLease, _, err := guard.AcquireRevert(ctx, "legacy", "phase-zero-revert", "phase-zero-attempt-b")
	require.NoError(t, err)
	require.True(t, concurrentLegacyAdmitted)
	require.True(t, concurrentLegacyLease)
	otherLegacyAdmitted, otherLegacyLease, otherLegacyPhase, err := guard.AcquireRevert(ctx, "legacy", "other-phase-zero-revert", "other-phase-zero-attempt")
	require.NoError(t, err)
	assert.True(t, otherLegacyAdmitted)
	assert.True(t, otherLegacyLease)
	assert.Equal(t, RevertUpdateFreezeActive, otherLegacyPhase)
	require.Error(t, guard.MarkPhaseZeroDrained(ctx), "drain cannot advance while a phase-zero revert is in flight")
	require.NoError(t, guard.ReleaseRevert(ctx, "legacy", "phase-zero-revert", "phase-zero-attempt-a"))
	require.NoError(t, guard.ReleaseRevert(ctx, "legacy", "phase-zero-revert", "phase-zero-attempt-a"),
		"release retry must be idempotent")
	require.Error(t, guard.MarkPhaseZeroDrained(ctx),
		"one exact attempt release cannot remove a distinct same-origin attempt")
	require.NoError(t, guard.CompleteRevert(ctx, "legacy", "phase-zero-revert"))
	require.NoError(t, guard.CompleteRevert(ctx, "legacy", "phase-zero-revert"),
		"a lost completion response must be safely retryable")
	terminalRetryAdmitted, terminalRetryLease, _, err := guard.AcquireRevert(ctx, "legacy",
		"phase-zero-revert", "late-terminal-retry")
	require.NoError(t, err)
	require.True(t, terminalRetryAdmitted)
	require.False(t, terminalRetryLease,
		"a delayed admission retry cannot recreate attempts after terminal completion")
	require.NoError(t, guard.ReleaseRevert(ctx, "legacy", "phase-zero-revert", "phase-zero-attempt-b"),
		"a delayed HTTP defer after terminal completion must not recreate the origin admission")
	require.Error(t, guard.MarkPhaseZeroDrained(ctx), "owner cleanup must not remove another request's rollout lease")
	require.NoError(t, guard.ReleaseRevert(ctx, "legacy", "other-phase-zero-revert", "other-phase-zero-attempt"))

	require.Error(t, guard.Finalize(ctx), "finalization cannot skip the machine-verifiable drain phase")
	require.NoError(t, guard.MarkPhaseZeroDrained(ctx))
	drainedRestart := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeDrained,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness, "")
	require.NoError(t, drainedRestart.MarkPhaseZeroDrained(ctx),
		"drained startup must be an exact idempotent replay")
	completed, err := infra.redisContainer.Client.SIsMember(ctx, revertPhaseZeroCompletedKey, "phase-zero-revert").Result()
	require.NoError(t, err)
	assert.True(t, completed, "phase transition must preserve append-only terminal origin proof")
	active, err = guard.Active(ctx)
	require.NoError(t, err)
	assert.True(t, active, "approved updates remain frozen throughout the drained phase")
	legacyReady, err = guard.ReadyForMode(ctx, "legacy")
	require.NoError(t, err)
	assert.False(t, legacyReady, "the drained marker must fence every surviving phase-zero pod")
	frozen, legacyUpdateReady, err := guard.ApprovedUpdatePolicy(ctx, "legacy")
	require.NoError(t, err)
	assert.True(t, frozen, "the drained phase keeps approved updates frozen")
	assert.False(t, legacyUpdateReady, "the same marker snapshot fences legacy readiness")
	bridgeReady, err = drainedRestart.ReadyForMode(ctx, "bridge")
	require.NoError(t, err)
	assert.True(t, bridgeReady)
	finalReady, err = drainedRestart.ReadyForMode(ctx, "final")
	require.NoError(t, err)
	assert.True(t, finalReady, "final becomes admissible only after phase zero is durably drained")
	bridgeRevertAdmitted, bridgeRevertLease, bridgeRevertPhase, err := drainedRestart.AcquireRevert(ctx, "bridge", "bridge-revert", "bridge-attempt")
	require.NoError(t, err)
	assert.True(t, bridgeRevertAdmitted)
	assert.True(t, bridgeRevertLease)
	assert.Empty(t, bridgeRevertPhase, "only legacy admission needs the active-vs-absent algorithm selector")
	require.Error(t, drainedRestart.Finalize(ctx), "finalization must wait for every admitted bridge revert to finish")
	require.NoError(t, drainedRestart.ReleaseRevert(ctx, "bridge", "bridge-revert", "bridge-attempt"))

	require.NoError(t, drainedRestart.Finalize(ctx))
	finalRestart := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeFinalized,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness, "")
	require.NoError(t, finalRestart.Finalize(ctx),
		"finalized startup must be an exact idempotent replay")
	active, err = finalRestart.Active(ctx)
	require.NoError(t, err)
	assert.False(t, active, "finalization must restore approved updates")
	bridgeReady, err = finalRestart.ReadyForMode(ctx, "bridge")
	require.NoError(t, err)
	assert.False(t, bridgeReady, "a finalized rollout must fence any remaining bridge pod")
	finalReady, err = finalRestart.ReadyForMode(ctx, "final")
	require.NoError(t, err)
	assert.True(t, finalReady, "final pods must remain restart-safe after unfreeze")
	frozen, finalUpdateReady, err := finalRestart.ApprovedUpdatePolicy(ctx, "final")
	require.NoError(t, err)
	assert.False(t, frozen)
	assert.True(t, finalUpdateReady)
	require.Error(t, guard.Activate(ctx), "a finalized rollout marker must never be reopened")
	updateAdmitted, updateFrozen, updateLease, err = finalRestart.AcquireApprovedUpdate(ctx, "final", "final-update")
	require.NoError(t, err)
	assert.True(t, updateAdmitted)
	assert.False(t, updateFrozen)
	assert.False(t, updateLease, "terminal final updates need no lease because no later rollout transition exists")
	require.NoError(t, finalRestart.ReleaseApprovedUpdate(ctx, "final-update"))
	legacyReady, err = finalRestart.ReadyForMode(ctx, "legacy")
	require.NoError(t, err)
	assert.False(t, legacyReady, "finalization must fence any surviving phase-zero pod")

	require.NoError(t, infra.redisContainer.Client.Del(ctx, FinancialDatasetGenerationKey).Err())
	finalTargetGuard := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeFinalized,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness, "")
	require.Error(t, finalTargetGuard.Finalize(ctx),
		"final startup must fail closed when the financial dataset witness is lost")
	require.Error(t, initializer.InitializeFinancialDatasetGeneration(ctx),
		"explicit initialization must not manufacture a new witness after finalization")
	require.NoError(t, infra.redisContainer.Client.Set(ctx, FinancialDatasetGenerationKey, preparedGeneration, 0).Err())
	require.NoError(t, infra.redisContainer.Client.Del(ctx, RevertUpdateFreezeKey).Err())
	require.Error(t, activeTargetGuard.Activate(ctx),
		"active startup must not recreate a consumed rollout marker after shared-state loss")
}

func TestIntegration_RevertRolloutInitializationIsConcurrentSingleAssignment(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	configureFinancialRedisDurability(t, ctx, infra.redisContainer.Client)
	connection := redistestutil.CreateConnection(t, infra.redisContainer.Addr)
	witness := &rolloutInitializationWitnessStub{}
	preparedBeforeRedis, created, err := witness.BeginRolloutInitialization(ctx,
		uuid.MustParse(redisIntegrationDatasetGeneration), uuid.MustParse(redisIntegrationInitializationRequestID))
	require.NoError(t, err)
	require.True(t, created)
	assert.False(t, preparedBeforeRedis,
		"a crash after PostgreSQL PREPARING and before Redis must be exactly resumable")
	require.NoError(t, infra.redisContainer.Client.Set(ctx,
		FinancialDatasetGenerationKey, redisIntegrationDatasetGeneration, 0).Err(),
		"simulate a crash after creating the financial generation and before the rollout marker")
	assert.Zero(t, infra.redisContainer.Client.Exists(ctx,
		RevertUpdateFreezeKey, RevertRolloutGenerationKey).Val())
	first := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeInitialize,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness,
		redisIntegrationInitializationRequestID)
	second := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeInitialize,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness,
		redisIntegrationInitializationRequestID)

	errs := make(chan error, 2)
	go func() { errs <- first.InitializeFinancialDatasetGeneration(ctx) }()
	go func() { errs <- second.InitializeFinancialDatasetGeneration(ctx) }()
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)

	configured, err := first.FinancialDatasetGeneration(ctx)
	require.NoError(t, err)
	assert.Equal(t, redisIntegrationDatasetGeneration, configured)
	prepared := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezePrepared,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness, "")
	require.NoError(t, prepared.ValidatePrepared(ctx))

	divergent := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeInitialize,
		"f40c098f-e043-44b4-9e19-d638f374cdd1").WithRolloutInitializationWitness(witness,
		redisIntegrationInitializationRequestID)
	require.Error(t, divergent.InitializeFinancialDatasetGeneration(ctx),
		"a concurrent or restarted initializer cannot replace the first dataset identity")
	differentRequest := NewRevertUpdateFreezeGuard(connection, RevertUpdateFreezeInitialize,
		redisIntegrationDatasetGeneration).WithRolloutInitializationWitness(witness,
		"6412727c-fbe2-461a-a486-bb9db3add330")
	require.Error(t, differentRequest.InitializeFinancialDatasetGeneration(ctx),
		"same generation with a different initialization request must conflict")

	require.NoError(t, infra.redisContainer.Client.Del(ctx,
		RevertUpdateFreezeKey, RevertRolloutGenerationKey).Err())
	require.Error(t, first.InitializeFinancialDatasetGeneration(ctx),
		"prepared birth certificate must prevent recreation after rollout marker loss")
	assert.Zero(t, infra.redisContainer.Client.Exists(ctx,
		RevertUpdateFreezeKey, RevertRolloutGenerationKey).Val())
	assert.Equal(t, redisIntegrationDatasetGeneration,
		infra.redisContainer.Client.Get(ctx, FinancialDatasetGenerationKey).Val())
	require.NoError(t, infra.redisContainer.Client.Set(ctx,
		RevertRolloutGenerationKey, redisIntegrationDatasetGeneration, 0).Err())
	require.NoError(t, infra.redisContainer.Client.Set(ctx,
		RevertUpdateFreezeKey, RevertUpdateFreezePrepared, 0).Err())
	require.NoError(t, infra.redisContainer.Client.Del(ctx,
		FinancialDatasetGenerationKey, RevertUpdateFreezeKey, RevertRolloutGenerationKey).Err())
	require.Error(t, prepared.ValidatePrepared(ctx),
		"loss after prepared must fail serving startup closed")
	require.Error(t, first.InitializeFinancialDatasetGeneration(ctx),
		"even total Redis loss cannot be reclassified as first installation")
	assert.Zero(t, infra.redisContainer.Client.Exists(ctx,
		FinancialDatasetGenerationKey, RevertUpdateFreezeKey, RevertRolloutGenerationKey).Val(),
		"failed reinitialization must not manufacture any shared state")
	admitted, _, _, err := prepared.AcquireRevert(ctx, "legacy", "lost-dataset", "attempt")
	require.NoError(t, err)
	assert.False(t, admitted, "lost financial generation must prevent money-path admission")
	require.Error(t, divergent.InitializeFinancialDatasetGeneration(ctx),
		"a divergent initializer cannot reinterpret prepared dataset loss as first install")
}

func startSingleNodeRedisCluster(t *testing.T, ctx context.Context) *redistestutil.ContainerResult {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "valkey/valkey:8",
			ExposedPorts: []string{"6379/tcp"},
			Cmd: []string{
				"valkey-server",
				"--cluster-enabled", "yes",
				"--cluster-config-file", "nodes.conf",
				"--maxmemory-policy", "noeviction",
				"--appendonly", "yes",
				"--appendfsync", "always",
			},
			WaitingFor: wait.ForAll(
				wait.ForLog("Ready to accept connections"),
				wait.ForListeningPort("6379/tcp"),
			).WithDeadline(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	addr := net.JoinHostPort(host, port.Port())
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Ping(ctx).Err())

	for start := 0; start < 16384; start += 1000 {
		end := start + 1000
		if end > 16384 {
			end = 16384
		}
		args := make([]any, 0, end-start+2)
		args = append(args, "CLUSTER", "ADDSLOTS")
		for slot := start; slot < end; slot++ {
			args = append(args, slot)
		}
		require.NoError(t, client.Do(ctx, args...).Err(), "assign cluster slots %d-%d", start, end-1)
	}
	require.Eventually(t, func() bool {
		info, infoErr := client.ClusterInfo(ctx).Result()
		return infoErr == nil && strings.Contains(info, "cluster_state:ok")
	}, 5*time.Second, 50*time.Millisecond, fmt.Sprintf("single-node cluster at %s must own all slots", addr))

	return &redistestutil.ContainerResult{Container: container, Client: client, Addr: addr}
}

func configureFinancialRedisDurability(t *testing.T, ctx context.Context, client redis.Cmdable) {
	t.Helper()

	require.NoError(t, client.ConfigSet(ctx, "maxmemory-policy", "noeviction").Err())
	require.NoError(t, client.ConfigSet(ctx, "appendfsync", "always").Err())
	require.NoError(t, client.ConfigSet(ctx, "appendonly", "yes").Err())
}
