// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/loader"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcHealth "google.golang.org/grpc/health"
	grpcHealthV1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type authorizerService struct {
	authorizerv1.UnimplementedBalanceAuthorizerServer

	engine  *engine.Engine
	loader  *loader.PostgresLoader
	pub     publisher.Publisher
	logger  libLog.Logger
	started time.Time
}

func (s *authorizerService) Authorize(_ context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	resp, err := s.engine.Authorize(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorize failed: %v", err)
	}

	return resp, nil
}

func (s *authorizerService) AuthorizeStream(stream grpc.BidiStreamingServer[authorizerv1.AuthorizeRequest, authorizerv1.AuthorizeResponse]) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}

			return err
		}

		resp, err := s.engine.Authorize(req)
		if err != nil {
			return status.Errorf(codes.Internal, "stream authorize failed: %v", err)
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func (s *authorizerService) LoadBalances(ctx context.Context, req *authorizerv1.LoadBalancesRequest) (*authorizerv1.LoadBalancesResponse, error) {
	balances, err := s.loader.LoadBalances(ctx, req.GetOrganizationId(), req.GetLedgerId(), req.GetShardIds())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load balances failed: %v", err)
	}

	loaded := s.engine.UpsertBalances(balances)

	return &authorizerv1.LoadBalancesResponse{
		BalancesLoaded: loaded,
		ShardsLoaded:   int64(len(req.GetShardIds())),
	}, nil
}

func (s *authorizerService) GetBalance(_ context.Context, req *authorizerv1.GetBalanceRequest) (*authorizerv1.GetBalanceResponse, error) {
	balance, ok := s.engine.GetBalance(req.GetOrganizationId(), req.GetLedgerId(), req.GetAccountAlias(), req.GetBalanceKey())
	if !ok {
		return nil, status.Error(codes.NotFound, "balance not found")
	}

	return &authorizerv1.GetBalanceResponse{
		Balance: &authorizerv1.BalanceSnapshot{
			AccountAlias:   balance.AccountAlias,
			BalanceKey:     balance.BalanceKey,
			BalanceId:      balance.ID,
			AccountId:      balance.AccountID,
			AssetCode:      balance.AssetCode,
			AccountType:    balance.AccountType,
			AllowSending:   balance.AllowSending,
			AllowReceiving: balance.AllowReceiving,
			Available:      balance.Available,
			OnHold:         balance.OnHold,
			Scale:          balance.Scale,
			Version:        balance.Version,
		},
	}, nil
}

func (s *authorizerService) PublishBalanceOperations(ctx context.Context, req *authorizerv1.PublishBalanceOperationsRequest) (*authorizerv1.PublishBalanceOperationsResponse, error) {
	if req == nil || len(req.GetPayload()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "payload is required")
	}

	if s.pub == nil {
		return nil, status.Error(codes.FailedPrecondition, "publisher is not configured")
	}

	err := s.pub.Publish(ctx, publisher.Message{
		Exchange:    req.GetExchange(),
		RoutingKey:  req.GetRoutingKey(),
		Payload:     req.GetPayload(),
		Headers:     req.GetHeaders(),
		ContentType: req.GetContentType(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "publish failed: %v", err)
	}

	return &authorizerv1.PublishBalanceOperationsResponse{Published: true, Message: "published"}, nil
}

// Run starts the authorizer gRPC server and blocks until shutdown.
func Run(ctx context.Context, cfg *Config, logger libLog.Logger) error {
	router := shard.NewRouter(cfg.ShardCount)

	eng := engine.New(router, wal.NewNoopWriter())

	balanceLoader, err := loader.NewPostgresLoader(ctx, cfg.PostgresDSN, router)
	if err != nil {
		return err
	}
	defer balanceLoader.Close()

	initial, err := balanceLoader.LoadBalances(ctx, "", "", nil)
	if err != nil {
		return err
	}

	loaded := eng.UpsertBalances(initial)
	logger.Infof("Authorizer loaded balances from PostgreSQL: %d", loaded)

	if cfg.WALEnabled {
		entries, err := wal.Replay(cfg.WALPath)
		if err != nil {
			return fmt.Errorf("replay wal: %w", err)
		}

		if err := eng.ReplayEntries(entries); err != nil {
			return fmt.Errorf("apply wal replay: %w", err)
		}

		logger.Infof("Authorizer replayed WAL entries: %d", len(entries))

		writer, err := wal.NewRingBufferWriter(cfg.WALPath, cfg.WALBufferSize, cfg.WALFlushInterval)
		if err != nil {
			return err
		}

		eng.SetWALWriter(writer)
		defer func() {
			_ = writer.Close()
		}()
	}

	pub := publisher.NewNoopPublisher()
	if cfg.RabbitMQEnabled {
		rabbitPublisher, err := publisher.NewRabbitMQPublisher(cfg.RabbitMQURL, logger)
		if err != nil {
			return fmt.Errorf("initialize rabbitmq publisher: %w", err)
		}

		pub = rabbitPublisher
		defer func() {
			_ = pub.Close()
		}()
	}

	server := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
		grpc.MaxConcurrentStreams(cfg.MaxConcurrentStreams),
		grpc.MaxRecvMsgSize(cfg.MaxReceiveMessageSizeBytes),
	)

	service := &authorizerService{
		engine:  eng,
		loader:  balanceLoader,
		pub:     pub,
		logger:  logger,
		started: time.Now(),
	}

	authorizerv1.RegisterBalanceAuthorizerServer(server, service)
	reflection.Register(server)

	healthServer := grpcHealth.NewServer()
	healthServer.SetServingStatus("authorizer.v1.BalanceAuthorizer", grpcHealthV1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("", grpcHealthV1.HealthCheckResponse_SERVING)
	grpcHealthV1.RegisterHealthServer(server, healthServer)

	listener, err := net.Listen("tcp", cfg.GRPCAddress)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.GRPCAddress, err)
	}

	logger.Infof(
		"Authorizer gRPC server starting on %s (shards=%s loaded=%d)",
		cfg.GRPCAddress,
		strconv.Itoa(eng.ShardCount()),
		eng.LoadedBalances(),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		logger.Infof("Authorizer shutting down")
		healthServer.SetServingStatus("", grpcHealthV1.HealthCheckResponse_NOT_SERVING)
		healthServer.SetServingStatus("authorizer.v1.BalanceAuthorizer", grpcHealthV1.HealthCheckResponse_NOT_SERVING)
		server.GracefulStop()
		return nil
	}
}
