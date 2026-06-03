package itestkit

import (
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var (
	hostGatewayIPOnce sync.Once
	hostGatewayIP     string
)

// HostGatewayIP returns the address that containers should use to reach the host.
// This is used to connect from containers to services running on the host.
// The result is cached.
//
// Resolution order:
//  1. TESTCONTAINERS_HOST_OVERRIDE env var (for CI/CD environments)
//  2. Docker Desktop: "host.docker.internal"
//  3. Linux bridge gateway IP (typically 172.17.0.1)
//  4. DNS lookup of host.docker.internal
//  5. Fallback to "host.docker.internal"
func HostGatewayIP() string {
	hostGatewayIPOnce.Do(func() {
		// Check for explicit override (useful in CI/CD with socket mount or remote Docker)
		if override := os.Getenv("TESTCONTAINERS_HOST_OVERRIDE"); override != "" {
			hostGatewayIP = override
			return
		}

		// Check if running on Docker Desktop
		out, err := exec.Command("docker", "info", "-f", "{{.OperatingSystem}}").Output()
		if err == nil {
			osName := strings.TrimSpace(string(out))
			if strings.Contains(strings.ToLower(osName), "docker desktop") {
				// Docker Desktop: use host.docker.internal directly
				// This works because e2ekit adds "host.docker.internal:host-gateway" as extraHost
				hostGatewayIP = "host.docker.internal"
				return
			}
		}

		// Linux: Try to get the Docker bridge gateway IP
		out, err = exec.Command("docker", "network", "inspect", "bridge", "-f", "{{range .IPAM.Config}}{{.Gateway}}{{end}}").Output()
		if err == nil {
			ip := strings.TrimSpace(string(out))
			if ip != "" && net.ParseIP(ip) != nil {
				hostGatewayIP = ip
				return
			}
		}

		// Fallback: try to resolve host.docker.internal to IPv4
		addrs, err := net.LookupIP("host.docker.internal")
		if err == nil {
			for _, addr := range addrs {
				if ipv4 := addr.To4(); ipv4 != nil {
					hostGatewayIP = ipv4.String()
					return
				}
			}
		}

		// Ultimate fallback
		hostGatewayIP = "host.docker.internal"
	})

	return hostGatewayIP
}

// NormalizeHost replaces localhost/loopback/wildcard addresses with the Docker gateway IP
// so containers can reach services running on the host.
//
// Addresses that are replaced:
//   - localhost, 127.0.0.1, ::1 (loopback)
//   - 0.0.0.0, :: (wildcard/all interfaces)
//
// This is automatically called by all infra HostPort() methods, so tests
// don't need to manually handle this conversion.
func NormalizeHost(host string) string {
	switch host {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "::":
		return HostGatewayIP()
	default:
		return host
	}
}
