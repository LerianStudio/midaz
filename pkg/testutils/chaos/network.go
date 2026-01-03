//go:build integration || chaos

package chaos

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	toxiproxyclient "github.com/Shopify/toxiproxy/v2/client"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tctoxiproxy "github.com/testcontainers/testcontainers-go/modules/toxiproxy"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NetworkChaosConfig holds configuration for network chaos operations.
type NetworkChaosConfig struct {
	// Image is the Toxiproxy container image.
	Image string
	// MemoryMB is the memory limit in MB.
	MemoryMB int64
	// CPULimit is the CPU limit in cores.
	CPULimit float64
}

// DefaultNetworkChaosConfig returns the default network chaos configuration.
func DefaultNetworkChaosConfig() NetworkChaosConfig {
	return NetworkChaosConfig{
		Image:    "ghcr.io/shopify/toxiproxy:2.12.0",
		MemoryMB: 128,
		CPULimit: 0.5,
	}
}

// ToxiproxyResult holds the result of starting a Toxiproxy container.
type ToxiproxyResult struct {
	Container testcontainers.Container
	Client    *toxiproxyclient.Client
	Host      string
	APIPort   string
	Cleanup   func()
}

// SetupToxiproxy starts a Toxiproxy container for network chaos testing.
func SetupToxiproxy(t *testing.T) *ToxiproxyResult {
	return SetupToxiproxyWithConfig(t, DefaultNetworkChaosConfig())
}

// SetupToxiproxyWithConfig starts a Toxiproxy container with custom configuration.
func SetupToxiproxyWithConfig(t *testing.T, cfg NetworkChaosConfig) *ToxiproxyResult {
	t.Helper()
	ctx := context.Background()

	container, err := tctoxiproxy.Run(ctx, cfg.Image)
	require.NoError(t, err, "failed to start Toxiproxy container")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get Toxiproxy host")

	apiPort, err := container.MappedPort(ctx, "8474")
	require.NoError(t, err, "failed to get Toxiproxy API port")

	apiURL := fmt.Sprintf("http://%s:%s", host, apiPort.Port())
	client := toxiproxyclient.NewClient(apiURL)

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate Toxiproxy container: %v", err)
		}
	}

	return &ToxiproxyResult{
		Container: container,
		Client:    client,
		Host:      host,
		APIPort:   apiPort.Port(),
		Cleanup:   cleanup,
	}
}

// ToxiproxyWithProxyResult extends ToxiproxyResult with pre-configured proxy information.
// Use this when you need Toxiproxy and target container on the same Docker network.
type ToxiproxyWithProxyResult struct {
	*ToxiproxyResult
	// Network is the shared Docker network for container-to-container communication.
	Network *testcontainers.DockerNetwork
	// ProxyHost is the host address to connect to the proxy from the test.
	ProxyHost string
	// ProxyPort is the port to connect to the proxy from the test.
	ProxyPort string
}

// ProxyConfig defines a proxy to be created with the Toxiproxy container.
type ProxyConfig struct {
	// Name is the proxy name (used to retrieve and manipulate the proxy).
	Name string
	// Upstream is the target service address (e.g., "rabbitmq:5672" on the shared network).
	Upstream string
}

// SetupToxiproxyWithProxy creates a Toxiproxy container with a pre-configured proxy on a shared network.
// This is the recommended approach for chaos testing as it handles Docker networking correctly.
// The upstream container must be on the same network for container-to-container communication.
func SetupToxiproxyWithProxy(t *testing.T, proxyCfg ProxyConfig, opts ...testcontainers.ContainerCustomizer) *ToxiproxyWithProxyResult {
	return SetupToxiproxyWithProxyAndConfig(t, DefaultNetworkChaosConfig(), proxyCfg, opts...)
}

// SetupToxiproxyWithProxyAndConfig creates a Toxiproxy container with custom configuration and a pre-configured proxy.
func SetupToxiproxyWithProxyAndConfig(t *testing.T, cfg NetworkChaosConfig, proxyCfg ProxyConfig, opts ...testcontainers.ContainerCustomizer) *ToxiproxyWithProxyResult {
	t.Helper()
	ctx := context.Background()

	// Create a shared Docker network for container-to-container communication
	sharedNetwork, err := network.New(ctx, network.WithCheckDuplicate())
	require.NoError(t, err, "failed to create shared Docker network")

	// Custom wait strategy with longer timeout for Docker Desktop networking delays
	// When using custom networks, port mappings may take longer to become available
	customWaitStrategy := wait.ForHTTP("/version").
		WithPort("8474/tcp").
		WithStatusCodeMatcher(func(status int) bool { return status == http.StatusOK }).
		WithStartupTimeout(2 * time.Minute).
		WithPollInterval(500 * time.Millisecond)

	// Build options: shared network + pre-configured proxy + custom wait strategy
	allOpts := []testcontainers.ContainerCustomizer{
		network.WithNetwork([]string{"toxiproxy"}, sharedNetwork),
		tctoxiproxy.WithProxy(proxyCfg.Name, proxyCfg.Upstream),
		testcontainers.WithWaitStrategy(customWaitStrategy),
	}
	allOpts = append(allOpts, opts...)

	// Create Toxiproxy container with the proxy pre-configured
	container, err := tctoxiproxy.Run(ctx, cfg.Image, allOpts...)
	require.NoError(t, err, "failed to start Toxiproxy container with proxy")

	// Get the proxied endpoint (host:port accessible from the test)
	// WithProxy allocates ports starting from 8666
	const firstProxyPort = 8666
	proxyHost, proxyPort, err := container.ProxiedEndpoint(firstProxyPort)
	require.NoError(t, err, "failed to get proxied endpoint for %s", proxyCfg.Name)

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get Toxiproxy host")

	apiPort, err := container.MappedPort(ctx, "8474")
	require.NoError(t, err, "failed to get Toxiproxy API port")

	apiURL := fmt.Sprintf("http://%s:%s", host, apiPort.Port())
	client := toxiproxyclient.NewClient(apiURL)

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate Toxiproxy container: %v", err)
		}
		if err := sharedNetwork.Remove(ctx); err != nil {
			t.Logf("failed to remove shared network: %v", err)
		}
	}

	return &ToxiproxyWithProxyResult{
		ToxiproxyResult: &ToxiproxyResult{
			Container: container,
			Client:    client,
			Host:      host,
			APIPort:   apiPort.Port(),
			Cleanup:   cleanup,
		},
		Network:   sharedNetwork,
		ProxyHost: proxyHost,
		ProxyPort: proxyPort,
	}
}

// NetworkName returns the name of the shared Docker network.
// Use this to add other containers to the same network.
func (r *ToxiproxyWithProxyResult) NetworkName() string {
	return r.Network.Name
}

// Proxy represents a Toxiproxy proxy configuration.
type Proxy struct {
	client *toxiproxyclient.Client
	proxy  *toxiproxyclient.Proxy
	t      *testing.T
}

// CreateProxy creates a new proxy in Toxiproxy.
// The proxy forwards traffic from listenAddr to upstream.
func (o *Orchestrator) CreateProxy(name, upstream, listen string) (*Proxy, error) {
	o.t.Helper()

	if o.toxiproxy == nil {
		return nil, ErrToxiproxyNotConfigured
	}

	o.t.Logf("Chaos: creating proxy %s (%s -> %s)", name, listen, upstream)

	proxy, err := o.toxiproxy.CreateProxy(name, listen, upstream)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy: %w", err)
	}

	return &Proxy{
		client: o.toxiproxy,
		proxy:  proxy,
		t:      o.t,
	}, nil
}

// GetProxy retrieves an existing proxy by name.
func (o *Orchestrator) GetProxy(name string) (*Proxy, error) {
	o.t.Helper()

	if o.toxiproxy == nil {
		return nil, ErrToxiproxyNotConfigured
	}

	proxy, err := o.toxiproxy.Proxy(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}

	return &Proxy{
		client: o.toxiproxy,
		proxy:  proxy,
		t:      o.t,
	}, nil
}

// AddLatency adds latency to the proxy.
// This simulates slow network connections.
func (p *Proxy) AddLatency(latency, jitter time.Duration) error {
	p.t.Helper()
	p.t.Logf("Chaos: adding latency to proxy %s (latency: %v, jitter: %v)", p.proxy.Name, latency, jitter)

	_, err := p.proxy.AddToxic("latency", "latency", "downstream", 1.0, toxiproxyclient.Attributes{
		"latency": int(latency.Milliseconds()),
		"jitter":  int(jitter.Milliseconds()),
	})
	return err
}

// AddPacketLoss adds packet loss to the proxy.
// Percent should be between 0 and 100.
func (p *Proxy) AddPacketLoss(percent float64) error {
	p.t.Helper()
	p.t.Logf("Chaos: adding packet loss to proxy %s (percent: %.1f%%)", p.proxy.Name, percent)

	_, err := p.proxy.AddToxic("packet_loss", "timeout", "downstream", float32(percent/100), toxiproxyclient.Attributes{
		"timeout": 0, // Drop immediately
	})
	return err
}

// AddBandwidthLimit adds bandwidth limit to the proxy.
// Rate is in KB/s.
func (p *Proxy) AddBandwidthLimit(rateKBps int64) error {
	p.t.Helper()
	p.t.Logf("Chaos: adding bandwidth limit to proxy %s (rate: %d KB/s)", p.proxy.Name, rateKBps)

	_, err := p.proxy.AddToxic("bandwidth", "bandwidth", "downstream", 1.0, toxiproxyclient.Attributes{
		"rate": rateKBps,
	})
	return err
}

// Disconnect disables the proxy, simulating network partition.
func (p *Proxy) Disconnect() error {
	p.t.Helper()
	p.t.Logf("Chaos: disconnecting proxy %s", p.proxy.Name)
	p.proxy.Enabled = false
	return p.proxy.Save()
}

// Reconnect enables the proxy, restoring connectivity.
func (p *Proxy) Reconnect() error {
	p.t.Helper()
	p.t.Logf("Chaos: reconnecting proxy %s", p.proxy.Name)
	p.proxy.Enabled = true
	return p.proxy.Save()
}

// RemoveAllToxics removes all toxics from the proxy.
func (p *Proxy) RemoveAllToxics() error {
	p.t.Helper()
	p.t.Logf("Chaos: removing all toxics from proxy %s", p.proxy.Name)

	toxics, err := p.proxy.Toxics()
	if err != nil {
		return err
	}

	for _, toxic := range toxics {
		if err := p.proxy.RemoveToxic(toxic.Name); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the proxy from Toxiproxy.
func (p *Proxy) Delete() error {
	p.t.Helper()
	p.t.Logf("Chaos: deleting proxy %s", p.proxy.Name)
	return p.proxy.Delete()
}

// Listen returns the listen address of the proxy.
func (p *Proxy) Listen() string {
	return p.proxy.Listen
}

// Upstream returns the upstream address of the proxy.
func (p *Proxy) Upstream() string {
	return p.proxy.Upstream
}
