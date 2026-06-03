package rabbitmq

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/testcontainers/testcontainers-go"
	rmq "github.com/testcontainers/testcontainers-go/modules/rabbitmq"

	"github.com/LerianStudio/reporter/pkg/itestkit"
)

type RabbitConfig struct {
	Name string

	Image       string
	Username    string
	Password    string
	VHost       string
	EnableProxy bool
	ProxyName   string

	Options []RabbitOption
}

type RabbitEndpoint struct {
	AMQPURL     string
	Upstream    string
	ProxyListen string
}

type RabbitInfra struct {
	cfg          RabbitConfig
	container    *rmq.RabbitMQContainer
	endpoint     *RabbitEndpoint
	networkAlias string // alias for internal network communication
}

func NewRabbitInfra(cfg RabbitConfig) *RabbitInfra {
	if cfg.Image == "" {
		cfg.Image = "rabbitmq:3.13-management-alpine"
	}

	if cfg.Username == "" {
		cfg.Username = "guest"
	}

	if cfg.Password == "" {
		cfg.Password = "guest"
	}

	if cfg.VHost == "" {
		cfg.VHost = "/"
	}

	if cfg.Name == "" {
		cfg.Name = "default"
	}

	if cfg.ProxyName == "" {
		cfg.ProxyName = "amqp-" + cfg.Name
	}

	return &RabbitInfra{cfg: cfg}
}

func (r *RabbitInfra) Start(ctx context.Context, env *itestkit.Env) error {
	opts := defaultRabbitOptions()

	for _, opt := range r.cfg.Options {
		if opt != nil {
			opt(opts)
		}
	}

	// Build network alias based on infra name
	alias := fmt.Sprintf("rabbitmq-%s", r.cfg.Name)

	runOpts := []testcontainers.ContainerCustomizer{
		rmq.WithAdminUsername(r.cfg.Username),
		rmq.WithAdminPassword(r.cfg.Password),
	}

	// Add to shared network if available
	if env != nil && env.Network != "" {
		runOpts = append(runOpts,
			itestkit.CNetworks(env.Network),
			itestkit.CNetworkAliases(env.Network, alias),
		)
		r.networkAlias = alias
	}

	runOpts = append(runOpts, opts.runOpts...)

	c, err := rmq.Run(ctx, r.cfg.Image, runOpts...)
	if err != nil {
		return err
	}

	r.container = c

	host, err := c.Host(ctx)
	if err != nil {
		return err
	}

	amqpPort, err := c.MappedPort(ctx, "5672/tcp")
	if err != nil {
		return err
	}

	upstream := fmt.Sprintf("%s:%s", host, amqpPort.Port())
	finalAddr := upstream
	proxyListen := ""

	if r.cfg.EnableProxy && env != nil && env.Chaos != nil {
		// Use the container's network alias for proxy upstream when in shared network
		var proxyUpstream string
		if r.networkAlias != "" {
			proxyUpstream = fmt.Sprintf("%s:5672", r.networkAlias)
		} else {
			// Fallback to host.docker.internal for backward compatibility
			proxyUpstream = fmt.Sprintf("host.docker.internal:%s", amqpPort.Port())
		}

		ref, err := env.Chaos.CreateProxy(ctx, r.cfg.ProxyName, proxyUpstream)
		if err != nil {
			return err
		}

		finalAddr = ref.ListenAddr
		proxyListen = ref.ListenAddr
	}

	endpoint := RabbitEndpoint{
		Upstream:    upstream,
		ProxyListen: proxyListen,
		AMQPURL:     fmt.Sprintf("amqp://%s:%s@%s%s", r.cfg.Username, r.cfg.Password, finalAddr, r.cfg.VHost),
	}
	r.endpoint = &endpoint

	return nil
}

func (r *RabbitInfra) Endpoint() (RabbitEndpoint, error) {
	if r.endpoint == nil {
		return RabbitEndpoint{}, fmt.Errorf("rabbitmq endpoint not ready")
	}

	return *r.endpoint, nil
}

func (r *RabbitInfra) AMQPURL() (string, error) {
	endpoint, err := r.Endpoint()
	if err != nil {
		return "", err
	}

	return endpoint.AMQPURL, nil
}

// HostPort returns the host and port as separate values.
// If a proxy is configured, returns the proxy address.
// If in a shared network (no proxy), returns the network alias and internal port.
// Otherwise returns the upstream address normalized for Docker access.
func (r *RabbitInfra) HostPort() (host string, port int, err error) {
	endpoint, err := r.Endpoint()
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
	if r.networkAlias != "" {
		return r.networkAlias, 5672, nil
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

func (r *RabbitInfra) Terminate(ctx context.Context) error {
	if r.container != nil {
		return r.container.Terminate(ctx)
	}

	return nil
}

func (r *RabbitInfra) InfraKind() string { return "rabbitmq" }
func (r *RabbitInfra) InfraName() string { return r.cfg.Name }
