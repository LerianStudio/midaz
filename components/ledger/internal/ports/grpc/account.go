package grpc

import (
	"context"
	"reflect"

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/common"
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
)

// AccountProto struct contains an account use case for managing account related operations.
type AccountProto struct {
	Command *command.UseCase
	Query   *query.UseCase
	proto.UnimplementedAccountProtoServer
}

// GetAccountsByIds is a method that retrieves Account information by a given ids.
func (ap *AccountProto) GetAccountsByIds(ctx context.Context, ids *proto.AccountsID) (*proto.AccountsResponse, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	uuids := make([]uuid.UUID, len(ids.GetIds()))
	for _, id := range ids.GetIds() {
		uuids = append(uuids, uuid.MustParse(id))
	}

	acc, err := ap.Query.ListAccountsByIDs(ctx, uuid.MustParse(ids.GetOrganizationId()), uuid.MustParse(ids.GetLedgerId()), uuids)
	if err != nil {
		logger.Errorf("Failed to retrieve Accounts by ids for grpc, Error: %s", err.Error())

		return nil, common.ValidateBusinessError(cn.ErrNoAccountsFound, reflect.TypeOf(a.Account{}).Name())
	}

	accounts := make([]*proto.Account, 0)
	for _, ac := range acc {
		accounts = append(accounts, ac.ToProto())
	}

	response := proto.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

// GetAccountsByAliases is a method that retrieves Account information by a given aliases.
func (ap *AccountProto) GetAccountsByAliases(ctx context.Context, aliases *proto.AccountsAlias) (*proto.AccountsResponse, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	acc, err := ap.Query.ListAccountsByAlias(ctx, uuid.MustParse(aliases.GetOrganizationId()), uuid.MustParse(aliases.GetLedgerId()), aliases.GetAliases())
	if err != nil {
		logger.Errorf("Failed to retrieve Accounts by aliases for grpc, Error: %s", err.Error())

		return nil, common.ValidateBusinessError(cn.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(a.Account{}).Name())
	}

	accounts := make([]*proto.Account, 0)
	for _, ac := range acc {
		accounts = append(accounts, ac.ToProto())
	}

	response := proto.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

// UpdateAccounts is a method that update Account balances by a given ids.
func (ap *AccountProto) UpdateAccounts(ctx context.Context, update *proto.AccountsRequest) (*proto.AccountsResponse, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	accounts := make([]*proto.Account, 0)

	uuids := make([]uuid.UUID, 0)

	for _, account := range update.GetAccounts() {
		if common.IsNilOrEmpty(&account.Id) {
			logger.Errorf("Failed to update Accounts because id is empty")

			return nil, common.ValidateBusinessError(cn.ErrNoAccountIDsProvided, reflect.TypeOf(a.Account{}).Name())
		}

		balance := a.Balance{
			Available: &account.Balance.Available,
			OnHold:    &account.Balance.OnHold,
			Scale:     &account.Balance.Scale,
		}

		_, err := ap.Command.UpdateAccountByID(ctx, uuid.MustParse(account.OrganizationId), uuid.MustParse(account.LedgerId), uuid.MustParse(account.Id), &balance)
		if err != nil {
			logger.Errorf("Failed to update balance in Account by id for organizationId %s and ledgerId %s in grpc, Error: %s", account.OrganizationId, account.LedgerId, err.Error())

			return nil, common.ValidateBusinessError(cn.ErrBalanceUpdateFailed, reflect.TypeOf(a.Account{}).Name())
		}

		uuids = append(uuids, uuid.MustParse(account.Id))
	}

	organizationID := update.GetOrganizationId()
	ledgerID := update.GetLedgerId()

	acc, err := ap.Query.ListAccountsByIDs(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuids)
	if err != nil {
		logger.Errorf("Failed to retrieve Accounts by ids for organizationId %s and ledgerId %s in grpc, Error: %s", organizationID, ledgerID, err.Error())

		return nil, common.ValidationError{
			Code:    "0001",
			Message: "Failed to retrieve Accounts by ids for grpc",
		}
	}

	for _, ac := range acc {
		accounts = append(accounts, ac.ToProto())
	}

	response := proto.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}
