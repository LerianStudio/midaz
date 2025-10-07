// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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

// CreateBalance creates initial balances for accounts received from the onboarding service.
//
// This method is called asynchronously when the onboarding service publishes account
// creation events to RabbitMQ. It:
// 1. Unmarshals account data from the queue message
// 2. Creates a default balance entry for each account
// 3. Initializes balance with zero available and on-hold amounts
// 4. Sets allow_sending and allow_receiving flags to true
// 5. Handles duplicate balance creation gracefully (idempotent)
//
// Business Rules:
//   - Each account gets one default balance per asset code
//   - Balance alias matches account alias
//   - Initial available and on-hold amounts are zero
//   - Both sending and receiving are enabled by default
//   - Duplicate balance creation is logged but not treated as error (idempotent)
//
// Queue Message Format:
//   - Contains account ID and array of account data
//   - Each account has ID, alias, organization_id, ledger_id, asset_code, type
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - data: Queue message containing account data
//
// Returns:
//   - error: nil on success, error if balance creation fails
//
// OpenTelemetry: Creates span "command.create_balance"
func (uc *UseCase) CreateBalance(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance")
	defer span.End()

	logger.Infof("Initializing the create balance for account id: %v", data.AccountID)

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		var account mmodel.Account

		err := json.Unmarshal(item.Value, &account)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			return err
		}

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

		err = uc.BalanceRepo.Create(ctx, balance)
		if err != nil {
			var pgErr *pgconn.PgError
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
