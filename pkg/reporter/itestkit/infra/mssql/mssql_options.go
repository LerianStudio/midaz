package mssql

import (
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
)

type MSSQLOption func(*mssqlOptions)

type mssqlOptions struct {
	runOpts []testcontainers.ContainerCustomizer
}

func defaultMSSQLOptions() *mssqlOptions {
	return &mssqlOptions{runOpts: []testcontainers.ContainerCustomizer{}}
}

func WithMSSQLImage(image string) MSSQLOption {
	return func(o *mssqlOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithImage(image))
	}
}

func WithMSSQLEnv(key, value string) MSSQLOption {
	return func(o *mssqlOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithEnv(map[string]string{key: value}))
	}
}

// WithMSSQLFixedPort binds the SQL Server container to a specific host port.
// Use this for debugging scenarios where the local app needs to connect
// to the containerized database on a predictable port.
func WithMSSQLFixedPort(hostPort string) MSSQLOption {
	return func(o *mssqlOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithHostConfigModifier(
			func(hc *container.HostConfig) {
				if hc.PortBindings == nil {
					hc.PortBindings = network.PortMap{}
				}

				hc.PortBindings[network.MustParsePort("1433/tcp")] = []network.PortBinding{
					{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: hostPort},
				}
			},
		))
	}
}
