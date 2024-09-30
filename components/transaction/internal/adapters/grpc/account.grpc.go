package grpc

import (
	"context"

	"github.com/LerianStudio/midaz/common/mgrpc"
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
)

// AccountGRPCRepository is a gRPC implementation of the account.proto
type AccountGRPCRepository struct {
	conn *mgrpc.GRPCConnection
}

// NewAccountGRPC returns a new instance of AccountGRPCRepository using the given gRPC connection.
func NewAccountGRPC(c *mgrpc.GRPCConnection) *AccountGRPCRepository {
	agrpc := &AccountGRPCRepository{
		conn: c,
	}

	_, err := c.GetNewClient()
	if err != nil {
		panic("Failed to connect gRPC")
	}

	return agrpc
}

func (a *AccountGRPCRepository) GetByIds(ctx context.Context, ids *proto.ManyAccountsID) (*proto.ManyAccountsResponse, error) {
	conn, err := a.conn.GetNewClient()
	if err != nil {
		return nil, err
	}

	account := proto.NewAccountProtoClient(conn)

	aliasD := &proto.AccountAlias{Alias: "@wallet_21712486"}
	aliasC := &proto.AccountAlias{Alias: "@wallet_27744039"}
	aliases := []*proto.AccountAlias{aliasD, aliasC}
	manyAccountsAlias := proto.ManyAccountsAlias{
		Aliases: aliases,
	}

	manyAccountsResponse, _ := account.GetByAlias(ctx, &manyAccountsAlias)

	return manyAccountsResponse, nil
}

func (a *AccountGRPCRepository) GetByAlias(ctx context.Context, ids *proto.ManyAccountsAlias) (*proto.ManyAccountsResponse, error) {
	conn, err := a.conn.GetNewClient()
	if err != nil {
		return nil, err
	}

	account := proto.NewAccountProtoClient(conn)

	aliasD := &proto.AccountAlias{Alias: "@wallet_74571295"}
	aliasC := &proto.AccountAlias{Alias: "@wallet_62552967"}
	aliases := []*proto.AccountAlias{aliasD, aliasC}
	manyAccountsAlias := proto.ManyAccountsAlias{
		Aliases: aliases,
	}

	manyAccountsResponse, err := account.GetByAlias(ctx, &manyAccountsAlias)
	if err != nil {
		return nil, err
	}

	return manyAccountsResponse, nil
}

func (a *AccountGRPCRepository) Update(ctx context.Context, account *proto.UpdateRequest) (*proto.Account, error) {
	return nil, nil
}
