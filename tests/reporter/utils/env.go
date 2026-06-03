// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

type Environment struct {
	ManagerURL  string
	HTTPTimeout time.Duration
	ManageStack bool

	// Optional infra identifiers for chaos
	RabbitContainer    string
	WorkerContainer    string
	MongoContainer     string
	RedisContainer     string
	SeaweedFSContainer string
}

func LoadEnvironment() Environment {
	mgr := getenvDefault("MANAGER_URL", "http://127.0.0.1:4005")
	timeoutStr := getenvDefault("HTTP_TIMEOUT_SECS", "30")

	secs, _ := strconv.Atoi(timeoutStr)
	if secs <= 0 {
		secs = 30
	}

	manage := getenvDefault("MANAGE_STACK", "false") == "true"

	env := Environment{
		ManagerURL:  mgr,
		HTTPTimeout: time.Duration(secs) * time.Second,
		ManageStack: manage,

		RabbitContainer:    getenvDefault("RABBIT_CONTAINER", "reporter-rabbitmq"),
		WorkerContainer:    getenvDefault("WORKER_CONTAINER", "reporter-worker"),
		MongoContainer:     getenvDefault("MONGO_CONTAINER", "reporter-mongodb"),
		RedisContainer:     getenvDefault("REDIS_CONTAINER", "reporter-valkey"),
		SeaweedFSContainer: getenvDefault("SEAWEEDFS_CONTAINER", "reporter-seaweedfs-filer"),
	}

	return env
}

// RequireReachable performs a single health check against the target service
// and fails the test immediately if the service is unreachable. This prevents
// fuzz and integration tests from silently passing as no-ops.
func RequireReachable(tb testing.TB, baseURL string) {
	tb.Helper()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		tb.Fatalf("preflight check failed: service unreachable at %s: %v", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		tb.Fatalf("preflight check failed: service unhealthy at %s: status %d", baseURL, resp.StatusCode)
	}
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}
