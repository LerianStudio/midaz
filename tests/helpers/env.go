// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
// This file contains environment configuration and service URL management.
package helpers

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Environment holds the base URLs for the Midaz services and configuration flags
// that control test behavior.
type Environment struct {
	OnboardingURL  string
	TransactionURL string
	ManageStack    bool // if true, tests may start/stop stack via Makefile
	HTTPTimeout    time.Duration
}

// LoadEnvironment reads the test environment configuration from environment variables,
// providing sensible defaults that match the local docker-compose setup.
func LoadEnvironment() Environment {
	onboarding := getenv("ONBOARDING_URL", "http://localhost:3000")
	transaction := getenv("TRANSACTION_URL", "http://localhost:3001")
	manage := getenv("MIDAZ_TEST_MANAGE_STACK", "false") == "true"
	timeoutStr := getenv("MIDAZ_TEST_HTTP_TIMEOUT", "20s")

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 20 * time.Second
	}

	return Environment{
		OnboardingURL:  onboarding,
		TransactionURL: transaction,
		ManageStack:    manage,
		HTTPTimeout:    timeout,
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

// WaitForTCP polls a given host:port until it accepts a TCP connection or until
// a timeout is reached.
func WaitForTCP(hostPort string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", hostPort, 1*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s: %w", hostPort, err)
		}

		time.Sleep(300 * time.Millisecond)
	}
}

// URLHostPort extracts the host:port combination from a raw URL string.
// It defaults to port 80 for HTTP and 443 for HTTPS if no port is specified.
func URLHostPort(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	if u.Host == "" {
		return "", fmt.Errorf("missing host in url: %s", raw)
	}

	host := u.Hostname()
	port := u.Port()

	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			port = "80"
		}
	}

	return net.JoinHostPort(host, port), nil
}

// WaitForHTTP200 polls a URL until it returns an HTTP 200 OK status code or
// until a timeout is reached.
func WaitForHTTP200(fullURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for {
		req, _ := http.NewRequest(http.MethodGet, fullURL, nil)

		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("timeout waiting for %s: %v", fullURL, err)
			}

			return fmt.Errorf("timeout waiting for %s: status != 200", fullURL)
		}

		time.Sleep(300 * time.Millisecond)
	}
}
