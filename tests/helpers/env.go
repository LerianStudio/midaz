package helpers

import (
    "fmt"
    "net"
    "net/url"
    "os"
    "time"
    "net/http"
)

// Environment holds base URLs for Midaz services and behavior flags.
type Environment struct {
    OnboardingURL    string
    TransactionURL   string
    ManageStack      bool // if true, tests may start/stop stack via Makefile
    HTTPTimeout      time.Duration
}

// LoadEnvironment loads environment configuration with sensible defaults
// matching the local docker-compose setup.
func LoadEnvironment() Environment {
    onboarding := getenv("ONBOARDING_URL", "http://localhost:3000")
    transaction := getenv("TRANSACTION_URL", "http://localhost:3001")
    manage := getenv("MIDAZ_TEST_MANAGE_STACK", "false") == "true"
    timeoutStr := getenv("MIDAZ_TEST_HTTP_TIMEOUT", "10s")
    timeout, err := time.ParseDuration(timeoutStr)
    if err != nil {
        timeout = 10 * time.Second
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
func URLHostPort(raw string) (string, error) {
    u, err := url.Parse(raw)
    if err != nil {
        return "", err
    }
    if u.Host == "" {
        return "", fmt.Errorf("missing host in url: %s", raw)
    }
    return u.Host, nil
}

// WaitForHTTP200 polls a URL until it returns HTTP 200 or timeout elapses.
func WaitForHTTP200(fullURL string, timeout time.Duration) error {
    client := &http.Client{ Timeout: 2 * time.Second }
    deadline := time.Now().Add(timeout)
    for {
        req, _ := http.NewRequest(http.MethodGet, fullURL, nil)
        resp, err := client.Do(req)
        if err == nil {
            _ = resp.Body.Close()
            if resp.StatusCode == http.StatusOK { return nil }
        }
        if time.Now().After(deadline) {
            if err != nil { return fmt.Errorf("timeout waiting for %s: %v", fullURL, err) }
            return fmt.Errorf("timeout waiting for %s: status != 200", fullURL)
        }
        time.Sleep(300 * time.Millisecond)
    }
}
