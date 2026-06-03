//go:build itestkit
// +build itestkit

package mssql_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/microsoft/go-mssqldb"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/itestkit"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/itestkit/infra/mssql"
)

func TestMSSQLInfra(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	t.Run("basic mssql with query verification", func(t *testing.T) {
		t.Parallel()

		infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{
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

		db, err := sql.Open("sqlserver", dsn)
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

	t.Run("mssql with custom database", func(t *testing.T) {
		t.Parallel()

		infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{
			Name:     "test-custom",
			Database: "master",
			Password: "CustomP@ssw0rd!",
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

		if !strings.Contains(dsn, "sqlserver://sa:") {
			t.Errorf("DSN should start with sqlserver://sa:, got: %s", dsn)
		}
		if !strings.Contains(dsn, "?database=master") {
			t.Errorf("DSN should contain database parameter, got: %s", dsn)
		}

		db, err := sql.Open("sqlserver", dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()

		err = db.PingContext(ctx)
		if err != nil {
			t.Fatalf("failed to ping with custom config: %v", err)
		}
	})

	t.Run("mssql with chaos proxy", func(t *testing.T) {
		t.Skip("chaos proxy requires Docker network setup; covered by E2E tests in tests/chaos/")
	})
}

func TestMSSQLInfra_ErrorBeforeStart(t *testing.T) {
	t.Parallel()

	infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{
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

func TestMSSQLInfra_NamedInfraInterface(t *testing.T) {
	t.Parallel()

	t.Run("returns correct InfraKind", func(t *testing.T) {
		infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{})

		if got := infra.InfraKind(); got != "mssql" {
			t.Errorf("InfraKind() = %q, want %q", got, "mssql")
		}
	})

	t.Run("returns configured InfraName", func(t *testing.T) {
		infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{
			Name: "custom-name",
		})

		if got := infra.InfraName(); got != "custom-name" {
			t.Errorf("InfraName() = %q, want %q", got, "custom-name")
		}
	})

	t.Run("returns default InfraName when not configured", func(t *testing.T) {
		infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{})

		if got := infra.InfraName(); got != "default" {
			t.Errorf("InfraName() with empty config = %q, want %q", got, "default")
		}
	})
}

func TestMSSQLInfra_DefaultConfiguration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{})

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

	if !strings.HasPrefix(dsn, "sqlserver://sa:") {
		t.Errorf("DSN should start with 'sqlserver://sa:', got: %s", dsn)
	}
	if !strings.Contains(dsn, "YourStrong@Passw0rd") {
		t.Errorf("DSN should contain default password, got: %s", dsn)
	}
	if strings.Contains(dsn, "?database=") {
		t.Errorf("default DSN should not have database parameter, got: %s", dsn)
	}

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping with default config: %v", err)
	}
}

func TestMSSQLInfra_EndpointStructure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{
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
	if !strings.HasPrefix(endpoint.DSN, "sqlserver://") {
		t.Errorf("Endpoint.DSN should start with sqlserver://, got: %s", endpoint.DSN)
	}

	if endpoint.ProxyListen != "" {
		t.Errorf("Endpoint.ProxyListen should be empty when proxy disabled, got: %s", endpoint.ProxyListen)
	}
}

func TestMSSQLInfra_TerminateIdempotent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	t.Run("terminate before start should not error", func(t *testing.T) {
		infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{
			Name: "test-terminate-before-start",
		})

		if err := infra.Terminate(ctx); err != nil {
			t.Errorf("Terminate() before Start() returned error: %v", err)
		}
	})

	t.Run("double terminate should not error", func(t *testing.T) {
		infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{
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

func TestMSSQLInfra_WithOptions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	infra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{
		Name: "test-with-options",
		Options: []mssql.MSSQLOption{
			mssql.WithMSSQLEnv("MSSQL_MEMORY_LIMIT_MB", "512"),
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

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping with custom options: %v", err)
	}
}
