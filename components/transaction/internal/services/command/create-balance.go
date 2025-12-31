package command

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/jackc/pgx/v5/pgconn"
)

// CreateBalance processes a queue message to create balances for accounts.
// It unmarshals account data and creates corresponding balance records.
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

			return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
		}

		assert.That(account.ID == item.ID.String(),
			"account ID must match queue data item ID",
			"account_id", account.ID,
			"queue_item_id", item.ID)
		assert.That(account.ID == data.AccountID.String(),
			"account ID must match queue account ID",
			"account_id", account.ID,
			"queue_account_id", data.AccountID)
		assert.That(account.OrganizationID == data.OrganizationID.String(),
			"account organization ID must match queue organization ID",
			"account_organization_id", account.OrganizationID,
			"queue_organization_id", data.OrganizationID)
		assert.That(account.LedgerID == data.LedgerID.String(),
			"account ledger ID must match queue ledger ID",
			"account_ledger_id", account.LedgerID,
			"queue_ledger_id", data.LedgerID)
		assert.NotEmpty(account.AssetCode,
			"account asset code must not be empty",
			"account_id", account.ID)

		// Invariant: account must have an alias for balance creation
		assert.NotNil(account.Alias, "account.Alias must not be nil for balance creation",
			"accountID", account.ID,
			"assetCode", account.AssetCode)

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

				return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
			}
		}
	}

	return nil
}

// CreateBalanceSync creates a new balance synchronously using the request-supplied properties.
// If key != "default", it validates that the default balance exists and that the account type allows additional balances.
// This method implements mbootstrap.BalancePort, allowing the transaction module
// to be used directly by the onboarding module in unified ledger mode.
func (uc *UseCase) CreateBalanceSync(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance_sync")
	defer span.End()

	normalizedKey := strings.ToLower(strings.TrimSpace(input.Key))

	logger.Infof("Creating balance for account id: %v with key: %v", input.AccountID, normalizedKey)

	if normalizedKey != constant.DefaultBalanceKey {
		existsDefault, err := uc.BalanceRepo.ExistsByAccountIDAndKey(ctx, input.OrganizationID, input.LedgerID, input.AccountID, constant.DefaultBalanceKey)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check default balance existence", err)

			logger.Errorf("Failed to check default balance existence: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
		}

		if !existsDefault {
			berr := pkg.ValidateBusinessError(constant.ErrDefaultBalanceNotFound, reflect.TypeOf(mmodel.Balance{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Default balance not found", berr)

			logger.Errorf("Default balance not found: %v", berr)

			return nil, berr
		}

		// Validate additional balance not allowed for external account type
		if input.AccountType == constant.ExternalAccountType {
			err := pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, reflect.TypeOf(mmodel.Balance{}).Name(), input.Alias)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Additional balance not allowed for external account type", err)

			logger.Errorf("Additional balance not allowed for external account type: %v", err)

			return nil, err
		}
	}

	existsKey, err := uc.BalanceRepo.ExistsByAccountIDAndKey(ctx, input.OrganizationID, input.LedgerID, input.AccountID, normalizedKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check if balance already exists", err)

		logger.Errorf("Failed to check if balance already exists: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	if existsKey {
		err := pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, reflect.TypeOf(mmodel.Balance{}).Name(), normalizedKey)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance key already exists", err)

		logger.Errorf("Balance key already exists: %v", err)

		return nil, err
	}

	newBalance := &mmodel.Balance{
		ID:             libCommons.GenerateUUIDv7().String(),
		Alias:          input.Alias,
		Key:            normalizedKey,
		OrganizationID: input.OrganizationID.String(),
		LedgerID:       input.LedgerID.String(),
		AccountID:      input.AccountID.String(),
		AssetCode:      input.AssetCode,
		AccountType:    input.AccountType,
		AllowSending:   input.AllowSending,
		AllowReceiving: input.AllowReceiving,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := uc.BalanceRepo.Create(ctx, newBalance); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create balance on repo", err)

		logger.Errorf("Failed to create balance on repo: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	return newBalance, nil
}
