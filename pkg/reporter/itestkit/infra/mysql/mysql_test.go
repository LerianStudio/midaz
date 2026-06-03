//go:build itestkit
// +build itestkit

package mysql_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/itestkit"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/itestkit/infra/mysql"
)

func TestMySQLInfra(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("basic mysql with query verification", func(t *testing.T) {
		t.Parallel()

		infra := mysql.NewMySQLInfra(mysql.MySQLConfig{
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

		db, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()

		err = db.PingContext(ctx)
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

	t.Run("mysql with custom config", func(t *testing.T) {
		t.Parallel()

		infra := mysql.NewMySQLInfra(mysql.MySQLConfig{
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

		// Verify DSN contains custom values
		if !strings.Contains(dsn, "customuser:") {
			t.Errorf("DSN should contain custom username, got: %s", dsn)
		}
		if !strings.Contains(dsn, ":custompass@") {
			t.Errorf("DSN should contain custom password, got: %s", dsn)
		}
		if !strings.Contains(dsn, "/customdb?") {
			t.Errorf("DSN should contain custom database, got: %s", dsn)
		}

		// Verify connectivity
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()

		err = db.PingContext(ctx)
		if err != nil {
			t.Fatalf("failed to ping with custom config: %v", err)
		}
	})

	t.Run("mysql with chaos proxy", func(t *testing.T) {
		t.Skip("chaos proxy requires Docker network setup; covered by E2E tests in tests/chaos/")
	})
}

func TestMySQLInfra_ErrorBeforeStart(t *testing.T) {
	t.Parallel()

	infra := mysql.NewMySQLInfra(mysql.MySQLConfig{
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

func TestMySQLInfra_NamedInfraInterface(t *testing.T) {
	t.Parallel()

	t.Run("returns correct InfraKind", func(t *testing.T) {
		infra := mysql.NewMySQLInfra(mysql.MySQLConfig{})

		if got := infra.InfraKind(); got != "mysql" {
			t.Errorf("InfraKind() = %q, want %q", got, "mysql")
		}
	})

	t.Run("returns configured InfraName", func(t *testing.T) {
		infra := mysql.NewMySQLInfra(mysql.MySQLConfig{
			Name: "custom-name",
		})

		if got := infra.InfraName(); got != "custom-name" {
			t.Errorf("InfraName() = %q, want %q", got, "custom-name")
		}
	})

	t.Run("returns default InfraName when not configured", func(t *testing.T) {
		infra := mysql.NewMySQLInfra(mysql.MySQLConfig{})

		if got := infra.InfraName(); got != "default" {
			t.Errorf("InfraName() with empty config = %q, want %q", got, "default")
		}
	})
}

func TestMySQLInfra_DefaultConfiguration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := mysql.NewMySQLInfra(mysql.MySQLConfig{})

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

	if !strings.Contains(dsn, "testuser:") {
		t.Errorf("DSN should contain default username 'testuser', got: %s", dsn)
	}
	if !strings.Contains(dsn, ":testpass@") {
		t.Errorf("DSN should contain default password 'testpass', got: %s", dsn)
	}
	if !strings.Contains(dsn, "/testdb?") {
		t.Errorf("DSN should contain default database 'testdb', got: %s", dsn)
	}
	if !strings.Contains(dsn, "parseTime=true") {
		t.Errorf("DSN should contain parseTime=true, got: %s", dsn)
	}
	if !strings.Contains(dsn, "@tcp(") {
		t.Errorf("DSN should use tcp() wrapper, got: %s", dsn)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping with default config: %v", err)
	}
}

func TestMySQLInfra_EndpointStructure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := mysql.NewMySQLInfra(mysql.MySQLConfig{
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

	if endpoint.DSN == "" {
		t.Error("Endpoint.DSN should not be empty")
	}

	if endpoint.ProxyListen != "" {
		t.Errorf("Endpoint.ProxyListen should be empty when proxy disabled, got: %s", endpoint.ProxyListen)
	}
}

func TestMySQLInfra_TerminateIdempotent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("terminate before start should not error", func(t *testing.T) {
		infra := mysql.NewMySQLInfra(mysql.MySQLConfig{
			Name: "test-terminate-before-start",
		})

		if err := infra.Terminate(ctx); err != nil {
			t.Errorf("Terminate() before Start() returned error: %v", err)
		}
	})

	t.Run("double terminate should not error", func(t *testing.T) {
		infra := mysql.NewMySQLInfra(mysql.MySQLConfig{
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

func TestMySQLInfra_WithOptions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := mysql.NewMySQLInfra(mysql.MySQLConfig{
		Name: "test-with-options",
		Options: []mysql.MySQLOption{
			mysql.WithMySQLEnv("MYSQL_ALLOW_EMPTY_PASSWORD", "no"),
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

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping with custom options: %v", err)
	}
}
