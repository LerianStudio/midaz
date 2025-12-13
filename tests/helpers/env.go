package helpers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	envDefaultHTTPTimeout    = 20 * time.Second
	envTCPDialTimeout        = 1 * time.Second
	envTCPPollInterval       = 300 * time.Millisecond
	envHTTPCheckTimeout      = 2 * time.Second
	envHTTPCheckPollInterval = 300 * time.Millisecond
	envDefaultHTTPSPort      = "443"
	envDefaultHTTPPort       = "80"
)

var (
	// ErrURLMissingHost indicates the URL is missing a host component
	ErrURLMissingHost = errors.New("missing host in url")
	// ErrWaitForHTTPTimeout indicates timeout waiting for HTTP 200
	ErrWaitForHTTPTimeout = errors.New("timeout waiting for HTTP 200")
)

// Environment holds base URLs for Midaz services and behavior flags.
type Environment struct {
	OnboardingURL  string
	TransactionURL string
	ManageStack    bool // if true, tests may start/stop stack via Makefile
	HTTPTimeout    time.Duration
}

// LoadEnvironment loads environment configuration with sensible defaults
// matching the local docker-compose setup.
func LoadEnvironment() Environment {
	onboarding := getenv("ONBOARDING_URL", "http://localhost:3000")
	transaction := getenv("TRANSACTION_URL", "http://localhost:3001")
	manage := getenv("MIDAZ_TEST_MANAGE_STACK", "false") == "true"
	timeoutStr := getenv("MIDAZ_TEST_HTTP_TIMEOUT", "20s")

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = envDefaultHTTPTimeout
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

// WaitForTCP waits until the given host:port is accepting connections or timeout elapses.
func WaitForTCP(hostPort string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	var dialer net.Dialer

	dialer.Timeout = envTCPDialTimeout

	for {
		ctx, cancel := context.WithTimeout(context.Background(), envTCPDialTimeout)
		conn, err := dialer.DialContext(ctx, "tcp", hostPort)

		cancel()

		if err == nil {
			_ = conn.Close()
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s: %w", hostPort, err)
		}

		time.Sleep(envTCPPollInterval)
	}
}

// URLHostPort extracts host:port from a base URL string.
// If no port is specified, it defaults based on scheme (https=443, http=80).
func URLHostPort(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	if u.Host == "" {
		return "", fmt.Errorf("%w: %s", ErrURLMissingHost, raw)
	}

	host := u.Hostname()
	port := u.Port()

	if port == "" {
		switch u.Scheme {
		case "https":
			port = envDefaultHTTPSPort
		case "http":
			port = envDefaultHTTPPort
		default:
			port = envDefaultHTTPPort
		}
	}

	return net.JoinHostPort(host, port), nil
}

// WaitForHTTP200 polls a URL until it returns HTTP 200 or timeout elapses.
func WaitForHTTP200(fullURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: envHTTPCheckTimeout}
	deadline := time.Now().Add(timeout)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), envHTTPCheckTimeout)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		resp, err := client.Do(req)

		cancel()

		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("timeout waiting for %s: %w", fullURL, err)
			}

			return fmt.Errorf("%w: %s (status != 200)", ErrWaitForHTTPTimeout, fullURL)
		}

		time.Sleep(envHTTPCheckPollInterval)
	}
}
