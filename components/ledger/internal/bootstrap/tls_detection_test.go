// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectPostgresTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dsn  string
		want bool
	}{
		{
			name: "empty string returns false",
			dsn:  "",
			want: false,
		},
		{
			name: "sslmode=disable returns false",
			dsn:  "host=localhost user=midaz password=secret dbname=onboarding port=5432 sslmode=disable",
			want: false,
		},
		{
			name: "sslmode=require returns true",
			dsn:  "host=localhost user=midaz password=secret dbname=onboarding port=5432 sslmode=require",
			want: true,
		},
		{
			name: "sslmode=verify-ca returns true",
			dsn:  "host=localhost user=midaz password=secret dbname=onboarding port=5432 sslmode=verify-ca",
			want: true,
		},
		{
			name: "sslmode=verify-full returns true",
			dsn:  "host=localhost user=midaz password=secret dbname=onboarding port=5432 sslmode=verify-full",
			want: true,
		},
		{
			name: "sslmode=allow returns false (optional TLS)",
			dsn:  "host=localhost user=midaz password=secret dbname=onboarding port=5432 sslmode=allow",
			want: false,
		},
		{
			name: "sslmode=prefer returns false (optional TLS)",
			dsn:  "host=localhost user=midaz password=secret dbname=onboarding port=5432 sslmode=prefer",
			want: false,
		},
		{
			name: "no sslmode parameter returns false",
			dsn:  "host=localhost user=midaz password=secret dbname=onboarding port=5432",
			want: false,
		},
		{
			name: "sslmode uppercase returns true",
			dsn:  "host=localhost user=midaz password=secret dbname=onboarding port=5432 SSLMODE=REQUIRE",
			want: true,
		},
		{
			name: "sslmode mixed case returns true",
			dsn:  "host=localhost user=midaz password=secret dbname=onboarding port=5432 SslMode=Verify-Full",
			want: true,
		},
		{
			name: "sslmode with extra spaces",
			dsn:  "host=localhost  user=midaz  password=secret  dbname=onboarding  port=5432  sslmode=require",
			want: true,
		},
		{
			name: "password with equals sign",
			dsn:  "host=localhost user=midaz password=sec=ret dbname=onboarding port=5432 sslmode=require",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := detectPostgresTLS(tt.dsn)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectMongoTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		uri     string
		want    bool
		wantErr bool
	}{
		{
			name:    "empty string returns false",
			uri:     "",
			want:    false,
			wantErr: false,
		},
		{
			name:    "mongodb scheme without tls returns false",
			uri:     "mongodb://user:pass@localhost:27017/dbname",
			want:    false,
			wantErr: false,
		},
		{
			name:    "mongodb+srv scheme returns true (always TLS)",
			uri:     "mongodb+srv://user:pass@cluster.mongodb.net/dbname",
			want:    true,
			wantErr: false,
		},
		{
			name:    "mongodb with tls=true returns true",
			uri:     "mongodb://user:pass@localhost:27017/dbname?tls=true",
			want:    true,
			wantErr: false,
		},
		{
			name:    "mongodb with tls=false returns false",
			uri:     "mongodb://user:pass@localhost:27017/dbname?tls=false",
			want:    false,
			wantErr: false,
		},
		{
			name:    "mongodb with ssl=true returns true (legacy)",
			uri:     "mongodb://user:pass@localhost:27017/dbname?ssl=true",
			want:    true,
			wantErr: false,
		},
		{
			name:    "mongodb with ssl=false returns false",
			uri:     "mongodb://user:pass@localhost:27017/dbname?ssl=false",
			want:    false,
			wantErr: false,
		},
		{
			name:    "mongodb with TLS=TRUE uppercase returns true",
			uri:     "mongodb://user:pass@localhost:27017/dbname?TLS=TRUE",
			want:    true,
			wantErr: false,
		},
		{
			name:    "mongodb+srv uppercase scheme returns true",
			uri:     "MONGODB+SRV://user:pass@cluster.mongodb.net/dbname",
			want:    true,
			wantErr: false,
		},
		{
			name:    "mongodb with multiple query params including tls",
			uri:     "mongodb://user:pass@localhost:27017/dbname?retryWrites=true&tls=true&w=majority",
			want:    true,
			wantErr: false,
		},
		{
			name:    "mongodb with url-encoded params",
			uri:     "mongodb://user:pass@localhost:27017/dbname?authSource=admin&tls=true",
			want:    true,
			wantErr: false,
		},
		{
			name:    "malformed URI returns error",
			uri:     "://invalid-uri",
			want:    false,
			wantErr: true,
		},
		{
			name:    "mongodb with replicaSet and tls",
			uri:     "mongodb://user:pass@host1:27017,host2:27017/dbname?replicaSet=rs0&tls=true",
			want:    true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := detectMongoTLS(tt.uri)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectRedisTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		host       string
		tlsEnabled bool
		want       bool
	}{
		{
			name:       "empty host with tlsEnabled false returns false",
			host:       "",
			tlsEnabled: false,
			want:       false,
		},
		{
			name:       "empty host with tlsEnabled true returns true",
			host:       "",
			tlsEnabled: true,
			want:       true,
		},
		{
			name:       "plain host with tlsEnabled false returns false",
			host:       "localhost:6379",
			tlsEnabled: false,
			want:       false,
		},
		{
			name:       "plain host with tlsEnabled true returns true",
			host:       "localhost:6379",
			tlsEnabled: true,
			want:       true,
		},
		{
			name:       "rediss scheme returns true",
			host:       "rediss://localhost:6379",
			tlsEnabled: false,
			want:       true,
		},
		{
			name:       "rediss scheme uppercase returns true",
			host:       "REDISS://localhost:6379",
			tlsEnabled: false,
			want:       true,
		},
		{
			name:       "redis scheme (single s) returns false",
			host:       "redis://localhost:6379",
			tlsEnabled: false,
			want:       false,
		},
		{
			name:       "rediss scheme with tlsEnabled true returns true",
			host:       "rediss://localhost:6379",
			tlsEnabled: true,
			want:       true,
		},
		{
			name:       "sentinel hosts comma-separated without TLS",
			host:       "sentinel1:26379,sentinel2:26379,sentinel3:26379",
			tlsEnabled: false,
			want:       false,
		},
		{
			name:       "sentinel hosts with tlsEnabled true",
			host:       "sentinel1:26379,sentinel2:26379,sentinel3:26379",
			tlsEnabled: true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := detectRedisTLS(tt.host, tt.tlsEnabled)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectAMQPTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		uri     string
		want    bool
		wantErr bool
	}{
		{
			name:    "empty string returns false",
			uri:     "",
			want:    false,
			wantErr: false,
		},
		{
			name:    "amqp scheme returns false",
			uri:     "amqp://user:pass@localhost:5672/vhost",
			want:    false,
			wantErr: false,
		},
		{
			name:    "amqps scheme returns true",
			uri:     "amqps://user:pass@localhost:5671/vhost",
			want:    true,
			wantErr: false,
		},
		{
			name:    "AMQPS uppercase scheme returns true",
			uri:     "AMQPS://user:pass@localhost:5671/vhost",
			want:    true,
			wantErr: false,
		},
		{
			name:    "amqp without vhost returns false",
			uri:     "amqp://user:pass@localhost:5672",
			want:    false,
			wantErr: false,
		},
		{
			name:    "amqps without vhost returns true",
			uri:     "amqps://user:pass@localhost:5671",
			want:    true,
			wantErr: false,
		},
		{
			name:    "malformed URI returns error",
			uri:     "://invalid-uri",
			want:    false,
			wantErr: true,
		},
		{
			name:    "amqp with query params",
			uri:     "amqp://user:pass@localhost:5672/vhost?heartbeat=30",
			want:    false,
			wantErr: false,
		},
		{
			name:    "amqps with query params",
			uri:     "amqps://user:pass@localhost:5671/vhost?heartbeat=30",
			want:    true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := detectAMQPTLS(tt.uri)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateSaaSTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deploymentMode string
		dependencies   []TLSValidationResult
		wantErr        bool
		errContains    string
	}{
		{
			name:           "local_mode_skips_validation",
			deploymentMode: "local",
			dependencies: []TLSValidationResult{
				{Name: "postgres", TLSEnabled: false},
				{Name: "redis", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "LOCAL_uppercase_skips_validation",
			deploymentMode: "LOCAL",
			dependencies: []TLSValidationResult{
				{Name: "rabbitmq", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "byoc_mode_skips_validation",
			deploymentMode: "byoc",
			dependencies: []TLSValidationResult{
				{Name: "postgres", TLSEnabled: false},
				{Name: "mongo", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "BYOC_uppercase_skips_validation",
			deploymentMode: "BYOC",
			dependencies: []TLSValidationResult{
				{Name: "postgres", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "empty_mode_defaults_to_no_enforcement",
			deploymentMode: "",
			dependencies: []TLSValidationResult{
				{Name: "postgres", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "saas_mode_all_tls_enabled_passes",
			deploymentMode: "saas",
			dependencies: []TLSValidationResult{
				{Name: "postgres", TLSEnabled: true},
				{Name: "mongo", TLSEnabled: true},
				{Name: "redis", TLSEnabled: true},
				{Name: "rabbitmq", TLSEnabled: true},
			},
			wantErr: false,
		},
		{
			name:           "SAAS_uppercase_all_tls_enabled_passes",
			deploymentMode: "SAAS",
			dependencies: []TLSValidationResult{
				{Name: "postgres", TLSEnabled: true},
				{Name: "redis", TLSEnabled: true},
			},
			wantErr: false,
		},
		{
			name:           "saas_mode_one_insecure_fails",
			deploymentMode: "saas",
			dependencies: []TLSValidationResult{
				{Name: "postgres", TLSEnabled: true},
				{Name: "redis", TLSEnabled: false},
			},
			wantErr:     true,
			errContains: "redis",
		},
		{
			name:           "saas_mode_multiple_insecure_fails",
			deploymentMode: "saas",
			dependencies: []TLSValidationResult{
				{Name: "postgres", TLSEnabled: false},
				{Name: "mongo", TLSEnabled: false},
				{Name: "redis", TLSEnabled: true},
			},
			wantErr:     true,
			errContains: "postgres",
		},
		{
			name:           "saas_mode_empty_dependencies_passes",
			deploymentMode: "saas",
			dependencies:   []TLSValidationResult{},
			wantErr:        false,
		},
		{
			name:           "saas_mode_nil_dependencies_passes",
			deploymentMode: "saas",
			dependencies:   nil,
			wantErr:        false,
		},
		{
			name:           "unknown_mode_skips_validation",
			deploymentMode: "production",
			dependencies: []TLSValidationResult{
				{Name: "postgres", TLSEnabled: false},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateSaaSTLS(tt.deploymentMode, tt.dependencies)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Contains(t, err.Error(), "DEPLOYMENT_MODE=saas")

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIsTLSEnforcementRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deploymentMode string
		want           bool
	}{
		{"saas_requires_enforcement", "saas", true},
		{"SAAS_uppercase_requires_enforcement", "SAAS", true},
		{"byoc_no_enforcement", "byoc", false},
		{"local_no_enforcement", "local", false},
		{"empty_no_enforcement", "", false},
		{"unknown_no_enforcement", "production", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsTLSEnforcementRequired(tt.deploymentMode)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsTLSRecommended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deploymentMode string
		want           bool
	}{
		{"byoc_recommended", "byoc", true},
		{"BYOC_uppercase_recommended", "BYOC", true},
		{"saas_not_recommended_but_required", "saas", false},
		{"local_not_recommended", "local", false},
		{"empty_not_recommended", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsTLSRecommended(tt.deploymentMode)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultDeploymentMode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "local", DefaultDeploymentMode, "default deployment mode should be 'local'")
}

func TestResolveDeploymentMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty_returns_default", "", DefaultDeploymentMode},
		{"whitespace_returns_default", "   ", DefaultDeploymentMode},
		{"saas_preserved", "saas", "saas"},
		{"SAAS_lowercase", "SAAS", "saas"},
		{"byoc_preserved", "byoc", "byoc"},
		{"local_preserved", "local", "local"},
		{"unknown_preserved_lowercase", "production", "production"},
		{"leading_whitespace_trimmed", "  saas", "saas"},
		{"trailing_whitespace_trimmed", "byoc  ", "byoc"},
		{"mixed_case_lowercased", "SaaS", "saas"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveDeploymentMode(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectPostgresTLS_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dsn  string
		want bool
	}{
		{
			name: "sslmode_at_start",
			dsn:  "sslmode=verify-full host=localhost user=midaz",
			want: true,
		},
		{
			name: "multiple_sslmode_last_wins",
			dsn:  "host=localhost sslmode=require sslmode=disable",
			want: false, // Last value wins with map
		},
		{
			name: "sslmode_empty_value",
			dsn:  "host=localhost sslmode= user=midaz",
			want: false,
		},
		{
			name: "sslmode_partial_match_sslmodex",
			dsn:  "host=localhost sslmodex=require user=midaz",
			want: false, // Should not match sslmodex
		},
		{
			name: "unicode_in_password_before_sslmode",
			dsn:  "host=localhost user=midaz password=pässwörd sslmode=require",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := detectPostgresTLS(tt.dsn)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectMongoTLS_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		uri     string
		want    bool
		wantErr bool
	}{
		{
			name:    "tls_value_with_whitespace",
			uri:     "mongodb://user:pass@localhost:27017/db?tls= true",
			want:    false, // " true" != "true"
			wantErr: false,
		},
		{
			name:    "multiple_tls_params",
			uri:     "mongodb://user:pass@localhost:27017/db?tls=false&tls=true",
			want:    true, // Should find at least one true
			wantErr: false,
		},
		{
			name:    "ssl_and_tls_mixed",
			uri:     "mongodb://user:pass@localhost:27017/db?ssl=false&tls=true",
			want:    true,
			wantErr: false,
		},
		{
			name:    "fragment_with_tls",
			uri:     "mongodb://user:pass@localhost:27017/db#tls=true",
			want:    false, // Fragment is not a query param
			wantErr: false,
		},
		{
			name:    "ipv6_host_with_tls",
			uri:     "mongodb://user:pass@[::1]:27017/db?tls=true",
			want:    true,
			wantErr: false,
		},
		{
			name:    "mongodb_plus_srv_mixed_case",
			uri:     "MongoDb+SrV://user:pass@cluster.mongodb.net/db",
			want:    true, // SRV always uses TLS
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := detectMongoTLS(tt.uri)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectAMQPTLS_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		uri     string
		want    bool
		wantErr bool
	}{
		{
			name:    "amqps_with_ipv6",
			uri:     "amqps://user:pass@[::1]:5671/vhost",
			want:    true,
			wantErr: false,
		},
		{
			name:    "amqp_uppercase_partial",
			uri:     "AMqp://user:pass@localhost:5672/",
			want:    false, // "amqp" not "amqps"
			wantErr: false,
		},
		{
			name:    "scheme_only",
			uri:     "amqps://",
			want:    true,
			wantErr: false,
		},
		{
			name:    "amqps_with_query_and_fragment",
			uri:     "amqps://user:pass@localhost:5671/vhost?heartbeat=30#section",
			want:    true,
			wantErr: false,
		},
		{
			name:    "empty_scheme",
			uri:     "://localhost:5672/",
			want:    false,
			wantErr: true, // Parse error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := detectAMQPTLS(tt.uri)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectRedisTLS_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		host       string
		tlsEnabled bool
		want       bool
	}{
		{
			name:       "rediss_with_path",
			host:       "rediss://localhost:6379/0",
			tlsEnabled: false,
			want:       true,
		},
		{
			name:       "redis_scheme_not_rediss",
			host:       "redis://localhost:6379",
			tlsEnabled: false,
			want:       false, // Single 's'
		},
		{
			name:       "rediss_uppercase_mixed",
			host:       "ReDiSs://localhost:6379",
			tlsEnabled: false,
			want:       true,
		},
		{
			name:       "cluster_hosts_no_scheme",
			host:       "node1:6379,node2:6379,node3:6379",
			tlsEnabled: false,
			want:       false,
		},
		{
			name:       "sentinel_with_tls_flag",
			host:       "sentinel1:26379,sentinel2:26379",
			tlsEnabled: true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := detectRedisTLS(tt.host, tt.tlsEnabled)
			assert.Equal(t, tt.want, got)
		})
	}
}
