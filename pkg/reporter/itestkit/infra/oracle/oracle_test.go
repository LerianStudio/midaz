//go:build itestkit
// +build itestkit

package oracle_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/sijms/go-ora/v2"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/oracle"
)

func TestOracleInfra(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	t.Run("basic oracle with query verification", func(t *testing.T) {
		t.Parallel()

		infra := oracle.NewOracleInfra(oracle.OracleConfig{
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

		db, err := sql.Open("oracle", dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()

		err = db.PingContext(ctx)
		if err != nil {
			t.Fatalf("failed to ping: %v", err)
		}

		var result int
		err = db.QueryRowContext(ctx, "SELECT 1 FROM DUAL").Scan(&result)
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		if result != 1 {
			t.Errorf("expected 1, got %d", result)
		}
	})

	t.Run("oracle with custom password", func(t *testing.T) {
		t.Parallel()

		infra := oracle.NewOracleInfra(oracle.OracleConfig{
			Name:     "test-custom",
			Password: "CustomP@ssword123",
			SID:      "XE",
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

		if !strings.Contains(dsn, "oracle://system:") {
			t.Errorf("DSN should start with oracle://system:, got: %s", dsn)
		}
		if !strings.Contains(dsn, "/XE") {
			t.Errorf("DSN should contain SID, got: %s", dsn)
		}

		db, err := sql.Open("oracle", dsn)
		if err != nil {
			t.Fatalf("failed to open connection: %v", err)
		}
		defer db.Close()

		err = db.PingContext(ctx)
		if err != nil {
			t.Fatalf("failed to ping with custom config: %v", err)
		}
	})

	t.Run("oracle with chaos proxy", func(t *testing.T) {
		t.Skip("chaos proxy requires Docker network setup; covered by E2E tests in tests/chaos/")
	})
}

func TestOracleInfra_ErrorBeforeStart(t *testing.T) {
	t.Parallel()

	infra := oracle.NewOracleInfra(oracle.OracleConfig{
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

	_, err = infra.GoDRORDSN()
	if err == nil {
		t.Error("GoDRORDSN() should return error before Start()")
	}
}

func TestOracleInfra_NamedInfraInterface(t *testing.T) {
	t.Parallel()

	t.Run("returns correct InfraKind", func(t *testing.T) {
		infra := oracle.NewOracleInfra(oracle.OracleConfig{})

		if got := infra.InfraKind(); got != "oracle" {
			t.Errorf("InfraKind() = %q, want %q", got, "oracle")
		}
	})

	t.Run("returns configured InfraName", func(t *testing.T) {
		infra := oracle.NewOracleInfra(oracle.OracleConfig{
			Name: "custom-name",
		})

		if got := infra.InfraName(); got != "custom-name" {
			t.Errorf("InfraName() = %q, want %q", got, "custom-name")
		}
	})

	t.Run("returns default InfraName when not configured", func(t *testing.T) {
		infra := oracle.NewOracleInfra(oracle.OracleConfig{})

		if got := infra.InfraName(); got != "default" {
			t.Errorf("InfraName() with empty config = %q, want %q", got, "default")
		}
	})
}

func TestOracleInfra_DefaultConfiguration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	infra := oracle.NewOracleInfra(oracle.OracleConfig{})

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

	if !strings.HasPrefix(dsn, "oracle://system:") {
		t.Errorf("DSN should start with 'oracle://system:', got: %s", dsn)
	}
	if !strings.Contains(dsn, "testpass") {
		t.Errorf("DSN should contain default password 'testpass', got: %s", dsn)
	}
	if !strings.Contains(dsn, "/XE") {
		t.Errorf("DSN should contain default SID 'XE', got: %s", dsn)
	}

	db, err := sql.Open("oracle", dsn)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping with default config: %v", err)
	}
}

func TestOracleInfra_GoDRORDSN(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	infra := oracle.NewOracleInfra(oracle.OracleConfig{
		Name: "test-godror-dsn",
	})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite: %v", err)
	}
	defer suite.Terminate(ctx)

	dsn, err := infra.GoDRORDSN()
	if err != nil {
		t.Fatalf("failed to get GoDROR DSN: %v", err)
	}

	if !strings.HasPrefix(dsn, "system/") {
		t.Errorf("GoDRORDSN should start with 'system/', got: %s", dsn)
	}
	if !strings.Contains(dsn, "/XE") {
		t.Errorf("GoDRORDSN should contain default SID '/XE', got: %s", dsn)
	}
	if !strings.Contains(dsn, "@") {
		t.Errorf("GoDRORDSN should contain '@' separator, got: %s", dsn)
	}
}

func TestOracleInfra_EndpointStructure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	infra := oracle.NewOracleInfra(oracle.OracleConfig{
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
	if !strings.HasPrefix(endpoint.DSN, "oracle://") {
		t.Errorf("Endpoint.DSN should start with oracle://, got: %s", endpoint.DSN)
	}

	if endpoint.ProxyListen != "" {
		t.Errorf("Endpoint.ProxyListen should be empty when proxy disabled, got: %s", endpoint.ProxyListen)
	}
}

func TestOracleInfra_TerminateIdempotent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	t.Run("terminate before start should not error", func(t *testing.T) {
		infra := oracle.NewOracleInfra(oracle.OracleConfig{
			Name: "test-terminate-before-start",
		})

		if err := infra.Terminate(ctx); err != nil {
			t.Errorf("Terminate() before Start() returned error: %v", err)
		}
	})

	t.Run("double terminate should not error", func(t *testing.T) {
		infra := oracle.NewOracleInfra(oracle.OracleConfig{
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

func TestOracleInfra_WithOptions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	infra := oracle.NewOracleInfra(oracle.OracleConfig{
		Name: "test-with-options",
		Options: []oracle.OracleOption{
			oracle.WithOracleEnv("ORACLE_CHARACTERSET", "AL32UTF8"),
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

	db, err := sql.Open("oracle", dsn)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping with custom options: %v", err)
	}
}
