package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// maxRetries is the maximum number of delivery attempts (including first delivery)
// before rejecting as a poison message to prevent infinite retry loops.
const maxRetries = 4

// retryCountHeader is the custom header used to track retry attempts.
// We use a custom header instead of RabbitMQ's x-death because x-death
// is only populated when messages go through a Dead Letter Exchange, not on
// direct Nack requeues. Our custom header provides accurate retry tracking.
const retryCountHeader = "x-midaz-retry-count"

// dlqSuffix is the suffix appended to queue names to form Dead Letter Queue names.
// Example: "transactions" -> "transactions.dlq"
// Messages that exceed maxRetries are routed to DLQ for post-mortem analysis
// and manual replay during chaos scenarios or incident investigation.
const dlqSuffix = ".dlq"

// buildDLQName creates the Dead Letter Queue name for a given queue.
// TODO(review): Consider adding validation for empty queueName - returns ".dlq" which may indicate programming error (reported by code-reviewer and business-logic-reviewer on 2025-12-14, severity: Low)
func buildDLQName(queueName string) string {
	return queueName + dlqSuffix
}

// Retry backoff delays - designed to span ~50 seconds total
// to cover typical PostgreSQL restart times (10-30s)
var retryBackoffDelays = []time.Duration{
	0,                // Retry 1 (attempt 2): immediate
	5 * time.Second,  // Retry 2 (attempt 3): 5s delay
	15 * time.Second, // Retry 3 (attempt 4): 15s delay
	30 * time.Second, // Retry 4 (attempt 5): 30s delay
}

// calculateRetryBackoff returns the delay to wait before the given retry attempt.
// retryCount is 1-based (1 = first retry, which is attempt 2 overall).
// Returns 0 for first retry (immediate), then 5s, 15s, 30s.
func calculateRetryBackoff(retryCount int) time.Duration {
	if retryCount <= 0 {
		return 0
	}
	if retryCount > len(retryBackoffDelays) {
		return retryBackoffDelays[len(retryBackoffDelays)-1]
	}
	return retryBackoffDelays[retryCount-1]
}

// ConsumerRepository provides an interface for Consumer related to rabbitmq.
// It defines methods for registering queues and running consumers.
type ConsumerRepository interface {
	Register(queueName string, handler QueueHandlerFunc)
	RunConsumers() error
}

// getRetryCount extracts the retry count from our custom retry tracking header.
// This header is set and incremented by this consumer on each republish.
// Returns 0 for first delivery (header not present).
func getRetryCount(headers amqp.Table) int {
	if val, ok := headers[retryCountHeader].(int32); ok {
		return int(val)
	}
	// Check int64 for compatibility
	if val, ok := headers[retryCountHeader].(int64); ok {
		return int(val)
	}

	return 0
}

// copyHeaders creates a deep copy of amqp.Table for safe header modification
func copyHeaders(src amqp.Table) amqp.Table {
	if src == nil {
		return amqp.Table{}
	}

	dst := make(amqp.Table, len(src))
	for k, v := range src {
		dst[k] = v
	}

	return dst
}

// safeHeadersAllowlist defines headers safe to propagate to DLQ and retry messages.
// Only these headers are copied to prevent sensitive data leakage (CWE-200).
// Headers NOT in this list (auth tokens, PII, internal paths) are filtered out.
var safeHeadersAllowlist = map[string]bool{
	"x-correlation-id":    true,
	"x-midaz-header-id":   true,
	"content-type":        true,
	retryCountHeader:      true, // x-midaz-retry-count
	libConstants.HeaderID: true, // x-midaz-id
}

// copyHeadersSafe copies only allowlisted headers to prevent sensitive data propagation.
// This is a security measure to filter out auth tokens, PII, and internal paths (CWE-200).
func copyHeadersSafe(src amqp.Table) amqp.Table {
	if src == nil {
		return amqp.Table{}
	}

	dst := make(amqp.Table)
	for k, v := range src {
		if safeHeadersAllowlist[k] {
			dst[k] = v
		}
	}

	return dst
}

// sanitizeErrorForDLQ returns a safe error description for DLQ headers without sensitive details.
// This prevents information disclosure (CWE-209) by mapping errors to generic categories
// instead of exposing SQL queries, internal paths, user IDs, or stack traces.
func sanitizeErrorForDLQ(err error) string {
	if err == nil {
		return "unknown_error"
	}

	// For typed business errors, use generic descriptions
	if errors.Is(err, constant.ErrStaleBalanceUpdateSkipped) {
		return "stale_balance_version_conflict"
	}

	// Check common error patterns and return generic categories
	errorMsg := err.Error()
	switch {
	case strings.Contains(errorMsg, "connection"):
		return "database_connection_error"
	case strings.Contains(errorMsg, "timeout"):
		return "operation_timeout"
	case strings.Contains(errorMsg, "validation"):
		return "validation_error"
	case strings.Contains(errorMsg, "not found"):
		return "resource_not_found"
	case strings.Contains(errorMsg, "duplicate"):
		return "duplicate_entry"
	case strings.Contains(errorMsg, "permission") || strings.Contains(errorMsg, "unauthorized"):
		return "authorization_error"
	default:
		return "processing_error"
	}
}

// sanitizePanicForDLQ returns a safe panic description for DLQ headers.
// Similar to sanitizeErrorForDLQ but handles panic values which may contain stack traces.
func sanitizePanicForDLQ(panicValue any) string {
	if panicValue == nil {
		return "unknown_panic"
	}

	// Convert panic value to string for pattern matching
	panicStr := fmt.Sprintf("%v", panicValue)

	// Check common panic patterns and return generic categories
	switch {
	case strings.Contains(panicStr, "nil pointer"):
		return "nil_pointer_dereference"
	case strings.Contains(panicStr, "index out of range"):
		return "index_out_of_bounds"
	case strings.Contains(panicStr, "slice bounds"):
		return "slice_bounds_error"
	case strings.Contains(panicStr, "map"):
		return "map_access_error"
	case strings.Contains(panicStr, "channel"):
		return "channel_operation_error"
	case strings.Contains(panicStr, "runtime error"):
		return "runtime_error"
	default:
		return "unhandled_panic"
	}
}

// safeIncrementRetryCount increments retry count with int32 overflow protection.
// Returns math.MaxInt32 if increment would overflow.
func safeIncrementRetryCount(retryCount int) int32 {
	// Check for overflow BEFORE incrementing to satisfy gosec G115
	if retryCount >= math.MaxInt32 {
		return math.MaxInt32
	}

	//nolint:gosec // G115: Safe after bounds check above ensures retryCount+1 <= MaxInt32
	return int32(retryCount + 1)
}

// dlqPublishParams holds parameters for publishing to a Dead Letter Queue.
// Used by publishToDLQShared to consolidate DLQ publishing logic.
type dlqPublishParams struct {
	conn          *libRabbitmq.RabbitMQConnection
	dlqName       string
	msg           *amqp.Delivery
	headers       amqp.Table
	logger        libLog.Logger
	workerID      int
	originalQueue string // Original queue name for logging
	retryCount    int    // Retry count for logging
	reason        string // Sanitized reason for logging
}

// publishToDLQShared publishes a message to the Dead Letter Queue with publisher confirms.
// This is shared logic used by both businessErrorContext and panicRecoveryContext.
// Uses publisher confirms to prevent data loss - without confirms, broker crash
// after Publish() but before persistence causes message loss (original already Ack'd).
// NOTE: Single attempt only (no retry loop) - tradeoff to avoid blocking consumer worker.
func publishToDLQShared(params *dlqPublishParams) error {
	ch, err := params.conn.Connection.Channel()
	if err != nil {
		return fmt.Errorf("failed to get channel for DLQ: %w", err)
	}
	defer ch.Close()

	// Declare DLQ if it doesn't exist (idempotent)
	_, err = ch.QueueDeclare(
		params.dlqName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare DLQ: %w", err)
	}

	// Enable publisher confirm mode to ensure message persistence
	if err = ch.Confirm(false); err != nil {
		return fmt.Errorf("failed to enable confirm mode for DLQ: %w", err)
	}

	// Create channel to receive publish confirmation (buffer size 1 is sufficient)
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	err = ch.Publish(
		"",             // exchange (default)
		params.dlqName, // routing key (queue name)
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			Headers:      params.headers,
			Body:         params.msg.Body,
			ContentType:  params.msg.ContentType,
			DeliveryMode: amqp.Persistent,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish to DLQ: %w", err)
	}

	// Wait for broker confirmation with timeout
	// This is critical - without confirmation, message may be lost if broker crashes
	select {
	case confirmation, ok := <-confirms:
		if !ok {
			return fmt.Errorf("failed to publish to DLQ: %w", ErrConfirmChannelClosed)
		}

		if confirmation.Ack {
			params.logger.Warnf("DLQ_PUBLISH_SUCCESS: worker=%d, delivery_tag=%d, dlq=%s, queue=%s, retry_count=%d, reason=%s",
				params.workerID, confirmation.DeliveryTag, params.dlqName, params.originalQueue, params.retryCount, params.reason)

			return nil
		}

		return fmt.Errorf("failed to publish to DLQ: %w: delivery tag %d", ErrBrokerNack, confirmation.DeliveryTag)

	case <-time.After(publishConfirmTimeout):
		return fmt.Errorf("failed to publish to DLQ: %w: after %v", ErrConfirmTimeout, publishConfirmTimeout)
	}
}

// panicRecoveryContext holds context for panic recovery handling
type panicRecoveryContext struct {
	logger     libLog.Logger
	msg        *amqp.Delivery
	queue      string
	workerID   int
	retryCount int
	conn       *libRabbitmq.RabbitMQConnection
}

// businessErrorContext holds context for business error handling with retry tracking.
// Used by processHandler to track retries and route to DLQ after max attempts.
type businessErrorContext struct {
	logger     libLog.Logger
	msg        *amqp.Delivery
	queue      string
	workerID   int
	retryCount int
	conn       *libRabbitmq.RabbitMQConnection
	err        error
}

// handleBusinessError handles a business error with retry tracking.
// Routes to DLQ after max retries exceeded, or republishes with incremented counter.
func (bec *businessErrorContext) handleBusinessError() {
	if bec.retryCount >= maxRetries-1 {
		// Max retries exceeded - route to DLQ
		bec.logger.Errorf("Worker %d: business error after %d delivery attempts - routing to DLQ: %v",
			bec.workerID, bec.retryCount+1, bec.err)

		dlqName := buildDLQName(bec.queue)
		if err := bec.publishToDLQ(dlqName); err != nil {
			bec.logger.Errorf("Worker %d: CRITICAL - DLQ publish failed for business error, message will be PERMANENTLY LOST - queue=%s, dlq=%s, retry_count=%d, error=%v, original_error=%v",
				bec.workerID, bec.queue, dlqName, bec.retryCount+1, err, bec.err)

			// Fall back to reject (message is lost - tradeoff accepted for max retries exceeded)
			if rejectErr := bec.msg.Reject(false); rejectErr != nil {
				bec.logger.Warnf("Worker %d: failed to reject business error message: %v", bec.workerID, rejectErr)
			}

			return
		}

		// Ack original message since we successfully published to DLQ
		if err := bec.msg.Ack(false); err != nil {
			bec.logger.Warnf("Worker %d: failed to ack original message after DLQ publish: %v", bec.workerID, err)
		}

		return
	}

	// Retry with incremented counter
	bec.republishWithRetry()
}

// publishToDLQ publishes a business error message to the Dead Letter Queue.
// Uses publishToDLQShared to eliminate code duplication with panicRecoveryContext.
func (bec *businessErrorContext) publishToDLQ(dlqName string) error {
	// Copy only safe headers to prevent sensitive data propagation (CWE-200)
	headers := copyHeadersSafe(bec.msg.Headers)
	// Sanitize error message to prevent information disclosure (CWE-209)
	sanitizedReason := sanitizeErrorForDLQ(bec.err)
	headers["x-dlq-reason"] = fmt.Sprintf("business_error: %s", sanitizedReason)
	headers["x-dlq-original-queue"] = bec.queue
	headers["x-dlq-retry-count"] = safeIncrementRetryCount(bec.retryCount)
	headers["x-dlq-timestamp"] = time.Now().Unix()
	headers["x-dlq-error-type"] = "business_error"

	return publishToDLQShared(&dlqPublishParams{
		conn:          bec.conn,
		dlqName:       dlqName,
		msg:           bec.msg,
		headers:       headers,
		logger:        bec.logger,
		workerID:      bec.workerID,
		originalQueue: bec.queue,
		retryCount:    bec.retryCount + 1, // +1 because this is after the final attempt
		reason:        fmt.Sprintf("business_error: %s", sanitizedReason),
	})
}

// republishWithRetry republishes the message with an incremented retry counter.
// TODO(review): No backoff between retries - tradeoff: consistent with panic handler pattern,
// immediate redelivery to maximize throughput. RabbitMQ's consumer prefetch provides natural
// rate limiting. (reported by code-reviewer on 2025-12-14, severity: Medium)
// TODO(review): republishWithRetry does not use publisher confirms - tradeoff:
// performance over guaranteed delivery. DLQ uses confirms (last chance), but
// retry path accepts potential message loss during broker failure between
// Publish success and message persistence. (reported by business-logic-reviewer
// on 2025-12-14, severity: Medium)
func (bec *businessErrorContext) republishWithRetry() {
	bec.logger.Warnf("Worker %d: redelivering business error message (delivery %d of %d max): %v",
		bec.workerID, bec.retryCount+1, maxRetries, bec.err)

	// Copy only safe headers to prevent sensitive data propagation (CWE-200)
	headers := copyHeadersSafe(bec.msg.Headers)
	headers[retryCountHeader] = safeIncrementRetryCount(bec.retryCount)

	ch, err := bec.conn.Connection.Channel()
	if err != nil {
		bec.logger.Errorf("Worker %d: failed to get channel for business error republish: %v", bec.workerID, err)
		bec.nackWithLogging()

		return
	}
	defer ch.Close()

	err = ch.Publish(
		"",        // exchange (use default)
		bec.queue, // routing key (queue name)
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			Headers:      headers,
			Body:         bec.msg.Body,
			ContentType:  bec.msg.ContentType,
			DeliveryMode: amqp.Persistent,
		},
	)
	if err != nil {
		bec.logger.Errorf("Worker %d: failed to republish business error message: %v", bec.workerID, err)
		bec.nackWithLogging()

		return
	}

	if err := bec.msg.Ack(false); err != nil {
		bec.logger.Warnf("Worker %d: failed to ack original message after business error republish: %v", bec.workerID, err)
	}
}

// nackWithLogging performs a Nack with logging on failure.
// This is a fallback path when channel acquisition fails - message will be redelivered
// without retry count increment (reported by business-logic-reviewer on 2025-12-14).
func (bec *businessErrorContext) nackWithLogging() {
	bec.logger.Warnf("Worker %d: falling back to NACK without retry increment (channel unavailable)", bec.workerID)

	if nackErr := bec.msg.Nack(false, true); nackErr != nil {
		bec.logger.Warnf("Worker %d: failed to nack business error message: %v", bec.workerID, nackErr)
	}
}

// handlePoisonMessage routes a message that has exceeded max retry attempts to the DLQ.
// Returns true if the message was handled as a poison message.
func (prc *panicRecoveryContext) handlePoisonMessage(panicValue any) bool {
	if prc.retryCount < maxRetries-1 {
		return false
	}

	prc.logger.Errorf("Worker %d: poison message after %d delivery attempts: %v - routing to DLQ",
		prc.workerID, prc.retryCount+1, panicValue)

	// Attempt to publish to DLQ
	// TODO(review): Consider adding single retry with 1-2s backoff before fallback to reject (reported by business-logic-reviewer on 2025-12-14, severity: Medium)
	dlqName := buildDLQName(prc.queue)
	if err := prc.publishToDLQ(dlqName, panicValue); err != nil {
		// CRITICAL: This is a double-failure scenario (max retries + DLQ unavailable)
		// The message will be permanently lost via Reject(false) below
		// TODO(review): Add metrics for alerting: metrics.IncrCounter("dlq.publish.failure", 1) (reported by business-logic-reviewer on 2025-12-14, severity: High)
		prc.logger.Errorf("Worker %d: CRITICAL - DLQ publish failed, message will be PERMANENTLY LOST - queue=%s, dlq=%s, retry_count=%d, error=%v",
			prc.workerID, prc.queue, dlqName, prc.retryCount+1, err)

		// Fall back to reject (message is lost - tradeoff accepted for double-failure)
		if rejectErr := prc.msg.Reject(false); rejectErr != nil {
			prc.logger.Warnf("Worker %d: failed to reject poison message: %v", prc.workerID, rejectErr)
		}

		return true
	}

	// Ack original message since we successfully published to DLQ
	if err := prc.msg.Ack(false); err != nil {
		prc.logger.Warnf("Worker %d: failed to ack original message after DLQ publish: %v", prc.workerID, err)
	}

	return true
}

// publishToDLQ publishes a message to the Dead Letter Queue with error context.
// Uses publishToDLQShared to eliminate code duplication with businessErrorContext.
// TODO(review): Consider adding unit tests for confirmation scenarios (Ack/Nack/Timeout) using mock channels (reported by code-reviewer on 2025-12-14, severity: Medium)
func (prc *panicRecoveryContext) publishToDLQ(dlqName string, panicValue any) error {
	// Copy only safe headers to prevent sensitive data propagation (CWE-200)
	headers := copyHeadersSafe(prc.msg.Headers)
	// Sanitize panic value to prevent information disclosure (CWE-209)
	sanitizedReason := sanitizePanicForDLQ(panicValue)
	headers["x-dlq-reason"] = fmt.Sprintf("panic: %s", sanitizedReason)
	headers["x-dlq-original-queue"] = prc.queue
	headers["x-dlq-retry-count"] = safeIncrementRetryCount(prc.retryCount)
	headers["x-dlq-timestamp"] = time.Now().Unix()

	return publishToDLQShared(&dlqPublishParams{
		conn:          prc.conn,
		dlqName:       dlqName,
		msg:           prc.msg,
		headers:       headers,
		logger:        prc.logger,
		workerID:      prc.workerID,
		originalQueue: prc.queue,
		retryCount:    prc.retryCount + 1, // +1 because this is after the final attempt
		reason:        fmt.Sprintf("panic: %s", sanitizedReason),
	})
}

// republishWithRetry republishes a message with an incremented retry counter.
// Falls back to Nack if republish fails.
// TODO(review): No backoff between retries - tradeoff: consistent with business error handler,
// immediate redelivery to maximize throughput. RabbitMQ's consumer prefetch provides natural
// rate limiting. (reported by code-reviewer on 2025-12-14, severity: Medium)
// TODO(review): Does not use publisher confirms - tradeoff: performance over guaranteed delivery.
// DLQ uses confirms (last chance), but retry path accepts potential message loss during broker
// failure. (reported by business-logic-reviewer on 2025-12-14, severity: Medium)
func (prc *panicRecoveryContext) republishWithRetry(panicValue any) {
	prc.logger.Warnf("Worker %d: redelivering message (delivery %d of %d max): %v",
		prc.workerID, prc.retryCount+1, maxRetries, panicValue)

	// Copy only safe headers to prevent sensitive data propagation (CWE-200)
	headers := copyHeadersSafe(prc.msg.Headers)
	headers[retryCountHeader] = safeIncrementRetryCount(prc.retryCount)

	ch, err := prc.conn.Connection.Channel()
	if err != nil {
		prc.handleChannelError(err)
		return
	}

	defer ch.Close()

	prc.publishAndAck(ch, headers)
}

// handleChannelError handles failure to get a channel for republishing
func (prc *panicRecoveryContext) handleChannelError(err error) {
	prc.logger.Errorf("Worker %d: failed to get channel for republish: %v", prc.workerID, err)
	prc.nackWithLogging()
}

// publishAndAck publishes the message with updated headers and acks the original
func (prc *panicRecoveryContext) publishAndAck(ch *amqp.Channel, headers amqp.Table) {
	err := ch.Publish(
		"",        // exchange (use default)
		prc.queue, // routing key (queue name)
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			Headers:      headers,
			Body:         prc.msg.Body,
			ContentType:  prc.msg.ContentType,
			DeliveryMode: amqp.Persistent,
		},
	)
	if err != nil {
		prc.logger.Errorf("Worker %d: failed to republish message: %v", prc.workerID, err)
		prc.nackWithLogging()

		return
	}

	if err := prc.msg.Ack(false); err != nil {
		prc.logger.Warnf("Worker %d: failed to ack original message after republish: %v", prc.workerID, err)
	}
}

// nackWithLogging performs a Nack with logging on failure.
// This is a fallback path when channel acquisition fails - message will be redelivered
// without retry count increment (reported by business-logic-reviewer on 2025-12-14).
func (prc *panicRecoveryContext) nackWithLogging() {
	prc.logger.Warnf("Worker %d: falling back to NACK without retry increment (channel unavailable)", prc.workerID)

	if nackErr := prc.msg.Nack(false, true); nackErr != nil {
		prc.logger.Warnf("Worker %d: failed to nack message: %v", prc.workerID, nackErr)
	}
}

// extractMidazID extracts the Midaz ID from message headers.
// Returns a new UUID if header is not present or has an unsupported type.
func extractMidazID(headers amqp.Table) string {
	raw, ok := headers[libConstants.HeaderID]
	if !ok {
		return libCommons.GenerateUUIDv7().String()
	}

	switch v := raw.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return libCommons.GenerateUUIDv7().String()
	}
}

// messageProcessingContext holds all context needed for processing a single message.
type messageProcessingContext struct {
	ctx      context.Context
	logger   libLog.Logger
	span     trace.Span
	msg      *amqp.Delivery
	queue    string
	workerID int
	conn     *libRabbitmq.RabbitMQConnection
}

// createMessageProcessingContext creates all the context and tracing for message processing.
func (cr *ConsumerRoutes) createMessageProcessingContext(msg *amqp.Delivery, queue string, workerID int) *messageProcessingContext {
	midazID := extractMidazID(msg.Headers)

	log := cr.Logger.WithFields(
		libConstants.HeaderID, midazID,
	).WithDefaultMessageTemplate(midazID + libConstants.LoggerDefaultSeparator)

	ctx := libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(context.Background(), midazID),
		log,
	)

	ctx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(ctx, msg.Headers)

	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "rabbitmq.consumer.process_message")
	ctx = libCommons.ContextWithSpanAttributes(ctx, attribute.String("app.request.request_id", reqID))

	return &messageProcessingContext{
		ctx:      ctx,
		logger:   logger,
		span:     span,
		msg:      msg,
		queue:    queue,
		workerID: workerID,
		conn:     cr.conn,
	}
}

// handlePanicRecovery handles panic recovery within message processing.
func (mpc *messageProcessingContext) handlePanicRecovery(panicValue any) {
	stack := debug.Stack()
	retryCount := getRetryCount(mpc.msg.Headers)

	mpc.span.AddEvent("panic.recovered", trace.WithAttributes(
		attribute.String("panic.value", fmt.Sprintf("%v", panicValue)),
		attribute.String("panic.stack", string(stack)),
		attribute.String("rabbitmq.queue", mpc.queue),
		attribute.Int("rabbitmq.worker_id", mpc.workerID),
		attribute.Int("rabbitmq.retry_count", retryCount),
	))

	mpc.logger.Errorf("Worker %d: panic recovered while processing message from queue %s: %v\n%s",
		mpc.workerID, mpc.queue, panicValue, string(stack))

	prc := &panicRecoveryContext{
		logger:     mpc.logger,
		msg:        mpc.msg,
		queue:      mpc.queue,
		workerID:   mpc.workerID,
		retryCount: retryCount,
		conn:       mpc.conn,
	}

	if !prc.handlePoisonMessage(panicValue) {
		prc.republishWithRetry(panicValue)
	}

	mruntime.RecordPanicToSpanWithComponent(mpc.ctx, panicValue, stack, "rabbitmq_consumer", "worker_"+mpc.queue)
}

// processHandler invokes the handler and handles errors with retry tracking.
func (mpc *messageProcessingContext) processHandler(handlerFunc QueueHandlerFunc) {
	if err := libOpentelemetry.SetSpanAttributesFromStruct(&mpc.span, "app.request.rabbitmq.consumer.message", mpc.msg.Body); err != nil {
		libOpentelemetry.HandleSpanError(&mpc.span, "Failed to convert message to JSON string", err)
	}

	if err := handlerFunc(mpc.ctx, mpc.msg.Body); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&mpc.span, "Error processing message from queue", err)
		mpc.logger.Errorf("Worker %d: Error processing message from queue %s: %v", mpc.workerID, mpc.queue, err)

		// Use retry tracking for business errors instead of infinite NACK loop
		retryCount := getRetryCount(mpc.msg.Headers)
		bec := &businessErrorContext{
			logger:     mpc.logger,
			msg:        mpc.msg,
			queue:      mpc.queue,
			workerID:   mpc.workerID,
			retryCount: retryCount,
			conn:       mpc.conn,
			err:        err,
		}
		bec.handleBusinessError()

		return
	}

	mpc.ackMessage()
}

// ackMessage sends an Ack for the message.
func (mpc *messageProcessingContext) ackMessage() {
	if ackErr := mpc.msg.Ack(false); ackErr != nil {
		mpc.logger.Warnf("Worker %d: failed to ack message (may cause redelivery): %v", mpc.workerID, ackErr)
	}
}

// QueueHandlerFunc is a function that process a specific queue.
type QueueHandlerFunc func(ctx context.Context, body []byte) error

// ConsumerRoutes struct
type ConsumerRoutes struct {
	conn              *libRabbitmq.RabbitMQConnection
	routes            map[string]QueueHandlerFunc
	NumbersOfWorkers  int
	NumbersOfPrefetch int
	libLog.Logger
	libOpentelemetry.Telemetry
}

// NewConsumerRoutes creates a new instance of ConsumerRoutes.
func NewConsumerRoutes(conn *libRabbitmq.RabbitMQConnection, numbersOfWorkers int, numbersOfPrefetch int, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *ConsumerRoutes {
	if numbersOfWorkers == 0 {
		numbersOfWorkers = 5
	}

	if numbersOfPrefetch == 0 {
		numbersOfPrefetch = 10
	}

	cr := &ConsumerRoutes{
		conn:              conn,
		routes:            make(map[string]QueueHandlerFunc),
		NumbersOfWorkers:  numbersOfWorkers,
		NumbersOfPrefetch: numbersOfWorkers * numbersOfPrefetch,
		Logger:            logger,
		Telemetry:         *telemetry,
	}

	_, err := conn.GetNewConnect()
	assert.NoError(err, "RabbitMQ connection must succeed during initialization",
		"component", "rabbitmq_consumer",
		"workers", numbersOfWorkers,
		"prefetch", numbersOfPrefetch)

	return cr
}

// Register add a new queue to handler.
func (cr *ConsumerRoutes) Register(queueName string, handler QueueHandlerFunc) {
	cr.routes[queueName] = handler
}

// RunConsumers init consume for all registry queues.
//
//nolint:gocognit // Complexity from panic recovery, backoff, and reconnection logic is necessary for resilience
func (cr *ConsumerRoutes) RunConsumers() error {
	for queueName, handler := range cr.routes {
		cr.Infof("Initializing consumer for queue: %s", queueName)

		// Capture loop variables before SafeGo
		queue := queueName
		queueHandler := handler

		mruntime.SafeGo(cr.Logger, "rabbitmq_consumer_"+queue, mruntime.KeepRunning, func() {
			backoff := utils.InitialBackoff

			for {
				// Wrap each iteration in an anonymous function with panic recovery
				shouldContinue := func() bool {
					defer mruntime.RecoverAndLog(cr.Logger, "rabbitmq_consumer_loop_"+queue)

					if err := cr.conn.EnsureChannel(); err != nil {
						cr.Errorf("[Consumer %s] failed to ensure channel: %v", queue, err)

						sleepDuration := utils.FullJitter(backoff)
						cr.Infof("[Consumer %s] retrying EnsureChannel in %v...", queue, sleepDuration)
						time.Sleep(sleepDuration)

						backoff = utils.NextBackoff(backoff)

						return true
					}

					if err := cr.conn.Channel.Qos(
						cr.NumbersOfPrefetch,
						0,
						false,
					); err != nil {
						cr.Errorf("[Consumer %s] failed to set QoS: %v", queue, err)

						sleepDuration := utils.FullJitter(backoff)
						cr.Infof("[Consumer %s] retrying QoS in %v...", queue, sleepDuration)
						time.Sleep(sleepDuration)

						backoff = utils.NextBackoff(backoff)

						return true
					}

					messages, err := cr.conn.Channel.Consume(
						queue,
						"",
						false,
						false,
						false,
						false,
						nil,
					)
					if err != nil {
						cr.Errorf("[Consumer %s] failed to start consuming: %v", queue, err)

						sleepDuration := utils.FullJitter(backoff)
						cr.Infof("[Consumer %s] retrying Consume in %v...", queue, sleepDuration)
						time.Sleep(sleepDuration)

						backoff = utils.NextBackoff(backoff)

						return true
					}

					cr.Infof("[Consumer %s] consuming started", queue)

					backoff = utils.InitialBackoff

					notifyClose := make(chan *amqp.Error, 1)
					cr.conn.Channel.NotifyClose(notifyClose)

					for i := 0; i < cr.NumbersOfWorkers; i++ {
						workerID := i

						mruntime.SafeGo(cr.Logger, "rabbitmq_worker_"+queue, mruntime.KeepRunning, func() {
							cr.startWorker(workerID, queue, queueHandler, messages)
						})
					}

					if errClose := <-notifyClose; errClose != nil {
						cr.Warnf("[Consumer %s] channel closed: %v", queue, errClose)
					} else {
						cr.Warnf("[Consumer %s] channel closed: no error info", queue)
					}

					cr.Warnf("[Consumer %s] restarting...", queue)

					return true
				}()

				if !shouldContinue {
					break
				}
			}
		})
	}

	return nil
}

// startWorker starts a worker that processes messages from the queue.
func (cr *ConsumerRoutes) startWorker(workerID int, queue string, handlerFunc QueueHandlerFunc, messages <-chan amqp.Delivery) {
	for msg := range messages {
		cr.processOneMessage(&msg, queue, workerID, handlerFunc)
	}

	cr.Warnf("[Consumer %s] worker %d stopped (channel closed)", queue, workerID)
}

// processOneMessage handles a single message with panic recovery.
// Does NOT re-panic so the worker survives and continues processing.
func (cr *ConsumerRoutes) processOneMessage(msg *amqp.Delivery, queue string, workerID int, handlerFunc QueueHandlerFunc) {
	mpc := cr.createMessageProcessingContext(msg, queue, workerID)

	defer mpc.span.End()

	defer func() {
		if r := recover(); r != nil {
			mpc.handlePanicRecovery(r)
		}
	}()

	mpc.processHandler(handlerFunc)
}
