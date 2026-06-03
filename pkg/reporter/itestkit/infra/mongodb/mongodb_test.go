//go:build itestkit
// +build itestkit

package mongodb_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/itestkit"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/itestkit/infra/mongodb"
)

func TestMongoDBInfra(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("basic mongodb with ping verification", func(t *testing.T) {
		t.Parallel()

		infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
			Name: "test-basic",
		})

		suite, err := itestkit.New(t).
			WithInfra(infra).
			Build(ctx)
		if err != nil {
			t.Fatalf("failed to build suite: %v", err)
		}
		defer suite.Terminate(ctx)

		uri, err := infra.URI()
		if err != nil {
			t.Fatalf("failed to get URI: %v", err)
		}

		if !strings.HasPrefix(uri, "mongodb://") {
			t.Errorf("URI should start with mongodb://, got: %s", uri)
		}
		if strings.Contains(uri, "@") {
			t.Errorf("default URI should not contain credentials, got: %s", uri)
		}

		client, err := mongo.Connect(options.Client().ApplyURI(uri))
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer client.Disconnect(ctx)

		err = client.Ping(ctx, nil)
		if err != nil {
			t.Fatalf("failed to ping: %v", err)
		}
	})

	t.Run("mongodb with auth", func(t *testing.T) {
		t.Parallel()

		infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
			Name:     "test-auth",
			Username: "testuser",
			Password: "testpass",
		})

		suite, err := itestkit.New(t).
			WithInfra(infra).
			Build(ctx)
		if err != nil {
			t.Fatalf("failed to build suite: %v", err)
		}
		defer suite.Terminate(ctx)

		uri, err := infra.URI()
		if err != nil {
			t.Fatalf("failed to get URI: %v", err)
		}

		if !strings.Contains(uri, "testuser:") {
			t.Errorf("URI should contain username, got: %s", uri)
		}
		if !strings.Contains(uri, ":testpass@") {
			t.Errorf("URI should contain password, got: %s", uri)
		}

		client, err := mongo.Connect(options.Client().ApplyURI(uri))
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer client.Disconnect(ctx)

		err = client.Ping(ctx, nil)
		if err != nil {
			t.Fatalf("failed to ping with auth: %v", err)
		}
	})

	t.Run("mongodb with chaos proxy", func(t *testing.T) {
		t.Skip("chaos proxy requires Docker network setup; covered by E2E tests in tests/chaos/")
	})
}

func TestMongoDBInfra_ErrorBeforeStart(t *testing.T) {
	t.Parallel()

	infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
		Name: "test-error-before-start",
	})

	_, err := infra.Endpoint()
	if err == nil {
		t.Error("Endpoint() should return error before Start()")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("error should mention 'not ready', got: %v", err)
	}

	_, err = infra.URI()
	if err == nil {
		t.Error("URI() should return error before Start()")
	}
}

func TestMongoDBInfra_NamedInfraInterface(t *testing.T) {
	t.Parallel()

	t.Run("returns correct InfraKind", func(t *testing.T) {
		infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{})

		if got := infra.InfraKind(); got != "mongodb" {
			t.Errorf("InfraKind() = %q, want %q", got, "mongodb")
		}
	})

	t.Run("returns configured InfraName", func(t *testing.T) {
		infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
			Name: "custom-name",
		})

		if got := infra.InfraName(); got != "custom-name" {
			t.Errorf("InfraName() = %q, want %q", got, "custom-name")
		}
	})

	t.Run("returns default InfraName when not configured", func(t *testing.T) {
		infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{})

		if got := infra.InfraName(); got != "default" {
			t.Errorf("InfraName() with empty config = %q, want %q", got, "default")
		}
	})
}

func TestMongoDBInfra_DefaultConfiguration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite with default config: %v", err)
	}
	defer suite.Terminate(ctx)

	uri, err := infra.URI()
	if err != nil {
		t.Fatalf("failed to get URI: %v", err)
	}

	if !strings.HasPrefix(uri, "mongodb://") {
		t.Errorf("URI should start with 'mongodb://', got: %s", uri)
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect(ctx)

	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("failed to ping with default config: %v", err)
	}
}

func TestMongoDBInfra_EndpointStructure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
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

	if endpoint.URI == "" {
		t.Error("Endpoint.URI should not be empty")
	}
	if !strings.HasPrefix(endpoint.URI, "mongodb://") {
		t.Errorf("Endpoint.URI should start with mongodb://, got: %s", endpoint.URI)
	}

	if endpoint.ProxyListen != "" {
		t.Errorf("Endpoint.ProxyListen should be empty when proxy disabled, got: %s", endpoint.ProxyListen)
	}
}

func TestMongoDBInfra_TerminateIdempotent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("terminate before start should not error", func(t *testing.T) {
		infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
			Name: "test-terminate-before-start",
		})

		if err := infra.Terminate(ctx); err != nil {
			t.Errorf("Terminate() before Start() returned error: %v", err)
		}
	})

	t.Run("double terminate should not error", func(t *testing.T) {
		infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
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

func TestMongoDBInfra_WithOptions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
		Name: "test-with-options",
		Options: []mongodb.MongoDBOption{
			mongodb.WithMongoDBEnv("MONGO_INITDB_DATABASE", "testdb"),
		},
	})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite with options: %v", err)
	}
	defer suite.Terminate(ctx)

	uri, err := infra.URI()
	if err != nil {
		t.Fatalf("failed to get URI: %v", err)
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect(ctx)

	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("failed to ping with custom options: %v", err)
	}
}
