// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package out

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// AuthorizerConfig configures the outbound authorizer gRPC client.
type AuthorizerConfig struct {
	Enabled     bool
	Host        string
	Port        string
	Timeout     time.Duration
	Streaming   bool
	TLSEnabled  bool
	Environment string
	RoutingMode string
	Instances   string
	ShardRanges string
	ShardCount  int
}

// AuthorizerClient groups all methods required by command/query use cases.
type AuthorizerClient interface {
	Enabled() bool
	Authorize(ctx context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error)
	LoadBalances(ctx context.Context, req *authorizerv1.LoadBalancesRequest) (*authorizerv1.LoadBalancesResponse, error)
	PublishBalanceOperations(ctx context.Context, topic, partitionKey string, payload []byte, headers map[string]string) error
	Close() error
}

// Compile-time interface assertions.
var (
	_ AuthorizerClient = (*AuthorizerGRPCRepository)(nil)
	_ AuthorizerClient = (*ShardedAuthorizerGRPCRepository)(nil)
)

// AuthorizerGRPCRepository is a gRPC client for the authorizer service.
type AuthorizerGRPCRepository struct {
	enabled bool
	timeout time.Duration

	conn   *grpc.ClientConn
	client authorizerv1.BalanceAuthorizerClient
	logger libLog.Logger
}

func NewAuthorizerGRPC(cfg AuthorizerConfig, logger libLog.Logger) (*AuthorizerGRPCRepository, error) {
	repo := &AuthorizerGRPCRepository{
		enabled: cfg.Enabled,
		timeout: cfg.Timeout,
		logger:  logger,
	}

	if !cfg.Enabled {
		return repo, nil
	}

	if repo.timeout <= 0 {
		repo.timeout = 100 * time.Millisecond
	}

	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	var transportCredentials grpc.DialOption
	if cfg.TLSEnabled {
		transportCredentials = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}))
	} else {
		if !isDevelopmentEnvironment(cfg.Environment) {
			return nil, fmt.Errorf("authorizer gRPC insecure transport is not allowed in environment %q", cfg.Environment)
		}

		logger.Warn("gRPC authorizer connection using insecure transport - not recommended for production")

		transportCredentials = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.NewClient(
		addr,
		transportCredentials,
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to authorizer gRPC: %w", err)
	}

	repo.conn = conn
	repo.client = authorizerv1.NewBalanceAuthorizerClient(conn)

	logger.Infof("Connected to authorizer gRPC at %s", addr)

	return repo, nil
}

// NewAuthorizerClient builds a single or shard-routed authorizer client based on config.
func NewAuthorizerClient(cfg AuthorizerConfig, logger libLog.Logger) (AuthorizerClient, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.RoutingMode))
	if mode == "" {
		mode = "single"
	}

	if mode != "sharded" {
		return NewAuthorizerGRPC(cfg, logger)
	}

	instanceAddrs := parseCSV(cfg.Instances)
	if len(instanceAddrs) == 0 {
		instanceAddrs = []string{net.JoinHostPort(cfg.Host, cfg.Port)}
	}

	if len(instanceAddrs) <= 1 {
		return NewAuthorizerGRPC(cfg, logger)
	}

	ranges, err := parseShardRanges(cfg.ShardRanges)
	if err != nil {
		return nil, err
	}

	if len(ranges) != len(instanceAddrs) {
		return nil, fmt.Errorf("authorizer sharded routing requires one shard range per instance: instances=%d ranges=%d", len(instanceAddrs), len(ranges))
	}

	clients := make([]*AuthorizerGRPCRepository, 0, len(instanceAddrs))
	for _, addr := range instanceAddrs {
		host, port, splitErr := net.SplitHostPort(strings.TrimSpace(addr))
		if splitErr != nil {
			return nil, fmt.Errorf("invalid authorizer instance address %q: %w", addr, splitErr)
		}

		client, clientErr := NewAuthorizerGRPC(AuthorizerConfig{
			Enabled:     cfg.Enabled,
			Host:        host,
			Port:        port,
			Timeout:     cfg.Timeout,
			Streaming:   cfg.Streaming,
			TLSEnabled:  cfg.TLSEnabled,
			Environment: cfg.Environment,
		}, logger)
		if clientErr != nil {
			var closeErr error

			for _, created := range clients {
				if created == nil {
					continue
				}

				closeErr = errors.Join(closeErr, created.Close())
			}

			if closeErr != nil {
				return nil, errors.Join(clientErr, fmt.Errorf("failed to close previously created authorizer clients: %w", closeErr))
			}

			return nil, clientErr
		}

		clients = append(clients, client)
	}

	effectiveShardCount := cfg.ShardCount
	if effectiveShardCount <= 0 {
		effectiveShardCount = shard.DefaultShardCount
	}

	router := shard.NewRouter(effectiveShardCount)

	clientByShard := make(map[int]*AuthorizerGRPCRepository)

	for idx, shardRange := range ranges {
		for shardID := shardRange.start; shardID <= shardRange.end; shardID++ {
			clientByShard[shardID] = clients[idx]
		}
	}

	logger.Infof("Authorizer sharded routing enabled: instances=%d ranges=%s", len(instanceAddrs), cfg.ShardRanges)

	return &ShardedAuthorizerGRPCRepository{
		enabled:       cfg.Enabled,
		clients:       clients,
		clientByShard: clientByShard,
		router:        router,
		logger:        logger,
	}, nil
}

type shardRange struct {
	start int
	end   int
}

func parseShardRanges(raw string) ([]shardRange, error) {
	parts := parseCSV(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("AUTHORIZER_SHARD_RANGES is required in sharded routing mode")
	}

	ranges := make([]shardRange, 0, len(parts))
	for _, part := range parts {
		bounds := strings.Split(strings.TrimSpace(part), "-")
		if len(bounds) != 2 {
			return nil, fmt.Errorf("invalid shard range %q: expected start-end", part)
		}

		start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid shard range start %q: %w", bounds[0], err)
		}

		end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid shard range end %q: %w", bounds[1], err)
		}

		if start < 0 || end < start {
			return nil, fmt.Errorf("invalid shard range %q: start=%d end=%d", part, start, end)
		}

		ranges = append(ranges, shardRange{start: start, end: end})
	}

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].start == ranges[j].start {
			return ranges[i].end < ranges[j].end
		}

		return ranges[i].start < ranges[j].start
	})

	for i := 1; i < len(ranges); i++ {
		previous := ranges[i-1]

		current := ranges[i]
		if current.start <= previous.end {
			return nil, fmt.Errorf(
				"invalid shard ranges: overlap between %d-%d and %d-%d",
				previous.start,
				previous.end,
				current.start,
				current.end,
			)
		}
	}

	return ranges, nil
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")

	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		out = append(out, trimmed)
	}

	return out
}

func (a *AuthorizerGRPCRepository) Enabled() bool {
	return a != nil && a.enabled && a.client != nil
}

func (a *AuthorizerGRPCRepository) Authorize(ctx context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	if !a.Enabled() {
		return nil, fmt.Errorf("authorizer gRPC client is disabled")
	}

	var lastErr error

	for attempt := 1; attempt <= 2; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, a.timeout)
		resp, err := a.client.Authorize(callCtx, req, grpc.WaitForReady(true))

		cancel()

		if err == nil {
			return resp, nil
		}

		lastErr = err
		if !isRetryableAuthorizerError(err) || attempt == 2 {
			break
		}

		if sleepErr := sleepWithContext(ctx, 5*time.Millisecond); sleepErr != nil {
			return nil, sleepErr
		}
	}

	return nil, lastErr
}

func (a *AuthorizerGRPCRepository) LoadBalances(ctx context.Context, req *authorizerv1.LoadBalancesRequest) (*authorizerv1.LoadBalancesResponse, error) {
	if !a.Enabled() {
		return nil, fmt.Errorf("authorizer gRPC client is disabled")
	}

	var lastErr error

	for attempt := 1; attempt <= 2; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, a.timeout)
		resp, err := a.client.LoadBalances(callCtx, req, grpc.WaitForReady(true))

		cancel()

		if err == nil {
			return resp, nil
		}

		lastErr = err
		if !isRetryableAuthorizerError(err) || attempt == 2 {
			break
		}

		if sleepErr := sleepWithContext(ctx, 5*time.Millisecond); sleepErr != nil {
			return nil, sleepErr
		}
	}

	return nil, lastErr
}

func (a *AuthorizerGRPCRepository) PublishBalanceOperations(ctx context.Context, topic, partitionKey string, payload []byte, headers map[string]string) error {
	if !a.Enabled() {
		return fmt.Errorf("authorizer gRPC client is disabled")
	}

	req := &authorizerv1.PublishBalanceOperationsRequest{
		Topic:        topic,
		PartitionKey: partitionKey,
		Payload:      payload,
		ContentType:  "application/msgpack",
		Headers:      headers,
	}

	var lastErr error

	for attempt := 1; attempt <= 2; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, a.timeout)
		_, err := a.client.PublishBalanceOperations(callCtx, req, grpc.WaitForReady(true))

		cancel()

		if err == nil {
			return nil
		}

		lastErr = err
		if !isRetryableAuthorizerError(err) || attempt == 2 {
			break
		}

		if sleepErr := sleepWithContext(ctx, 5*time.Millisecond); sleepErr != nil {
			return sleepErr
		}
	}

	return lastErr
}

func isRetryableAuthorizerError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ShardedAuthorizerGRPCRepository routes calls to authorizer instances based on shard.
type ShardedAuthorizerGRPCRepository struct {
	enabled       bool
	clients       []*AuthorizerGRPCRepository
	clientByShard map[int]*AuthorizerGRPCRepository
	router        *shard.Router
	logger        libLog.Logger
}

func (s *ShardedAuthorizerGRPCRepository) Enabled() bool {
	return s != nil && s.enabled && len(s.clients) > 0
}

func (s *ShardedAuthorizerGRPCRepository) Authorize(ctx context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("authorizer gRPC client is disabled")
	}

	shardID := s.resolvePrimaryShard(req.GetOperations())

	client := s.clientForShard(shardID)
	if client == nil {
		return nil, fmt.Errorf("authorizer gRPC client not configured for shard %d", shardID)
	}

	return client.Authorize(ctx, req)
}

func (s *ShardedAuthorizerGRPCRepository) LoadBalances(ctx context.Context, req *authorizerv1.LoadBalancesRequest) (*authorizerv1.LoadBalancesResponse, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("authorizer gRPC client is disabled")
	}

	if len(req.GetShardIds()) == 0 {
		client := s.clientForShard(0)
		if client == nil {
			return nil, fmt.Errorf("authorizer gRPC client not configured")
		}

		return client.LoadBalances(ctx, req)
	}

	grouped := make(map[*AuthorizerGRPCRepository][]int32)

	for _, shardID := range req.GetShardIds() {
		client := s.clientForShard(int(shardID))
		if client == nil {
			return nil, fmt.Errorf("authorizer gRPC client not configured for shard %d", shardID)
		}

		grouped[client] = append(grouped[client], shardID)
	}

	var mu sync.Mutex

	response := &authorizerv1.LoadBalancesResponse{}
	g, gCtx := errgroup.WithContext(ctx)

	for client, shardIDs := range grouped {
		g.Go(func() error {
			copyReq := &authorizerv1.LoadBalancesRequest{
				OrganizationId: req.GetOrganizationId(),
				LedgerId:       req.GetLedgerId(),
				ShardIds:       shardIDs,
			}

			res, err := client.LoadBalances(gCtx, copyReq)
			if err != nil {
				return err
			}

			mu.Lock()
			response.BalancesLoaded += res.GetBalancesLoaded()
			response.ShardsLoaded += res.GetShardsLoaded()
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return response, nil
}

func (s *ShardedAuthorizerGRPCRepository) PublishBalanceOperations(ctx context.Context, topic, partitionKey string, payload []byte, headers map[string]string) error {
	if !s.Enabled() {
		return fmt.Errorf("authorizer gRPC client is disabled")
	}

	shardID, err := strconv.Atoi(strings.TrimSpace(partitionKey))
	if err != nil {
		shardID = 0
	}

	client := s.clientForShard(shardID)
	if client == nil {
		return fmt.Errorf("authorizer gRPC client not configured for shard %d", shardID)
	}

	return client.PublishBalanceOperations(ctx, topic, partitionKey, payload, headers)
}

func (s *ShardedAuthorizerGRPCRepository) Close() error {
	if s == nil {
		return nil
	}

	var closeErr error

	for _, client := range s.clients {
		if client == nil {
			continue
		}

		closeErr = errors.Join(closeErr, client.Close())
	}

	return closeErr
}

func (s *ShardedAuthorizerGRPCRepository) clientForShard(shardID int) *AuthorizerGRPCRepository {
	if s == nil || len(s.clients) == 0 {
		return nil
	}

	if client, ok := s.clientByShard[shardID]; ok && client != nil {
		return client
	}

	return nil
}

func isDevelopmentEnvironment(environment string) bool {
	return brokersecurity.IsNonProductionEnvironment(environment)
}

func (s *ShardedAuthorizerGRPCRepository) resolvePrimaryShard(ops []*authorizerv1.BalanceOperation) int {
	if s == nil || s.router == nil || len(ops) == 0 {
		return 0
	}

	for _, op := range ops {
		if op == nil {
			continue
		}

		alias := op.GetAccountAlias()
		if alias == "" {
			continue
		}

		if op.GetOperation() == constant.DEBIT && !shard.IsExternal(alias) {
			return s.router.ResolveBalance(alias, op.GetBalanceKey())
		}
	}

	for _, op := range ops {
		if op == nil {
			continue
		}

		alias := op.GetAccountAlias()
		if alias == "" {
			continue
		}

		return s.router.ResolveBalance(alias, op.GetBalanceKey())
	}

	return 0
}

func (a *AuthorizerGRPCRepository) Close() error {
	if a == nil || a.conn == nil {
		return nil
	}

	return a.conn.Close()
}
