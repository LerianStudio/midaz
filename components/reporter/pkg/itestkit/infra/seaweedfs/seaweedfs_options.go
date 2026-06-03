package seaweedfs

import (
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

// SeaweedFSOption is a functional option for configuring SeaweedFS.
type SeaweedFSOption func(*seaweedfsOptions)

type seaweedfsOptions struct {
	hostConfigModifiers []func(*container.HostConfig)
}

func defaultSeaweedFSOptions() *seaweedfsOptions {
	return &seaweedfsOptions{}
}

func WithSeaweedFSFixedPort(hostPort string) SeaweedFSOption {
	return func(o *seaweedfsOptions) {
		o.hostConfigModifiers = append(o.hostConfigModifiers, func(hc *container.HostConfig) {
			if hc.PortBindings == nil {
				hc.PortBindings = network.PortMap{}
			}

			hc.PortBindings[network.MustParsePort("8888/tcp")] = []network.PortBinding{
				{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: hostPort},
			}
		})
	}
}
