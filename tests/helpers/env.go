// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package helpers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	defaultHTTPTimeout      = 20 * time.Second
	waitForHTTPClientTimout = 2 * time.Second
	waitForTCPDialTimeout   = 1 * time.Second
	waitPollInterval        = 300 * time.Millisecond
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
	onboarding := getenv("ONBOARDING_URL", "http://localhost:3010")
	transaction := getenv("TRANSACTION_URL", "http://localhost:3010")
	manage := getenv("MIDAZ_TEST_MANAGE_STACK", "false") == "true"
	timeoutStr := getenv("MIDAZ_TEST_HTTP_TIMEOUT", "20s")

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = defaultHTTPTimeout
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

	for {
		dialer := &net.Dialer{Timeout: waitForTCPDialTimeout}

		conn, err := dialer.DialContext(context.Background(), "tcp", hostPort)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s: %w", hostPort, err)
		}

		time.Sleep(waitPollInterval)
	}
}

// URLHostPort extracts host:port from a base URL string.
// If no port is specified, it defaults based on scheme (https=443, http=80).
func URLHostPort(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}

	if u.Host == "" {
		return "", fmt.Errorf("missing host in url: %s", raw) //nolint:err113 // dynamic error with context info
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
func WaitForHTTP200(fullURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: waitForHTTPClientTimout}
	deadline := time.Now().Add(timeout)

	for {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, fullURL, http.NoBody)

		resp, err := client.Do(req) //nolint:gosec // G704: SSRF intentional in test helper
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

			return fmt.Errorf("timeout waiting for %s: status != 200", fullURL) //nolint:err113 // dynamic error with context info
		}

		time.Sleep(waitPollInterval)
	}
}
