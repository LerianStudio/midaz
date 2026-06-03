package seaweedfs

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/itestkit"
)

const (
	defaultImage          = "chrislusf/seaweedfs:latest"
	defaultStartupTimeout = 60 * time.Second
)

// SeaweedFSConfig configures the SeaweedFS infrastructure.
type SeaweedFSConfig struct {
	Name string

	Image          string
	StartupTimeout time.Duration

	EnableProxy bool
	ProxyName   string

	Options []SeaweedFSOption
}

// SeaweedFSEndpoint holds connection details for the SeaweedFS cluster.
type SeaweedFSEndpoint struct {
	URL         string // http://host:port
	Host        string
	Port        string
	Upstream    string // direct address without proxy
	ProxyListen string // proxy address (empty if proxy disabled)
}

// SeaweedFSInfra manages a SeaweedFS cluster (master + volume + filer).
// Note: SeaweedFS uses its own internal network for master/volume/filer communication.
// External containers can reach the filer via the Docker gateway IP and mapped port.
type SeaweedFSInfra struct {
	cfg      SeaweedFSConfig
	master   testcontainers.Container
	volume   testcontainers.Container
	filer    testcontainers.Container
	endpoint *SeaweedFSEndpoint
	network  *testcontainers.DockerNetwork
}

// NewSeaweedFSInfra creates a new SeaweedFS infrastructure component.
func NewSeaweedFSInfra(cfg SeaweedFSConfig) *SeaweedFSInfra {
	if cfg.Image == "" {
		cfg.Image = defaultImage
	}

	if cfg.Name == "" {
		cfg.Name = "default"
	}

	if cfg.StartupTimeout == 0 {
		cfg.StartupTimeout = defaultStartupTimeout
	}

	if cfg.ProxyName == "" {
		cfg.ProxyName = "seaweed-" + cfg.Name
	}

	return &SeaweedFSInfra{cfg: cfg}
}

func (s *SeaweedFSInfra) Start(ctx context.Context, env *itestkit.Env) error {
	opts := defaultSeaweedFSOptions()

	for _, opt := range s.cfg.Options {
		if opt != nil {
			opt(opts)
		}
	}

	// Create a dedicated network for inter-container communication
	nw, err := network.New(ctx, network.WithDriver("bridge"))
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}

	s.network = nw

	networkName := nw.Name

	masterAlias := fmt.Sprintf("seaweedfs-master-%s", s.cfg.Name)
	volumeAlias := fmt.Sprintf("seaweedfs-volume-%s", s.cfg.Name)
	filerAlias := fmt.Sprintf("seaweedfs-filer-%s", s.cfg.Name)

	// Start Master
	masterReq := testcontainers.ContainerRequest{
		Image:        s.cfg.Image,
		ExposedPorts: []string{"9333/tcp"},
		Cmd:          []string{"master"},
		WaitingFor:   wait.ForHTTP("/cluster/status").WithPort("9333/tcp").WithStartupTimeout(s.cfg.StartupTimeout),
		Networks:     []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {masterAlias},
		},
	}

	master, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: masterReq,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("start master: %w", err)
	}

	s.master = master

	// Start Volume
	volumeReq := testcontainers.ContainerRequest{
		Image:        s.cfg.Image,
		ExposedPorts: []string{"8080/tcp"},
		Cmd:          []string{"volume", "-mserver=" + masterAlias + ":9333", "-port=8080"},
		WaitingFor:   wait.ForHTTP("/status").WithPort("8080/tcp").WithStartupTimeout(s.cfg.StartupTimeout),
		Networks:     []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {volumeAlias},
		},
	}

	volume, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: volumeReq,
		Started:          true,
	})
	if err != nil {
		_ = master.Terminate(ctx)
		_ = nw.Remove(ctx)

		return fmt.Errorf("start volume: %w", err)
	}

	s.volume = volume

	// Start Filer
	filerReq := testcontainers.ContainerRequest{
		Image:        s.cfg.Image,
		ExposedPorts: []string{"8888/tcp"},
		Cmd:          []string{"filer", "-master=" + masterAlias + ":9333"},
		WaitingFor:   wait.ForHTTP("/").WithPort("8888/tcp").WithStartupTimeout(s.cfg.StartupTimeout),
		Networks:     []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {filerAlias},
		},
		HostConfigModifier: func(hc *container.HostConfig) {
			for _, modifier := range opts.hostConfigModifiers {
				modifier(hc)
			}
		},
	}

	filer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: filerReq,
		Started:          true,
	})
	if err != nil {
		_ = volume.Terminate(ctx)
		_ = master.Terminate(ctx)
		_ = nw.Remove(ctx)

		return fmt.Errorf("start filer: %w", err)
	}

	s.filer = filer

	// Get filer endpoint
	host, err := filer.Host(ctx)
	if err != nil {
		return fmt.Errorf("get filer host: %w", err)
	}

	port, err := filer.MappedPort(ctx, "8888/tcp")
	if err != nil {
		return fmt.Errorf("get filer port: %w", err)
	}

	upstream := fmt.Sprintf("%s:%s", host, port.Port())
	finalAddr := upstream
	proxyListen := ""

	// Create proxy if enabled and chaos interface is available
	if s.cfg.EnableProxy && env != nil && env.Chaos != nil {
		// For the Toxiproxy container to reach the SeaweedFS container,
		// we need to use host.docker.internal which resolves to the Docker host
		proxyUpstream := fmt.Sprintf("host.docker.internal:%s", port.Port())

		ref, err := env.Chaos.CreateProxy(ctx, s.cfg.ProxyName, proxyUpstream)
		if err != nil {
			return fmt.Errorf("create seaweedfs proxy: %w", err)
		}

		finalAddr = ref.ListenAddr
		proxyListen = ref.ListenAddr
	}

	// Parse final address for host/port (using net.SplitHostPort to handle IPv6)
	finalHost, finalPort, err := net.SplitHostPort(finalAddr)
	if err != nil {
		return fmt.Errorf("parse seaweedfs address: %w", err)
	}

	s.endpoint = &SeaweedFSEndpoint{
		URL:         fmt.Sprintf("http://%s", finalAddr),
		Host:        finalHost,
		Port:        finalPort,
		Upstream:    upstream,
		ProxyListen: proxyListen,
	}

	return nil
}

// Endpoint returns the SeaweedFS endpoint details.
func (s *SeaweedFSInfra) Endpoint() (SeaweedFSEndpoint, error) {
	if s.endpoint == nil {
		return SeaweedFSEndpoint{}, fmt.Errorf("seaweedfs endpoint not ready")
	}

	return *s.endpoint, nil
}

// URL returns the SeaweedFS filer URL.
func (s *SeaweedFSInfra) URL() (string, error) {
	endpoint, err := s.Endpoint()
	if err != nil {
		return "", err
	}

	return endpoint.URL, nil
}

// HostPort returns the host and port as separate values.
// The host is automatically normalized so containers can reach it (localhost is replaced with
// the Docker gateway IP).
func (s *SeaweedFSInfra) HostPort() (host string, port int, err error) {
	endpoint, err := s.Endpoint()
	if err != nil {
		return "", 0, err
	}

	// Endpoint already has Host and Port separated
	portNum, err := strconv.Atoi(endpoint.Port)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", endpoint.Port)
	}

	return itestkit.NormalizeHost(endpoint.Host), portNum, nil
}

func (s *SeaweedFSInfra) Terminate(ctx context.Context) error {
	var errs []error

	if s.filer != nil {
		if err := s.filer.Terminate(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if s.volume != nil {
		if err := s.volume.Terminate(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if s.master != nil {
		if err := s.master.Terminate(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if s.network != nil {
		if err := s.network.Remove(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors terminating seaweedfs: %v", errs)
	}

	return nil
}

func (s *SeaweedFSInfra) InfraKind() string { return "seaweedfs" }
func (s *SeaweedFSInfra) InfraName() string { return s.cfg.Name }
