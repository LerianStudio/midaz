// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration || chaos

package chaos

import (
	"context"
	"fmt"
	"testing"
	"time"

	toxiproxyclient "github.com/Shopify/toxiproxy/v2/client"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tctoxiproxy "github.com/testcontainers/testcontainers-go/modules/toxiproxy"
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
}

// SetupToxiproxy starts a Toxiproxy container for network chaos testing.
func SetupToxiproxy(t *testing.T) *ToxiproxyResult {
	t.Helper()
	return SetupToxiproxyWithConfig(t, DefaultNetworkChaosConfig())
}

// SetupToxiproxyWithConfig starts a Toxiproxy container with custom configuration.
// Exposes ports 8666-8676 for dynamic proxy creation (in addition to 8474 for API).
func SetupToxiproxyWithConfig(t *testing.T, cfg NetworkChaosConfig) *ToxiproxyResult {
	t.Helper()

	ctx := context.Background()

	// Expose a range of ports for dynamic proxy creation
	// Ports 8666-8676 are commonly used for Toxiproxy proxies
	toxiContainer, err := tctoxiproxy.Run(ctx, cfg.Image,
		testcontainers.WithExposedPorts("8666/tcp", "8667/tcp", "8668/tcp", "8669/tcp", "8670/tcp"),
		// Add host.docker.internal mapping for Linux compatibility.
		// On macOS/Windows, Docker Desktop provides this automatically.
		// On Linux, we need to explicitly map it to the host gateway.
		testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
			hc.ExtraHosts = append(hc.ExtraHosts, "host.docker.internal:host-gateway")
		}),
	)
	require.NoError(t, err, "failed to start Toxiproxy container")

	host, err := toxiContainer.Host(ctx)
	require.NoError(t, err, "failed to get Toxiproxy host")

	apiPort, err := toxiContainer.MappedPort(ctx, "8474")
	require.NoError(t, err, "failed to get Toxiproxy API port")

	apiURL := fmt.Sprintf("http://%s:%s", host, apiPort.Port())
	client := toxiproxyclient.NewClient(apiURL)

	t.Cleanup(func() {
		if err := toxiContainer.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate Toxiproxy container: %v", err)
		}
	})

	return &ToxiproxyResult{
		Container: toxiContainer,
		Client:    client,
		Host:      host,
		APIPort:   apiPort.Port(),
	}
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
