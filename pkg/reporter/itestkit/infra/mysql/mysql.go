package mysql

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/itestkit"
)

type MySQLConfig struct {
	Name string

	Image        string
	Database     string
	Username     string
	Password     string
	RootPassword string

	EnableProxy bool
	ProxyName   string

	Options []MySQLOption
}

type MySQLEndpoint struct {
	DSN         string
	Upstream    string
	ProxyListen string
}

type MySQLInfra struct {
	cfg          MySQLConfig
	container    *tcmysql.MySQLContainer
	endpoint     *MySQLEndpoint
	networkAlias string // alias for internal network communication
	stubHost     string // used by stub to return raw host without normalization
	stubPort     int    // used by stub to return raw port
}

func NewMySQLInfra(cfg MySQLConfig) *MySQLInfra {
	if cfg.Image == "" {
		cfg.Image = "mysql:8.0"
	}

	if cfg.Database == "" {
		cfg.Database = "testdb"
	}

	if cfg.Username == "" {
		cfg.Username = "testuser"
	}

	if cfg.Password == "" {
		cfg.Password = "testpass"
	}

	if cfg.RootPassword == "" {
		cfg.RootPassword = cfg.Password
	}

	if cfg.Name == "" {
		cfg.Name = "default"
	}

	if cfg.ProxyName == "" {
		cfg.ProxyName = "mysql-" + cfg.Name
	}

	return &MySQLInfra{cfg: cfg}
}

func (m *MySQLInfra) Start(ctx context.Context, env *itestkit.Env) error {
	opts := defaultMySQLOptions()

	for _, opt := range m.cfg.Options {
		if opt != nil {
			opt(opts)
		}
	}

	// Build network alias based on infra name
	alias := fmt.Sprintf("mysql-%s", m.cfg.Name)

	runOpts := []testcontainers.ContainerCustomizer{
		testcontainers.WithImage(m.cfg.Image),
		tcmysql.WithDatabase(m.cfg.Database),
		tcmysql.WithUsername(m.cfg.Username),
		tcmysql.WithPassword(m.cfg.Password),
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

	c, err := tcmysql.Run(ctx, m.cfg.Image, runOpts...)
	if err != nil {
		return err
	}

	m.container = c

	host, err := c.Host(ctx)
	if err != nil {
		return err
	}

	port, err := c.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return err
	}

	upstream := fmt.Sprintf("%s:%s", host, port.Port())
	finalAddr := upstream
	proxyListen := ""

	if m.cfg.EnableProxy && env != nil && env.Chaos != nil {
		var proxyUpstream string
		if m.networkAlias != "" {
			proxyUpstream = fmt.Sprintf("%s:3306", m.networkAlias)
		} else {
			proxyUpstream = fmt.Sprintf("host.docker.internal:%s", port.Port())
		}

		ref, err := env.Chaos.CreateProxy(ctx, m.cfg.ProxyName, proxyUpstream)
		if err != nil {
			return err
		}

		finalAddr = ref.ListenAddr
		proxyListen = ref.ListenAddr
	}

	endpoint := MySQLEndpoint{
		Upstream:    upstream,
		ProxyListen: proxyListen,
		DSN:         fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", m.cfg.Username, m.cfg.Password, finalAddr, m.cfg.Database),
	}
	m.endpoint = &endpoint

	return nil
}

func (m *MySQLInfra) Endpoint() (MySQLEndpoint, error) {
	if m.endpoint == nil {
		return MySQLEndpoint{}, fmt.Errorf("mysql endpoint not ready")
	}

	return *m.endpoint, nil
}

func (m *MySQLInfra) DSN() (string, error) {
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
func (m *MySQLInfra) HostPort() (host string, port int, err error) {
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
		proxyHost, proxyPort, splitErr := net.SplitHostPort(endpoint.ProxyListen)
		if splitErr != nil {
			return "", 0, fmt.Errorf("invalid proxy address: %s: %w", endpoint.ProxyListen, splitErr)
		}

		portNum, _ := strconv.Atoi(proxyPort)

		return proxyHost, portNum, nil
	}

	// If in shared network, return network alias and internal port
	if m.networkAlias != "" {
		return m.networkAlias, 3306, nil
	}

	// Fallback: return upstream address normalized for Docker access
	upstreamHost, upstreamPort, splitErr := net.SplitHostPort(endpoint.Upstream)
	if splitErr != nil {
		return "", 0, fmt.Errorf("invalid address format: %s: %w", endpoint.Upstream, splitErr)
	}

	portNum, err := strconv.Atoi(upstreamPort)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", upstreamPort)
	}

	return itestkit.NormalizeHost(upstreamHost), portNum, nil
}

func (m *MySQLInfra) Terminate(ctx context.Context) error {
	if m.container != nil {
		return m.container.Terminate(ctx)
	}

	return nil
}

func (m *MySQLInfra) InfraKind() string { return "mysql" }
func (m *MySQLInfra) InfraName() string { return m.cfg.Name }

// NewMySQLInfraStub creates a MySQLInfra with a pre-configured endpoint.
// Use this when reusing existing infrastructure that was started separately.
// The stub doesn't manage a container, just provides connection details.
func NewMySQLInfraStub(cfg MySQLConfig, host string, port int) *MySQLInfra {
	m := NewMySQLInfra(cfg)
	upstream := fmt.Sprintf("%s:%d", host, port)
	m.endpoint = &MySQLEndpoint{
		Upstream: upstream,
		DSN:      fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", cfg.Username, cfg.Password, upstream, cfg.Database),
	}
	// Store the raw host for HostPort() to return without normalization
	m.stubHost = host
	m.stubPort = port

	return m
}
