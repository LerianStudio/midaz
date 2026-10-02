// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// seamCall is one RPC the bufconn tracer received, with its incoming metadata.
type seamCall struct {
	method string
	md     metadata.MD
}

// seamRecorder is a server interceptor that records every call and answers it
// with the scripted status for that attempt (nil lets the handler run).
type seamRecorder struct {
	mu     sync.Mutex
	calls  []seamCall
	answer func(attempt int, md metadata.MD) error
}

func (r *seamRecorder) intercept(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)

	r.mu.Lock()
	r.calls = append(r.calls, seamCall{method: info.FullMethod, md: md.Copy()})
	attempt := len(r.calls)
	answer := r.answer
	r.mu.Unlock()

	if answer != nil {
		if err := answer(attempt, md); err != nil {
			return nil, err
		}
	}

	return handler(ctx, req)
}

func (r *seamRecorder) snapshot() []seamCall {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]seamCall(nil), r.calls...)
}

// okReservationServer answers every RPC successfully.
func okReservationServer() *stubReservationServer {
	return &stubReservationServer{
		reserveFn: func(req *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
			return &reservationv1.ReserveResult{TransactionId: req.GetTransactionId()}, nil
		},
		confirmByTransactionFn: func(*reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
			return &reservationv1.ConfirmByTransactionResponse{Confirmed: 1}, nil
		},
		releaseByTransactionFn: func(*reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
			return &reservationv1.ReleaseByTransactionResponse{}, nil
		},
		confirmByIDFn: func(*reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
			return &reservationv1.ConfirmByIdResponse{}, nil
		},
		releaseByIDFn: func(*reservationv1.ReleaseByIdRequest) (*reservationv1.ReleaseByIdResponse, error) {
			return &reservationv1.ReleaseByIdResponse{}, nil
		},
	}
}

// newSeamClient builds a client through NewTracerGRPCClient (so the production
// interceptor chain is exercised) dialed to a bufconn tracer.
func newSeamClient(t *testing.T, recorder *seamRecorder, opts ...TracerGRPCClientOption) *TracerGRPCClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)

	srv := grpc.NewServer(grpc.UnaryInterceptor(recorder.intercept))
	reservationv1.RegisterReservationServiceServer(srv, okReservationServer())

	go func() { _ = srv.Serve(lis) }()

	all := append([]TracerGRPCClientOption{
		WithGRPCOperationTimeout(5 * time.Second),
		WithGRPCDialOptions(
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	}, opts...)

	client, err := NewTracerGRPCClient("passthrough:///bufnet", all...)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = client.Close()
		srv.Stop()
		_ = lis.Close()
	})

	return client
}

// scriptedTokenSource hands out the scripted tokens in order and records the
// tokens it was asked to invalidate. After an invalidation it answers
// errAfterInvalidate when that is set.
type scriptedTokenSource struct {
	mu                 sync.Mutex
	tokens             []string
	err                error
	errAfterInvalidate error
	served             int
	invalidated        []string
}

func (s *scriptedTokenSource) Token(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return "", s.err
	}

	if s.errAfterInvalidate != nil && len(s.invalidated) > 0 {
		return "", s.errAfterInvalidate
	}

	token := s.tokens[min(s.served, len(s.tokens)-1)]
	s.served++

	return token, nil
}

func (s *scriptedTokenSource) Invalidate(_ context.Context, rejected string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.invalidated = append(s.invalidated, rejected)

	return true
}

func (s *scriptedTokenSource) invalidatedTokens() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.invalidated...)
}

func TestTracerGRPCClient_SeamMetadataKeysAreLiterals(t *testing.T) {
	t.Parallel()

	t.Run("token identity", func(t *testing.T) {
		t.Parallel()

		recorder := &seamRecorder{}
		client := newSeamClient(t, recorder, WithM2MCredentials(&scriptedTokenSource{tokens: []string{"tok-1"}}))

		_, err := client.Reserve(tmcore.ContextWithTenantID(context.Background(), "tenant-007"), ReserveRequest{TransactionID: fixedTransactionID})
		require.NoError(t, err)

		calls := recorder.snapshot()
		require.Len(t, calls, 1)
		assert.Equal(t, []string{"Bearer tok-1"}, calls[0].md.Get("authorization"))
		assert.Empty(t, calls[0].md.Get("x-api-key"))
		assert.Equal(t, []string{"tenant-007"}, calls[0].md.Get("x-tenant-id"))
	})

	t.Run("api key identity", func(t *testing.T) {
		t.Parallel()

		recorder := &seamRecorder{}
		client := newSeamClient(t, recorder, WithAPIKey("k3y"))

		_, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
		require.NoError(t, err)

		calls := recorder.snapshot()
		require.Len(t, calls, 1)
		assert.Equal(t, []string{"k3y"}, calls[0].md.Get("x-api-key"))
		assert.Empty(t, calls[0].md.Get("authorization"))
	})
}

func TestTracerGRPCClient_M2MCredentials_AttachesBearerOnEveryRPC(t *testing.T) {
	t.Parallel()

	recorder := &seamRecorder{}
	client := newSeamClient(t, recorder, WithM2MCredentials(&scriptedTokenSource{tokens: []string{"tok-1"}}))

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-007")

	_, err := client.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
	require.NoError(t, err)

	_, err = client.ConfirmByTransaction(ctx, fixedTransactionID)
	require.NoError(t, err)

	require.NoError(t, client.ReleaseByTransaction(ctx, fixedTransactionID))

	_, err = client.Confirm(ctx, fixedReservationID)
	require.NoError(t, err)

	require.NoError(t, client.Release(ctx, fixedReservationID))

	calls := recorder.snapshot()
	require.Len(t, calls, 5)

	for _, call := range calls {
		assert.Equal(t, []string{"Bearer tok-1"}, call.md.Get(authorizationMetadataKey), call.method)
		assert.Empty(t, call.md.Get(apiKeyMetadataKey), "the API key never travels with a token: %s", call.method)
		assert.Equal(t, []string{"tenant-007"}, call.md.Get(tenantMetadataKey), "x-tenant-id still travels: %s", call.method)
	}
}

func TestTracerGRPCClient_M2MCredentials_UnauthenticatedRefreshesAndRetriesOnce(t *testing.T) {
	t.Parallel()

	t.Run("a rejected token is replaced and the call succeeds on the retry", func(t *testing.T) {
		t.Parallel()

		recorder := &seamRecorder{answer: func(attempt int, _ metadata.MD) error {
			if attempt == 1 {
				return status.Error(codes.Unauthenticated, "token expired")
			}

			return nil
		}}
		src := &scriptedTokenSource{tokens: []string{"stale", "fresh"}}
		client := newSeamClient(t, recorder, WithM2MCredentials(src))

		_, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
		require.NoError(t, err)

		calls := recorder.snapshot()
		require.Len(t, calls, 2)
		assert.Equal(t, []string{"Bearer stale"}, calls[0].md.Get(authorizationMetadataKey))
		assert.Equal(t, []string{"Bearer fresh"}, calls[1].md.Get(authorizationMetadataKey))
		assert.Equal(t, []string{"stale"}, src.invalidatedTokens(), "only the rejected token is invalidated")
	})

	t.Run("a second rejection is returned as unauthorized, without a third attempt", func(t *testing.T) {
		t.Parallel()

		recorder := &seamRecorder{answer: func(int, metadata.MD) error {
			return status.Error(codes.Unauthenticated, "bad client")
		}}
		src := &scriptedTokenSource{tokens: []string{"stale", "fresh"}}
		client := newSeamClient(t, recorder, WithM2MCredentials(src))

		_, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTracerUnauthorized)
		assert.NotErrorIs(t, err, ErrTracerUnavailable)
		assert.NotErrorIs(t, err, ErrTracerNoAnswer)
		assert.Len(t, recorder.snapshot(), 2)
	})

	t.Run("permission denied is not retried", func(t *testing.T) {
		t.Parallel()

		recorder := &seamRecorder{answer: func(int, metadata.MD) error {
			return status.Error(codes.PermissionDenied, "not granted")
		}}
		src := &scriptedTokenSource{tokens: []string{"tok"}}
		client := newSeamClient(t, recorder, WithM2MCredentials(src))

		_, err := client.ConfirmByTransaction(context.Background(), fixedTransactionID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTracerUnauthorized)
		assert.Len(t, recorder.snapshot(), 1)
		assert.Empty(t, src.invalidatedTokens())
	})

	t.Run("a retry whose token cannot be obtained returns the original rejection", func(t *testing.T) {
		t.Parallel()

		recorder := &seamRecorder{answer: func(int, metadata.MD) error {
			return status.Error(codes.Unauthenticated, "token revoked")
		}}
		src := &scriptedTokenSource{
			tokens:             []string{"stale"},
			errAfterInvalidate: credentialNotSent(errors.New("access manager unreachable")),
		}
		client := newSeamClient(t, recorder, WithM2MCredentials(src))

		_, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTracerUnauthorized, "the rejection the tracer answered is what the caller sees")
		assert.NotErrorIs(t, err, ErrTracerUnavailable)
		assert.NotErrorIs(t, err, ErrTracerCredentialUnavailable)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Len(t, recorder.snapshot(), 1, "no retry is sent without a token")
	})
}

func TestTracerGRPCClient_M2MCredentials_RejectionKeepsTheCredential(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	fetches := &fetchRecorder{}
	creds, err := NewTenantCredentials(fetches.fetch, WithCredentialClock(clock.Now))
	require.NoError(t, err)

	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	src := newTestTokenSource(t, minter, creds, clock)

	recorder := &seamRecorder{answer: func(attempt int, _ metadata.MD) error {
		if attempt == 1 {
			return status.Error(codes.Unauthenticated, "token revoked")
		}

		return nil
	}}
	client := newSeamClient(t, recorder, WithM2MCredentials(src))
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	_, err = src.Token(ctx)
	require.NoError(t, err)

	clock.Advance(rejectedTokenMinAge)

	_, err = client.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
	require.NoError(t, err)

	assert.Equal(t, 2, minter.totalMints(), "the rejected token is replaced")
	assert.Equal(t, 1, fetches.count(), "a rejected token never re-reads the credential")
}

func TestTracerGRPCClient_M2MCredentials_ConcurrentRejectionsMintOnce(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

	rejected, err := src.Token(context.Background())
	require.NoError(t, err)

	clock.Advance(rejectedTokenMinAge)

	recorder := &seamRecorder{answer: func(_ int, md metadata.MD) error {
		if slices.Contains(md.Get(authorizationMetadataKey), "Bearer "+rejected) {
			return status.Error(codes.Unauthenticated, "token revoked")
		}

		return nil
	}}
	client := newSeamClient(t, recorder, WithM2MCredentials(src))

	const callers = 8

	start := make(chan struct{})
	errs := make([]error, callers)

	var wg sync.WaitGroup

	for i := range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			errs[i] = client.ReleaseByTransaction(context.Background(), fixedTransactionID)
		}()
	}

	close(start)
	wg.Wait()

	for _, callErr := range errs {
		require.NoError(t, callErr)
	}

	assert.Equal(t, 2, minter.totalMints(), "a late rejection of the old token never deletes its replacement")
}

func TestTracerGRPCClient_M2MCredentials_NothingSentWithoutAToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  *scriptedTokenSource
	}{
		{name: "mint failure", src: &scriptedTokenSource{err: credentialNotSent(errors.New("access manager unreachable"))}},
		{name: "empty token", src: &scriptedTokenSource{tokens: []string{""}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := &seamRecorder{}
			client := newSeamClient(t, recorder, WithM2MCredentials(tc.src))

			_, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrTracerUnavailable)
			assert.NotErrorIs(t, err, ErrTracerNoAnswer, "nothing was sent, so nothing can be held")
			assert.Empty(t, recorder.snapshot(), "no call reaches the tracer without a token")
		})
	}
}

func TestTracerGRPCClient_APIKey(t *testing.T) {
	t.Parallel()

	t.Run("the key travels on every RPC and no authorization header does", func(t *testing.T) {
		t.Parallel()

		recorder := &seamRecorder{}
		client := newSeamClient(t, recorder, WithAPIKey("k3y"))

		_, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
		require.NoError(t, err)

		require.NoError(t, client.ReleaseByTransaction(context.Background(), fixedTransactionID))

		calls := recorder.snapshot()
		require.Len(t, calls, 2)

		for _, call := range calls {
			assert.Equal(t, []string{"k3y"}, call.md.Get(apiKeyMetadataKey), call.method)
			assert.Empty(t, call.md.Get(authorizationMetadataKey), call.method)
		}
	})

	t.Run("an unauthenticated answer is not retried", func(t *testing.T) {
		t.Parallel()

		recorder := &seamRecorder{answer: func(int, metadata.MD) error {
			return status.Error(codes.Unauthenticated, "invalid api key")
		}}
		client := newSeamClient(t, recorder, WithAPIKey("wrong"))

		_, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTracerUnauthorized)
		assert.Len(t, recorder.snapshot(), 1)
	})
}

func TestTracerGRPCClient_NoIdentitySendsNoCredential(t *testing.T) {
	t.Parallel()

	recorder := &seamRecorder{}
	client := newSeamClient(t, recorder)

	_, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
	require.NoError(t, err)

	calls := recorder.snapshot()
	require.Len(t, calls, 1)
	assert.Empty(t, calls[0].md.Get(authorizationMetadataKey))
	assert.Empty(t, calls[0].md.Get(apiKeyMetadataKey))
}

func TestNewTracerGRPCClient_RefusesTwoIdentities(t *testing.T) {
	t.Parallel()

	client, err := NewTracerGRPCClient("passthrough:///tracer:4021",
		WithM2MCredentials(&scriptedTokenSource{tokens: []string{"tok"}}),
		WithAPIKey("k3y"))

	require.Error(t, err)
	assert.Nil(t, client)
}

func TestTracerGRPCClient_M2MCredentials_YoungRejectedTokenIsNotReminted(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

	recorder := &seamRecorder{answer: func(int, metadata.MD) error {
		return status.Error(codes.Unauthenticated, "credential not granted")
	}}
	client := newSeamClient(t, recorder, WithM2MCredentials(src))

	for range 2 {
		err := client.ReleaseByTransaction(context.Background(), fixedTransactionID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTracerUnauthorized)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	}

	assert.Equal(t, 1, minter.totalMints(), "a token rejected moments after its mint is not re-minted")
	assert.Len(t, recorder.snapshot(), 2, "the rejection of a young token is not retried")

	clock.Advance(rejectedTokenMinAge)

	err := client.ReleaseByTransaction(context.Background(), fixedTransactionID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTracerUnauthorized)
	assert.Equal(t, 2, minter.totalMints(), "once the token is old enough a rejection re-mints it")
	assert.Len(t, recorder.snapshot(), 4, "and the call is retried once with the new token")
}

func TestTracerGRPCClient_M2MCredentials_AbandonedTokenWaitIsPlainUnavailable(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	minter.arm()
	src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

	t.Cleanup(func() { close(minter.gate) })

	recorder := &seamRecorder{}
	client := newSeamClient(t, recorder, WithM2MCredentials(src), WithGRPCOperationTimeout(20*time.Millisecond))

	_, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTracerUnavailable)
	assert.NotErrorIs(t, err, ErrTracerCredentialUnavailable)
	assert.NotErrorIs(t, err, ErrTracerNoAnswer)
	assert.Empty(t, recorder.snapshot(), "nothing was sent")
}

// closingTokenSource is a scriptedTokenSource that records being closed.
type closingTokenSource struct {
	*scriptedTokenSource
	closed atomic.Bool
}

func (c *closingTokenSource) Close() error {
	c.closed.Store(true)

	return nil
}

func TestTracerGRPCClient_CloseClosesTheTokenSource(t *testing.T) {
	t.Parallel()

	src := &closingTokenSource{scriptedTokenSource: &scriptedTokenSource{tokens: []string{"tok"}}}

	client, err := NewTracerGRPCClient("passthrough:///tracer-unused", WithM2MCredentials(src))
	require.NoError(t, err)

	require.NoError(t, client.Close())
	assert.True(t, src.closed.Load(), "the token source's scheduled refreshes stop with the client")
}
