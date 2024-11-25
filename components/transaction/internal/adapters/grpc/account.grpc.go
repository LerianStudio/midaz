package grpc

import (
	"context"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mgrpc"
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/google/uuid"
)

// Repository provides an interface for gRPC operations related to account in the Ledger.
//
//go:generate mockgen --destination=account.mock.go --package=grpc . Repository
type Repository interface {
	GetAccountsByIds(ctx context.Context, token string, organizationID, ledgerID uuid.UUID, ids []string) (*proto.AccountsResponse, error)
	GetAccountsByAlias(ctx context.Context, token string, organizationID, ledgerID uuid.UUID, aliases []string) (*proto.AccountsResponse, error)
	UpdateAccounts(ctx context.Context, token string, organizationID, ledgerID uuid.UUID, accounts []*proto.Account) (*proto.AccountsResponse, error)
}

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
func (a *AccountGRPCRepository) GetAccountsByIds(ctx context.Context, token string, organizationID, ledgerID uuid.UUID, ids []string) (*proto.AccountsResponse, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.get_accounts_by_ids")
	defer span.End()

	conn, err := a.conn.GetNewClient()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get new client", err)

		return nil, err
	}

	client := proto.NewAccountProtoClient(conn)

	accountsID := &proto.AccountsID{
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		Ids:            ids,
	}

	ctx, spanClientReq := tracer.Start(ctx, "grpc.get_accounts_by_ids.client_request")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanClientReq, "accounts_id_grpc_args", accountsID)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanClientReq, "Failed to convert accountsID from proto struct to JSON string", err)

		return nil, err
	}

	ctx = a.conn.ContextMetadataInjection(ctx, token)

	accountsResponse, err := client.GetAccountsByIds(ctx, accountsID)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanClientReq, "Failed to get accounts by ids", err)

		return nil, err
	}

	spanClientReq.End()

	return accountsResponse, nil
}

// GetAccountsByAlias returns a grpc accounts on ledger bi given aliases.
func (a *AccountGRPCRepository) GetAccountsByAlias(ctx context.Context, token string, organizationID, ledgerID uuid.UUID, aliases []string) (*proto.AccountsResponse, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.get_accounts_by_alias")
	defer span.End()

	conn, err := a.conn.GetNewClient()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get new client", err)

		return nil, err
	}

	client := proto.NewAccountProtoClient(conn)

	accountsAlias := &proto.AccountsAlias{
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		Aliases:        aliases,
	}

	ctx, spanClientReq := tracer.Start(ctx, "grpc.get_accounts_by_alias.client_request")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanClientReq, "accounts_id_grpc_metadata", accountsAlias)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanClientReq, "Failed to convert accountsAlias from proto struct to JSON string", err)

		return nil, err
	}

	ctx = a.conn.ContextMetadataInjection(ctx, token)

	accountsResponse, err := client.GetAccountsByAliases(ctx, accountsAlias)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanClientReq, "Failed to get accounts by aliases", err)

		return nil, err
	}

	spanClientReq.End()

	return accountsResponse, nil
}

// UpdateAccounts update a grpc accounts on ledger.
func (a *AccountGRPCRepository) UpdateAccounts(ctx context.Context, token string, organizationID, ledgerID uuid.UUID, accounts []*proto.Account) (*proto.AccountsResponse, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.update_accounts")
	defer span.End()

	conn, err := a.conn.GetNewClient()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get new client", err)

		return nil, err
	}

	client := proto.NewAccountProtoClient(conn)

	accountsRequest := &proto.AccountsRequest{
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		Accounts:       accounts,
	}

	ctx, spanClientReq := tracer.Start(ctx, "grpc.update_accounts.client_request")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanClientReq, "accounts_request_grpc_metadata", accountsRequest)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanClientReq, "Failed to convert accountsRequest from proto struct to JSON string", err)

		return nil, err
	}

	ctx = a.conn.ContextMetadataInjection(ctx, token)

	accountsResponse, err := client.UpdateAccounts(ctx, accountsRequest)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanClientReq, "Failed to update accounts", err)

		return nil, err
	}

	spanClientReq.End()

	return accountsResponse, nil
}
