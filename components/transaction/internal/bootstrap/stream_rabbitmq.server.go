package bootstrap

import (
	"context"
	"os"

	"github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/vmihailenco/msgpack/v5"
)

// StreamQueueConsumer represents a stream queue consumer.
type StreamQueueConsumer struct {
	StreamConsumer *rabbitmq.ConsumerStreamRabbit
	UseCase        *command.UseCase
}

// NewStreamQueueConsumer initializes a new instance of StreamQueueConsumer.
func NewStreamQueueConsumer(consumer *rabbitmq.ConsumerStreamRabbit, useCase *command.UseCase) *StreamQueueConsumer {
	return &StreamQueueConsumer{
		StreamConsumer: consumer,
		UseCase:        useCase,
	}
}

// Run initializes the stream queue consumer.
func (s *StreamQueueConsumer) Run(l *commons.Launcher) error {
	if s.StreamConsumer == nil {
		logger := commons.NewLoggerFromContext(context.Background())
		logger.Warn("Stream consumer is not initialized - skipping stream consumer")
		return nil
	}

	streamName := os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_STREAM_QUEUE")
	if streamName == "" {
		logger := commons.NewLoggerFromContext(context.Background())
		logger.Warn("RABBITMQ_TRANSACTION_BALANCE_OPERATION_STREAM_QUEUE not set - skipping stream consumer")
		return nil
	}

	// Set the handler before running the consumer
	s.StreamConsumer.SetHandler(s.Handle)

	return s.StreamConsumer.RunConsumer(streamName)
}

// Handle to process the stream message
func (s *StreamQueueConsumer) Handle(ctx context.Context, body []byte) error {
	logger := commons.NewLoggerFromContext(ctx)
	tracer := commons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "stream.consumer.handler")
	defer span.End()

	logger.Info("Processing stream message...")

	var message mmodel.Queue
	if err := msgpack.Unmarshal(body, &message); err != nil {
		logger.Errorf("Erro ao deserializar msgpack da stream: %v", err)
		return err
	}

	logger.Infof("Stream message ID: %s", message.QueueData[0].ID)

	err := s.UseCase.CreateBalanceTransactionOperationsAsync(ctx, message)
	if err != nil {
		logger.Errorf("Erro no processamento da stream: %v", err)
		return err
	}

	return nil
}
