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
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	postgreTransaction "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2/log"
)

const CronTimeToRun = 30 * time.Minute
const MessageTimeOfLife = 30
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

		logger := r.Logger.WithFields(
			libConstants.HeaderID, transaction.HeaderID,
		).WithDefaultMessageTemplate(transaction.HeaderID + " | ")

		ctxWithBackground := libCommons.ContextWithLogger(
			libCommons.ContextWithHeaderID(ctx, transaction.HeaderID),
			logger,
		)

		if transaction.TTL.Unix() > time.Now().Add(-MessageTimeOfLife*time.Minute).Unix() {
			totalMessagesLessThanOneHour++

			continue
		}

		sem <- struct{}{}

		wg.Add(1)

		go func(m mmodel.TransactionRedisQueue, ctx context.Context, logger libLog.Logger) {
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

			balances := make([]*mmodel.Balance, 0)
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
					OrganizationID: m.OrganizationID.String(),
					LedgerID:       m.LedgerID.String(),
				})
			}

			fromTo := append(m.ParserDSL.Send.Source.From, m.ParserDSL.Send.Distribute.To...)

			var parentTransactionID *string
			tran := &postgreTransaction.Transaction{
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

			operations, _, err := r.TransactionHandler.BuildOperations(ctx, logger, tracer, balances, fromTo, m.ParserDSL, *tran, m.Validate, m.TransactionDate, m.TransactionStatus == constant.NOTED)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to validate balances", err)

				logger.Errorf("Failed to validate balance: %v", err.Error())

				return
			}

			tran.Source = m.Validate.Sources
			tran.Destination = m.Validate.Destinations
			tran.Operations = operations

			err = r.TransactionHandler.Command.SendBTOExecuteAsync(ctx, m.OrganizationID, m.LedgerID, &m.ParserDSL, m.Validate, balances, tran)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed sending message to queue", err)

				logger.Errorf("Failed sending message: %s to queue: %v", key, err.Error())

				return
			}

			log.Infof("Transaction message processed: %s", key)
		}(transaction, ctxWithBackground, logger)
	}

	wg.Wait()

	r.Logger.Infof("Total of messagens under %d minute(s) : %d", MessageTimeOfLife, totalMessagesLessThanOneHour)
	r.Logger.Infof("Finished processing total of %d eligible messages", total-totalMessagesLessThanOneHour)
}
