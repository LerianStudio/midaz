package out

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mgrpc"
	proto "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Repository provides an interface for gRPC operations related to balance in the Transaction component.
//
//go:generate mockgen --destination=balance.grpc_mock.go --package=out . Repository
type Repository interface {
	CreateBalance(ctx context.Context, token string, req *proto.BalanceRequest) (*proto.BalanceResponse, error)
	DeleteAllBalancesByAccountID(ctx context.Context, token string, req *proto.DeleteAllBalancesByAccountIDRequest) error
}

// BalanceGRPCRepository is a gRPC implementation for balance.proto
type BalanceGRPCRepository struct {
	conn *mgrpc.GRPCConnection
}

// NewBalanceGRPC returns a new instance of BalanceGRPCRepository using the given gRPC connection.
func NewBalanceGRPC(c *mgrpc.GRPCConnection) *BalanceGRPCRepository {
	agrpc := &BalanceGRPCRepository{conn: c}

	_, err := c.GetNewClient()
	if err != nil {
		panic("Failed to connect gRPC")
	}

	return agrpc
}

// CreateBalance creates a balance via gRPC using the provided request.
func (b *BalanceGRPCRepository) CreateBalance(ctx context.Context, token string, req *proto.BalanceRequest) (*proto.BalanceResponse, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.create_balance")
	defer span.End()

	conn, err := b.conn.GetNewClient()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get new client", err)

		return nil, err
	}

	client := proto.NewBalanceProtoClient(conn)

	ctxReq, spanClientReq := tracer.Start(ctx, "grpc.create_balance.client_request")
	if err := libOpentelemetry.SetSpanAttributesFromStruct(&spanClientReq, "app.request.payload", req); err != nil {
		libOpentelemetry.HandleSpanError(&spanClientReq, "Failed to convert BalanceRequest to JSON payload", err)

		return nil, err
	}

	// Inject trace context and propagate request_id and authorization (if provided)
	ctxReq = b.conn.ContextMetadataInjection(ctxReq, token)

	resp, err := client.CreateBalance(ctxReq, req)

	spanClientReq.End()

	if err != nil {
		mapped := mgrpc.MapAuthGRPCError(ctxReq, err, constant.ErrAccountCreationFailed.Error(), "Account Creation Failed", "Account could not be created")
		if mapped != err {
			return nil, mapped
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to create balance", err)
		logger.Errorf("gRPC CreateBalance error: %v", err)

		return nil, err
	}

	return resp, nil
}

// DeleteBalance deletes a balance via gRPC using the provided request.
func (b *BalanceGRPCRepository) DeleteAllBalancesByAccountID(ctx context.Context, token string, req *proto.DeleteAllBalancesByAccountIDRequest) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.delete_all_balances_by_account_id")
	defer span.End()

	conn, err := b.conn.GetNewClient()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get new client", err)
		return err
	}

	client := proto.NewBalanceProtoClient(conn)

	ctxReq, spanClientReq := tracer.Start(ctx, "grpc.delete_all_balances_by_account_id.client_request")
	if err := libOpentelemetry.SetSpanAttributesFromStruct(&spanClientReq, "app.request.payload", req); err != nil {
		libOpentelemetry.HandleSpanError(&spanClientReq, "Failed to convert DeleteAllBalancesByAccountIDRequest to JSON payload", err)
		return err
	}

	ctxReq = b.conn.ContextMetadataInjection(ctxReq, token)

	_, err = client.DeleteAllBalancesByAccountID(ctxReq, req)

	spanClientReq.End()

	if err != nil {
		mapped := mgrpc.MapAuthGRPCError(ctxReq, err, constant.ErrAccountBalanceDeletion.Error(), "All Balances Deletion Failed", "All balances could not be deleted")
		if mapped != err {
			return mapped
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to delete all balances by account id", err)
		logger.Errorf("gRPC DeleteAllBalancesByAccountID error: %v", err)

		return err
	}

	return nil
}

// BalanceAdapter wraps BalanceGRPCRepository to implement mbootstrap.BalancePort.
// This adapter translates between the transport-agnostic interface (using native Go types)
// and the gRPC-specific implementation (using protobuf types).
type BalanceAdapter struct {
	grpcRepo *BalanceGRPCRepository
}

// NewBalanceAdapter creates a new BalanceAdapter wrapping the given gRPC connection.
func NewBalanceAdapter(c *mgrpc.GRPCConnection) *BalanceAdapter {
	return &BalanceAdapter{
		grpcRepo: NewBalanceGRPC(c),
	}
}

// CreateBalanceSync implements mbootstrap.BalancePort by converting native types to proto
// and delegating to the gRPC repository.
func (a *BalanceAdapter) CreateBalanceSync(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	// Convert native input to proto request
	req := &proto.BalanceRequest{
		OrganizationId: input.OrganizationID.String(),
		LedgerId:       input.LedgerID.String(),
		AccountId:      input.AccountID.String(),
		Alias:          input.Alias,
		Key:            input.Key,
		AssetCode:      input.AssetCode,
		AccountType:    input.AccountType,
		AllowSending:   input.AllowSending,
		AllowReceiving: input.AllowReceiving,
	}

	// Call gRPC repository (token is empty, auth is handled via context)
	resp, err := a.grpcRepo.CreateBalance(ctx, "", req)
	if err != nil {
		return nil, err
	}

	// Convert proto response to native model
	available, err := decimal.NewFromString(resp.Available)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Available for balance %s: %w", resp.Id, err)
	}

	onHold, err := decimal.NewFromString(resp.OnHold)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OnHold for balance %s: %w", resp.Id, err)
	}

	return &mmodel.Balance{
		ID:             resp.Id,
		Alias:          resp.Alias,
		Key:            resp.Key,
		AssetCode:      resp.AssetCode,
		Available:      available,
		OnHold:         onHold,
		AllowSending:   resp.AllowSending,
		AllowReceiving: resp.AllowReceiving,
	}, nil
}

// DeleteAllBalancesByAccountID implements mbootstrap.BalancePort by converting
// native types to proto and delegating to the gRPC repository.
func (a *BalanceAdapter) DeleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) error {
	req := &proto.DeleteAllBalancesByAccountIDRequest{
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		AccountId:      accountID.String(),
	}

	return a.grpcRepo.DeleteAllBalancesByAccountID(ctx, "", req)
}

// Ensure BalanceAdapter implements mbootstrap.BalancePort at compile time
var _ mbootstrap.BalancePort = (*BalanceAdapter)(nil)
