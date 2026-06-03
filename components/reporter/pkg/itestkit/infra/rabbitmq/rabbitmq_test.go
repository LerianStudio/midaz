//go:build itestkit
// +build itestkit

package rabbitmq_test

import (
	"context"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/LerianStudio/reporter/pkg/itestkit"
	"github.com/LerianStudio/reporter/pkg/itestkit/infra/rabbitmq"
)

func TestRabbitInfra(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("basic rabbitmq with queue declaration", func(t *testing.T) {
		t.Parallel()

		infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
			Name: "test-basic",
		})

		suite, err := itestkit.New(t).
			WithInfra(infra).
			Build(ctx)
		if err != nil {
			t.Fatalf("failed to build suite: %v", err)
		}
		defer suite.Terminate(ctx)

		amqpURL, err := infra.AMQPURL()
		if err != nil {
			t.Fatalf("failed to get AMQP URL: %v", err)
		}

		conn, err := amqp.Dial(amqpURL)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		ch, err := conn.Channel()
		if err != nil {
			t.Fatalf("failed to open channel: %v", err)
		}
		defer ch.Close()

		q, err := ch.QueueDeclare("test-queue", false, true, false, false, nil)
		if err != nil {
			t.Fatalf("failed to declare queue: %v", err)
		}

		if q.Name != "test-queue" {
			t.Errorf("expected queue name 'test-queue', got '%s'", q.Name)
		}
	})

	t.Run("rabbitmq with custom credentials", func(t *testing.T) {
		t.Parallel()

		infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
			Name:     "test-custom",
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

		amqpURL, err := infra.AMQPURL()
		if err != nil {
			t.Fatalf("failed to get AMQP URL: %v", err)
		}

		if !strings.Contains(amqpURL, "testuser:") {
			t.Errorf("AMQP URL should contain custom username, got: %s", amqpURL)
		}
		if !strings.Contains(amqpURL, ":testpass@") {
			t.Errorf("AMQP URL should contain custom password, got: %s", amqpURL)
		}

		conn, err := amqp.Dial(amqpURL)
		if err != nil {
			t.Fatalf("failed to connect with custom config: %v", err)
		}
		defer conn.Close()
	})

	t.Run("rabbitmq with chaos proxy", func(t *testing.T) {
		t.Skip("chaos proxy requires Docker network setup; covered by E2E tests in tests/chaos/")
	})
}

func TestRabbitInfra_ErrorBeforeStart(t *testing.T) {
	t.Parallel()

	infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
		Name: "test-error-before-start",
	})

	_, err := infra.Endpoint()
	if err == nil {
		t.Error("Endpoint() should return error before Start()")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("error should mention 'not ready', got: %v", err)
	}

	_, err = infra.AMQPURL()
	if err == nil {
		t.Error("AMQPURL() should return error before Start()")
	}
}

func TestRabbitInfra_NamedInfraInterface(t *testing.T) {
	t.Parallel()

	t.Run("returns correct InfraKind", func(t *testing.T) {
		infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{})

		if got := infra.InfraKind(); got != "rabbitmq" {
			t.Errorf("InfraKind() = %q, want %q", got, "rabbitmq")
		}
	})

	t.Run("returns configured InfraName", func(t *testing.T) {
		infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
			Name: "custom-name",
		})

		if got := infra.InfraName(); got != "custom-name" {
			t.Errorf("InfraName() = %q, want %q", got, "custom-name")
		}
	})

	t.Run("returns default InfraName when not configured", func(t *testing.T) {
		infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{})

		if got := infra.InfraName(); got != "default" {
			t.Errorf("InfraName() with empty config = %q, want %q", got, "default")
		}
	})
}

func TestRabbitInfra_DefaultConfiguration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite with default config: %v", err)
	}
	defer suite.Terminate(ctx)

	amqpURL, err := infra.AMQPURL()
	if err != nil {
		t.Fatalf("failed to get AMQP URL: %v", err)
	}

	if !strings.HasPrefix(amqpURL, "amqp://") {
		t.Errorf("AMQP URL should start with 'amqp://', got: %s", amqpURL)
	}
	if !strings.Contains(amqpURL, "guest:guest@") {
		t.Errorf("AMQP URL should contain default credentials 'guest:guest@', got: %s", amqpURL)
	}
	if !strings.HasSuffix(amqpURL, "/") {
		t.Errorf("AMQP URL should end with default VHost '/', got: %s", amqpURL)
	}

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		t.Fatalf("failed to connect with default config: %v", err)
	}
	defer conn.Close()
}

func TestRabbitInfra_EndpointStructure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
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
	if endpoint.AMQPURL == "" {
		t.Error("Endpoint.AMQPURL should not be empty")
	}
	if !strings.HasPrefix(endpoint.AMQPURL, "amqp://") {
		t.Errorf("Endpoint.AMQPURL should start with amqp://, got: %s", endpoint.AMQPURL)
	}

	if endpoint.ProxyListen != "" {
		t.Errorf("Endpoint.ProxyListen should be empty when proxy disabled, got: %s", endpoint.ProxyListen)
	}
}

func TestRabbitInfra_TerminateIdempotent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("terminate before start should not error", func(t *testing.T) {
		infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
			Name: "test-terminate-before-start",
		})

		if err := infra.Terminate(ctx); err != nil {
			t.Errorf("Terminate() before Start() returned error: %v", err)
		}
	})

	t.Run("double terminate should not error", func(t *testing.T) {
		infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
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

func TestRabbitInfra_WithOptions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
		Name: "test-with-options",
		Options: []rabbitmq.RabbitOption{
			rabbitmq.WithRabbitEnv("RABBITMQ_VM_MEMORY_HIGH_WATERMARK", "0.6"),
		},
	})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite with options: %v", err)
	}
	defer suite.Terminate(ctx)

	amqpURL, err := infra.AMQPURL()
	if err != nil {
		t.Fatalf("failed to get AMQP URL: %v", err)
	}

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		t.Fatalf("failed to connect with custom options: %v", err)
	}
	defer conn.Close()
}

func TestRabbitInfra_WithDefinitions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	infra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
		Name:     "test-definitions",
		Username: "admin",
		Password: "admin",
		Options: []rabbitmq.RabbitOption{
			rabbitmq.WithRabbitDefinitions("testdata/definitions.json"),
		},
	})

	suite, err := itestkit.New(t).
		WithInfra(infra).
		Build(ctx)
	if err != nil {
		t.Fatalf("failed to build suite with definitions: %v", err)
	}
	defer suite.Terminate(ctx)

	amqpURL, err := infra.AMQPURL()
	if err != nil {
		t.Fatalf("failed to get AMQP URL: %v", err)
	}

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open channel: %v", err)
	}
	defer ch.Close()

	t.Run("exchanges from definitions exist", func(t *testing.T) {
		exchanges := []string{
			"test.events",
			"test.dlx",
		}

		for _, ex := range exchanges {
			err := ch.ExchangeDeclarePassive(ex, "direct", true, false, false, false, nil)
			if err != nil {
				t.Errorf("exchange %q should exist from definitions: %v", ex, err)
				ch, _ = conn.Channel()
			}
		}
	})

	t.Run("queues from definitions exist", func(t *testing.T) {
		queues := []string{
			"test.queue",
			"test.dlq",
		}

		for _, q := range queues {
			_, err := ch.QueueDeclarePassive(q, true, false, false, false, nil)
			if err != nil {
				t.Errorf("queue %q should exist from definitions: %v", q, err)
				ch, _ = conn.Channel()
			}
		}
	})

	t.Run("can publish to exchange with routing key", func(t *testing.T) {
		err := ch.PublishWithContext(ctx,
			"test.events",
			"test.message",
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        []byte(`{"test": true}`),
			},
		)
		if err != nil {
			t.Errorf("failed to publish to exchange: %v", err)
		}
	})
}
