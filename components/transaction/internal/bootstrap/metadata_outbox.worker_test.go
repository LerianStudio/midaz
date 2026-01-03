package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/LerianStudio/midaz/v3/pkg/mretry"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// Mock implementations for testing
// These embed the interface to satisfy nil checks without behavior.
type mockOutboxRepo struct {
	outbox.Repository
}

type mockMetadataRepo struct {
	mongodb.Repository
}

func TestNewMetadataOutboxWorker_PanicsOnNilLogger(t *testing.T) {
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(nil, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 7)
	}, "Expected panic on nil Logger")
}

func TestNewMetadataOutboxWorker_PanicsOnNilOutboxRepo(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(mockLogger, nil, mockMetadata, mockPostgres, mockMongo, 5, 7)
	}, "Expected panic on nil OutboxRepository")
}

func TestNewMetadataOutboxWorker_PanicsOnNilMetadataRepo(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(mockLogger, mockOutbox, nil, mockPostgres, mockMongo, 5, 7)
	}, "Expected panic on nil MetadataRepository")
}

func TestNewMetadataOutboxWorker_PanicsOnNilPostgresConn(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, nil, mockMongo, 5, 7)
	}, "Expected panic on nil PostgresConnection")
}

func TestNewMetadataOutboxWorker_SucceedsWithValidDependencies(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.NotPanics(t, func() {
		worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 7)
		assert.NotNil(t, worker)
		assert.Equal(t, 5, worker.maxWorkers)
		assert.Equal(t, 7, worker.retentionDays)
	})
}

func TestNewMetadataOutboxWorker_DefaultsMaxWorkersWhenZero(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 0, 7)
	assert.Equal(t, 5, worker.maxWorkers)
}

func TestNewMetadataOutboxWorker_DefaultsRetentionDaysWhenZero(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 0)
	assert.Equal(t, 7, worker.retentionDays)
}

func TestCalculateBackoff_ExponentialGrowth(t *testing.T) {
	// Create a worker with default config for testing
	worker := createTestWorker()

	// Test that backoff increases exponentially
	backoff1 := worker.calculateBackoff(1)
	backoff2 := worker.calculateBackoff(2)
	backoff3 := worker.calculateBackoff(3)

	// Due to jitter, we keep a relaxed upper-bound tolerance for attempt 1 to avoid flakiness.
	// The base values are: 1s, 2s, 4s (before jitter)
	assert.GreaterOrEqual(t, backoff1.Seconds(), 1.0, "attempt 1 should be at least 1s")
	assert.LessOrEqual(t, backoff1.Seconds(), 1.5, "attempt 1 should be at most 1.5s (with jitter tolerance)")

	// Attempt 2 should be roughly 2x attempt 1 (before jitter)
	assert.GreaterOrEqual(t, backoff2.Seconds(), 2.0, "attempt 2 should be at least 2s")

	// Attempt 3 should be roughly 2x attempt 2 (before jitter)
	assert.GreaterOrEqual(t, backoff3.Seconds(), 4.0, "attempt 3 should be at least 4s")
}

func TestCalculateBackoff_CapsAtMax(t *testing.T) {
	// Create a worker with default config for testing
	worker := createTestWorker()

	// Test that backoff caps at 30 minutes
	backoff := worker.calculateBackoff(100) // Very high attempt number
	assert.LessOrEqual(t, backoff, mretry.DefaultMaxBackoff, "backoff should be capped at 30 minutes")
}

func TestCalculateBackoff_ZeroAttempt(t *testing.T) {
	// Create a worker with default config for testing
	worker := createTestWorker()

	// Test that zero attempt returns initial backoff
	backoff := worker.calculateBackoff(0)
	assert.Equal(t, mretry.DefaultInitialBackoff, backoff, "attempt 0 should return initial backoff")
}

func TestCalculateBackoff_WithCustomConfig(t *testing.T) {
	// Create a worker with custom config
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	customConfig := mretry.DefaultMetadataOutboxConfig().WithInitialBackoff(2 * mretry.DefaultInitialBackoff)

	worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 7, customConfig)

	// Test that custom initial backoff is used
	backoff := worker.calculateBackoff(0)
	assert.Equal(t, 2*mretry.DefaultInitialBackoff, backoff, "attempt 0 should return custom initial backoff")
}

// createTestWorker creates a MetadataOutboxWorker with default config for testing.
func createTestWorker() *MetadataOutboxWorker {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	return NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 7)
}

// TestHandleProcessingError_DLQRouting verifies that when an entry has exhausted
// all retries (RetryCount=9, MaxRetries=10), handleProcessingError calls MarkDLQ
// instead of MarkFailed.
func TestHandleProcessingError_DLQRouting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock repository
	mockRepo := outbox.NewMockRepository(ctrl)

	// Create worker with the mock
	mockLogger := &libLog.NoneLogger{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	worker := &MetadataOutboxWorker{
		logger:        mockLogger,
		outboxRepo:    mockRepo,
		metadataRepo:  mockMetadata,
		postgresConn:  mockPostgres,
		mongoConn:     mockMongo,
		maxWorkers:    5,
		retentionDays: 7,
		retryConfig:   mretry.DefaultMetadataOutboxConfig(),
	}

	// Create entry at final retry attempt (RetryCount=9, MaxRetries=10)
	// After this failure, newRetryCount = 10, which >= 10, so should go to DLQ
	entryID := uuid.New()
	entry := &outbox.MetadataOutbox{
		ID:         entryID,
		EntityID:   "test-entity-123",
		EntityType: "Transaction",
		Metadata:   map[string]any{"key": "value"},
		Status:     outbox.StatusProcessing,
		RetryCount: 9,  // 9th retry (0-indexed), so this is the 10th attempt
		MaxRetries: 10, // Max is 10
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Expect MarkDLQ to be called (NOT MarkFailed)
	mockRepo.EXPECT().
		MarkDLQ(gomock.Any(), entryID.String(), gomock.Any()).
		Return(nil).
		Times(1)

	// MarkFailed should NOT be called
	// (gomock will fail the test if MarkFailed is called unexpectedly)

	// Setup context with tracking
	ctx := context.Background()
	correlationID := libCommons.GenerateUUIDv7().String()
	ctx = libCommons.ContextWithHeaderID(ctx, correlationID)
	ctx = libCommons.ContextWithLogger(ctx, mockLogger)

	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	_, span := tracer.Start(ctx, "test")
	defer span.End()

	// Call handleProcessingError with a simulated error
	processingErr := errors.New("simulated processing failure")
	worker.handleProcessingError(ctx, entry, processingErr, mockLogger, &span)
}

// TestHandleProcessingError_MarkFailed verifies that when an entry still has retries
// remaining (RetryCount=5, MaxRetries=10), handleProcessingError calls MarkFailed
// instead of MarkDLQ.
func TestHandleProcessingError_MarkFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock repository
	mockRepo := outbox.NewMockRepository(ctrl)

	// Create worker with the mock
	mockLogger := &libLog.NoneLogger{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	worker := &MetadataOutboxWorker{
		logger:        mockLogger,
		outboxRepo:    mockRepo,
		metadataRepo:  mockMetadata,
		postgresConn:  mockPostgres,
		mongoConn:     mockMongo,
		maxWorkers:    5,
		retentionDays: 7,
		retryConfig:   mretry.DefaultMetadataOutboxConfig(),
	}

	// Create entry with retries remaining (RetryCount=5, MaxRetries=10)
	// After this failure, newRetryCount = 6, which < 10, so should retry
	entryID := uuid.New()
	entry := &outbox.MetadataOutbox{
		ID:         entryID,
		EntityID:   "test-entity-456",
		EntityType: "Transaction",
		Metadata:   map[string]any{"key": "value"},
		Status:     outbox.StatusProcessing,
		RetryCount: 5, // Has more retries available
		MaxRetries: 10,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Expect MarkFailed to be called (NOT MarkDLQ)
	mockRepo.EXPECT().
		MarkFailed(gomock.Any(), entryID.String(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// MarkDLQ should NOT be called
	// (gomock will fail the test if MarkDLQ is called unexpectedly)

	// Setup context with tracking
	ctx := context.Background()
	correlationID := libCommons.GenerateUUIDv7().String()
	ctx = libCommons.ContextWithHeaderID(ctx, correlationID)
	ctx = libCommons.ContextWithLogger(ctx, mockLogger)

	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	_, span := tracer.Start(ctx, "test")
	defer span.End()

	// Call handleProcessingError with a simulated error
	processingErr := errors.New("simulated processing failure")
	worker.handleProcessingError(ctx, entry, processingErr, mockLogger, &span)
}
