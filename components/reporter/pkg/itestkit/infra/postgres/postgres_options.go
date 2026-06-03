package postgres

import (
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
)

type PostgresOption func(*postgresOptions)

type postgresOptions struct {
	runOpts []testcontainers.ContainerCustomizer
}

func defaultPostgresOptions() *postgresOptions {
	return &postgresOptions{runOpts: []testcontainers.ContainerCustomizer{}}
}

func WithPGImage(image string) PostgresOption {
	return func(o *postgresOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithImage(image))
	}
}

func WithPGEnv(key, value string) PostgresOption {
	return func(o *postgresOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithEnv(map[string]string{key: value}))
	}
}

func WithPGCommand(cmd ...string) PostgresOption {
	return func(o *postgresOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithCmd(cmd...))
	}
}

func WithPGInitFile(hostPath string, containerFileName string) PostgresOption {
	return func(o *postgresOptions) {
		if containerFileName == "" {
			containerFileName = "init.sql"
		}

		o.runOpts = append(o.runOpts,
			testcontainers.WithFiles(
				testcontainers.ContainerFile{
					HostFilePath:      hostPath,
					ContainerFilePath: "/docker-entrypoint-initdb.d/" + containerFileName,
					FileMode:          0o755,
				},
			),
		)
	}
}

// WithPGFixedPort binds the PostgreSQL container to a specific host port.
// Use this for debugging scenarios where the local app needs to connect
// to the containerized database on a predictable port.
func WithPGFixedPort(hostPort string) PostgresOption {
	return func(o *postgresOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithHostConfigModifier(
			func(hc *container.HostConfig) {
				if hc.PortBindings == nil {
					hc.PortBindings = network.PortMap{}
				}

				hc.PortBindings[network.MustParsePort("5432/tcp")] = []network.PortBinding{
					{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: hostPort},
				}
			},
		))
	}
}
