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

// GetByIds is a method that retrieves Account information by a given ids.
func (ap *AccountProto) GetByIds(ctx context.Context, ids *proto.ManyAccountsID) (*proto.ManyAccountsResponse, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	var uuids []uuid.UUID
	for i, id := range ids.Ids {
		uuids[i] = uuid.MustParse(id.Id)
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

	response := proto.ManyAccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

func (ap *AccountProto) GetByAlias(ctx context.Context, aliases *proto.ManyAccountsAlias) (*proto.ManyAccountsResponse, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	var als []string
	for i, alias := range aliases.Aliases {
		als[i] = alias.Alias
	}

	acc, err := ap.Query.ListAccountsByAlias(ctx, als)
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

	response := proto.ManyAccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

func (ap *AccountProto) Update(ctx context.Context, update *proto.UpdateRequest) (*proto.Account, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	if common.IsNilOrEmpty(&update.Id) {
		logger.Errorf("Failed to update Accounts because id is empty")

		return nil, common.ValidationError{
			Code:    "0001",
			Message: "Failed to update Accounts because id is empty",
		}
	}

	balance := a.Balance{
		Available: &update.Balance.Available,
		OnHold:    &update.Balance.OnHold,
		Scale:     &update.Balance.Scale,
	}

	acu, err := ap.Command.UpdateAccountByID(ctx, update.Id, &balance)
	if err != nil {
		logger.Errorf("Failed to update balance in Account by id for grpc, Error: %s", err.Error())

		return nil, common.ValidationError{
			Code:    "0002",
			Message: "Failed to update balance in Account by id for grpc",
		}
	}

	return acu.ToProto(), nil
}
