package account

import (
	"context"

	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
)

// Repository provides an interface for gRPC operations related to account in the Ledger.
//
//go:generate mockgen --destination=../../gen/mock/account/account_mock.go --package=mock . Repository
type Repository interface {
	GetByIds(ctx context.Context, ids *proto.ManyAccountsID) (*proto.ManyAccountsResponse, error)
	GetByAlias(ctx context.Context, ids *proto.ManyAccountsAlias) (*proto.ManyAccountsResponse, error)
	Update(ctx context.Context, account *proto.UpdateRequest) (*proto.Account, error)
}
