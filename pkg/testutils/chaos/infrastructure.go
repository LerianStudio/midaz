//go:build integration || chaos

package chaos

import (
	"context"
	"fmt"
	"testing"

	toxiproxyclient "github.com/Shopify/toxiproxy/v2/client"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcNetwork "github.com/testcontainers/testcontainers-go/network"
)

// Infrastructure manages the test infrastructure for chaos testing.
// It provides a unified way to set up containers with network proxies for chaos injection.
type Infrastructure struct {
	t          *testing.T
	network    *testcontainers.DockerNetwork
	toxiproxy  *ToxiproxyResult
	containers map[string]*ContainerInfo
	proxies    map[string]*Proxy
	orch       *Orchestrator
}

// ContainerInfo holds information about a managed container.
type ContainerInfo struct {
	Container    testcontainers.Container
	ID           string
	Host         string
	Port         string
	ProxyListen  string // Address to connect through proxy (if proxied)
	DirectAddr   string // Direct address from host (bypassing proxy)
	UpstreamAddr string // Address for Toxiproxy to reach this container (uses host.docker.internal)
}

// InfrastructureConfig holds configuration for the chaos infrastructure.
type InfrastructureConfig struct {
	// NetworkName is the name of the Docker network to create.
	NetworkName string
	// SetupToxiproxy indicates whether to set up Toxiproxy for network chaos.
	SetupToxiproxy bool
}

// DefaultInfrastructureConfig returns the default infrastructure configuration.
func DefaultInfrastructureConfig() InfrastructureConfig {
	return InfrastructureConfig{
		NetworkName:    "chaos-test-network",
		SetupToxiproxy: true,
	}
}

// NewInfrastructure creates a new chaos test infrastructure.
func NewInfrastructure(t *testing.T) *Infrastructure {
	return NewInfrastructureWithConfig(t, DefaultInfrastructureConfig())
}

// NewInfrastructureWithConfig creates a new chaos test infrastructure with custom configuration.
func NewInfrastructureWithConfig(t *testing.T, cfg InfrastructureConfig) *Infrastructure {
	t.Helper()
	ctx := context.Background()

	// Create Docker network for containers
	network, err := tcNetwork.New(ctx, tcNetwork.WithCheckDuplicate())
	require.NoError(t, err, "failed to create Docker network")

	infra := &Infrastructure{
		t:          t,
		network:    network,
		containers: make(map[string]*ContainerInfo),
		proxies:    make(map[string]*Proxy),
		orch:       NewOrchestrator(t),
	}

	// Set up Toxiproxy if requested
	if cfg.SetupToxiproxy {
		infra.toxiproxy = SetupToxiproxy(t)
		infra.orch.SetToxiproxyClient(infra.toxiproxy.Client)
	}

	return infra
}

// Orchestrator returns the chaos orchestrator.
func (i *Infrastructure) Orchestrator() *Orchestrator {
	return i.orch
}

// Network returns the Docker network.
func (i *Infrastructure) Network() *testcontainers.DockerNetwork {
	return i.network
}

// NetworkName returns the name of the Docker network.
func (i *Infrastructure) NetworkName() string {
	return i.network.Name
}

// RegisterContainer registers a container with the infrastructure.
// This allows the infrastructure to track and manage the container.
func (i *Infrastructure) RegisterContainer(name string, container testcontainers.Container) (*ContainerInfo, error) {
	i.t.Helper()
	ctx := context.Background()

	id := container.GetContainerID()

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	// Get the first mapped port (caller should specify if needed)
	ports, err := container.Ports(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container ports: %w", err)
	}

	var port string
	for _, bindings := range ports {
		if len(bindings) > 0 {
			port = bindings[0].HostPort
			break
		}
	}

	info := &ContainerInfo{
		Container:  container,
		ID:         id,
		Host:       host,
		Port:       port,
		DirectAddr: fmt.Sprintf("%s:%s", host, port),
	}

	i.containers[name] = info
	return info, nil
}

// RegisterContainerWithPort registers a container with a specific port.
func (i *Infrastructure) RegisterContainerWithPort(name string, container testcontainers.Container, portID string) (*ContainerInfo, error) {
	i.t.Helper()
	ctx := context.Background()

	id := container.GetContainerID()

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, nat.Port(portID))
	if err != nil {
		return nil, fmt.Errorf("failed to get mapped port %s: %w", portID, err)
	}

	info := &ContainerInfo{
		Container:    container,
		ID:           id,
		Host:         host,
		Port:         mappedPort.Port(),
		DirectAddr:   fmt.Sprintf("%s:%s", host, mappedPort.Port()),
		UpstreamAddr: fmt.Sprintf("host.docker.internal:%s", mappedPort.Port()),
	}

	i.containers[name] = info
	return info, nil
}

// GetContainer returns information about a registered container.
func (i *Infrastructure) GetContainer(name string) (*ContainerInfo, bool) {
	info, ok := i.containers[name]
	return info, ok
}

// CreateProxyFor creates a Toxiproxy proxy for a registered container.
// Returns the proxy listen address that clients should connect to.
func (i *Infrastructure) CreateProxyFor(containerName string, listenPort string) (*Proxy, error) {
	i.t.Helper()

	if i.toxiproxy == nil {
		return nil, ErrToxiproxyNotConfigured
	}

	info, ok := i.containers[containerName]
	if !ok {
		return nil, fmt.Errorf("container %s not registered", containerName)
	}

	proxyName := fmt.Sprintf("%s-proxy", containerName)
	// Use UpstreamAddr which uses host.docker.internal to reach the target from inside Toxiproxy container
	upstream := info.UpstreamAddr
	// Extract just the port number from the port ID (e.g., "8666/tcp" -> "8666")
	portNum := nat.Port(listenPort).Port()
	listen := fmt.Sprintf("0.0.0.0:%s", portNum)

	proxy, err := i.orch.CreateProxy(proxyName, upstream, listen)
	if err != nil {
		return nil, err
	}

	// Update container info with proxy address
	ctx := context.Background()
	toxiHost, _ := i.toxiproxy.Container.Host(ctx)
	mappedPort, _ := i.toxiproxy.Container.MappedPort(ctx, nat.Port(listenPort))
	info.ProxyListen = fmt.Sprintf("%s:%s", toxiHost, mappedPort.Port())

	i.proxies[containerName] = proxy
	return proxy, nil
}

// GetProxy returns the proxy for a container.
func (i *Infrastructure) GetProxy(containerName string) (*Proxy, bool) {
	proxy, ok := i.proxies[containerName]
	return proxy, ok
}

// Cleanup releases all resources held by the infrastructure.
func (i *Infrastructure) Cleanup() {
	i.t.Helper()
	ctx := context.Background()

	// Delete all proxies
	for name, proxy := range i.proxies {
		if err := proxy.Delete(); err != nil {
			i.t.Logf("failed to delete proxy %s: %v", name, err)
		}
	}

	// Terminate Toxiproxy
	if i.toxiproxy != nil {
		i.toxiproxy.Cleanup()
	}

	// Terminate all containers
	for name, info := range i.containers {
		if err := info.Container.Terminate(ctx); err != nil {
			i.t.Logf("failed to terminate container %s: %v", name, err)
		}
	}

	// Remove network
	if i.network != nil {
		if err := i.network.Remove(ctx); err != nil {
			i.t.Logf("failed to remove network: %v", err)
		}
	}

	// Close orchestrator
	if i.orch != nil {
		i.orch.Close()
	}
}

// ToxiproxyHost returns the Toxiproxy host address.
func (i *Infrastructure) ToxiproxyHost() string {
	if i.toxiproxy == nil {
		return ""
	}
	return i.toxiproxy.Host
}

// ToxiproxyClient returns the Toxiproxy client.
func (i *Infrastructure) ToxiproxyClient() *toxiproxyclient.Client {
	if i.toxiproxy == nil {
		return nil
	}
	return i.toxiproxy.Client
}
