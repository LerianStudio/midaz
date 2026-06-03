// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"testing"

	"github.com/LerianStudio/reporter/pkg"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_DirectMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  ProviderConfig
	}{
		{
			name: "returns DirectProvider when FetcherEnabled is false",
			cfg: ProviderConfig{
				FetcherEnabled:     false,
				MultiTenantEnabled: false,
				SafeDataSources:    pkg.NewSafeDataSources(nil),
			},
		},
		{
			name: "returns DirectProvider with all optional deps",
			cfg: ProviderConfig{
				FetcherEnabled:        false,
				MultiTenantEnabled:    false,
				SafeDataSources:       pkg.NewSafeDataSources(nil),
				CircuitBreakerManager: createTestCircuitBreakerManager(),
				HealthChecker:         createTestHealthChecker(t, nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.cfg)
			require.NoError(t, err)
			require.NotNil(t, provider)

			_, ok := provider.(*DirectProvider)
			assert.True(t, ok, "expected *DirectProvider, got %T", provider)
		})
	}
}

func TestNewProvider_FetcherMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  ProviderConfig
	}{
		{
			name: "returns FetcherProvider when FetcherEnabled is true (single-tenant)",
			cfg: ProviderConfig{
				FetcherEnabled:     true,
				FetcherURL:         "http://fetcher:4007",
				MultiTenantEnabled: false,
			},
		},
		{
			name: "returns FetcherProvider when FetcherEnabled is true (multi-tenant)",
			cfg: ProviderConfig{
				FetcherEnabled:     true,
				FetcherURL:         "http://fetcher:4007",
				MultiTenantEnabled: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.cfg)
			require.NoError(t, err)
			require.NotNil(t, provider)

			_, ok := provider.(*FetcherProvider)
			assert.True(t, ok, "expected *FetcherProvider, got %T", provider)
		})
	}
}

func TestNewProvider_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		cfg        ProviderConfig
		wantErrMsg string
	}{
		{
			name: "fails when FetcherEnabled but FetcherURL empty",
			cfg: ProviderConfig{
				FetcherEnabled:     true,
				FetcherURL:         "",
				MultiTenantEnabled: false,
			},
			wantErrMsg: "FETCHER_ENABLED=true requires FETCHER_URL to be set",
		},
		{
			name: "fails when MultiTenantEnabled but FetcherEnabled false",
			cfg: ProviderConfig{
				FetcherEnabled:     false,
				FetcherURL:         "",
				MultiTenantEnabled: true,
				SafeDataSources:    pkg.NewSafeDataSources(nil),
			},
			wantErrMsg: "MULTI_TENANT_ENABLED=true requires FETCHER_ENABLED=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.cfg)
			require.Error(t, err)
			assert.Nil(t, provider)
			assert.Contains(t, err.Error(), tt.wantErrMsg)
		})
	}
}

func TestValidateProviderConfig(t *testing.T) {
	tests := []struct {
		name       string
		cfg        ProviderConfig
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "valid direct mode config",
			cfg: ProviderConfig{
				FetcherEnabled:     false,
				MultiTenantEnabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid fetcher mode config",
			cfg: ProviderConfig{
				FetcherEnabled:     true,
				FetcherURL:         "http://fetcher:4007",
				MultiTenantEnabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid fetcher + multi-tenant config",
			cfg: ProviderConfig{
				FetcherEnabled:     true,
				FetcherURL:         "http://fetcher:4007",
				MultiTenantEnabled: true,
			},
			wantErr: false,
		},
		{
			name: "invalid: fetcher enabled without URL",
			cfg: ProviderConfig{
				FetcherEnabled:     true,
				FetcherURL:         "",
				MultiTenantEnabled: false,
			},
			wantErr:    true,
			wantErrMsg: "FETCHER_ENABLED=true requires FETCHER_URL to be set",
		},
		{
			name: "invalid: multi-tenant without fetcher",
			cfg: ProviderConfig{
				FetcherEnabled:     false,
				MultiTenantEnabled: true,
			},
			wantErr:    true,
			wantErrMsg: "MULTI_TENANT_ENABLED=true requires FETCHER_ENABLED=true",
		},
		{
			name: "invalid: whitespace-only FetcherURL treated as empty",
			cfg: ProviderConfig{
				FetcherEnabled:     true,
				FetcherURL:         "   ",
				MultiTenantEnabled: false,
			},
			wantErr:    true,
			wantErrMsg: "FETCHER_ENABLED=true requires FETCHER_URL to be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviderConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProviderConfig_Defaults(t *testing.T) {
	// Verify zero-value ProviderConfig defaults to direct mode (no fetcher).
	cfg := ProviderConfig{}
	assert.False(t, cfg.FetcherEnabled)
	assert.False(t, cfg.MultiTenantEnabled)
	assert.Empty(t, cfg.FetcherURL)
}
