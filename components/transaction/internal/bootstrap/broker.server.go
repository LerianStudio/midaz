// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

var (
	// ErrMultiQueueConsumerNil is returned when the multi queue consumer is nil.
	ErrMultiQueueConsumerNil = errors.New("multi queue consumer is nil")
	// ErrConsumerRoutesNotConfigured is returned when the consumer routes are not configured.
	ErrConsumerRoutesNotConfigured = errors.New("consumer routes are not configured")
	// ErrConsumerUseCaseNotConfigured is returned when the consumer use case is not configured.
	ErrConsumerUseCaseNotConfigured = errors.New("consumer use case is not configured")
	// ErrEmptyQueueData is returned when the queue payload has empty queue data.
	ErrEmptyQueueData = errors.New("invalid queue payload: empty queue data")
)

// MultiQueueConsumer represents a multi-topic consumer.
type MultiQueueConsumer struct {
	consumerRoutes *redpanda.ConsumerRoutes
	UseCase        *command.UseCase
}

// NewMultiQueueConsumer create a new instance of MultiQueueConsumer.
func NewMultiQueueConsumer(routes *redpanda.ConsumerRoutes, useCase *command.UseCase) *MultiQueueConsumer {
	consumer := &MultiQueueConsumer{
		consumerRoutes: routes,
		UseCase:        useCase,
	}

	if routes == nil || useCase == nil {
		return consumer
	}

	if useCase.BalanceCreateTopic != "" {
		routes.Register(useCase.BalanceCreateTopic, consumer.handlerBalanceCreateQueue)
	}

	if useCase.BalanceOperationsTopic != "" {
		routes.Register(useCase.BalanceOperationsTopic, consumer.handlerBTOQueue)
		routes.RegisterBatch(useCase.BalanceOperationsTopic, consumer.handlerBTOQueueBatch)
	}

	return consumer
}

// Run starts consumers for all registered topics.
func (mq *MultiQueueConsumer) Run(l *libCommons.Launcher) error {
	if mq == nil {
		return ErrMultiQueueConsumerNil
	}

	if mq.consumerRoutes == nil {
		return ErrConsumerRoutesNotConfigured
	}

	if err := mq.consumerRoutes.RunConsumers(); err != nil {
		return fmt.Errorf("consumer routes run failed: %w", err)
	}

	return nil
}

// handlerBalanceCreateQueue processes messages from the audit queue, unmarshal the JSON, and creates balances on database.
func (mq *MultiQueueConsumer) handlerBalanceCreateQueue(ctx context.Context, body []byte) error {
	if mq == nil || mq.UseCase == nil {
		return ErrConsumerUseCaseNotConfigured
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handler_balance_create_queue")
	defer span.End()

	logger.Info("Processing message from transaction_balance_queue")

	var message mmodel.Queue

	err := json.Unmarshal(body, &message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)

		logger.Errorf("Error unmarshalling accounts message JSON: %v", err)

		return err
	}

	logger.Infof("Account message consumed: %s", message.AccountID)

	err = mq.UseCase.CreateBalance(ctx, message)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Error creating balance", err)

		logger.Errorf("Error creating balance: %v", err)

		return err
	}

	return nil
}

// handlerBTOQueue processes messages from the balance fifo queue, unmarshal the JSON, and update balances on database.
func (mq *MultiQueueConsumer) handlerBTOQueue(ctx context.Context, body []byte) error {
	if mq == nil || mq.UseCase == nil {
		return ErrConsumerUseCaseNotConfigured
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handler_balance_update")
	defer span.End()

	logger.Info("Processing message from balance_retry_queue_fifo")

	var message mmodel.Queue

	err := msgpack.Unmarshal(body, &message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)

		logger.Errorf("Error unmarshalling balance message JSON: %v", err)

		return fmt.Errorf("unmarshal balance queue message: %w", err)
	}

	if len(message.QueueData) == 0 {
		err := ErrEmptyQueueData
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Empty queue data in balance_retry_queue_fifo message", err)

		logger.Warn("Empty queue data in balance_retry_queue_fifo message")

		return err
	}

	logger.Infof("Transaction message consumed: %s", message.QueueData[0].ID)

	err = mq.UseCase.CreateBalanceTransactionOperationsAsync(ctx, message)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Error creating transaction", err)

		logger.Errorf("Error creating transaction: %v", err)

		return err
	}

	return nil
}

// handlerBTOQueueBatch processes batches from the balance operations topic.
// It keeps behavior equivalent to single-message mode while enabling consumer-side
// micro-batch scheduling (size/window/idle flush).
func (mq *MultiQueueConsumer) handlerBTOQueueBatch(ctx context.Context, bodies [][]byte) error {
	if mq == nil || mq.UseCase == nil {
		return ErrConsumerUseCaseNotConfigured
	}

	if len(bodies) == 0 {
		return nil
	}

	batchMessages := make([]mmodel.Queue, 0, len(bodies))

	for i, body := range bodies {
		var message mmodel.Queue

		err := msgpack.Unmarshal(body, &message)
		if err != nil {
			return fmt.Errorf("batch item %d: failed to unmarshal queue message: %w", i, err)
		}

		if len(message.QueueData) == 0 {
			return fmt.Errorf("batch item %d: %w", i, ErrEmptyQueueData)
		}

		batchMessages = append(batchMessages, message)
	}

	return mq.UseCase.CreateBalanceTransactionOperationsBatch(ctx, batchMessages)
}
