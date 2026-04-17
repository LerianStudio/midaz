// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package out

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	mgrpc "github.com/LerianStudio/midaz/v3/pkg/mgrpc"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
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

var errAuthorizeStreamNotImplemented = errors.New("AuthorizeStream not implemented in stub")

func (s *stubBalanceAuthorizerClient) AuthorizeStream(_ context.Context, _ ...grpc.CallOption) (grpc.BidiStreamingClient[authorizerv1.AuthorizeRequest, authorizerv1.AuthorizeResponse], error) {
	return nil, errAuthorizeStreamNotImplemented
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

func (s *stubBalanceAuthorizerClient) PrepareAuthorize(_ context.Context, _ *authorizerv1.AuthorizeRequest, _ ...grpc.CallOption) (*authorizerv1.PrepareAuthorizeResponse, error) {
	return &authorizerv1.PrepareAuthorizeResponse{}, nil
}

func (s *stubBalanceAuthorizerClient) CommitPrepared(_ context.Context, _ *authorizerv1.CommitPreparedRequest, _ ...grpc.CallOption) (*authorizerv1.CommitPreparedResponse, error) {
	return &authorizerv1.CommitPreparedResponse{}, nil
}

func (s *stubBalanceAuthorizerClient) AbortPrepared(_ context.Context, _ *authorizerv1.AbortPreparedRequest, _ ...grpc.CallOption) (*authorizerv1.AbortPreparedResponse, error) {
	return &authorizerv1.AbortPreparedResponse{}, nil
}

func (s *stubBalanceAuthorizerClient) ResolveManualIntervention(_ context.Context, _ *authorizerv1.ResolveManualInterventionRequest, _ ...grpc.CallOption) (*authorizerv1.ResolveManualInterventionResponse, error) {
	return &authorizerv1.ResolveManualInterventionResponse{}, nil
}

func TestParseShardRanges_AllowsNonOverlappingRanges(t *testing.T) {
	t.Parallel()

	ranges, err := parseShardRanges("0-3,4-7")
	require.NoError(t, err)
	require.Len(t, ranges, 2)
	assert.Equal(t, shardRange{Start: 0, End: 3}, ranges[0])
	assert.Equal(t, shardRange{Start: 4, End: 7}, ranges[1])
}

func TestParseShardRanges_RejectsOverlappingRanges(t *testing.T) {
	t.Parallel()

	_, err := parseShardRanges("0-5,4-7")
	require.Error(t, err)
	assert.ErrorContains(t, err, "overlap")
}

func TestParseShardRanges_RejectsReversedBounds(t *testing.T) {
	t.Parallel()

	_, err := parseShardRanges("5-3")
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid shard range")
}

func TestParseShardRanges_AllowsSingleElementRange(t *testing.T) {
	t.Parallel()

	ranges, err := parseShardRanges("3-3")
	require.NoError(t, err)
	require.Len(t, ranges, 1)
	assert.Equal(t, shardRange{Start: 3, End: 3}, ranges[0])
}

func TestShardedAuthorizerClientMethods_ReturnErrorWhenNoClientResolved(t *testing.T) {
	t.Parallel()

	repo := &ShardedAuthorizerGRPCRepository{
		enabled:       true,
		clients:       []*AuthorizerGRPCRepository{nil},
		clientByShard: map[int]*AuthorizerGRPCRepository{},
	}

	_, err := repo.Authorize(context.Background(), &authorizerv1.AuthorizeRequest{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAuthorizerShardClientMissing)

	_, err = repo.LoadBalances(context.Background(), &authorizerv1.LoadBalancesRequest{ShardIds: []int32{1}})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAuthorizerShardClientMissing)

	err = repo.PublishBalanceOperations(context.Background(), "topic", "1", []byte("payload"), nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAuthorizerShardClientMissing)
}

func TestParseCSV_TrimsAndSkipsEmptyValues(t *testing.T) {
	t.Parallel()

	parsed := parseCSV(" a, ,b , , c ")
	assert.Equal(t, []string{"a", "b", "c"}, parsed)
}

func TestResolvePrimaryShard_PrefersInternalDebit(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	router := shard.NewRouter(8)
	repo := &ShardedAuthorizerGRPCRepository{router: router}

	operations := []*authorizerv1.BalanceOperation{
		{AccountAlias: "@external/USD", Operation: "CREDIT", BalanceKey: "shard_0"},
	}

	expected := router.ResolveBalance("@external/USD", "shard_0")
	assert.Equal(t, expected, repo.resolvePrimaryShard(operations))
}

func TestShardedAuthorizerGRPCRepository_AuthorizeRoutesToMappedShard(t *testing.T) {
	t.Parallel()

	router := shard.NewRouter(8)
	shardID := router.ResolveBalance("@alice", "default")

	clientAStub := &stubBalanceAuthorizerClient{}
	clientBStub := &stubBalanceAuthorizerClient{}

	clientA := &AuthorizerGRPCRepository{enabled: true, timeout: 10 * time.Millisecond, clients: []authorizerv1.BalanceAuthorizerClient{clientAStub}}
	clientB := &AuthorizerGRPCRepository{enabled: true, timeout: 10 * time.Millisecond, clients: []authorizerv1.BalanceAuthorizerClient{clientBStub}}

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
	t.Parallel()

	client := &AuthorizerGRPCRepository{enabled: true, timeout: 10 * time.Millisecond, clients: []authorizerv1.BalanceAuthorizerClient{&stubBalanceAuthorizerClient{}}}

	repo := &ShardedAuthorizerGRPCRepository{
		enabled:       true,
		clients:       []*AuthorizerGRPCRepository{client},
		clientByShard: map[int]*AuthorizerGRPCRepository{},
		router:        shard.NewRouter(8),
	}

	err := repo.PublishBalanceOperations(context.Background(), "topic", "1", []byte("payload"), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthorizerShardClientMissing)
}

func TestNewAuthorizerGRPC_RejectsInsecureTransportInProduction(t *testing.T) {
	t.Parallel()

	logger, logErr := libZap.InitializeLoggerWithError()
	require.NoError(t, logErr)

	_, err := NewAuthorizerGRPC(AuthorizerConfig{
		Enabled:     true,
		Host:        "127.0.0.1",
		Port:        "50051",
		TLSEnabled:  false,
		Environment: "production",
	}, logger)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthorizerInsecureTransport)
}

// ---------------------------------------------------------------------------
// Peer-auth signing path tests
// ---------------------------------------------------------------------------.

func TestWithAuthorizerPeerAuth_NonEmptyToken(t *testing.T) {
	t.Parallel()

	token := "test-secret-token"
	method := authorizerRPCMethodAuthorize
	req := &authorizerv1.AuthorizeRequest{
		Operations: []*authorizerv1.BalanceOperation{
			{AccountAlias: "@alice", Operation: "DEBIT", BalanceKey: "default"},
		},
	}

	ctx, err := withAuthorizerPeerAuth(context.Background(), token, method, req)
	require.NoError(t, err)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok, "expected outgoing metadata on the context")

	expectedHeaders := []string{
		mgrpc.PeerAuthTimestampHeader,
		mgrpc.PeerAuthNonceHeader,
		mgrpc.PeerAuthMethodHeader,
		mgrpc.PeerAuthBodyHashHeader,
		mgrpc.PeerAuthSignatureHeader,
	}

	for _, header := range expectedHeaders {
		vals := md.Get(header)
		require.NotEmpty(t, vals, "header %q must be present in outgoing metadata", header)
		assert.NotEmpty(t, vals[0], "header %q must have a non-empty value", header)
	}

	// Verify the method header carries the exact method string we passed.
	assert.Equal(t, method, md.Get(mgrpc.PeerAuthMethodHeader)[0])
}

func TestWithAuthorizerPeerAuth_EmptyToken(t *testing.T) {
	t.Parallel()

	baseCtx := context.Background()
	req := &authorizerv1.AuthorizeRequest{}

	resultCtx, err := withAuthorizerPeerAuth(baseCtx, "", "some-method", req)
	require.NoError(t, err)

	// When the token is empty, the function must return the context
	// unchanged -- no outgoing metadata should be added.
	md, ok := metadata.FromOutgoingContext(resultCtx)
	if ok {
		// If metadata exists (e.g., was already on the context), none of the
		// peer-auth headers should be present.
		assert.Empty(t, md.Get(mgrpc.PeerAuthTimestampHeader), "no peer-auth headers expected for empty token")
		assert.Empty(t, md.Get(mgrpc.PeerAuthNonceHeader), "no peer-auth headers expected for empty token")
		assert.Empty(t, md.Get(mgrpc.PeerAuthMethodHeader), "no peer-auth headers expected for empty token")
		assert.Empty(t, md.Get(mgrpc.PeerAuthBodyHashHeader), "no peer-auth headers expected for empty token")
		assert.Empty(t, md.Get(mgrpc.PeerAuthSignatureHeader), "no peer-auth headers expected for empty token")
	}
}

func TestSignPeerAuth_Deterministic(t *testing.T) {
	t.Parallel()

	token := "shared-secret"
	timestamp := "1700000000"
	nonce := "abc123"
	method := authorizerRPCMethodAuthorize
	bodyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	sig1 := mgrpc.SignPeerAuth(token, timestamp, nonce, method, bodyHash)
	sig2 := mgrpc.SignPeerAuth(token, timestamp, nonce, method, bodyHash)

	require.NotEmpty(t, sig1, "signature must not be empty")
	assert.Equal(t, sig1, sig2, "identical inputs must produce identical HMAC signatures")

	// Sanity-check: the signature should be a valid hex string of the expected
	// length (SHA-256 HMAC = 32 bytes = 64 hex chars).
	decoded, err := hex.DecodeString(sig1)
	require.NoError(t, err, "signature must be valid hex")
	assert.Len(t, decoded, 32, "HMAC-SHA256 produces 32 bytes")
}

func TestHashPeerAuthBody_NilRequest(t *testing.T) {
	t.Parallel()

	// SHA-256 of an empty byte slice.
	expectedHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	result, err := mgrpc.HashPeerAuthBody(nil)
	require.NoError(t, err)
	assert.Equal(t, expectedHash, result, "nil request must produce the SHA-256 of empty bytes")
}

func TestHashPeerAuthBody_WithMessage(t *testing.T) {
	t.Parallel()

	msg := &authorizerv1.AuthorizeRequest{
		Operations: []*authorizerv1.BalanceOperation{
			{AccountAlias: "@alice", Operation: "DEBIT", BalanceKey: "default", Amount: 1000},
		},
	}

	result, hashErr := mgrpc.HashPeerAuthBody(msg)
	require.NoError(t, hashErr)
	require.NotEmpty(t, result, "hash of a populated message must not be empty")

	// Verify the result is valid hex.
	decoded, err := hex.DecodeString(result)
	require.NoError(t, err, "hash must be a valid hex string")
	assert.Len(t, decoded, 32, "SHA-256 digest is 32 bytes")

	// The hash of a populated message must differ from the empty-body hash.
	emptyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	assert.NotEqual(t, emptyHash, result, "non-nil message must produce a different hash than nil")
}

func TestPickRoundRobinDistribution(t *testing.T) {
	t.Parallel()

	stub0 := &stubBalanceAuthorizerClient{}
	stub1 := &stubBalanceAuthorizerClient{}
	stub2 := &stubBalanceAuthorizerClient{}

	repo := &AuthorizerGRPCRepository{
		enabled: true,
		clients: []authorizerv1.BalanceAuthorizerClient{stub0, stub1, stub2},
	}

	// Call pick() 6 times and collect the sequence of returned clients.
	sequence := make([]authorizerv1.BalanceAuthorizerClient, 6)
	for i := 0; i < 6; i++ {
		sequence[i] = repo.pick()
	}

	// Assert round-robin order: 0, 1, 2, 0, 1, 2.
	expected := []authorizerv1.BalanceAuthorizerClient{stub0, stub1, stub2, stub0, stub1, stub2}
	for i, want := range expected {
		assert.Equal(t, want, sequence[i], "pick() call %d returned wrong client", i)
	}
}

func TestGeneratePeerNonce(t *testing.T) {
	t.Parallel()

	nonce1, err1 := mgrpc.GeneratePeerNonce()
	require.NoError(t, err1)
	require.NotEmpty(t, nonce1, "nonce must not be empty")

	nonce2, err2 := mgrpc.GeneratePeerNonce()
	require.NoError(t, err2)
	require.NotEmpty(t, nonce2, "nonce must not be empty")

	assert.NotEqual(t, nonce1, nonce2, "two consecutive nonces must differ (cryptographic randomness)")

	// Verify the nonce is valid hex (16 random bytes = 32 hex chars).
	decoded, err := hex.DecodeString(nonce1)
	require.NoError(t, err, "nonce must be valid hex")
	assert.Len(t, decoded, 16, "nonce is derived from 16 random bytes")
}
