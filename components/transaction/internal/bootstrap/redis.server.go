package bootstrap

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/vmihailenco/msgpack/v5"
	"time"
)

const CronTimeToRun = 1 * time.Minute

type RedisQueueConsumer struct {
	UseCase *command.UseCase
	Logger  libLog.Logger
}

func NewRedisQueueConsumer(useCase *command.UseCase, logger libLog.Logger) *RedisQueueConsumer {
	return &RedisQueueConsumer{
		UseCase: useCase,
		Logger:  logger,
	}
}

// Run starts redis consumers for queues
func (r *RedisQueueConsumer) Run(l *libCommons.Launcher) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticker := time.NewTicker(CronTimeToRun)
	defer ticker.Stop()

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

func (r *RedisQueueConsumer) readMessagesAndProcess(ctx context.Context) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.consumer.read_messages_from_queue")

	r.Logger.Infof("Init cron to read messages from redis...")

	messages, err := r.UseCase.RedisRepo.ReadAllMessagesFromQueue(ctx)
	if err != nil {
		r.Logger.Errorf("Err to read messages from redis: %v", err)

		span.End()

		return
	}

	r.Logger.Infof("Total of read %d messages from queue", len(messages))

	for _, msg := range messages {
		r.Logger.Infof("Message received from queue: %s", msg.ID)

		payloadBytes, err := msgpack.Marshal(msg.Payload)
		if err != nil {
			r.Logger.Errorf("failed to marshal payload to casto in queue: %v", err)

			continue
		}

		var data mmodel.Queue
		
		err = msgpack.Unmarshal(payloadBytes, &data)
		if err != nil {
			r.Logger.Errorf("failed to unmarshal payload into queue: %v", err)

			continue
		}

		if err = r.UseCase.CreateBalanceTransactionOperationsAsync(ctx, data); err != nil {
			r.Logger.Errorf("Failed to create balance transaction operations: %v", err)

			continue
		}
	}

	span.End()
}
