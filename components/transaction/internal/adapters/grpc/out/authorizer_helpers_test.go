// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package out

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsRetryableAuthorizerError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "non-status error returns false",
			err:  errors.New("plain error"),
			want: false,
		},
		{
			name: "nil error is treated as non-status",
			err:  nil,
			// status.FromError(nil) yields ok=true with codes.OK, which is
			// not in the retryable set, so this returns false.
			want: false,
		},
		{
			name: "Unavailable is retryable",
			err:  status.Error(codes.Unavailable, "service unavailable"),
			want: true,
		},
		{
			name: "DeadlineExceeded is retryable",
			err:  status.Error(codes.DeadlineExceeded, "deadline exceeded"),
			want: true,
		},
		{
			name: "ResourceExhausted is retryable",
			err:  status.Error(codes.ResourceExhausted, "resource exhausted"),
			want: true,
		},
		{
			name: "Aborted is retryable",
			err:  status.Error(codes.Aborted, "aborted"),
			want: true,
		},
		{
			name: "InvalidArgument is not retryable",
			err:  status.Error(codes.InvalidArgument, "bad arg"),
			want: false,
		},
		{
			name: "NotFound is not retryable",
			err:  status.Error(codes.NotFound, "not found"),
			want: false,
		},
		{
			name: "PermissionDenied is not retryable",
			err:  status.Error(codes.PermissionDenied, "denied"),
			want: false,
		},
		{
			name: "Internal is not retryable",
			err:  status.Error(codes.Internal, "internal"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isRetryableAuthorizerError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSleepWithContext_CompletesNormally(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	start := time.Now()
	err := sleepWithContext(ctx, 10*time.Millisecond)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 10*time.Millisecond)
}

func TestSleepWithContext_CanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := sleepWithContext(ctx, 5*time.Second)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "sleep interrupted")
}

func TestSleepWithContext_DeadlineExceeded(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := sleepWithContext(ctx, 5*time.Second)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	// Should return close to 5ms, not wait the full 5s.
	assert.Less(t, elapsed, 1*time.Second)
}
