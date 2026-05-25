// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	// CreateAdditionalBalance creates a new additional balance.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

const (
	balanceAccountKeyUniqueIndex = "idx_unique_balance_account_key"
	balanceAliasKeyUniqueIndex   = "idx_unique_balance_alias_key"
)

func (uc *UseCase) CreateAdditionalBalance(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, cbi *mmodel.CreateAdditionalBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_additional_balance")
	defer span.End()

	normalizedKey := strings.ToLower(cbi.Key)

	existingBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, normalizedKey)
	if err != nil {
		var notFound pkg.EntityNotFoundError
		if !errors.As(err, &notFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check if additional balance already exists", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to check if additional balance already exists: %v", err))

			return nil, err
		}
	}

	if existingBalance != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Additional balance already exists", nil)

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Additional balance already exists: %v", cbi.Key))

		return nil, pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, reflect.TypeOf(mmodel.Balance{}).Name(), cbi.Key)
	}

	defaultBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, constant.DefaultBalanceKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get default balance", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get default balance: %v", err))

		return nil, err
	}

	if defaultBalance.AccountType == constant.ExternalAccountType {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Additional balance not allowed for external account type", nil)

		return nil, pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, reflect.TypeOf(mmodel.Balance{}).Name(), defaultBalance.Alias)
	}

	additionalBalance := &mmodel.Balance{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
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

	created, err := uc.BalanceRepo.Create(ctx, additionalBalance)
	if err != nil {
		// Migration 032 adds a unique balance key index. If another pod wins the
		// race after the precheck, return the same business error as the precheck.
		if isBalanceKeyUniqueViolation(err) {
			berr := pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, reflect.TypeOf(mmodel.Balance{}).Name(), normalizedKey)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Additional balance already exists", berr)

			logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Additional balance already exists: %v", berr))

			return nil, berr
		}
		if isPostgresUniqueViolation(err) {
			libOpentelemetry.HandleSpanEvent(span, "Additional balance unique constraint violation")

			logger.Log(ctx, libLog.LevelWarn, "Additional balance unique constraint violation")

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating additional balance on repo: %v", err))

		return nil, err
	}

	return created, nil
}

func isBalanceKeyUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr == nil || pgErr.Code != constant.UniqueViolationCode {
		return false
	}

	return pgErr.ConstraintName == balanceAccountKeyUniqueIndex || pgErr.ConstraintName == balanceAliasKeyUniqueIndex
}

func isPostgresUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr != nil && pgErr.Code == constant.UniqueViolationCode
}
