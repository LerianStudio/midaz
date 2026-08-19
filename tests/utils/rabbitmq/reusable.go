//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	testutils "github.com/LerianStudio/midaz/v4/tests/utils"

	"github.com/moby/moby/api/types/container"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type reusableRabbitMQServer struct {
	container testcontainers.Container
	host      string
	amqpPort  string
	mgmtPort  string
	config    ContainerConfig
	sequence  atomic.Uint64
}

var reusableRabbitMQServers = struct {
	sync.Mutex
	servers map[string]*reusableRabbitMQServer
}{servers: make(map[string]*reusableRabbitMQServer)}

// CleanupReusableContainers closes package-scoped fixture resources before a
// package-level leak audit. Ordinary test binaries can rely on process exit.
func CleanupReusableContainers() error {
	reusableRabbitMQServers.Lock()
	servers := reusableRabbitMQServers.servers
	reusableRabbitMQServers.servers = make(map[string]*reusableRabbitMQServer)
	reusableRabbitMQServers.Unlock()

	var cleanupErrors []error

	for _, server := range servers {
		if server.container != nil {
			cleanupErrors = append(cleanupErrors, server.container.Terminate(context.Background()))
		}
	}

	return errors.Join(cleanupErrors...)
}

// SetupReusableContainer reuses one RabbitMQ process within the test binary and
// allocates a virtual host owned exclusively by the calling test.
// SetupContainer remains the exclusive-process contract for lifecycle, fixed
// port, network, and chaos tests.
func SetupReusableContainer(t *testing.T) *ContainerResult {
	t.Helper()

	return SetupReusableContainerWithConfig(t, DefaultContainerConfig())
}

// SetupReusableContainerWithConfig is SetupReusableContainer with explicit
// server configuration. Identical configurations share the same process.
func SetupReusableContainerWithConfig(t *testing.T, cfg ContainerConfig) *ContainerResult {
	t.Helper()

	server := getReusableRabbitMQServer(t, cfg)
	sequence := server.sequence.Add(1)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", t.Name(), sequence)))
	vhost := fmt.Sprintf("midaz_test_%x", sum[:8])

	status, err := rabbitManagementRequest(
		cfg,
		server.host,
		server.mgmtPort,
		http.MethodPut,
		"/api/vhosts/"+url.PathEscape(vhost),
		nil,
	)
	require.NoError(t, err, "failed to create RabbitMQ test virtual host")
	require.Contains(t, []int{http.StatusCreated, http.StatusNoContent}, status, "unexpected create-vhost status")

	permissions := []byte(`{"configure":".*","write":".*","read":".*"}`)
	status, err = rabbitManagementRequest(
		cfg,
		server.host,
		server.mgmtPort,
		http.MethodPut,
		"/api/permissions/"+url.PathEscape(vhost)+"/"+url.PathEscape(cfg.User),
		permissions,
	)
	require.NoError(t, err, "failed to grant RabbitMQ test virtual host permissions")
	require.Equal(t, http.StatusNoContent, status, "unexpected grant-permissions status")

	uri := fmt.Sprintf(
		"amqp://%s:%s@%s:%s/%s",
		url.QueryEscape(cfg.User),
		url.QueryEscape(cfg.Password),
		server.host,
		server.amqpPort,
		url.PathEscape(vhost),
	)
	conn, err := amqp.Dial(uri)
	require.NoError(t, err, "failed to connect to RabbitMQ test virtual host")
	ch, err := conn.Channel()
	require.NoError(t, err, "failed to open RabbitMQ test virtual host channel")

	t.Cleanup(func() {
		if ch != nil {
			_ = ch.Close()
		}

		if conn != nil {
			_ = conn.Close()
		}

		cleanupStatus, cleanupErr := rabbitManagementRequest(
			cfg,
			server.host,
			server.mgmtPort,
			http.MethodDelete,
			"/api/vhosts/"+url.PathEscape(vhost),
			nil,
		)
		if cleanupErr != nil {
			t.Errorf("failed to drop RabbitMQ test virtual host %q: %v", vhost, cleanupErr)
			return
		}

		if cleanupStatus != http.StatusNoContent {
			t.Errorf("failed to drop RabbitMQ test virtual host %q: status %d", vhost, cleanupStatus)
			return
		}

		auditStatus, auditErr := rabbitManagementRequest(
			cfg,
			server.host,
			server.mgmtPort,
			http.MethodGet,
			"/api/vhosts/"+url.PathEscape(vhost),
			nil,
		)
		if auditErr != nil {
			t.Errorf("failed to audit RabbitMQ test virtual host cleanup %q: %v", vhost, auditErr)
		} else if auditStatus != http.StatusNotFound {
			t.Errorf("RabbitMQ test virtual host %q leaked after cleanup: status %d", vhost, auditStatus)
		}
	})

	return &ContainerResult{
		Container: server.container,
		Conn:      conn,
		Channel:   ch,
		Host:      server.host,
		AMQPPort:  server.amqpPort,
		MgmtPort:  server.mgmtPort,
		URI:       uri,
		VHost:     vhost,
	}
}

func getReusableRabbitMQServer(t *testing.T, cfg ContainerConfig) *reusableRabbitMQServer {
	t.Helper()

	key := fmt.Sprintf("%s|%s|%s|%d|%g", cfg.Image, cfg.User, cfg.Password, cfg.MemoryMB, cfg.CPULimit)

	reusableRabbitMQServers.Lock()
	defer reusableRabbitMQServers.Unlock()

	if server := reusableRabbitMQServers.servers[key]; server != nil {
		return server
	}

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{"5672/tcp", "15672/tcp"},
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": cfg.User,
			"RABBITMQ_DEFAULT_PASS": cfg.Password,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("Server startup complete").WithStartupTimeout(rabbitMQLogStartupTimeout),
			wait.ForHTTP("/api/health/checks/alarms").
				WithPort("15672/tcp").
				WithBasicAuth(cfg.User, cfg.Password).
				WithStartupTimeout(rabbitMQManagementStartupTimeout),
		),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	ctr := startContainerWithRetry(t, ctx, req, "failed to start reusable RabbitMQ container")
	host, err := ctr.Host(ctx)
	require.NoError(t, err, "failed to get reusable RabbitMQ container host")
	amqpPort, err := ctr.MappedPort(ctx, "5672/tcp")
	require.NoError(t, err, "failed to get reusable RabbitMQ AMQP port")
	mgmtPort, err := ctr.MappedPort(ctx, "15672/tcp")
	require.NoError(t, err, "failed to get reusable RabbitMQ management port")

	server := &reusableRabbitMQServer{
		container: ctr,
		host:      host,
		amqpPort:  amqpPort.Port(),
		mgmtPort:  mgmtPort.Port(),
		config:    cfg,
	}
	reusableRabbitMQServers.servers[key] = server

	return server
}

func rabbitManagementRequest(cfg ContainerConfig, host, port, method, path string, body []byte) (int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		method,
		fmt.Sprintf("http://%s:%s%s", host, port, path),
		reader,
	)
	if err != nil {
		return 0, fmt.Errorf("build RabbitMQ management request: %w", err)
	}

	req.SetBasicAuth(cfg.User, cfg.Password)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("execute RabbitMQ management request: %w", err)
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}
