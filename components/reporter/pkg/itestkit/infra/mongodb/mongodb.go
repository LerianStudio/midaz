package mongodb

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/testcontainers/testcontainers-go"
	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"

	"github.com/LerianStudio/reporter/pkg/itestkit"
)

type MongoDBConfig struct {
	Name string

	Image    string
	Username string
	Password string

	EnableProxy bool
	ProxyName   string

	Options []MongoDBOption
}

type MongoDBEndpoint struct {
	URI         string
	Upstream    string
	ProxyListen string
}

type MongoDBInfra struct {
	cfg          MongoDBConfig
	container    *tcmongo.MongoDBContainer
	endpoint     *MongoDBEndpoint
	networkAlias string // alias for internal network communication
	stubHost     string // used by stub to return raw host without normalization
	stubPort     int    // used by stub to return raw port
}

func NewMongoDBInfra(cfg MongoDBConfig) *MongoDBInfra {
	if cfg.Image == "" {
		cfg.Image = "mongo:7"
	}

	if cfg.Name == "" {
		cfg.Name = "default"
	}

	if cfg.ProxyName == "" {
		cfg.ProxyName = "mongo-" + cfg.Name
	}

	return &MongoDBInfra{cfg: cfg}
}

func (m *MongoDBInfra) Start(ctx context.Context, env *itestkit.Env) error {
	opts := defaultMongoDBOptions()

	for _, opt := range m.cfg.Options {
		if opt != nil {
			opt(opts)
		}
	}

	// Build network alias based on infra name
	alias := fmt.Sprintf("mongodb-%s", m.cfg.Name)

	runOpts := []testcontainers.ContainerCustomizer{
		testcontainers.WithImage(m.cfg.Image),
	}

	if m.cfg.Username != "" && m.cfg.Password != "" {
		runOpts = append(runOpts,
			tcmongo.WithUsername(m.cfg.Username),
			tcmongo.WithPassword(m.cfg.Password),
		)
	}

	// Add to shared network if available
	if env != nil && env.Network != "" {
		runOpts = append(runOpts,
			itestkit.CNetworks(env.Network),
			itestkit.CNetworkAliases(env.Network, alias),
		)
		m.networkAlias = alias
	}

	runOpts = append(runOpts, opts.runOpts...)

	c, err := tcmongo.Run(ctx, m.cfg.Image, runOpts...)
	if err != nil {
		return err
	}

	m.container = c

	host, err := c.Host(ctx)
	if err != nil {
		return err
	}

	port, err := c.MappedPort(ctx, "27017/tcp")
	if err != nil {
		return err
	}

	upstream := fmt.Sprintf("%s:%s", host, port.Port())
	finalAddr := upstream
	proxyListen := ""

	if m.cfg.EnableProxy && env != nil && env.Chaos != nil {
		// Use the container's network alias for proxy upstream when in shared network
		var proxyUpstream string
		if m.networkAlias != "" {
			proxyUpstream = fmt.Sprintf("%s:27017", m.networkAlias)
		} else {
			// Fallback to host.docker.internal for backward compatibility
			proxyUpstream = fmt.Sprintf("host.docker.internal:%s", port.Port())
		}

		ref, err := env.Chaos.CreateProxy(ctx, m.cfg.ProxyName, proxyUpstream)
		if err != nil {
			return err
		}

		finalAddr = ref.ListenAddr
		proxyListen = ref.ListenAddr
	}

	var uri string
	if m.cfg.Username != "" && m.cfg.Password != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s", m.cfg.Username, m.cfg.Password, finalAddr)
	} else {
		uri = fmt.Sprintf("mongodb://%s", finalAddr)
	}

	endpoint := MongoDBEndpoint{
		Upstream:    upstream,
		ProxyListen: proxyListen,
		URI:         uri,
	}
	m.endpoint = &endpoint

	return nil
}

func (m *MongoDBInfra) Endpoint() (MongoDBEndpoint, error) {
	if m.endpoint == nil {
		return MongoDBEndpoint{}, fmt.Errorf("mongodb endpoint not ready")
	}

	return *m.endpoint, nil
}

func (m *MongoDBInfra) URI() (string, error) {
	endpoint, err := m.Endpoint()
	if err != nil {
		return "", err
	}

	return endpoint.URI, nil
}

// HostPort returns the host and port as separate values.
// If a proxy is configured, returns the proxy address.
// If in a shared network (no proxy), returns the network alias and internal port.
// Otherwise returns the upstream address normalized for Docker access.
func (m *MongoDBInfra) HostPort() (host string, port int, err error) {
	// If stub values are set, return them directly without normalization
	if m.stubHost != "" {
		return m.stubHost, m.stubPort, nil
	}

	endpoint, err := m.Endpoint()
	if err != nil {
		return "", 0, err
	}

	// If proxy is configured, return proxy address
	if endpoint.ProxyListen != "" {
		hostStr, portStr, err := net.SplitHostPort(endpoint.ProxyListen)
		if err != nil {
			return "", 0, fmt.Errorf("invalid proxy address: %s: %w", endpoint.ProxyListen, err)
		}

		portNum, parseErr := strconv.Atoi(portStr)
		if parseErr != nil {
			return "", 0, fmt.Errorf("invalid proxy port %q: %w", portStr, parseErr)
		}

		return hostStr, portNum, nil
	}

	// If in shared network, return network alias and internal port
	if m.networkAlias != "" {
		return m.networkAlias, 27017, nil
	}

	// Fallback: return upstream address normalized for Docker access
	hostStr, portStr, err := net.SplitHostPort(endpoint.Upstream)
	if err != nil {
		return "", 0, fmt.Errorf("invalid address format: %s: %w", endpoint.Upstream, err)
	}

	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", portStr)
	}

	return itestkit.NormalizeHost(hostStr), portNum, nil
}

func (m *MongoDBInfra) Terminate(ctx context.Context) error {
	if m.container != nil {
		return m.container.Terminate(ctx)
	}

	return nil
}

func (m *MongoDBInfra) InfraKind() string { return "mongodb" }
func (m *MongoDBInfra) InfraName() string { return m.cfg.Name }

// NewMongoDBInfraStub creates a MongoDBInfra with a pre-configured endpoint.
// Use this when reusing existing infrastructure that was started separately.
// The stub doesn't manage a container, just provides connection details.
func NewMongoDBInfraStub(cfg MongoDBConfig, host string, port int) *MongoDBInfra {
	m := NewMongoDBInfra(cfg)
	upstream := fmt.Sprintf("%s:%d", host, port)

	var uri string
	if cfg.Username != "" && cfg.Password != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s", cfg.Username, cfg.Password, upstream)
	} else {
		uri = fmt.Sprintf("mongodb://%s", upstream)
	}

	m.endpoint = &MongoDBEndpoint{
		Upstream: upstream,
		URI:      uri,
	}
	// Store the raw host for HostPort() to return without normalization
	m.stubHost = host
	m.stubPort = port

	return m
}
