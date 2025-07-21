package bootstrap

import (
	"context"
	libConstants "github.com/LerianStudio/lib-commons/commons/constants"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/vmihailenco/msgpack/v5"
)

const CronTimeToRun = 15 * time.Minute
const MessageTimeOfLife = 60
const MaxWorkers = 100

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

func (r *RedisQueueConsumer) readMessagesAndProcess(ctx context.Context) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.consumer.read_messages_from_queue")
	defer span.End()

	r.Logger.Infof("Init cron to read messages from redis...")

	messages, err := r.UseCase.RedisRepo.ReadAllMessagesFromQueue(ctx)
	if err != nil {
		r.Logger.Errorf("Err to read messages from redis: %v", err)
		return
	}

	total := len(messages)
	r.Logger.Infof("Total of read %d messages from queue", total)

	if total == 0 {
		return
	}

	sem := make(chan struct{}, MaxWorkers)

	var wg sync.WaitGroup

	cutoff := time.Now().Add(-MessageTimeOfLife * time.Minute).Unix()

	totalMessagesLessThanOneHour := 0

	for _, msg := range messages {
		log := r.Logger.WithFields(
			libConstants.HeaderID, msg.HeaderID,
		).WithDefaultMessageTemplate(msg.HeaderID + " | ")

		ctxWithBackground := libCommons.ContextWithLogger(
			libCommons.ContextWithHeaderID(ctx, msg.HeaderID),
			log,
		)

		if msg.Timestamp > cutoff {
			totalMessagesLessThanOneHour++

			continue
		}

		sem <- struct{}{}

		wg.Add(1)

		go func(m redis.RedisMessage, ctx context.Context, log libLog.Logger) {
			defer func() {
				<-sem
				wg.Done()
			}()

			select {
			case <-ctx.Done():
				log.Warn("Message processing cancelled due to shutdown signal")
				return
			default:
			}

			payloadBytes, err := msgpack.Marshal(m.Payload)
			if err != nil {
				log.Errorf("failed to marshal msg.Payload: %v", err)
				return
			}

			var data mmodel.Queue
			if err := msgpack.Unmarshal(payloadBytes, &data); err != nil {
				log.Errorf("failed to unmarshal payload into mmodel.Queue: %v", err)
				return
			}

			if err := r.UseCase.CreateBalanceTransactionOperationsAsync(ctx, data); err != nil {
				log.Errorf("Failed to create balance transaction operations: %v", err)
				return
			}

			log.Infof("Message processed: %s", m.ID)
		}(msg, ctxWithBackground, log)
	}

	wg.Wait()
	r.Logger.Infof("Total of messagens under %d minute(s) : %d", MessageTimeOfLife, totalMessagesLessThanOneHour)
	r.Logger.Infof("Finished processing total of %d eligible messages", total-totalMessagesLessThanOneHour)
}
