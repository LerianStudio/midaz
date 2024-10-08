package grpc

import (
	"context"

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

	acc, err := ap.Query.ListAccountsByIDs(ctx, uuids)
	if err != nil {
		logger.Errorf("Failed to retrieve Accounts by ids for grpc, Error: %s", err.Error())

		return nil, common.ValidationError{
			Code:    "0001",
			Message: "Failed to retrieve Accounts by ids for grpc",
		}
	}

	var accounts []*proto.Account
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

	acc, err := ap.Query.ListAccountsByAlias(ctx, aliases.GetAliases())
	if err != nil {
		logger.Errorf("Failed to retrieve Accounts by aliases for grpc, Error: %s", err.Error())

		return nil, common.ValidationError{
			Code:    "0001",
			Message: "Failed to retrieve Accounts by aliases for grpc",
		}
	}

	var accounts []*proto.Account
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

	var accounts []*proto.Account
	for _, account := range update.GetAccounts() {
		if common.IsNilOrEmpty(&account.Id) {
			logger.Errorf("Failed to update Accounts because id is empty")

			return nil, common.ValidationError{
				Code:    "0001",
				Message: "Failed to update Accounts because id is empty",
			}
		}

		balance := a.Balance{
			Available: &account.Balance.Available,
			OnHold:    &account.Balance.OnHold,
			Scale:     &account.Balance.Scale,
		}

		acu, err := ap.Command.UpdateAccountByID(ctx, account.Id, &balance)
		if err != nil {
			logger.Errorf("Failed to update balance in Account by id for grpc, Error: %s", err.Error())

			return nil, common.ValidationError{
				Code:    "0002",
				Message: "Failed to update balance in Account by id for grpc",
			}
		}

		accounts = append(accounts, acu.ToProto())
	}

	response := proto.AccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}
