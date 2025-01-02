package query

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"reflect"
	"time"
)

// GetAccountRedisOrDatabase is responsible to get account on redis or database and lock it!
func (uc *UseCase) GetAccountRedisOrDatabase(ctx context.Context, alorid *account.Accounts, isAlias bool) ([]*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_redis_or_database")
	defer span.End()

	organizationID, err := uuid.Parse(alorid.GetOrganizationId())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to parse OrganizationId to UUID", err)

		logger.Errorf("Failed to parse OrganizationId to UUID, Error: %s", err.Error())

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), organizationID)
	}

	ledgerID, err := uuid.Parse(alorid.GetLedgerId())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to parse LedgerId to UUID", err)

		logger.Errorf("Failed to parse LedgerId to UUID, Error: %s", err.Error())

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), ledgerID)
	}

	accounts := make([]*mmodel.Account, 0)

	for key, value := range alorid.GetAliasId() {
		if value.IsFrom {
			internalKey := organizationID.String() + ":" + ledgerID.String() + ":" + key

			redisAccount, err := uc.RedisRepo.Get(ctx, internalKey)
			if errors.Is(err, redis.Nil) {
				logger.Infof("Account not found on redis, searching on database: %v", internalKey)

				lockInternalKey := "lock:" + internalKey
				logger.Infof("Account acquire lock on redis: %v", internalKey)

				lockAcquired, err := uc.RedisRepo.SetNX(ctx, lockInternalKey, "processing", 60)
				if err != nil {
					logger.Infof("Error to acquire lock: %v", err)

				}

				if !lockAcquired {
					acc, err := uc.lockNotAcquired(ctx, alorid.OrganizationId, alorid.LedgerId, key, value, isAlias)
					if err != nil {
						return nil, pkg.ValidateBusinessError(constant.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(mmodel.Account{}).Name())
					}

					accounts = append(accounts, acc...)
				}

				if isAlias {
					acc, err := uc.AccountRepo.ListAccountsByAliases(ctx, organizationID, ledgerID, []string{key})
					if err != nil {
						return nil, pkg.ValidateBusinessError(constant.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(mmodel.Account{}).Name())
					}

					accounts = append(accounts, acc...)
				} else {
					acc, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, []uuid.UUID{uuid.MustParse(key)})
					if err != nil {
						return nil, pkg.ValidateBusinessError(constant.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(mmodel.Account{}).Name())
					}

					accounts = append(accounts, acc...)
				}
			} else {
				var acc *mmodel.Account

				err := json.Unmarshal([]byte(redisAccount), &acc)
				if err != nil {
					return nil, pkg.ValidateBusinessError(constant.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(mmodel.Account{}).Name())
				}
				accounts = append(accounts, acc)
			}
		} else { //&& acc.Type == constant.ExternalAccountType {
			logger.Infof("account: %v", key)
		}

	}

	return accounts, nil
}

func (uc *UseCase) putAccountsOnRedis(ctx context.Context, alorid *account.Accounts, oldAccounts []*mmodel.Account) ([]*account.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.put_accounts_on_redis")
	defer span.End()

	newAccounts := make([]*account.Account, 0)

	for _, oa := range oldAccounts {

		balance := &account.Balance{
			Available: *oa.Balance.Available,
			OnHold:    *oa.Balance.OnHold,
			Scale:     *oa.Balance.Scale,
		}

		if value, exists := alorid.AliasId[oa.ID]; exists {
			fromTo := goldModel.Amount{
				Asset: value.Asset,
				Scale: int(value.Scale),
				Value: int(value.Value),
			}

			if value.IsFrom && oa.Type == constant.ExternalAccountType {
				b := goldModel.OperateAmounts(fromTo, balance, constant.DEBIT)
				na := oa.ToProtoChangeBalance(&b)

				jsonString, err := json.Marshal(na)
				if err != nil {
					logger.Error("Err to convert account to string: %v", err)
					return nil, err
				}

				internalKey := alorid.OrganizationId + ":" + alorid.LedgerId + ":" + oa.ID
				err = uc.RedisRepo.Set(ctx, internalKey, string(jsonString), 600)
				if err != nil {
					logger.Error("Err to put account on reids: %v", err)

					return nil, err
				}

				newAccounts = append(newAccounts, na)
			}

		} else if value, exists = alorid.AliasId[*oa.Alias]; exists {
			fromTo := goldModel.Amount{
				Asset: value.Asset,
				Scale: int(value.Scale),
				Value: int(value.Value),
			}

			if value.IsFrom && oa.Type == constant.ExternalAccountType {
				b := goldModel.OperateAmounts(fromTo, balance, constant.DEBIT)
				na := oa.ToProtoChangeBalance(&b)

				jsonString, err := json.Marshal(na)
				if err != nil {
					logger.Error("Err to convert account to string: %v", err)
					return nil, err
				}

				internalKey := alorid.OrganizationId + ":" + alorid.LedgerId + ":" + *oa.Alias
				err = uc.RedisRepo.Set(ctx, internalKey, string(jsonString), 600)
				if err != nil {
					logger.Error("Err to put account on reids: %v", err)

					return nil, err
				}

				newAccounts = append(newAccounts, na)
			}
		}

	}

	return newAccounts, nil
}

func (uc *UseCase) lockNotAcquired(ctx context.Context, organizationID, ledgerID, key string, alias *account.Amount, isAlias bool) ([]*mmodel.Account, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.lock_not_acquired")
	defer span.End()

	accounts := make([]*mmodel.Account, 0)

	logger.Infof("Another process is using this account, waiting while finish it...")
	time.Sleep(5 * time.Second)

	aliases := &account.Accounts{
		OrganizationId: organizationID,
		LedgerId:       ledgerID,
		AliasId: map[string]*account.Amount{
			key: alias,
		},
	}

	acc, err := uc.GetAccountRedisOrDatabase(ctx, aliases, isAlias)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Accounts by aliases for grpc", err)

		logger.Errorf("Failed to retrieve Accounts by aliases for grpc, Error: %s", err.Error())

		return nil, pkg.ValidateBusinessError(constant.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(mmodel.Account{}).Name())
	}

	accounts = append(accounts, acc...)

	return accounts, nil
}
