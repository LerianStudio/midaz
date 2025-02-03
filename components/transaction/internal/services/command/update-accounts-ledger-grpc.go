package command

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// UpdateAccounts methods that is responsible to update accounts on ledger by gRpc.
func (uc *UseCase) UpdateAccounts(ctx context.Context, logger mlog.Logger, validate goldModel.Responses, token string, organizationID, ledgerID uuid.UUID, hash string, accounts []*account.Account) error {
	span := trace.SpanFromContext(ctx)

	e := make(chan error)
	result := make(chan []*account.Account)

	var accountsToUpdate []*account.Account

	go goldModel.UpdateAccounts(constant.DEBIT, validate.From, accounts, result, e)
	select {
	case r := <-result:
		accountsToUpdate = append(accountsToUpdate, r...)
	case err := <-e:
		mopentelemetry.HandleSpanError(&span, "Failed to update debit accounts", err)

		return err
	}

	go goldModel.UpdateAccounts(constant.CREDIT, validate.To, accounts, result, e)
	select {
	case r := <-result:
		accountsToUpdate = append(accountsToUpdate, r...)
	case err := <-e:
		mopentelemetry.HandleSpanError(&span, "Failed to update credit accounts", err)

		return err
	}

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload_grpc_update_accounts", accountsToUpdate)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert accountsToUpdate from struct to JSON string", err)

		return err
	}

	uc.UpsertAccountsInCache(ctx, organizationID, ledgerID, validate, hash, accountsToUpdate)

	_, err = uc.AccountGRPCRepo.UpdateAccounts(ctx, token, organizationID, ledgerID, accountsToUpdate)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update accounts gRPC on Ledger", err)

		logger.Error("Failed to update accounts gRPC on Ledger", err.Error())

		return err
	}

	return nil
}

// UpsertAccountsInCache adds or updates accounts in the cache for a given organization and ledger.
// It uses account aliases and IDs as keys to store the accounts for a specified time-to-live (TTL).
// Errors encountered during the cache operation are logged, and spans are updated for traceability.
func (uc *UseCase) UpsertAccountsInCache(ctx context.Context, organizationID, ledgerID uuid.UUID, validate goldModel.Responses, hash string, accounts []*account.Account) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "command.upsert_accounts_in_cache")
	defer span.End()

	for _, acc := range accounts {
		newAcc := *acc
		newAcc.Version = newAcc.Version + 1

		am, err := json.Marshal(newAcc)
		if err != nil {
			logger.Warnf("Error marshaling account: %v", err)
		}

		jsonAccount := string(am)

		aliasLockAccount := pkg.LockAccount(organizationID, ledgerID, newAcc.GetAlias())

		err = uc.RedisRepo.Set(ctx, aliasLockAccount, jsonAccount, constant.TimeToSetAccountsInRedis)
		if err != nil && !errors.Is(err, redis.Nil) {
			mopentelemetry.HandleSpanError(&span, "Error setting account in cache", err)

			logger.Errorf("Error setting account by alias in cache: %v", err)
		}

		idLockAccount := pkg.LockAccount(organizationID, ledgerID, newAcc.GetId())

		err = uc.RedisRepo.Set(ctx, idLockAccount, jsonAccount, constant.TimeToSetAccountsInRedis)
		if err != nil && !errors.Is(err, redis.Nil) {
			mopentelemetry.HandleSpanError(&span, "Error setting account by id in cache", err)

			logger.Errorf("Error setting account in cache: %v", err)
		}
	}

	_, spanReleaseLock := tracer.Start(ctx, "command.update_accounts.delete_locks_race_condition")
	uc.DeleteLocks(ctx, organizationID, ledgerID, validate.Aliases, hash)

	spanReleaseLock.End()
}
