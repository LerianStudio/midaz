//go:build itestkit
// +build itestkit

package postgres_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/itestkit"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/itestkit/infra/postgres"
)

func waitForDB(ctx context.Context, db *sql.DB) error {
	for range 30 {
		if err := db.PingContext(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return db.PingContext(ctx)
}

func TestPostgresInfra(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("basic postgres with query verification", func(t *testing.T) {
		t.Parallel()

		infra := postgres.NewPostgresInfra(postgres.PostgresConfig{
			Name: "test-basic",
		})

		suite, err := itestkit.New(t).
			WithInfra(infra).
			Build(ctx)
		if err != nil {
			t.Fatalf("failed to build suite: %v", err)
		}
		defer suite.Terminate(ctx)

		dsn, err := infra.DSN()
		if err != nil {
			t.Fatalf("failed to get DSN: %v", err)
		}

		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()

		err = waitForDB(ctx, db)
		if err != nil {
			t.Fatalf("failed to ping: %v", err)
		}

		var result int
		err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		if result != 1 {
			t.Errorf("expected 1, got %d", result)
		}
	})

	t.Run("postgres with custom config", func(t *testing.T) {
		t.Parallel()

		infra := postgres.NewPostgresInfra(postgres.PostgresConfig{
			Name:     "test-custom",
			Database: "customdb",
			Username: "customuser",
			Password: "custompass",
		})

		suite, err := itestkit.New(t).
			WithInfra(infra).
			Build(ctx)
		if err != nil {
			t.Fatalf("failed to build suite: %v", err)
		}
		defer suite.Terminate(ctx)

		dsn, err := infra.DSN()
		if err != nil {
			t.Fatalf("failed to get DSN: %v", err)
		}

		if !strings.Contains(dsn, "customuser:") {
			t.Errorf("DSN should contain custom username, got: %s", dsn)
		}
		if !strings.Contains(dsn, ":custompass@") {
			t.Errorf("DSN should contain custom password, got: %s", dsn)
		}
		if !strings.Contains(dsn, "/customdb?") {
			t.Errorf("DSN should contain custom database, got: %s", dsn)
		}

		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()

		err = waitForDB(ctx, db)
		if err != nil {
			t.Fatalf("failed to ping with custom config: %v", err)
		}
	})

	t.Run("postgres with chaos proxy", func(t *testing.T) {
		t.Skip("chaos proxy requires Docker network setup; covered by E2E tests in tests/chaos/")
	})
}

func TestPostgresInfra_ErrorBeforeStart(t *testing.T) {
	t.Parallel()

	infra := postgres.NewPostgresInfra(postgres.PostgresConfig{
		Name: "test-error-before-start",
	})

	_, err := infra.Endpoint()
	if err == nil {
		t.Error("Endpoint() should return error before Start()")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("error should mention 'not ready', got: %v", err)
	}

	_, err = infra.DSN()
	if err == nil {
		t.Error("DSN() should return error before Start()")
	}
}

func TestPostgresInfra_NamedInfraInterface(t *testing.T) {
	t.Parallel()

	t.Run("returns correct InfraKind", func(t *testing.T) {
		infra := postgres.NewPostgresInfra(postgres.PostgresConfig{})

		if got := infra.InfraKind(); got != "postgres" {
			t.Errorf("InfraKind() = %q, want %q", got, "postgres")
		}
	})

	t.Run("returns configured InfraName", func(t *testing.T) {
		infra := postgres.NewPostgresInfra(postgres.PostgresConfig{
			Name: "custom-name",
		})

		if got := infra.InfraName(); got != "custom-name" {
			t.Errorf("InfraName() = %q, want %q", got, "custom-name")
		}
	})

	t.Run("returns default InfraName when not configured", func(t *testing.T) {
		infra := postgres.NewPostgresInfra(postgres.PostgresConfig{})

		if got := infra.InfraName(); got != "default" {
			t.Errorf("InfraName() with empty config = %q, want %q", got, "default")
		}
	})
}

func TestPostgresInfra_DefaultConfiguration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := postgres.NewPostgresInfra(postgres.PostgresConfig{})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite with default config: %v", err)
	}
	defer suite.Terminate(ctx)

	dsn, err := infra.DSN()
	if err != nil {
		t.Fatalf("failed to get DSN: %v", err)
	}

	if !strings.HasPrefix(dsn, "postgres://") {
		t.Errorf("DSN should start with 'postgres://', got: %s", dsn)
	}
	if !strings.Contains(dsn, "app:app@") {
		t.Errorf("DSN should contain default user:pass 'app:app@', got: %s", dsn)
	}
	if !strings.Contains(dsn, "/app?") {
		t.Errorf("DSN should contain default database '/app?', got: %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Errorf("DSN should contain 'sslmode=disable', got: %s", dsn)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	if err := waitForDB(ctx, db); err != nil {
		t.Fatalf("failed to ping with default config: %v", err)
	}
}

func TestPostgresInfra_EndpointStructure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := postgres.NewPostgresInfra(postgres.PostgresConfig{
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

	if endpoint.Upstream == "" {
		t.Error("Endpoint.Upstream should not be empty")
	}
	if !strings.Contains(endpoint.Upstream, ":") {
		t.Errorf("Endpoint.Upstream should be host:port format, got: %s", endpoint.Upstream)
	}

	// Verify DSN is set
	if endpoint.DSN == "" {
		t.Error("Endpoint.DSN should not be empty")
	}
	if !strings.HasPrefix(endpoint.DSN, "postgres://") {
		t.Errorf("Endpoint.DSN should start with postgres://, got: %s", endpoint.DSN)
	}

	if endpoint.ProxyListen != "" {
		t.Errorf("Endpoint.ProxyListen should be empty when proxy disabled, got: %s", endpoint.ProxyListen)
	}
}

func TestPostgresInfra_TerminateIdempotent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("terminate before start should not error", func(t *testing.T) {
		infra := postgres.NewPostgresInfra(postgres.PostgresConfig{
			Name: "test-terminate-before-start",
		})

		if err := infra.Terminate(ctx); err != nil {
			t.Errorf("Terminate() before Start() returned error: %v", err)
		}
	})

	t.Run("double terminate should not error", func(t *testing.T) {
		infra := postgres.NewPostgresInfra(postgres.PostgresConfig{
			Name: "test-double-terminate",
		})

		suite, err := itestkit.New(t).
			WithInfra(infra).
			Build(ctx)
		if err != nil {
			t.Fatalf("failed to build suite: %v", err)
		}

		if err := suite.Terminate(ctx); err != nil {
			t.Errorf("first Terminate() returned error: %v", err)
		}

		if err := infra.Terminate(ctx); err != nil {
			t.Errorf("second Terminate() returned error: %v", err)
		}
	})
}

func TestPostgresInfra_WithOptions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := postgres.NewPostgresInfra(postgres.PostgresConfig{
		Name: "test-with-options",
		Options: []postgres.PostgresOption{
			postgres.WithPGEnv("POSTGRES_INITDB_ARGS", "--encoding=UTF8"),
		},
	})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite with options: %v", err)
	}
	defer suite.Terminate(ctx)

	dsn, err := infra.DSN()
	if err != nil {
		t.Fatalf("failed to get DSN: %v", err)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	if err := waitForDB(ctx, db); err != nil {
		t.Fatalf("failed to ping with custom options: %v", err)
	}
}
