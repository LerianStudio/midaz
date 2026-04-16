// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// TestEnforceDSNSSLMode_URIForm verifies the URI-form DSN is rejected when
// sslmode=disable and the environment is production-like. The equivalent
// check exists in transaction/config (enforcePostgresSSLMode) but the
// authorizer loader previously accepted raw DSNs without re-checking —
// this guard closes the audit D1 finding #4.
func TestEnforceDSNSSLMode_URIForm(t *testing.T) {
	cases := []struct {
		name    string
		dsn     string
		envName string
		wantErr bool
	}{
		{
			name:    "production rejects sslmode=disable",
			dsn:     "postgres://u:p@h:5432/db?sslmode=disable",
			envName: "production",
			wantErr: true,
		},
		{
			name:    "production accepts sslmode=require",
			dsn:     "postgres://u:p@h:5432/db?sslmode=require",
			envName: "production",
			wantErr: false,
		},
		{
			name:    "production accepts sslmode absent (libpq defaults to prefer)",
			dsn:     "postgres://u:p@h:5432/db",
			envName: "production",
			wantErr: false,
		},
		{
			name:    "development accepts sslmode=disable",
			dsn:     "postgres://u:p@h:5432/db?sslmode=disable",
			envName: "development",
			wantErr: false,
		},
		{
			name:    "test accepts sslmode=disable",
			dsn:     "postgres://u:p@h:5432/db?sslmode=disable",
			envName: "test",
			wantErr: false,
		},
		{
			name:    "empty env defaults to production behavior",
			dsn:     "postgres://u:p@h:5432/db?sslmode=disable",
			envName: "",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceDSNSSLMode(tc.dsn, tc.envName)
			if tc.wantErr {
				require.ErrorIs(t, err, ErrLoaderSSLDisableInProduction)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestEnforceDSNSSLMode_KeywordForm verifies the keyword=value form is also
// honored by the scanner.
func TestEnforceDSNSSLMode_KeywordForm(t *testing.T) {
	err := enforceDSNSSLMode("host=h port=5432 user=u password=p dbname=db sslmode=disable", "production")
	require.ErrorIs(t, err, ErrLoaderSSLDisableInProduction)

	err = enforceDSNSSLMode("host=h port=5432 user=u password=p dbname=db sslmode=require", "production")
	require.NoError(t, err)
}

// TestLoader_RejectsTransactionDBSSLModeDisableInProduction exercises the
// end-to-end path: NewPostgresLoaderWithConfig refuses sslmode=disable in a
// production env before attempting to dial. This closes the audit D1 finding
// that bypassed SSL enforcement via the raw-DSN path.
func TestLoader_RejectsTransactionDBSSLModeDisableInProduction(t *testing.T) {
	_, err := NewPostgresLoaderWithConfig(
		context.Background(),
		"postgres://u:p@unreachable-host:5432/db?sslmode=disable",
		shard.NewRouter(8),
		PoolConfig{EnvName: "production"},
	)
	require.ErrorIs(t, err, ErrLoaderSSLDisableInProduction)
}

// TestLoader_AcceptsSSLDisableInDev documents the complementary case: local
// dev fixtures routinely use sslmode=disable against localhost. The check
// must not fire there. Note: pgxpool creates a background health-check
// goroutine at NewWithConfig time regardless of whether a real dial happens,
// so we MUST Close() the returned loader to let goleak's TestMain succeed.
func TestLoader_AcceptsSSLDisableInDev(t *testing.T) {
	ldr, err := NewPostgresLoaderWithConfig(
		context.Background(),
		"postgres://u:p@127.0.0.1:1/db?sslmode=disable",
		shard.NewRouter(8),
		PoolConfig{EnvName: "development"},
	)
	if ldr != nil {
		defer ldr.Close()
	}

	if err != nil {
		// Must NOT be the SSL error — any other dial error is fine.
		require.NotErrorIs(t, err, ErrLoaderSSLDisableInProduction)
	}
}
