//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

const (
	sentinelMasterName = "accounting-master"
	sentinelPassword   = "sentinel-adapter-integration-password"
)

type sentinelFailoverFixture struct {
	primaryInternal string
	replicaInternal string
	sentinelAddress string
	primaryAddress  string
	replicaAddress  string
	sentinel        *redis.SentinelClient
	primary         *redis.Client
	replica         *redis.Client
}

// Sentinel returns container-network addresses. The test client records those
// discovered addresses, then maps only the final TCP target to its host port.
type sentinelAddressDialer struct {
	mu      sync.Mutex
	targets map[string]string
	seen    []string
}

func (d *sentinelAddressDialer) dial(ctx context.Context, network, address string) (net.Conn, error) {
	d.mu.Lock()
	d.seen = append(d.seen, address)
	target := d.targets[address]
	d.mu.Unlock()
	if target == "" {
		target = address
	}

	return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, target)
}

func (d *sentinelAddressDialer) saw(address string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, seen := range d.seen {
		if seen == address {
			return true
		}
	}

	return false
}

func TestIntegration_AdapterExecute_SurvivesSentinelMasterSwitch(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker for an isolated Valkey Sentinel topology")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	fixture := newSentinelFailoverFixture(t, ctx)
	dialer := &sentinelAddressDialer{targets: map[string]string{
		fixture.primaryInternal: fixture.primaryAddress,
		fixture.replicaInternal: fixture.replicaAddress,
	}}
	shared := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName: sentinelMasterName, SentinelAddrs: []string{fixture.sentinelAddress},
		Password: sentinelPassword, DB: 2, Protocol: 2, MaxRetries: 3,
		Dialer: dialer.dial,
	})
	t.Cleanup(func() { require.NoError(t, shared.Close()) })
	provider := &integrationClientProvider{client: shared}
	input, limits := richAdapterExecution(t)
	adapter, err := newAdapterWithLimits(provider, limits)
	require.NoError(t, err)

	initial, err := adapter.Execute(ctx, input)
	require.NoError(t, err)
	require.True(t, initial.Final[0].Available.Equal(decimal.NewFromInt(70)))
	require.Equal(t, int64(8), initial.Final[0].Version)
	require.True(t, dialer.saw(fixture.primaryInternal), "the failover client must dial the primary address returned by Sentinel")
	acknowledged, err := shared.Wait(ctx, 1, 10*time.Second).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), acknowledged, "WAIT only accelerates replica acknowledgement; direct state equality below is the replication proof")

	keys, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)
	assertSentinelAccountingArtifacts(t, ctx, shared, input, keys)
	committed := captureSentinelAdapterState(t, ctx, fixture.primary, keys)
	replicationCtx, replicationCancel := context.WithTimeout(ctx, 15*time.Second)
	defer replicationCancel()
	require.NoError(t, waitForSentinelCondition(replicationCtx, func(checkCtx context.Context) (bool, error) {
		return reflect.DeepEqual(committed, captureSentinelAdapterState(t, checkCtx, fixture.replica, keys)), nil
	}), "the complete committed state did not reach the replica before promotion")
	replicaServerInfo, err := fixture.replica.Info(ctx, "server").Result()
	require.NoError(t, err)
	replicaRunID := sentinelInfoField(replicaServerInfo, "run_id")
	require.NotEmpty(t, replicaRunID)

	masterBefore, err := fixture.sentinel.GetMasterAddrByName(ctx, sentinelMasterName).Result()
	require.NoError(t, err)
	primaryHost, primaryPort, err := net.SplitHostPort(fixture.primaryInternal)
	require.NoError(t, err)
	require.Equal(t, []string{primaryHost, primaryPort}, masterBefore)
	// Force a real Sentinel-coordinated promotion without simulating a crash.
	require.Equal(t, "OK", fixture.sentinel.Failover(ctx, sentinelMasterName).Val())
	waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
	defer waitCancel()
	require.NoError(t, waitForSentinelCondition(waitCtx, func(checkCtx context.Context) (bool, error) {
		master, getErr := fixture.sentinel.GetMasterAddrByName(checkCtx, sentinelMasterName).Result()
		return getErr == nil && len(master) == 2 && net.JoinHostPort(master[0], master[1]) == fixture.replicaInternal, getErr
	}), "Sentinel did not publish the promoted replica as master")
	require.NoError(t, waitForSentinelCondition(waitCtx, func(checkCtx context.Context) (bool, error) {
		info, infoErr := shared.Info(checkCtx, "server", "replication").Result()
		return infoErr == nil && dialer.saw(fixture.replicaInternal) &&
			sentinelInfoField(info, "run_id") == replicaRunID && sentinelInfoField(info, "role") == "master", infoErr
	}), "the provider-owned failover client did not switch to the promoted master")
	require.True(t, dialer.saw(fixture.replicaInternal), "the failover client must dial the promoted address returned by Sentinel")

	for range 2 {
		replayed, executeErr := adapter.Execute(ctx, input)
		require.NoError(t, executeErr)
		require.Len(t, replayed.Movements, 1)
		require.True(t, replayed.Final[0].Available.Equal(decimal.NewFromInt(70)))
		require.Equal(t, int64(8), replayed.Final[0].Version)
		require.Equal(t, committed, captureSentinelAdapterState(t, ctx, shared, keys), "receipt replay after failover must not mutate accounting state or expiry")
		assertSentinelAccountingArtifacts(t, ctx, shared, input, keys)
	}

	require.Equal(t, 3, provider.calls)
	require.Equal(t, "FailoverClient", shared.Options().Addr)
	require.Equal(t, 3, shared.Options().MaxRetries)
	require.Equal(t, 2, shared.Options().DB)
	require.NoError(t, shared.Ping(ctx).Err(), "the adapter must not close the provider-owned Sentinel client")
}

func newSentinelFailoverFixture(t *testing.T, ctx context.Context) *sentinelFailoverFixture {
	t.Helper()
	networkName := "midaz-sentinel-" + uuid.NewString()
	testNetwork, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{Name: networkName},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		require.NoError(t, testNetwork.Remove(cleanupCtx))
	})

	primaryIP, primaryAddress := startSentinelValkey(t, ctx, networkName, "primary", nil)
	replicaIP, replicaAddress := startSentinelValkey(t, ctx, networkName, "replica", []string{
		"--replicaof", primaryIP, "6379", "--masterauth", sentinelPassword,
	})
	primaryInternal := net.JoinHostPort(primaryIP, "6379")
	replicaInternal := net.JoinHostPort(replicaIP, "6379")
	primary := redis.NewClient(&redis.Options{Addr: primaryAddress, Password: sentinelPassword, DB: 2, Protocol: 2, MaxRetries: -1})
	replica := redis.NewClient(&redis.Options{Addr: replicaAddress, Password: sentinelPassword, DB: 2, Protocol: 2, MaxRetries: -1})
	t.Cleanup(func() {
		require.NoError(t, replica.Close())
		require.NoError(t, primary.Close())
	})

	replicationCtx, replicationCancel := context.WithTimeout(ctx, 30*time.Second)
	defer replicationCancel()
	require.NoError(t, waitForSentinelCondition(replicationCtx, func(checkCtx context.Context) (bool, error) {
		primaryInfo, primaryErr := primary.Info(checkCtx, "replication").Result()
		if primaryErr != nil {
			return false, primaryErr
		}
		replicaInfo, replicaErr := replica.Info(checkCtx, "replication").Result()
		return replicaErr == nil && strings.Contains(primaryInfo, "connected_slaves:1") &&
			strings.Contains(replicaInfo, "role:slave") && strings.Contains(replicaInfo, "master_link_status:up"), replicaErr
	}), "the data replica did not synchronize with the primary")

	sentinelConfig := fmt.Sprintf(`port 26379
bind 0.0.0.0
protected-mode no
dir /tmp
sentinel monitor %s %s 6379 1
sentinel auth-pass %s %s
sentinel down-after-milliseconds %s 500
sentinel failover-timeout %s 10000
sentinel parallel-syncs %s 1
`, sentinelMasterName, primaryIP, sentinelMasterName, sentinelPassword, sentinelMasterName, sentinelMasterName, sentinelMasterName)
	sentinelContainer := startSentinelContainer(t, ctx, networkName, sentinelConfig)
	sentinelAddress := sentinelContainerAddress(t, ctx, sentinelContainer, "26379/tcp")
	sentinel := redis.NewSentinelClient(&redis.Options{Addr: sentinelAddress, Protocol: 2, MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, sentinel.Close()) })

	discoveryCtx, discoveryCancel := context.WithTimeout(ctx, 30*time.Second)
	defer discoveryCancel()
	require.NoError(t, waitForSentinelCondition(discoveryCtx, func(checkCtx context.Context) (bool, error) {
		master, masterErr := sentinel.GetMasterAddrByName(checkCtx, sentinelMasterName).Result()
		if masterErr != nil || len(master) != 2 || net.JoinHostPort(master[0], master[1]) != primaryInternal {
			return false, masterErr
		}
		replicas, replicasErr := sentinel.Replicas(checkCtx, sentinelMasterName).Result()
		if replicasErr != nil {
			return false, replicasErr
		}
		for _, candidate := range replicas {
			if net.JoinHostPort(candidate["ip"], candidate["port"]) == replicaInternal && !strings.Contains(candidate["flags"], "down") && !strings.Contains(candidate["flags"], "disconnected") {
				return true, nil
			}
		}
		return false, nil
	}), "Sentinel did not discover the synchronized replica")

	return &sentinelFailoverFixture{
		primaryInternal: primaryInternal, replicaInternal: replicaInternal,
		sentinelAddress: sentinelAddress, primaryAddress: primaryAddress, replicaAddress: replicaAddress,
		sentinel: sentinel, primary: primary, replica: replica,
	}
}

func startSentinelValkey(t *testing.T, ctx context.Context, networkName, alias string, extraArgs []string) (string, string) {
	t.Helper()
	args := []string{"valkey-server", "--bind", "0.0.0.0", "--appendonly", "no", "--save", "", "--requirepass", sentinelPassword}
	args = append(args, extraArgs...)
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "valkey/valkey:8", ExposedPorts: []string{"6379/tcp"}, Cmd: args,
			Networks: []string{networkName}, NetworkAliases: map[string][]string{networkName: {alias}},
			WaitingFor: wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	cleanupSentinelContainer(t, container)
	ip, err := container.ContainerIP(ctx)
	require.NoError(t, err)
	return ip, sentinelContainerAddress(t, ctx, container, "6379/tcp")
}

func startSentinelContainer(t *testing.T, ctx context.Context, networkName, config string) testcontainers.Container {
	t.Helper()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "valkey/valkey:8", ExposedPorts: []string{"26379/tcp"}, Cmd: []string{"valkey-sentinel", "/tmp/sentinel.conf"},
			Networks: []string{networkName}, NetworkAliases: map[string][]string{networkName: {"sentinel"}},
			Files:      []testcontainers.ContainerFile{{Reader: strings.NewReader(config), ContainerFilePath: "/tmp/sentinel.conf", FileMode: 0o644}},
			WaitingFor: wait.ForListeningPort("26379/tcp").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	cleanupSentinelContainer(t, container)
	return container
}

func cleanupSentinelContainer(t *testing.T, container testcontainers.Container) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		require.NoError(t, container.Terminate(cleanupCtx))
	})
}

func sentinelContainerAddress(t *testing.T, ctx context.Context, container testcontainers.Container, port string) string {
	t.Helper()
	host, err := container.Host(ctx)
	require.NoError(t, err)
	mapped, err := container.MappedPort(ctx, port)
	require.NoError(t, err)
	return net.JoinHostPort(host, mapped.Port())
}

func waitForSentinelCondition(ctx context.Context, check func(context.Context) (bool, error)) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		ready, err := check(ctx)
		if ready {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("%w (last check: %v)", ctx.Err(), lastErr)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func sentinelInfoField(info, field string) string {
	prefix := field + ":"
	for _, line := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}

	return ""
}

func captureSentinelAdapterState(t *testing.T, ctx context.Context, client *redis.Client, keys resolvedExecutionKeys) map[string]any {
	t.Helper()
	state := make(map[string]any)
	inventory := []string{keys.Schedule, keys.Recovery, keys.Receipts, keys.Guards}
	for _, balance := range keys.Balances {
		inventory = append(inventory, balance.Balance, balance.Deleted, balance.LegacyDeleted)
	}
	for _, key := range inventory {
		dump, err := client.Dump(ctx, key).Result()
		if err == redis.Nil {
			dump, err = "", nil
		}
		require.NoError(t, err)
		expiry, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		state[key] = []any{dump, expiry}
	}
	return state
}

func assertSentinelAccountingArtifacts(t *testing.T, ctx context.Context, client *redis.Client, input command.EngineExecution, keys resolvedExecutionKeys) {
	t.Helper()
	transactionID := input.Execution.Transactions[0].ID.String()
	executionID := input.Execution.ExecutionID.String()
	exists, err := client.Exists(ctx, keys.Balances[input.Execution.Balances[0].BalanceRef].Balance).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), exists)
	receiptExists, err := client.HExists(ctx, keys.Receipts, executionID).Result()
	require.NoError(t, err)
	require.True(t, receiptExists)
	recoveryExists, err := client.HExists(ctx, keys.Recovery, transactionID+":"+executionID).Result()
	require.NoError(t, err)
	require.True(t, recoveryExists)
	guard, err := client.HGet(ctx, keys.Guards, transactionID).Result()
	require.NoError(t, err)
	require.Equal(t, "executed-once", guard)
	_, err = client.ZScore(ctx, keys.Schedule, keys.Balances[input.Execution.Balances[0].BalanceRef].Balance).Result()
	require.NoError(t, err)
}
