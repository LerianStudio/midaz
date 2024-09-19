package grpc

import (
	"context"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	proto "github.com/LerianStudio/midaz/components/ledger/proto/account"
	"github.com/google/uuid"
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

	uuids := make([]uuid.UUID, len(ids.Ids))
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

	accounts := make([]*proto.Account, len(acc))

	for _, ac := range acc {
		account := proto.Account{
			Id:               ac.ID,
			Alias:            *ac.Alias,
			AvailableBalance: *ac.Balance.Available,
			OnHoldBalance:    *ac.Balance.OnHold,
			BalanceScale:     *ac.Balance.Scale,
			AllowSending:     ac.Status.AllowSending,
			AllowReceiving:   ac.Status.AllowReceiving,
		}
		accounts = append(accounts, &account)
	}

	response := proto.ManyAccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

func (ap *AccountProto) GetByAlias(ctx context.Context, aliases *proto.ManyAccountsAlias) (*proto.ManyAccountsResponse, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	als := make([]string, len(aliases.Aliases))
	for i, aA := range aliases.Aliases {
		als[i] = aA.Alias
	}

	acc, err := ap.Query.ListAccountsByAlias(ctx, als)
	if err != nil {
		logger.Errorf("Failed to retrieve Accounts by aliases for grpc, Error: %s", err.Error())

		return nil, common.ValidationError{
			Code:    "0001",
			Message: "Failed to retrieve Accounts by aliases for grpc",
		}
	}

	accounts := make([]*proto.Account, len(acc))

	for _, ac := range acc {
		account := proto.Account{
			Id:               ac.ID,
			Alias:            *ac.Alias,
			AvailableBalance: *ac.Balance.Available,
			OnHoldBalance:    *ac.Balance.OnHold,
			BalanceScale:     *ac.Balance.Scale,
			AllowSending:     ac.Status.AllowSending,
			AllowReceiving:   ac.Status.AllowReceiving,
		}
		accounts = append(accounts, &account)
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
		Available: &update.AvailableBalance,
		OnHold:    &update.OnHoldBalance,
		Scale:     &update.BalanceScale,
	}

	acu, err := ap.Command.UpdateAccountByID(ctx, update.Id, &balance)
	if err != nil {
		logger.Errorf("Failed to update balance in Account by id for grpc, Error: %s", err.Error())

		return nil, common.ValidationError{
			Code:    "0002",
			Message: "Failed to update balance in Account by id for grpc",
		}
	}

	account := proto.Account{
		Id:               acu.ID,
		Alias:            update.Alias,
		AvailableBalance: *acu.Balance.Available,
		OnHoldBalance:    *acu.Balance.OnHold,
		BalanceScale:     *acu.Balance.Scale,
	}

	return &account, nil
}
