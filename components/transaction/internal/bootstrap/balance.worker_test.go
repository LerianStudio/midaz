package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newTestLogger creates a real logger for tests (no-op by using high log level filtering)
func newTestLogger() libLog.Logger {
	return &libLog.GoLogger{Level: libLog.FatalLevel}
}

// --- Tests for NewBalanceSyncWorker ---

func TestNewBalanceSyncWorker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		maxWorkers         int
		expectedMaxWorkers int
	}{
		{
			name:               "positive max workers",
			maxWorkers:         10,
			expectedMaxWorkers: 10,
		},
		{
			name:               "zero max workers defaults to 5",
			maxWorkers:         0,
			expectedMaxWorkers: 5,
		},
		{
			name:               "negative max workers defaults to 5",
			maxWorkers:         -1,
			expectedMaxWorkers: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := &libRedis.RedisConnection{}
			logger := newTestLogger()
			useCase := &command.UseCase{}

			worker := NewBalanceSyncWorker(conn, logger, useCase, tt.maxWorkers)

			require.NotNil(t, worker)
			assert.Equal(t, tt.expectedMaxWorkers, worker.maxWorkers)
			assert.Equal(t, int64(tt.expectedMaxWorkers), worker.batchSize)
			assert.Equal(t, 600*time.Second, worker.idleWait)
			assert.Same(t, conn, worker.redisConn)
			assert.Same(t, useCase, worker.useCase)
		})
	}
}

// --- Tests for extractIDsFromMember ---

func TestExtractIDsFromMember(t *testing.T) {
	t.Parallel()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	tests := []struct {
		name           string
		member         string
		wantOrgID      uuid.UUID
		wantLedgerID   uuid.UUID
		wantErr        bool
		errMsgContains string
	}{
		{
			name:         "valid key with standard format",
			member:       "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":@account#key",
			wantOrgID:    orgID,
			wantLedgerID: ledgerID,
			wantErr:      false,
		},
		{
			name:         "valid key with default balance key",
			member:       "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":default",
			wantOrgID:    orgID,
			wantLedgerID: ledgerID,
			wantErr:      false,
		},
		{
			name:           "empty string",
			member:         "",
			wantErr:        true,
			errMsgContains: "missing two UUIDs",
		},
		{
			name:           "only one UUID",
			member:         "balance:{transactions}:" + orgID.String() + ":notauuid:@account",
			wantErr:        true,
			errMsgContains: "missing two UUIDs",
		},
		{
			name:           "no UUIDs at all",
			member:         "balance:{transactions}:invalid:also-invalid:@account",
			wantErr:        true,
			errMsgContains: "missing two UUIDs",
		},
		{
			name:           "malformed UUID format",
			member:         "balance:{transactions}:not-a-valid-uuid-format:also-not-valid:@account",
			wantErr:        true,
			errMsgContains: "missing two UUIDs",
		},
		{
			name:         "UUIDs at different positions",
			member:       "prefix:" + orgID.String() + ":middle:" + ledgerID.String() + ":suffix",
			wantOrgID:    orgID,
			wantLedgerID: ledgerID,
			wantErr:      false,
		},
		{
			name:         "UUIDs with no prefix",
			member:       orgID.String() + ":" + ledgerID.String(),
			wantOrgID:    orgID,
			wantLedgerID: ledgerID,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worker := &BalanceSyncWorker{}

			gotOrgID, gotLedgerID, err := worker.extractIDsFromMember(tt.member)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsgContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOrgID, gotOrgID)
			assert.Equal(t, tt.wantLedgerID, gotLedgerID)
		})
	}
}

// --- Tests for waitOrDone ---

func TestWaitOrDone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		duration       time.Duration
		cancelBefore   bool
		expectedResult bool
	}{
		{
			name:           "zero duration returns immediately",
			duration:       0,
			cancelBefore:   false,
			expectedResult: false,
		},
		{
			name:           "negative duration returns immediately",
			duration:       -1 * time.Second,
			cancelBefore:   false,
			expectedResult: false,
		},
		{
			name:           "cancelled context returns true",
			duration:       1 * time.Hour, // Long duration to ensure context cancellation wins
			cancelBefore:   true,
			expectedResult: true,
		},
		{
			name:           "short wait completes normally",
			duration:       1 * time.Millisecond,
			cancelBefore:   false,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelBefore {
				cancel()
			}

			result := waitOrDone(ctx, tt.duration, newTestLogger())

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// --- Tests for waitUntilDue ---

func TestWaitUntilDue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		dueAtUnix      int64
		cancelBefore   bool
		expectedResult bool
	}{
		{
			name:           "past time returns immediately",
			dueAtUnix:      time.Now().Unix() - 100,
			cancelBefore:   false,
			expectedResult: false,
		},
		{
			name:           "current time returns immediately",
			dueAtUnix:      time.Now().Unix(),
			cancelBefore:   false,
			expectedResult: false,
		},
		{
			name:           "cancelled context returns true",
			dueAtUnix:      time.Now().Unix() + 3600, // 1 hour in future
			cancelBefore:   true,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelBefore {
				cancel()
			}

			worker := &BalanceSyncWorker{}
			result := worker.waitUntilDue(ctx, tt.dueAtUnix, newTestLogger())

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// --- Tests for shouldShutdown ---

func TestShouldShutdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		cancelContext  bool
		expectedResult bool
	}{
		{
			name:           "active context returns false",
			cancelContext:  false,
			expectedResult: false,
		},
		{
			name:           "cancelled context returns true",
			cancelContext:  true,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelContext {
				cancel()
			}

			worker := &BalanceSyncWorker{}
			result := worker.shouldShutdown(ctx)

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// --- Tests for processBalancesToExpire ---

func TestProcessBalancesToExpire_NoMembers(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		GetBalanceSyncKeys(gomock.Any(), int64(5)).
		Return([]string{}, nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:     newTestLogger(),
		batchSize:  5,
		maxWorkers: 5,
		useCase:    useCase,
	}

	ctx := context.Background()
	result := worker.processBalancesToExpire(ctx, nil)

	assert.False(t, result, "should return false when no members to process")
}

func TestProcessBalancesToExpire_ErrorGettingKeys(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		GetBalanceSyncKeys(gomock.Any(), int64(5)).
		Return(nil, errors.New("redis connection error")).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:     newTestLogger(),
		batchSize:  5,
		maxWorkers: 5,
		useCase:    useCase,
	}

	ctx := context.Background()
	result := worker.processBalancesToExpire(ctx, nil)

	assert.False(t, result, "should return false on error")
}

func TestProcessBalancesToExpire_RedisNilError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		GetBalanceSyncKeys(gomock.Any(), int64(5)).
		Return(nil, goredis.Nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:     newTestLogger(),
		batchSize:  5,
		maxWorkers: 5,
		useCase:    useCase,
	}

	ctx := context.Background()
	result := worker.processBalancesToExpire(ctx, nil)

	assert.False(t, result, "should return false on redis.Nil (no warning)")
}

func TestProcessBalancesToExpire_ShutdownDuringProcessing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	member := "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":default"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		GetBalanceSyncKeys(gomock.Any(), int64(5)).
		Return([]string{member}, nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:     newTestLogger(),
		batchSize:  5,
		maxWorkers: 5,
		useCase:    useCase,
	}

	// Cancel context before processing
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := worker.processBalancesToExpire(ctx, nil)

	assert.True(t, result, "should return true when shutdown detected")
}

// --- Tests for processBalanceToExpire ---

// mockRedisClient is a stub for redis.UniversalClient used in tests
type mockRedisClient struct {
	goredis.UniversalClient
	ttlResult    time.Duration
	ttlErr       error
	getResult    string
	getErr       error
	zRemResult   int64
	zRemErr      error
	ttlCallCount int
	getCallCount int
}

func (m *mockRedisClient) TTL(ctx context.Context, key string) *goredis.DurationCmd {
	m.ttlCallCount++
	cmd := goredis.NewDurationCmd(ctx, time.Second, "TTL", key)
	if m.ttlErr != nil {
		cmd.SetErr(m.ttlErr)
	} else {
		cmd.SetVal(m.ttlResult)
	}
	return cmd
}

func (m *mockRedisClient) Get(ctx context.Context, key string) *goredis.StringCmd {
	m.getCallCount++
	cmd := goredis.NewStringCmd(ctx, "GET", key)
	if m.getErr != nil {
		cmd.SetErr(m.getErr)
	} else {
		cmd.SetVal(m.getResult)
	}
	return cmd
}

func TestProcessBalanceToExpire_EmptyMember(t *testing.T) {
	t.Parallel()

	worker := &BalanceSyncWorker{
		logger: newTestLogger(),
	}

	mockClient := &mockRedisClient{}

	// Should return early without calling any Redis methods
	worker.processBalanceToExpire(context.Background(), mockClient, "")

	assert.Equal(t, 0, mockClient.ttlCallCount, "TTL should not be called for empty member")
	assert.Equal(t, 0, mockClient.getCallCount, "GET should not be called for empty member")
}

func TestProcessBalanceToExpire_TTLError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	mockClient := &mockRedisClient{
		ttlErr: errors.New("TTL error"),
	}

	// Should return early after TTL error
	worker.processBalanceToExpire(context.Background(), mockClient, "some:key")

	assert.Equal(t, 1, mockClient.ttlCallCount, "TTL should be called once")
	assert.Equal(t, 0, mockClient.getCallCount, "GET should not be called after TTL error")
}

func TestProcessBalanceToExpire_KeyAlreadyGone(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	member := "balance:{transactions}:some-key"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		RemoveBalanceSyncKey(gomock.Any(), member).
		Return(nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	mockClient := &mockRedisClient{
		ttlResult: -2 * time.Second, // Key doesn't exist
	}

	worker.processBalanceToExpire(context.Background(), mockClient, member)

	assert.Equal(t, 1, mockClient.ttlCallCount)
	assert.Equal(t, 0, mockClient.getCallCount, "GET should not be called for gone key")
}

func TestProcessBalanceToExpire_GetNilError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	member := "balance:{transactions}:some-key"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		RemoveBalanceSyncKey(gomock.Any(), member).
		Return(nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	mockClient := &mockRedisClient{
		ttlResult: 100 * time.Second,
		getErr:    goredis.Nil,
	}

	worker.processBalanceToExpire(context.Background(), mockClient, member)

	assert.Equal(t, 1, mockClient.ttlCallCount)
	assert.Equal(t, 1, mockClient.getCallCount)
}

func TestProcessBalanceToExpire_GetError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	mockClient := &mockRedisClient{
		ttlResult: 100 * time.Second,
		getErr:    errors.New("GET error"),
	}

	worker.processBalanceToExpire(context.Background(), mockClient, "some:key")

	assert.Equal(t, 1, mockClient.ttlCallCount)
	assert.Equal(t, 1, mockClient.getCallCount)
}

func TestProcessBalanceToExpire_InvalidMemberFormat(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	member := "invalid:key:format:no-uuids"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		RemoveBalanceSyncKey(gomock.Any(), member).
		Return(nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	validJSON := `{"id":"test-id","alias":"@test"}`
	mockClient := &mockRedisClient{
		ttlResult: 100 * time.Second,
		getResult: validJSON,
	}

	worker.processBalanceToExpire(context.Background(), mockClient, member)

	assert.Equal(t, 1, mockClient.ttlCallCount)
	assert.Equal(t, 1, mockClient.getCallCount)
}

func TestProcessBalanceToExpire_InvalidJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	member := "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":default"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		RemoveBalanceSyncKey(gomock.Any(), member).
		Return(nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	mockClient := &mockRedisClient{
		ttlResult: 100 * time.Second,
		getResult: "invalid-json{",
	}

	worker.processBalanceToExpire(context.Background(), mockClient, member)

	assert.Equal(t, 1, mockClient.ttlCallCount)
	assert.Equal(t, 1, mockClient.getCallCount)
}

func TestProcessBalanceToExpire_SyncError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	member := "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":default"

	balanceRedis := mmodel.BalanceRedis{
		ID:        libCommons.GenerateUUIDv7().String(),
		Alias:     "@test",
		Available: decimal.NewFromInt(1000),
		OnHold:    decimal.Zero,
	}
	balanceJSON, _ := json.Marshal(balanceRedis)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Sync(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(false, errors.New("sync error")).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo:   mockRedisRepo,
		BalanceRepo: mockBalanceRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	mockClient := &mockRedisClient{
		ttlResult: 100 * time.Second,
		getResult: string(balanceJSON),
	}

	// Should not remove key on sync error (allows retry)
	worker.processBalanceToExpire(context.Background(), mockClient, member)

	assert.Equal(t, 1, mockClient.ttlCallCount)
	assert.Equal(t, 1, mockClient.getCallCount)
}

func TestProcessBalanceToExpire_SyncSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	member := "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":default"

	balanceRedis := mmodel.BalanceRedis{
		ID:        libCommons.GenerateUUIDv7().String(),
		Alias:     "@test",
		Available: decimal.NewFromInt(1000),
		OnHold:    decimal.Zero,
	}
	balanceJSON, _ := json.Marshal(balanceRedis)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Sync(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(true, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		RemoveBalanceSyncKey(gomock.Any(), member).
		Return(nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo:   mockRedisRepo,
		BalanceRepo: mockBalanceRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	mockClient := &mockRedisClient{
		ttlResult: 100 * time.Second,
		getResult: string(balanceJSON),
	}

	worker.processBalanceToExpire(context.Background(), mockClient, member)

	assert.Equal(t, 1, mockClient.ttlCallCount)
	assert.Equal(t, 1, mockClient.getCallCount)
}

func TestProcessBalanceToExpire_SyncSkipped(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	member := "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":default"

	balanceRedis := mmodel.BalanceRedis{
		ID:        libCommons.GenerateUUIDv7().String(),
		Alias:     "@test",
		Available: decimal.NewFromInt(1000),
		OnHold:    decimal.Zero,
	}
	balanceJSON, _ := json.Marshal(balanceRedis)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	// Sync returns false (balance is newer in DB, skipped)
	mockBalanceRepo.EXPECT().
		Sync(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(false, nil).
		Times(1)

	// Key should still be removed even when sync is skipped
	mockRedisRepo.EXPECT().
		RemoveBalanceSyncKey(gomock.Any(), member).
		Return(nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo:   mockRedisRepo,
		BalanceRepo: mockBalanceRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	mockClient := &mockRedisClient{
		ttlResult: 100 * time.Second,
		getResult: string(balanceJSON),
	}

	worker.processBalanceToExpire(context.Background(), mockClient, member)

	assert.Equal(t, 1, mockClient.ttlCallCount)
	assert.Equal(t, 1, mockClient.getCallCount)
}

// --- Tests for TTL sentinel values ---

func TestProcessBalanceToExpire_TTLSentinelNegativeTwo(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	member := "balance:{transactions}:some-key"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		RemoveBalanceSyncKey(gomock.Any(), member).
		Return(nil).
		Times(1)

	useCase := &command.UseCase{
		RedisRepo: mockRedisRepo,
	}

	worker := &BalanceSyncWorker{
		logger:  newTestLogger(),
		useCase: useCase,
	}

	// TTL returns -2 (integer sentinel for key doesn't exist)
	mockClient := &mockRedisClient{
		ttlResult: -2,
	}

	worker.processBalanceToExpire(context.Background(), mockClient, member)

	assert.Equal(t, 1, mockClient.ttlCallCount)
	assert.Equal(t, 0, mockClient.getCallCount, "GET should not be called for TTL=-2")
}

// --- Property-based test for extractIDsFromMember ---

func TestProperty_ExtractIDsFromMember_ValidKeys(t *testing.T) {
	t.Parallel()

	// Property: For any valid org/ledger UUID pair in a properly formatted key,
	// extractIDsFromMember should return those exact UUIDs
	testCases := []struct {
		prefix string
		suffix string
	}{
		{"balance:{transactions}:", ":default"},
		{"balance:{transactions}:", ":@account#key"},
		{"prefix:", ":suffix"},
		{"", ""},
		{"a:b:c:", ":d:e:f"},
	}

	for i := 0; i < 10; i++ {
		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()

		for _, tc := range testCases {
			t.Run(tc.prefix+tc.suffix, func(t *testing.T) {
				t.Parallel()

				member := tc.prefix + orgID.String() + ":" + ledgerID.String() + tc.suffix
				worker := &BalanceSyncWorker{}

				gotOrg, gotLedger, err := worker.extractIDsFromMember(member)

				require.NoError(t, err)
				assert.Equal(t, orgID, gotOrg)
				assert.Equal(t, ledgerID, gotLedger)
			})
		}
	}
}
