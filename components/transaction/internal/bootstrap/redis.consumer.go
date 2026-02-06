// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

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
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	postgreTransaction "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

const (
	CronTimeToRun     = 30 * time.Minute
	MessageTimeOfLife = 30
	MaxWorkers        = 100
	ConsumerLockTTL   = 1500 // 25 minutes in seconds
)

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

//nolint:dogsled,gocognit
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

	sem := make(chan struct{}, MaxWorkers)

	var wg sync.WaitGroup

	totalMessagesLessThanOneHour := 0

Outer:
	for key, message := range messages {
		if ctx.Err() != nil {
			r.Logger.Warnf("Shutdown in progress: skipping remaining messages")
			break Outer
		}

		var transaction mmodel.TransactionRedisQueue
		if err := json.Unmarshal([]byte(message), &transaction); err != nil {
			r.Logger.Warnf("Error unmarshalling message from Redis: %v", err)
			continue
		}

		if transaction.TTL.Unix() > time.Now().Add(-MessageTimeOfLife*time.Minute).Unix() {
			totalMessagesLessThanOneHour++
			continue
		}

		sem <- struct{}{}

		wg.Add(1)

		go func(key string, m mmodel.TransactionRedisQueue) {
			defer func() {
				if rec := recover(); rec != nil {
					r.Logger.Warnf("Panic recovered while processing message (key: %s): %v. Message will remain in queue.", key, rec)
				}

				<-sem
				wg.Done()
			}()

			msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
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

			select {
			case <-msgCtxWithSpan.Done():
				logger.Warn("Transaction message processing cancelled due to shutdown/timeout")

				return
			default:
			}

			// Acquire distributed lock to prevent duplicate processing across pods
			lockKey := utils.RedisConsumerLockKey(m.OrganizationID, m.LedgerID, m.TransactionID.String())

			success, err := r.TransactionHandler.Command.RedisRepo.SetNX(msgCtxWithSpan, lockKey, "", ConsumerLockTTL)
			if err != nil {
				logger.Warnf("Failed to acquire lock for message %s: %v", key, err)

				return
			}

			if !success {
				logger.Infof("Message %s already being processed by another pod, skipping", key)

				return
			}

			if m.Validate == nil {
				logger.Warnf("Message (key: %s) has nil Validate field, skipping. Message will remain in queue.", key)

				return
			}

			balances := make([]*mmodel.Balance, 0, len(m.Balances))
			for _, balance := range m.Balances {
				balanceKey := balance.Key
				if balanceKey == "" {
					balanceKey = constant.DefaultBalanceKey
				}

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
					Key:            balanceKey,
					OrganizationID: m.OrganizationID.String(),
					LedgerID:       m.LedgerID.String(),
				})
			}

			var parentTransactionID *string

			tran := &postgreTransaction.Transaction{
				ID:                       m.TransactionID.String(),
				ParentTransactionID:      parentTransactionID,
				OrganizationID:           m.OrganizationID.String(),
				LedgerID:                 m.LedgerID.String(),
				Description:              m.TransactionInput.Description,
				Amount:                   &m.TransactionInput.Send.Value,
				AssetCode:                m.TransactionInput.Send.Asset,
				ChartOfAccountsGroupName: m.TransactionInput.ChartOfAccountsGroupName,
				CreatedAt:                m.TransactionDate,
				UpdatedAt:                time.Now(),
				Route:                    m.TransactionInput.Route,
				Metadata:                 m.TransactionInput.Metadata,
				Status: postgreTransaction.Status{
					Code:        m.TransactionStatus,
					Description: &m.TransactionStatus,
				},
			}

			var operations []*operation.Operation

			if len(m.Operations) > 0 {
				operations = make([]*operation.Operation, 0, len(m.Operations))
				for _, r := range m.Operations {
					operations = append(operations, operation.OperationFromRedis(r))
				}

				logger.Infof("Using %d materialized operations from backup", len(operations))
			} else {
				var fromTo []pkgTransaction.FromTo

				fromTo = append(fromTo, r.TransactionHandler.HandleAccountFields(m.TransactionInput.Send.Source.From, true)...)
				to := r.TransactionHandler.HandleAccountFields(m.TransactionInput.Send.Distribute.To, true)

				if m.TransactionStatus != constant.PENDING {
					fromTo = append(fromTo, to...)
				}

				var buildErr error

				operations, _, buildErr = r.TransactionHandler.BuildOperations(
					msgCtxWithSpan, balances, fromTo, m.TransactionInput, *tran, m.Validate, m.TransactionDate, m.TransactionStatus == constant.NOTED,
				)
				if buildErr != nil {
					libOpentelemetry.HandleSpanError(&msgSpan, "Failed to validate balances", buildErr)

					logger.Errorf("Failed to validate balance: %v", buildErr.Error())

					return
				}
			}

			tran.Source = m.Validate.Sources
			tran.Destination = m.Validate.Destinations
			tran.Operations = operations

			utils.SanitizeAccountAliases(&m.TransactionInput)

			if err := r.TransactionHandler.Command.WriteTransactionAsync(
				msgCtxWithSpan, m.OrganizationID, m.LedgerID, &m.TransactionInput, m.Validate, balances, tran,
			); err != nil {
				libOpentelemetry.HandleSpanError(&msgSpan, "Failed sending message to queue", err)

				logger.Errorf("Failed sending message: %s to queue: %v", key, err.Error())

				return
			}

			logger.Infof("Transaction message processed: %s", key)
		}(key, transaction)
	}

	wg.Wait()

	r.Logger.Infof("Total of messagens under %d minute(s) : %d", MessageTimeOfLife, totalMessagesLessThanOneHour)
	r.Logger.Infof("Finished processing total of %d eligible messages", len(messages)-totalMessagesLessThanOneHour)
}
