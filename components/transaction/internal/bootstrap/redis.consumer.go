package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

const CronTimeToRun = 5 * time.Minute
const MessageTimeOfLife = 60
const MaxWorkers = 100

type RedisQueueConsumer struct {
	Logger             libLog.Logger
	TransactionHandler in.TransactionHandler
}

func NewRedisQueueConsumer(logger libLog.Logger, handler in.TransactionHandler) *RedisQueueConsumer {
	return &RedisQueueConsumer{
		Logger:             logger,
		TransactionHandler: handler,
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

	messages, err := r.TransactionHandler.Command.RedisRepo.ReadAllMessagesFromQueue(ctx)
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

	totalMessagesLessThanOneHour := 0

	for key, message := range messages {
		var transaction mmodel.TransactionRedisQueue
		err := json.Unmarshal([]byte(message), &transaction)
		if err != nil {
			r.Logger.Warnf("Error unmarshalling message from Redis: %v", err)

			continue
		}

		log := r.Logger.WithFields(
			libConstants.HeaderID, transaction.HeaderID,
		).WithDefaultMessageTemplate(transaction.HeaderID + " | ")

		ctxWithBackground := libCommons.ContextWithLogger(
			libCommons.ContextWithHeaderID(ctx, transaction.HeaderID),
			log,
		)

		if transaction.TTL.Unix() > time.Now().Add(-MessageTimeOfLife*time.Minute).Unix() {
			totalMessagesLessThanOneHour++

			continue
		}

		sem <- struct{}{}

		wg.Add(1)

		go func(m mmodel.TransactionRedisQueue, ctx context.Context, log libLog.Logger) {
			defer func() {
				<-sem
				wg.Done()
			}()

			select {
			case <-ctx.Done():
				log.Warn("Transaction message processing cancelled due to shutdown signal")
				return
			default:
			}

			_ = r.TransactionHandler.RetryTransaction(transaction.ParserDSL, transaction.Context)

			log.Infof("Transaction message processed: %s", key)
		}(transaction, ctxWithBackground, log)
	}

	wg.Wait()

	r.Logger.Infof("Total of messagens under %d minute(s) : %d", MessageTimeOfLife, totalMessagesLessThanOneHour)
	r.Logger.Infof("Finished processing total of %d eligible messages", total-totalMessagesLessThanOneHour)
}
