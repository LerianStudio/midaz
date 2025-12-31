// Package bootstrap provides initialization and consumer implementation for the transaction service,
// including Redis queue consumer for handling transaction messages.
package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	postgreTransaction "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CronTimeToRun is the interval between Redis queue processing cycles.
const CronTimeToRun = 30 * time.Minute

const (
	// MessageTimeOfLife is the time-to-live duration (in seconds) for messages in the Redis queue before expiration.
	MessageTimeOfLife = 30
	// MaxWorkers is the maximum number of concurrent workers processing messages from the Redis queue.
	MaxWorkers               = 100
	messageProcessingTimeout = 30
)

// ErrPanicRecovered is returned when a panic is recovered during message processing
var ErrPanicRecovered = errors.New("panic recovered")

// RedisQueueConsumer processes transaction messages from the Redis backup queue.
type RedisQueueConsumer struct {
	Logger             libLog.Logger
	TransactionHandler in.TransactionHandler
}

// NewRedisQueueConsumer creates a new RedisQueueConsumer with the provided logger and handler.
func NewRedisQueueConsumer(logger libLog.Logger, handler in.TransactionHandler) *RedisQueueConsumer {
	assert.NotNil(logger, "Logger required for RedisQueueConsumer")

	return &RedisQueueConsumer{
		Logger:             logger,
		TransactionHandler: handler,
	}
}

// Run starts the Redis queue consumer loop that periodically processes backup messages.
// It blocks until the context is cancelled or an interrupt signal is received.
func (r *RedisQueueConsumer) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(CronTimeToRun)
	defer ticker.Stop()

	r.Logger.Info("RedisQueueConsumer started")

	for {
		select {
		case <-ctx.Done():
			r.Logger.Info("RedisQueueConsumer: shutting down...")
			return nil

		case <-ticker.C:
			r.readMessagesAndProcess(ctx)
		}
	}
}

//nolint:dogsled
func (r *RedisQueueConsumer) readMessagesAndProcess(ctx context.Context) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.consumer.read_messages_from_queue")
	defer span.End()

	r.Logger.Infof("Init cron to read messages from redis...")

	messages, err := r.TransactionHandler.Command.RedisRepo.ReadAllMessagesFromQueue(ctx)
	if err != nil {
		r.Logger.Errorf("Err to read messages from redis: %v", err)
		return
	}

	r.Logger.Infof("Total of read %d messages from queue", len(messages))

	if len(messages) == 0 {
		return
	}

	totalMessagesLessThanOneHour := r.processMessages(ctx, tracer, messages)

	r.Logger.Infof("Total of messages under %d minute(s): %d", MessageTimeOfLife, totalMessagesLessThanOneHour)
	r.Logger.Infof("Finished processing total of %d eligible messages", len(messages)-totalMessagesLessThanOneHour)
}

// processMessages processes all messages from Redis queue
func (r *RedisQueueConsumer) processMessages(ctx context.Context, tracer trace.Tracer, messages map[string]string) int {
	sem := make(chan struct{}, MaxWorkers)

	var wg sync.WaitGroup

	totalMessagesLessThanOneHour := 0

Outer:
	for key, message := range messages {
		if ctx.Err() != nil {
			r.Logger.Warnf("Shutdown in progress: skipping remaining messages")
			break Outer
		}

		transaction, skip, err := r.unmarshalAndValidateMessage(message)
		if err != nil {
			r.Logger.Warnf("Error unmarshalling message from Redis: %v", err)
			continue
		}

		r.validateTransactionKey(key, transaction)

		if skip {
			totalMessagesLessThanOneHour++
			continue
		}

		sem <- struct{}{}

		wg.Add(1)

		mruntime.SafeGoWithContextAndComponent(ctx, r.Logger, "transaction", "redis_consumer_process_message", mruntime.KeepRunning, func(ctx context.Context) {
			r.processMessage(ctx, tracer, sem, &wg, key, transaction)
		})
	}

	wg.Wait()

	return totalMessagesLessThanOneHour
}

// unmarshalAndValidateMessage unmarshals and validates message TTL
func (r *RedisQueueConsumer) unmarshalAndValidateMessage(message string) (mmodel.TransactionRedisQueue, bool, error) {
	var transaction mmodel.TransactionRedisQueue
	if err := json.Unmarshal([]byte(message), &transaction); err != nil {
		return mmodel.TransactionRedisQueue{}, false, pkg.ValidateInternalError(err, "TransactionRedisQueue")
	}

	// Post-deserialization validation: catch corrupted messages early
	assert.NotEmpty(transaction.HeaderID,
		"header_id must not be empty in redis queue message",
		"transaction_id", transaction.TransactionID)
	assert.That(transaction.TransactionID != uuid.Nil,
		"transaction_id must not be nil UUID in redis queue message",
		"header_id", transaction.HeaderID)
	assert.That(transaction.OrganizationID != uuid.Nil,
		"organization_id must not be nil UUID",
		"transaction_id", transaction.TransactionID,
		"header_id", transaction.HeaderID)
	assert.That(transaction.LedgerID != uuid.Nil,
		"ledger_id must not be nil UUID",
		"transaction_id", transaction.TransactionID,
		"header_id", transaction.HeaderID)
	assert.NotNil(transaction.Validate,
		"validate responses must not be nil",
		"transaction_id", transaction.TransactionID,
		"header_id", transaction.HeaderID)
	assert.That(assert.ValidTransactionStatus(transaction.TransactionStatus),
		"invalid transaction status in redis queue message",
		"status", transaction.TransactionStatus,
		"transaction_id", transaction.TransactionID)
	assert.That(assert.DateNotInFuture(transaction.TransactionDate),
		"transaction_date cannot be in the future",
		"transaction_id", transaction.TransactionID,
		"transaction_date", transaction.TransactionDate)
	assert.That(assert.DateNotInFuture(transaction.TTL),
		"ttl cannot be in the future",
		"transaction_id", transaction.TransactionID,
		"ttl", transaction.TTL)
	assert.NotEmpty(transaction.ParserDSL.Send.Asset,
		"parser_dsl.send.asset must not be empty",
		"transaction_id", transaction.TransactionID)
	assert.That(transaction.ParserDSL.Send.Value.IsPositive(),
		"parser_dsl.send.value must be positive",
		"transaction_id", transaction.TransactionID,
		"value", transaction.ParserDSL.Send.Value)

	if transaction.TransactionStatus != constant.NOTED {
		assert.That(len(transaction.Balances) > 0,
			"balances must not be empty for non-NOTED transactions",
			"transaction_id", transaction.TransactionID,
			"status", transaction.TransactionStatus)
	}

	skip := transaction.TTL.Unix() > time.Now().Add(-MessageTimeOfLife*time.Minute).Unix()

	return transaction, skip, nil
}

// validateTransactionKey ensures the redis key matches the transaction identifiers.
func (r *RedisQueueConsumer) validateTransactionKey(key string, transaction mmodel.TransactionRedisQueue) {
	parts := strings.Split(key, ":")
	assert.That(len(parts) == 5,
		"transaction redis key has invalid format",
		"key", key,
		"parts", len(parts))
	assert.That(parts[0] == "transaction",
		"transaction redis key must start with 'transaction'",
		"key", key)
	assert.That(parts[1] == "{transactions}",
		"transaction redis key must include '{transactions}' context",
		"key", key)

	organizationID := parts[2]
	ledgerID := parts[3]
	transactionID := parts[4]

	assert.That(assert.ValidUUID(organizationID),
		"transaction redis key organization_id must be valid UUID",
		"key", key)
	assert.That(assert.ValidUUID(ledgerID),
		"transaction redis key ledger_id must be valid UUID",
		"key", key)
	assert.That(assert.ValidUUID(transactionID),
		"transaction redis key transaction_id must be valid UUID",
		"key", key)

	assert.That(organizationID == transaction.OrganizationID.String(),
		"transaction redis key organization_id mismatch",
		"key", key,
		"organization_id", transaction.OrganizationID)
	assert.That(ledgerID == transaction.LedgerID.String(),
		"transaction redis key ledger_id mismatch",
		"key", key,
		"ledger_id", transaction.LedgerID)
	assert.That(transactionID == transaction.TransactionID.String(),
		"transaction redis key transaction_id mismatch",
		"key", key,
		"transaction_id", transaction.TransactionID)
}

// processMessage processes a single message in a goroutine
func (r *RedisQueueConsumer) processMessage(ctx context.Context, tracer trace.Tracer, sem chan struct{}, wg *sync.WaitGroup, key string, m mmodel.TransactionRedisQueue) {
	defer func() {
		<-sem
		wg.Done()
	}()

	msgCtx, cancel := context.WithTimeout(ctx, messageProcessingTimeout*time.Second)
	defer cancel()

	logger := r.Logger.WithFields(
		libConstants.HeaderID, m.HeaderID,
	).WithDefaultMessageTemplate(m.HeaderID + " | ")

	ctxWithLogger := libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(msgCtx, m.HeaderID),
		logger,
	)

	msgCtxWithSpan, msgSpan := tracer.Start(ctxWithLogger, "redis.consumer.process_message")
	defer msgSpan.End()

	// Panic recovery with span event recording
	// Records custom span fields for debugging, then re-panics so the outer mruntime.SafeGo*
	// wrapper can observe the panic for metrics and error reporting.
	// TODO(review): Consider implementing dead-letter queue for messages that cause repeated panics
	// to avoid infinite processing loops. (reported by business-logic-reviewer on 2025-12-13, severity: Medium)
	// TRACKING: Deferred to separate feature - requires Redis Streams DLQ design.
	// See: https://redis.io/docs/data-types/streams-tutorial/#consumer-groups
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()
			msgSpan.AddEvent("panic.recovered", trace.WithAttributes(
				attribute.String("panic.value", fmt.Sprintf("%v", rec)),
				attribute.String("panic.stack", string(stack)),
				attribute.String("message.key", key),
				attribute.String("header_id", m.HeaderID),
			))
			libOpentelemetry.HandleSpanError(&msgSpan, "Panic during Redis message processing", r.panicAsError(rec))
			// Logger.Errorf removed - outer mruntime.SafeGo* wrapper logs with full context
			// Re-panic so outer mruntime.SafeGo* wrapper can record metrics and invoke error reporter
			//nolint:panicguardwarn // Intentional re-panic for observability chain
			panic(rec)
		}
	}()

	if r.shouldCancelProcessing(msgCtxWithSpan, logger) {
		return
	}

	balances := r.convertToBalances(m)
	tran := r.buildTransaction(m)

	operations, err := r.buildOperationsForTransaction(msgCtxWithSpan, &msgSpan, logger, balances, m, tran)
	if err != nil {
		return
	}

	tran.Source = m.Validate.Sources
	tran.Destination = m.Validate.Destinations
	tran.Operations = operations

	if err := r.sendTransactionToQueue(msgCtxWithSpan, &msgSpan, logger, key, m, balances, tran); err != nil {
		return
	}

	logger.Infof("Transaction message processed: %s", key)
}

// shouldCancelProcessing checks if processing should be cancelled
func (r *RedisQueueConsumer) shouldCancelProcessing(ctx context.Context, logger libLog.Logger) bool {
	select {
	case <-ctx.Done():
		logger.Warn("Transaction message processing cancelled due to shutdown/timeout")
		return true
	default:
		return false
	}
}

// convertToBalances converts Redis balances to model balances
func (r *RedisQueueConsumer) convertToBalances(m mmodel.TransactionRedisQueue) []*mmodel.Balance {
	balances := make([]*mmodel.Balance, 0, len(m.Balances))
	for _, balance := range m.Balances {
		balances = append(balances, &mmodel.Balance{
			Alias:          balance.Alias,
			ID:             balance.ID,
			AccountID:      balance.AccountID,
			Available:      balance.Available,
			OnHold:         balance.OnHold,
			Version:        balance.Version,
			AccountType:    balance.AccountType,
			AllowSending:   balance.AllowSending == 1,
			AllowReceiving: balance.AllowReceiving == 1,
			AssetCode:      balance.AssetCode,
			Key:            balance.Key,
			OrganizationID: m.OrganizationID.String(),
			LedgerID:       m.LedgerID.String(),
		})
	}

	return balances
}

// buildTransaction builds a transaction from queue message
func (r *RedisQueueConsumer) buildTransaction(m mmodel.TransactionRedisQueue) *postgreTransaction.Transaction {
	var parentTransactionID *string

	return &postgreTransaction.Transaction{
		ID:                       m.TransactionID.String(),
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           m.OrganizationID.String(),
		LedgerID:                 m.LedgerID.String(),
		Description:              m.ParserDSL.Description,
		Amount:                   &m.ParserDSL.Send.Value,
		AssetCode:                m.ParserDSL.Send.Asset,
		ChartOfAccountsGroupName: m.ParserDSL.ChartOfAccountsGroupName,
		CreatedAt:                m.TransactionDate,
		UpdatedAt:                time.Now(),
		Route:                    m.ParserDSL.Route,
		Metadata:                 m.ParserDSL.Metadata,
		Status: postgreTransaction.Status{
			Code:        m.TransactionStatus,
			Description: &m.TransactionStatus,
		},
	}
}

// buildOperationsForTransaction builds operations for a transaction
func (r *RedisQueueConsumer) buildOperationsForTransaction(ctx context.Context, span *trace.Span, logger libLog.Logger, balances []*mmodel.Balance, m mmodel.TransactionRedisQueue, tran *postgreTransaction.Transaction) ([]*operation.Operation, error) {
	fromTo := m.ParserDSL.Send.Source.From
	fromTo = append(fromTo, m.ParserDSL.Send.Distribute.To...)

	operations, _, err := r.TransactionHandler.BuildOperations(
		ctx, balances, fromTo, m.ParserDSL, *tran, m.Validate, m.TransactionDate, m.TransactionStatus == constant.NOTED,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)
		logger.Errorf("Failed to validate balance: %v", err.Error())

		return nil, pkg.ValidateInternalError(err, "RedisConsumer")
	}

	return operations, nil
}

// sendTransactionToQueue sends transaction to the execution queue
func (r *RedisQueueConsumer) sendTransactionToQueue(ctx context.Context, span *trace.Span, logger libLog.Logger, key string, m mmodel.TransactionRedisQueue, balances []*mmodel.Balance, tran *postgreTransaction.Transaction) error {
	if err := r.TransactionHandler.Command.SendBTOExecuteAsync(
		ctx, m.OrganizationID, m.LedgerID, &m.ParserDSL, m.Validate, balances, tran,
	); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed sending message to queue", err)
		logger.Errorf("Failed sending message: %s to queue: %v", key, err.Error())

		return pkg.ValidateInternalError(err, "RedisConsumer")
	}

	return nil
}

// panicAsError converts a recovered panic value to an error
func (r *RedisQueueConsumer) panicAsError(rec any) error {
	var panicErr error

	if err, ok := rec.(error); ok {
		panicErr = fmt.Errorf("%w: %w", ErrPanicRecovered, err)
	} else {
		panicErr = fmt.Errorf("%w: %s", ErrPanicRecovered, fmt.Sprint(rec))
	}

	return pkg.ValidateInternalError(panicErr, "RedisConsumer")
}
