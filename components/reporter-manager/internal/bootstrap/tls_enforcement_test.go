// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/readyz"
)

// validSaaSTLSManagerConfig returns a Manager Config where every DSN uses
// TLS, suitable as the baseline for SaaS-mode enforcement tests.
//
// It builds on top of validManagerConfig() (defined in config_test.go) and
// only flips the TLS-relevant fields. Non-TLS-relevant fields stay identical
// so the regular Validate() pipeline keeps passing.
func validSaaSTLSManagerConfig() *Config {
	cfg := validManagerConfig()
	cfg.DeploymentMode = "saas"
	cfg.MongoURI = "mongodb+srv://cluster.example.net/reporter"
	cfg.RabbitURI = "amqps://reporter-user:secret@rabbitmq.example.com:5671/"
	cfg.RedisTLS = true
	cfg.RedisHost = "valkey.example.com:6380"
	cfg.ObjectStorageEndpoint = "https://s3.example.com"
	cfg.ObjectStorageDisableSSL = false

	return cfg
}

// TestBuildManagerSaaSTLSDeps_BaselineSaaS verifies that a fully TLS-configured
// Manager Config passes ValidateSaaSTLS. This is the minimum sanity check —
// if this fails, every other test in this file is suspect.
func TestBuildManagerSaaSTLSDeps_BaselineSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()

	deps := buildManagerSaaSTLSDeps(cfg)
	require.NotEmpty(t, deps)

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, deps)
	require.NoError(t, err, "valid SaaS config should pass TLS enforcement")
}

// TestBuildManagerSaaSTLSDeps_NonTLSMongoBlocksSaaS verifies that a SaaS
// Manager Config with a non-TLS Mongo URI is rejected by ValidateSaaSTLS, and
// that the error explicitly nominates "mongodb".
func TestBuildManagerSaaSTLSDeps_NonTLSMongoBlocksSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()
	cfg.MongoURI = "mongodb://insecure-host:27017/reporter"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mongodb")
	assert.Contains(t, err.Error(), "DEPLOYMENT_MODE=saas")
}

// TestBuildManagerSaaSTLSDeps_NonTLSRabbitBlocksSaaS verifies that a SaaS
// Manager Config with an amqp:// (non-TLS) URI is rejected and the error
// nominates "rabbitmq".
func TestBuildManagerSaaSTLSDeps_NonTLSRabbitBlocksSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()
	cfg.RabbitURI = "amqp://reporter-user:secret@rabbitmq.example.com:5672/"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rabbitmq")
}

// TestBuildManagerSaaSTLSDeps_RedisTLSFalseBlocksSaaS verifies the Redis
// "Option A" wiring: when REDIS_TLS=false the synthesized URI is "redis://"
// (not "rediss://"), DetectRedisTLS reads the scheme, and the error names
// "redis".
func TestBuildManagerSaaSTLSDeps_RedisTLSFalseBlocksSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()
	cfg.RedisTLS = false
	// Move Mongo/Rabbit/storage to TLS so Redis is the first non-TLS dep.
	cfg.MongoURI = "mongodb+srv://cluster.example.net/reporter"
	cfg.RabbitURI = "amqps://rabbit.example.com:5671/"
	cfg.ObjectStorageEndpoint = "https://s3.example.com"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis")
}

// TestBuildManagerSaaSTLSDeps_StorageHTTPBlocksSaaS verifies that a SaaS
// Manager Config pointing OBJECT_STORAGE_ENDPOINT at an http:// URL is
// rejected, with the error nominating "storage".
func TestBuildManagerSaaSTLSDeps_StorageHTTPBlocksSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()
	cfg.ObjectStorageEndpoint = "http://seaweedfs:8333"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage")
}

// TestBuildManagerSaaSTLSDeps_LocalModeBypassesEnforcement verifies that
// DEPLOYMENT_MODE=local (the default) skips enforcement entirely, even when
// every DSN is plaintext. This protects dev workstations from accidentally
// failing to boot against local docker-compose.
func TestBuildManagerSaaSTLSDeps_LocalModeBypassesEnforcement(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()
	cfg.DeploymentMode = "local"
	cfg.MongoURI = "mongodb://localhost:27017/reporter"
	cfg.RabbitURI = "amqp://localhost:5672/"
	cfg.RedisTLS = false
	cfg.ObjectStorageEndpoint = "http://localhost:8333"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
	require.NoError(t, err, "DEPLOYMENT_MODE=local must NOT enforce TLS")
}

// TestBuildManagerSaaSTLSDeps_BYOCModeBypassesEnforcement verifies that
// DEPLOYMENT_MODE=byoc skips enforcement. BYOC = Bring Your Own Cluster, where
// customers operate their own infra and may choose plaintext intra-VPC links.
func TestBuildManagerSaaSTLSDeps_BYOCModeBypassesEnforcement(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()
	cfg.DeploymentMode = "byoc"
	cfg.MongoURI = "mongodb://customer-host:27017/reporter"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
	require.NoError(t, err, "DEPLOYMENT_MODE=byoc must NOT enforce TLS")
}

// TestBuildManagerSaaSTLSDeps_FetcherEnforcedOnlyWhenEnabled verifies that
// the Fetcher dep is added to the list ONLY when FETCHER_ENABLED=true, so a
// SaaS deployment with Fetcher disabled isn't accidentally blocked by an
// unset/local FETCHER_URL.
func TestBuildManagerSaaSTLSDeps_FetcherEnforcedOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	t.Run("disabled fetcher with http URL is ignored", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSManagerConfig()
		cfg.FetcherEnabled = false
		cfg.FetcherURL = "http://fetcher:4006" // plaintext, but disabled

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
		require.NoError(t, err)
	})

	t.Run("enabled fetcher with http URL blocks SaaS", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSManagerConfig()
		cfg.FetcherEnabled = true
		cfg.FetcherURL = "http://fetcher:4006"

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetcher")
	})

	t.Run("enabled fetcher with https URL passes", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSManagerConfig()
		cfg.FetcherEnabled = true
		cfg.FetcherURL = "https://fetcher.example.com"

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
		require.NoError(t, err)
	})
}

// TestBuildManagerSaaSTLSDeps_MultiTenantRedisOnlyWhenEnabled verifies that
// the multi_tenant_redis dep is only added when MULTI_TENANT_ENABLED=true AND
// MULTI_TENANT_REDIS_HOST is set, so plain SaaS deployments without MT aren't
// blocked by an unset MT-Redis host.
func TestBuildManagerSaaSTLSDeps_MultiTenantRedisOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	t.Run("multi-tenant disabled: MT redis dep absent", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSManagerConfig()
		cfg.MultiTenantEnabled = false
		cfg.MultiTenantRedisHost = "tenant-redis"

		deps := buildManagerSaaSTLSDeps(cfg)
		for _, d := range deps {
			assert.NotEqual(t, "multi_tenant_redis", d.Name,
				"multi_tenant_redis must not be enforced when MT is disabled")
		}
	})

	t.Run("multi-tenant enabled with non-TLS MT redis blocks SaaS", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSManagerConfig()
		cfg.MultiTenantEnabled = true
		cfg.MultiTenantURL = "https://tenant-manager.example.com"
		cfg.MultiTenantServiceAPIKey = "test-key"
		cfg.MultiTenantCircuitBreakerThreshold = 5
		cfg.MultiTenantCircuitBreakerTimeoutSec = 30
		cfg.MultiTenantRedisHost = "mt-redis.example.com"
		cfg.MultiTenantRedisPort = "6379"
		cfg.MultiTenantRedisTLS = false

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "multi_tenant_redis")
	})
}

// TestSynthesizeRedisURI verifies the Manager-side synthesizer used to feed
// DetectRedisTLS:
//
//   - empty host → empty string (so ValidateSaaSTLS skips the dep).
//   - tls=false → "redis://...".
//   - tls=true  → "rediss://...".
//   - whitespace is trimmed.
func TestSynthesizeRedisURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hostPort string
		tls      bool
		want     string
	}{
		{name: "empty host returns empty string", hostPort: "", tls: false, want: ""},
		{name: "whitespace-only host returns empty string", hostPort: "   ", tls: true, want: ""},
		{name: "tls=false yields redis scheme", hostPort: "valkey:6379", tls: false, want: "redis://valkey:6379"},
		{name: "tls=true yields rediss scheme", hostPort: "valkey:6380", tls: true, want: "rediss://valkey:6380"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := synthesizeRedisURI(tt.hostPort, tt.tls)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestJoinHostPort verifies the helper used to combine MULTI_TENANT_REDIS_HOST
// and MULTI_TENANT_REDIS_PORT for the synthesized Redis URI:
//
//   - empty host → empty string.
//   - host already contains ":" → returned as-is.
//   - host without colon + port → "host:port".
//   - empty port → host as-is.
func TestJoinHostPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		port string
		want string
	}{
		{name: "empty host returns empty", host: "", port: "6379", want: ""},
		{name: "host with embedded port returned verbatim", host: "valkey:6380", port: "6379", want: "valkey:6380"},
		{name: "host plus port joined", host: "valkey", port: "6379", want: "valkey:6379"},
		{name: "host without port returned as-is", host: "valkey", port: "", want: "valkey"},
		{name: "whitespace trimmed before join", host: "  valkey  ", port: "  6379  ", want: "valkey:6379"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := joinHostPort(tt.host, tt.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// tenant_manager dep is conditional on multi-tenant mode
// ---------------------------------------------------------------------------
//
// buildManagerSaaSTLSDeps must only append a tenant_manager dep when
// MultiTenantEnabled && MultiTenantURL != "". Otherwise an operator who
// leaves MULTI_TENANT_URL set in env (leftover from a previous deployment)
// but flips MULTI_TENANT_ENABLED=false would have ValidateSaaSTLS enforce
// TLS on a URL the service never calls — blocking SaaS bootstrap.
func TestBuildManagerSaaSTLSDeps_TenantManagerConditional(t *testing.T) {
	t.Parallel()

	t.Run("multi-tenant disabled but URL leftover: dep absent", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSManagerConfig()
		cfg.MultiTenantEnabled = false
		cfg.MultiTenantURL = "http://leftover-tenant-manager.example.com" // non-TLS leftover

		deps := buildManagerSaaSTLSDeps(cfg)
		for _, d := range deps {
			assert.NotEqual(t, "tenant_manager", d.Name,
				"tenant_manager must not be enforced when multi-tenant is disabled")
		}

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, deps)
		require.NoError(t, err,
			"a leftover non-TLS MULTI_TENANT_URL must NOT block SaaS when MULTI_TENANT_ENABLED=false")
	})

	t.Run("multi-tenant enabled with empty URL: dep absent", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSManagerConfig()
		cfg.MultiTenantEnabled = true
		cfg.MultiTenantURL = ""

		deps := buildManagerSaaSTLSDeps(cfg)
		for _, d := range deps {
			assert.NotEqual(t, "tenant_manager", d.Name,
				"tenant_manager must not be enforced when MULTI_TENANT_URL is empty")
		}
	})

	t.Run("multi-tenant enabled with TLS URL: dep present", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSManagerConfig()
		cfg.MultiTenantEnabled = true
		cfg.MultiTenantURL = "https://tenant-manager.example.com"

		deps := buildManagerSaaSTLSDeps(cfg)
		var found bool
		for _, d := range deps {
			if d.Name == "tenant_manager" {
				found = true
				assert.Equal(t, "https://tenant-manager.example.com", d.URI)
			}
		}
		assert.True(t, found, "tenant_manager dep must be present when MT enabled + URL set")
	})
}

// ---------------------------------------------------------------------------
// synthesizeMongoURI / synthesizeRabbitMQURI helpers
// ---------------------------------------------------------------------------
//
// In split-field deployments (MONGO_HOST + MONGO_PORT + ... rather than
// MONGO_URI), buildManagerSaaSTLSDeps must NOT pass an empty MongoURI to
// ValidateSaaSTLS — that would silently skip the dep and leave SaaS
// bootstrap insecure. Same gap for RabbitMQ. The synthesizers build a
// URI from split fields when the composite URI is empty so
// DetectMongoTLS / DetectAMQPTLS can still inspect the scheme.
func TestSynthesizeMongoURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{
			name: "explicit MongoURI returned as-is",
			cfg: &Config{
				MongoURI:    "mongodb+srv://cluster.example.net/reporter",
				MongoDBHost: "ignored.example.com",
			},
			want: "mongodb+srv://cluster.example.net/reporter",
		},
		{
			name: "empty URI + host yields synthesized mongodb URI",
			cfg: &Config{
				MongoURI:        "",
				MongoDBHost:     "mongo.example.com",
				MongoDBPort:     "27017",
				MongoDBUser:     "alice",
				MongoDBPassword: "hunter2",
				MongoDBName:     "reporter",
			},
			want: "mongodb://alice:hunter2@mongo.example.com:27017/reporter",
		},
		{
			name: "empty URI + host without credentials",
			cfg: &Config{
				MongoURI:    "",
				MongoDBHost: "mongo.example.com",
				MongoDBPort: "27017",
				MongoDBName: "reporter",
			},
			want: "mongodb://mongo.example.com:27017/reporter",
		},
		{
			name: "empty URI + empty host returns empty string",
			cfg: &Config{
				MongoURI:    "",
				MongoDBHost: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := synthesizeMongoURI(tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSynthesizeRabbitMQURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{
			name: "explicit RabbitURI returned as-is",
			cfg: &Config{
				RabbitURI:    "amqps://user:pass@rabbit.example.com:5671/",
				RabbitMQHost: "ignored.example.com",
			},
			want: "amqps://user:pass@rabbit.example.com:5671/",
		},
		{
			name: "empty URI + host yields synthesized amqp URI",
			cfg: &Config{
				RabbitURI:        "",
				RabbitMQHost:     "rabbit.example.com",
				RabbitMQPortAMQP: "5672",
				RabbitMQUser:     "guest",
				RabbitMQPass:     "guest",
			},
			want: "amqp://guest:guest@rabbit.example.com:5672/",
		},
		{
			name: "empty URI + empty host returns empty string",
			cfg: &Config{
				RabbitURI:    "",
				RabbitMQHost: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := synthesizeRabbitMQURI(tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildManagerSaaSTLSDeps_SplitFieldMongo verifies that when MongoURI is
// empty but split fields (MongoDBHost/Port/...) are populated, the synthesized
// URI gets fed to ValidateSaaSTLS — the previous behavior silently skipped
// the dep, leaving SaaS bootstrap insecure.
func TestBuildManagerSaaSTLSDeps_SplitFieldMongo(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()
	// Wipe MongoURI; rely on split fields. mongodb:// (non-TLS) → must block.
	cfg.MongoURI = ""
	cfg.MongoDBHost = "mongo.internal"
	cfg.MongoDBPort = "27017"
	cfg.MongoDBName = "reporter"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
	require.Error(t, err,
		"non-TLS Mongo via split fields must block SaaS — previously silently allowed")
	assert.Contains(t, err.Error(), "mongodb")
}

// TestBuildManagerSaaSTLSDeps_SplitFieldRabbit verifies the analogous gap
// for RabbitMQ split-field deployments.
func TestBuildManagerSaaSTLSDeps_SplitFieldRabbit(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSManagerConfig()
	cfg.RabbitURI = ""
	cfg.RabbitMQHost = "rabbit.internal"
	cfg.RabbitMQPortAMQP = "5672"
	cfg.RabbitMQUser = "guest"
	cfg.RabbitMQPass = "guest"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildManagerSaaSTLSDeps(cfg))
	require.Error(t, err,
		"non-TLS RabbitMQ via split fields must block SaaS — previously silently allowed")
	assert.Contains(t, err.Error(), "rabbitmq")
}

// TestSynthesizeMongoURI_PreservesParameters verifies that MONGO_PARAMETERS
// is appended to the synthesized URI as a query string. Operators in
// split-field deployments rely on MONGO_PARAMETERS=tls=true&authSource=admin
// to declare TLS posture; if synthesizeMongoURI drops it, ValidateSaaSTLS
// inspects a non-TLS URI and rejects a deployment that is in fact TLS-secure
// at runtime.
func TestSynthesizeMongoURI_PreservesParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{
			name: "params appended when split-field URI is synthesized",
			cfg: &Config{
				MongoURI:          "",
				MongoDBHost:       "mongo.example.com",
				MongoDBPort:       "27017",
				MongoDBName:       "reporter",
				MongoDBParameters: "tls=true&authSource=admin",
			},
			want: "mongodb://mongo.example.com:27017/reporter?tls=true&authSource=admin",
		},
		{
			name: "params appended without dbname",
			cfg: &Config{
				MongoURI:          "",
				MongoDBHost:       "mongo.example.com",
				MongoDBPort:       "27017",
				MongoDBParameters: "tls=true",
			},
			want: "mongodb://mongo.example.com:27017?tls=true",
		},
		{
			name: "no params yields URI without query string",
			cfg: &Config{
				MongoURI:    "",
				MongoDBHost: "mongo.example.com",
				MongoDBPort: "27017",
				MongoDBName: "reporter",
			},
			want: "mongodb://mongo.example.com:27017/reporter",
		},
		{
			name: "params ignored when MongoURI is set explicitly",
			cfg: &Config{
				MongoURI:          "mongodb+srv://cluster.example.net/reporter",
				MongoDBParameters: "tls=true",
			},
			want: "mongodb+srv://cluster.example.net/reporter",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := synthesizeMongoURI(tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSynthesizeMongoURI_TLSDetectionWithParameters confirms that the
// synthesized URI round-trips through DetectMongoTLS — that is, when an
// operator declares TLS via MONGO_PARAMETERS, ValidateSaaSTLS correctly
// reports the deployment as TLS-enforced.
func TestSynthesizeMongoURI_TLSDetectionWithParameters(t *testing.T) {
	t.Parallel()

	withTLS := &Config{
		MongoURI:          "",
		MongoDBHost:       "mongo.example.com",
		MongoDBName:       "reporter",
		MongoDBParameters: "tls=true&authSource=admin",
	}
	uri := synthesizeMongoURI(withTLS)

	got, err := readyz.DetectMongoTLS(uri)
	require.NoError(t, err)
	assert.True(t, got, "tls=true in MongoDBParameters MUST surface as TLS-enforced")

	withoutTLS := &Config{
		MongoURI:    "",
		MongoDBHost: "mongo.example.com",
		MongoDBName: "reporter",
	}
	uri = synthesizeMongoURI(withoutTLS)

	got, err = readyz.DetectMongoTLS(uri)
	require.NoError(t, err)
	assert.False(t, got, "no TLS query param means not TLS-enforced")
}

// TestSynthesizeRabbitMQURI_HonorsTLSFlag verifies that RABBITMQ_TLS=true
// flips the synthesized scheme from amqp:// to amqps://. Operators using
// split fields (RABBITMQ_HOST + RABBITMQ_PORT_AMQP + RABBITMQ_TLS=true)
// otherwise get blocked by SaaS enforcement against a synthesized amqp://
// URI even though their runtime connection is amqps://.
func TestSynthesizeRabbitMQURI_HonorsTLSFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        *Config
		wantPrefix string
	}{
		{
			name: "RABBITMQ_TLS=true emits amqps scheme",
			cfg: &Config{
				RabbitURI:        "",
				RabbitMQHost:     "rabbit.example.com",
				RabbitMQPortAMQP: "5671",
				RabbitMQTLS:      true,
			},
			wantPrefix: "amqps://",
		},
		{
			name: "RABBITMQ_TLS=false emits amqp scheme",
			cfg: &Config{
				RabbitURI:        "",
				RabbitMQHost:     "rabbit.example.com",
				RabbitMQPortAMQP: "5672",
				RabbitMQTLS:      false,
			},
			wantPrefix: "amqp://",
		},
		{
			name: "explicit RabbitURI overrides TLS flag",
			cfg: &Config{
				RabbitURI:    "amqp://elsewhere.example.com:5672/",
				RabbitMQHost: "ignored.example.com",
				RabbitMQTLS:  true,
			},
			wantPrefix: "amqp://",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := synthesizeRabbitMQURI(tt.cfg)
			assert.True(t, strings.HasPrefix(got, tt.wantPrefix),
				"want prefix %q, got %q", tt.wantPrefix, got)
		})
	}
}

// TestSynthesizeMongoURI_PercentEncodesCredentials verifies that when MongoDB
// user/password contain reserved URI characters (@, :, /, ?), the synthesized
// URI percent-encodes them so url.Parse re-tokenizes the URI correctly. Raw
// concatenation produces a URI like "mongodb://user:p@ss@host:27017" which
// url.Parse treats the first '@' as the userinfo separator, leaving
// "ss@host:27017" as the host. DetectMongoTLS would then either fail to parse
// or misclassify TLS posture, blocking a SaaS deployment whose actual
// connection (using split fields directly) would succeed.
func TestSynthesizeMongoURI_PercentEncodesCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      *Config
		wantHost string
		wantUser string // optional decoded user check
		wantPass string // optional decoded password check
	}{
		{
			name: "password with @ sign",
			cfg: &Config{
				MongoDBHost:     "mongo.example.com",
				MongoDBPort:     "27017",
				MongoDBUser:     "reporter",
				MongoDBPassword: "p@ss",
				MongoDBName:     "reporter",
			},
			wantHost: "mongo.example.com:27017",
			wantUser: "reporter",
			wantPass: "p@ss",
		},
		{
			name: "password with colon",
			cfg: &Config{
				MongoDBHost:     "mongo.example.com",
				MongoDBPort:     "27017",
				MongoDBUser:     "reporter",
				MongoDBPassword: "se:cret",
				MongoDBName:     "reporter",
			},
			wantHost: "mongo.example.com:27017",
			wantUser: "reporter",
			wantPass: "se:cret",
		},
		{
			name: "password with forward slash",
			cfg: &Config{
				MongoDBHost:     "mongo.example.com",
				MongoDBPort:     "27017",
				MongoDBUser:     "reporter",
				MongoDBPassword: "he/llo",
				MongoDBName:     "reporter",
			},
			wantHost: "mongo.example.com:27017",
			wantUser: "reporter",
			wantPass: "he/llo",
		},
		{
			name: "password with question mark",
			cfg: &Config{
				MongoDBHost:     "mongo.example.com",
				MongoDBPort:     "27017",
				MongoDBUser:     "reporter",
				MongoDBPassword: "qu?ery",
				MongoDBName:     "reporter",
			},
			wantHost: "mongo.example.com:27017",
			wantUser: "reporter",
			wantPass: "qu?ery",
		},
		{
			name: "user with @ sign",
			cfg: &Config{
				MongoDBHost:     "mongo.example.com",
				MongoDBPort:     "27017",
				MongoDBUser:     "us@er",
				MongoDBPassword: "pass",
				MongoDBName:     "reporter",
			},
			wantHost: "mongo.example.com:27017",
			wantUser: "us@er",
			wantPass: "pass",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uri := synthesizeMongoURI(tt.cfg)

			parsed, err := url.Parse(uri)
			require.NoError(t, err, "synthesized URI must parse cleanly: %q", uri)
			assert.Equal(t, tt.wantHost, parsed.Host,
				"host must be preserved when credentials contain reserved chars")

			require.NotNil(t, parsed.User, "userinfo must be present")
			assert.Equal(t, tt.wantUser, parsed.User.Username(),
				"user must round-trip through percent decoding")

			pwd, ok := parsed.User.Password()
			require.True(t, ok, "password must be present")
			assert.Equal(t, tt.wantPass, pwd,
				"password must round-trip through percent decoding")

			// Round-trip through DetectMongoTLS so SaaS enforcement classifies
			// the synthesized URI without parse errors.
			_, err = readyz.DetectMongoTLS(uri)
			require.NoError(t, err, "DetectMongoTLS must accept the synthesized URI")
		})
	}
}

// TestSynthesizeRabbitMQURI_PercentEncodesCredentials is the AMQP analogue:
// reserved characters in RabbitMQ user/password must be percent-encoded so
// the synthesized URI parses cleanly and DetectAMQPTLS reads the scheme
// rather than tripping on a malformed URI.
func TestSynthesizeRabbitMQURI_PercentEncodesCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      *Config
		wantHost string
		wantUser string
		wantPass string
	}{
		{
			name: "password with @ sign",
			cfg: &Config{
				RabbitMQHost:     "rabbit.example.com",
				RabbitMQPortAMQP: "5672",
				RabbitMQUser:     "reporter",
				RabbitMQPass:     "p@ss",
			},
			wantHost: "rabbit.example.com:5672",
			wantUser: "reporter",
			wantPass: "p@ss",
		},
		{
			name: "password with colon",
			cfg: &Config{
				RabbitMQHost:     "rabbit.example.com",
				RabbitMQPortAMQP: "5672",
				RabbitMQUser:     "reporter",
				RabbitMQPass:     "se:cret",
			},
			wantHost: "rabbit.example.com:5672",
			wantUser: "reporter",
			wantPass: "se:cret",
		},
		{
			name: "password with forward slash",
			cfg: &Config{
				RabbitMQHost:     "rabbit.example.com",
				RabbitMQPortAMQP: "5672",
				RabbitMQUser:     "reporter",
				RabbitMQPass:     "he/llo",
			},
			wantHost: "rabbit.example.com:5672",
			wantUser: "reporter",
			wantPass: "he/llo",
		},
		{
			name: "password with question mark",
			cfg: &Config{
				RabbitMQHost:     "rabbit.example.com",
				RabbitMQPortAMQP: "5672",
				RabbitMQUser:     "reporter",
				RabbitMQPass:     "qu?ery",
			},
			wantHost: "rabbit.example.com:5672",
			wantUser: "reporter",
			wantPass: "qu?ery",
		},
		{
			name: "user with @ sign",
			cfg: &Config{
				RabbitMQHost:     "rabbit.example.com",
				RabbitMQPortAMQP: "5672",
				RabbitMQUser:     "us@er",
				RabbitMQPass:     "pass",
			},
			wantHost: "rabbit.example.com:5672",
			wantUser: "us@er",
			wantPass: "pass",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uri := synthesizeRabbitMQURI(tt.cfg)

			parsed, err := url.Parse(uri)
			require.NoError(t, err, "synthesized URI must parse cleanly: %q", uri)
			assert.Equal(t, tt.wantHost, parsed.Host,
				"host must be preserved when credentials contain reserved chars")

			require.NotNil(t, parsed.User, "userinfo must be present")
			assert.Equal(t, tt.wantUser, parsed.User.Username(),
				"user must round-trip through percent decoding")

			pwd, ok := parsed.User.Password()
			require.True(t, ok, "password must be present")
			assert.Equal(t, tt.wantPass, pwd,
				"password must round-trip through percent decoding")

			// Round-trip through DetectAMQPTLS so SaaS enforcement classifies
			// the synthesized URI without parse errors.
			_, err = readyz.DetectAMQPTLS(uri)
			require.NoError(t, err, "DetectAMQPTLS must accept the synthesized URI")
		})
	}
}

// TestSynthesizeRabbitMQURI_TLSDetectionRoundtrip confirms the synthesized
// URI is correctly classified by DetectAMQPTLS so SaaS enforcement does
// not falsely reject a TLS-enabled split-field deployment.
func TestSynthesizeRabbitMQURI_TLSDetectionRoundtrip(t *testing.T) {
	t.Parallel()

	withTLS := &Config{
		RabbitURI:        "",
		RabbitMQHost:     "rabbit.example.com",
		RabbitMQPortAMQP: "5671",
		RabbitMQTLS:      true,
	}
	uri := synthesizeRabbitMQURI(withTLS)

	got, err := readyz.DetectAMQPTLS(uri)
	require.NoError(t, err)
	assert.True(t, got, "RABBITMQ_TLS=true MUST surface as TLS-enforced")

	withoutTLS := &Config{
		RabbitURI:        "",
		RabbitMQHost:     "rabbit.example.com",
		RabbitMQPortAMQP: "5672",
		RabbitMQTLS:      false,
	}
	uri = synthesizeRabbitMQURI(withoutTLS)

	got, err = readyz.DetectAMQPTLS(uri)
	require.NoError(t, err)
	assert.False(t, got, "RABBITMQ_TLS=false MUST surface as non-TLS")
}
