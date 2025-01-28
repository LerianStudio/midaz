package query

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"reflect"
	"strings"
)

// GetAccountsLedger methods responsible to get accounts on ledger by gRpc.
func (uc *UseCase) GetAccountsLedger(ctx context.Context, logger mlog.Logger, organizationID, ledgerID uuid.UUID, input []string) ([]*mmodel.Account, error) {
	span := trace.SpanFromContext(ctx)

	var uuids []uuid.UUID

	var invalidUUIDs []string

	var aliases []string

	for _, item := range input {
		if pkg.IsUUID(item) {
			parsedUUID, err := uuid.Parse(item)

			if err != nil {
				invalidUUIDs = append(invalidUUIDs, item)
				continue
			} else {
				uuids = append(uuids, parsedUUID)
			}
		} else {
			aliases = append(aliases, item)
		}
	}

	if len(invalidUUIDs) > 0 {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), strings.Join(invalidUUIDs, ", "))
	}

	var accounts []*mmodel.Account

	if len(uuids) > 0 {
		acc, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, uuids)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get account by ids gRPC on Ledger", err)

			logger.Error("Failed to get account gRPC by ids on Ledger", err.Error())

			return nil, err
		}

		accounts = append(accounts, acc...)
	}

	if len(aliases) > 0 {
		acc, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, aliases)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get account by alias gRPC on Ledger", err)

			logger.Error("Failed to get account by alias gRPC on Ledger", err.Error())

			return nil, err
		}

		accounts = append(accounts, acc...)
	}

	return accounts, nil
}
