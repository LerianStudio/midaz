package bootstrap

import (
	"encoding/json"
	"fmt"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"golang.org/x/net/context"
	"strings"
	"time"
)

// CronConsumer represents cron consumer.
type CronConsumer struct {
	UseCase   *command.UseCase
	Logger    mlog.Logger
	Telemetry *mopentelemetry.Telemetry
	Timer     time.Duration
}

// NewCronConsumer create a new instance of NewCronConsumer.
func NewCronConsumer(logger mlog.Logger, telemetry *mopentelemetry.Telemetry, useCase *command.UseCase) *CronConsumer {
	consumer := &CronConsumer{
		UseCase:   useCase,
		Logger:    logger,
		Telemetry: telemetry,
		Timer:     10 * time.Second,
	}

	return consumer
}

// Run starts cron consumer.
func (cc *CronConsumer) Run(l *pkg.Launcher) error {
	ctx := context.Background()

	tracer := pkg.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "run.cron_consumer")
	defer span.End()

	ticker := time.NewTicker(cc.Timer)
	defer ticker.Stop()

	var organizationID, ledgerID uuid.UUID
	aliases := make([]string, 0)
	balances := make([]mmodel.Balance, 0)

	for range ticker.C {
		keys, err := cc.UseCase.RedisRepo.Scan(ctx, "lock:*")
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get redis locks", err)

			cc.Logger.Error("Failed to get redis locks")
		}

		if len(keys) > 0 {
			values, err := cc.UseCase.RedisRepo.MGet(ctx, keys)
			if err != nil {
				mopentelemetry.HandleSpanError(&span, "Failed to get redis locks", err)

				cc.Logger.Error("Failed to get redis locks")
			}

			for i, key := range keys {
				var balance mmodel.Balance

				var balanceJSON string
				switch v := values[i].(type) {
				case string:
					balanceJSON = v
				case []byte:
					balanceJSON = string(v)
				default:
					err = fmt.Errorf("unexpected result type from Redis: %T", key)

					cc.Logger.Warnf("Warning: %v", err)
				}

				if err := json.Unmarshal([]byte(balanceJSON), &balance); err != nil {
					mopentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

					cc.Logger.Errorf("Error to Deserialization json: %v", err)
				}

				parts := strings.Split(key, ":")

				organizationID = uuid.MustParse(parts[1])
				ledgerID = uuid.MustParse(parts[2])
				aliases = append(aliases, parts[3])
				balance.Alias = parts[3]
				balances = append(balances, balance)

				cc.Logger.Infof("Chave encontrada: %v", key)

				continue
			}

			err = cc.UseCase.BalanceRepo.SelectForUpdateNew(ctx, organizationID, ledgerID, aliases, balances)
			if err != nil {
				mopentelemetry.HandleSpanError(&span, "Error to SelectForUpdate", err)

				cc.Logger.Errorf("Error to SelectForUpdate: %v", err)
			}
		}
	}

	return nil
}
