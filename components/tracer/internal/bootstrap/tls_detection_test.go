// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDetectPostgresTLS exercises both DSN forms supported by libpq:
//  1. URL form (postgres://...?sslmode=...)
//  2. keyword/value form (host=... user=... sslmode=...)
//
// The keyword/value tokenizer MUST NOT use strings.Contains against the full
// DSN — that would be substring matching (anti-pattern N4). Tokenization via
// strings.Fields followed by per-token "k=v" parsing is structural parsing
// and is the only acceptable way to read the keyword/value DSN.
func TestDetectPostgresTLS(t *testing.T) {
	tests := []struct {
		name      string
		dsn       string
		want      bool
		wantErr   bool
		wantErrIs string
	}{
		// --- URL form ---
		{
			name: "url_form_sslmode_require_returns_true",
			dsn:  "postgres://user:pass@db.example.com:5432/tracer?sslmode=require",
			want: true,
		},
		{
			name: "url_form_sslmode_disable_returns_false",
			dsn:  "postgres://user:pass@db.example.com:5432/tracer?sslmode=disable",
			want: false,
		},
		{
			name: "url_form_sslmode_verify_full_returns_true",
			dsn:  "postgres://user:pass@db.example.com:5432/tracer?sslmode=verify-full",
			want: true,
		},
		{
			name: "url_form_postgresql_scheme_sslmode_require_returns_true",
			dsn:  "postgresql://user:pass@db.example.com:5432/tracer?sslmode=require",
			want: true,
		},
		{
			name: "url_form_url_encoded_sslmode_value_returns_true",
			// %72%65%71%75%69%72%65 == "require"; ensures we use Query().Get
			// (which decodes), not raw substring matching.
			dsn:  "postgres://user:pass@db.example.com:5432/tracer?sslmode=%72%65%71%75%69%72%65",
			want: true,
		},
		{
			name: "url_form_no_sslmode_returns_false",
			dsn:  "postgres://user:pass@db.example.com:5432/tracer",
			want: false,
		},

		// --- keyword/value form ---
		{
			name: "kv_form_sslmode_disable_returns_false",
			dsn:  "host=localhost user=tracer password=secret dbname=tracer port=5432 sslmode=disable",
			want: false,
		},
		{
			name: "kv_form_sslmode_require_returns_true",
			dsn:  "host=localhost user=tracer password=secret dbname=tracer port=5432 sslmode=require",
			want: true,
		},
		{
			name: "kv_form_sslmode_verify_full_returns_true",
			dsn:  "host=localhost user=tracer password=secret dbname=tracer port=5432 sslmode=verify-full",
			want: true,
		},
		{
			name: "kv_form_no_sslmode_key_returns_false",
			dsn:  "host=localhost user=tracer password=secret dbname=tracer port=5432",
			want: false,
		},

		// --- substring-ambiguous safety ---
		{
			// dbname embedding "tls=false" must NOT trigger false-positive TLS=true.
			name: "kv_form_substring_ambiguous_dbname_returns_false",
			dsn:  "host=localhost user=tracer password=secret dbname=mydb_tls_false port=5432 sslmode=disable",
			want: false,
		},
		{
			// Password containing "sslmode=" substring must not confuse the parser
			// when the DSN has no actual sslmode key. (Edge case: passwords with
			// equals signs are syntactically invalid in keyword/value DSN; we
			// document by asserting a valid no-sslmode DSN with similar-looking
			// values stays at false.)
			name: "kv_form_no_sslmode_with_dbname_lookalike_returns_false",
			dsn:  "host=localhost user=tracer password=plainsecret dbname=tracer_sslmode_test port=5432",
			want: false,
		},

		// --- empty / malformed ---
		{
			name: "empty_dsn_returns_false",
			dsn:  "",
			want: false,
		},
		{
			name:    "url_form_malformed_returns_error",
			dsn:     "postgres://user:p%ZZ@host/db?sslmode=require",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectPostgresTLS(tt.dsn)
			if tt.wantErr {
				require.Error(t, err)
				require.False(t, got, "TLS must default to false on parse error")

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestPostgresSSLModeIsTLS covers the centralized rule that any non-empty
// sslmode other than "disable" implies TLS.
func TestPostgresSSLModeIsTLS(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{mode: "", want: false},
		{mode: "disable", want: false},
		{mode: "allow", want: true},
		{mode: "prefer", want: true},
		{mode: "require", want: true},
		{mode: "verify-ca", want: true},
		{mode: "verify-full", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			require.Equal(t, tt.want, postgresSSLModeIsTLS(tt.mode))
		})
	}
}
