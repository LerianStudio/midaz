package itestkit

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	toxiclient "github.com/Shopify/toxiproxy/v2/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// proxyPortRangeStart is the first port in the range for proxy allocation.
	proxyPortRangeStart = 10000
	// proxyPortRangeEnd is the last port in the range for proxy allocation.
	proxyPortRangeEnd = 10019
	// maxProxies is the maximum number of proxies that can be created.
	maxProxies = proxyPortRangeEnd - proxyPortRangeStart + 1
)

const toxiproxyAlias = "toxiproxy"

type toxiproxyChaos struct {
	container     testcontainers.Container
	client        *toxiclient.Client
	proxies       map[string]*toxiclient.Proxy
	proxyMappings map[string]string // proxyName -> host:port (mapped port on host)
	nextPort      atomic.Int32
	hostIP        string
	networkAlias  string // alias for internal network communication
}

func NewToxiproxyChaos(ctx context.Context, cfg ChaosConfig, networkName string) (ChaosInterface, error) {
	image := cfg.Image
	if image == "" {
		image = "ghcr.io/shopify/toxiproxy:2.9.0"
	}

	// Build list of exposed ports: API port + proxy ports range
	exposedPorts := []string{"8474/tcp"}
	for port := proxyPortRangeStart; port <= proxyPortRangeEnd; port++ {
		exposedPorts = append(exposedPorts, fmt.Sprintf("%d/tcp", port))
	}

	req := testcontainers.ContainerRequest{
		Image:        image,
		ExposedPorts: exposedPorts,
		WaitingFor:   wait.ForListeningPort("8474/tcp").WithStartupTimeout(30 * time.Second),
		ExtraHosts:   []string{"host.docker.internal:host-gateway"},
	}

	// Add to shared network if provided
	if networkName != "" {
		req.Networks = []string{networkName}
		req.NetworkAliases = map[string][]string{
			networkName: {toxiproxyAlias},
		}
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	host, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}

	port, err := c.MappedPort(ctx, "8474/tcp")
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}

	api := fmt.Sprintf("http://%s:%s", host, port.Port())

	tc := &toxiproxyChaos{
		container:     c,
		client:        toxiclient.NewClient(api),
		proxies:       make(map[string]*toxiclient.Proxy),
		proxyMappings: make(map[string]string),
		hostIP:        host,
		networkAlias:  toxiproxyAlias,
	}
	tc.nextPort.Store(proxyPortRangeStart)

	return tc, nil
}

func (t *toxiproxyChaos) CreateProxy(ctx context.Context, name string, upstream string) (ProxyRef, error) {
	// Allocate the next available port from our pre-exposed range
	internalPort := int(t.nextPort.Add(1) - 1)
	if internalPort > proxyPortRangeEnd {
		return ProxyRef{}, fmt.Errorf("too many proxies: max %d supported", maxProxies)
	}

	// Create proxy listening on the allocated port inside the container
	listenAddr := fmt.Sprintf("0.0.0.0:%d", internalPort)

	p, err := t.client.CreateProxy(name, listenAddr, upstream)
	if err != nil {
		return ProxyRef{}, err
	}

	t.proxies[name] = p

	// If we have a network alias, use internal address for container-to-container communication
	if t.networkAlias != "" {
		internalAddr := fmt.Sprintf("%s:%d", t.networkAlias, internalPort)
		t.proxyMappings[name] = internalAddr

		return ProxyRef{Name: name, ListenAddr: internalAddr, Upstream: upstream}, nil
	}

	// Fallback: Get the mapped port on the host for this internal port
	mappedPort, err := t.container.MappedPort(ctx, fmt.Sprintf("%d/tcp", internalPort))
	if err != nil {
		return ProxyRef{}, fmt.Errorf("get mapped port for proxy %s: %w", name, err)
	}

	// Store the mapping and return the host-accessible address
	hostAddr := fmt.Sprintf("%s:%s", t.hostIP, mappedPort.Port())
	t.proxyMappings[name] = hostAddr

	return ProxyRef{Name: name, ListenAddr: hostAddr, Upstream: upstream}, nil
}

func (t *toxiproxyChaos) AddLatency(ctx context.Context, proxyName string, latency, jitter time.Duration) error {
	p := t.proxies[proxyName]
	if p == nil {
		return fmt.Errorf("proxy not found: %s", proxyName)
	}

	_, err := p.AddToxic(
		"latency",
		"latency",
		"downstream",
		1.0,
		toxiclient.Attributes{
			"latency": int(latency / time.Millisecond),
			"jitter":  int(jitter / time.Millisecond),
		},
	)

	return err
}

func (t *toxiproxyChaos) CutConnection(ctx context.Context, proxyName string) error {
	p := t.proxies[proxyName]
	if p == nil {
		return fmt.Errorf("proxy not found: %s", proxyName)
	}

	_, err := p.AddToxic(
		"cut",
		"timeout",
		"downstream",
		1.0,
		toxiclient.Attributes{
			"timeout": 1,
		},
	)

	return err
}

func (t *toxiproxyChaos) AddTimeout(ctx context.Context, proxyName string, timeout time.Duration) error {
	p := t.proxies[proxyName]
	if p == nil {
		return fmt.Errorf("proxy not found: %s", proxyName)
	}

	_, err := p.AddToxic(
		"timeout",
		"timeout",
		"downstream",
		1.0,
		toxiclient.Attributes{
			"timeout": int(timeout / time.Millisecond),
		},
	)

	return err
}

func (t *toxiproxyChaos) AddBandwidth(ctx context.Context, proxyName string, rateKBps int64) error {
	p := t.proxies[proxyName]
	if p == nil {
		return fmt.Errorf("proxy not found: %s", proxyName)
	}

	_, err := p.AddToxic(
		"bandwidth",
		"bandwidth",
		"downstream",
		1.0,
		toxiclient.Attributes{
			"rate": rateKBps,
		},
	)

	return err
}

func (t *toxiproxyChaos) RemoveToxic(ctx context.Context, proxyName, toxicName string) error {
	p := t.proxies[proxyName]
	if p == nil {
		return fmt.Errorf("proxy not found: %s", proxyName)
	}

	return p.RemoveToxic(toxicName)
}

func (t *toxiproxyChaos) RemoveAllToxics(ctx context.Context, proxyName string) error {
	p := t.proxies[proxyName]
	if p == nil {
		return fmt.Errorf("proxy not found: %s", proxyName)
	}

	toxicsAny, err := p.Toxics()
	if err != nil {
		return err
	}

	// toxiclient.Toxics is defined as []Toxic
	for _, toxic := range toxicsAny {
		if toxic.Name == "" {
			continue
		}

		if err := p.RemoveToxic(toxic.Name); err != nil {
			return err
		}
	}

	return nil
}

func (t *toxiproxyChaos) Close(ctx context.Context) error {
	for _, p := range t.proxies {
		_ = p.Delete()
	}

	if t.container != nil {
		return t.container.Terminate(ctx)
	}

	return nil
}
