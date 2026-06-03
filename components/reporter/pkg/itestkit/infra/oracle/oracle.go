package oracle

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/itestkit"
)

type OracleConfig struct {
	Name string

	Image    string
	Password string
	SID      string

	EnableProxy bool
	ProxyName   string

	Options []OracleOption
}

type OracleEndpoint struct {
	DSN         string
	Upstream    string
	ProxyListen string
}

type OracleInfra struct {
	cfg          OracleConfig
	container    testcontainers.Container
	endpoint     *OracleEndpoint
	networkAlias string // alias for internal network communication
	stubHost     string // used by stub to return raw host without normalization
	stubPort     int    // used by stub to return raw port
}

func NewOracleInfra(cfg OracleConfig) *OracleInfra {
	if cfg.Image == "" {
		cfg.Image = "gvenzl/oracle-xe:21-slim"
	}

	if cfg.Password == "" {
		cfg.Password = "testpass"
	}

	if cfg.SID == "" {
		cfg.SID = "XE"
	}

	if cfg.Name == "" {
		cfg.Name = "default"
	}

	if cfg.ProxyName == "" {
		cfg.ProxyName = "oracle-" + cfg.Name
	}

	return &OracleInfra{cfg: cfg}
}

func (o *OracleInfra) Start(ctx context.Context, env *itestkit.Env) error {
	opts := defaultOracleOptions()

	for _, opt := range o.cfg.Options {
		if opt != nil {
			opt(opts)
		}
	}

	// Build network alias based on infra name
	alias := fmt.Sprintf("oracle-%s", o.cfg.Name)

	req := testcontainers.ContainerRequest{
		Image:        o.cfg.Image,
		ExposedPorts: []string{"1521/tcp"},
		Env: map[string]string{
			"ORACLE_PASSWORD": o.cfg.Password,
		},
		WaitingFor: wait.ForLog("DATABASE IS READY TO USE!").WithStartupTimeout(5 * time.Minute),
	}

	// Add to shared network if available
	if env != nil && env.Network != "" {
		req.Networks = []string{env.Network}
		req.NetworkAliases = map[string][]string{
			env.Network: {alias},
		}
		o.networkAlias = alias
	}

	genericReq := testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	}

	for _, runOpt := range opts.runOpts {
		if err := runOpt.Customize(&genericReq); err != nil {
			return err
		}
	}

	c, err := testcontainers.GenericContainer(ctx, genericReq)
	if err != nil {
		return err
	}

	o.container = c

	host, err := c.Host(ctx)
	if err != nil {
		return err
	}

	port, err := c.MappedPort(ctx, "1521/tcp")
	if err != nil {
		return err
	}

	upstream := fmt.Sprintf("%s:%s", host, port.Port())
	finalAddr := upstream
	proxyListen := ""

	if o.cfg.EnableProxy && env != nil && env.Chaos != nil {
		// Use the container's network alias for proxy upstream when in shared network
		var proxyUpstream string
		if o.networkAlias != "" {
			proxyUpstream = fmt.Sprintf("%s:1521", o.networkAlias)
		} else {
			// Fallback to host.docker.internal for backward compatibility
			proxyUpstream = fmt.Sprintf("host.docker.internal:%s", port.Port())
		}

		ref, err := env.Chaos.CreateProxy(ctx, o.cfg.ProxyName, proxyUpstream)
		if err != nil {
			return err
		}

		finalAddr = ref.ListenAddr
		proxyListen = ref.ListenAddr
	}

	endpoint := OracleEndpoint{
		Upstream:    upstream,
		ProxyListen: proxyListen,
		DSN:         fmt.Sprintf("oracle://system:%s@%s/%s", o.cfg.Password, finalAddr, o.cfg.SID),
	}
	o.endpoint = &endpoint

	return nil
}

func (o *OracleInfra) Endpoint() (OracleEndpoint, error) {
	if o.endpoint == nil {
		return OracleEndpoint{}, fmt.Errorf("oracle endpoint not ready")
	}

	return *o.endpoint, nil
}

func (o *OracleInfra) DSN() (string, error) {
	endpoint, err := o.Endpoint()
	if err != nil {
		return "", err
	}

	return endpoint.DSN, nil
}

func (o *OracleInfra) GoDRORDSN() (string, error) {
	endpoint, err := o.Endpoint()
	if err != nil {
		return "", err
	}

	addr := endpoint.Upstream
	if endpoint.ProxyListen != "" {
		addr = endpoint.ProxyListen
	}

	return fmt.Sprintf("system/%s@%s/%s", o.cfg.Password, addr, o.cfg.SID), nil
}

// HostPort returns the host and port as separate values.
// If a proxy is configured, returns the proxy address.
// If in a shared network (no proxy), returns the network alias and internal port.
// Otherwise returns the upstream address normalized for Docker access.
func (o *OracleInfra) HostPort() (host string, port int, err error) {
	// If stub values are set, return them directly without normalization
	if o.stubHost != "" {
		return o.stubHost, o.stubPort, nil
	}

	endpoint, err := o.Endpoint()
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
	if o.networkAlias != "" {
		return o.networkAlias, 1521, nil
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

func (o *OracleInfra) Terminate(ctx context.Context) error {
	if o.container != nil {
		return o.container.Terminate(ctx)
	}

	return nil
}

func (o *OracleInfra) InfraKind() string { return "oracle" }
func (o *OracleInfra) InfraName() string { return o.cfg.Name }

// NewOracleInfraStub creates an OracleInfra with a pre-configured endpoint.
// Use this when reusing existing infrastructure that was started separately.
// The stub doesn't manage a container, just provides connection details.
func NewOracleInfraStub(cfg OracleConfig, host string, port int) *OracleInfra {
	o := NewOracleInfra(cfg)
	upstream := fmt.Sprintf("%s:%d", host, port)
	o.endpoint = &OracleEndpoint{
		Upstream: upstream,
		DSN:      fmt.Sprintf("oracle://system:%s@%s/%s", cfg.Password, upstream, cfg.SID),
	}
	// Store the raw host for HostPort() to return without normalization
	o.stubHost = host
	o.stubPort = port

	return o
}
