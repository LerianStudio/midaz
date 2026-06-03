// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"context"
	"fmt"

	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// ToxiproxyImage is the Docker image for the Toxiproxy container.
	ToxiproxyImage = "ghcr.io/shopify/toxiproxy:2.9.0"

	// ToxiproxyAPIPort is the API port for Toxiproxy management.
	ToxiproxyAPIPort = "8474/tcp"

	// Proxy names for each external dependency.
	ProxyNameMongoDB   = "mongodb"
	ProxyNameRabbitMQ  = "rabbitmq"
	ProxyNameValkey    = "valkey"
	ProxyNameSeaweedFS = "seaweedfs"

	// percentageDivisor converts an integer percentage (0-100) to a float fraction (0.0-1.0).
	percentageDivisor = 100.0
)

// ProxyConfig defines the upstream target for a Toxiproxy proxy.
type ProxyConfig struct {
	Name     string
	Listen   string // host:port inside Toxiproxy container
	Upstream string // host:port of the real service (container alias:port)
}

// ToxiproxyInfrastructure holds the Toxiproxy container and its managed proxies.
type ToxiproxyInfrastructure struct {
	Container testcontainers.Container
	Client    *toxiproxy.Client
	Proxies   map[string]*toxiproxy.Proxy
	Host      string
	APIPort   string
}

// StartToxiproxy creates and starts a Toxiproxy container on the given network.
// It returns a ToxiproxyInfrastructure that can be used to create and manage proxies.
func StartToxiproxy(ctx context.Context, networkName string) (*ToxiproxyInfrastructure, error) {
	req := testcontainers.ContainerRequest{
		Image:        ToxiproxyImage,
		ExposedPorts: []string{ToxiproxyAPIPort},
		Networks:     []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {"toxiproxy"},
		},
		WaitingFor: wait.ForHTTP("/version").WithPort("8474/tcp"),
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start toxiproxy container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("get toxiproxy host: %w", err)
	}

	mappedPort, err := ctr.MappedPort(ctx, "8474/tcp")
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("get toxiproxy mapped port: %w", err)
	}

	apiAddr := fmt.Sprintf("%s:%s", host, mappedPort.Port())
	client := toxiproxy.NewClient(apiAddr)

	return &ToxiproxyInfrastructure{
		Container: ctr,
		Client:    client,
		Proxies:   make(map[string]*toxiproxy.Proxy),
		Host:      host,
		APIPort:   mappedPort.Port(),
	}, nil
}

// CreateProxy creates a named proxy that routes traffic from a listen address
// inside the Toxiproxy container to the upstream service.
// The listen address should be "0.0.0.0:<port>" so it is accessible from outside
// the container via the mapped port.
func (t *ToxiproxyInfrastructure) CreateProxy(cfg ProxyConfig) (*toxiproxy.Proxy, error) {
	proxy, err := t.Client.CreateProxy(cfg.Name, cfg.Listen, cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("create proxy %s: %w", cfg.Name, err)
	}

	t.Proxies[cfg.Name] = proxy

	return proxy, nil
}

// GetProxy returns a previously created proxy by name.
func (t *ToxiproxyInfrastructure) GetProxy(name string) (*toxiproxy.Proxy, bool) {
	p, ok := t.Proxies[name]
	return p, ok
}

// InjectLatency adds a latency toxic to the given proxy.
// latencyMs is the base latency in milliseconds, jitterMs adds randomness.
func InjectLatency(proxy *toxiproxy.Proxy, latencyMs int, jitterMs int) error {
	_, err := proxy.AddToxic("latency_downstream", "latency", "downstream", 1.0, toxiproxy.Attributes{
		"latency": latencyMs,
		"jitter":  jitterMs,
	})
	if err != nil {
		return fmt.Errorf("add latency toxic to %s: %w", proxy.Name, err)
	}

	return nil
}

// InjectConnectionLoss simulates a complete connection loss by setting bandwidth to zero
// in both directions, effectively blocking all traffic.
func InjectConnectionLoss(proxy *toxiproxy.Proxy) error {
	_, err := proxy.AddToxic("bandwidth_downstream_zero", "bandwidth", "downstream", 1.0, toxiproxy.Attributes{
		"rate": 0,
	})
	if err != nil {
		return fmt.Errorf("add downstream bandwidth toxic to %s: %w", proxy.Name, err)
	}

	_, err = proxy.AddToxic("bandwidth_upstream_zero", "bandwidth", "upstream", 1.0, toxiproxy.Attributes{
		"rate": 0,
	})
	if err != nil {
		return fmt.Errorf("add upstream bandwidth toxic to %s: %w", proxy.Name, err)
	}

	return nil
}

// InjectPacketLoss simulates packet loss by dropping a percentage of data.
// percentLoss should be between 0 and 100.
func InjectPacketLoss(proxy *toxiproxy.Proxy, percentLoss int) error {
	if percentLoss < 0 || percentLoss > 100 {
		return fmt.Errorf("percentLoss must be between 0 and 100, got %d", percentLoss)
	}

	_, err := proxy.AddToxic("timeout_downstream", "timeout", "downstream", float32(percentLoss)/percentageDivisor, toxiproxy.Attributes{
		"timeout": 1,
	})
	if err != nil {
		return fmt.Errorf("add packet loss toxic to %s: %w", proxy.Name, err)
	}

	return nil
}

// RemoveAllToxics removes all toxics from the given proxy, restoring normal operation.
func RemoveAllToxics(proxy *toxiproxy.Proxy) error {
	toxics, err := proxy.Toxics()
	if err != nil {
		return fmt.Errorf("list toxics for %s: %w", proxy.Name, err)
	}

	for _, toxic := range toxics {
		if err := proxy.RemoveToxic(toxic.Name); err != nil {
			return fmt.Errorf("remove toxic %s from %s: %w", toxic.Name, proxy.Name, err)
		}
	}

	return nil
}

// DisableProxy disables the proxy entirely (simulates a hard connection cut).
func DisableProxy(proxy *toxiproxy.Proxy) error {
	proxy.Enabled = false

	return proxy.Save()
}

// EnableProxy re-enables the proxy (restores connectivity).
func EnableProxy(proxy *toxiproxy.Proxy) error {
	proxy.Enabled = true

	return proxy.Save()
}

// Terminate stops and removes the Toxiproxy container.
func (t *ToxiproxyInfrastructure) Terminate(ctx context.Context) error {
	if t.Container != nil {
		return t.Container.Terminate(ctx)
	}

	return nil
}
