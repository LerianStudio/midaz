package grcp

import (
	"context"

	proto "github.com/LerianStudio/midaz/components/ledger/proto/account"
)

type Account struct {
	proto.UnimplementedAccountServiceServer
}

func NewAccount() *Account {
	return &Account{}
}

func (account *Account) GetByIds(ctx context.Context, ids *proto.ManyAccountsID) (*proto.ManyAccountsResponse, error) {
	return nil, nil
}

func (account *Account) GetByAlias(ctx context.Context, alias *proto.ManyAccountsAlias) (*proto.ManyAccountsResponse, error) {
	return nil, nil
}

func (account *Account) Update(ctx context.Context, update *proto.UpdateRequest) (*proto.Account, error) {
	return nil, nil
}

func (account *Account) GetByFilters(ctx context.Context, filter *proto.GetByFiltersRequest) (*proto.ManyAccountsResponse, error) {
	return nil, nil
}
