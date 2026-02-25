// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package out

import (
	"context"
	"testing"
	"time"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type stubBalanceAuthorizerClient struct {
	authorizeCalls int
	loadCalls      int
	publishCalls   int

	authorizeResp *authorizerv1.AuthorizeResponse
	loadResp      *authorizerv1.LoadBalancesResponse
	publishErr    error
}

func (s *stubBalanceAuthorizerClient) Authorize(_ context.Context, _ *authorizerv1.AuthorizeRequest, _ ...grpc.CallOption) (*authorizerv1.AuthorizeResponse, error) {
	s.authorizeCalls++
	if s.authorizeResp != nil {
		return s.authorizeResp, nil
	}

	return &authorizerv1.AuthorizeResponse{}, nil
}

func (s *stubBalanceAuthorizerClient) AuthorizeStream(_ context.Context, _ ...grpc.CallOption) (grpc.BidiStreamingClient[authorizerv1.AuthorizeRequest, authorizerv1.AuthorizeResponse], error) {
	return nil, nil
}

func (s *stubBalanceAuthorizerClient) LoadBalances(_ context.Context, _ *authorizerv1.LoadBalancesRequest, _ ...grpc.CallOption) (*authorizerv1.LoadBalancesResponse, error) {
	s.loadCalls++
	if s.loadResp != nil {
		return s.loadResp, nil
	}

	return &authorizerv1.LoadBalancesResponse{}, nil
}

func (s *stubBalanceAuthorizerClient) GetBalance(_ context.Context, _ *authorizerv1.GetBalanceRequest, _ ...grpc.CallOption) (*authorizerv1.GetBalanceResponse, error) {
	return &authorizerv1.GetBalanceResponse{}, nil
}

func (s *stubBalanceAuthorizerClient) PublishBalanceOperations(_ context.Context, _ *authorizerv1.PublishBalanceOperationsRequest, _ ...grpc.CallOption) (*authorizerv1.PublishBalanceOperationsResponse, error) {
	s.publishCalls++
	if s.publishErr != nil {
		return nil, s.publishErr
	}

	return &authorizerv1.PublishBalanceOperationsResponse{}, nil
}

func TestParseShardRanges_AllowsNonOverlappingRanges(t *testing.T) {
	ranges, err := parseShardRanges("0-3,4-7")
	require.NoError(t, err)
	require.Len(t, ranges, 2)
	assert.Equal(t, shardRange{start: 0, end: 3}, ranges[0])
	assert.Equal(t, shardRange{start: 4, end: 7}, ranges[1])
}

func TestParseShardRanges_RejectsOverlappingRanges(t *testing.T) {
	_, err := parseShardRanges("0-5,4-7")
	require.Error(t, err)
	assert.ErrorContains(t, err, "overlap")
}

func TestParseShardRanges_RejectsReversedBounds(t *testing.T) {
	_, err := parseShardRanges("5-3")
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid shard range")
}

func TestParseShardRanges_AllowsSingleElementRange(t *testing.T) {
	ranges, err := parseShardRanges("3-3")
	require.NoError(t, err)
	require.Len(t, ranges, 1)
	assert.Equal(t, shardRange{start: 3, end: 3}, ranges[0])
}

func TestShardedAuthorizerClientMethods_ReturnErrorWhenNoClientResolved(t *testing.T) {
	repo := &ShardedAuthorizerGRPCRepository{
		enabled:       true,
		clients:       []*AuthorizerGRPCRepository{nil},
		clientByShard: map[int]*AuthorizerGRPCRepository{},
	}

	_, err := repo.Authorize(context.Background(), &authorizerv1.AuthorizeRequest{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "not configured")

	_, err = repo.LoadBalances(context.Background(), &authorizerv1.LoadBalancesRequest{ShardIds: []int32{1}})
	require.Error(t, err)
	assert.ErrorContains(t, err, "not configured")

	err = repo.PublishBalanceOperations(context.Background(), "topic", "1", []byte("payload"), nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "not configured")
}

func TestParseCSV_TrimsAndSkipsEmptyValues(t *testing.T) {
	parsed := parseCSV(" a, ,b , , c ")
	assert.Equal(t, []string{"a", "b", "c"}, parsed)
}

func TestResolvePrimaryShard_PrefersInternalDebit(t *testing.T) {
	router := shard.NewRouter(8)
	repo := &ShardedAuthorizerGRPCRepository{router: router}

	operations := []*authorizerv1.BalanceOperation{
		{AccountAlias: "@external/USD", Operation: "DEBIT", BalanceKey: "shard_1"},
		{AccountAlias: "@alice", Operation: "DEBIT", BalanceKey: "default"},
		{AccountAlias: "@bob", Operation: "CREDIT", BalanceKey: "default"},
	}

	expected := router.ResolveBalance("@alice", "default")
	assert.Equal(t, expected, repo.resolvePrimaryShard(operations))
}

func TestResolvePrimaryShard_FallsBackToFirstAlias(t *testing.T) {
	router := shard.NewRouter(8)
	repo := &ShardedAuthorizerGRPCRepository{router: router}

	operations := []*authorizerv1.BalanceOperation{
		{AccountAlias: "@external/USD", Operation: "CREDIT", BalanceKey: "shard_0"},
	}

	expected := router.ResolveBalance("@external/USD", "shard_0")
	assert.Equal(t, expected, repo.resolvePrimaryShard(operations))
}

func TestShardedAuthorizerGRPCRepository_AuthorizeRoutesToMappedShard(t *testing.T) {
	router := shard.NewRouter(8)
	shardID := router.ResolveBalance("@alice", "default")

	clientAStub := &stubBalanceAuthorizerClient{}
	clientBStub := &stubBalanceAuthorizerClient{}

	clientA := &AuthorizerGRPCRepository{enabled: true, timeout: 10 * time.Millisecond, client: clientAStub}
	clientB := &AuthorizerGRPCRepository{enabled: true, timeout: 10 * time.Millisecond, client: clientBStub}

	repo := &ShardedAuthorizerGRPCRepository{
		enabled: true,
		clients: []*AuthorizerGRPCRepository{clientA, clientB},
		clientByShard: map[int]*AuthorizerGRPCRepository{
			shardID: clientB,
		},
		router: router,
	}

	_, err := repo.Authorize(context.Background(), &authorizerv1.AuthorizeRequest{
		Operations: []*authorizerv1.BalanceOperation{{AccountAlias: "@alice", Operation: "DEBIT", BalanceKey: "default"}},
	})

	require.NoError(t, err)
	assert.Equal(t, 0, clientAStub.authorizeCalls)
	assert.Equal(t, 1, clientBStub.authorizeCalls)
}

func TestShardedAuthorizerGRPCRepository_DoesNotFallbackWhenShardUnmapped(t *testing.T) {
	client := &AuthorizerGRPCRepository{enabled: true, timeout: 10 * time.Millisecond, client: &stubBalanceAuthorizerClient{}}

	repo := &ShardedAuthorizerGRPCRepository{
		enabled:       true,
		clients:       []*AuthorizerGRPCRepository{client},
		clientByShard: map[int]*AuthorizerGRPCRepository{},
		router:        shard.NewRouter(8),
	}

	err := repo.PublishBalanceOperations(context.Background(), "topic", "1", []byte("payload"), nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "not configured for shard 1")
}

func TestNewAuthorizerGRPC_RejectsInsecureTransportInProduction(t *testing.T) {
	_, err := NewAuthorizerGRPC(AuthorizerConfig{
		Enabled:     true,
		Host:        "127.0.0.1",
		Port:        "50051",
		TLSEnabled:  false,
		Environment: "production",
	}, libZap.InitializeLogger())

	require.Error(t, err)
	assert.ErrorContains(t, err, "insecure transport is not allowed")
}
