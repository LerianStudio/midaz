package bootstrap

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"time"
)

const CronTimeToRun = 15

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
	ticker := time.NewTicker(CronTimeToRun * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx := context.Background()
			tracer := libCommons.NewTracerFromContext(ctx)

			ctx, span := tracer.Start(ctx, "redis.consumer.read_messages_from_queue")
			defer span.End()

			r.Logger.Infof("Init cron to read messages from redis...")

			messages, err := r.UseCase.RedisRepo.ReadAllMessagesFromQueue(ctx)
			if err != nil {
				r.Logger.Errorf("Err to read messages from redis: %v", err)

				continue
			}

			for _, msg := range messages {
				r.Logger.Infof("Mensagem received from queue: %s", msg.ID)

				data, ok := msg.Payload.(mmodel.Queue)
				if !ok {
					r.Logger.Errorf("Payload can't be casted to Queue: %v", msg.Payload)

					continue
				}

				err := r.UseCase.CreateBalanceTransactionOperationsAsync(ctx, data)
				if err != nil {
					r.Logger.Errorf("Failed to create balance transaction operations: %v", err)

					continue
				}
			}
		}
	}

}
