package oracle

import (
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
)

type OracleOption func(*oracleOptions)

type oracleOptions struct {
	runOpts []testcontainers.ContainerCustomizer
}

func defaultOracleOptions() *oracleOptions {
	return &oracleOptions{runOpts: []testcontainers.ContainerCustomizer{}}
}

func WithOracleImage(image string) OracleOption {
	return func(o *oracleOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithImage(image))
	}
}

func WithOracleEnv(key, value string) OracleOption {
	return func(o *oracleOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithEnv(map[string]string{key: value}))
	}
}

func WithOracleInitScript(hostPath, containerFileName string) OracleOption {
	return func(o *oracleOptions) {
		if containerFileName == "" {
			containerFileName = "init.sql"
		}

		o.runOpts = append(o.runOpts,
			testcontainers.WithFiles(
				testcontainers.ContainerFile{
					HostFilePath:      hostPath,
					ContainerFilePath: "/container-entrypoint-initdb.d/" + containerFileName,
					FileMode:          0o755,
				},
			),
		)
	}
}

// WithOracleFixedPort binds the Oracle container to a specific host port.
// Use this for debugging scenarios where the local app needs to connect
// to the containerized database on a predictable port.
func WithOracleFixedPort(hostPort string) OracleOption {
	return func(o *oracleOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithHostConfigModifier(
			func(hc *container.HostConfig) {
				if hc.PortBindings == nil {
					hc.PortBindings = network.PortMap{}
				}

				hc.PortBindings[network.MustParsePort("1521/tcp")] = []network.PortBinding{
					{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: hostPort},
				}
			},
		))
	}
}
