// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// seamRPCTimeout is the RPC timeout the token-timeout tests send under: the
	// TRACER_TIMEOUT_MS default, roomy enough for a bufconn call under -race.
	seamRPCTimeout = DefaultOperationTimeout
	// seamShortWait is a token wait no mint in these tests finishes within.
	seamShortWait = 20 * time.Millisecond
)

// deadlineTokenSource serves one token and records whether the context it was
// asked under carried a deadline.
type deadlineTokenSource struct {
	mu          sync.Mutex
	hasDeadline []bool
}

func (s *deadlineTokenSource) Token(ctx context.Context) (string, error) {
	_, ok := ctx.Deadline()

	s.mu.Lock()
	s.hasDeadline = append(s.hasDeadline, ok)
	s.mu.Unlock()

	return "tok", nil
}

func (s *deadlineTokenSource) Invalidate(context.Context, string) bool { return true }

func (s *deadlineTokenSource) deadlines() []bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]bool(nil), s.hasDeadline...)
}

// seamOperations runs each of the client's five RPCs, so every test covers the
// reserve, the by-id transitions and the by-transaction transitions alike.
var seamOperations = []struct {
	name string
	call func(ctx context.Context, c *TracerGRPCClient) error
}{
	{"reserve", func(ctx context.Context, c *TracerGRPCClient) error {
		_, err := c.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
		return err
	}},
	{"confirm", func(ctx context.Context, c *TracerGRPCClient) error {
		_, err := c.Confirm(ctx, fixedReservationID)
		return err
	}},
	{"release", func(ctx context.Context, c *TracerGRPCClient) error {
		return c.Release(ctx, fixedReservationID)
	}},
	{"confirm by transaction", func(ctx context.Context, c *TracerGRPCClient) error {
		_, err := c.ConfirmByTransaction(ctx, fixedTransactionID)
		return err
	}},
	{"release by transaction", func(ctx context.Context, c *TracerGRPCClient) error {
		return c.ReleaseByTransaction(ctx, fixedTransactionID)
	}},
}

func TestTracerGRPCClient_TokenTimeout_TokenWaitIsNotBoundByTheRPCTimeout(t *testing.T) {
	t.Parallel()

	for _, op := range seamOperations {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()

			src := &deadlineTokenSource{}
			recorder := &seamRecorder{}
			client := newSeamClient(t, recorder, WithM2MCredentials(src), WithGRPCOperationTimeout(seamRPCTimeout))

			require.NoError(t, op.call(context.Background(), client))

			assert.Equal(t, []bool{false}, src.deadlines(),
				"the token is resolved before the RPC timeout starts, so the token wait never inherits it")

			calls := recorder.snapshot()
			require.Len(t, calls, 1)
			assert.Positive(t, calls[0].remaining, "the RPC itself is still bounded")
			assert.LessOrEqual(t, calls[0].remaining, seamRPCTimeout, "by the operation timeout")
		})
	}
}

func TestTracerGRPCClient_TokenTimeout_MintSlowerThanTheRPCTimeoutIsSent(t *testing.T) {
	t.Parallel()

	for _, op := range seamOperations {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()

			clock := newTestClock()
			minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
			minter.arm()
			src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock, WithTokenWaitTimeout(MaxTokenWaitTimeout))

			recorder := &seamRecorder{}
			client := newSeamClient(t, recorder, WithM2MCredentials(src), WithGRPCOperationTimeout(seamRPCTimeout))

			done := make(chan error, 1)

			go func() { done <- op.call(context.Background(), client) }()

			<-minter.entered
			// The mint outlasts the RPC timeout several times over before it
			// answers, as a cold Access Manager mint does.
			<-time.After(3 * seamRPCTimeout)
			close(minter.gate)

			require.NoError(t, <-done, "a mint within the token timeout does not cost the call")

			calls := recorder.snapshot()
			require.Len(t, calls, 1)
			assert.Positive(t, calls[0].remaining)
			assert.LessOrEqual(t, calls[0].remaining, seamRPCTimeout,
				"the RPC timeout starts once the token is in hand")
		})
	}
}

func TestTracerGRPCClient_TokenTimeout_MintSlowerThanTheTokenTimeoutIsNotSent(t *testing.T) {
	t.Parallel()

	for _, op := range seamOperations {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()

			clock := newTestClock()
			minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
			minter.arm()
			src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock, WithTokenWaitTimeout(seamShortWait))

			t.Cleanup(func() { close(minter.gate) })

			recorder := &seamRecorder{}
			client := newSeamClient(t, recorder, WithM2MCredentials(src), WithGRPCOperationTimeout(time.Hour))

			done := make(chan error, 1)

			go func() { done <- op.call(context.Background(), client) }()

			var err error
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the token wait must end at the token timeout, not at the operation timeout")
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrTracerUnavailable)
			assert.ErrorIs(t, err, errTokenWaitAbandoned)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.NotErrorIs(t, err, ErrTracerNoAnswer, "a call that never left is never an unanswered one")
			assert.NotErrorIs(t, err, ErrTracerCredentialUnavailable)
			assert.Empty(t, recorder.snapshot(), "nothing was sent")
		})
	}
}

func TestTracerGRPCClient_TokenTimeout_CallerDeadlineStillBoundsTheTokenWait(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	minter.arm()
	src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock, WithTokenWaitTimeout(MaxTokenWaitTimeout))

	t.Cleanup(func() { close(minter.gate) })

	recorder := &seamRecorder{}
	client := newSeamClient(t, recorder, WithM2MCredentials(src), WithGRPCOperationTimeout(time.Hour))

	ctx, cancel := context.WithTimeout(context.Background(), seamShortWait)
	defer cancel()

	_, err := client.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
	require.Error(t, err)
	assert.ErrorIs(t, err, errTokenWaitAbandoned)
	assert.NotErrorIs(t, err, ErrTracerNoAnswer)
	assert.Empty(t, recorder.snapshot(), "nothing was sent")
}

func TestTracerGRPCClient_CallTimeout_TightensOnlyTheRPC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		operation   time.Duration
		callTimeout time.Duration
		wantAtMost  time.Duration
	}{
		{"a shorter call timeout tightens the RPC", time.Hour, seamRPCTimeout, seamRPCTimeout},
		{"the operation timeout stays the ceiling", seamRPCTimeout, time.Hour, seamRPCTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := &deadlineTokenSource{}
			recorder := &seamRecorder{}
			client := newSeamClient(t, recorder, WithM2MCredentials(src), WithGRPCOperationTimeout(tt.operation))

			ctx := ContextWithCallTimeout(context.Background(), tt.callTimeout)

			_, err := client.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
			require.NoError(t, err)

			assert.Equal(t, []bool{false}, src.deadlines(), "the call timeout never bounds the token wait")

			calls := recorder.snapshot()
			require.Len(t, calls, 1)
			assert.Positive(t, calls[0].remaining)
			assert.LessOrEqual(t, calls[0].remaining, tt.wantAtMost)
		})
	}
}

func TestContextWithCallTimeout(t *testing.T) {
	t.Parallel()

	_, ok := CallTimeout(context.Background())
	assert.False(t, ok, "no call timeout unless one was set")

	got, ok := CallTimeout(ContextWithCallTimeout(context.Background(), 250*time.Millisecond))
	assert.True(t, ok)
	assert.Equal(t, 250*time.Millisecond, got)

	_, ok = CallTimeout(ContextWithCallTimeout(context.Background(), 0))
	assert.False(t, ok, "a non-positive call timeout sets none")
}

func TestM2MTokenSource_TokenWaitTimeout(t *testing.T) {
	t.Parallel()

	t.Run("a mint slower than the wait timeout is abandoned", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		minter.arm()
		src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock, WithTokenWaitTimeout(seamShortWait))

		t.Cleanup(func() { close(minter.gate) })

		_, err := src.Token(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTracerUnavailable)
		assert.ErrorIs(t, err, errTokenWaitAbandoned)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("an abandoned mint still lands in the cache for the next call", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		minter.arm()
		src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock, WithTokenWaitTimeout(seamShortWait))

		_, err := src.Token(context.Background())
		require.ErrorIs(t, err, errTokenWaitAbandoned)

		close(minter.gate)

		require.Eventually(t, func() bool {
			_, ok := src.fresh("")
			return ok
		}, 5*time.Second, time.Millisecond)

		token, err := src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ledger-1", subjectOf(t, token))
		assert.Equal(t, 1, minter.totalMints())
	})

	t.Run("a cached token is served whatever the wait timeout", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock, WithTokenWaitTimeout(time.Millisecond))

		// Prime the cache with a mint no caller waits for.
		first, err := src.mint(context.Background(), "")
		require.NoError(t, err)

		second, err := src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, first, second)
		assert.Equal(t, 1, minter.totalMints())
	})

	t.Run("the default wait timeout is the documented one", func(t *testing.T) {
		t.Parallel()

		src, err := NewM2MTokenSource(&fakeMinter{}, NewStaticCredentials("ledger", "s3cret"))
		require.NoError(t, err)
		assert.Equal(t, DefaultTokenWaitTimeout, src.waitTimeout)
		assert.LessOrEqual(t, DefaultTokenWaitTimeout, MaxTokenWaitTimeout)
	})

	t.Run("a non-positive wait timeout keeps the default", func(t *testing.T) {
		t.Parallel()

		src, err := NewM2MTokenSource(&fakeMinter{}, NewStaticCredentials("ledger", "s3cret"), WithTokenWaitTimeout(0))
		require.NoError(t, err)
		assert.Equal(t, DefaultTokenWaitTimeout, src.waitTimeout)
	})

	t.Run("a wait timeout above the mint timeout is refused", func(t *testing.T) {
		t.Parallel()

		_, err := NewM2MTokenSource(&fakeMinter{}, NewStaticCredentials("ledger", "s3cret"), WithTokenWaitTimeout(MaxTokenWaitTimeout+time.Millisecond))
		require.Error(t, err)
	})
}
