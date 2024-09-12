package grcp

import (
	"context"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	proto "github.com/LerianStudio/midaz/components/ledger/proto/account"
	"github.com/google/uuid"
)

type AccountService struct {
	proto.UnimplementedAccountServiceServer
	Command *command.UseCase
	Query   *query.UseCase
}

func NewAccountService() *AccountService {
	return &AccountService{}
}

func (as *AccountService) GetByIds(ctx context.Context, ids *proto.ManyAccountsID) (*proto.ManyAccountsResponse, error) {
	logger := mlog.NewLoggerFromContext(ctx)

	uuids := make([]uuid.UUID, len(ids.Ids))
	for i, id := range ids.Ids {
		uuids[i] = uuid.MustParse(id.Id)
	}

	acc, err := as.Query.ListAccountsByIDs(ctx, uuids)
	if err != nil {
		logger.Errorf("Failed to retrieve Accounts by ids for grpc, Error: %s", err.Error())
		return nil, err
	}

	var accounts []*proto.Account
	for _, a := range acc {
		ac := proto.Account{
			ID:               a.ID,
			Alias:            *a.Alias,
			AvailableBalance: *a.Balance.Available,
			OnHoldBalance:    *a.Balance.OnHold,
			BalanceScale:     *a.Balance.Scale,
			AllowSending:     a.Status.AllowSending,
			AllowReceiving:   a.Status.AllowReceiving,
		}
		accounts = append(accounts, &ac)
	}

	response := proto.ManyAccountsResponse{
		Accounts: accounts,
	}

	return &response, nil
}

func (account *AccountService) GetByAlias(ctx context.Context, alias *proto.ManyAccountsAlias) (*proto.ManyAccountsResponse, error) {
	return nil, nil
}

func (account *AccountService) Update(ctx context.Context, update *proto.UpdateRequest) (*proto.Account, error) {
	return nil, nil
}

func (account *AccountService) GetByFilters(ctx context.Context, filter *proto.GetByFiltersRequest) (*proto.ManyAccountsResponse, error) {
	return nil, nil
}
