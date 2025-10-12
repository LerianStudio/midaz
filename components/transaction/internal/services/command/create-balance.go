package command

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/jackc/pgx/v5/pgconn"
)

// CreateBalance initializes balance tracking for newly created accounts.
//
// This function is invoked asynchronously when the transaction service receives account
// creation events from the onboarding service via RabbitMQ. It creates the default
// balance record that will track the account's holdings for its associated asset.
//
// Balance Initialization:
// - Creates a "default" balance with zero Available and zero OnHold amounts
// - Sets AllowSending and AllowReceiving to true by default
// - Links the balance to the account via AccountID
// - Uses the account's alias for transaction routing
//
// Event-Driven Architecture:
// 1. Onboarding service creates an account
// 2. Publishes account creation event to RabbitMQ
// 3. Transaction service consumer receives event
// 4. This function creates the balance tracking record
// 5. Account is now ready to participate in transactions
//
// Idempotency:
// - Handles duplicate balance creation gracefully (unique constraint violation)
// - This can occur if messages are redelivered or processed multiple times
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - data: Queue message containing account creation data from onboarding service
//
// Returns:
//   - error: JSON unmarshaling errors or non-duplicate persistence errors
func (uc *UseCase) CreateBalance(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance")
	defer span.End()

	logger.Infof("Initializing the create balance for account id: %v", data.AccountID)

	// Process each account in the queue message (typically one per message)
	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		var account mmodel.Account

		// Deserialize account data from queue message
		err := json.Unmarshal(item.Value, &account)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			return err
		}

		// Initialize default balance with zero amounts
		balance := &mmodel.Balance{
			ID:             libCommons.GenerateUUIDv7().String(),
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

		// Persist balance to PostgreSQL
		err = uc.BalanceRepo.Create(ctx, balance)
		if err != nil {
			var pgErr *pgconn.PgError
			// Handle idempotent creation: balance may already exist from prior processing
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				logger.Infof("Balance already exists: %v", balance.ID)
			} else {
				logger.Errorf("Error creating balance on repo: %v", err)

				return err
			}
		}
	}

	return nil
}
