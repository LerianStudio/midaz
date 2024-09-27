package grpc

import (
	"context"
	"github.com/LerianStudio/midaz/common/mgrpc"
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
	"google.golang.org/grpc"
)

// AccountGRPC is a gRPC implementation of the account.proto.
type AccountGRPC struct {
	conn *grpc.ClientConn
}

func NewAccountGRPC(c mgrpc.GRPCConnection) *AccountGRPC {
	err := c.Connect()
	if err != nil {
		panic("Failed to connect gRPC")
	}

	return &AccountGRPC{
		conn: c.Conn,
	}
}

func (a *AccountGRPC) GetByIds(ctx context.Context, ids *proto.ManyAccountsID) *proto.ManyAccountsResponse {
	account := proto.NewAccountProtoClient(a.conn)

	aliasD := &proto.AccountAlias{Alias: "@wallet_97666534"}
	aliasC := &proto.AccountAlias{Alias: "@wallet_86555196"}
	aliases := []*proto.AccountAlias{aliasD, aliasC}
	manyAccountsAlias := proto.ManyAccountsAlias{
		Aliases: aliases,
	}

	manyAccountsResponse, _ := account.GetByAlias(ctx, &manyAccountsAlias)

	return manyAccountsResponse
}

func (a *AccountGRPC) GetByAlias(ctx context.Context, ids *proto.ManyAccountsAlias) *proto.ManyAccountsResponse {
	account := proto.NewAccountProtoClient(a.conn)

	aliasD := &proto.AccountAlias{Alias: "@wallet_97666534"}
	aliasC := &proto.AccountAlias{Alias: "@wallet_86555196"}
	aliases := []*proto.AccountAlias{aliasD, aliasC}
	manyAccountsAlias := proto.ManyAccountsAlias{
		Aliases: aliases,
	}

	manyAccountsResponse, _ := account.GetByAlias(ctx, &manyAccountsAlias)

	return manyAccountsResponse
}

func (a *AccountGRPC) Update(ctx context.Context, account *proto.UpdateRequest) *proto.Account {
	return nil
}
