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

// GetAccountsByIds returns a grpc accounts on ledger bi given ids.
func (a *AccountGRPCRepository) GetAccountsByIds(ctx context.Context, ids []string) (*proto.AccountsResponse, error) {
	conn, err := a.conn.GetNewClient()
	if err != nil {
		return nil, err
	}

	client := proto.NewAccountProtoClient(conn)

	accountsID := &proto.AccountsID{
		Ids: ids,
	}

	accountsResponse, err := client.GetAccountsByIds(ctx, accountsID)
	if err != nil {
		return nil, err
	}

	return accountsResponse, nil
}

// GetAccountsByAlias returns a grpc accounts on ledger bi given aliases.
func (a *AccountGRPCRepository) GetAccountsByAlias(ctx context.Context, aliases []string) (*proto.AccountsResponse, error) {
	conn, err := a.conn.GetNewClient()
	if err != nil {
		return nil, err
	}

	client := proto.NewAccountProtoClient(conn)

	accountsAlias := &proto.AccountsAlias{
		Aliases: aliases,
	}

	accountsResponse, err := client.GetAccountsByAliases(ctx, accountsAlias)
	if err != nil {
		return nil, err
	}

	return accountsResponse, nil
}

// UpdateAccounts update a grpc accounts on ledger.
func (a *AccountGRPCRepository) UpdateAccounts(ctx context.Context, accounts []*proto.Account) (*proto.AccountsResponse, error) {
	conn, err := a.conn.GetNewClient()
	if err != nil {
		return nil, err
	}

	client := proto.NewAccountProtoClient(conn)

	accountsRequest := &proto.AccountsRequest{
		Accounts: accounts,
	}

	accountsResponse, err := client.UpdateAccounts(ctx, accountsRequest)
	if err != nil {
		return nil, err
	}

	return accountsResponse, nil
}
