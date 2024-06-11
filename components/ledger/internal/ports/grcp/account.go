package grcp

import (
	"context"
	"github.com/google/uuid"

	proto "github.com/LerianStudio/midaz/components/ledger/proto/account"
)

type AccountService struct {
	proto.UnimplementedAccountServiceServer
}

func NewAccountService() *AccountService {
	return &AccountService{}
}

func (account *AccountService) GetByIds(ctx context.Context, ids *proto.ManyAccountsID) (*proto.ManyAccountsResponse, error) {

	a := proto.Account{
		ID:    uuid.NewString(),
		Alias: "Teste",
	}

	var accounts []*proto.Account

	accounts = append(accounts, &a)

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
