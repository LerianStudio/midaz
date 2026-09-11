// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// =============================================================================
// isResponseLostError — "could the server have applied and the reply be lost?"
// =============================================================================

// timeoutNetError is the shape go-redis surfaces on a socket read timeout: a
// net.Error whose Timeout() is true.
type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "read tcp 127.0.0.1:6379: i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

// nonTimeoutNetError is a net.Error that is NOT a timeout: a refused dial never
// reached the server, so it must not be treated as a lost response.
type nonTimeoutNetError struct{}

func (nonTimeoutNetError) Error() string   { return "dial tcp 127.0.0.1:6379: connection refused" }
func (nonTimeoutNetError) Timeout() bool   { return false }
func (nonTimeoutNetError) Temporary() bool { return false }

func TestIsResponseLostError(t *testing.T) {
	t.Parallel()

	businessErr := pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance")

	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "business error 0018", err: businessErr, want: false},
		{name: "wrapped business error", err: fmt.Errorf("script: %w", businessErr), want: false},
		{name: "redis nil", err: redis.Nil, want: false},
		{name: "wrapped redis nil", err: fmt.Errorf("get: %w", redis.Nil), want: false},
		{name: "context canceled", err: context.Canceled, want: false},
		{name: "wrapped context canceled", err: fmt.Errorf("run: %w", context.Canceled), want: false},
		{name: "generic error", err: errors.New("boom"), want: false},
		{name: "lua error reply", err: errors.New("ERR 0018 insufficient funds"), want: false},
		{name: "non-timeout net error", err: nonTimeoutNetError{}, want: false},

		{name: "context deadline exceeded", err: context.DeadlineExceeded, want: true},
		{name: "wrapped deadline exceeded", err: fmt.Errorf("run: %w", context.DeadlineExceeded), want: true},
		{name: "io EOF", err: io.EOF, want: true},
		{name: "wrapped io EOF", err: fmt.Errorf("read: %w", io.EOF), want: true},
		{name: "unexpected EOF", err: io.ErrUnexpectedEOF, want: true},
		{name: "connection reset", err: syscall.ECONNRESET, want: true},
		{name: "wrapped connection reset", err: fmt.Errorf("read: %w", syscall.ECONNRESET), want: true},
		{name: "broken pipe", err: syscall.EPIPE, want: true},
		{name: "net timeout", err: timeoutNetError{}, want: true},
		{name: "wrapped net timeout", err: fmt.Errorf("run: %w", timeoutNetError{}), want: true},
		{name: "op error with timeout", err: &net.OpError{Op: "read", Err: timeoutNetError{}}, want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isResponseLostError(tc.err))
		})
	}
}

// TestIsResponseLostError_CanceledBeatsDeadline pins the precedence that keeps a
// cancelled caller out of the reconciliation path even when the cancellation
// travels alongside an expired deadline: nobody is left to consume a converted
// success.
func TestIsResponseLostError_CanceledBeatsDeadline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.False(t, isResponseLostError(ctx.Err()))
}

// =============================================================================
// recordBalanceScriptIdempotency — nil factory is a no-op
// =============================================================================

func TestRecordBalanceScriptIdempotency_NilFactoryIsNoOp(t *testing.T) {
	t.Parallel()

	assert.NotPanics(t, func() {
		recordBalanceScriptIdempotency(context.Background(), nil, nil, balanceScriptIdempotencyReplayed)
		recordBalanceScriptIdempotency(context.Background(), nil, nil, balanceScriptIdempotencyReconciled)
		recordBalanceScriptIdempotency(context.Background(), nil, nil, balanceScriptIdempotencyReconcileMiss)
	})
}

// TestBalanceScriptIdempotencyOutcomes locks the label set: the metric's only
// dimension is bounded, so a new outcome has to be added deliberately here.
func TestBalanceScriptIdempotencyOutcomes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "replayed", balanceScriptIdempotencyReplayed)
	assert.Equal(t, "reconciled", balanceScriptIdempotencyReconciled)
	assert.Equal(t, "reconcile_miss", balanceScriptIdempotencyReconcileMiss)
	assert.Equal(t, "balance_script_idempotency_total", balanceScriptIdempotencyTotal.Name)
}

// =============================================================================
// decodeBalanceAtomicResult — the replay flag reaches the adapter, not mmodel
// =============================================================================

func decodeTestMapBalances() map[string]*mmodel.Balance {
	return map[string]*mmodel.Balance{
		"@decode": {
			ID:             "balance-1",
			Alias:          "@decode",
			Key:            constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			OrganizationID: "org",
			LedgerID:       "ledger",
		},
	}
}

const decodeTestPayload = `{"before":[{"id":"balance-1","alias":"@decode","available":"500","onHold":"0","version":1}],` +
	`"after":[{"id":"balance-1","alias":"@decode","available":"300","onHold":"0","version":2}]}`

func TestDecodeBalanceAtomicResult_WithoutReplayFlag(t *testing.T) {
	t.Parallel()

	result, replayed, missingAliases, err := decodeBalanceAtomicResult(context.Background(), decodeTestPayload, decodeTestMapBalances())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, replayed, "a first execution carries no flag")
	assert.Empty(t, missingAliases, "every alias in the payload resolves against the fixture map")
	require.Len(t, result.Before, 1)
	require.Len(t, result.After, 1)
	assert.Equal(t, "500", result.Before[0].Available.String())
	assert.Equal(t, "300", result.After[0].Available.String())
	assert.Equal(t, int64(2), result.After[0].Version)
}

func TestDecodeBalanceAtomicResult_WithReplayFlag(t *testing.T) {
	t.Parallel()

	// Exactly the splice the Lua replay path performs on the stored payload.
	replayedPayload := `{"replayed":true,` + decodeTestPayload[1:]

	result, replayed, missingAliases, err := decodeBalanceAtomicResult(context.Background(), replayedPayload, decodeTestMapBalances())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, replayed, "the spliced flag must reach the adapter")
	assert.Empty(t, missingAliases)
	require.Len(t, result.After, 1)
	assert.Equal(t, "300", result.After[0].Available.String(),
		"a replay must decode to the same posting as the original")
}

func TestDecodeBalanceAtomicResult_MalformedPayload(t *testing.T) {
	t.Parallel()

	result, replayed, missingAliases, err := decodeBalanceAtomicResult(context.Background(), `{"before":`, decodeTestMapBalances())

	require.Error(t, err)
	assert.Nil(t, result)
	assert.False(t, replayed)
	assert.Empty(t, missingAliases)
}

func TestDecodeBalanceAtomicResult_UnexpectedResultType(t *testing.T) {
	t.Parallel()

	result, replayed, missingAliases, err := decodeBalanceAtomicResult(context.Background(), 42, decodeTestMapBalances())

	require.Error(t, err)
	assert.Nil(t, result)
	assert.False(t, replayed)
	assert.Empty(t, missingAliases)
}

// TestDecodeBalanceAtomicResult_MissingAliasReportedNotDropped pins Fix 1's
// contract at the decode level: an alias the payload names but mapBalances
// cannot resolve is surfaced to the caller instead of being silently absorbed
// into a truncated Before/After. The caller (ProcessBalanceAtomicOperation for
// a Lua replay, reconcileFromApplyMarker for a Go-side reconciliation) is what
// decides whether that miss is tolerable — decode itself never fails on it.
func TestDecodeBalanceAtomicResult_MissingAliasReportedNotDropped(t *testing.T) {
	t.Parallel()

	// @gone is not in decodeTestMapBalances(): a stand-in for a companion an
	// earlier plan produced (e.g. via enrichOverdraftOperations) that the
	// CURRENT mapBalances no longer carries.
	payload := `{"before":[` +
		`{"id":"balance-1","alias":"@decode","available":"500","onHold":"0","version":1},` +
		`{"id":"balance-2","alias":"@gone","available":"100","onHold":"0","version":1}` +
		`],"after":[` +
		`{"id":"balance-1","alias":"@decode","available":"300","onHold":"0","version":2}` +
		`]}`

	result, replayed, missingAliases, err := decodeBalanceAtomicResult(context.Background(), payload, decodeTestMapBalances())

	require.NoError(t, err, "decode itself never fails on a missing alias")
	require.NotNil(t, result)
	assert.False(t, replayed)
	require.Len(t, result.Before, 1, "the resolvable balance is still collected")
	assert.Equal(t, []string{"@gone"}, missingAliases, "the unresolved alias is reported, not silently dropped")
}
