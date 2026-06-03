package rabbitmq

import (
	"io"
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
)

type RabbitOption func(*rabbitOptions)

type rabbitOptions struct {
	runOpts []testcontainers.ContainerCustomizer
}

func defaultRabbitOptions() *rabbitOptions {
	return &rabbitOptions{runOpts: []testcontainers.ContainerCustomizer{}}
}

func WithRabbitImage(image string) RabbitOption {
	return func(o *rabbitOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithImage(image))
	}
}

func WithRabbitEnv(key, value string) RabbitOption {
	return func(o *rabbitOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithEnv(map[string]string{key: value}))
	}
}

func WithRabbitCommand(cmd ...string) RabbitOption {
	return func(o *rabbitOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithCmd(cmd...))
	}
}

func WithRabbitFixedPort(hostPort string) RabbitOption {
	return func(o *rabbitOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithHostConfigModifier(
			func(hc *container.HostConfig) {
				if hc.PortBindings == nil {
					hc.PortBindings = network.PortMap{}
				}

				hc.PortBindings[network.MustParsePort("5672/tcp")] = []network.PortBinding{
					{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: hostPort},
				}
			},
		))
	}
}

func WithRabbitDefinitions(hostPath string) RabbitOption {
	return func(o *rabbitOptions) {
		o.runOpts = append(o.runOpts,
			testcontainers.WithFiles(
				testcontainers.ContainerFile{
					HostFilePath:      hostPath,
					ContainerFilePath: "/etc/rabbitmq/definitions.json",
					FileMode:          0o644,
				},
			),
			testcontainers.WithFiles(
				testcontainers.ContainerFile{
					Reader:            rabbitConfReader(),
					ContainerFilePath: "/etc/rabbitmq/conf.d/20-definitions.conf",
					FileMode:          0o644,
				},
			),
		)
	}
}

func rabbitConfReader() *configReader {
	return &configReader{content: "management.load_definitions = /etc/rabbitmq/definitions.json\n"}
}

type configReader struct {
	content string
	offset  int
}

func (r *configReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.content) {
		return 0, io.EOF
	}

	n = copy(p, r.content[r.offset:])
	r.offset += n

	return n, nil
}
