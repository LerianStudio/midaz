package itestkit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/testcontainers/testcontainers-go"
)

func CImage(image string) Customizer { return testcontainers.WithImage(image) }

func CEnv(key, value string) Customizer {
	return testcontainers.WithEnv(map[string]string{key: value})
}

func CEnvs(env map[string]string) Customizer {
	cp := make(map[string]string, len(env))
	for k, v := range env {
		cp[k] = v
	}

	return testcontainers.WithEnv(cp)
}

func CCmd(cmd ...string) Customizer { return testcontainers.WithCmd(cmd...) }

func CExposedPorts(ports ...string) Customizer {
	return CustomizerFunc(func(r *testcontainers.GenericContainerRequest) error {
		r.ExposedPorts = uniqueAppendMany(r.ExposedPorts, ports...)
		return nil
	})
}

func CCopyFile(hostPath, containerPath string, mode int64) Customizer {
	return testcontainers.WithFiles(
		testcontainers.ContainerFile{
			HostFilePath:      hostPath,
			ContainerFilePath: containerPath,
			FileMode:          mode,
		},
	)
}

func CCopyDir(hostDir, containerDir string, mode int64) Customizer {
	return testcontainers.WithFiles(
		testcontainers.ContainerFile{
			HostFilePath:      hostDir,
			ContainerFilePath: containerDir,
			FileMode:          mode,
		},
	)
}

func CInitScriptDirEntryPoint(hostPath, containerInitDir string, mode int64) Customizer {
	name := filepath.Base(hostPath)
	return CCopyFile(hostPath, strings.TrimRight(containerInitDir, "/")+"/"+name, mode)
}

func CHostDockerInternal() Customizer {
	return CustomizerFunc(func(r *testcontainers.GenericContainerRequest) error {
		r.ExtraHosts = uniqueAppendMany(
			r.ExtraHosts,
			"host.docker.internal:host-gateway",
		)

		return nil
	})
}

func CAll(customizers ...Customizer) []Customizer {
	out := make([]Customizer, 0, len(customizers))
	for _, c := range customizers {
		if c != nil {
			out = append(out, c)
		}
	}

	return out
}

func CEnvFromOS(key string) Customizer {
	val, ok := os.LookupEnv(key)
	if !ok {
		return CustomizerFunc(func(*testcontainers.GenericContainerRequest) error { return nil })
	}

	return CEnv(key, val)
}

func CLabel(key, value string) Customizer {
	return testcontainers.WithLabels(map[string]string{key: value})
}

func CName(name string) Customizer { return testcontainers.WithName(name) }

func CNetworks(networks ...string) Customizer {
	return CustomizerFunc(func(r *testcontainers.GenericContainerRequest) error {
		r.Networks = uniqueAppendMany(r.Networks, networks...)
		return nil
	})
}

func CNetworkAliases(network string, aliases ...string) Customizer {
	return CustomizerFunc(func(r *testcontainers.GenericContainerRequest) error {
		if network == "" || len(aliases) == 0 {
			return nil
		}

		if r.NetworkAliases == nil {
			r.NetworkAliases = map[string][]string{}
		}

		existing := r.NetworkAliases[network]
		r.NetworkAliases[network] = uniqueAppendMany(existing, aliases...)

		return nil
	})
}

func CBindMount(hostPath, containerPath, mode string) Customizer {
	return CustomizerFunc(func(r *testcontainers.GenericContainerRequest) error {
		bind := fmt.Sprintf("%s:%s:%s", hostPath, containerPath, mode)
		r.Binds = uniqueAppendMany(r.Binds, bind)

		return nil
	})
}

func uniqueAppendMany(list []string, vals ...string) []string {
	exists := make(map[string]struct{}, len(list))
	for _, v := range list {
		exists[v] = struct{}{}
	}

	for _, v := range vals {
		if v == "" {
			continue
		}

		if _, ok := exists[v]; ok {
			continue
		}

		exists[v] = struct{}{}
		list = append(list, v)
	}

	return list
}
