package command

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateAdditionalBalance creates a new additional balance.
func (uc *UseCase) CreateAdditionalBalance(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, cbi *mmodel.CreateAdditionalBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_additional_balance")
	defer span.End()

	normalizedKey := strings.ToLower(cbi.Key)

	existingBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, normalizedKey)
	if err != nil {
		var notFound pkg.EntityNotFoundError
		if !errors.As(err, &notFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check if additional balance already exists", err)

			logger.Errorf("Failed to check if additional balance already exists: %v", err)

			return nil, err
		}
	}

	if existingBalance != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Additional balance already exists", nil)

		logger.Infof("Additional balance already exists: %v", cbi.Key)

		return nil, pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, reflect.TypeOf(mmodel.Balance{}).Name(), cbi.Key)
	}

	defaultBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, constant.DefaultBalanceKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get default balance", err)

		logger.Errorf("Failed to get default balance: %v", err)

		return nil, err
	}

	if defaultBalance.AccountType == constant.ExternalAccountType {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Additional balance not allowed for external account type", nil)

		return nil, pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, reflect.TypeOf(mmodel.Balance{}).Name(), defaultBalance.Alias)
	}

	additionalBalance := &mmodel.Balance{
		ID:             libCommons.GenerateUUIDv7().String(),
		Alias:          defaultBalance.Alias,
		Key:            normalizedKey,
		OrganizationID: defaultBalance.OrganizationID,
		LedgerID:       defaultBalance.LedgerID,
		AccountID:      defaultBalance.AccountID,
		AssetCode:      defaultBalance.AssetCode,
		AccountType:    defaultBalance.AccountType,
		AllowSending:   cbi.AllowSending == nil || *cbi.AllowSending,
		AllowReceiving: cbi.AllowReceiving == nil || *cbi.AllowReceiving,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err = uc.BalanceRepo.Create(ctx, additionalBalance)
	if err != nil {
		// Migration 032 adds a unique balance key index. If another pod wins the
		// race after the precheck, return the same business error as the precheck.
		if isBalanceKeyUniqueViolation(err) {
			berr := pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, reflect.TypeOf(mmodel.Balance{}).Name(), normalizedKey)
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Additional balance already exists", berr)

			logger.Warnf("Additional balance already exists: %v", berr)

			return nil, berr
		}
		if isPostgresUniqueViolation(err) {
			libOpentelemetry.HandleSpanEvent(&span, "Additional balance unique constraint violation")

			logger.Warnf("Additional balance unique constraint violation")

			return nil, err
		}

		logger.Errorf("Error creating additional balance on repo: %v", err)

		return nil, err
	}

	return additionalBalance, nil
}
