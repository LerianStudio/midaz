package account

import (
	"context"

	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
)

// Repository provides an interface for gRPC operations related to account in the Ledger.
//
//go:generate mockgen --destination=../../gen/mock/account/account_mock.go --package=mock . Repository
type Repository interface {
	GetAccountsByIds(ctx context.Context, token string, ids []string) (*proto.AccountsResponse, error)
	GetAccountsByAlias(ctx context.Context, token string, aliases []string) (*proto.AccountsResponse, error)
	UpdateAccounts(ctx context.Context, token string, accounts []*proto.Account) (*proto.AccountsResponse, error)
}
