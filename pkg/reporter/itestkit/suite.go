package itestkit

import (
	"context"
	"fmt"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

type Suite struct {
	t       *testing.T
	infra   []Infra
	chaos   ChaosInterface
	env     *Env
	network *testcontainers.DockerNetwork
}

type Env struct {
	Containers map[string]ContainerEndpoint
	Chaos      ChaosInterface
	Network    string // Name of the shared Docker network for container communication
}

type Builder struct {
	t         *testing.T
	infra     []Infra
	chaosConf ChaosConfig
}

func New(t *testing.T) *Builder { //nolint:thelper // t can be nil when called from TestMain
	if t != nil {
		t.Helper()
	}

	return &Builder{
		t:     t,
		infra: make([]Infra, 0, 4),
		chaosConf: ChaosConfig{
			Enabled: false,
		},
	}
}

func (b *Builder) WithInfra(infra Infra) *Builder {
	if infra != nil {
		b.infra = append(b.infra, infra)
	}

	return b
}

func (b *Builder) WithInfras(infras ...Infra) *Builder {
	for _, infra := range infras {
		if infra == nil {
			continue
		}

		b.infra = append(b.infra, infra)
	}

	return b
}

func (b *Builder) WithChaos(cfg ChaosConfig) *Builder {
	b.chaosConf = cfg
	return b
}

func (b *Builder) Build(ctx context.Context) (*Suite, error) {
	if b.t != nil {
		b.t.Helper()
	}

	// Create shared Docker network for container communication
	nw, err := network.New(ctx, network.WithDriver("bridge"))
	if err != nil {
		return nil, fmt.Errorf("create network: %w", err)
	}

	var chaos ChaosInterface

	if b.chaosConf.Enabled {
		tc, err := NewToxiproxyChaos(ctx, b.chaosConf, nw.Name)
		if err != nil {
			_ = nw.Remove(ctx)
			return nil, err
		}

		chaos = tc
	}

	s := &Suite{
		t:       b.t,
		infra:   b.infra,
		chaos:   chaos,
		network: nw,
		env: &Env{
			Containers: map[string]ContainerEndpoint{},
			Chaos:      chaos,
			Network:    nw.Name,
		},
	}

	if err := validateUniqueInfraNames(s.infra); err != nil {
		_ = s.Terminate(ctx)
		return nil, err
	}

	for _, inf := range s.infra {
		if err := inf.Start(ctx, s.env); err != nil {
			_ = s.Terminate(ctx)
			return nil, err
		}
	}

	return s, nil
}

func (s *Suite) Env() *Env { return s.env }

func (s *Suite) Chaos() ChaosInterface { return s.chaos }

func (s *Suite) Terminate(ctx context.Context) error {
	for i := len(s.infra) - 1; i >= 0; i-- {
		_ = s.infra[i].Terminate(ctx)
	}

	if s.chaos != nil {
		_ = s.chaos.Close(ctx)
	}

	if s.network != nil {
		_ = s.network.Remove(ctx)
	}

	return nil
}

// Network returns the name of the shared Docker network.
// Use this to add additional containers to the same network.
func (s *Suite) Network() string {
	if s.env != nil {
		return s.env.Network
	}

	return ""
}
