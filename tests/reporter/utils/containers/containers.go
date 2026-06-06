// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package containers

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/v4/tests/reporter/utils/chaos"

	mobycontainer "github.com/moby/moby/api/types/container"
	mobynetwork "github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

// freeHostPort reserves an ephemeral TCP port on the loopback interface and
// returns it as a string. The listener is closed before returning so the port
// can be handed to Docker as an explicit host-side binding.
//
// There is an inherent race between closing the listener here and Docker
// binding the port: another process could claim it in between. For this test
// harness the window is negligible — the reporter suites run with `-p 1`
// (serial), so no two containers contend for the same allocation — and the
// payoff is a host port that survives container stop/start, which is what the
// chaos restart tests rely on (see applyFixedHostPorts).
func freeHostPort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("reserve free host port: %w", err)
	}
	defer l.Close()

	return fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port), nil
}

// applyFixedHostPorts returns a HostConfigModifier that pins each given
// container port to an explicit host port. Without an explicit binding Docker
// assigns a new ephemeral host port on every container start, which strands the
// in-process Manager/Worker services after a chaos restart: their connection
// config is captured once at suite startup and never re-read. Fixed bindings
// make the host address stable across stop/start so recovery actually works.
//
// bindings maps a container port (e.g. "6379/tcp") to a host port string.
func applyFixedHostPorts(bindings map[string]string) func(*mobycontainer.HostConfig) {
	return func(hc *mobycontainer.HostConfig) {
		if hc.PortBindings == nil {
			hc.PortBindings = mobynetwork.PortMap{}
		}

		for containerPort, hostPort := range bindings {
			hc.PortBindings[mobynetwork.MustParsePort(containerPort)] = []mobynetwork.PortBinding{
				{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: hostPort},
			}
		}
	}
}

// TestInfrastructure holds all test containers and provides connection information.
type TestInfrastructure struct {
	MongoDB   *MongoDBContainer
	RabbitMQ  *RabbitMQContainer
	SeaweedFS *SeaweedFSContainer
	Valkey    *ValkeyContainer
	Postgres  *PostgresContainer
	Toxiproxy *chaos.ToxiproxyInfrastructure

	network     *testcontainers.DockerNetwork
	networkName string
	mu          sync.Mutex
}

const defaultStartTimeoutSeconds = 120

// InfrastructureConfig holds configuration for container startup.
type InfrastructureConfig struct {
	MongoImage    string
	RabbitImage   string
	SeaweedImage  string
	ValkeyImage   string
	PostgresImage string
	NetworkName   string
	StartTimeout  time.Duration
}

// DefaultConfig returns default configuration for test infrastructure.
func DefaultConfig() *InfrastructureConfig {
	return &InfrastructureConfig{
		MongoImage:    "mongo:latest",
		RabbitImage:   "rabbitmq:4.0-management-alpine",
		SeaweedImage:  "chrislusf/seaweedfs:3.97",
		ValkeyImage:   "valkey/valkey:latest",
		PostgresImage: "postgres:16-alpine",
		NetworkName:   "reporter-test-network",
		StartTimeout:  defaultStartTimeoutSeconds * time.Second,
	}
}

// StartInfrastructure starts all required containers for testing.
// Containers are started in parallel for faster startup.
func StartInfrastructure(ctx context.Context) (*TestInfrastructure, error) {
	return StartInfrastructureWithConfig(ctx, DefaultConfig())
}

// StartInfrastructureWithConfig starts all containers with custom configuration.
func StartInfrastructureWithConfig(ctx context.Context, cfg *InfrastructureConfig) (*TestInfrastructure, error) {
	// Create network for container communication
	dockerNet, err := network.New(ctx,
		network.WithDriver("bridge"),
	)
	if err != nil {
		return nil, fmt.Errorf("create network: %w", err)
	}

	networkName := dockerNet.Name

	infra := &TestInfrastructure{
		network:     dockerNet,
		networkName: networkName,
	}

	// Start containers in parallel
	var wg sync.WaitGroup

	errCh := make(chan error, 5)

	// MongoDB
	wg.Add(1)

	go func() {
		defer wg.Done()

		mongo, err := StartMongoDB(ctx, networkName, cfg.MongoImage)
		if err != nil {
			errCh <- fmt.Errorf("mongodb: %w", err)
			return
		}

		infra.mu.Lock()
		infra.MongoDB = mongo
		infra.mu.Unlock()
	}()

	// RabbitMQ
	wg.Add(1)

	go func() {
		defer wg.Done()

		rabbit, err := StartRabbitMQ(ctx, networkName, cfg.RabbitImage)
		if err != nil {
			errCh <- fmt.Errorf("rabbitmq: %w", err)
			return
		}

		infra.mu.Lock()
		infra.RabbitMQ = rabbit
		infra.mu.Unlock()
	}()

	// SeaweedFS
	wg.Add(1)

	go func() {
		defer wg.Done()

		seaweed, err := StartSeaweedFS(ctx, networkName, cfg.SeaweedImage)
		if err != nil {
			errCh <- fmt.Errorf("seaweedfs: %w", err)
			return
		}

		infra.mu.Lock()
		infra.SeaweedFS = seaweed
		infra.mu.Unlock()
	}()

	// Valkey
	wg.Add(1)

	go func() {
		defer wg.Done()

		valkey, err := StartValkey(ctx, networkName, cfg.ValkeyImage)
		if err != nil {
			errCh <- fmt.Errorf("valkey: %w", err)
			return
		}

		infra.mu.Lock()
		infra.Valkey = valkey
		infra.mu.Unlock()
	}()

	// PostgreSQL (onboarding datasource backing report rendering)
	wg.Add(1)

	go func() {
		defer wg.Done()

		pg, err := StartPostgres(ctx, networkName, cfg.PostgresImage)
		if err != nil {
			errCh <- fmt.Errorf("postgres: %w", err)
			return
		}

		infra.mu.Lock()
		infra.Postgres = pg
		infra.mu.Unlock()
	}()

	// Wait for all containers to start
	wg.Wait()
	close(errCh)

	// Check for errors
	errs := make([]error, 0, 5)
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		// Cleanup any started containers
		_ = infra.Stop(ctx)
		return nil, fmt.Errorf("failed to start containers: %v", errs)
	}

	return infra, nil
}

// Stop terminates all containers and cleans up resources.
func (i *TestInfrastructure) Stop(ctx context.Context) error {
	var errs []error

	// Terminate Toxiproxy first (it depends on other containers)
	if i.Toxiproxy != nil {
		if err := i.Toxiproxy.Terminate(ctx); err != nil {
			errs = append(errs, fmt.Errorf("toxiproxy terminate: %w", err))
		}
	}

	if i.MongoDB != nil {
		if err := i.MongoDB.Terminate(ctx); err != nil {
			errs = append(errs, fmt.Errorf("mongodb terminate: %w", err))
		}
	}

	if i.RabbitMQ != nil {
		if err := i.RabbitMQ.Terminate(ctx); err != nil {
			errs = append(errs, fmt.Errorf("rabbitmq terminate: %w", err))
		}
	}

	if i.SeaweedFS != nil {
		if err := i.SeaweedFS.Terminate(ctx); err != nil {
			errs = append(errs, fmt.Errorf("seaweedfs terminate: %w", err))
		}
	}

	if i.Valkey != nil {
		if err := i.Valkey.Terminate(ctx); err != nil {
			errs = append(errs, fmt.Errorf("valkey terminate: %w", err))
		}
	}

	if i.Postgres != nil {
		if err := i.Postgres.Terminate(ctx); err != nil {
			errs = append(errs, fmt.Errorf("postgres terminate: %w", err))
		}
	}

	if i.network != nil {
		if err := i.network.Remove(ctx); err != nil {
			errs = append(errs, fmt.Errorf("network remove: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}

	return nil
}

// proxyPlan describes a Toxiproxy proxy: which dependency it fronts, the
// data-plane port it listens on inside the container, and the container-alias
// upstream it forwards to. Listen ports are restart-stable network aliases on
// the upstream side, so proxies survive datastore container restarts.
type proxyPlan struct {
	name       string
	listenPort string
	upstream   string
	present    bool
}

// proxyPlans returns the proxy definitions for every running dependency.
// A single source of truth shared by StartToxiproxy and GetToxiproxyEndpoints.
func (i *TestInfrastructure) proxyPlans() []proxyPlan {
	return []proxyPlan{
		{chaos.ProxyNameRabbitMQ, chaos.ProxyListenPortRabbitMQ, "rabbitmq:5672", i.RabbitMQ != nil},
		{chaos.ProxyNameMongoDB, chaos.ProxyListenPortMongoDB, "mongodb:27017", i.MongoDB != nil},
		{chaos.ProxyNameValkey, chaos.ProxyListenPortValkey, "valkey:6379", i.Valkey != nil},
		{chaos.ProxyNameSeaweedFS, chaos.ProxyListenPortSeaweedFS, "seaweedfs:8333", i.SeaweedFS != nil},
	}
}

// StartToxiproxy starts a Toxiproxy container on the test network and creates
// proxies for all running external dependencies. Services should connect through
// the proxy endpoints instead of directly to containers when chaos testing.
func (i *TestInfrastructure) StartToxiproxy(ctx context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	plans := i.proxyPlans()

	// Expose every listen port at container creation; Docker cannot publish a
	// port after the container has started, and the host-side services need
	// these ports mapped to route traffic through Toxiproxy.
	listenPorts := make([]string, 0, len(plans))
	for _, p := range plans {
		if p.present {
			listenPorts = append(listenPorts, p.listenPort)
		}
	}

	toxi, err := chaos.StartToxiproxy(ctx, i.networkName, listenPorts...)
	if err != nil {
		return fmt.Errorf("start toxiproxy: %w", err)
	}

	i.Toxiproxy = toxi

	for _, p := range plans {
		if !p.present {
			continue
		}

		if _, err := toxi.CreateProxy(chaos.ProxyConfig{
			Name:     p.name,
			Listen:   "0.0.0.0:" + p.listenPort,
			Upstream: p.upstream,
		}); err != nil {
			return fmt.Errorf("create %s proxy: %w", p.name, err)
		}
	}

	return nil
}

// GetToxiproxyEndpoints returns host-accessible endpoints that route through Toxiproxy
// for each dependency. Use these endpoints instead of the direct container endpoints
// when you want Toxiproxy to control the traffic.
func (i *TestInfrastructure) GetToxiproxyEndpoints(ctx context.Context) (map[string]string, error) {
	if i.Toxiproxy == nil {
		return nil, fmt.Errorf("toxiproxy not started")
	}

	endpoints := make(map[string]string)

	for _, p := range i.proxyPlans() {
		if _, ok := i.Toxiproxy.Proxies[p.name]; !ok {
			continue
		}

		mapped, err := i.Toxiproxy.Container.MappedPort(ctx, p.listenPort+"/tcp")
		if err != nil {
			return nil, fmt.Errorf("get mapped port for %s: %w", p.name, err)
		}

		endpoints[p.name] = fmt.Sprintf("%s:%s", i.Toxiproxy.Host, mapped.Port())
	}

	return endpoints, nil
}

// ConnectionConfig returns all connection strings for services.
type ConnectionConfig struct {
	MongoURI       string
	MongoHost      string
	MongoPort      string
	RabbitURL      string
	RabbitHost     string
	RabbitPort     string
	RabbitMgmtPort string
	S3Endpoint     string
	S3Host         string
	S3Port         string
	RedisHost      string
	RedisPort      string
	RedisAddr      string
}

// GetConnectionConfig returns connection configuration for all services.
func (i *TestInfrastructure) GetConnectionConfig() *ConnectionConfig {
	cfg := &ConnectionConfig{}

	if i.MongoDB != nil {
		cfg.MongoURI = i.MongoDB.ConnectionString
		cfg.MongoHost = i.MongoDB.Host
		cfg.MongoPort = i.MongoDB.Port
	}

	if i.RabbitMQ != nil {
		cfg.RabbitURL = i.RabbitMQ.AmqpURL
		cfg.RabbitHost = i.RabbitMQ.Host
		cfg.RabbitPort = i.RabbitMQ.AmqpPort
		cfg.RabbitMgmtPort = i.RabbitMQ.MgmtPort
	}

	if i.SeaweedFS != nil {
		cfg.S3Endpoint = i.SeaweedFS.S3Endpoint
		cfg.S3Host = i.SeaweedFS.Host
		cfg.S3Port = i.SeaweedFS.S3Port
	}

	if i.Valkey != nil {
		cfg.RedisHost = i.Valkey.Host
		cfg.RedisPort = i.Valkey.Port
		cfg.RedisAddr = i.Valkey.Address
	}

	return cfg
}
