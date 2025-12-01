// Package helpers provides environment configuration for Midaz integration tests.
//
// # Purpose
//
// This file provides environment configuration loading and service connectivity
// utilities for integration and E2E tests. It manages service URLs, timeouts,
// and test behavior flags.
//
// # Environment Variables
//
// Service URLs:
//   - ONBOARDING_URL: Onboarding service base URL (default: http://localhost:3000)
//   - TRANSACTION_URL: Transaction service base URL (default: http://localhost:3001)
//
// Test Behavior:
//   - MIDAZ_TEST_MANAGE_STACK: If "true", tests may start/stop stack via Makefile
//   - MIDAZ_TEST_HTTP_TIMEOUT: HTTP client timeout (default: 20s)
//
// # Usage
//
//	env := helpers.LoadEnvironment()
//	client := helpers.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
//
//	// Wait for service availability
//	hp, _ := helpers.URLHostPort(env.OnboardingURL)
//	if err := helpers.WaitForTCP(hp, 30*time.Second); err != nil {
//	    t.Fatal("Service not available")
//	}
//
// # Service Readiness
//
// Three readiness functions are provided:
//   - WaitForTCP: Wait for TCP port to accept connections
//   - WaitForHTTP200: Wait for HTTP 200 response from URL
//   - URLHostPort: Extract host:port from URL for TCP checks
package helpers

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Environment holds base URLs for Midaz services and behavior flags.
//
// # Fields
//
//   - OnboardingURL: Base URL for onboarding service (e.g., http://localhost:3000)
//   - TransactionURL: Base URL for transaction service (e.g., http://localhost:3001)
//   - ManageStack: If true, tests may start/stop services via Makefile
//   - HTTPTimeout: Timeout for HTTP client requests
//
// # Thread Safety
//
// Environment is immutable after creation and safe for concurrent read access.
type Environment struct {
	OnboardingURL  string
	TransactionURL string
	ManageStack    bool // if true, tests may start/stop stack via Makefile
	HTTPTimeout    time.Duration
}

// LoadEnvironment loads environment configuration with sensible defaults
// matching the local docker-compose setup.
//
// # Process
//
//	Step 1: Read ONBOARDING_URL (default: http://localhost:3000)
//	Step 2: Read TRANSACTION_URL (default: http://localhost:3001)
//	Step 3: Read MIDAZ_TEST_MANAGE_STACK (default: false)
//	Step 4: Read and parse MIDAZ_TEST_HTTP_TIMEOUT (default: 20s)
//
// # Returns
//
//   - Environment: Configured environment with defaults applied
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

// getenv returns the value of an environment variable or a default value.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

// WaitForTCP waits until the given host:port is accepting connections or timeout elapses.
//
// This function is useful for waiting for services to start accepting TCP connections
// before attempting HTTP requests.
//
// # Parameters
//
//   - hostPort: Host and port to connect to (e.g., "localhost:3000")
//   - timeout: Maximum time to wait for connection
//
// # Process
//
//	Step 1: Calculate deadline from current time + timeout
//	Step 2: Loop until deadline:
//	  - Attempt TCP connection with 1s timeout
//	  - If successful, close connection and return nil
//	  - Sleep 300ms between attempts
//	Step 3: Return timeout error if deadline exceeded
//
// # Returns
//
//   - nil: Connection successful
//   - error: Timeout with last connection error
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

// URLHostPort extracts host:port from a base URL string.
//
// This function parses a URL and returns the host:port suitable for TCP
// connection checks. If no port is specified, it defaults based on scheme.
//
// # Parameters
//
//   - raw: URL string to parse (e.g., "http://localhost:3000")
//
// # Returns
//
//   - string: Host:port string (e.g., "localhost:3000")
//   - error: URL parsing error or missing host
//
// # Default Ports
//
//   - https: 443
//   - http: 80
//   - other: 80
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

// WaitForHTTP200 polls a URL until it returns HTTP 200 or timeout elapses.
//
// This function is useful for waiting for HTTP services to become fully ready
// and responding to requests.
//
// # Parameters
//
//   - fullURL: Complete URL to poll (e.g., "http://localhost:3000/health")
//   - timeout: Maximum time to wait for 200 response
//
// # Process
//
//	Step 1: Create HTTP client with 2s per-request timeout
//	Step 2: Calculate deadline from current time + timeout
//	Step 3: Loop until deadline:
//	  - Send GET request to URL
//	  - If 200 OK, return nil
//	  - Sleep 300ms between attempts
//	Step 4: Return timeout error with last status or error
//
// # Returns
//
//   - nil: HTTP 200 received
//   - error: Timeout with last error or non-200 status
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
