package command

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"time"
)

func (uc *UseCase) CreateBalance(ctx context.Context, data mmodel.Queue) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance")
	defer span.End()

	logger.Infof("Initializing the create balance: %v", data)

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		var account mmodel.Account
		err := json.Unmarshal(item.Value, &account)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			return err
		}

		b := &mmodel.Balance{
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

		err = uc.BalanceRepo.Create(ctx, b)
		if err != nil {
			logger.Errorf("Error creating balance on repo: %v", err)

			return err
		}
	}

	return nil
}
