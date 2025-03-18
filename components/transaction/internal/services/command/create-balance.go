package command

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
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

	// Expanded operation metrics with more context
	uc.RecordBalanceMetric(ctx, "create_attempt", data.AccountID.String(),
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.Int("queue_data_count", len(data.QueueData)))

	logger.Infof("Initializing the create balance for account id: %v", data.AccountID)

	successCount := 0
	duplicateCount := 0
	errorCount := 0

	for i, item := range data.QueueData {
		itemStartTime := time.Now()
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		var account mmodel.Account

		err := json.Unmarshal(item.Value, &account)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to unmarshal account data", err)
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			// Record error metric with item index
			uc.RecordEntityError(ctx, "balance", "unmarshal_error", item.ID.String(),
				attribute.String("error_detail", err.Error()),
				attribute.Int("item_index", i))

			// Record balance duration with error status
			uc.recordEntityDuration(ctx, "balance", startTime, "create", "error",
				attribute.String("account_id", item.ID.String()),
				attribute.String("error", "unmarshal_error"))

			// Increment error count
			errorCount++

			return err
		}

		// Record detailed account information for metrics
		uc.RecordBalanceMetric(ctx, "create_processing", account.ID,
			attribute.String("asset_code", account.AssetCode),
			attribute.String("account_type", account.Type),
			attribute.String("item_index", strconv.Itoa(i)))

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
				uc.RecordBalanceMetric(ctx, "already_exists", account.ID,
					attribute.String("balance_id", balance.ID),
					attribute.String("asset_code", account.AssetCode),
					attribute.String("account_type", account.Type),
					attribute.String("organization_id", account.OrganizationID),
					attribute.String("ledger_id", account.LedgerID))

				// Record item duration
				uc.recordEntityDuration(ctx, "balance", itemStartTime, "create_item", "duplicate",
					attribute.String("balance_id", balance.ID),
					attribute.String("account_id", account.ID),
					attribute.String("asset_code", account.AssetCode))

				// Increment duplicate count
				duplicateCount++
			} else {
				mopentelemetry.HandleSpanError(&span, "Failed to create balance", err)
				logger.Errorf("Error creating balance on repo: %v", err)
				logger.Infof("Error creating balance on repo")

				// Record error metric with more details
				uc.RecordEntityError(ctx, "balance", "creation_error", balance.ID,
					attribute.String("account_id", account.ID),
					attribute.String("asset_code", account.AssetCode),
					attribute.String("error_detail", err.Error()),
					attribute.Int("item_index", i))

				// Record balance duration with error status
				uc.recordEntityDuration(ctx, "balance", startTime, "create", "error",
					attribute.String("account_id", account.ID),
					attribute.String("asset_code", account.AssetCode),
					attribute.String("error", "creation_error"))

				// Increment error count
				errorCount++

				return err
			}
		} else {
			// Record business metric for balance creation success
			uc.RecordBalanceMetric(ctx, "create_success", account.ID,
				attribute.String("balance_id", balance.ID),
				attribute.String("asset_code", account.AssetCode),
				attribute.String("account_type", account.Type),
				attribute.String("organization_id", account.OrganizationID),
				attribute.String("ledger_id", account.LedgerID))

			// Record item duration
			uc.recordEntityDuration(ctx, "balance", itemStartTime, "create_item", "success",
				attribute.String("balance_id", balance.ID),
				attribute.String("account_id", account.ID),
				attribute.String("asset_code", account.AssetCode))

			// Increment success count
			successCount++
		}
	}

	// Record summary metrics
	uc.RecordBalanceMetric(ctx, "create_summary", data.AccountID.String(),
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.Int("success_count", successCount),
		attribute.Int("duplicate_count", duplicateCount),
		attribute.Int("error_count", errorCount),
		attribute.Int("total_count", len(data.QueueData)))

	// Record balance duration with success status and summary data
	uc.recordEntityDuration(ctx, "balance", startTime, "create", "success",
		attribute.String("account_id", data.AccountID.String()),
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.Int("success_count", successCount),
		attribute.Int("duplicate_count", duplicateCount))

	return nil
}
