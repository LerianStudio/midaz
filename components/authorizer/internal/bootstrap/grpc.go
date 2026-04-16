// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/loader"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/publisher"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	brokerpkg "github.com/LerianStudio/midaz/v3/pkg/broker"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	mgrpc "github.com/LerianStudio/midaz/v3/pkg/mgrpc"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

const defaultPeerAuthMaxSkew = 30 * time.Second

// gRPC keepalive and window size constants.
const (
	keepaliveTimeSec        = 30
	keepaliveTimeoutSec     = 10
	enforcementMinTimeSec   = 10
	nonceStoreMapDivisor    = 2
	shortenedIDMaxLen       = 12
	grpcInitialWindowBytes  = 8 * 1024 * 1024
	grpcInitialConnWinBytes = 16 * 1024 * 1024
)

const (
	peerRPCMethodAuthorize        = "/authorizer.v1.BalanceAuthorizer/Authorize"
	peerRPCMethodAuthorizeStream  = "/authorizer.v1.BalanceAuthorizer/AuthorizeStream"
	peerRPCMethodPrepareAuthorize = "/authorizer.v1.BalanceAuthorizer/PrepareAuthorize"
	peerRPCMethodCommitPrepared   = "/authorizer.v1.BalanceAuthorizer/CommitPrepared"
	peerRPCMethodAbortPrepared    = "/authorizer.v1.BalanceAuthorizer/AbortPrepared"
	peerRPCMethodLoadBalances     = "/authorizer.v1.BalanceAuthorizer/LoadBalances"
	peerRPCMethodGetBalance       = "/authorizer.v1.BalanceAuthorizer/GetBalance"
	peerRPCMethodPublishBalanceOp = "/authorizer.v1.BalanceAuthorizer/PublishBalanceOperations"
)

// Unauthorized RPC rejection reasons reported via authorizer_unauthorized_rpc_total.
// Values are stable metric labels — changing them is a breaking dashboard/alert change.
const (
	// unauthorizedReasonMissingToken: inbound request arrived while the server has
	// no peer-auth token configured (bootstrap misconfiguration — should be
	// impossible after B4 fix but kept as defense-in-depth).
	unauthorizedReasonMissingToken = "missing_token"
	// unauthorizedReasonMissingHeaders: one or more peer-auth headers absent.
	unauthorizedReasonMissingHeaders = "missing_headers"
	// unauthorizedReasonBadTimestamp: timestamp header is not a valid unix seconds integer.
	unauthorizedReasonBadTimestamp = "bad_timestamp"
	// unauthorizedReasonTimestampSkew: timestamp is outside the accepted clock-skew window.
	unauthorizedReasonTimestampSkew = "timestamp_skew"
	// unauthorizedReasonWrongAlgo: request signed for a different RPC method than invoked
	// (method-binding violation; treated as algorithm/protocol mismatch).
	unauthorizedReasonWrongAlgo = "wrong_algo"
	// unauthorizedReasonBodyMismatch: body hash does not match computed hash of request body.
	unauthorizedReasonBodyMismatch = "body_mismatch"
	// unauthorizedReasonInvalidHMAC: HMAC signature does not match either the current or
	// previous token signature.
	unauthorizedReasonInvalidHMAC = "invalid_hmac"
	// unauthorizedReasonNonceReplay: nonce has been seen before within the replay window.
	unauthorizedReasonNonceReplay = "nonce_replay"
	// unauthorizedReasonNonceInternal: peer nonce store not initialized (bootstrap bug).
	unauthorizedReasonNonceInternal = "nonce_internal"
	// unauthorizedReasonHashInternal: internal body-hash computation error.
	unauthorizedReasonHashInternal = "hash_internal"
)

type peerNonceStore struct {
	mu        sync.Mutex
	window    time.Duration
	maxSize   int
	current   map[string]struct{}
	previous  map[string]struct{}
	rotatedAt time.Time
}

func newPeerNonceStore(window time.Duration, maxSize int) *peerNonceStore {
	if window <= 0 {
		window = defaultPeerAuthMaxSkew
	}

	if maxSize <= 0 {
		maxSize = 100000
	}

	return &peerNonceStore{
		window:    window,
		maxSize:   maxSize,
		current:   make(map[string]struct{}, maxSize/nonceStoreMapDivisor),
		previous:  make(map[string]struct{}, maxSize/nonceStoreMapDivisor),
		rotatedAt: time.Now(),
	}
}

// MarkIfNew atomically checks whether a nonce has been seen before and, if new,
// records it in the current generation map. Returns true when the nonce is fresh
// (first occurrence), false on duplicate or capacity exhaustion.
//
// The method has two rotation check-points, each serving a distinct purpose:
//
//  1. Pre-insert capacity guard (emergency valve): when total stored nonces reach
//     maxSize, rotation is attempted so the insert can proceed. If the window has
//     not elapsed yet, the nonce is rejected to prevent unbounded memory growth
//     under burst traffic. This is an emergency back-pressure mechanism.
//
//  2. Post-insert periodic rotation: after the nonce is successfully stored, this
//     check rotates when the current generation reaches half-capacity AND the time
//     window has elapsed. This is the steady-state rotation path that keeps memory
//     bounded during normal operation, ensuring old nonces are evicted on schedule.
//
// Both check-points are necessary: the pre-insert guard prevents OOM under
// adversarial load, while the post-insert rotation maintains healthy eviction
// cadence during normal traffic. Consolidating them would either lose the
// emergency back-pressure or miss the periodic eviction trigger.
func (s *peerNonceStore) MarkIfNew(nonce string, now time.Time) bool {
	if s == nil || nonce == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.current[nonce]; exists {
		return false
	}

	if _, exists := s.previous[nonce]; exists {
		return false
	}

	// Rotation point 1: capacity-triggered emergency valve.
	if len(s.current)+len(s.previous) >= s.maxSize {
		if now.Sub(s.rotatedAt) >= s.window {
			s.previous = s.current
			s.current = make(map[string]struct{}, s.maxSize/nonceStoreMapDivisor)
			s.rotatedAt = now
		} else {
			return false
		}
	}

	s.current[nonce] = struct{}{}

	// Rotation point 2: periodic steady-state rotation.
	if len(s.current) >= s.maxSize/nonceStoreMapDivisor && now.Sub(s.rotatedAt) >= s.window {
		s.previous = s.current
		s.current = make(map[string]struct{}, s.maxSize/nonceStoreMapDivisor)
		s.rotatedAt = now
	}

	return true
}

// peerClient represents a gRPC connection to another authorizer instance in the
// cluster. Each peer owns a contiguous range of shard IDs. During cross-shard
// transactions, the coordinator calls PrepareAuthorize/CommitPrepared/AbortPrepared
// on the peer that owns the remote shards.
type peerClient struct {
	addr       string
	clients    []authorizerv1.BalanceAuthorizerClient
	conns      []*grpc.ClientConn
	next       atomic.Uint64
	shardStart int
	shardEnd   int
}

// pickClient returns the next client in the pool using lock-free round-robin.
// Returns nil when the receiver is nil or the pool is empty (defense-in-depth).
func (p *peerClient) pickClient() authorizerv1.BalanceAuthorizerClient {
	if p == nil || len(p.clients) == 0 {
		return nil
	}

	idx := p.next.Add(1) - 1

	return p.clients[idx%uint64(len(p.clients))]
}

type authorizerService struct {
	authorizerv1.UnimplementedBalanceAuthorizerServer

	engine                 *engine.Engine
	loader                 *loader.PostgresLoader
	pub                    publisher.Publisher
	logger                 libLog.Logger
	metrics                *authorizerMetrics
	started                time.Time
	grpcAddr               string
	instanceAddr           string
	ownedShardStart        int
	ownedShardEnd          int
	peers                  []*peerClient
	peerAuthToken          string
	peerAuthTokenPrev      string
	peerAuthMaxSkew        time.Duration
	abortRPCDeadline       time.Duration
	commitRPCDeadline      time.Duration
	peerPrepareSem         chan struct{}
	peerPrepareBoundedWait time.Duration
	asyncCommitIntent      bool
	peerNonceStore         *peerNonceStore
	walReconciler          *walReconciler
}

// Authorize processes a transaction authorization request.
//
// SECURITY: All internal RPCs require peer-auth headers regardless of peer
// count. The peer-auth token is a local-daemon authentication requirement,
// not a cross-instance-only feature. Single-instance deployments MUST still
// configure AUTHORIZER_PEER_AUTH_TOKEN, and callers (the transaction service
// as well as any sidecar tooling) MUST sign every internal RPC. Network
// isolation (Kubernetes NetworkPolicy, Docker network, or equivalent) is a
// defense-in-depth control, not the sole gate; a misconfigured NetworkPolicy
// or internal recon tool reaching :50051 cannot issue LoadBalances,
// GetBalance, PublishBalanceOperations, or Authorize without valid HMAC
// credentials.
func (s *authorizerService) Authorize(ctx context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	if err := s.authorizeInternalRPC(ctx, peerRPCMethodAuthorize, req); err != nil {
		return nil, err
	}

	return s.authorizeWithMetric(ctx, req, "authorize")
}

func (s *authorizerService) authorizeWithMetric(ctx context.Context, req *authorizerv1.AuthorizeRequest, metricOperation string) (*authorizerv1.AuthorizeResponse, error) {
	// Early O(1) check: reject oversized payloads before allocating shard maps.
	if rejection := s.engine.ValidateRequestLimits(req); rejection != nil {
		return rejection, nil
	}

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

				return nil, status.Error(codes.Internal, "authorizer shard ownership misconfiguration") //nolint:wrapcheck // gRPC status error
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
			s.metrics.RecordAuthorize(ctx, metricOperation, "error", engine.RejectionInternalError, pending, transactionStatus, operationsCount, shardCount, latency, false)
		}

		s.logger.Errorf("Authorizer authorize failed: tx_id=%s org_id=%s ledger_id=%s ops=%d err=%v", transactionID, organizationID, ledgerID, operationsCount, err)

		return nil, status.Error(codes.Internal, "authorize failed") //nolint:wrapcheck // gRPC status error
	}

	result := "authorized"
	rejectionCode := ""

	if !resp.GetAuthorized() {
		result = "rejected"
		rejectionCode = resp.GetRejectionCode()
	}

	if metricsEnabled {
		s.metrics.RecordAuthorize(ctx, metricOperation, result, rejectionCode, pending, transactionStatus, operationsCount, shardCount, latency, false)
	}

	return resp, nil
}

// AuthorizeStream processes a bidirectional stream of authorization requests.
func (s *authorizerService) AuthorizeStream(stream grpc.BidiStreamingServer[authorizerv1.AuthorizeRequest, authorizerv1.AuthorizeResponse]) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("stream recv: %w", err)
		}

		if err := s.authorizeInternalRPC(stream.Context(), peerRPCMethodAuthorizeStream, req); err != nil {
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
			return fmt.Errorf("stream send: %w", err)
		}
	}
}

// LoadBalances loads balances from PostgreSQL into the in-memory engine for the given shards.
func (s *authorizerService) LoadBalances(ctx context.Context, req *authorizerv1.LoadBalancesRequest) (*authorizerv1.LoadBalancesResponse, error) {
	if err := s.authorizeInternalRPC(ctx, peerRPCMethodLoadBalances, req); err != nil {
		return nil, err
	}

	balances, err := s.loader.LoadBalances(ctx, req.GetOrganizationId(), req.GetLedgerId(), req.GetShardIds())
	if err != nil {
		return nil, status.Error(codes.Internal, "load balances failed") //nolint:wrapcheck // gRPC status error
	}

	loaded := s.engine.UpsertBalances(balances)

	return &authorizerv1.LoadBalancesResponse{
		BalancesLoaded: loaded,
		ShardsLoaded:   int64(len(req.GetShardIds())),
	}, nil
}

// GetBalance returns the current snapshot of a single in-memory balance.
func (s *authorizerService) GetBalance(ctx context.Context, req *authorizerv1.GetBalanceRequest) (*authorizerv1.GetBalanceResponse, error) {
	if err := s.authorizeInternalRPC(ctx, peerRPCMethodGetBalance, req); err != nil {
		return nil, err
	}

	balance, ok := s.engine.GetBalance(req.GetOrganizationId(), req.GetLedgerId(), req.GetAccountAlias(), req.GetBalanceKey())
	if !ok {
		return nil, status.Error(codes.NotFound, "balance not found") //nolint:wrapcheck // gRPC status error
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

// PublishBalanceOperations publishes balance operation payloads to the configured message broker.
func (s *authorizerService) PublishBalanceOperations(ctx context.Context, req *authorizerv1.PublishBalanceOperationsRequest) (*authorizerv1.PublishBalanceOperationsResponse, error) {
	if err := s.authorizeInternalRPC(ctx, peerRPCMethodPublishBalanceOp, req); err != nil {
		return nil, err
	}

	if req == nil || len(req.GetPayload()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "payload is required") //nolint:wrapcheck // gRPC status error
	}

	if s.pub == nil {
		return nil, status.Error(codes.FailedPrecondition, "publisher is not configured") //nolint:wrapcheck // gRPC status error
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

		return nil, status.Error(codes.Unavailable, "publish failed") //nolint:wrapcheck // gRPC status error
	}

	return &authorizerv1.PublishBalanceOperationsResponse{Published: true, Message: "published"}, nil
}

func (s *authorizerService) authorizeInternalRPC(ctx context.Context, expectedMethod string, req proto.Message) error {
	if s == nil {
		return status.Error(codes.Internal, "authorizer service not initialized") //nolint:wrapcheck // gRPC status error
	}

	// Peer authentication is mandatory for every internal RPC regardless of
	// whether peers are configured. Single-instance deployments MUST also
	// supply AUTHORIZER_PEER_AUTH_TOKEN; the token is a local-daemon
	// authentication requirement, not a cross-instance-only feature. Any
	// request that reaches :50051 without valid HMAC headers is rejected.
	if s.peerAuthToken == "" {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonMissingToken)

		return status.Error(codes.Unauthenticated, "peer authentication required (AUTHORIZER_PEER_AUTH_TOKEN not configured)") //nolint:wrapcheck // gRPC status error
	}

	return s.authorizePeerRPC(ctx, expectedMethod, req)
}

// recordUnauthorizedRPC emits authorizer_unauthorized_rpc_total and an audit
// log line for every peer-auth rejection. Metric labels are method (gRPC full
// method name or "unknown" when unavailable) and reason (stable enum).
func (s *authorizerService) recordUnauthorizedRPC(ctx context.Context, method, reason string) {
	if s == nil {
		return
	}

	normalizedMethod := method
	if normalizedMethod == "" {
		normalizedMethod = labelUnknown
	}

	if s.metrics != nil {
		s.metrics.RecordUnauthorizedRPC(ctx, normalizedMethod, reason)
	}

	if s.logger != nil {
		s.logger.Warnf("Authorizer internal RPC rejected: method=%s reason=%s", normalizedMethod, reason)
	}
}

// withPeerAuth delegates to the shared mgrpc.WithPeerAuth package.
func withPeerAuth(ctx context.Context, token, method string, req proto.Message) (context.Context, error) {
	return mgrpc.WithPeerAuth(ctx, token, method, req)
}

func (s *authorizerService) authorizePeerRPC(ctx context.Context, expectedMethod string, req proto.Message) error { //nolint:cyclop,gocyclo // peer auth validation requires sequential checks
	if s.peerAuthToken == "" {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonMissingToken)

		return status.Error(codes.Unauthenticated, "peer authentication is not configured") //nolint:wrapcheck // gRPC status error
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonMissingHeaders)

		return status.Error(codes.PermissionDenied, "missing peer credentials") //nolint:wrapcheck // gRPC status error
	}

	receivedTimestamp := md.Get(mgrpc.PeerAuthTimestampHeader)
	receivedNonce := md.Get(mgrpc.PeerAuthNonceHeader)
	receivedMethod := md.Get(mgrpc.PeerAuthMethodHeader)
	receivedBodyHash := md.Get(mgrpc.PeerAuthBodyHashHeader)

	receivedSignature := md.Get(mgrpc.PeerAuthSignatureHeader)
	if len(receivedTimestamp) == 0 || len(receivedNonce) == 0 || len(receivedMethod) == 0 || len(receivedBodyHash) == 0 || len(receivedSignature) == 0 {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonMissingHeaders)

		return status.Error(codes.PermissionDenied, "missing peer credentials") //nolint:wrapcheck // gRPC status error
	}

	rawTimestamp := strings.TrimSpace(receivedTimestamp[0])
	rawNonce := strings.TrimSpace(receivedNonce[0])
	rawMethod := strings.TrimSpace(receivedMethod[0])
	rawBodyHash := strings.TrimSpace(receivedBodyHash[0])
	providedSignature := strings.TrimSpace(receivedSignature[0])

	if rawNonce == "" || rawMethod == "" || rawBodyHash == "" || providedSignature == "" {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonMissingHeaders)

		return status.Error(codes.PermissionDenied, "invalid peer credentials") //nolint:wrapcheck // gRPC status error
	}

	if expectedMethod != "" && rawMethod != expectedMethod {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonWrongAlgo)

		return status.Error(codes.PermissionDenied, "invalid peer credentials") //nolint:wrapcheck // gRPC status error
	}

	unixSeconds, err := strconv.ParseInt(rawTimestamp, 10, 64)
	if err != nil {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonBadTimestamp)

		return status.Error(codes.PermissionDenied, "invalid peer credentials") //nolint:wrapcheck // gRPC status error
	}

	issuedAt := time.Unix(unixSeconds, 0)
	now := time.Now()

	maxSkew := s.peerAuthMaxSkew
	if maxSkew <= 0 {
		maxSkew = defaultPeerAuthMaxSkew
	}

	if issuedAt.After(now.Add(maxSkew)) || now.Sub(issuedAt) > maxSkew {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonTimestampSkew)

		return status.Error(codes.PermissionDenied, "expired peer credentials") //nolint:wrapcheck // gRPC status error
	}

	expectedBodyHash, hashErr := mgrpc.HashPeerAuthBody(req)
	if hashErr != nil {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonHashInternal)

		return status.Errorf(codes.Internal, "hash peer auth body: %v", hashErr) //nolint:wrapcheck // gRPC status error
	}

	if subtle.ConstantTimeCompare([]byte(rawBodyHash), []byte(expectedBodyHash)) != 1 {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonBodyMismatch)

		return status.Error(codes.PermissionDenied, "invalid peer credentials") //nolint:wrapcheck // gRPC status error
	}

	expectedSignatures := []string{mgrpc.SignPeerAuth(s.peerAuthToken, rawTimestamp, rawNonce, rawMethod, rawBodyHash)}
	if s.peerAuthTokenPrev != "" {
		expectedSignatures = append(expectedSignatures, mgrpc.SignPeerAuth(s.peerAuthTokenPrev, rawTimestamp, rawNonce, rawMethod, rawBodyHash))
	}

	validSignature := false

	for _, expectedSignature := range expectedSignatures {
		if subtle.ConstantTimeCompare([]byte(providedSignature), []byte(expectedSignature)) == 1 {
			validSignature = true
			break
		}
	}

	if !validSignature {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonInvalidHMAC)

		return status.Error(codes.PermissionDenied, "invalid peer credentials") //nolint:wrapcheck // gRPC status error
	}

	if s.peerNonceStore == nil {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonNonceInternal)

		return status.Error(codes.Internal, "peer nonce store not initialized") //nolint:wrapcheck // gRPC status error
	}

	if !s.peerNonceStore.MarkIfNew(rawNonce, now) {
		s.recordUnauthorizedRPC(ctx, expectedMethod, unauthorizedReasonNonceReplay)

		return status.Error(codes.PermissionDenied, "replayed peer credentials") //nolint:wrapcheck // gRPC status error
	}

	return nil
}

// PrepareAuthorize executes the prepare phase of 2PC for cross-shard transactions.
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
			// Slow path: bounded wait before shedding.
			if s.peerPrepareBoundedWait > 0 {
				waitCtx, waitCancel := context.WithTimeout(ctx, s.peerPrepareBoundedWait)
				defer waitCancel()

				select {
				case s.peerPrepareSem <- struct{}{}:
					defer func() {
						<-s.peerPrepareSem
					}()
				case <-waitCtx.Done():
					return nil, status.Error(codes.ResourceExhausted, "too many concurrent prepare requests") //nolint:wrapcheck // gRPC status error
				}
			} else {
				return nil, status.Error(codes.ResourceExhausted, "too many concurrent prepare requests") //nolint:wrapcheck // gRPC status error
			}
		}
	}

	start := time.Now()

	ptx, resp, err := s.engine.PrepareAuthorize(req)

	latency := time.Since(start)

	if err != nil {
		if s.metrics.Enabled() {
			// PrepareAuthorize is only ever called by a 2PC coordinator on a remote peer,
			// so cross_shard=true correctly identifies this as part of a cross-shard path.
			s.metrics.RecordAuthorize(ctx, "prepare_authorize", "error", engine.RejectionInternalError, false, "", 0, 0, latency, true)
		}

		return nil, status.Error(codes.Internal, "prepare authorize failed") //nolint:wrapcheck // gRPC status error
	}

	if resp == nil {
		return nil, status.Error(codes.Internal, "prepare authorize failed") //nolint:wrapcheck // gRPC status error
	}

	if !resp.GetAuthorized() {
		return &authorizerv1.PrepareAuthorizeResponse{
			Authorized:       false,
			RejectionCode:    resp.GetRejectionCode(),
			RejectionMessage: resp.GetRejectionMessage(),
		}, nil
	}

	if s.metrics.Enabled() {
		// PrepareAuthorize is only ever called by a 2PC coordinator on a remote peer,
		// so cross_shard=true correctly identifies this as part of a cross-shard path.
		s.metrics.RecordAuthorize(ctx, "prepare_authorize", "prepared", "", req.GetPending(), req.GetTransactionStatus(), len(req.GetOperations()), 0, latency, true)
	}

	preparedTxID := ""
	if ptx != nil {
		preparedTxID = ptx.ID

		// Persist the prepared intent to the WAL (D1 audit finding #2).
		// A crash between here and CommitPrepared would otherwise leave the
		// coordinator holding a prepared_tx_id that points at nothing; on
		// restart replayPreparedIntents re-prepares this transaction so
		// coordinator-initiated CommitPrepared / AbortPrepared calls find
		// the matching entry.
		//
		// Persistence failure is non-fatal to the prepare: the in-memory
		// prepStore entry remains live, the caller receives the standard
		// PrepareAuthorizeResponse, and the WAL write-error metric
		// (observeWALWriteError counter) carries the signal so operators
		// can alert on append failures independently. Failing the prepare
		// here would discard a perfectly good in-memory lock purely
		// because the durability-enhancement path failed — making the
		// enhancement strictly worse than the baseline behavior.
		if persistErr := persistPreparedIntent(s.engine, ptx, s.logger); persistErr != nil {
			s.logger.Warnf("prepared-intent WAL persistence degraded (prepare continues) tx=%s err=%v", preparedTxID, persistErr)
		}
	}

	return &authorizerv1.PrepareAuthorizeResponse{
		Authorized:   true,
		PreparedTxId: preparedTxID,
		Balances:     resp.GetBalances(),
	}, nil
}

// CommitPrepared commits a previously prepared 2PC transaction.
func (s *authorizerService) CommitPrepared(ctx context.Context, req *authorizerv1.CommitPreparedRequest) (*authorizerv1.CommitPreparedResponse, error) {
	if err := s.authorizePeerRPC(ctx, peerRPCMethodCommitPrepared, req); err != nil {
		return nil, err
	}

	if req == nil || strings.TrimSpace(req.GetPreparedTxId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "prepared_tx_id is required") //nolint:wrapcheck // gRPC status error
	}

	start := time.Now()

	resp, err := s.engine.CommitPrepared(req.GetPreparedTxId())

	latency := time.Since(start)

	if err != nil {
		s.logger.Warnf("CommitPrepared failed: prepared_tx_id=%s err=%v latency=%v", shortenPreparedTxID(req.GetPreparedTxId()), err, latency)

		if errors.Is(err, engine.ErrPreparedTxNotFound) {
			return nil, status.Error(codes.NotFound, "prepared transaction not found") //nolint:wrapcheck // gRPC status error
		}

		return nil, status.Error(codes.Internal, "commit prepared failed") //nolint:wrapcheck // gRPC status error
	}

	if s.metrics.Enabled() {
		// CommitPrepared is only ever called by a 2PC coordinator on a remote peer,
		// so cross_shard=true correctly identifies this as part of a cross-shard path.
		s.metrics.RecordAuthorize(ctx, "commit_prepared", "committed", "", false, "", 0, 0, latency, true)
	}

	return &authorizerv1.CommitPreparedResponse{
		Committed: true,
		Balances:  resp.GetBalances(),
	}, nil
}

// AbortPrepared rolls back a previously prepared 2PC transaction.
func (s *authorizerService) AbortPrepared(ctx context.Context, req *authorizerv1.AbortPreparedRequest) (*authorizerv1.AbortPreparedResponse, error) {
	if err := s.authorizePeerRPC(ctx, peerRPCMethodAbortPrepared, req); err != nil {
		return nil, err
	}

	if req == nil || strings.TrimSpace(req.GetPreparedTxId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "prepared_tx_id is required") //nolint:wrapcheck // gRPC status error
	}

	err := s.engine.AbortPrepared(req.GetPreparedTxId())
	if err != nil {
		s.logger.Warnf("AbortPrepared failed: prepared_tx_id=%s err=%v", shortenPreparedTxID(req.GetPreparedTxId()), err)

		if errors.Is(err, engine.ErrPreparedTxNotFound) {
			return nil, status.Error(codes.NotFound, "prepared transaction not found") //nolint:wrapcheck // gRPC status error
		}

		if errors.Is(err, engine.ErrPreparedTxAlreadyCommitted) || errors.Is(err, engine.ErrPreparedTxCommitDecided) {
			return nil, status.Error(codes.FailedPrecondition, "prepared transaction commit already decided") //nolint:wrapcheck // gRPC status error
		}

		return nil, status.Error(codes.Internal, "abort prepared failed") //nolint:wrapcheck // gRPC status error
	}

	return &authorizerv1.AbortPreparedResponse{Aborted: true}, nil
}

func shortenPreparedTxID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= shortenedIDMaxLen {
		return id
	}

	return id[:shortenedIDMaxLen] + "..."
}

type peerShardRange struct {
	start int
	end   int
}

func parsePeerShardRange(raw string, shardCount int) (peerShardRange, error) {
	sr, err := mgrpc.ParseShardRange(raw)
	if err != nil {
		return peerShardRange{}, fmt.Errorf("invalid peer shard range: %w", err)
	}

	if err := mgrpc.ValidateShardRangeBounds(sr, shardCount); err != nil {
		return peerShardRange{}, err
	}

	return peerShardRange{start: sr.Start, end: sr.End}, nil
}

func validateShardCoverage(shardCount, localStart, localEnd int, peers []peerShardRange) error {
	if shardCount <= 0 {
		return fmt.Errorf("validate shard coverage: %w", constant.ErrShardCountInvalid)
	}

	covered := make([]bool, shardCount)

	mark := func(start, end int, owner string) error {
		for shardID := start; shardID <= end; shardID++ {
			if covered[shardID] {
				return fmt.Errorf("shard %d (latest=%s): %w", shardID, owner, constant.ErrShardMultipleOwners)
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
			return fmt.Errorf("shard %d (expected coverage 0-%d): %w", shardID, shardCount-1, constant.ErrShardNoOwner)
		}
	}

	return nil
}

// resolvePeerShardRanges computes and validates the shard ranges for all configured peers.
func resolvePeerShardRanges(cfg *Config) ([]peerShardRange, error) {
	peerRanges := make([]peerShardRange, 0, len(cfg.PeerInstances))

	if len(cfg.PeerInstances) == 0 {
		if cfg.OwnedShardStart != 0 || cfg.OwnedShardEnd != cfg.ShardCount-1 {
			return nil, fmt.Errorf("expected 0-%d got %d-%d: %w", cfg.ShardCount-1, cfg.OwnedShardStart, cfg.OwnedShardEnd, constant.ErrLocalShardCoverageIncomplete)
		}

		return peerRanges, nil
	}

	if len(cfg.PeerShardRanges) > 0 { //nolint:nestif // explicit vs inferred shard ranges
		if len(cfg.PeerShardRanges) != len(cfg.PeerInstances) {
			return nil, fmt.Errorf("ranges=%d instances=%d: %w", len(cfg.PeerShardRanges), len(cfg.PeerInstances), constant.ErrPeerShardRangeCountMismatch)
		}

		for _, rawRange := range cfg.PeerShardRanges {
			parsed, parseErr := parsePeerShardRange(rawRange, cfg.ShardCount)
			if parseErr != nil {
				return nil, parseErr
			}

			if parsed.end >= cfg.OwnedShardStart && parsed.start <= cfg.OwnedShardEnd {
				return nil, fmt.Errorf("peer=%d-%d local=%d-%d: %w", parsed.start, parsed.end, cfg.OwnedShardStart, cfg.OwnedShardEnd, constant.ErrPeerShardRangeOverlapsLocal)
			}

			peerRanges = append(peerRanges, parsed)
		}
	} else {
		inferred, inferErr := inferSinglePeerRange(cfg)
		if inferErr != nil {
			return nil, inferErr
		}

		peerRanges = append(peerRanges, inferred)
	}

	if err := validatePeerRangeOverlaps(peerRanges); err != nil {
		return nil, err
	}

	if err := validateShardCoverage(cfg.ShardCount, cfg.OwnedShardStart, cfg.OwnedShardEnd, peerRanges); err != nil {
		return nil, err
	}

	return peerRanges, nil
}

// inferSinglePeerRange infers the shard range for a single peer based on local ownership.
func inferSinglePeerRange(cfg *Config) (peerShardRange, error) {
	if len(cfg.PeerInstances) > 1 {
		return peerShardRange{}, fmt.Errorf("peers=%d: %w", len(cfg.PeerInstances), constant.ErrPeerShardRangesRequired)
	}

	if cfg.OwnedShardStart > 0 && cfg.OwnedShardEnd < cfg.ShardCount-1 {
		return peerShardRange{}, fmt.Errorf("ownership=%d-%d: %w", cfg.OwnedShardStart, cfg.OwnedShardEnd, constant.ErrPeerShardRangeCannotInfer)
	}

	if cfg.OwnedShardStart > 0 {
		return peerShardRange{start: 0, end: cfg.OwnedShardStart - 1}, nil
	}

	if cfg.OwnedShardEnd >= cfg.ShardCount-1 {
		return peerShardRange{}, fmt.Errorf("ownership=%d-%d: %w", cfg.OwnedShardStart, cfg.OwnedShardEnd, constant.ErrPeerOwnsAllShards)
	}

	return peerShardRange{start: cfg.OwnedShardEnd + 1, end: cfg.ShardCount - 1}, nil
}

// validatePeerRangeOverlaps checks that no two peer shard ranges overlap.
func validatePeerRangeOverlaps(peerRanges []peerShardRange) error {
	for i := 0; i < len(peerRanges); i++ {
		for j := i + 1; j < len(peerRanges); j++ {
			if peerRanges[i].end >= peerRanges[j].start && peerRanges[j].end >= peerRanges[i].start {
				return fmt.Errorf("ranges %d-%d and %d-%d: %w", peerRanges[i].start, peerRanges[i].end, peerRanges[j].start, peerRanges[j].end, constant.ErrPeerShardRangesOverlap)
			}
		}
	}

	return nil
}

// buildPeerTransportOption creates the gRPC transport credentials dial option
// based on TLS configuration for peer connections.
func buildPeerTransportOption(cfg *Config) (grpc.DialOption, error) {
	if !cfg.GRPCTLSEnabled {
		return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if cfg.PeerTLSCAFile != "" {
		caBundle, readErr := os.ReadFile(cfg.PeerTLSCAFile)
		if readErr != nil {
			return nil, fmt.Errorf("read AUTHORIZER_PEER_TLS_CA_FILE: %w", readErr)
		}

		rootCAs := x509.NewCertPool()
		if !rootCAs.AppendCertsFromPEM(caBundle) {
			return nil, fmt.Errorf("file=%s: %w", cfg.PeerTLSCAFile, constant.ErrPeerTLSCANoCertificates)
		}

		tlsCfg.RootCAs = rootCAs
	}

	return grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)), nil
}

// productionLikeEnvNamesForPublisher is the authoritative list of ENV_NAME values
// that require a durable publisher. Unlike the broader broker/security helper, this
// list treats staging as production-equivalent so commit intents and balance
// operations cannot be silently dropped in any pre-prod environment where correctness
// testing mirrors production behavior.
var productionLikeEnvNamesForPublisher = map[string]struct{}{
	"production": {},
	"staging":    {},
	"prod":       {},
	"stg":        {},
	"prd":        {},
	"pre-prod":   {},
	"preprod":    {},
}

// isProductionLikeEnvForPublisher reports whether the environment must use a durable
// publisher (i.e. the NoopPublisher is not acceptable because publishes would be
// silently dropped).
func isProductionLikeEnvForPublisher(envName string) bool {
	_, ok := productionLikeEnvNamesForPublisher[strings.ToLower(strings.TrimSpace(envName))]
	return ok
}

// validatePublisherSelection emits an audit log line noting which publisher was
// selected and fail-closes when the NoopPublisher would be selected in a
// production-like environment (where silent drops of commit intents and balance
// operations are unacceptable).
func validatePublisherSelection(cfg *Config, logger libLog.Logger) error {
	if cfg.RedpandaEnabled {
		logger.Infof(
			"publisher audit: selected=redpanda env_name=%q redpanda_enabled=%t",
			cfg.EnvName,
			cfg.RedpandaEnabled,
		)

		return nil
	}

	if isProductionLikeEnvForPublisher(cfg.EnvName) {
		logger.Errorf(
			"publisher audit: selected=noop env_name=%q redpanda_enabled=%t decision=abort reason=%q",
			cfg.EnvName,
			cfg.RedpandaEnabled,
			"NoopPublisher is not permitted in production-like environments; enable AUTHORIZER_REDPANDA_ENABLED=true",
		)

		return fmt.Errorf("environment %q: %w", cfg.EnvName, constant.ErrNoopPublisherNotAllowedInProd)
	}

	logger.Warnf(
		"publisher audit: selected=noop env_name=%q redpanda_enabled=%t SILENT DROP RISK — commit intents and balance ops publishes will NOT be durable",
		cfg.EnvName,
		cfg.RedpandaEnabled,
	)

	return nil
}

// Run starts the authorizer gRPC server and blocks until shutdown.
func Run(ctx context.Context, cfg *Config, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) error { //nolint:gocognit,gocyclo,cyclop // startup orchestration function
	metricRecorder := newAuthorizerMetrics(telemetry, logger, cfg.AuthorizeLatencySLO)
	router := shard.NewRouter(cfg.ShardCount)

	// Readiness gate (D1 audit finding #1): the gRPC health service is
	// created up-front and pinned to NOT_SERVING until the initial balance
	// load AND WAL replay complete. Flipping to SERVING too early allows
	// the transaction component and other gRPC clients to route Authorize
	// RPCs against an empty engine — every request would then be rejected
	// with "balance not found" despite the balance existing in PostgreSQL.
	//
	// We construct the health server before the load begins and register it
	// on the gRPC server later in this function; the listener only binds
	// after load+replay so on cold start clients see connection refused
	// (which k8s readiness interprets as NotReady, identical to NOT_SERVING).
	// Post-load the listener binds and the health status is promoted, giving
	// operators a single flip transition to watch for.
	healthServer := grpcHealth.NewServer()
	healthServer.SetServingStatus("authorizer.v1.BalanceAuthorizer", grpcHealthV1.HealthCheckResponse_NOT_SERVING)
	healthServer.SetServingStatus("", grpcHealthV1.HealthCheckResponse_NOT_SERVING)

	eng := engine.New(router, wal.NewNoopWriter())
	defer eng.Close()

	eng.SetObserver(metricRecorder)
	eng.ConfigurePreparedTxStore(cfg.PrepareTimeout, cfg.PrepareMaxPending)
	eng.ConfigureAuthorizationLimits(cfg.MaxOperationsPerRequest, cfg.MaxUniqueBalancesPerRequest)
	eng.ConfigureReplayPolicy(
		cfg.WALReplayMaxMutationsPerEntry,
		cfg.WALReplayMaxUniqueBalancesPerEntry,
		cfg.WALReplayStrictMode,
	)

	if cfg.WALReplayStrictMode {
		logger.Warnf("Authorizer WAL replay strict mode enabled: entries with replay inconsistencies will fail startup")
	}

	eng.ConfigurePreparedTxRetention(cfg.PrepareCommittedRetention, cfg.PrepareCommitRetryLimit)

	balanceLoader, err := loader.NewPostgresLoaderWithConfig(ctx, cfg.PostgresDSN, router, loader.PoolConfig{
		MaxConns:          cfg.PostgresPoolMaxConns,
		MinConns:          cfg.PostgresPoolMinConns,
		MaxConnLifetime:   cfg.PostgresPoolMaxConnLife,
		MaxConnIdleTime:   cfg.PostgresPoolMaxConnIdle,
		HealthCheckPeriod: cfg.PostgresPoolHealthCheck,
		ConnectTimeout:    cfg.PostgresConnectTimeout,
		EnvName:           cfg.EnvName,
		StatementTimeout:  cfg.PostgresStatementTimeout,
	})
	if err != nil {
		return fmt.Errorf("initialize postgres loader: %w", err)
	}

	defer balanceLoader.Close()

	// Streaming load (D1 audit finding #3): keyset-paginated batches fan out to
	// UpsertBalances workers so cold start memory use is bounded to one
	// batch × workers instead of the previous unbounded full-table scan that
	// pinned ~50GB for large ledgers. On failure the error propagates up to
	// the caller which terminates the process — by design, health has not
	// yet been flipped to SERVING so readiness consumers (k8s, LB) will
	// see NOT_SERVING and route traffic elsewhere.
	var loaded int64

	loaded, err = balanceLoader.LoadBalancesStreaming(
		ctx,
		"",
		"",
		cfg.ShardIDs,
		time.Time{}, // zero == include all rows
		func(batch []*engine.Balance) error {
			eng.UpsertBalances(batch)
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("load initial balances: %w", err)
	}

	logger.Infof("Authorizer loaded balances from PostgreSQL: %d", loaded)
	logger.Infof(
		"Authorizer runtime config: grpc_address=%s shards=%d shard_ids=%v wal_buffer_size=%d wal_flush_interval_ms=%d wal_sync_on_append=%t prepare_timeout_ms=%d prepare_max_pending=%d max_ops_per_request=%d max_unique_balances_per_request=%d wal_replay_max_mutations=%d wal_replay_max_unique_balances=%d wal_replay_strict_mode=%t authorize_latency_slo_ms=%d max_streams=%d max_recv_bytes=%d postgres_pool_max_conns=%d postgres_pool_min_conns=%d postgres_conn_lifetime_ms=%d postgres_conn_idle_ms=%d postgres_healthcheck_ms=%d redpanda_enabled=%t redpanda_backpressure_policy=%s redpanda_retries=%d redpanda_delivery_timeout_ms=%d redpanda_publish_timeout_ms=%d telemetry_enabled=%t",
		cfg.GRPCAddress,
		cfg.ShardCount,
		cfg.ShardIDs,
		cfg.WALBufferSize,
		cfg.WALFlushInterval.Milliseconds(),
		cfg.WALSyncOnAppend,
		cfg.PrepareTimeout.Milliseconds(),
		cfg.PrepareMaxPending,
		cfg.MaxOperationsPerRequest,
		cfg.MaxUniqueBalancesPerRequest,
		cfg.WALReplayMaxMutationsPerEntry,
		cfg.WALReplayMaxUniqueBalancesPerEntry,
		cfg.WALReplayStrictMode,
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

	// Replay verifies every frame's HMAC against the current key; rotation is
	// enabled by supplying AUTHORIZER_WAL_HMAC_KEY_PREVIOUS (WALHMACKeyPrevious).
	replayKeys := [][]byte{cfg.WALHMACKey}
	if len(cfg.WALHMACKeyPrevious) > 0 {
		replayKeys = append(replayKeys, cfg.WALHMACKeyPrevious)
	}

	entries, err := wal.Replay(cfg.WALPath, replayKeys, metricRecorder)
	if err != nil {
		return fmt.Errorf("replay wal: %w", err)
	}

	if err := eng.ReplayEntries(entries); err != nil {
		return fmt.Errorf("apply wal replay: %w", err)
	}

	logger.Infof("Authorizer replayed WAL entries: %d", len(entries))

	// Prepared-intent replay (D1 audit finding #2): rebuild in-memory
	// prepStore entries for prepared-but-not-committed 2PC transactions so
	// post-restart CommitPrepared / AbortPrepared calls from coordinators
	// find the matching entry. This MUST run AFTER ReplayEntries (so balance
	// state reflects all committed mutations) and BEFORE the readiness gate
	// flips to SERVING (so the first incoming RPC is served against a
	// fully-restored prepStore).
	if err := replayPreparedIntents(eng, entries, logger, cfg.WALReplayStrictMode, cfg.PrepareTimeout); err != nil {
		return fmt.Errorf("replay prepared intents: %w", err)
	}

	writer, err := wal.NewRingBufferWriterWithOptions(
		cfg.WALPath,
		cfg.WALBufferSize,
		cfg.WALFlushInterval,
		cfg.WALSyncOnAppend,
		metricRecorder,
		cfg.WALHMACKey,
	)
	if err != nil {
		return fmt.Errorf("initialize WAL writer: %w", err)
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

	if err := validatePublisherSelection(cfg, logger); err != nil {
		return err
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
				// D6: propagate the env so SASL/PLAIN-without-TLS is rejected
				// in production-like environments at publisher construction time.
				Environment: cfg.EnvName,
			},
		)
		if err != nil {
			return fmt.Errorf("initialize redpanda publisher: %w", err)
		}

		pub = redpandaPublisher

		defer func() {
			if closeErr := pub.Close(); closeErr != nil {
				logger.Warnf("publisher close error: %v", closeErr)
			}
		}()
	}

	if len(cfg.PeerInstances) > 0 && !cfg.RedpandaEnabled {
		return fmt.Errorf("durable commit intent: %w", constant.ErrCrossShardRequiresRedpanda)
	}

	if cfg.PeerInsecureAllowed && !brokersecurity.IsNonProductionEnvironment(cfg.EnvName) {
		return fmt.Errorf("environment %q: %w", cfg.EnvName, constant.ErrPeerInsecureNotAllowedInProd)
	}

	allowInsecurePeerTransport := cfg.PeerInsecureAllowed && brokersecurity.IsNonProductionEnvironment(cfg.EnvName)
	if len(cfg.PeerInstances) > 0 && !cfg.GRPCTLSEnabled && !allowInsecurePeerTransport {
		return fmt.Errorf("set AUTHORIZER_GRPC_TLS_ENABLED=true or AUTHORIZER_PEER_INSECURE_ALLOWED=true: %w", constant.ErrPeerRPCRequiresTLS)
	}

	if len(cfg.PeerInstances) > 0 && allowInsecurePeerTransport {
		logger.Warnf("Authorizer peer RPC is using insecure transport for non-production environment %q", cfg.EnvName)
	}

	peerRanges, err := resolvePeerShardRanges(cfg)
	if err != nil {
		return err
	}

	// Connect to peer authorizer instances for cross-shard 2PC coordination.
	var peers []*peerClient

	transportOption, transportErr := buildPeerTransportOption(cfg)
	if transportErr != nil {
		return transportErr
	}

	for i, peerAddr := range cfg.PeerInstances {
		poolSize := cfg.PeerConnPoolSize
		if poolSize < 1 {
			poolSize = 4
		}

		peerConns := make([]*grpc.ClientConn, 0, poolSize)
		peerClients := make([]authorizerv1.BalanceAuthorizerClient, 0, poolSize)

		for j := 0; j < poolSize; j++ {
			peerConn, peerErr := grpc.NewClient(peerAddr,
				transportOption,
				grpc.WithKeepaliveParams(keepalive.ClientParameters{
					Time:                keepaliveTimeSec * time.Second,
					Timeout:             keepaliveTimeoutSec * time.Second,
					PermitWithoutStream: true,
				}),
			)
			if peerErr != nil {
				// Close connections opened in this pool.
				for _, c := range peerConns {
					if closeErr := c.Close(); closeErr != nil {
						logger.Warnf("peer pool connection close error: %v", closeErr)
					}
				}
				// Close any previously opened peer connections.
				for _, p := range peers {
					for _, c := range p.conns {
						if closeErr := c.Close(); closeErr != nil {
							logger.Warnf("peer connection close error: %v", closeErr)
						}
					}
				}

				return fmt.Errorf("connect to authorizer peer %s (conn %d/%d): %w", peerAddr, j+1, poolSize, peerErr)
			}

			peerConns = append(peerConns, peerConn)
			peerClients = append(peerClients, authorizerv1.NewBalanceAuthorizerClient(peerConn))
		}

		peerRange := peerRanges[i]

		peers = append(peers, &peerClient{
			addr:       peerAddr,
			clients:    peerClients,
			conns:      peerConns,
			shardStart: peerRange.start,
			shardEnd:   peerRange.end,
		})

		logger.Infof("Connected to authorizer peer at %s (owns shards %d-%d, pool=%d)", peerAddr, peerRange.start, peerRange.end, poolSize)
	}

	defer func() {
		for _, p := range peers {
			for _, c := range p.conns {
				if closeErr := c.Close(); closeErr != nil {
					logger.Warnf("peer connection close error: %v", closeErr)
				}
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
			Time:    keepaliveTimeSec * time.Second,
			Timeout: keepaliveTimeoutSec * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             enforcementMinTimeSec * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxConcurrentStreams(cfg.MaxConcurrentStreams),
		grpc.MaxRecvMsgSize(cfg.MaxReceiveMessageSizeBytes),
		grpc.InitialWindowSize(grpcInitialWindowBytes),
		grpc.InitialConnWindowSize(grpcInitialConnWinBytes),
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
		engine:                 eng,
		loader:                 balanceLoader,
		pub:                    pub,
		logger:                 logger,
		metrics:                metricRecorder,
		started:                time.Now(),
		grpcAddr:               cfg.GRPCAddress,
		instanceAddr:           cfg.InstanceAddress,
		ownedShardStart:        cfg.OwnedShardStart,
		ownedShardEnd:          cfg.OwnedShardEnd,
		peers:                  peers,
		peerAuthToken:          cfg.PeerAuthToken,
		peerAuthTokenPrev:      cfg.PeerAuthTokenPrevious,
		peerAuthMaxSkew:        cfg.PeerAuthMaxSkew,
		abortRPCDeadline:       cfg.PeerAbortTimeout,
		commitRPCDeadline:      cfg.PeerCommitTimeout,
		peerPrepareSem:         peerPrepareSem,
		peerPrepareBoundedWait: time.Duration(cfg.PeerPrepareBoundedWaitMs) * time.Millisecond,
		asyncCommitIntent:      cfg.AsyncCommitIntent,
		peerNonceStore:         newPeerNonceStore(cfg.PeerAuthMaxSkew, cfg.PeerNonceMaxEntries),
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

		if cfg.WALReconcilerEnabled && cfg.AsyncCommitIntent {
			reconciler := newWALReconciler(cfg, service, logger, metricRecorder)
			service.walReconciler = reconciler
			reconcilerCtx, stopReconciler := context.WithCancel(ctx)

			defer stopReconciler()

			go reconciler.Run(reconcilerCtx)
		}
	}

	authorizerv1.RegisterBalanceAuthorizerServer(server, service)

	if cfg.ReflectionEnabled {
		reflection.Register(server)
		logger.Infof("Authorizer gRPC reflection enabled")
	}

	// Register the health server that was constructed up-front in NOT_SERVING
	// mode. Flip to SERVING only after the initial balance load and WAL
	// replay have both completed — both prerequisites are satisfied above
	// before we reach this point (load errors return early; replay errors
	// return early). LoadedBalances is captured as the observed gauge value
	// so operators can see in dashboards the exact count that gated readiness.
	grpcHealthV1.RegisterHealthServer(server, healthServer)

	metricRecorder.ObserveLoadedBalancesAbsolute(ctx, eng.LoadedBalances())

	healthServer.SetServingStatus("authorizer.v1.BalanceAuthorizer", grpcHealthV1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("", grpcHealthV1.HealthCheckResponse_SERVING)

	logger.Infof(
		"Authorizer health flipped to SERVING (loaded_balances=%d)",
		eng.LoadedBalances(),
	)

	lc := net.ListenConfig{}

	listener, err := lc.Listen(ctx, "tcp", cfg.GRPCAddress)
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
