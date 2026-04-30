// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		{"BYOC_lowercase", "BYOC", "byoc"},
		{"local_preserved", "local", "local"},
		{"LOCAL_lowercase", "LOCAL", "local"},
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
				{Name: "mongo", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "LOCAL_uppercase_skips_validation",
			deploymentMode: "LOCAL",
			dependencies: []TLSValidationResult{
				{Name: "mongo", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "byoc_mode_skips_validation",
			deploymentMode: "byoc",
			dependencies: []TLSValidationResult{
				{Name: "mongo", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "BYOC_uppercase_skips_validation",
			deploymentMode: "BYOC",
			dependencies: []TLSValidationResult{
				{Name: "mongo", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "empty_mode_defaults_to_no_enforcement",
			deploymentMode: "",
			dependencies: []TLSValidationResult{
				{Name: "mongo", TLSEnabled: false},
			},
			wantErr: false,
		},
		{
			name:           "saas_mode_all_tls_enabled_passes",
			deploymentMode: "saas",
			dependencies: []TLSValidationResult{
				{Name: "mongo", TLSEnabled: true},
			},
			wantErr: false,
		},
		{
			name:           "SAAS_uppercase_all_tls_enabled_passes",
			deploymentMode: "SAAS",
			dependencies: []TLSValidationResult{
				{Name: "mongo", TLSEnabled: true},
			},
			wantErr: false,
		},
		{
			name:           "saas_mode_one_insecure_fails",
			deploymentMode: "saas",
			dependencies: []TLSValidationResult{
				{Name: "mongo", TLSEnabled: false},
			},
			wantErr:     true,
			errContains: "mongo",
		},
		{
			name:           "saas_mode_multiple_insecure_fails",
			deploymentMode: "saas",
			dependencies: []TLSValidationResult{
				{Name: "mongo", TLSEnabled: false},
				{Name: "upstream", TLSEnabled: false},
			},
			wantErr:     true,
			errContains: "mongo",
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
				{Name: "mongo", TLSEnabled: false},
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
