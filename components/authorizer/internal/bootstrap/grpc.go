// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/loader"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	brokerpkg "github.com/LerianStudio/midaz/v3/pkg/broker"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
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
	metrics *authorizerMetrics
	started time.Time
}

func (s *authorizerService) Authorize(ctx context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	start := time.Now()
	resp, err := s.engine.Authorize(req)
	latency := time.Since(start)
	operationsCount := 0
	pending := false
	transactionStatus := ""
	transactionID := ""
	organizationID := ""
	ledgerID := ""
	if req != nil {
		operationsCount = len(req.GetOperations())
		pending = req.GetPending()
		transactionStatus = req.GetTransactionStatus()
		transactionID = req.GetTransactionId()
		organizationID = req.GetOrganizationId()
		ledgerID = req.GetLedgerId()
	}

	var operations []*authorizerv1.BalanceOperation
	if req != nil {
		operations = req.GetOperations()
	}

	shardCount := 0
	metricsEnabled := s.metrics.Enabled()
	if metricsEnabled {
		shardCount = s.engine.CountShardsForOperations(operations)
	}
	if err != nil {
		if metricsEnabled {
			s.metrics.RecordAuthorize(ctx, "authorize", "error", engine.RejectionInternalError, pending, transactionStatus, operationsCount, shardCount, latency)
		}
		s.logger.Errorf("Authorizer authorize failed: tx_id=%s org_id=%s ledger_id=%s ops=%d err=%v", transactionID, organizationID, ledgerID, operationsCount, err)

		return nil, status.Error(codes.Internal, "authorize failed")
	}

	result := "authorized"
	rejectionCode := ""
	if !resp.GetAuthorized() {
		result = "rejected"
		rejectionCode = resp.GetRejectionCode()
	}

	if metricsEnabled {
		s.metrics.RecordAuthorize(ctx, "authorize", result, rejectionCode, pending, transactionStatus, operationsCount, shardCount, latency)
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

		start := time.Now()
		resp, err := s.engine.Authorize(req)
		latency := time.Since(start)
		operationsCount := 0
		pending := false
		transactionStatus := ""
		transactionID := ""
		organizationID := ""
		ledgerID := ""
		var operations []*authorizerv1.BalanceOperation
		if req != nil {
			operationsCount = len(req.GetOperations())
			pending = req.GetPending()
			transactionStatus = req.GetTransactionStatus()
			transactionID = req.GetTransactionId()
			organizationID = req.GetOrganizationId()
			ledgerID = req.GetLedgerId()
			operations = req.GetOperations()
		}

		metricsEnabled := s.metrics.Enabled()
		shardCount := 0
		if metricsEnabled {
			shardCount = s.engine.CountShardsForOperations(operations)
		}
		if err != nil {
			if metricsEnabled {
				s.metrics.RecordAuthorize(stream.Context(), "authorize_stream", "error", engine.RejectionInternalError, pending, transactionStatus, operationsCount, shardCount, latency)
			}
			s.logger.Errorf("Authorizer stream authorize failed: tx_id=%s org_id=%s ledger_id=%s ops=%d err=%v", transactionID, organizationID, ledgerID, operationsCount, err)

			return status.Error(codes.Internal, "stream authorize failed")
		}

		result := "authorized"
		rejectionCode := ""
		if !resp.GetAuthorized() {
			result = "rejected"
			rejectionCode = resp.GetRejectionCode()
		}

		if metricsEnabled {
			s.metrics.RecordAuthorize(stream.Context(), "authorize_stream", result, rejectionCode, pending, transactionStatus, operationsCount, shardCount, latency)
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func (s *authorizerService) LoadBalances(ctx context.Context, req *authorizerv1.LoadBalancesRequest) (*authorizerv1.LoadBalancesResponse, error) {
	balances, err := s.loader.LoadBalances(ctx, req.GetOrganizationId(), req.GetLedgerId(), req.GetShardIds())
	if err != nil {
		return nil, status.Error(codes.Internal, "load balances failed")
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

	start := time.Now()
	err := s.pub.Publish(ctx, publisher.Message{
		Topic:        req.GetTopic(),
		PartitionKey: req.GetPartitionKey(),
		Payload:      req.GetPayload(),
		Headers:      req.GetHeaders(),
		ContentType:  req.GetContentType(),
	})
	publishLatency := time.Since(start)
	if s.metrics.Enabled() {
		s.metrics.RecordPublish(ctx, req.GetTopic(), err, publishLatency)
	}
	if err != nil {
		s.logger.Warnf("Authorizer publish failed: topic=%s partition_key=%s err=%v", req.GetTopic(), req.GetPartitionKey(), err)

		return nil, status.Error(codes.Unavailable, "publish failed")
	}

	return &authorizerv1.PublishBalanceOperationsResponse{Published: true, Message: "published"}, nil
}

// Run starts the authorizer gRPC server and blocks until shutdown.
func Run(ctx context.Context, cfg *Config, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) error {
	metricRecorder := newAuthorizerMetrics(telemetry, logger)
	router := shard.NewRouter(cfg.ShardCount)

	eng := engine.New(router, wal.NewNoopWriter())
	eng.SetObserver(metricRecorder)

	balanceLoader, err := loader.NewPostgresLoaderWithConfig(ctx, cfg.PostgresDSN, router, loader.PoolConfig{
		MaxConns:          cfg.PostgresPoolMaxConns,
		MinConns:          cfg.PostgresPoolMinConns,
		MaxConnLifetime:   cfg.PostgresPoolMaxConnLife,
		MaxConnIdleTime:   cfg.PostgresPoolMaxConnIdle,
		HealthCheckPeriod: cfg.PostgresPoolHealthCheck,
		ConnectTimeout:    cfg.PostgresConnectTimeout,
	})
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
	logger.Infof(
		"Authorizer runtime config: grpc_address=%s shards=%d wal_enabled=%t wal_buffer_size=%d wal_flush_interval_ms=%d wal_sync_on_append=%t max_streams=%d max_recv_bytes=%d postgres_pool_max_conns=%d postgres_pool_min_conns=%d postgres_conn_lifetime_ms=%d postgres_conn_idle_ms=%d postgres_healthcheck_ms=%d redpanda_enabled=%t redpanda_backpressure_policy=%s redpanda_retries=%d redpanda_delivery_timeout_ms=%d redpanda_publish_timeout_ms=%d telemetry_enabled=%t",
		cfg.GRPCAddress,
		cfg.ShardCount,
		cfg.WALEnabled,
		cfg.WALBufferSize,
		cfg.WALFlushInterval.Milliseconds(),
		cfg.WALSyncOnAppend,
		cfg.MaxConcurrentStreams,
		cfg.MaxReceiveMessageSizeBytes,
		cfg.PostgresPoolMaxConns,
		cfg.PostgresPoolMinConns,
		cfg.PostgresPoolMaxConnLife.Milliseconds(),
		cfg.PostgresPoolMaxConnIdle.Milliseconds(),
		cfg.PostgresPoolHealthCheck.Milliseconds(),
		cfg.RedpandaEnabled,
		cfg.RedpandaBackpressurePolicy,
		cfg.RedpandaRecordRetries,
		cfg.RedpandaDeliveryTimeout.Milliseconds(),
		cfg.RedpandaPublishTimeout.Milliseconds(),
		cfg.EnableTelemetry,
	)

	if cfg.WALEnabled {
		entries, err := wal.Replay(cfg.WALPath)
		if err != nil {
			return fmt.Errorf("replay wal: %w", err)
		}

		if err := eng.ReplayEntries(entries); err != nil {
			return fmt.Errorf("apply wal replay: %w", err)
		}

		logger.Infof("Authorizer replayed WAL entries: %d", len(entries))

		writer, err := wal.NewRingBufferWriterWithOptions(
			cfg.WALPath,
			cfg.WALBufferSize,
			cfg.WALFlushInterval,
			cfg.WALSyncOnAppend,
			metricRecorder,
		)
		if err != nil {
			return err
		}

		eng.SetWALWriter(writer)
		defer func() {
			_ = writer.Close()
		}()
	}

	deprecatedBrokerEnvs := brokerpkg.DeprecatedBrokerEnvVariables(os.Environ())
	if len(deprecatedBrokerEnvs) > 0 {
		logger.Warnf(
			"Deprecated broker environment variables detected (ignored by this version): %s. Regenerate .env from .env.example and remove deprecated entries.",
			strings.Join(deprecatedBrokerEnvs, ", "),
		)
	}

	pub := publisher.NewNoopPublisher()
	if cfg.RedpandaEnabled {
		warnings, err := brokersecurity.ValidateRuntimeConfig(brokersecurity.RuntimeConfig{
			Environment:           cfg.EnvName,
			TLSEnabled:            cfg.RedpandaTLSEnabled,
			TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkip,
			SASLEnabled:           cfg.RedpandaSASLEnabled,
		})
		if err != nil {
			return err
		}

		for _, warning := range warnings {
			logger.Warnf("Authorizer Redpanda security warning: %s (ENV_NAME=%s)", warning, cfg.EnvName)
		}

		redpandaPublisher, err := publisher.NewRedpandaPublisherWithSecurity(
			cfg.RedpandaBrokers,
			logger,
			publisher.Config{
				ProducerLinger:        cfg.RedpandaProducerLinger,
				MaxBufferedRecords:    cfg.RedpandaMaxBufferedRecords,
				RecordRetries:         cfg.RedpandaRecordRetries,
				RecordDeliveryTimeout: cfg.RedpandaDeliveryTimeout,
				PublishTimeout:        cfg.RedpandaPublishTimeout,
				BackpressurePolicy:    cfg.RedpandaBackpressurePolicy,
			},
			publisher.SecurityConfig{
				TLSEnabled:            cfg.RedpandaTLSEnabled,
				TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkip,
				TLSCAFile:             cfg.RedpandaTLSCAFile,
				SASLEnabled:           cfg.RedpandaSASLEnabled,
				SASLMechanism:         cfg.RedpandaSASLMechanism,
				SASLUsername:          cfg.RedpandaSASLUsername,
				SASLPassword:          cfg.RedpandaSASLPassword,
			},
		)
		if err != nil {
			return fmt.Errorf("initialize redpanda publisher: %w", err)
		}

		pub = redpandaPublisher
		defer func() {
			_ = pub.Close()
		}()
	}

	tlMid := libHTTP.NewTelemetryMiddleware(telemetry)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			tlMid.WithTelemetryInterceptor(telemetry),
			libHTTP.WithGrpcLogging(libHTTP.WithCustomLogger(logger)),
		),
		grpc.ChainStreamInterceptor(
			streamTelemetryInterceptor(telemetry),
			streamLoggingInterceptor(logger),
		),
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
		metrics: metricRecorder,
		started: time.Now(),
	}

	authorizerv1.RegisterBalanceAuthorizerServer(server, service)
	if cfg.ReflectionEnabled {
		reflection.Register(server)
		logger.Infof("Authorizer gRPC reflection enabled")
	}

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
