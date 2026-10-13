//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

type cleanupIsolationFixture struct {
	t                  *testing.T
	ctx                context.Context
	client             *redis.Client
	repo               *RedisConsumerRepository
	tenant             string
	organizationID     uuid.UUID
	ledgerID           uuid.UUID
	queue              string
	receipts, guards   string
	protection         string
	schedule           string
	retentionSeconds   int64
	terminalCompletion time.Time
}

func newCleanupIsolationFixture(t *testing.T, client *redis.Client) *cleanupIsolationFixture {
	t.Helper()

	tenant := "cleanup-isolation-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenant)
	repo, err := NewConsumerRedis(&recoveryAckClient{client: client})
	require.NoError(t, err)

	fixture := &cleanupIsolationFixture{
		t: t, ctx: ctx, client: client, repo: repo, tenant: tenant,
		organizationID:     uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		ledgerID:           uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		retentionSeconds:   300,
		terminalCompletion: time.Date(2042, time.February, 3, 4, 5, 6, 0, time.UTC),
	}
	scope := fixture.organizationID.String() + ":" + fixture.ledgerID.String()
	fixture.queue = fixture.key(TransactionBackupQueue)
	fixture.receipts = fixture.key("engine:" + cachepolicy.HashTag + ":receipts:" + scope)
	fixture.guards = fixture.key("engine:" + cachepolicy.HashTag + ":guards:" + scope)
	fixture.protection = fixture.key("engine:" + cachepolicy.HashTag + ":protection:" + scope)
	fixture.schedule = fixture.key(EngineRecoveryCleanupSchedule)

	return fixture
}

func (fixture *cleanupIsolationFixture) key(name string) string {
	fixture.t.Helper()

	key, err := tenantKeyFromContextOrError(fixture.ctx, name)
	require.NoError(fixture.t, err)

	return key
}

func (fixture *cleanupIsolationFixture) deadline() time.Time {
	return fixture.terminalCompletion.Add(time.Duration(fixture.retentionSeconds) * time.Second)
}

// acknowledged seeds one single-transaction execution and completes it through
// the protected acknowledgement, so its cleanup proof is the one production writes.
func (fixture *cleanupIsolationFixture) acknowledged(executionID, transactionID uuid.UUID) string {
	fixture.t.Helper()

	field := transactionID.String() + ":" + executionID.String()
	receipt := retentionReceipt{
		FormatVersion: 1, TenantID: fixture.tenant, OrganizationID: fixture.organizationID.String(), LedgerID: fixture.ledgerID.String(),
		ExecutionID: executionID.String(), IntentFingerprint: "isolation", Response: `{"protocolVersion":1,"movements":[],"final":[]}`,
		Protection: retentionReceiptProtection{
			FormatVersion: 1, RetentionSeconds: fixture.retentionSeconds,
			Transactions: []string{transactionID.String()}, RecoveryFields: []string{field},
			Acknowledged: map[string]bool{}, TerminalCompletedAtMS: map[string]int64{},
		},
	}
	rawReceipt, err := json.Marshal(receipt)
	require.NoError(fixture.t, err)
	require.NoError(fixture.t, fixture.client.HSet(fixture.ctx, fixture.receipts, executionID.String(), rawReceipt).Err())
	require.NoError(fixture.t, fixture.client.HSet(fixture.ctx, fixture.queue, field, "recovery-"+executionID.String()).Err())
	require.NoError(fixture.t, fixture.client.HSet(fixture.ctx, fixture.guards, transactionID.String(), "APPROVED").Err())
	coordinator := `{"formatVersion":1,"executions":{"` + executionID.String() + `":0}}`
	require.NoError(fixture.t, fixture.client.HSet(fixture.ctx, fixture.protection, transactionID.String(), coordinator).Err())

	status, err := fixture.repo.CompareAndDeleteRecoveryWithProtection(
		fixture.ctx, fixture.organizationID, fixture.ledgerID, field, "recovery-"+executionID.String(), true, fixture.terminalCompletion,
	)
	require.NoError(fixture.t, err)
	require.Equal(fixture.t, RecoveryAckDeleted, status)

	return recoveryCleanupMember(fixture.organizationID, fixture.ledgerID, executionID)
}

// forgedDeadline seeds an acknowledged execution whose scheduled deadline is
// earlier than its terminal proof allows, so the cleanup script rejects it.
func (fixture *cleanupIsolationFixture) forgedDeadline(executionID, transactionID uuid.UUID, scheduledAt time.Time) string {
	fixture.t.Helper()

	field := transactionID.String() + ":" + executionID.String()
	forged := scheduledAt.UnixMilli()
	receipt := retentionReceipt{
		FormatVersion: 1, TenantID: fixture.tenant, OrganizationID: fixture.organizationID.String(), LedgerID: fixture.ledgerID.String(),
		ExecutionID: executionID.String(), IntentFingerprint: "forged", Response: `{"protocolVersion":1,"movements":[],"final":[]}`,
		Protection: retentionReceiptProtection{
			FormatVersion: 1, RetentionSeconds: fixture.retentionSeconds,
			Transactions: []string{transactionID.String()}, RecoveryFields: []string{field},
			Acknowledged:          map[string]bool{transactionID.String(): true},
			TerminalCompletedAtMS: map[string]int64{transactionID.String(): fixture.terminalCompletion.UnixMilli()},
			CleanupAfterMS:        forged,
		},
	}
	rawReceipt, err := json.Marshal(receipt)
	require.NoError(fixture.t, err)

	member := recoveryCleanupMember(fixture.organizationID, fixture.ledgerID, executionID)
	deadlineText := strconv.FormatInt(forged, 10)
	require.NoError(fixture.t, fixture.client.HSet(fixture.ctx, fixture.receipts, executionID.String(), rawReceipt).Err())
	require.NoError(fixture.t, fixture.client.HSet(fixture.ctx, fixture.guards, transactionID.String(), "APPROVED").Err())
	require.NoError(fixture.t, fixture.client.HSet(fixture.ctx, fixture.protection, transactionID.String(),
		`{"formatVersion":1,"executions":{"`+executionID.String()+`":`+deadlineText+`},"cleanupAfterMs":`+deadlineText+`}`).Err())
	require.NoError(fixture.t, fixture.client.ZAdd(fixture.ctx, fixture.schedule, redis.Z{Score: float64(forged), Member: member}).Err())

	return member
}

// mismatchedScopes seeds a scheduled execution whose receipt names fewer
// transactions than scopes, which cleanup rejects before running its script.
func (fixture *cleanupIsolationFixture) mismatchedScopes(executionID, transactionID uuid.UUID, scheduledAt time.Time) string {
	fixture.t.Helper()

	rawReceipt := `{"formatVersion":1,"tenantId":"` + fixture.tenant + `","organizationId":"` + fixture.organizationID.String() +
		`","ledgerId":"` + fixture.ledgerID.String() + `","executionId":"` + executionID.String() +
		`","protection":{"formatVersion":2,"transactions":["` + transactionID.String() + `"],"scopes":[` +
		`{"organizationId":"` + fixture.organizationID.String() + `","ledgerId":"` + fixture.ledgerID.String() + `"},` +
		`{"organizationId":"` + fixture.organizationID.String() + `","ledgerId":"` + fixture.ledgerID.String() + `"}]}}`

	member := recoveryCleanupMember(fixture.organizationID, fixture.ledgerID, executionID)
	require.NoError(fixture.t, fixture.client.HSet(fixture.ctx, fixture.receipts, executionID.String(), rawReceipt).Err())
	require.NoError(fixture.t, fixture.client.ZAdd(fixture.ctx, fixture.schedule, redis.Z{Score: float64(scheduledAt.UnixMilli()), Member: member}).Err())

	return member
}

func (fixture *cleanupIsolationFixture) receiptExists(executionID uuid.UUID) bool {
	return fixture.client.HExists(fixture.ctx, fixture.receipts, executionID.String()).Val()
}

func (fixture *cleanupIsolationFixture) score(member string) float64 {
	return fixture.client.ZScore(fixture.ctx, fixture.schedule, member).Val()
}

func TestIntegrationRecoveryCleanupReschedulesRejectedProofAndCleansTheRest(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newCleanupIsolationFixture(t, container.Client)

	forgedExecution := uuid.MustParse("a1111111-1111-4111-8111-111111111111")
	forgedTransaction := uuid.MustParse("a2222222-2222-4222-8222-222222222222")
	scopesExecution := uuid.MustParse("b1111111-1111-4111-8111-111111111111")
	scopesTransaction := uuid.MustParse("b2222222-2222-4222-8222-222222222222")
	firstExecution := uuid.MustParse("c1111111-1111-4111-8111-111111111111")
	firstTransaction := uuid.MustParse("c2222222-2222-4222-8222-222222222222")
	secondExecution := uuid.MustParse("d1111111-1111-4111-8111-111111111111")
	secondTransaction := uuid.MustParse("d2222222-2222-4222-8222-222222222222")

	// The rejected entries sort first, so a pass that stops at the first
	// rejection would clean neither valid execution.
	forgedMember := fixture.forgedDeadline(forgedExecution, forgedTransaction, fixture.terminalCompletion.Add(time.Second))
	scopesMember := fixture.mismatchedScopes(scopesExecution, scopesTransaction, fixture.terminalCompletion.Add(2*time.Second))
	firstMember := fixture.acknowledged(firstExecution, firstTransaction)
	secondMember := fixture.acknowledged(secondExecution, secondTransaction)
	forgedCoordinator := fixture.client.HGet(fixture.ctx, fixture.protection, forgedTransaction.String()).Val()

	now := fixture.deadline()
	result, err := fixture.repo.CleanupEngineRecovery(fixture.ctx, now, 10)
	require.NoError(t, err)
	require.Equal(t, 4, result.Scanned)
	require.Equal(t, 2, result.Cleaned)
	require.Equal(t, 2, result.Failed)
	require.Zero(t, result.Stale)
	require.Zero(t, result.Rescheduled)
	require.ErrorContains(t, result.FirstFailure, "cleanup receipt deadline differs from terminal proof")

	retryAt := float64(now.Add(time.Minute).UnixMilli())
	require.Equal(t, retryAt, fixture.score(forgedMember))
	require.Equal(t, retryAt, fixture.score(scopesMember))
	require.True(t, fixture.receiptExists(forgedExecution))
	require.True(t, fixture.receiptExists(scopesExecution))
	require.Equal(t, "APPROVED", fixture.client.HGet(fixture.ctx, fixture.guards, forgedTransaction.String()).Val())
	require.Equal(t, forgedCoordinator, fixture.client.HGet(fixture.ctx, fixture.protection, forgedTransaction.String()).Val())

	require.False(t, fixture.receiptExists(firstExecution))
	require.False(t, fixture.receiptExists(secondExecution))
	require.False(t, fixture.client.HExists(fixture.ctx, fixture.guards, firstTransaction.String()).Val())
	require.False(t, fixture.client.HExists(fixture.ctx, fixture.guards, secondTransaction.String()).Val())
	require.Zero(t, fixture.client.ZScore(fixture.ctx, fixture.schedule, firstMember).Val())
	require.Zero(t, fixture.client.ZScore(fixture.ctx, fixture.schedule, secondMember).Val())

	result, err = fixture.repo.CleanupEngineRecovery(fixture.ctx, now, 10)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupResult{}, result, "rescheduled entries are not due again within the same instant")

	result, err = fixture.repo.CleanupEngineRecovery(fixture.ctx, now.Add(time.Minute), 10)
	require.NoError(t, err)
	require.Equal(t, 2, result.Scanned)
	require.Equal(t, 2, result.Failed)
	require.Equal(t, float64(now.Add(2*time.Minute).UnixMilli()), fixture.score(forgedMember))
}

func TestIntegrationRecoveryCleanupRescheduleNeverRevivesOrDelaysBackward(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newCleanupIsolationFixture(t, container.Client)
	now := fixture.deadline()
	removed := recoveryCleanupMember(fixture.organizationID, fixture.ledgerID, uuid.MustParse("e1111111-1111-4111-8111-111111111111"))
	later := recoveryCleanupMember(fixture.organizationID, fixture.ledgerID, uuid.MustParse("f1111111-1111-4111-8111-111111111111"))
	laterScore := float64(now.Add(10 * time.Minute).UnixMilli())
	require.NoError(t, fixture.client.ZAdd(fixture.ctx, fixture.schedule, redis.Z{Score: laterScore, Member: later}).Err())

	rescheduled, err := rescheduleRejectedRecoveryCleanup(fixture.ctx, fixture.client, fixture.schedule, removed, now)
	require.NoError(t, err)
	require.False(t, rescheduled)
	require.Equal(t, redis.Nil, fixture.client.ZScore(fixture.ctx, fixture.schedule, removed).Err())

	rescheduled, err = rescheduleRejectedRecoveryCleanup(fixture.ctx, fixture.client, fixture.schedule, later, now)
	require.NoError(t, err)
	require.False(t, rescheduled)
	require.Equal(t, laterScore, fixture.score(later))
}

func TestIntegrationRecoveryCleanupAbortsWithoutWritesOnTransportFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newCleanupIsolationFixture(t, container.Client)
	firstExecution := uuid.MustParse("c1111111-1111-4111-8111-111111111111")
	secondExecution := uuid.MustParse("d1111111-1111-4111-8111-111111111111")
	firstMember := fixture.acknowledged(firstExecution, uuid.MustParse("c2222222-2222-4222-8222-222222222222"))
	secondMember := fixture.acknowledged(secondExecution, uuid.MustParse("d2222222-2222-4222-8222-222222222222"))

	failing := redis.NewClient(&redis.Options{Addr: container.Addr, DB: container.DB})
	t.Cleanup(func() { require.NoError(t, failing.Close()) })
	failing.AddHook(scriptTransportFailure{})
	repo, err := NewConsumerRedis(&recoveryAckClient{client: failing})
	require.NoError(t, err)

	result, err := repo.CleanupEngineRecovery(fixture.ctx, fixture.deadline(), 10)
	require.ErrorIs(t, err, errScriptConnectionReset)
	require.Zero(t, result.Cleaned)
	require.Zero(t, result.Failed)
	require.Equal(t, float64(fixture.deadline().UnixMilli()), fixture.score(firstMember))
	require.Equal(t, float64(fixture.deadline().UnixMilli()), fixture.score(secondMember))
	require.True(t, fixture.receiptExists(firstExecution))
	require.True(t, fixture.receiptExists(secondExecution))
}

func TestIntegrationRecoveryCleanupBacklogReportsDueCountAndOldestDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newCleanupIsolationFixture(t, container.Client)
	now := fixture.deadline()

	backlog, err := fixture.repo.EngineRecoveryCleanupBacklog(fixture.ctx, now)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupBacklog{}, backlog)

	oldest := now.Add(-3 * time.Minute).UnixMilli()
	require.NoError(t, fixture.client.ZAdd(
		fixture.ctx, fixture.schedule,
		redis.Z{Score: float64(oldest), Member: "oldest"},
		redis.Z{Score: float64(now.Add(-time.Minute).UnixMilli()), Member: "middle"},
		redis.Z{Score: float64(now.UnixMilli()), Member: "now"},
		redis.Z{Score: float64(now.Add(time.Millisecond).UnixMilli()), Member: "future"},
	).Err())

	backlog, err = fixture.repo.EngineRecoveryCleanupBacklog(fixture.ctx, now)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupBacklog{Due: 3, OldestDueMs: oldest}, backlog)

	backlog, err = fixture.repo.EngineRecoveryCleanupBacklog(fixture.ctx, now.Add(-4*time.Minute))
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupBacklog{}, backlog, "a schedule with only future entries has no due backlog")
}

var errScriptConnectionReset = errors.New("connection reset by peer")

// scriptTransportFailure fails every script call the way a dropped connection
// does, while schedule reads still succeed.
type scriptTransportFailure struct{}

func (scriptTransportFailure) DialHook(next redis.DialHook) redis.DialHook { return next }

func (scriptTransportFailure) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if strings.HasPrefix(strings.ToLower(cmd.Name()), "eval") {
			cmd.SetErr(errScriptConnectionReset)
			return errScriptConnectionReset
		}

		return next(ctx, cmd)
	}
}

func (scriptTransportFailure) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
