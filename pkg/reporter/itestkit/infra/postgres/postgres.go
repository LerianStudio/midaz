package postgres

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/testcontainers/testcontainers-go"
	pg "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/itestkit"
)

type PostgresConfig struct {
	Name string

	Image    string
	Database string
	Username string
	Password string

	EnableProxy bool
	ProxyName   string

	Options []PostgresOption
}

type PostgresEndpoint struct {
	DSN         string
	Upstream    string
	ProxyListen string
}

type PostgresInfra struct {
	cfg          PostgresConfig
	container    *pg.PostgresContainer
	endpoint     *PostgresEndpoint
	networkAlias string // alias for internal network communication
	stubHost     string // used by stub to return raw host without normalization
	stubPort     int    // used by stub to return raw port
}

func NewPostgresInfra(cfg PostgresConfig) *PostgresInfra {
	if cfg.Image == "" {
		cfg.Image = "postgres:16-alpine"
	}

	if cfg.Database == "" {
		cfg.Database = "app"
	}

	if cfg.Username == "" {
		cfg.Username = "app"
	}

	if cfg.Password == "" {
		cfg.Password = "app"
	}

	if cfg.Name == "" {
		cfg.Name = "default"
	}

	if cfg.ProxyName == "" {
		cfg.ProxyName = "pg-" + cfg.Name
	}

	return &PostgresInfra{cfg: cfg}
}

func (p *PostgresInfra) Start(ctx context.Context, env *itestkit.Env) error {
	opts := defaultPostgresOptions()

	for _, opt := range p.cfg.Options {
		if opt != nil {
			opt(opts)
		}
	}

	// Build network alias based on infra name
	alias := fmt.Sprintf("postgres-%s", p.cfg.Name)

	runOpts := []testcontainers.ContainerCustomizer{
		testcontainers.WithImage(p.cfg.Image),
		pg.WithDatabase(p.cfg.Database),
		pg.WithUsername(p.cfg.Username),
		pg.WithPassword(p.cfg.Password),
	}

	// Add to shared network if available
	if env != nil && env.Network != "" {
		runOpts = append(runOpts,
			itestkit.CNetworks(env.Network),
			itestkit.CNetworkAliases(env.Network, alias),
		)
		p.networkAlias = alias
	}

	runOpts = append(runOpts, opts.runOpts...)

	c, err := pg.Run(ctx, p.cfg.Image, runOpts...)
	if err != nil {
		return err
	}

	p.container = c

	host, err := c.Host(ctx)
	if err != nil {
		return err
	}

	port, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return err
	}

	upstream := fmt.Sprintf("%s:%s", host, port.Port())
	finalAddr := upstream
	proxyListen := ""

	if p.cfg.EnableProxy && env != nil && env.Chaos != nil {
		// Use the container's network alias for proxy upstream when in shared network
		var proxyUpstream string
		if p.networkAlias != "" {
			proxyUpstream = fmt.Sprintf("%s:5432", p.networkAlias)
		} else {
			// Fallback to host.docker.internal for backward compatibility
			proxyUpstream = fmt.Sprintf("host.docker.internal:%s", port.Port())
		}

		ref, err := env.Chaos.CreateProxy(ctx, p.cfg.ProxyName, proxyUpstream)
		if err != nil {
			return err
		}

		finalAddr = ref.ListenAddr
		proxyListen = ref.ListenAddr
	}

	endpoint := PostgresEndpoint{
		Upstream:    upstream,
		ProxyListen: proxyListen,
		DSN:         fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", p.cfg.Username, p.cfg.Password, finalAddr, p.cfg.Database),
	}
	p.endpoint = &endpoint

	return nil
}

func (p *PostgresInfra) Endpoint() (PostgresEndpoint, error) {
	if p.endpoint == nil {
		return PostgresEndpoint{}, fmt.Errorf("postgres endpoint not ready")
	}

	return *p.endpoint, nil
}

func (p *PostgresInfra) DSN() (string, error) {
	endpoint, err := p.Endpoint()
	if err != nil {
		return "", err
	}

	return endpoint.DSN, nil
}

// HostPort returns the host and port as separate values.
// If a proxy is configured, returns the proxy address.
// If in a shared network (no proxy), returns the network alias and internal port.
// Otherwise returns the upstream address normalized for Docker access.
func (p *PostgresInfra) HostPort() (host string, port int, err error) {
	// If stub values are set, return them directly without normalization
	if p.stubHost != "" {
		return p.stubHost, p.stubPort, nil
	}

	endpoint, err := p.Endpoint()
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
	if p.networkAlias != "" {
		return p.networkAlias, 5432, nil
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

func (p *PostgresInfra) Terminate(ctx context.Context) error {
	if p.container != nil {
		return p.container.Terminate(ctx)
	}

	return nil
}

func (p *PostgresInfra) InfraKind() string { return "postgres" }
func (p *PostgresInfra) InfraName() string { return p.cfg.Name }

// NewPostgresInfraStub creates a PostgresInfra with a pre-configured endpoint.
// Use this when reusing existing infrastructure that was started separately.
// The stub doesn't manage a container, just provides connection details.
// Note: Sets networkAlias to empty to bypass NormalizeHost in HostPort().
func NewPostgresInfraStub(cfg PostgresConfig, host string, port int) *PostgresInfra {
	p := NewPostgresInfra(cfg)
	upstream := fmt.Sprintf("%s:%d", host, port)
	p.endpoint = &PostgresEndpoint{
		Upstream: upstream,
		DSN:      fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", cfg.Username, cfg.Password, upstream, cfg.Database),
	}
	// Store the raw host for HostPort() to return without normalization
	p.stubHost = host
	p.stubPort = port

	return p
}
