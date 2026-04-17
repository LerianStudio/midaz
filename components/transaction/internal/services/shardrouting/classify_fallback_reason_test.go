// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package shardrouting

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
)

func TestClassifyFallbackReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error returns unknown",
			err:  nil,
			want: "unknown",
		},
		{
			name: "invalid shard count",
			err:  internalsharding.ErrInvalidShardCount,
			want: "invalid_shard_count",
		},
		{
			name: "wrapped invalid shard count",
			err:  fmt.Errorf("wrapper: %w", internalsharding.ErrInvalidShardCount),
			want: "invalid_shard_count",
		},
		{
			name: "context canceled",
			err:  context.Canceled,
			want: "context_canceled",
		},
		{
			name: "wrapped context canceled",
			err:  fmt.Errorf("op failed: %w", context.Canceled),
			want: "context_canceled",
		},
		{
			name: "context deadline exceeded",
			err:  context.DeadlineExceeded,
			want: "context_deadline",
		},
		{
			name: "wrapped context deadline exceeded",
			err:  fmt.Errorf("timeout: %w", context.DeadlineExceeded),
			want: "context_deadline",
		},
		{
			name: "unknown error defaults to manager_error",
			err:  errors.New("some random error"),
			want: "manager_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classifyFallbackReason(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
