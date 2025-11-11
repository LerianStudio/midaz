package out

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mgrpc"
	proto "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
)

// Repository provides an interface for gRPC operations related to balance in the Transaction component.
//
//go:generate mockgen --destination=balance.grpc_mock.go --package=out . Repository
type Repository interface {
	CreateBalance(ctx context.Context, token string, req *proto.BalanceRequest) (*proto.BalanceResponse, error)
	GetBalance(ctx context.Context, token string, req *proto.BalanceRequest) (*proto.BalanceResponse, error)
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

// GetBalance gets a balance via gRPC using the provided request.
func (b *BalanceGRPCRepository) GetBalance(ctx context.Context, token string, req *proto.BalanceRequest) (*proto.BalanceResponse, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.get_balance")
	defer span.End()

	conn, err := b.conn.GetNewClient()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get new client", err)
		return nil, err
	}

	client := proto.NewBalanceProtoClient(conn)

	ctxReq, spanClientReq := tracer.Start(ctx, "grpc.get_balance.client_request")
	if err := libOpentelemetry.SetSpanAttributesFromStruct(&spanClientReq, "app.request.payload", req); err != nil {
		libOpentelemetry.HandleSpanError(&spanClientReq, "Failed to convert BalanceRequest to JSON payload", err)
		return nil, err
	}

	// Inject trace context and propagate request_id and authorization (if provided)
	ctxReq = b.conn.ContextMetadataInjection(ctxReq, token)

	resp, err := client.GetBalance(ctxReq, req)
	spanClientReq.End()
	if err != nil {
		mapped := mgrpc.MapAuthGRPCError(ctxReq, err, constant.ErrNoBalancesFound.Error(), "Balance Not Found", "Balance could not be found")
		if mapped != err {
			return nil, mapped
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to get balance", err)
		logger.Errorf("gRPC GetBalance error: %v", err)

		return nil, err
	}

	return resp, nil
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
