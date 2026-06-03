package mssql

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/testcontainers/testcontainers-go"
	tcmssql "github.com/testcontainers/testcontainers-go/modules/mssql"

	"github.com/LerianStudio/reporter/pkg/itestkit"
)

type MSSQLConfig struct {
	Name string

	Image    string
	Password string
	Database string

	EnableProxy bool
	ProxyName   string

	Options []MSSQLOption
}

type MSSQLEndpoint struct {
	DSN         string
	Upstream    string
	ProxyListen string
}

type MSSQLInfra struct {
	cfg          MSSQLConfig
	container    *tcmssql.MSSQLServerContainer
	endpoint     *MSSQLEndpoint
	networkAlias string // alias for internal network communication
	stubHost     string // used by stub to return raw host without normalization
	stubPort     int    // used by stub to return raw port
}

func NewMSSQLInfra(cfg MSSQLConfig) *MSSQLInfra {
	if cfg.Image == "" {
		cfg.Image = "mcr.microsoft.com/mssql/server:2022-latest"
	}

	if cfg.Password == "" {
		cfg.Password = "YourStrong@Passw0rd"
	}

	if cfg.Name == "" {
		cfg.Name = "default"
	}

	if cfg.ProxyName == "" {
		cfg.ProxyName = "mssql-" + cfg.Name
	}

	return &MSSQLInfra{cfg: cfg}
}

func (m *MSSQLInfra) Start(ctx context.Context, env *itestkit.Env) error {
	opts := defaultMSSQLOptions()

	for _, opt := range m.cfg.Options {
		if opt != nil {
			opt(opts)
		}
	}

	// Build network alias based on infra name
	alias := fmt.Sprintf("mssql-%s", m.cfg.Name)

	runOpts := []testcontainers.ContainerCustomizer{
		testcontainers.WithImage(m.cfg.Image),
		tcmssql.WithAcceptEULA(),
		tcmssql.WithPassword(m.cfg.Password),
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

	c, err := tcmssql.Run(ctx, m.cfg.Image, runOpts...)
	if err != nil {
		return err
	}

	m.container = c

	host, err := c.Host(ctx)
	if err != nil {
		return err
	}

	port, err := c.MappedPort(ctx, "1433/tcp")
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
			proxyUpstream = fmt.Sprintf("%s:1433", m.networkAlias)
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

	dsn := fmt.Sprintf("sqlserver://sa:%s@%s", m.cfg.Password, finalAddr)
	if m.cfg.Database != "" {
		dsn = fmt.Sprintf("%s?database=%s", dsn, m.cfg.Database)
	}

	endpoint := MSSQLEndpoint{
		Upstream:    upstream,
		ProxyListen: proxyListen,
		DSN:         dsn,
	}
	m.endpoint = &endpoint

	return nil
}

func (m *MSSQLInfra) Endpoint() (MSSQLEndpoint, error) {
	if m.endpoint == nil {
		return MSSQLEndpoint{}, fmt.Errorf("mssql endpoint not ready")
	}

	return *m.endpoint, nil
}

func (m *MSSQLInfra) DSN() (string, error) {
	endpoint, err := m.Endpoint()
	if err != nil {
		return "", err
	}

	return endpoint.DSN, nil
}

// HostPort returns the host and port as separate values.
// If a proxy is configured, returns the proxy address.
// If in a shared network (no proxy), returns the network alias and internal port.
// Otherwise returns the upstream address normalized for Docker access.
func (m *MSSQLInfra) HostPort() (host string, port int, err error) {
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

		portNum, _ := strconv.Atoi(portStr)

		return hostStr, portNum, nil
	}

	// If in shared network, return network alias and internal port
	if m.networkAlias != "" {
		return m.networkAlias, 1433, nil
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

func (m *MSSQLInfra) Terminate(ctx context.Context) error {
	if m.container != nil {
		return m.container.Terminate(ctx)
	}

	return nil
}

func (m *MSSQLInfra) InfraKind() string { return "mssql" }
func (m *MSSQLInfra) InfraName() string { return m.cfg.Name }

// NewMSSQLInfraStub creates a MSSQLInfra with a pre-configured endpoint.
// Use this when reusing existing infrastructure that was started separately.
// The stub doesn't manage a container, just provides connection details.
func NewMSSQLInfraStub(cfg MSSQLConfig, host string, port int) *MSSQLInfra {
	m := NewMSSQLInfra(cfg)
	upstream := fmt.Sprintf("%s:%d", host, port)

	dsn := fmt.Sprintf("sqlserver://sa:%s@%s", cfg.Password, upstream)
	if cfg.Database != "" {
		dsn = fmt.Sprintf("%s?database=%s", dsn, cfg.Database)
	}

	m.endpoint = &MSSQLEndpoint{
		Upstream: upstream,
		DSN:      dsn,
	}
	// Store the raw host for HostPort() to return without normalization
	m.stubHost = host
	m.stubPort = port

	return m
}
