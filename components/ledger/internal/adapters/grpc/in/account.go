package in

import (
	"context"
	"reflect"
	"strings"
	"sync"

	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

// AccountProto struct contains an account use case for managing account related operations.
type AccountProto struct {
	Command *command.UseCase
	Query   *query.UseCase
	account.UnimplementedAccountProtoServer
}

// GetAccountsByIds is a method that retrieves Account information by a given ids.
func (ap *AccountProto) GetAccountsByIds(ctx context.Context, ids *account.AccountsID) (*account.AccountsResponse, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.GetAccountsByIds")
	defer span.End()

	organizationUUID, err := uuid.Parse(ids.GetOrganizationId())
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), organizationUUID)
	}

	ledgerUUID, err := uuid.Parse(ids.GetLedgerId())
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), ledgerUUID)
	}

	var invalidUUIDs []string

	uuids := make([]uuid.UUID, len(ids.GetIds()))

	for _, id := range ids.GetIds() {
		parsedUUID, err := uuid.Parse(id)

		if err != nil {
			invalidUUIDs = append(invalidUUIDs, id)
			continue
		} else {
			uuids = append(uuids, parsedUUID)
		}
	}

	if len(invalidUUIDs) > 0 {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), strings.Join(invalidUUIDs, ", "))
	}

	acc, err := ap.Query.ListAccountsByIDs(ctx, organizationUUID, ledgerUUID, uuids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Accounts by ids for grpc", err)

		logger.Errorf("Failed to retrieve Accounts by ids for grpc, Error: %s", err.Error())

		return nil, pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())
	}

	accounts := make([]*account.Account, 0)
	for _, ac := range acc {
		accounts = append(accounts, ac.ToProto())
	}

	response := account.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

// GetAccountsByAliases is a method that retrieves Account information by a given aliases.
func (ap *AccountProto) GetAccountsByAliases(ctx context.Context, aliases *account.AccountsAlias) (*account.AccountsResponse, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.GetAccountsByAliases")
	defer span.End()

	organizationUUID, err := uuid.Parse(aliases.GetOrganizationId())
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), organizationUUID)
	}

	ledgerUUID, err := uuid.Parse(aliases.GetLedgerId())
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), ledgerUUID)
	}

	acc, err := ap.Query.ListAccountsByAlias(ctx, organizationUUID, ledgerUUID, aliases.GetAliases())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Accounts by aliases for grpc", err)

		logger.Errorf("Failed to retrieve Accounts by aliases for grpc, Error: %s", err.Error())

		return nil, pkg.ValidateBusinessError(constant.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(mmodel.Account{}).Name())
	}

	accounts := make([]*account.Account, 0)
	for _, a := range acc {
		accounts = append(accounts, a.ToProto())
	}

	response := account.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

// UpdateAccounts is a method that update Account balances by a given ids.
func (ap *AccountProto) UpdateAccounts(ctx context.Context, update *account.AccountsRequest) (*account.AccountsResponse, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.UpdateAccounts")
	defer span.End()

	accounts := make([]*account.Account, 0)

	uuids := make([]uuid.UUID, 0)

	organizationID, err := uuid.Parse(update.OrganizationId)
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), organizationID)
	}

	ledgerID, err := uuid.Parse(update.LedgerId)
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), ledgerID)
	}

	var wg sync.WaitGroup

	errChan := make(chan error, len(update.GetAccounts()))

	for _, acc := range update.GetAccounts() {
		wg.Add(1)

		go func(acc *account.Account) {
			defer wg.Done()

			if pkg.IsNilOrEmpty(&acc.Id) {
				mopentelemetry.HandleSpanError(&span, "Failed to update Accounts because id is empty", nil)

				logger.Errorf("Failed to update Accounts because id is empty")

				errChan <- pkg.ValidateBusinessError(constant.ErrNoAccountIDsProvided, reflect.TypeOf(mmodel.Account{}).Name())
			}

			accountID, err := uuid.Parse(acc.Id)
			if err != nil {
				errChan <- pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), accountID)
			}

			balance := mmodel.Balance{
				Available: &acc.Balance.Available,
				OnHold:    &acc.Balance.OnHold,
				Scale:     &acc.Balance.Scale,
			}

			_, err = ap.Command.UpdateAccountByID(ctx, organizationID, ledgerID, accountID, &balance)
			if err != nil {
				mopentelemetry.HandleSpanError(&span, "Failed to update balance in Account by id", err)

				logger.Errorf("Failed to update balance in Account by id for organizationId %v and ledgerId %v in grpc, Error: %v", organizationID, ledgerID, err.Error())

				errChan <- pkg.ValidateBusinessError(constant.ErrBalanceUpdateFailed, reflect.TypeOf(mmodel.Account{}).Name())
			}

			uuids = append(uuids, accountID)
		}(acc)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {

			return nil, err
		}
	}

	acc, err := ap.Query.ListAccountsByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Accounts by ids for grpc", err)

		logger.Errorf("Failed to retrieve Accounts by ids for organizationId %s and ledgerId %s in grpc, Error: %s", organizationID, ledgerID, err.Error())

		return nil, pkg.ValidateBusinessError(constant.ErrNoAccountsFound, reflect.TypeOf(mmodel.Account{}).Name())
	}

	for _, ac := range acc {
		accounts = append(accounts, ac.ToProto())
	}

	response := account.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

// UpdateAccountsTrue is a method that update Account balances by a given ids.
func (ap *AccountProto) UpdateAccountsTrue(ctx context.Context, update *account.AccountsRequest) (*account.AccountsResponse, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.UpdateAccounts")
	defer span.End()

	organizationID, err := uuid.Parse(update.OrganizationId)
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), organizationID)
	}

	ledgerID, err := uuid.Parse(update.LedgerId)
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.Account{}).Name(), ledgerID)
	}

	err = ap.Command.UpdateAccounts(ctx, organizationID, ledgerID, update.GetAccounts())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update balance in Account by id", err)

		logger.Errorf("Failed to update balance in Account by id for organizationId %v and ledgerId %v in grpc, Error: %v", organizationID, ledgerID, err.Error())

		return nil, err
	}

	return nil, nil
}
