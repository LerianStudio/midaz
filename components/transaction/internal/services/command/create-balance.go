// Package command implements write operations (commands) for the transaction service.
// This file contains the command for creating a balance.
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

// CreateBalance creates the initial balance for an account from a queue message.
//
// This function is triggered by an asynchronous message from the onboarding service
// when a new account is created. It is responsible for creating the default balance
// record for the account, initializing it with zero amounts, and enabling both
// sending and receiving by default. The operation is idempotent, meaning it will
// not fail if a balance for the account already exists.
//
// Business Rules:
//   - A default balance is created for each new account.
//   - Initial available and on-hold amounts are set to zero.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - data: The queue message containing the account data.
//
// Returns:
//   - error: An error if unmarshaling the message or creating the balance fails.
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
