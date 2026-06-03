//go:build itestkit
// +build itestkit

package redis_test

import (
	"context"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/itestkit"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/itestkit/infra/redis"
)

func TestRedisInfra(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Run("basic redis with read-write verification", func(t *testing.T) {
		t.Parallel()

		infra := redis.NewRedisInfra(redis.RedisConfig{
			Name: "test-basic",
		})

		suite, err := itestkit.New(t).
			WithInfra(infra).
			Build(ctx)
		if err != nil {
			t.Fatalf("failed to build suite: %v", err)
		}
		defer suite.Terminate(ctx)

		addr, err := infra.Addr()
		if err != nil {
			t.Fatalf("failed to get addr: %v", err)
		}

		client := goredis.NewClient(&goredis.Options{Addr: addr})
		defer client.Close()

		err = client.Set(ctx, "test-key", "test-value", 0).Err()
		if err != nil {
			t.Fatalf("failed to SET: %v", err)
		}

		val, err := client.Get(ctx, "test-key").Result()
		if err != nil {
			t.Fatalf("failed to GET: %v", err)
		}

		if val != "test-value" {
			t.Errorf("expected 'test-value', got '%s'", val)
		}
	})

	t.Run("redis with password", func(t *testing.T) {
		t.Parallel()

		infra := redis.NewRedisInfra(redis.RedisConfig{
			Name:     "test-password",
			Password: "secretpass",
		})

		suite, err := itestkit.New(t).
			WithInfra(infra).
			Build(ctx)
		if err != nil {
			t.Fatalf("failed to build suite: %v", err)
		}
		defer suite.Terminate(ctx)

		// Verify URL contains password
		url, err := infra.URL()
		if err != nil {
			t.Fatalf("failed to get URL: %v", err)
		}

		if !strings.Contains(url, "redis://") {
			t.Errorf("URL should start with redis://, got: %s", url)
		}
		if !strings.Contains(url, ":secretpass@") {
			t.Errorf("URL should contain password, got: %s", url)
		}

		// Verify connectivity with password
		addr, err := infra.Addr()
		if err != nil {
			t.Fatalf("failed to get addr: %v", err)
		}

		client := goredis.NewClient(&goredis.Options{
			Addr:     addr,
			Password: "secretpass",
		})
		defer client.Close()

		if err := client.Ping(ctx).Err(); err != nil {
			t.Fatalf("failed to ping with password: %v", err)
		}
	})

	t.Run("redis with chaos proxy", func(t *testing.T) {
		t.Skip("chaos proxy requires Docker network setup; covered by E2E tests in tests/chaos/")
	})
}

func TestRedisInfra_ErrorBeforeStart(t *testing.T) {
	t.Parallel()

	infra := redis.NewRedisInfra(redis.RedisConfig{
		Name: "test-error-before-start",
	})

	// All accessor methods should return error before Start()
	_, err := infra.Endpoint()
	if err == nil {
		t.Error("Endpoint() should return error before Start()")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("error should mention 'not ready', got: %v", err)
	}

	_, err = infra.Addr()
	if err == nil {
		t.Error("Addr() should return error before Start()")
	}

	_, err = infra.URL()
	if err == nil {
		t.Error("URL() should return error before Start()")
	}
}

func TestRedisInfra_NamedInfraInterface(t *testing.T) {
	t.Parallel()

	t.Run("returns correct InfraKind", func(t *testing.T) {
		infra := redis.NewRedisInfra(redis.RedisConfig{})

		if got := infra.InfraKind(); got != "redis" {
			t.Errorf("InfraKind() = %q, want %q", got, "redis")
		}
	})

	t.Run("returns configured InfraName", func(t *testing.T) {
		infra := redis.NewRedisInfra(redis.RedisConfig{
			Name: "custom-name",
		})

		if got := infra.InfraName(); got != "custom-name" {
			t.Errorf("InfraName() = %q, want %q", got, "custom-name")
		}
	})

	t.Run("returns default InfraName when not configured", func(t *testing.T) {
		infra := redis.NewRedisInfra(redis.RedisConfig{})

		if got := infra.InfraName(); got != "default" {
			t.Errorf("InfraName() with empty config = %q, want %q", got, "default")
		}
	})
}

func TestRedisInfra_DefaultConfiguration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create with empty config to test defaults
	infra := redis.NewRedisInfra(redis.RedisConfig{})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite with default config: %v", err)
	}
	defer suite.Terminate(ctx)

	// Verify URL format (no password in default)
	url, err := infra.URL()
	if err != nil {
		t.Fatalf("failed to get URL: %v", err)
	}

	if !strings.HasPrefix(url, "redis://") {
		t.Errorf("URL should start with 'redis://', got: %s", url)
	}
	// Default has no password, so URL should not contain @
	if strings.Contains(url, "@") {
		t.Errorf("default URL should not contain credentials, got: %s", url)
	}

	// Verify connectivity
	addr, err := infra.Addr()
	if err != nil {
		t.Fatalf("failed to get addr: %v", err)
	}

	client := goredis.NewClient(&goredis.Options{Addr: addr})
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("failed to ping with default config: %v", err)
	}
}

func TestRedisInfra_EndpointStructure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	infra := redis.NewRedisInfra(redis.RedisConfig{
		Name: "test-endpoint-structure",
	})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite: %v", err)
	}
	defer suite.Terminate(ctx)

	endpoint, err := infra.Endpoint()
	if err != nil {
		t.Fatalf("failed to get endpoint: %v", err)
	}

	// Verify Upstream is set (host:port format)
	if endpoint.Upstream == "" {
		t.Error("Endpoint.Upstream should not be empty")
	}
	if !strings.Contains(endpoint.Upstream, ":") {
		t.Errorf("Endpoint.Upstream should be host:port format, got: %s", endpoint.Upstream)
	}

	// Verify URL is set
	if endpoint.URL == "" {
		t.Error("Endpoint.URL should not be empty")
	}
	if !strings.HasPrefix(endpoint.URL, "redis://") {
		t.Errorf("Endpoint.URL should start with redis://, got: %s", endpoint.URL)
	}

	// ProxyListen should be empty when proxy is disabled
	if endpoint.ProxyListen != "" {
		t.Errorf("Endpoint.ProxyListen should be empty when proxy disabled, got: %s", endpoint.ProxyListen)
	}
}

func TestRedisInfra_TerminateIdempotent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Run("terminate before start should not error", func(t *testing.T) {
		infra := redis.NewRedisInfra(redis.RedisConfig{
			Name: "test-terminate-before-start",
		})

		// Terminate before start should not error
		if err := infra.Terminate(ctx); err != nil {
			t.Errorf("Terminate() before Start() returned error: %v", err)
		}
	})

	t.Run("double terminate should not error", func(t *testing.T) {
		infra := redis.NewRedisInfra(redis.RedisConfig{
			Name: "test-double-terminate",
		})

		suite, err := itestkit.New(t).
			WithInfra(infra).
			Build(ctx)
		if err != nil {
			t.Fatalf("failed to build suite: %v", err)
		}

		// First terminate via suite
		if err := suite.Terminate(ctx); err != nil {
			t.Errorf("first Terminate() returned error: %v", err)
		}

		// Second terminate directly on infra should also succeed
		if err := infra.Terminate(ctx); err != nil {
			t.Errorf("second Terminate() returned error: %v", err)
		}
	})
}

func TestRedisInfra_WithOptions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	infra := redis.NewRedisInfra(redis.RedisConfig{
		Name: "test-with-options",
		Options: []redis.RedisOption{
			redis.WithRedisEnv("REDIS_ARGS", "--maxmemory 50mb"),
		},
	})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite with options: %v", err)
	}
	defer suite.Terminate(ctx)

	// Verify container is working
	addr, err := infra.Addr()
	if err != nil {
		t.Fatalf("failed to get addr: %v", err)
	}

	client := goredis.NewClient(&goredis.Options{Addr: addr})
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("failed to ping with custom options: %v", err)
	}
}
