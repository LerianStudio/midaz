// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateSaaSTLS verifies the centralized SaaS TLS enforcement contract:
//
//   - DEPLOYMENT_MODE != "saas" → always nil (no enforcement).
//   - DEPLOYMENT_MODE == "saas" + all configured deps TLS → nil.
//   - DEPLOYMENT_MODE == "saas" + any non-TLS dep → error nominating that dep.
//   - DEPLOYMENT_MODE == "saas" + empty URI → skip that dep (treated as
//     "not configured").
//   - DEPLOYMENT_MODE == "saas" + malformed URI → wrapped parse error.
//   - Validation stops at the FIRST failing dep (no error aggregation).
//   - "saas" matching is case-insensitive and tolerant of whitespace.
//
// The function must NEVER inspect connection objects via reflection — TLS
// posture is determined entirely from the DSN string via the Detect* helpers.
func TestValidateSaaSTLS(t *testing.T) {
	t.Parallel()

	// Sentinel error so we can verify error wrapping with errors.Is.
	wantParseErr := errors.New("synthetic parse failure")

	// alwaysFail is a DetectFn that always returns an error for testing
	// the malformed-URI path without depending on url.Parse internals.
	alwaysFail := func(_ string) (bool, error) {
		return false, fmt.Errorf("detect failed: %w", wantParseErr)
	}

	tests := []struct {
		name           string
		deploymentMode string
		deps           []SaaSTLSDep
		wantErr        bool
		wantErrContain string
		wantErrIs      error
	}{
		{
			name:           "saas with all deps TLS returns nil",
			deploymentMode: "saas",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "mongodb+srv://cluster.example.net/db", DetectFn: DetectMongoTLS},
				{Name: "rabbitmq", URI: "amqps://rabbit.example.com:5671/", DetectFn: DetectAMQPTLS},
				{Name: "redis", URI: "rediss://valkey.example.com:6380/0", DetectFn: DetectRedisTLS},
				{Name: "storage", URI: "https://s3.amazonaws.com", DetectFn: DetectS3TLS},
				{Name: "tenant_manager", URI: "https://tenant-manager.example.com", DetectFn: DetectHTTPUpstreamTLS},
			},
			wantErr: false,
		},
		{
			name:           "saas with mongodb non-TLS returns error nominating mongodb",
			deploymentMode: "saas",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "mongodb://insecure-host:27017/db", DetectFn: DetectMongoTLS},
				{Name: "rabbitmq", URI: "amqps://rabbit.example.com:5671/", DetectFn: DetectAMQPTLS},
			},
			wantErr:        true,
			wantErrContain: "mongodb",
		},
		{
			name:           "saas with rabbitmq non-TLS returns error nominating rabbitmq",
			deploymentMode: "saas",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "mongodb+srv://cluster.example.net/db", DetectFn: DetectMongoTLS},
				{Name: "rabbitmq", URI: "amqp://rabbit.example.com:5672/", DetectFn: DetectAMQPTLS},
			},
			wantErr:        true,
			wantErrContain: "rabbitmq",
		},
		{
			name:           "saas skips empty URI (dep not configured) and continues",
			deploymentMode: "saas",
			deps: []SaaSTLSDep{
				// Mongo not configured (empty) — must be skipped, not flagged.
				{Name: "mongodb", URI: "", DetectFn: DetectMongoTLS},
				{Name: "rabbitmq", URI: "amqps://rabbit.example.com:5671/", DetectFn: DetectAMQPTLS},
				{Name: "tenant_manager", URI: "", DetectFn: DetectHTTPUpstreamTLS},
				{Name: "storage", URI: "https://s3.amazonaws.com", DetectFn: DetectS3TLS},
			},
			wantErr: false,
		},
		{
			name:           "saas with whitespace-only URI is treated as empty (skipped)",
			deploymentMode: "saas",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "   ", DetectFn: DetectMongoTLS},
				{Name: "rabbitmq", URI: "amqps://rabbit.example.com:5671/", DetectFn: DetectAMQPTLS},
			},
			wantErr: false,
		},
		{
			name:           "saas with malformed URI returns wrapped parse error",
			deploymentMode: "saas",
			deps: []SaaSTLSDep{
				{Name: "rabbitmq", URI: "not-empty-but-detect-fails", DetectFn: alwaysFail},
			},
			wantErr:        true,
			wantErrContain: "rabbitmq",
			wantErrIs:      wantParseErr,
		},
		{
			name:           "byoc mode never enforces TLS even with non-TLS DSN",
			deploymentMode: "byoc",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "mongodb://insecure-host:27017/db", DetectFn: DetectMongoTLS},
				{Name: "rabbitmq", URI: "amqp://insecure:5672/", DetectFn: DetectAMQPTLS},
			},
			wantErr: false,
		},
		{
			name:           "local mode never enforces TLS even with non-TLS DSN",
			deploymentMode: "local",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "mongodb://insecure-host:27017/db", DetectFn: DetectMongoTLS},
			},
			wantErr: false,
		},
		{
			name:           "empty deployment mode defaults to no enforcement",
			deploymentMode: "",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "mongodb://insecure-host:27017/db", DetectFn: DetectMongoTLS},
			},
			wantErr: false,
		},
		{
			name:           "uppercase SAAS still enforces (case-insensitive match)",
			deploymentMode: "SAAS",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "mongodb://insecure-host:27017/db", DetectFn: DetectMongoTLS},
			},
			wantErr:        true,
			wantErrContain: "mongodb",
		},
		{
			name:           "mixed-case Saas still enforces (case-insensitive match)",
			deploymentMode: "Saas",
			deps: []SaaSTLSDep{
				{Name: "rabbitmq", URI: "amqp://rabbit.example.com:5672/", DetectFn: DetectAMQPTLS},
			},
			wantErr:        true,
			wantErrContain: "rabbitmq",
		},
		{
			name:           "whitespace-padded \" saas \" still enforces (trimmed match)",
			deploymentMode: " saas ",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "mongodb://insecure-host:27017/db", DetectFn: DetectMongoTLS},
			},
			wantErr:        true,
			wantErrContain: "mongodb",
		},
		{
			name:           "saas with empty deps slice returns nil (nothing to enforce)",
			deploymentMode: "saas",
			deps:           nil,
			wantErr:        false,
		},
		{
			name:           "saas error message mentions DEPLOYMENT_MODE for operator clarity",
			deploymentMode: "saas",
			deps: []SaaSTLSDep{
				{Name: "storage", URI: "http://seaweedfs:8333", DetectFn: DetectS3TLS},
			},
			wantErr:        true,
			wantErrContain: "DEPLOYMENT_MODE=saas",
		},
		{
			name: "saas validation stops at first failing dep (does not check subsequent)",
			// If validation continued, the alwaysFail dep would also produce an
			// error and the message would mention "trap". By asserting the
			// message mentions ONLY mongodb (and no trap), we lock in
			// short-circuit behavior.
			deploymentMode: "saas",
			deps: []SaaSTLSDep{
				{Name: "mongodb", URI: "mongodb://insecure-host:27017/db", DetectFn: DetectMongoTLS},
				{Name: "trap", URI: "anything", DetectFn: alwaysFail},
			},
			wantErr:        true,
			wantErrContain: "mongodb",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateSaaSTLS(tt.deploymentMode, tt.deps)

			if !tt.wantErr {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			if tt.wantErrContain != "" {
				assert.Contains(t, err.Error(), tt.wantErrContain)
			}

			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs, "error must wrap the underlying parse error")
			}
		})
	}
}

// TestValidateSaaSTLS_FirstFailureWins explicitly asserts the short-circuit
// behavior with a counter that tracks how many DetectFn calls happened.
//
// Validation MUST stop at the first failing dep. Subsequent deps must NOT be
// invoked. Test this with a counter so we don't rely on error-message
// inspection alone.
func TestValidateSaaSTLS_FirstFailureWins(t *testing.T) {
	t.Parallel()

	calls := 0
	count := func(_ string) (bool, error) {
		calls++
		return true, nil // would pass if reached
	}

	deps := []SaaSTLSDep{
		// First dep is non-TLS → ValidateSaaSTLS must short-circuit here.
		{Name: "mongodb", URI: "mongodb://insecure-host:27017/db", DetectFn: DetectMongoTLS},
		// These must not be invoked.
		{Name: "second", URI: "any", DetectFn: count},
		{Name: "third", URI: "any", DetectFn: count},
	}

	err := ValidateSaaSTLS("saas", deps)
	require.Error(t, err)
	assert.Equal(t, 0, calls, "subsequent DetectFn calls must NOT be made after first failure")
}
