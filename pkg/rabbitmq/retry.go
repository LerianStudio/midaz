// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/trace"
)

// SleepFunc sleeps for the given duration between retries.
// Injecting a no-op implementation in tests removes real backoff delays.
type SleepFunc func(time.Duration)

// BackoffFunc returns the delay to wait before the given retry attempt (0-indexed).
type BackoffFunc func(attempt int) time.Duration

// RepublishFunc republishes a failed message for another attempt, using the
// supplied retry headers. It owns the transport choice (single-tenant channel,
// per-tenant vhost channel, etc.) and the nack-to-DLQ + span/log handling on its
// own failure path. A non-nil return tells the engine the republish did not
// succeed; the engine then leaves the message unacked (the hook is responsible
// for having dead-lettered it). A nil return means the message was republished
// and the engine should Ack the original delivery.
type RepublishFunc func(ctx context.Context, workerID int, queue string, message amqp.Delivery, headers amqp.Table, span trace.Span) error

// RetryManager is the generic retry engine for failed RabbitMQ consumer messages.
// It composes error classification and backoff to decide, per failure, whether a
// message is retried (republished with incremented headers, then Acked) or sent to
// the dead-letter queue. The transport-specific republish is supplied as a hook so
// the engine stays free of any single-tenant/multi-tenant or tenant-manager coupling.
type RetryManager struct {
	classifier ErrorClassifier
	backoff    BackoffFunc
	republish  RepublishFunc
	maxRetries int
	sleepFunc  SleepFunc
	logger     log.Logger
}

// Config holds the dependencies for a RetryManager. SleepFunc defaults to time.Sleep
// when nil so production callers need not supply it; tests inject a no-op.
type Config struct {
	Classifier ErrorClassifier
	Backoff    BackoffFunc
	Republish  RepublishFunc
	MaxRetries int
	SleepFunc  SleepFunc
	Logger     log.Logger
}

// NewRetryManager builds a RetryManager from cfg. A nil SleepFunc defaults to time.Sleep.
func NewRetryManager(cfg Config) *RetryManager {
	sleepFunc := cfg.SleepFunc
	if sleepFunc == nil {
		sleepFunc = time.Sleep
	}

	return &RetryManager{
		classifier: cfg.Classifier,
		backoff:    cfg.Backoff,
		republish:  cfg.Republish,
		maxRetries: cfg.MaxRetries,
		sleepFunc:  sleepFunc,
		logger:     cfg.Logger,
	}
}

// HandleFailure determines whether a failed message is retried or sent to the DLQ.
// Non-retryable errors and retry-count exhaustion route immediately to the DLQ.
// Retryable errors are republished with exponential backoff and incremented retry
// headers, then the original delivery is Acked.
func (rm *RetryManager) HandleFailure(ctx context.Context, workerID int, queue string, message amqp.Delivery, err error, retryCount int, span trace.Span) {
	if !rm.classifier.IsRetryable(err) {
		rm.logger.Log(ctx, log.LevelInfo, "Non-retryable error, sending to DLQ",
			log.Int("worker_id", workerID),
			log.String("queue", queue),
			log.Err(err),
		)
		libOtel.HandleSpanBusinessErrorEvent(span, "Non-retryable business error, routing to DLQ", err)
		NackToDLQ(ctx, rm.logger, workerID, queue, message)

		return
	}

	if retryCount >= rm.maxRetries {
		rm.logger.Log(ctx, log.LevelError, "Max retries exceeded, sending to DLQ",
			log.Int("worker_id", workerID),
			log.Int("max_retries", rm.maxRetries),
			log.String("queue", queue),
			log.Err(err),
		)
		libOtel.HandleSpanError(span, "Max retries exceeded, routing to DLQ", err)
		NackToDLQ(ctx, rm.logger, workerID, queue, message)

		return
	}

	backoff := rm.backoff(retryCount)

	rm.logger.Log(ctx, log.LevelInfo, "Retryable error before republish",
		log.Int("worker_id", workerID),
		log.String("queue", queue),
		log.Int("attempt", retryCount+1),
		log.Int("max_retries", rm.maxRetries),
		log.Any("backoff", backoff),
		log.Err(err),
	)

	rm.sleepFunc(backoff)

	failureReason := rm.classifier.ClassifyFailureReason(err)
	retryHeaders := BuildRetryHeaders(message.Headers, retryCount, failureReason)

	if publishErr := rm.republish(ctx, workerID, queue, message, retryHeaders, span); publishErr != nil {
		return // republish hook already handled nack + logging
	}

	if ackErr := message.Ack(false); ackErr != nil {
		rm.logger.Log(ctx, log.LevelError, "Ack failed after republish; message may be redelivered",
			log.Int("worker_id", workerID),
			log.String("queue", queue),
			log.Err(ackErr),
		)
	}

	rm.logger.Log(ctx, log.LevelInfo, "Message republished for retry",
		log.Int("worker_id", workerID),
		log.Int("attempt", retryCount+1),
		log.Int("max_retries", rm.maxRetries),
		log.String("queue", queue),
	)
}

// NackToDLQ sends a message to the dead-letter queue via Nack without requeue.
// A nil logger is tolerated so the helper can be used from contexts that do not
// carry one.
func NackToDLQ(ctx context.Context, logger log.Logger, workerID int, queue string, message amqp.Delivery) {
	if nackErr := message.Nack(false, false); nackErr != nil && logger != nil {
		logger.Log(ctx, log.LevelError, "Nack failed",
			log.Int("worker_id", workerID),
			log.String("queue", queue),
			log.Err(nackErr),
		)
	}
}
