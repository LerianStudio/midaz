// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

const (
	wantMultiTenantErr    = "MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true"
	wantLibAuthMTErr      = `MULTI_TENANT_ENABLED must be exactly "true"`
	wantSeamAllowlistErr  = "requires TRACER_SEAM_ALLOWED_CLIENTS"
	wantClearSaaSErr      = `must not travel in clear; set TRACER_TLS_MODE to "server", "mtls" or "mesh"`
	wantNoIdentitySaaSErr = `set PLUGIN_AUTH_ENABLED=true, API_KEY_ENABLED=true, or TRACER_TLS_MODE to "mtls" or "mesh"`
	wantAllowlistErr      = `set TRACER_TLS_CLIENT_ALLOWED_NAMES so the gRPC seam accepts only the ledger's client certificate`
	wantInvalidModeErr    = `invalid TRACER_TLS_MODE "bogus"`
	wantAuthCacheSaaSErr  = "DEPLOYMENT_MODE=saas with PLUGIN_AUTH_ENABLED=true requires AUTH_CACHE_TTL"

	wantNoIdentityWarn       = "reservation seam has no caller identity; any workload reaching :4021 can release reservations; set API_KEY_ENABLED, PLUGIN_AUTH_ENABLED, or TRACER_TLS_MODE=mtls|mesh"
	wantMeshWarn             = "gRPC reservation seam trusts x-tenant-id from the mesh-verified peer: the mesh must enforce STRICT mTLS and restrict the gRPC port to the ledger identity"
	wantClearWarn            = seamCredentialInClearMsg
	wantAllowlistLocalWarn   = "TRACER_SEAM_ALLOWED_CLIENTS is empty: the reservation seam refuses every caller; set it to the ledger's Access Manager client id(s), comma-separated (the token sub claim)"
	wantAllowlistIgnoredWarn = seamAllowlistIgnoredMsg
	wantAuthCacheWarn        = "AUTH_CACHE_TTL is unset or not a positive duration: every reservation seam call pays an Access Manager round trip; set AUTH_CACHE_TTL (e.g. 60s)"
	wantNoInversionWarn      = "AUTH_M2M_INVERSION_ENABLED is not true: the Access Manager authorizes application tokens on the reservation seam under a shared editor role instead of the ledger's own client, so only the seam's client allowlist (single-tenant) or ledger client name binding (multi-tenant) restricts who may reserve; set AUTH_M2M_INVERSION_ENABLED=true once the Access Manager supports it"
)

// seamCase is one row of the posture matrix.
type seamCase struct {
	name           string
	deploymentMode string
	tlsMode        string
	pluginAuth     bool
	apiKey         bool
	multiTenant    bool
	allowedClients string
	allowedNames   string
	noInversion    bool
	libAuthMTValue string
	// authCacheTTL is the raw AUTH_CACHE_TTL; empty means "60s" unless
	// noAuthCache is set.
	authCacheTTL string
	noAuthCache  bool
	wantErr      string
	wantWarns    []string
}

// env returns the lib-auth process environment of the row.
func (c seamCase) env() seamPostureEnv {
	libAuthMT := c.libAuthMTValue
	if libAuthMT == "" && c.multiTenant {
		libAuthMT = "true"
	}

	cacheTTL := c.authCacheTTL
	if cacheTTL == "" && !c.noAuthCache {
		cacheTTL = "60s"
	}

	return seamPostureEnv{
		m2mInversion:       !c.noInversion,
		libAuthMultiTenant: libAuthMT,
		authCacheTTL:       cacheTTL,
	}
}

// config returns the Config of the row.
func (c seamCase) config() *Config {
	return &Config{
		DeploymentMode:              c.deploymentMode,
		TracerTLSMode:               c.tlsMode,
		TracerTLSClientAllowedNames: c.allowedNames,
		TracerSeamAllowedClients:    c.allowedClients,
		TracerGRPCPort:              DefaultTracerGRPCPort,
		PluginAuthEnabled:           c.pluginAuth,
		APIKeyEnabled:               c.apiKey,
		MultiTenantEnabled:          c.multiTenant,
	}
}

// TestValidateSeamPosture locks every row of the seam posture matrix that a
// boot can reach, that is every row ValidateAuthPresence lets through: the
// identity ladder (token > API key > transport > none) decides which rules
// apply, SaaS refuses a clear-text or uncached token seam, and every other
// deployment boots with one Warn per exposure. The rows ValidateAuthPresence
// refuses first are in TestValidateSeamPosture_DefenseInDepth.
func TestValidateSeamPosture(t *testing.T) {
	t.Parallel()

	tests := []seamCase{
		// token identity, single-tenant
		{name: "token ST byoc without allowlist is refused", deploymentMode: "byoc", tlsMode: "server", pluginAuth: true, wantErr: wantSeamAllowlistErr},
		{name: "token ST unset deployment without allowlist is refused", tlsMode: "server", pluginAuth: true, wantErr: wantSeamAllowlistErr},
		{name: "token ST saas without allowlist is refused", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, wantErr: wantSeamAllowlistErr},
		{name: "token ST local without allowlist warns", deploymentMode: "local", tlsMode: "server", pluginAuth: true, wantWarns: []string{wantAllowlistLocalWarn}},
		{name: "token ST byoc with allowlist on server boots", deploymentMode: "byoc", tlsMode: "server", pluginAuth: true, allowedClients: "lerian/midaz-ledger"},
		{name: "token ST blank allowlist entries are empty", deploymentMode: "byoc", tlsMode: "server", pluginAuth: true, allowedClients: " , ", wantErr: wantSeamAllowlistErr},

		// token identity, inversion
		{name: "token ST byoc without inversion warns", deploymentMode: "byoc", tlsMode: "server", pluginAuth: true, allowedClients: "c", noInversion: true, wantWarns: []string{wantNoInversionWarn}},
		{name: "token ST unset deployment without inversion warns", tlsMode: "server", pluginAuth: true, allowedClients: "c", noInversion: true, wantWarns: []string{wantNoInversionWarn}},
		{name: "token ST local without inversion warns", deploymentMode: "local", tlsMode: "server", pluginAuth: true, allowedClients: "c", noInversion: true, wantWarns: []string{wantNoInversionWarn}},
		{name: "token ST saas without inversion warns", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, allowedClients: "c", authCacheTTL: "60s", noInversion: true, wantWarns: []string{wantNoInversionWarn}},
		{name: "token MT saas without inversion warns", deploymentMode: "saas", tlsMode: "mesh", pluginAuth: true, multiTenant: true, noInversion: true, wantWarns: []string{wantNoInversionWarn}},
		{name: "token MT byoc without inversion warns", deploymentMode: "byoc", tlsMode: "server", pluginAuth: true, multiTenant: true, noInversion: true, wantWarns: []string{wantNoInversionWarn}},
		{name: "token local without inversion or allowlist warns twice", deploymentMode: "local", tlsMode: "server", pluginAuth: true, noInversion: true, wantWarns: []string{wantNoInversionWarn, wantAllowlistLocalWarn}},
		{name: "token MT without inversion still ignores the allowlist", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, multiTenant: true, allowedClients: "c", noInversion: true, wantWarns: []string{wantNoInversionWarn, wantAllowlistIgnoredWarn}},
		{name: "token ST byoc without inversion or allowlist is refused", deploymentMode: "byoc", tlsMode: "server", pluginAuth: true, noInversion: true, wantErr: wantSeamAllowlistErr},
		{name: "token MT without inversion invisible to lib-auth is refused", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, multiTenant: true, libAuthMTValue: "1", noInversion: true, wantErr: wantLibAuthMTErr},
		{name: "token saas without inversion or decision cache is refused", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, allowedClients: "c", noAuthCache: true, noInversion: true, wantErr: wantAuthCacheSaaSErr},
		{name: "token saas without inversion in clear is refused", deploymentMode: "saas", pluginAuth: true, allowedClients: "c", noInversion: true, wantErr: wantClearSaaSErr},
		{name: "token byoc without inversion, cache or TLS warns three times", deploymentMode: "byoc", pluginAuth: true, allowedClients: "c", noAuthCache: true, noInversion: true, wantWarns: []string{wantNoInversionWarn, wantAuthCacheWarn, wantClearWarn}},

		// token identity, multi-tenant
		{name: "token MT on mesh boots", deploymentMode: "saas", tlsMode: "mesh", pluginAuth: true, multiTenant: true},
		{name: "token MT ignores the allowlist with a warn", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, multiTenant: true, allowedClients: "c", wantWarns: []string{wantAllowlistIgnoredWarn}},
		{name: "token MT invisible to lib-auth is refused", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, multiTenant: true, libAuthMTValue: "1", wantErr: wantLibAuthMTErr},

		// token identity, Access Manager decision cache
		{name: "token MT saas without a decision cache is refused", deploymentMode: "saas", tlsMode: "mesh", pluginAuth: true, multiTenant: true, noAuthCache: true, wantErr: wantAuthCacheSaaSErr},
		{name: "token ST saas with a zero decision cache is refused", deploymentMode: " SaaS ", tlsMode: "server", pluginAuth: true, allowedClients: "c", authCacheTTL: "0s", wantErr: wantAuthCacheSaaSErr},
		{name: "token saas with a negative decision cache is refused", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, allowedClients: "c", authCacheTTL: "-5s", wantErr: wantAuthCacheSaaSErr},
		{name: "token saas with an unparseable decision cache is refused", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, allowedClients: "c", authCacheTTL: "sixty", wantErr: wantAuthCacheSaaSErr},
		{name: "token saas with a padded decision cache boots", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, allowedClients: "c", authCacheTTL: " 30s "},
		{name: "token byoc without a decision cache warns", deploymentMode: "byoc", tlsMode: "server", pluginAuth: true, allowedClients: "c", noAuthCache: true, wantWarns: []string{wantAuthCacheWarn}},
		{name: "token unset deployment without a decision cache warns", tlsMode: "server", pluginAuth: true, allowedClients: "c", noAuthCache: true, wantWarns: []string{wantAuthCacheWarn}},
		{name: "token local without allowlist, cache or TLS warns three times", deploymentMode: "local", pluginAuth: true, noAuthCache: true, wantWarns: []string{wantAllowlistLocalWarn, wantAuthCacheWarn, wantClearWarn}},
		{name: "token saas allowlist refusal precedes the cache refusal", deploymentMode: "saas", tlsMode: "server", pluginAuth: true, noAuthCache: true, wantErr: wantSeamAllowlistErr},
		{name: "API key saas without a decision cache boots", deploymentMode: "saas", tlsMode: "server", apiKey: true, noAuthCache: true},

		// token or API key, transport
		{name: "token in clear on saas is refused", deploymentMode: "saas", pluginAuth: true, allowedClients: "c", wantErr: wantClearSaaSErr},
		{name: "API key in clear on saas is refused", deploymentMode: "saas", apiKey: true, wantErr: wantClearSaaSErr},
		{name: "token in clear on byoc warns", deploymentMode: "byoc", pluginAuth: true, allowedClients: "c", wantWarns: []string{wantClearWarn}},
		{name: "API key in clear on byoc warns", deploymentMode: "byoc", apiKey: true, wantWarns: []string{wantClearWarn}},
		{name: "API key in clear with unset deployment warns", apiKey: true, wantWarns: []string{wantClearWarn}},
		{name: "API key in clear locally warns", deploymentMode: "local", apiKey: true, wantWarns: []string{wantClearWarn}},
		{name: "token ST local without allowlist in clear warns twice", deploymentMode: "local", pluginAuth: true, wantWarns: []string{wantAllowlistLocalWarn, wantClearWarn}},
		{name: "API key on server boots", deploymentMode: "saas", tlsMode: "server", apiKey: true},
		{name: "API key on mtls boots without client allowlist", deploymentMode: "saas", tlsMode: "mtls", apiKey: true},
		{name: "API key on mesh boots", deploymentMode: "byoc", tlsMode: "mesh", apiKey: true},
		{name: "token on mtls on saas boots without client allowlist", deploymentMode: "saas", tlsMode: "mtls", pluginAuth: true, allowedClients: "c"},
		{name: "token wins over API key", deploymentMode: "byoc", tlsMode: "server", pluginAuth: true, apiKey: true, wantErr: wantSeamAllowlistErr},

		// transport identity
		{name: "transport mtls boots", deploymentMode: "byoc", tlsMode: "mtls"},
		{name: "transport mtls with unset deployment boots", tlsMode: "mtls"},
		{name: "transport mesh warns", deploymentMode: "byoc", tlsMode: " Mesh ", wantWarns: []string{wantMeshWarn}},
		{name: "transport mesh locally warns", deploymentMode: "local", tlsMode: "mesh", wantWarns: []string{wantMeshWarn}},

		// no identity
		{name: "none with unset deployment warns", wantWarns: []string{wantNoIdentityWarn}},
		{name: "none with blank deployment warns", deploymentMode: "  ", wantWarns: []string{wantNoIdentityWarn}},
		{name: "none on byoc warns", deploymentMode: "byoc", wantWarns: []string{wantNoIdentityWarn}},
		{name: "none on an undocumented mode warns", deploymentMode: "onprem", wantWarns: []string{wantNoIdentityWarn}},
		{name: "none locally warns", deploymentMode: " Local ", wantWarns: []string{wantNoIdentityWarn}},
		{name: "none on server on byoc warns", deploymentMode: "byoc", tlsMode: "server", wantWarns: []string{wantNoIdentityWarn}},

		// multi-tenant without plugin auth
		{name: "API key in MT is refused", deploymentMode: "saas", tlsMode: "server", apiKey: true, multiTenant: true, wantErr: wantMultiTenantErr},
		{name: "API key in MT on byoc is refused", deploymentMode: "byoc", tlsMode: "server", apiKey: true, multiTenant: true, wantErr: wantMultiTenantErr},

		// unknown TLS mode
		{name: "unknown TLS mode is refused", deploymentMode: "local", tlsMode: "bogus", wantErr: wantInvalidModeErr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := tt.config()
			require.NoError(t, ValidateAuthPresence(t.Context(), cfg, testutil.NewMockLogger()),
				"a matrix row must be one the auth presence gate lets through")

			logger := testutil.NewMockLogger()

			err := validateSeamPosture(t.Context(), cfg, logger, tt.env())

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, logger.Calls, "a refused boot logs nothing")

				return
			}

			require.NoError(t, err)
			require.Len(t, logger.Calls, len(tt.wantWarns), "one Warn per exposure")

			for i, want := range tt.wantWarns {
				assert.Equal(t, "warn", logger.Calls[i].Level)
				assert.Equal(t, want, logger.Calls[i].Message)
			}
		})
	}
}

// TestValidateSeamPosture_DefenseInDepth locks the rows production never
// reaches: ValidateAuthPresence runs first and refuses a saas or multi-tenant
// boot with neither API_KEY_ENABLED nor PLUGIN_AUTH_ENABLED, which is every
// saas transport/none row and every multi-tenant transport/none row. The seam
// gate still refuses them on its own, so a reordered bootstrap cannot open
// the seam.
func TestValidateSeamPosture_DefenseInDepth(t *testing.T) {
	t.Parallel()

	tests := []seamCase{
		{name: "transport mtls saas without client allowlist", deploymentMode: "saas", tlsMode: "mtls", wantErr: wantAllowlistErr},
		{name: "transport padded mtls saas with blank allowlist", deploymentMode: " SaaS ", tlsMode: " MTLS ", allowedNames: " , ", wantErr: wantAllowlistErr},
		{name: "none in clear on saas", deploymentMode: "saas", wantErr: wantNoIdentitySaaSErr},
		{name: "none on server on saas", deploymentMode: "saas", tlsMode: "server", wantErr: wantNoIdentitySaaSErr},
		{name: "none in MT", deploymentMode: "byoc", multiTenant: true, wantErr: wantMultiTenantErr},
		{name: "transport in MT", deploymentMode: "saas", tlsMode: "mtls", allowedNames: "ledger", multiTenant: true, wantErr: wantMultiTenantErr},
		{name: "transport mtls saas with client allowlist", deploymentMode: "saas", tlsMode: "mtls", allowedNames: "ledger.midaz.svc"},
		{name: "transport mesh saas", deploymentMode: "saas", tlsMode: "mesh", wantWarns: []string{wantMeshWarn}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := tt.config()

			presenceErr := ValidateAuthPresence(t.Context(), cfg, testutil.NewMockLogger())
			require.Error(t, presenceErr, "the auth presence gate is the one that fires in production")
			assert.Contains(t, presenceErr.Error(), "API_KEY_ENABLED=true or PLUGIN_AUTH_ENABLED=true")

			logger := testutil.NewMockLogger()
			err := validateSeamPosture(t.Context(), cfg, logger, tt.env())

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
			require.Len(t, logger.Calls, len(tt.wantWarns))

			for i, want := range tt.wantWarns {
				assert.Equal(t, want, logger.Calls[i].Message)
			}
		})
	}
}

// TestValidateSeamPosture_AuthCacheWarnIsOneStructuredLine proves the
// missing-cache Warn is one line carrying the listener and naming the knob.
func TestValidateSeamPosture_AuthCacheWarnIsOneStructuredLine(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := seamCase{deploymentMode: "byoc", tlsMode: "server", pluginAuth: true, allowedClients: "c"}.config()

	require.NoError(t, validateSeamPosture(t.Context(), cfg, logger, seamPostureEnv{m2mInversion: true}))
	require.Len(t, logger.Calls, 1)
	assert.Equal(t, "warn", logger.Calls[0].Level)
	assert.Contains(t, logger.Calls[0].Message, "AUTH_CACHE_TTL")
	assert.Contains(t, logger.Calls[0].Message, "Access Manager round trip")

	fields := map[string]any{}
	for _, f := range logger.Calls[0].Fields {
		fields[f.Key] = f.Value
	}

	assert.Equal(t, DefaultTracerGRPCPort, fields["grpc_port"])
}

// TestValidateSeamPosture_RefusalsNameTheRemedy proves every refusal names
// the variable to set and the accepted values.
func TestValidateSeamPosture_RefusalsNameTheRemedy(t *testing.T) {
	t.Parallel()

	refuse := func(cfg *Config, env seamPostureEnv) string {
		cfg.TracerGRPCPort = DefaultTracerGRPCPort

		err := validateSeamPosture(t.Context(), cfg, testutil.NewMockLogger(), env)
		require.Error(t, err)

		return err.Error()
	}

	inverted := seamPostureEnv{m2mInversion: true, authCacheTTL: "60s"}

	assert.Contains(t, refuse(&Config{DeploymentMode: "byoc", TracerTLSMode: "server", PluginAuthEnabled: true}, inverted), "TRACER_SEAM_ALLOWED_CLIENTS")
	assert.Contains(t, refuse(&Config{DeploymentMode: "saas", PluginAuthEnabled: true, TracerSeamAllowedClients: "c"}, inverted), "TRACER_TLS_MODE")
	assert.Contains(t, refuse(&Config{DeploymentMode: "saas"}, inverted), "API_KEY_ENABLED")
	assert.Contains(t, refuse(&Config{DeploymentMode: "saas", TracerTLSMode: "server", PluginAuthEnabled: true, TracerSeamAllowedClients: "c"}, seamPostureEnv{m2mInversion: true}), "AUTH_CACHE_TTL")
}

// TestValidateSeamPosture_NoIdentityWarnCarriesThePort proves the no-identity
// Warn is one structured line carrying the listener address as a field.
func TestValidateSeamPosture_NoIdentityWarnCarriesThePort(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()

	require.NoError(t, validateSeamPosture(t.Context(), &Config{TracerGRPCPort: ":5021"}, logger, seamPostureEnv{}))
	require.Len(t, logger.Calls, 1)

	fields := map[string]any{}
	for _, f := range logger.Calls[0].Fields {
		fields[f.Key] = f.Value
	}

	assert.Equal(t, ":5021", fields["grpc_port"])
}

// TestValidateSeamPosture_NilConfig proves a nil config is an error, never a
// silent pass.
func TestValidateSeamPosture_NilConfig(t *testing.T) {
	t.Parallel()

	require.Error(t, ValidateSeamPosture(t.Context(), nil, testutil.NewMockLogger()))
}

// TestValidateSeamPosture_ReadsLibAuthEnvironment proves the exported gate
// reads AUTH_M2M_INVERSION_ENABLED, MULTI_TENANT_ENABLED and AUTH_CACHE_TTL
// from the process environment, where lib-auth reads them.
func TestValidateSeamPosture_ReadsLibAuthEnvironment(t *testing.T) {
	t.Setenv("AUTH_CACHE_TTL", "60s")

	cfg := &Config{
		DeploymentMode:           "byoc",
		TracerTLSMode:            "server",
		PluginAuthEnabled:        true,
		TracerSeamAllowedClients: "lerian/midaz-ledger",
		TracerGRPCPort:           DefaultTracerGRPCPort,
	}

	t.Setenv("AUTH_M2M_INVERSION_ENABLED", "")

	logger := testutil.NewMockLogger()
	require.NoError(t, ValidateSeamPosture(t.Context(), cfg, logger))
	require.Len(t, logger.Calls, 1)
	assert.Equal(t, wantNoInversionWarn, logger.Calls[0].Message)

	t.Setenv("AUTH_M2M_INVERSION_ENABLED", "true")

	logger = testutil.NewMockLogger()
	require.NoError(t, ValidateSeamPosture(t.Context(), cfg, logger))
	assert.Empty(t, logger.Calls)

	cfg.MultiTenantEnabled = true
	t.Setenv("MULTI_TENANT_ENABLED", "TRUE")

	err := ValidateSeamPosture(t.Context(), cfg, testutil.NewMockLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), wantLibAuthMTErr)

	cfg.MultiTenantEnabled = false
	cfg.DeploymentMode = "saas"
	t.Setenv("AUTH_CACHE_TTL", "")

	err = ValidateSeamPosture(t.Context(), cfg, testutil.NewMockLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), wantAuthCacheSaaSErr)

	t.Setenv("AUTH_CACHE_TTL", "60s")
	require.NoError(t, ValidateSeamPosture(t.Context(), cfg, testutil.NewMockLogger()))
}

// TestInitCoreInfra_SeamPostureIsWired proves the posture gate runs at boot:
// a single-tenant Access Manager identity without TRACER_SEAM_ALLOWED_CLIENTS
// outside DEPLOYMENT_MODE=local passes every earlier gate and is refused by
// the seam posture gate alone.
func TestInitCoreInfra_SeamPostureIsWired(t *testing.T) {
	t.Setenv("AUTH_M2M_INVERSION_ENABLED", "")

	cfg := &Config{
		LogLevel:          "error",
		TracerGRPCPort:    DefaultTracerGRPCPort,
		DeploymentMode:    "byoc",
		TracerTLSMode:     "server",
		PluginAuthEnabled: true,
		PluginAuthAddress: "http://127.0.0.1:1",
	}

	_, _, _, _, err := initCoreInfra(t.Context(), cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "seam posture: ")
	assert.Contains(t, err.Error(), wantSeamAllowlistErr)
}
