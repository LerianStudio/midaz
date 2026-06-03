package itestkit

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type ContainerEndpoint struct {
	Name string
	Host string

	Ports     map[string]string
	Upstreams map[string]string
	Proxies   map[string]ProxyRef
}

type ContainerSpec struct {
	Name  string
	Image string

	ExposedPorts []string

	Customizers []Customizer

	Wait WaitStrategy

	EnableProxy bool

	ProxyPrefix string
}

type WaitStrategy interface {
	Apply(req *testcontainers.ContainerRequest)
}

type WaitListeningPort struct {
	Port    string // ex: "5432/tcp"
	Timeout time.Duration
}

func (w WaitListeningPort) Apply(req *testcontainers.ContainerRequest) {
	if w.Timeout == 0 {
		w.Timeout = 30 * time.Second
	}

	req.WaitingFor = wait.ForListeningPort(w.Port).WithStartupTimeout(w.Timeout)
}

func (b *Builder) WithContainerCustomize(spec ContainerSpec) *Builder {
	if spec.Name == "" {
		spec.Name = "default"
	}

	b.infra = append(b.infra, NewGenericContainerInfra(spec))

	return b
}

type genericContainerInfra struct {
	spec ContainerSpec
	c    testcontainers.Container
}

func NewGenericContainerInfra(spec ContainerSpec) Infra {
	return &genericContainerInfra{spec: spec}
}

func (g *genericContainerInfra) Kind() string { return "container" }

func (g *genericContainerInfra) Name() string { return g.spec.Name }

func (g *genericContainerInfra) Start(ctx context.Context, env *Env) error {
	if env.Containers == nil {
		env.Containers = map[string]ContainerEndpoint{}
	}

	req := testcontainers.ContainerRequest{
		Image:        g.spec.Image,
		ExposedPorts: append([]string{}, g.spec.ExposedPorts...),
	}

	if g.spec.Wait != nil {
		g.spec.Wait.Apply(&req)
	}

	gr := testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	}

	for _, c := range g.spec.Customizers {
		if c == nil {
			continue
		}

		if err := c.Customize(&gr); err != nil {
			return fmt.Errorf("customizer failed for container %q: %w", g.spec.Name, err)
		}
	}

	cn, err := testcontainers.GenericContainer(ctx, gr)
	if err != nil {
		return err
	}

	g.c = cn

	host, err := cn.Host(ctx)
	if err != nil {
		return err
	}

	ep := ContainerEndpoint{
		Name:      g.spec.Name,
		Host:      host,
		Ports:     map[string]string{},
		Upstreams: map[string]string{},
		Proxies:   map[string]ProxyRef{},
	}

	for _, p := range g.spec.ExposedPorts {
		mp, err := cn.MappedPort(ctx, p)
		if err != nil {
			return err
		}

		ep.Ports[p] = mp.Port()
		ep.Upstreams[p] = fmt.Sprintf("%s:%s", host, mp.Port())
	}

	if g.spec.EnableProxy && env.Chaos != nil {
		prefix := g.spec.ProxyPrefix
		if prefix == "" {
			prefix = "container"
		}

		for _, p := range g.spec.ExposedPorts {
			proxyName := fmt.Sprintf("%s-%s-%s", prefix, g.spec.Name, portKey(p))
			upstream := ep.Upstreams[p]

			ref, err := env.Chaos.CreateProxy(ctx, proxyName, upstream)
			if err != nil {
				return err
			}

			ep.Proxies[p] = ref
		}
	}

	env.Containers[g.spec.Name] = ep

	return nil
}

func (g *genericContainerInfra) Terminate(ctx context.Context) error {
	if g.c != nil {
		return g.c.Terminate(ctx)
	}

	return nil
}

func portKey(p string) string {
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			return p[:i]
		}
	}

	return p
}
