package command

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) CreateBalance(ctx context.Context, data mmodel.Queue) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.create_balance")
	defer span.End()

	// Record operation metrics
	uc.recordBusinessMetrics(ctx, "balance_create_attempt",
		attribute.String("account_id", data.AccountID.String()))

	logger.Infof("Initializing the create balance for account id: %v", data.AccountID)

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		var account mmodel.Account

		err := json.Unmarshal(item.Value, &account)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to unmarshal account data", err)
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			// Record error metric
			uc.recordTransactionError(ctx, "unmarshal_error",
				attribute.String("account_id", item.ID.String()),
				attribute.String("error_detail", err.Error()))

			// Record transaction duration with error status
			uc.recordTransactionDuration(ctx, startTime, "create_balance", "error",
				attribute.String("account_id", item.ID.String()),
				attribute.String("error", "unmarshal_error"))

			return err
		}

		balance := &mmodel.Balance{
			ID:             pkg.GenerateUUIDv7().String(),
			Alias:          *account.Alias,
			OrganizationID: account.OrganizationID,
			LedgerID:       account.LedgerID,
			AccountID:      account.ID,
			AssetCode:      account.AssetCode,
			AccountType:    account.Type,
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		err = uc.BalanceRepo.Create(ctx, balance)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				logger.Infof("Balance already exists: %v", balance.ID)

				// Record business metric for balance already exists
				uc.recordBusinessMetrics(ctx, "balance_already_exists",
					attribute.String("balance_id", balance.ID),
					attribute.String("account_id", account.ID),
					attribute.String("asset_code", account.AssetCode))
			} else {
				mopentelemetry.HandleSpanError(&span, "Failed to create balance", err)
				logger.Errorf("Error creating balance on repo: %v", err)
				logger.Infof("Error creating balance on repo")

				// Record error metric
				uc.recordTransactionError(ctx, "balance_creation_error",
					attribute.String("account_id", account.ID),
					attribute.String("asset_code", account.AssetCode),
					attribute.String("error_detail", err.Error()))

				// Record transaction duration with error status
				uc.recordTransactionDuration(ctx, startTime, "create_balance", "error",
					attribute.String("account_id", account.ID),
					attribute.String("asset_code", account.AssetCode),
					attribute.String("error", "balance_creation_error"))

				return err
			}
		} else {
			// Record business metric for balance creation success
			uc.recordBusinessMetrics(ctx, "balance_create_success",
				attribute.String("balance_id", balance.ID),
				attribute.String("account_id", account.ID),
				attribute.String("asset_code", account.AssetCode),
				attribute.String("account_type", account.Type))
		}
	}

	// Record transaction duration with success status
	uc.recordTransactionDuration(ctx, startTime, "create_balance", "success",
		attribute.String("account_id", data.AccountID.String()))

	return nil
}
