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
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/jackc/pgx/v5/pgconn"
)

// CreateBalance creates default balances for accounts from a queue message.
//
// This function processes balance creation messages from RabbitMQ, typically
// triggered when new accounts are created. Each account needs a default
// balance to participate in transactions.
//
// Balance Creation Process:
//
//	Step 1: Parse Queue Message
//	  - Deserialize account data from queue message
//	  - Extract account properties for balance creation
//
//	Step 2: Build Balance Record
//	  - Generate UUIDv7 for the new balance
//	  - Set alias from account alias
//	  - Initialize with zero Available and OnHold amounts
//	  - Enable sending and receiving by default
//
//	Step 3: Persist Balance
//	  - Store balance in PostgreSQL
//	  - Handle duplicate key (balance already exists) gracefully
//
// Default Balance Properties:
//
//	{
//	  "alias": "<account_alias>",
//	  "key": "default",
//	  "available": 0,
//	  "on_hold": 0,
//	  "allow_sending": true,
//	  "allow_receiving": true
//	}
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - data: Queue message containing account data
//
// Returns:
//   - error: Deserialization or database error
//
// Error Scenarios:
//   - JSON unmarshal error: Corrupted queue message
//   - Database errors: PostgreSQL unavailable
//   - Unique violation (handled): Balance already exists, logged and skipped
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

// CreateBalanceSync creates a new balance synchronously with validation.
//
// Unlike CreateBalance (which processes queue messages), this function
// creates balances directly from API requests with comprehensive validation.
// It supports creating both default balances and additional balances.
//
// Validation Process:
//
//	Step 1: Normalize Key
//	  - Convert key to lowercase
//	  - Trim whitespace
//
//	Step 2: Default Balance Check (if key != "default")
//	  - Verify default balance exists for this account
//	  - Additional balances require a default balance first
//
//	Step 3: Account Type Validation (if key != "default")
//	  - External accounts cannot have additional balances
//	  - Only internal accounts support multiple balances
//
//	Step 4: Duplicate Key Check
//	  - Verify no balance exists with the same key
//	  - Return ErrDuplicatedAliasKeyValue if duplicate
//
//	Step 5: Create Balance
//	  - Build balance record with provided properties
//	  - Persist to PostgreSQL
//
// Balance Key Concept:
//
// Each account can have multiple balances, distinguished by keys:
//   - "default": Primary balance, required for all accounts
//   - Custom keys: Additional balances for segregated funds (e.g., "escrow", "pending")
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - input: Balance creation input with all required properties
//
// Returns:
//   - *mmodel.Balance: Created balance record
//   - error: Validation or database error
//
// Error Scenarios:
//   - ErrDefaultBalanceNotFound: Trying to create additional balance without default
//   - ErrAdditionalBalanceNotAllowed: External account type
//   - ErrDuplicatedAliasKeyValue: Balance key already exists
//   - Database errors: PostgreSQL unavailable
func (uc *UseCase) CreateBalanceSync(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance_sync")
	defer span.End()

	normalizedKey := strings.ToLower(strings.TrimSpace(input.Key))

	if normalizedKey != constant.DefaultBalanceKey {
		existsDefault, err := uc.BalanceRepo.ExistsByAccountIDAndKey(ctx, input.OrganizationID, input.LedgerID, input.AccountID, constant.DefaultBalanceKey)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check default balance existence", err)

			logger.Errorf("Failed to check default balance existence: %v", err)

			return nil, err
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

		return nil, err
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

		return nil, err
	}

	return newBalance, nil
}
