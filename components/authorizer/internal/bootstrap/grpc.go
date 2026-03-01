// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpcHealth "google.golang.org/grpc/health"
	grpcHealthV1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	peerAuthTimestampHeader = "x-midaz-peer-ts"
	peerAuthNonceHeader     = "x-midaz-peer-nonce"
	peerAuthMethodHeader    = "x-midaz-peer-method"
	peerAuthBodyHashHeader  = "x-midaz-peer-body-sha256"
	peerAuthSignatureHeader = "x-midaz-peer-signature"
)

const defaultPeerAuthMaxSkew = 30 * time.Second

const (
	peerRPCMethodPrepareAuthorize = "/authorizer.v1.BalanceAuthorizer/PrepareAuthorize"
	peerRPCMethodCommitPrepared   = "/authorizer.v1.BalanceAuthorizer/CommitPrepared"
	peerRPCMethodAbortPrepared    = "/authorizer.v1.BalanceAuthorizer/AbortPrepared"
)

type peerNonceStore struct {
	mu      sync.Mutex
	window  time.Duration
	maxSize int
	seen    map[string]time.Time
}

func newPeerNonceStore(window time.Duration, maxSize int) *peerNonceStore {
	if window <= 0 {
		window = defaultPeerAuthMaxSkew
	}

	if maxSize <= 0 {
		maxSize = 100000
	}

	return &peerNonceStore{
		window:  window,
		maxSize: maxSize,
		seen:    make(map[string]time.Time),
	}
}

func (s *peerNonceStore) MarkIfNew(nonce string, now time.Time) bool {
	if s == nil || nonce == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for existingNonce, ts := range s.seen {
		if now.Sub(ts) > s.window {
			delete(s.seen, existingNonce)
		}
	}

	if _, exists := s.seen[nonce]; exists {
		return false
	}

	if len(s.seen) >= s.maxSize {
		oldestNonce := ""
		oldestTS := now

		for existingNonce, ts := range s.seen {
			if oldestNonce == "" || ts.Before(oldestTS) {
				oldestNonce = existingNonce
				oldestTS = ts
			}
		}

		if oldestNonce != "" {
			delete(s.seen, oldestNonce)
		}
	}

	s.seen[nonce] = now

	return true
}

// peerClient represents a gRPC connection to another authorizer instance in the
// cluster. Each peer owns a contiguous range of shard IDs. During cross-shard
// transactions, the coordinator calls PrepareAuthorize/CommitPrepared/AbortPrepared
// on the peer that owns the remote shards.
type peerClient struct {
	addr       string
	client     authorizerv1.BalanceAuthorizerClient
	conn       *grpc.ClientConn
	shardStart int
	shardEnd   int
}

type authorizerService struct {
	authorizerv1.UnimplementedBalanceAuthorizerServer

	engine            *engine.Engine
	loader            *loader.PostgresLoader
	pub               publisher.Publisher
	logger            libLog.Logger
	metrics           *authorizerMetrics
	started           time.Time
	grpcAddr          string
	instanceAddr      string
	ownedShardStart   int
	ownedShardEnd     int
	peers             []*peerClient
	peerAuthToken     string
	peerAuthTokenPrev string
	peerAuthMaxSkew   time.Duration
	abortRPCDeadline  time.Duration
	commitRPCDeadline time.Duration
	peerPrepareSem    chan struct{}
	peerNonceStore    *peerNonceStore
}

func (s *authorizerService) Authorize(ctx context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	return s.authorizeWithMetric(ctx, req, "authorize")
}

func (s *authorizerService) authorizeWithMetric(ctx context.Context, req *authorizerv1.AuthorizeRequest, metricOperation string) (*authorizerv1.AuthorizeResponse, error) {
	// Resolve shards using the engine's normalization logic (handles external accounts).
	shardOps := s.engine.ResolveOperationShards(req.GetOperations())

	if len(s.peers) == 0 {
		for shardID := range shardOps {
			if !s.isLocalShard(shardID) {
				s.logger.Errorf(
					"authorizer shard ownership misconfiguration: shard=%d outside local range=%d-%d with no peers configured",
					shardID,
					s.ownedShardStart,
					s.ownedShardEnd,
				)

				return nil, status.Error(codes.Internal, "authorizer shard ownership misconfiguration")
			}
		}

		return s.authorizeFastPath(ctx, req, metricOperation)
	}

	allLocal := true
	for shardID := range shardOps {
		if !s.isLocalShard(shardID) {
			allLocal = false
			break
		}
	}

	if allLocal {
		return s.authorizeFastPath(ctx, req, metricOperation)
	}

	// Cross-shard transaction detected — coordinate via 2PC protocol.
	return s.authorizeCrossShard(ctx, req, shardOps)
}

// authorizeFastPath is the single-instance fast path. It calls engine.Authorize
// directly without any 2PC overhead. This is the original Authorize handler logic.
func (s *authorizerService) authorizeFastPath(ctx context.Context, req *authorizerv1.AuthorizeRequest, metricOperation string) (*authorizerv1.AuthorizeResponse, error) {
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
			s.metrics.RecordAuthorize(ctx, metricOperation, "error", engine.RejectionInternalError, pending, transactionStatus, operationsCount, shardCount, latency)
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
		s.metrics.RecordAuthorize(ctx, metricOperation, result, rejectionCode, pending, transactionStatus, operationsCount, shardCount, latency)
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

		resp, err := s.authorizeWithMetric(stream.Context(), req, "authorize_stream")
		if err != nil {
			if req != nil {
				s.logger.Errorf(
					"Authorizer stream authorize failed: tx_id=%s org_id=%s ledger_id=%s ops=%d err=%v",
					req.GetTransactionId(),
					req.GetOrganizationId(),
					req.GetLedgerId(),
					len(req.GetOperations()),
					err,
				)
			}

			return err
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

func withPeerAuth(ctx context.Context, token, method string, req proto.Message) (context.Context, error) {
	if token == "" {
		return ctx, nil
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce, err := generatePeerNonce()
	if err != nil {
		return nil, fmt.Errorf("generate peer nonce: %w", err)
	}
	bodyHash := hashPeerAuthBody(req)
	signature := signPeerAuth(token, timestamp, nonce, method, bodyHash)

	return metadata.AppendToOutgoingContext(
		ctx,
		peerAuthTimestampHeader, timestamp,
		peerAuthNonceHeader, nonce,
		peerAuthMethodHeader, method,
		peerAuthBodyHashHeader, bodyHash,
		peerAuthSignatureHeader, signature,
	), nil
}

func signPeerAuth(token, timestamp, nonce, method, bodyHash string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(nonce))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(method))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(bodyHash))

	return hex.EncodeToString(mac.Sum(nil))
}

func hashPeerAuthBody(req proto.Message) string {
	if req == nil {
		digest := sha256.Sum256(nil)
		return hex.EncodeToString(digest[:])
	}

	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(req)
	if err != nil {
		digest := sha256.Sum256(nil)
		return hex.EncodeToString(digest[:])
	}

	digest := sha256.Sum256(payload)

	return hex.EncodeToString(digest[:])
}

func generatePeerNonce() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return hex.EncodeToString(raw), nil
}

func (s *authorizerService) authorizePeerRPC(ctx context.Context, expectedMethod string, req proto.Message) error {
	if s.peerAuthToken == "" {
		return status.Error(codes.FailedPrecondition, "peer authentication is not configured")
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.PermissionDenied, "missing peer credentials")
	}

	receivedTimestamp := md.Get(peerAuthTimestampHeader)
	receivedNonce := md.Get(peerAuthNonceHeader)
	receivedMethod := md.Get(peerAuthMethodHeader)
	receivedBodyHash := md.Get(peerAuthBodyHashHeader)
	receivedSignature := md.Get(peerAuthSignatureHeader)
	if len(receivedTimestamp) == 0 || len(receivedNonce) == 0 || len(receivedMethod) == 0 || len(receivedBodyHash) == 0 || len(receivedSignature) == 0 {
		return status.Error(codes.PermissionDenied, "missing peer credentials")
	}

	rawTimestamp := strings.TrimSpace(receivedTimestamp[0])
	rawNonce := strings.TrimSpace(receivedNonce[0])
	rawMethod := strings.TrimSpace(receivedMethod[0])
	rawBodyHash := strings.TrimSpace(receivedBodyHash[0])
	providedSignature := strings.TrimSpace(receivedSignature[0])

	if rawNonce == "" || rawMethod == "" || rawBodyHash == "" || providedSignature == "" {
		return status.Error(codes.PermissionDenied, "invalid peer credentials")
	}

	if expectedMethod != "" && rawMethod != expectedMethod {
		return status.Error(codes.PermissionDenied, "invalid peer credentials")
	}

	unixSeconds, err := strconv.ParseInt(rawTimestamp, 10, 64)
	if err != nil {
		return status.Error(codes.PermissionDenied, "invalid peer credentials")
	}

	issuedAt := time.Unix(unixSeconds, 0)
	now := time.Now()
	maxSkew := s.peerAuthMaxSkew
	if maxSkew <= 0 {
		maxSkew = defaultPeerAuthMaxSkew
	}

	if issuedAt.After(now.Add(maxSkew)) || now.Sub(issuedAt) > maxSkew {
		return status.Error(codes.PermissionDenied, "expired peer credentials")
	}

	expectedBodyHash := hashPeerAuthBody(req)
	if subtle.ConstantTimeCompare([]byte(rawBodyHash), []byte(expectedBodyHash)) != 1 {
		return status.Error(codes.PermissionDenied, "invalid peer credentials")
	}

	expectedSignatures := []string{signPeerAuth(s.peerAuthToken, rawTimestamp, rawNonce, rawMethod, rawBodyHash)}
	if s.peerAuthTokenPrev != "" {
		expectedSignatures = append(expectedSignatures, signPeerAuth(s.peerAuthTokenPrev, rawTimestamp, rawNonce, rawMethod, rawBodyHash))
	}

	validSignature := false
	for _, expectedSignature := range expectedSignatures {
		if subtle.ConstantTimeCompare([]byte(providedSignature), []byte(expectedSignature)) == 1 {
			validSignature = true
			break
		}
	}

	if !validSignature {
		return status.Error(codes.PermissionDenied, "invalid peer credentials")
	}

	if s.peerNonceStore == nil {
		s.peerNonceStore = newPeerNonceStore(maxSkew, 100000)
	}

	if !s.peerNonceStore.MarkIfNew(rawNonce, now) {
		return status.Error(codes.PermissionDenied, "replayed peer credentials")
	}

	return nil
}

func (s *authorizerService) PrepareAuthorize(ctx context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.PrepareAuthorizeResponse, error) {
	if err := s.authorizePeerRPC(ctx, peerRPCMethodPrepareAuthorize, req); err != nil {
		return nil, err
	}

	if s.peerPrepareSem != nil {
		select {
		case s.peerPrepareSem <- struct{}{}:
			defer func() {
				<-s.peerPrepareSem
			}()
		default:
			return nil, status.Error(codes.ResourceExhausted, "too many concurrent prepare requests")
		}
	}

	start := time.Now()

	ptx, resp, err := s.engine.PrepareAuthorize(req)

	latency := time.Since(start)

	if err != nil {
		if s.metrics.Enabled() {
			s.metrics.RecordAuthorize(ctx, "prepare_authorize", "error", engine.RejectionInternalError, false, "", 0, 0, latency)
		}

		return nil, status.Error(codes.Internal, "prepare authorize failed")
	}

	if resp == nil {
		return nil, status.Error(codes.Internal, "prepare authorize failed")
	}

	if !resp.GetAuthorized() {
		return &authorizerv1.PrepareAuthorizeResponse{
			Authorized:       false,
			RejectionCode:    resp.GetRejectionCode(),
			RejectionMessage: resp.GetRejectionMessage(),
		}, nil
	}

	if s.metrics.Enabled() {
		s.metrics.RecordAuthorize(ctx, "prepare_authorize", "prepared", "", req.GetPending(), req.GetTransactionStatus(), len(req.GetOperations()), 0, latency)
	}

	preparedTxID := ""
	if ptx != nil {
		preparedTxID = ptx.ID
	}

	return &authorizerv1.PrepareAuthorizeResponse{
		Authorized:   true,
		PreparedTxId: preparedTxID,
		Balances:     resp.GetBalances(),
	}, nil
}

func (s *authorizerService) CommitPrepared(ctx context.Context, req *authorizerv1.CommitPreparedRequest) (*authorizerv1.CommitPreparedResponse, error) {
	if err := s.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req); err != nil {
		return nil, err
	}

	if req == nil || strings.TrimSpace(req.GetPreparedTxId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "prepared_tx_id is required")
	}

	start := time.Now()

	resp, err := s.engine.CommitPrepared(req.GetPreparedTxId())

	latency := time.Since(start)

	if err != nil {
		s.logger.Warnf("CommitPrepared failed: prepared_tx_id=%s err=%v latency=%v", shortenPreparedTxID(req.GetPreparedTxId()), err, latency)
		if errors.Is(err, engine.ErrPreparedTxNotFound) {
			return nil, status.Error(codes.NotFound, "prepared transaction not found")
		}

		return nil, status.Error(codes.Internal, "commit prepared failed")
	}

	if s.metrics.Enabled() {
		s.metrics.RecordAuthorize(ctx, "commit_prepared", "committed", "", false, "", 0, 0, latency)
	}

	return &authorizerv1.CommitPreparedResponse{
		Committed: true,
		Balances:  resp.GetBalances(),
	}, nil
}

func (s *authorizerService) AbortPrepared(ctx context.Context, req *authorizerv1.AbortPreparedRequest) (*authorizerv1.AbortPreparedResponse, error) {
	if err := s.authorizePeerRPC(ctx, peerRPCMethodAbortPrepared, req); err != nil {
		return nil, err
	}

	if req == nil || strings.TrimSpace(req.GetPreparedTxId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "prepared_tx_id is required")
	}

	err := s.engine.AbortPrepared(req.GetPreparedTxId())
	if err != nil {
		s.logger.Warnf("AbortPrepared failed: prepared_tx_id=%s err=%v", shortenPreparedTxID(req.GetPreparedTxId()), err)
		if errors.Is(err, engine.ErrPreparedTxNotFound) {
			return nil, status.Error(codes.NotFound, "prepared transaction not found")
		}
		if errors.Is(err, engine.ErrPreparedTxAlreadyCommitted) || errors.Is(err, engine.ErrPreparedTxCommitDecided) {
			return nil, status.Error(codes.FailedPrecondition, "prepared transaction commit already decided")
		}

		return nil, status.Error(codes.Internal, "abort prepared failed")
	}

	return &authorizerv1.AbortPreparedResponse{Aborted: true}, nil
}

func shortenPreparedTxID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 12 {
		return id
	}

	return id[:12] + "..."
}

type peerShardRange struct {
	start int
	end   int
}

func parsePeerShardRange(raw string, shardCount int) (peerShardRange, error) {
	parts := strings.Split(strings.TrimSpace(raw), "-")
	if len(parts) != 2 {
		return peerShardRange{}, fmt.Errorf("invalid peer shard range %q (expected start-end)", raw)
	}

	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return peerShardRange{}, fmt.Errorf("invalid peer shard range start %q: %w", parts[0], err)
	}

	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return peerShardRange{}, fmt.Errorf("invalid peer shard range end %q: %w", parts[1], err)
	}

	if start < 0 || end < start || end >= shardCount {
		return peerShardRange{}, fmt.Errorf(
			"peer shard range %q is out of bounds for shard count %d",
			raw,
			shardCount,
		)
	}

	return peerShardRange{start: start, end: end}, nil
}

func validateShardCoverage(shardCount, localStart, localEnd int, peers []peerShardRange) error {
	if shardCount <= 0 {
		return fmt.Errorf("AUTHORIZER_SHARD_COUNT must be > 0")
	}

	covered := make([]bool, shardCount)

	mark := func(start, end int, owner string) error {
		for shardID := start; shardID <= end; shardID++ {
			if covered[shardID] {
				return fmt.Errorf("shard %d is assigned to multiple owners (latest=%s)", shardID, owner)
			}

			covered[shardID] = true
		}

		return nil
	}

	if err := mark(localStart, localEnd, "local"); err != nil {
		return err
	}

	for _, peer := range peers {
		if err := mark(peer.start, peer.end, "peer"); err != nil {
			return err
		}
	}

	for shardID, ok := range covered {
		if !ok {
			return fmt.Errorf("shard %d has no owner configured; local and peer shard ranges must cover 0-%d", shardID, shardCount-1)
		}
	}

	return nil
}

// Run starts the authorizer gRPC server and blocks until shutdown.
func Run(ctx context.Context, cfg *Config, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) error {
	metricRecorder := newAuthorizerMetrics(telemetry, logger, cfg.AuthorizeLatencySLO)
	router := shard.NewRouter(cfg.ShardCount)

	eng := engine.New(router, wal.NewNoopWriter())
	defer eng.Close()
	eng.SetObserver(metricRecorder)
	eng.ConfigurePreparedTxStore(cfg.PrepareTimeout, cfg.PrepareMaxPending)
	eng.ConfigurePreparedTxRetention(cfg.PrepareCommittedRetention, cfg.PrepareCommitRetryLimit)

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

	initial, err := balanceLoader.LoadBalances(ctx, "", "", cfg.ShardIDs)
	if err != nil {
		return err
	}

	loaded := eng.UpsertBalances(initial)
	logger.Infof("Authorizer loaded balances from PostgreSQL: %d", loaded)
	logger.Infof(
		"Authorizer runtime config: grpc_address=%s shards=%d shard_ids=%v wal_buffer_size=%d wal_flush_interval_ms=%d wal_sync_on_append=%t prepare_timeout_ms=%d prepare_max_pending=%d authorize_latency_slo_ms=%d max_streams=%d max_recv_bytes=%d postgres_pool_max_conns=%d postgres_pool_min_conns=%d postgres_conn_lifetime_ms=%d postgres_conn_idle_ms=%d postgres_healthcheck_ms=%d redpanda_enabled=%t redpanda_backpressure_policy=%s redpanda_retries=%d redpanda_delivery_timeout_ms=%d redpanda_publish_timeout_ms=%d telemetry_enabled=%t",
		cfg.GRPCAddress,
		cfg.ShardCount,
		cfg.ShardIDs,
		cfg.WALBufferSize,
		cfg.WALFlushInterval.Milliseconds(),
		cfg.WALSyncOnAppend,
		cfg.PrepareTimeout.Milliseconds(),
		cfg.PrepareMaxPending,
		cfg.AuthorizeLatencySLO.Milliseconds(),
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
		if closeErr := writer.Close(); closeErr != nil {
			logger.Warnf("Failed to close WAL writer: %v", closeErr)
		}
	}()

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

	if len(cfg.PeerInstances) > 0 && !cfg.RedpandaEnabled {
		return fmt.Errorf("cross-shard mode requires AUTHORIZER_REDPANDA_ENABLED=true for durable commit intent")
	}

	if cfg.PeerInsecureAllowed && !brokersecurity.IsNonProductionEnvironment(cfg.EnvName) {
		return fmt.Errorf("AUTHORIZER_PEER_INSECURE_ALLOWED=true is only allowed in non-production environments")
	}

	allowInsecurePeerTransport := cfg.PeerInsecureAllowed && brokersecurity.IsNonProductionEnvironment(cfg.EnvName)
	if len(cfg.PeerInstances) > 0 && !cfg.GRPCTLSEnabled && !allowInsecurePeerTransport {
		return fmt.Errorf(
			"peer RPC requires TLS by default; set AUTHORIZER_GRPC_TLS_ENABLED=true (recommended) or AUTHORIZER_PEER_INSECURE_ALLOWED=true for local-only usage",
		)
	}

	if len(cfg.PeerInstances) > 0 && allowInsecurePeerTransport {
		logger.Warnf("Authorizer peer RPC is using insecure transport for non-production environment %q", cfg.EnvName)
	}

	peerRanges := make([]peerShardRange, 0, len(cfg.PeerInstances))
	if len(cfg.PeerInstances) > 0 {
		if len(cfg.PeerShardRanges) > 0 {
			if len(cfg.PeerShardRanges) != len(cfg.PeerInstances) {
				return fmt.Errorf(
					"AUTHORIZER_PEER_SHARD_RANGES count (%d) must match AUTHORIZER_PEER_INSTANCES count (%d)",
					len(cfg.PeerShardRanges),
					len(cfg.PeerInstances),
				)
			}

			for _, rawRange := range cfg.PeerShardRanges {
				parsed, parseErr := parsePeerShardRange(rawRange, cfg.ShardCount)
				if parseErr != nil {
					return parseErr
				}

				if parsed.end >= cfg.OwnedShardStart && parsed.start <= cfg.OwnedShardEnd {
					return fmt.Errorf(
						"peer shard range %d-%d overlaps local owned range %d-%d",
						parsed.start,
						parsed.end,
						cfg.OwnedShardStart,
						cfg.OwnedShardEnd,
					)
				}

				peerRanges = append(peerRanges, parsed)
			}
		} else {
			if len(cfg.PeerInstances) > 1 {
				return fmt.Errorf(
					"AUTHORIZER_PEER_SHARD_RANGES is required when configuring multiple peers",
				)
			}

			if cfg.OwnedShardStart > 0 && cfg.OwnedShardEnd < cfg.ShardCount-1 {
				return fmt.Errorf(
					"cannot infer peer shard range from local ownership %d-%d; set AUTHORIZER_PEER_SHARD_RANGES explicitly",
					cfg.OwnedShardStart,
					cfg.OwnedShardEnd,
				)
			}

			if cfg.OwnedShardStart > 0 {
				peerRanges = append(peerRanges, peerShardRange{start: 0, end: cfg.OwnedShardStart - 1})
			} else {
				if cfg.OwnedShardEnd >= cfg.ShardCount-1 {
					return fmt.Errorf("peer instances configured but local instance owns all shards")
				}

				peerRanges = append(peerRanges, peerShardRange{start: cfg.OwnedShardEnd + 1, end: cfg.ShardCount - 1})
			}
		}

		for i := 0; i < len(peerRanges); i++ {
			for j := i + 1; j < len(peerRanges); j++ {
				if peerRanges[i].end >= peerRanges[j].start && peerRanges[j].end >= peerRanges[i].start {
					return fmt.Errorf(
						"peer shard ranges overlap: %d-%d and %d-%d",
						peerRanges[i].start,
						peerRanges[i].end,
						peerRanges[j].start,
						peerRanges[j].end,
					)
				}
			}
		}
	}

	if len(cfg.PeerInstances) > 0 {
		if err := validateShardCoverage(cfg.ShardCount, cfg.OwnedShardStart, cfg.OwnedShardEnd, peerRanges); err != nil {
			return err
		}
	} else if cfg.OwnedShardStart != 0 || cfg.OwnedShardEnd != cfg.ShardCount-1 {
		return fmt.Errorf(
			"without peer instances configured, local shard ownership must cover all shards (expected 0-%d, got %d-%d)",
			cfg.ShardCount-1,
			cfg.OwnedShardStart,
			cfg.OwnedShardEnd,
		)
	}

	// Connect to peer authorizer instances for cross-shard 2PC coordination.
	var peers []*peerClient

	for i, peerAddr := range cfg.PeerInstances {
		var transportOption grpc.DialOption
		if cfg.GRPCTLSEnabled {
			tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
			if cfg.PeerTLSCAFile != "" {
				caBundle, readErr := os.ReadFile(cfg.PeerTLSCAFile)
				if readErr != nil {
					return fmt.Errorf("read AUTHORIZER_PEER_TLS_CA_FILE: %w", readErr)
				}

				rootCAs := x509.NewCertPool()
				if !rootCAs.AppendCertsFromPEM(caBundle) {
					return fmt.Errorf("parse AUTHORIZER_PEER_TLS_CA_FILE: no certificates found")
				}

				tlsCfg.RootCAs = rootCAs
			}

			transportOption = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
		} else {
			transportOption = grpc.WithTransportCredentials(insecure.NewCredentials())
		}

		peerConn, peerErr := grpc.NewClient(peerAddr,
			transportOption,
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                30 * time.Second,
				Timeout:             10 * time.Second,
				PermitWithoutStream: true,
			}),
		)
		if peerErr != nil {
			// Close any previously opened connections before returning.
			for _, p := range peers {
				if p.conn != nil {
					_ = p.conn.Close()
				}
			}

			return fmt.Errorf("connect to authorizer peer %s: %w", peerAddr, peerErr)
		}

		peerRange := peerRanges[i]

		peers = append(peers, &peerClient{
			addr:       peerAddr,
			client:     authorizerv1.NewBalanceAuthorizerClient(peerConn),
			conn:       peerConn,
			shardStart: peerRange.start,
			shardEnd:   peerRange.end,
		})

		logger.Infof("Connected to authorizer peer at %s (owns shards %d-%d)", peerAddr, peerRange.start, peerRange.end)
	}

	defer func() {
		for _, p := range peers {
			if p.conn != nil {
				_ = p.conn.Close()
			}
		}
	}()

	tlMid := libHTTP.NewTelemetryMiddleware(telemetry)

	serverOptions := []grpc.ServerOption{
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
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxConcurrentStreams(cfg.MaxConcurrentStreams),
		grpc.MaxRecvMsgSize(cfg.MaxReceiveMessageSizeBytes),
	}

	if cfg.GRPCTLSEnabled {
		serverCreds, credsErr := credentials.NewServerTLSFromFile(cfg.GRPCTLSCertFile, cfg.GRPCTLSKeyFile)
		if credsErr != nil {
			return fmt.Errorf("load gRPC TLS credentials: %w", credsErr)
		}

		serverOptions = append(serverOptions, grpc.Creds(serverCreds))
	}

	server := grpc.NewServer(serverOptions...)

	var peerPrepareSem chan struct{}
	if cfg.PeerPrepareMaxInFlight > 0 {
		peerPrepareSem = make(chan struct{}, cfg.PeerPrepareMaxInFlight)
	}

	service := &authorizerService{
		engine:            eng,
		loader:            balanceLoader,
		pub:               pub,
		logger:            logger,
		metrics:           metricRecorder,
		started:           time.Now(),
		grpcAddr:          cfg.GRPCAddress,
		instanceAddr:      cfg.InstanceAddress,
		ownedShardStart:   cfg.OwnedShardStart,
		ownedShardEnd:     cfg.OwnedShardEnd,
		peers:             peers,
		peerAuthToken:     cfg.PeerAuthToken,
		peerAuthTokenPrev: cfg.PeerAuthTokenPrevious,
		peerAuthMaxSkew:   cfg.PeerAuthMaxSkew,
		abortRPCDeadline:  cfg.PeerAbortTimeout,
		commitRPCDeadline: cfg.PeerCommitTimeout,
		peerPrepareSem:    peerPrepareSem,
		peerNonceStore:    newPeerNonceStore(cfg.PeerAuthMaxSkew, cfg.PeerNonceMaxEntries),
	}

	if len(cfg.PeerInstances) > 0 {
		recoveryRunner, recoveryErr := newCommitIntentRecoveryRunner(cfg, service, logger)
		if recoveryErr != nil {
			return recoveryErr
		}

		recoveryCtx, stopRecovery := context.WithCancel(ctx)
		defer stopRecovery()
		defer recoveryRunner.Close()

		go recoveryRunner.Run(recoveryCtx)
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
