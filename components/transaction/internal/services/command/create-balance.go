package command

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) CreateBalance(ctx context.Context, data mmodel.Queue) error {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a batch balance operation telemetry entity
	op := uc.Telemetry.NewBalanceOperation("create_batch", data.AccountID.String())

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.Int("queue_data_count", len(data.QueueData)),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	logger.Infof("Initializing the create balance for account id: %v", data.AccountID)

	successCount := 0
	duplicateCount := 0
	errorCount := 0

	for i, item := range data.QueueData {
		// Create an individual balance operation for each item
		itemOp := uc.Telemetry.NewBalanceOperation("create_item", item.ID.String())
		itemOp.WithAttribute("item_index", strconv.Itoa(i))

		// Start tracing for this item operation
		itemCtx := itemOp.StartTrace(ctx)

		// Record systemic metric for this item
		itemOp.RecordSystemicMetric(itemCtx)

		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		var account mmodel.Account

		err := json.Unmarshal(item.Value, &account)
		if err != nil {
			// Record error for this item
			itemOp.RecordError(itemCtx, "unmarshal_error", err)
			itemOp.End(itemCtx, "failed")

			// No need to increment errorCount as we're returning immediately
			return err
		}

		// Add account details as attributes
		itemOp.WithAttributes(
			attribute.String("asset_code", account.AssetCode),
			attribute.String("account_type", account.Type),
		)

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

		// Add balance ID to telemetry
		itemOp.WithAttribute("balance_id", balance.ID)

		err = uc.BalanceRepo.Create(ctx, balance)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				logger.Infof("Balance already exists: %v", balance.ID)

				// Record duplicate status
				itemOp.End(itemCtx, "duplicate")

				// Update duplicate count
				duplicateCount++
			} else {
				// Record error for this item
				itemOp.RecordError(itemCtx, "creation_error", err)
				itemOp.End(itemCtx, "failed")

				logger.Errorf("Error creating balance on repo: %v", err)

				// No need to increment errorCount as we're returning immediately
				return err
			}
		} else {
			// Record business metrics - could add actual balance amount if available
			// Mark operation as successful
			itemOp.End(itemCtx, "success")

			// Update success count
			successCount++
		}
	}

	// Add summary attributes to the batch operation
	op.WithAttributes(
		attribute.Int("success_count", successCount),
		attribute.Int("duplicate_count", duplicateCount),
		attribute.Int("error_count", errorCount),
		attribute.Int("total_count", len(data.QueueData)),
	)

	// End the batch operation
	op.End(ctx, "success")

	return nil
}
