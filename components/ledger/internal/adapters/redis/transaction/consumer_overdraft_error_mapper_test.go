// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

// =============================================================================
// Error mapper — overdraft Lua error codes
// =============================================================================

func TestMapError_OverdraftLimitExceeded(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		luaErr  string
		wantErr error
	}{
		{
			name:    "bare code",
			luaErr:  "0167",
			wantErr: constant.ErrOverdraftLimitExceeded,
		},
		{
			name:    "prefixed message",
			luaErr:  "ERR 0167 overdraft limit exceeded for balance xyz",
			wantErr: constant.ErrOverdraftLimitExceeded,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tracer := noop.NewTracerProvider().Tracer("test")
			_, span := tracer.Start(t.Context(), "test")
			defer span.End()

			rawErr := errors.New(tc.luaErr)
			mapped := mapBalanceAtomicScriptError(span, rawErr)

			expectedMsg := pkg.ValidateBusinessError(tc.wantErr, "validateBalance").Error()
			assert.Equal(t, expectedMsg, mapped.Error(),
				"Lua error %q must map to ErrOverdraftLimitExceeded", tc.luaErr)
		})
	}
}

func TestMapError_ExistingCodes_StillWork(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		luaErr  string
		wantErr error
	}{
		{
			name:    "insufficient funds",
			luaErr:  "0018",
			wantErr: constant.ErrInsufficientFunds,
		},
		{
			name:    "on hold external",
			luaErr:  "0098",
			wantErr: constant.ErrOnHoldExternalAccount,
		},
		{
			name:    "backup cache retrieval failed",
			luaErr:  "0139",
			wantErr: constant.ErrTransactionBackupCacheRetrievalFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tracer := noop.NewTracerProvider().Tracer("test")
			_, span := tracer.Start(t.Context(), "test")
			defer span.End()

			rawErr := errors.New(tc.luaErr)
			mapped := mapBalanceAtomicScriptError(span, rawErr)

			expectedMsg := pkg.ValidateBusinessError(tc.wantErr, "validateBalance").Error()
			assert.Equal(t, expectedMsg, mapped.Error(),
				"Lua error %q must map to the correct business error", tc.luaErr)
		})
	}
}

func TestMapError_StaleBalance(t *testing.T) {
	t.Parallel()

	// The stale-balance Lua error is expected to use code "0175" which does
	// not exist yet — this test will fail until both the constant and the
	// mapper branch are added.
	const staleBalanceCode = "0175"

	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(t.Context(), "test")
	defer span.End()

	rawErr := errors.New(staleBalanceCode)
	mapped := mapBalanceAtomicScriptError(span, rawErr)

	// The mapped error must NOT be the raw input — it must be translated.
	assert.NotEqual(t, rawErr.Error(), mapped.Error(),
		"Stale-balance Lua error %q must be mapped to a typed business error, "+
			"not returned as raw redis error", staleBalanceCode)

	// The mapped error message must contain the code so callers can identify it.
	assert.Contains(t, mapped.Error(), staleBalanceCode,
		"Mapped stale-balance error must contain the error code")
}
