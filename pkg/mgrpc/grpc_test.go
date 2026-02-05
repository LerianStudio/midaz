// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestGetHealthCheckTimeout(t *testing.T) {
	// Cannot use t.Parallel() because t.Setenv modifies global state
	tests := []struct {
		name     string
		envValue string
		want     time.Duration
	}{
		{
			name:     "empty env returns default",
			envValue: "",
			want:     defaultHealthCheckTimeout,
		},
		{
			name:     "valid duration 10s",
			envValue: "10s",
			want:     10 * time.Second,
		},
		{
			name:     "valid duration 500ms",
			envValue: "500ms",
			want:     500 * time.Millisecond,
		},
		{
			name:     "valid duration 2m",
			envValue: "2m",
			want:     2 * time.Minute,
		},
		{
			name:     "invalid duration returns default",
			envValue: "invalid",
			want:     defaultHealthCheckTimeout,
		},
		{
			name:     "numeric without unit returns default",
			envValue: "5000",
			want:     defaultHealthCheckTimeout,
		},
		{
			name:     "negative duration returns default",
			envValue: "-5s",
			want:     defaultHealthCheckTimeout,
		},
		{
			name:     "zero duration returns default",
			envValue: "0s",
			want:     defaultHealthCheckTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GRPC_HEALTH_CHECK_TIMEOUT", tt.envValue)

			got := getHealthCheckTimeout()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetHealthCheckTimeout_UnsetEnv(t *testing.T) {
	// Ensure env is not set (t.Setenv automatically restores after test)
	t.Setenv("GRPC_HEALTH_CHECK_TIMEOUT", "")

	got := getHealthCheckTimeout()
	assert.Equal(t, defaultHealthCheckTimeout, got)
	assert.Equal(t, 5*time.Second, got, "default should be 5 seconds")
}

func TestContextMetadataInjection_PreservesExistingMetadata(t *testing.T) {
	t.Parallel()

	conn := &GRPCConnection{}

	// Create context with existing metadata
	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"x-custom-header", "custom-value",
	)

	result := conn.ContextMetadataInjection(ctx, "Bearer token")

	md, ok := metadata.FromOutgoingContext(result)
	require.True(t, ok)

	// Verify existing metadata is preserved
	customValues := md.Get("x-custom-header")
	require.Len(t, customValues, 1)
	assert.Equal(t, "custom-value", customValues[0])

	// Verify new authorization is added
	authValues := md.Get("authorization")
	require.Len(t, authValues, 1)
	assert.Equal(t, "Bearer token", authValues[0])
}

func TestContextMetadataInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		token           string
		wantAuthPresent bool
	}{
		{
			name:            "valid token",
			token:           "Bearer abc123",
			wantAuthPresent: true,
		},
		{
			name:            "empty token",
			token:           "",
			wantAuthPresent: false,
		},
		{
			name:            "whitespace only",
			token:           "   \t\n",
			wantAuthPresent: false,
		},
		{
			name:            "token with leading/trailing spaces",
			token:           "  Bearer token  ",
			wantAuthPresent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := &GRPCConnection{}
			ctx := context.Background()

			result := conn.ContextMetadataInjection(ctx, tt.token)

			md, _ := metadata.FromOutgoingContext(result)
			authValues := md.Get("authorization")

			if tt.wantAuthPresent {
				require.Len(t, authValues, 1, "authorization should be present")
				assert.Equal(t, tt.token, authValues[0])
			} else {
				assert.Empty(t, authValues, "authorization should not be present")
			}
		})
	}
}
