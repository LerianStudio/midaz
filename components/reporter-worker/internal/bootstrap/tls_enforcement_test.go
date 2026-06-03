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

	"github.com/LerianStudio/midaz/v3/pkg/reporter/readyz"
)

// validSaaSTLSWorkerConfig returns a Worker Config where every DSN uses TLS,
// suitable as the baseline for SaaS-mode enforcement tests.
//
// Worker has no Validate() pipeline as strict as the Manager, but we still
// populate enough fields that the produced config is realistic. Only the
// TLS-relevant fields actually drive the enforcement decision.
func validSaaSTLSWorkerConfig() *Config {
	return &Config{
		DeploymentMode:        "saas",
		MongoURI:              "mongodb+srv://cluster.example.net/reporter",
		RabbitURI:             "amqps://reporter-user:secret@rabbitmq.example.com:5671/",
		RedisHost:             "valkey.example.com:6380",
		RedisTLS:              true,
		ObjectStorageEndpoint: "https://s3.example.com",
		MultiTenantURL:        "", // not enforced unless set
	}
}

// TestBuildWorkerSaaSTLSDeps_BaselineSaaS verifies that a fully TLS-configured
// Worker Config passes ValidateSaaSTLS.
func TestBuildWorkerSaaSTLSDeps_BaselineSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()

	deps := buildWorkerSaaSTLSDeps(cfg)
	require.NotEmpty(t, deps)

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, deps)
	require.NoError(t, err, "valid SaaS worker config should pass TLS enforcement")
}

// TestBuildWorkerSaaSTLSDeps_NonTLSMongoBlocksSaaS verifies that a SaaS Worker
// Config with a non-TLS Mongo URI is rejected and the error names "mongodb".
func TestBuildWorkerSaaSTLSDeps_NonTLSMongoBlocksSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()
	cfg.MongoURI = "mongodb://insecure-host:27017/reporter"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mongodb")
	assert.Contains(t, err.Error(), "DEPLOYMENT_MODE=saas")
}

// TestBuildWorkerSaaSTLSDeps_NonTLSRabbitBlocksSaaS verifies that a SaaS
// Worker Config with an amqp:// URI is rejected and the error names "rabbitmq".
func TestBuildWorkerSaaSTLSDeps_NonTLSRabbitBlocksSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()
	cfg.RabbitURI = "amqp://reporter-user:secret@rabbitmq.example.com:5672/"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rabbitmq")
}

// TestBuildWorkerSaaSTLSDeps_RedisTLSFalseBlocksSaaS verifies the Redis
// "Option A" wiring on the Worker side. The Worker only opens a Redis
// connection when Fetcher or multi-tenant is enabled, so the test must
// turn one of those flags on for Redis to be in the enforcement list.
func TestBuildWorkerSaaSTLSDeps_RedisTLSFalseBlocksSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()
	cfg.FetcherEnabled = true
	cfg.FetcherURL = "https://fetcher.example.com"
	cfg.RedisTLS = false
	// Mongo & Rabbit & storage already TLS in baseline; Redis is the first
	// non-TLS dep we hit (now reachable because Fetcher is enabled).

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis")
}

// TestBuildWorkerSaaSTLSDeps_StorageHTTPBlocksSaaS verifies that an http://
// OBJECT_STORAGE_ENDPOINT is rejected on the Worker side.
func TestBuildWorkerSaaSTLSDeps_StorageHTTPBlocksSaaS(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()
	cfg.ObjectStorageEndpoint = "http://seaweedfs:8333"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage")
}

// TestBuildWorkerSaaSTLSDeps_LocalAndBYOCBypassEnforcement verifies that
// non-saas modes never enforce TLS, even with all-plaintext DSNs.
func TestBuildWorkerSaaSTLSDeps_LocalAndBYOCBypassEnforcement(t *testing.T) {
	t.Parallel()

	tests := []string{"local", "byoc", "", "DEV"}

	for _, mode := range tests {
		mode := mode
		t.Run("mode="+mode, func(t *testing.T) {
			t.Parallel()

			cfg := validSaaSTLSWorkerConfig()
			cfg.DeploymentMode = mode
			cfg.MongoURI = "mongodb://localhost:27017/reporter"
			cfg.RabbitURI = "amqp://localhost:5672/"
			cfg.RedisTLS = false
			cfg.ObjectStorageEndpoint = "http://localhost:8333"

			err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
			require.NoError(t, err, "mode %q must not enforce TLS", mode)
		})
	}
}

// TestBuildWorkerSaaSTLSDeps_FetcherEnforcedOnlyWhenEnabled verifies that
// FETCHER_ENABLED gates both the Fetcher upstream URL and the Fetcher S3
// storage endpoint in the dep list.
func TestBuildWorkerSaaSTLSDeps_FetcherEnforcedOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	t.Run("disabled fetcher with http URLs is ignored", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSWorkerConfig()
		cfg.FetcherEnabled = false
		cfg.FetcherURL = "http://fetcher:4006"
		cfg.FetcherStorageEndpoint = "http://fetcher-s3:8333"

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
		require.NoError(t, err)
	})

	t.Run("enabled fetcher with http upstream blocks SaaS", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSWorkerConfig()
		cfg.FetcherEnabled = true
		cfg.FetcherURL = "http://fetcher:4006"
		cfg.FetcherStorageEndpoint = "https://fetcher-s3.example.com"

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetcher")
	})

	t.Run("enabled fetcher with http storage blocks SaaS", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSWorkerConfig()
		cfg.FetcherEnabled = true
		cfg.FetcherURL = "https://fetcher.example.com"
		cfg.FetcherStorageEndpoint = "http://fetcher-s3:8333"

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetcher_storage")
	})

	t.Run("enabled fetcher with all https passes", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSWorkerConfig()
		cfg.FetcherEnabled = true
		cfg.FetcherURL = "https://fetcher.example.com"
		cfg.FetcherStorageEndpoint = "https://fetcher-s3.example.com"

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
		require.NoError(t, err)
	})
}

// TestBuildWorkerSaaSTLSDeps_MultiTenantRedisOnlyWhenEnabled verifies that
// MT-Redis enforcement only fires when both MT is enabled AND a MT-Redis host
// is configured.
func TestBuildWorkerSaaSTLSDeps_MultiTenantRedisOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	t.Run("MT disabled: MT redis dep absent", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSWorkerConfig()
		cfg.MultiTenantEnabled = false
		cfg.MultiTenantRedisHost = "tenant-redis"

		deps := buildWorkerSaaSTLSDeps(cfg)
		for _, d := range deps {
			assert.NotEqual(t, "multi_tenant_redis", d.Name)
		}
	})

	t.Run("MT enabled with non-TLS redis blocks SaaS", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSWorkerConfig()
		cfg.MultiTenantEnabled = true
		cfg.MultiTenantURL = "https://tenant-manager.example.com"
		cfg.MultiTenantRedisHost = "mt-redis.example.com"
		cfg.MultiTenantRedisPort = "6379"
		cfg.MultiTenantRedisTLS = false

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "multi_tenant_redis")
	})

	t.Run("MT enabled with TLS redis passes", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSWorkerConfig()
		cfg.MultiTenantEnabled = true
		cfg.MultiTenantURL = "https://tenant-manager.example.com"
		cfg.MultiTenantRedisHost = "mt-redis.example.com"
		cfg.MultiTenantRedisPort = "6380"
		cfg.MultiTenantRedisTLS = true

		err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
		require.NoError(t, err)
	})
}

// TestSynthesizeWorkerRedisURI mirrors TestSynthesizeRedisURI on the Manager
// side, verifying the Worker-local helper.
func TestSynthesizeWorkerRedisURI(t *testing.T) {
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

			got := synthesizeWorkerRedisURI(tt.hostPort, tt.tls)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestJoinWorkerHostPort verifies the Worker-local host/port joining helper.
func TestJoinWorkerHostPort(t *testing.T) {
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

			got := joinWorkerHostPort(tt.host, tt.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// tenant_manager dep is conditional on multi-tenant mode
// ---------------------------------------------------------------------------
//
// buildWorkerSaaSTLSDeps must only append a tenant_manager dep when
// MultiTenantEnabled && MultiTenantURL != "". A leftover MULTI_TENANT_URL
// in env paired with MULTI_TENANT_ENABLED=false must NOT block SaaS
// bootstrap.
func TestBuildWorkerSaaSTLSDeps_TenantManagerConditional(t *testing.T) {
	t.Parallel()

	t.Run("multi-tenant disabled but URL leftover: dep absent", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSWorkerConfig()
		cfg.MultiTenantEnabled = false
		cfg.MultiTenantURL = "http://leftover-tenant-manager.example.com"

		deps := buildWorkerSaaSTLSDeps(cfg)
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

		cfg := validSaaSTLSWorkerConfig()
		cfg.MultiTenantEnabled = true
		cfg.MultiTenantURL = ""

		deps := buildWorkerSaaSTLSDeps(cfg)
		for _, d := range deps {
			assert.NotEqual(t, "tenant_manager", d.Name,
				"tenant_manager must not be enforced when MULTI_TENANT_URL is empty")
		}
	})

	t.Run("multi-tenant enabled with TLS URL: dep present", func(t *testing.T) {
		t.Parallel()

		cfg := validSaaSTLSWorkerConfig()
		cfg.MultiTenantEnabled = true
		cfg.MultiTenantURL = "https://tenant-manager.example.com"

		deps := buildWorkerSaaSTLSDeps(cfg)
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
// synthesizeWorkerMongoURI / synthesizeWorkerRabbitMQURI helpers
// ---------------------------------------------------------------------------
//
// In split-field deployments (MONGO_HOST + MONGO_PORT + ... rather than
// MONGO_URI), buildWorkerSaaSTLSDeps must NOT pass an empty MongoURI to
// ValidateSaaSTLS — that would silently skip the dep and leave SaaS
// bootstrap insecure. Same gap for RabbitMQ. The synthesizers build a URI
// from split fields when the composite URI is empty.
func TestSynthesizeWorkerMongoURI(t *testing.T) {
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

			got := synthesizeWorkerMongoURI(tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSynthesizeWorkerRabbitMQURI(t *testing.T) {
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

			got := synthesizeWorkerRabbitMQURI(tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildWorkerSaaSTLSDeps_SplitFieldMongo verifies that when MongoURI is
// empty but split fields (MongoDBHost/Port/...) are populated, the synthesized
// URI is fed to ValidateSaaSTLS — previously the dep was silently skipped,
// leaving SaaS bootstrap insecure.
func TestBuildWorkerSaaSTLSDeps_SplitFieldMongo(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()
	cfg.MongoURI = ""
	cfg.MongoDBHost = "mongo.internal"
	cfg.MongoDBPort = "27017"
	cfg.MongoDBName = "reporter"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
	require.Error(t, err,
		"non-TLS Mongo via split fields must block SaaS")
	assert.Contains(t, err.Error(), "mongodb")
}

// TestBuildWorkerSaaSTLSDeps_SplitFieldRabbit verifies the analogous gap
// for RabbitMQ split-field deployments.
func TestBuildWorkerSaaSTLSDeps_SplitFieldRabbit(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()
	cfg.RabbitURI = ""
	cfg.RabbitMQHost = "rabbit.internal"
	cfg.RabbitMQPortAMQP = "5672"
	cfg.RabbitMQUser = "guest"
	cfg.RabbitMQPass = "guest"

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, buildWorkerSaaSTLSDeps(cfg))
	require.Error(t, err,
		"non-TLS RabbitMQ via split fields must block SaaS")
	assert.Contains(t, err.Error(), "rabbitmq")
}

// TestSynthesizeWorkerMongoURI_PreservesParameters verifies the worker's
// synthesizer appends MongoDBParameters as a query string, mirroring the
// Manager fix. Operators relying on MONGO_PARAMETERS=tls=true must not be
// rejected by SaaS enforcement against a synthesized URI that strips the
// posture indicator.
func TestSynthesizeWorkerMongoURI_PreservesParameters(t *testing.T) {
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

			got := synthesizeWorkerMongoURI(tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSynthesizeWorkerMongoURI_TLSDetectionWithParameters confirms the
// synthesized URI round-trips through DetectMongoTLS so the worker's
// SaaS bootstrap honors operator-declared TLS posture.
func TestSynthesizeWorkerMongoURI_TLSDetectionWithParameters(t *testing.T) {
	t.Parallel()

	withTLS := &Config{
		MongoURI:          "",
		MongoDBHost:       "mongo.example.com",
		MongoDBName:       "reporter",
		MongoDBParameters: "tls=true",
	}
	uri := synthesizeWorkerMongoURI(withTLS)

	got, err := readyz.DetectMongoTLS(uri)
	require.NoError(t, err)
	assert.True(t, got, "tls=true in MongoDBParameters MUST surface as TLS-enforced")

	withoutTLS := &Config{
		MongoURI:    "",
		MongoDBHost: "mongo.example.com",
		MongoDBName: "reporter",
	}
	uri = synthesizeWorkerMongoURI(withoutTLS)

	got, err = readyz.DetectMongoTLS(uri)
	require.NoError(t, err)
	assert.False(t, got, "no TLS query param means not TLS-enforced")
}

// TestSynthesizeWorkerRabbitMQURI_HonorsTLSFlag verifies the worker's
// synthesizer flips between amqp:// and amqps:// based on RabbitMQTLS.
func TestSynthesizeWorkerRabbitMQURI_HonorsTLSFlag(t *testing.T) {
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

			got := synthesizeWorkerRabbitMQURI(tt.cfg)
			assert.True(t, strings.HasPrefix(got, tt.wantPrefix),
				"want prefix %q, got %q", tt.wantPrefix, got)
		})
	}
}

// TestBuildWorkerSaaSTLSDeps_RedisOmittedWhenNotRequired verifies that the
// Worker's Redis dep is excluded from SaaS enforcement when both Fetcher
// and multi-tenant are disabled. The Worker does not open a Redis
// connection in this configuration, so a leftover plaintext REDIS_HOST in
// the environment must not block SaaS bootstrap.
//
// The decision mirrors BuildWorkerCheckers in health-server.go where
// redisRequired = FetcherEnabled || MultiTenantEnabled gates the
// RedisChecker as required.
func TestBuildWorkerSaaSTLSDeps_RedisOmittedWhenNotRequired(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()
	cfg.FetcherEnabled = false
	cfg.MultiTenantEnabled = false
	cfg.RedisTLS = false // would block if redis were enforced
	cfg.RedisHost = "redis.example.com"

	deps := buildWorkerSaaSTLSDeps(cfg)
	for _, dep := range deps {
		assert.NotEqual(t, "redis", dep.Name,
			"redis must be excluded from SaaS deps when neither fetcher nor multi-tenant is enabled")
	}

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, deps)
	require.NoError(t, err,
		"plaintext redis must not block SaaS bootstrap when worker does not open a redis connection")
}

// TestBuildWorkerSaaSTLSDeps_RedisRequiredWhenFetcherEnabled is the
// counterpart: with Fetcher enabled the worker opens Redis (reconciler
// distributed lock), so plaintext Redis MUST block SaaS bootstrap.
func TestBuildWorkerSaaSTLSDeps_RedisRequiredWhenFetcherEnabled(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()
	cfg.FetcherEnabled = true
	cfg.FetcherURL = "https://fetcher.example.com"
	cfg.MultiTenantEnabled = false
	cfg.RedisTLS = false

	deps := buildWorkerSaaSTLSDeps(cfg)

	var found bool

	for _, dep := range deps {
		if dep.Name == "redis" {
			found = true
			break
		}
	}

	assert.True(t, found, "redis must be present in SaaS deps when fetcher is enabled")

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis")
}

// TestBuildWorkerSaaSTLSDeps_RedisRequiredWhenMultiTenantEnabled covers
// the second activation path: with multi-tenant enabled the worker opens
// Redis (per-tenant client + tenant event-listener), so plaintext Redis
// MUST block SaaS bootstrap.
func TestBuildWorkerSaaSTLSDeps_RedisRequiredWhenMultiTenantEnabled(t *testing.T) {
	t.Parallel()

	cfg := validSaaSTLSWorkerConfig()
	cfg.FetcherEnabled = false
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = "https://tenant-manager.example.com"
	cfg.MultiTenantRedisHost = "tenant-redis.example.com"
	cfg.MultiTenantRedisTLS = true
	cfg.RedisTLS = false

	deps := buildWorkerSaaSTLSDeps(cfg)

	var found bool

	for _, dep := range deps {
		if dep.Name == "redis" {
			found = true
			break
		}
	}

	assert.True(t, found, "redis must be present in SaaS deps when multi-tenant is enabled")

	err := readyz.ValidateSaaSTLS(cfg.DeploymentMode, deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis")
}

// TestSynthesizeWorkerMongoURI_PercentEncodesCredentials verifies that when
// MongoDB user/password contain reserved URI characters (@, :, /, ?), the
// synthesized URI percent-encodes them so url.Parse re-tokenizes the URI
// correctly. Raw concatenation produces a URI like
// "mongodb://user:p@ss@host:27017" which url.Parse treats the first '@' as
// the userinfo separator, leaving "ss@host:27017" as the host. DetectMongoTLS
// would then either fail to parse or misclassify TLS posture, blocking a
// SaaS deployment whose actual connection (using split fields directly) would
// succeed.
func TestSynthesizeWorkerMongoURI_PercentEncodesCredentials(t *testing.T) {
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

			uri := synthesizeWorkerMongoURI(tt.cfg)

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

			_, err = readyz.DetectMongoTLS(uri)
			require.NoError(t, err, "DetectMongoTLS must accept the synthesized URI")
		})
	}
}

// TestSynthesizeWorkerRabbitMQURI_PercentEncodesCredentials is the AMQP
// analogue: reserved characters in RabbitMQ user/password must be
// percent-encoded so the synthesized URI parses cleanly and DetectAMQPTLS
// reads the scheme rather than tripping on a malformed URI.
func TestSynthesizeWorkerRabbitMQURI_PercentEncodesCredentials(t *testing.T) {
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

			uri := synthesizeWorkerRabbitMQURI(tt.cfg)

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

			_, err = readyz.DetectAMQPTLS(uri)
			require.NoError(t, err, "DetectAMQPTLS must accept the synthesized URI")
		})
	}
}

// TestSynthesizeWorkerRabbitMQURI_TLSDetectionRoundtrip confirms the
// synthesized URI is correctly classified by DetectAMQPTLS.
func TestSynthesizeWorkerRabbitMQURI_TLSDetectionRoundtrip(t *testing.T) {
	t.Parallel()

	withTLS := &Config{
		RabbitURI:        "",
		RabbitMQHost:     "rabbit.example.com",
		RabbitMQPortAMQP: "5671",
		RabbitMQTLS:      true,
	}
	uri := synthesizeWorkerRabbitMQURI(withTLS)

	got, err := readyz.DetectAMQPTLS(uri)
	require.NoError(t, err)
	assert.True(t, got, "RABBITMQ_TLS=true MUST surface as TLS-enforced")

	withoutTLS := &Config{
		RabbitURI:        "",
		RabbitMQHost:     "rabbit.example.com",
		RabbitMQPortAMQP: "5672",
		RabbitMQTLS:      false,
	}
	uri = synthesizeWorkerRabbitMQURI(withoutTLS)

	got, err = readyz.DetectAMQPTLS(uri)
	require.NoError(t, err)
	assert.False(t, got, "RABBITMQ_TLS=false MUST surface as non-TLS")
}
