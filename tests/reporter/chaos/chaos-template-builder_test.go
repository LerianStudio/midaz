//go:build chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v4/tests/reporter/utils"
	chaosutil "github.com/LerianStudio/midaz/v4/tests/reporter/utils/chaos"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChaos_BlocksConfig_MongoDBConnectionLoss verifies that the blocks-config endpoint
// remains fully operational when MongoDB is completely unreachable. Since this endpoint
// returns data from a pure function (GetBlockDefinitions), it should be immune to
// MongoDB failures — validating resilience isolation.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_BlocksConfig_MongoDBConnectionLoss(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for connection loss simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify blocks-config endpoint works under normal conditions
	t.Log("Phase 1 (Normal): Verifying blocks-config endpoint works before MongoDB connection loss...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, body, err := cli.Request(reqCtx, "GET", "/v1/templates/blocks-config", headers, nil)
	require.NoError(t, err, "blocks-config request should succeed before chaos injection")
	require.Equal(t, 200, code, "blocks-config should return 200 before chaos injection")

	var normalResponse json.RawMessage
	require.NoError(t, json.Unmarshal(body, &normalResponse), "blocks-config should return valid JSON before chaos injection")
	t.Log("Phase 1 (Normal): blocks-config endpoint is healthy and returns valid JSON")

	// Phase 2 (Inject): Simulate complete MongoDB connection loss via Toxiproxy
	t.Log("Phase 2 (Inject): Injecting MongoDB connection loss via Toxiproxy...")
	err = chaosutil.InjectConnectionLoss(mongoProxy)
	require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")

	// Wait for the connection loss to take effect on existing connections
	time.Sleep(5 * time.Second)
	t.Log("Phase 2 (Inject): MongoDB connection loss injected successfully")

	// Phase 3 (Verify): blocks-config endpoint should STILL return 200 (stateless endpoint)
	t.Log("Phase 3 (Verify): Verifying blocks-config endpoint remains operational during MongoDB outage...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	code, body, err = cli.Request(reqCtx3, "GET", "/v1/templates/blocks-config", headers, nil)
	require.NoError(t, err, "blocks-config should respond even when MongoDB is down (stateless endpoint)")
	require.Equal(t, 200, code, "blocks-config should return 200 when MongoDB is down (no DB dependency)")

	var chaosResponse json.RawMessage
	require.NoError(t, json.Unmarshal(body, &chaosResponse), "blocks-config should return valid JSON during MongoDB outage")
	assert.JSONEq(t, string(normalResponse), string(chaosResponse), "blocks-config response should be identical during MongoDB outage (pure function)")
	t.Log("Phase 3 (Verify): blocks-config endpoint confirmed operational during MongoDB connection loss")

	// Phase 4 (Restore): Remove all toxics to restore MongoDB connectivity
	t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore MongoDB connectivity...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB connectivity restoration")
	t.Log("Phase 4 (Restore): MongoDB connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify blocks-config still returns correct data after recovery
	t.Log("Phase 5 (Recovery): Verifying blocks-config endpoint after MongoDB recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, body, err = cli.Request(reqCtx5, "GET", "/v1/templates/blocks-config", headers, nil)
	require.NoError(t, err, "blocks-config should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "blocks-config should return 200 after MongoDB recovery")

	var recoveryResponse json.RawMessage
	require.NoError(t, json.Unmarshal(body, &recoveryResponse), "blocks-config should return valid JSON after recovery")
	assert.JSONEq(t, string(normalResponse), string(recoveryResponse), "blocks-config response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): blocks-config endpoint fully verified after MongoDB recovery")
}

// TestChaos_FiltersConfig_MongoDBConnectionLoss verifies that the filters endpoint
// remains fully operational when MongoDB is completely unreachable. Since this endpoint
// returns data from a pure function (GetFilterDefinitions), it should be immune to
// MongoDB failures — validating resilience isolation.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_FiltersConfig_MongoDBConnectionLoss(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for connection loss simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify filters endpoint works under normal conditions
	t.Log("Phase 1 (Normal): Verifying filters endpoint works before MongoDB connection loss...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, body, err := cli.Request(reqCtx, "GET", "/v1/templates/filters", headers, nil)
	require.NoError(t, err, "filters request should succeed before chaos injection")
	require.Equal(t, 200, code, "filters should return 200 before chaos injection")

	var normalResponse json.RawMessage
	require.NoError(t, json.Unmarshal(body, &normalResponse), "filters should return valid JSON before chaos injection")
	t.Log("Phase 1 (Normal): filters endpoint is healthy and returns valid JSON")

	// Phase 2 (Inject): Simulate complete MongoDB connection loss via Toxiproxy
	t.Log("Phase 2 (Inject): Injecting MongoDB connection loss via Toxiproxy...")
	err = chaosutil.InjectConnectionLoss(mongoProxy)
	require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")

	time.Sleep(5 * time.Second)
	t.Log("Phase 2 (Inject): MongoDB connection loss injected successfully")

	// Phase 3 (Verify): filters endpoint should STILL return 200 (stateless endpoint)
	t.Log("Phase 3 (Verify): Verifying filters endpoint remains operational during MongoDB outage...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	code, body, err = cli.Request(reqCtx3, "GET", "/v1/templates/filters", headers, nil)
	require.NoError(t, err, "filters should respond even when MongoDB is down (stateless endpoint)")
	require.Equal(t, 200, code, "filters should return 200 when MongoDB is down (no DB dependency)")

	var chaosResponse json.RawMessage
	require.NoError(t, json.Unmarshal(body, &chaosResponse), "filters should return valid JSON during MongoDB outage")
	assert.JSONEq(t, string(normalResponse), string(chaosResponse), "filters response should be identical during MongoDB outage (pure function)")
	t.Log("Phase 3 (Verify): filters endpoint confirmed operational during MongoDB connection loss")

	// Phase 4 (Restore): Remove all toxics to restore MongoDB connectivity
	t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore MongoDB connectivity...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB connectivity restoration")
	t.Log("Phase 4 (Restore): MongoDB connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify filters still returns correct data after recovery
	t.Log("Phase 5 (Recovery): Verifying filters endpoint after MongoDB recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, body, err = cli.Request(reqCtx5, "GET", "/v1/templates/filters", headers, nil)
	require.NoError(t, err, "filters should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "filters should return 200 after MongoDB recovery")

	var recoveryResponse json.RawMessage
	require.NoError(t, json.Unmarshal(body, &recoveryResponse), "filters should return valid JSON after recovery")
	assert.JSONEq(t, string(normalResponse), string(recoveryResponse), "filters response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): filters endpoint fully verified after MongoDB recovery")
}

// TestChaos_BlocksConfig_MongoDBHighLatency verifies that the blocks-config endpoint
// response time is unaffected by high MongoDB latency. Since this endpoint does not
// query MongoDB, latency on the MongoDB proxy should have zero impact.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_BlocksConfig_MongoDBHighLatency(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping latency test - Toxiproxy not available")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify blocks-config endpoint works and measure baseline response time
	t.Log("Phase 1 (Normal): Verifying blocks-config endpoint and measuring baseline response time...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	startNormal := time.Now()

	code, _, err := cli.Request(reqCtx, "GET", "/v1/templates/blocks-config", headers, nil)
	normalDuration := time.Since(startNormal)

	require.NoError(t, err, "blocks-config request should succeed before chaos injection")
	require.Equal(t, 200, code, "blocks-config should return 200 before chaos injection")
	t.Logf("Phase 1 (Normal): blocks-config responded in %v", normalDuration)

	// Phase 2 (Inject): Add 3000ms latency with 1000ms jitter to MongoDB
	t.Log("Phase 2 (Inject): Injecting 3000ms latency + 1000ms jitter into MongoDB connection...")
	err = chaosutil.InjectLatency(mongoProxy, 3000, 1000)
	require.NoError(t, err, "Failed to inject latency via Toxiproxy")

	time.Sleep(3 * time.Second)
	t.Log("Phase 2 (Inject): MongoDB latency injected successfully")

	// Phase 3 (Verify): blocks-config should respond fast (under 500ms) despite MongoDB latency
	t.Log("Phase 3 (Verify): Verifying blocks-config response time is unaffected by MongoDB latency...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	startChaos := time.Now()

	code, body3, err := cli.Request(reqCtx3, "GET", "/v1/templates/blocks-config", headers, nil)
	chaosDuration := time.Since(startChaos)

	require.NoError(t, err, "blocks-config should respond even with MongoDB latency (stateless endpoint)")
	require.Equal(t, 200, code, "blocks-config should return 200 under MongoDB latency")

	var chaosJSON json.RawMessage
	require.NoError(t, json.Unmarshal(body3, &chaosJSON), "blocks-config should return valid JSON under MongoDB latency")

	// Stateless endpoint should respond well under 500ms regardless of MongoDB latency
	assert.Less(t, chaosDuration, 500*time.Millisecond,
		"blocks-config response time should stay under 500ms when MongoDB has high latency (stateless endpoint, got %v)", chaosDuration)
	t.Logf("Phase 3 (Verify): blocks-config responded in %v under MongoDB latency (limit: 500ms)", chaosDuration)

	// Phase 4 (Restore): Remove all toxics to restore normal MongoDB operation
	t.Log("Phase 4 (Restore): Removing latency toxic from MongoDB proxy...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB latency removal")
	t.Log("Phase 4 (Restore): System recovered, MongoDB latency removed")

	// Phase 5 (Recovery): Verify blocks-config response time is back to normal
	t.Log("Phase 5 (Recovery): Verifying blocks-config response time after MongoDB recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	startRecovery := time.Now()

	code, _, err = cli.Request(reqCtx5, "GET", "/v1/templates/blocks-config", headers, nil)
	recoveryDuration := time.Since(startRecovery)

	require.NoError(t, err, "blocks-config should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "blocks-config should return 200 after MongoDB recovery")
	t.Logf("Phase 5 (Recovery): blocks-config responded in %v after recovery", recoveryDuration)
}

// TestChaos_FiltersConfig_MongoDBHighLatency verifies that the filters endpoint
// response time is unaffected by high MongoDB latency. Since this endpoint does not
// query MongoDB, latency on the MongoDB proxy should have zero impact.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_FiltersConfig_MongoDBHighLatency(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping latency test - Toxiproxy not available")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify filters endpoint works and measure baseline response time
	t.Log("Phase 1 (Normal): Verifying filters endpoint and measuring baseline response time...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	startNormal := time.Now()

	code, _, err := cli.Request(reqCtx, "GET", "/v1/templates/filters", headers, nil)
	normalDuration := time.Since(startNormal)

	require.NoError(t, err, "filters request should succeed before chaos injection")
	require.Equal(t, 200, code, "filters should return 200 before chaos injection")
	t.Logf("Phase 1 (Normal): filters responded in %v", normalDuration)

	// Phase 2 (Inject): Add 3000ms latency with 1000ms jitter to MongoDB
	t.Log("Phase 2 (Inject): Injecting 3000ms latency + 1000ms jitter into MongoDB connection...")
	err = chaosutil.InjectLatency(mongoProxy, 3000, 1000)
	require.NoError(t, err, "Failed to inject latency via Toxiproxy")

	time.Sleep(3 * time.Second)
	t.Log("Phase 2 (Inject): MongoDB latency injected successfully")

	// Phase 3 (Verify): filters should respond fast (under 500ms) despite MongoDB latency
	t.Log("Phase 3 (Verify): Verifying filters response time is unaffected by MongoDB latency...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	startChaos := time.Now()

	code, body3, err := cli.Request(reqCtx3, "GET", "/v1/templates/filters", headers, nil)
	chaosDuration := time.Since(startChaos)

	require.NoError(t, err, "filters should respond even with MongoDB latency (stateless endpoint)")
	require.Equal(t, 200, code, "filters should return 200 under MongoDB latency")

	var chaosJSON json.RawMessage
	require.NoError(t, json.Unmarshal(body3, &chaosJSON), "filters should return valid JSON under MongoDB latency")

	assert.Less(t, chaosDuration, 500*time.Millisecond,
		"filters response time should stay under 500ms when MongoDB has high latency (stateless endpoint, got %v)", chaosDuration)
	t.Logf("Phase 3 (Verify): filters responded in %v under MongoDB latency (limit: 500ms)", chaosDuration)

	// Phase 4 (Restore): Remove all toxics to restore normal MongoDB operation
	t.Log("Phase 4 (Restore): Removing latency toxic from MongoDB proxy...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB latency removal")
	t.Log("Phase 4 (Restore): System recovered, MongoDB latency removed")

	// Phase 5 (Recovery): Verify filters response time is back to normal
	t.Log("Phase 5 (Recovery): Verifying filters response time after MongoDB recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	startRecovery := time.Now()

	code, _, err = cli.Request(reqCtx5, "GET", "/v1/templates/filters", headers, nil)
	recoveryDuration := time.Since(startRecovery)

	require.NoError(t, err, "filters should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "filters should return 200 after MongoDB recovery")
	t.Logf("Phase 5 (Recovery): filters responded in %v after recovery", recoveryDuration)
}

// TestChaos_BlocksConfig_RabbitMQNetworkPartition verifies that the blocks-config endpoint
// response time stays under 500ms when RabbitMQ is experiencing a network partition.
// Since blocks-config does not use RabbitMQ, it should be completely immune.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_BlocksConfig_RabbitMQNetworkPartition(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	rabbitProxy, useToxiproxy := getRabbitMQProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for network partition simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify blocks-config works and measure baseline
	t.Log("Phase 1 (Normal): Verifying blocks-config endpoint before RabbitMQ partition...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, body, err := cli.Request(reqCtx, "GET", "/v1/templates/blocks-config", headers, nil)
	require.NoError(t, err, "blocks-config should succeed before chaos injection")
	require.Equal(t, 200, code, "blocks-config should return 200 before chaos injection")

	var normalResponse json.RawMessage
	require.NoError(t, json.Unmarshal(body, &normalResponse), "blocks-config should return valid JSON before chaos injection")
	t.Log("Phase 1 (Normal): blocks-config endpoint is healthy")

	// Phase 2 (Inject): Simulate network partition on RabbitMQ (50% packet loss)
	t.Log("Phase 2 (Inject): Injecting network partition (50% packet loss) on RabbitMQ...")
	err = chaosutil.InjectPacketLoss(rabbitProxy, 50)
	require.NoError(t, err, "Failed to inject packet loss via Toxiproxy")

	time.Sleep(3 * time.Second)
	t.Log("Phase 2 (Inject): RabbitMQ network partition injected successfully")

	// Phase 3 (Verify): blocks-config should respond fast (under 500ms) despite RabbitMQ partition
	t.Log("Phase 3 (Verify): Verifying blocks-config response time during RabbitMQ partition...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	startChaos := time.Now()

	code, body3, err := cli.Request(reqCtx3, "GET", "/v1/templates/blocks-config", headers, nil)
	chaosDuration := time.Since(startChaos)

	require.NoError(t, err, "blocks-config should respond during RabbitMQ partition (no RabbitMQ dependency)")
	require.Equal(t, 200, code, "blocks-config should return 200 during RabbitMQ partition")

	var chaosResponse json.RawMessage
	require.NoError(t, json.Unmarshal(body3, &chaosResponse), "blocks-config should return valid JSON during RabbitMQ partition")
	assert.JSONEq(t, string(normalResponse), string(chaosResponse), "blocks-config response should be identical during RabbitMQ partition")

	assert.Less(t, chaosDuration, 500*time.Millisecond,
		"blocks-config response time should stay under 500ms during RabbitMQ partition (got %v)", chaosDuration)
	t.Logf("Phase 3 (Verify): blocks-config responded in %v during RabbitMQ partition (limit: 500ms)", chaosDuration)

	// Phase 4 (Restore): Remove all toxics to restore RabbitMQ connectivity
	t.Log("Phase 4 (Restore): Removing packet loss toxic from RabbitMQ proxy...")
	err = chaosutil.RemoveAllToxics(rabbitProxy)
	require.NoError(t, err, "Failed to remove toxics from RabbitMQ proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after RabbitMQ partition removal")
	t.Log("Phase 4 (Restore): RabbitMQ connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify blocks-config still works correctly after recovery
	t.Log("Phase 5 (Recovery): Verifying blocks-config after RabbitMQ recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, body5, err := cli.Request(reqCtx5, "GET", "/v1/templates/blocks-config", headers, nil)
	require.NoError(t, err, "blocks-config should succeed after RabbitMQ recovery")
	require.Equal(t, 200, code, "blocks-config should return 200 after RabbitMQ recovery")

	var recoveryResponse json.RawMessage
	require.NoError(t, json.Unmarshal(body5, &recoveryResponse), "blocks-config should return valid JSON after recovery")
	assert.JSONEq(t, string(normalResponse), string(recoveryResponse), "blocks-config response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): blocks-config endpoint verified after RabbitMQ recovery")
}

// TestChaos_BothEndpoints_ValkeyUnavailable verifies that both template builder endpoints
// (blocks-config and filters) return valid JSON when Valkey/Redis is completely unavailable.
// Since both endpoints are pure functions, they should be immune to cache failures.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_BothEndpoints_ValkeyUnavailable(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	valkeyProxy, useToxiproxy := getValkeyProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for Valkey unavailability simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify both endpoints work under normal conditions
	t.Log("Phase 1 (Normal): Verifying both template builder endpoints before Valkey outage...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Check blocks-config
	reqCtxBlocks, cancelBlocks := context.WithTimeout(ctx, 10*time.Second)
	defer cancelBlocks()

	code, bodyBlocks, err := cli.Request(reqCtxBlocks, "GET", "/v1/templates/blocks-config", headers, nil)
	require.NoError(t, err, "blocks-config should succeed before chaos injection")
	require.Equal(t, 200, code, "blocks-config should return 200 before chaos injection")

	var normalBlocks json.RawMessage
	require.NoError(t, json.Unmarshal(bodyBlocks, &normalBlocks), "blocks-config should return valid JSON")

	// Check filters
	reqCtxFilters, cancelFilters := context.WithTimeout(ctx, 10*time.Second)
	defer cancelFilters()

	code, bodyFilters, err := cli.Request(reqCtxFilters, "GET", "/v1/templates/filters", headers, nil)
	require.NoError(t, err, "filters should succeed before chaos injection")
	require.Equal(t, 200, code, "filters should return 200 before chaos injection")

	var normalFilters json.RawMessage
	require.NoError(t, json.Unmarshal(bodyFilters, &normalFilters), "filters should return valid JSON")
	t.Log("Phase 1 (Normal): Both endpoints are healthy")

	// Phase 2 (Inject): Simulate complete Valkey unavailability via Toxiproxy
	t.Log("Phase 2 (Inject): Injecting complete Valkey connection loss via Toxiproxy...")
	err = chaosutil.InjectConnectionLoss(valkeyProxy)
	require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")

	time.Sleep(5 * time.Second)
	t.Log("Phase 2 (Inject): Valkey connection loss injected successfully")

	// Phase 3 (Verify): Both endpoints should STILL return 200 with valid JSON
	t.Log("Phase 3 (Verify): Verifying both endpoints remain operational during Valkey outage...")

	// Verify blocks-config
	reqCtx3Blocks, cancel3Blocks := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3Blocks()

	code, body3Blocks, err := cli.Request(reqCtx3Blocks, "GET", "/v1/templates/blocks-config", headers, nil)
	require.NoError(t, err, "blocks-config should respond during Valkey outage (stateless endpoint)")
	require.Equal(t, 200, code, "blocks-config should return 200 during Valkey outage")

	var chaosBlocks json.RawMessage
	require.NoError(t, json.Unmarshal(body3Blocks, &chaosBlocks), "blocks-config should return valid JSON during Valkey outage")
	assert.JSONEq(t, string(normalBlocks), string(chaosBlocks), "blocks-config response should be identical during Valkey outage")
	t.Log("Phase 3 (Verify): blocks-config confirmed operational during Valkey outage")

	// Verify filters
	reqCtx3Filters, cancel3Filters := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3Filters()

	code, body3Filters, err := cli.Request(reqCtx3Filters, "GET", "/v1/templates/filters", headers, nil)
	require.NoError(t, err, "filters should respond during Valkey outage (stateless endpoint)")
	require.Equal(t, 200, code, "filters should return 200 during Valkey outage")

	var chaosFilters json.RawMessage
	require.NoError(t, json.Unmarshal(body3Filters, &chaosFilters), "filters should return valid JSON during Valkey outage")
	assert.JSONEq(t, string(normalFilters), string(chaosFilters), "filters response should be identical during Valkey outage")
	t.Log("Phase 3 (Verify): filters confirmed operational during Valkey outage")

	// Phase 4 (Restore): Remove all toxics to restore Valkey connectivity
	t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore Valkey connectivity...")
	err = chaosutil.RemoveAllToxics(valkeyProxy)
	require.NoError(t, err, "Failed to remove toxics from Valkey proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after Valkey connectivity restoration")
	t.Log("Phase 4 (Restore): Valkey connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify both endpoints return correct data after recovery
	t.Log("Phase 5 (Recovery): Verifying both endpoints after Valkey recovery...")

	reqCtx5Blocks, cancel5Blocks := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Blocks()

	code, body5Blocks, err := cli.Request(reqCtx5Blocks, "GET", "/v1/templates/blocks-config", headers, nil)
	require.NoError(t, err, "blocks-config should succeed after Valkey recovery")
	require.Equal(t, 200, code, "blocks-config should return 200 after Valkey recovery")

	var recoveryBlocks json.RawMessage
	require.NoError(t, json.Unmarshal(body5Blocks, &recoveryBlocks), "blocks-config should return valid JSON after recovery")
	assert.JSONEq(t, string(normalBlocks), string(recoveryBlocks), "blocks-config response should be consistent after recovery")

	reqCtx5Filters, cancel5Filters := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Filters()

	code, body5Filters, err := cli.Request(reqCtx5Filters, "GET", "/v1/templates/filters", headers, nil)
	require.NoError(t, err, "filters should succeed after Valkey recovery")
	require.Equal(t, 200, code, "filters should return 200 after Valkey recovery")

	var recoveryFilters json.RawMessage
	require.NoError(t, json.Unmarshal(body5Filters, &recoveryFilters), "filters should return valid JSON after recovery")
	assert.JSONEq(t, string(normalFilters), string(recoveryFilters), "filters response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): Both endpoints fully verified after Valkey recovery")
}

// generateCodeRequestBody returns a valid request body for the generate-code endpoint.
// This is a pure-function endpoint that converts JSON blocks to Pongo2 template code.
func generateCodeRequestBody() map[string]any {
	return map[string]any{
		"blocks": []map[string]any{
			{"type": "text", "content": "chaos test"},
		},
		"format": "",
	}
}

// TestChaos_GenerateCode_MongoDBConnectionLoss verifies that the generate-code endpoint
// remains fully operational when MongoDB is completely unreachable. Since this endpoint
// uses pure functions to convert JSON blocks to Pongo2 code, it should be immune to
// MongoDB failures — validating resilience isolation.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_GenerateCode_MongoDBConnectionLoss(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for connection loss simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()
	body := generateCodeRequestBody()

	// Phase 1 (Normal): Verify generate-code endpoint works under normal conditions
	t.Log("Phase 1 (Normal): Verifying generate-code endpoint works before MongoDB connection loss...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, respBody, err := cli.Request(reqCtx, "POST", "/v1/templates/generate-code", headers, body)
	require.NoError(t, err, "generate-code request should succeed before chaos injection")
	require.Equal(t, 200, code, "generate-code should return 200 before chaos injection")

	var normalResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &normalResponse), "generate-code should return valid JSON before chaos injection")
	t.Log("Phase 1 (Normal): generate-code endpoint is healthy and returns valid JSON")

	// Phase 2 (Inject): Simulate complete MongoDB connection loss via Toxiproxy
	t.Log("Phase 2 (Inject): Injecting MongoDB connection loss via Toxiproxy...")
	err = chaosutil.InjectConnectionLoss(mongoProxy)
	require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")

	// Wait for the connection loss to take effect on existing connections
	time.Sleep(5 * time.Second)
	t.Log("Phase 2 (Inject): MongoDB connection loss injected successfully")

	// Phase 3 (Verify): generate-code endpoint should STILL return 200 (stateless endpoint)
	t.Log("Phase 3 (Verify): Verifying generate-code endpoint remains operational during MongoDB outage...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	code, respBody, err = cli.Request(reqCtx3, "POST", "/v1/templates/generate-code", headers, body)
	require.NoError(t, err, "generate-code should respond even when MongoDB is down (stateless endpoint)")
	require.Equal(t, 200, code, "generate-code should return 200 when MongoDB is down (no DB dependency)")

	var chaosResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &chaosResponse), "generate-code should return valid JSON during MongoDB outage")
	assert.JSONEq(t, string(normalResponse), string(chaosResponse), "generate-code response should be identical during MongoDB outage (pure function)")
	t.Log("Phase 3 (Verify): generate-code endpoint confirmed operational during MongoDB connection loss")

	// Phase 4 (Restore): Remove all toxics to restore MongoDB connectivity
	t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore MongoDB connectivity...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB connectivity restoration")
	t.Log("Phase 4 (Restore): MongoDB connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify generate-code still returns correct data after recovery
	t.Log("Phase 5 (Recovery): Verifying generate-code endpoint after MongoDB recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, respBody, err = cli.Request(reqCtx5, "POST", "/v1/templates/generate-code", headers, body)
	require.NoError(t, err, "generate-code should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "generate-code should return 200 after MongoDB recovery")

	var recoveryResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &recoveryResponse), "generate-code should return valid JSON after recovery")
	assert.JSONEq(t, string(normalResponse), string(recoveryResponse), "generate-code response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): generate-code endpoint fully verified after MongoDB recovery")
}

// TestChaos_GenerateCode_RabbitMQNetworkPartition verifies that the generate-code endpoint
// remains fully operational when RabbitMQ is experiencing a network partition (50% packet loss).
// Since generate-code is a pure function that does not publish or consume messages, it should
// be completely immune to RabbitMQ failures — validating resilience isolation.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_GenerateCode_RabbitMQNetworkPartition(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	rabbitProxy, useToxiproxy := getRabbitMQProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for network partition simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()
	body := generateCodeRequestBody()

	// Phase 1 (Normal): Verify generate-code works and capture baseline response
	t.Log("Phase 1 (Normal): Verifying generate-code endpoint before RabbitMQ partition...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, respBody, err := cli.Request(reqCtx, "POST", "/v1/templates/generate-code", headers, body)
	require.NoError(t, err, "generate-code should succeed before chaos injection")
	require.Equal(t, 200, code, "generate-code should return 200 before chaos injection")

	var normalResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &normalResponse), "generate-code should return valid JSON before chaos injection")
	t.Log("Phase 1 (Normal): generate-code endpoint is healthy")

	// Phase 2 (Inject): Simulate network partition on RabbitMQ (50% packet loss)
	t.Log("Phase 2 (Inject): Injecting network partition (50% packet loss) on RabbitMQ...")
	err = chaosutil.InjectPacketLoss(rabbitProxy, 50)
	require.NoError(t, err, "Failed to inject packet loss via Toxiproxy")

	time.Sleep(3 * time.Second)
	t.Log("Phase 2 (Inject): RabbitMQ network partition injected successfully")

	// Phase 3 (Verify): generate-code should respond fast (under 500ms) despite RabbitMQ partition
	t.Log("Phase 3 (Verify): Verifying generate-code response during RabbitMQ partition...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	startChaos := time.Now()

	code, respBody, err = cli.Request(reqCtx3, "POST", "/v1/templates/generate-code", headers, body)
	chaosDuration := time.Since(startChaos)

	require.NoError(t, err, "generate-code should respond during RabbitMQ partition (no RabbitMQ dependency)")
	require.Equal(t, 200, code, "generate-code should return 200 during RabbitMQ partition")

	var chaosResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &chaosResponse), "generate-code should return valid JSON during RabbitMQ partition")
	assert.JSONEq(t, string(normalResponse), string(chaosResponse), "generate-code response should be identical during RabbitMQ partition")

	assert.Less(t, chaosDuration, 500*time.Millisecond,
		"generate-code response time should stay under 500ms during RabbitMQ partition (got %v)", chaosDuration)
	t.Logf("Phase 3 (Verify): generate-code responded in %v during RabbitMQ partition (limit: 500ms)", chaosDuration)

	// Phase 4 (Restore): Remove all toxics to restore RabbitMQ connectivity
	t.Log("Phase 4 (Restore): Removing packet loss toxic from RabbitMQ proxy...")
	err = chaosutil.RemoveAllToxics(rabbitProxy)
	require.NoError(t, err, "Failed to remove toxics from RabbitMQ proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after RabbitMQ partition removal")
	t.Log("Phase 4 (Restore): RabbitMQ connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify generate-code still works correctly after recovery
	t.Log("Phase 5 (Recovery): Verifying generate-code after RabbitMQ recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, respBody, err = cli.Request(reqCtx5, "POST", "/v1/templates/generate-code", headers, body)
	require.NoError(t, err, "generate-code should succeed after RabbitMQ recovery")
	require.Equal(t, 200, code, "generate-code should return 200 after RabbitMQ recovery")

	var recoveryResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &recoveryResponse), "generate-code should return valid JSON after recovery")
	assert.JSONEq(t, string(normalResponse), string(recoveryResponse), "generate-code response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): generate-code endpoint verified after RabbitMQ recovery")
}

// TestChaos_GenerateCode_ValkeyUnavailable verifies that the generate-code endpoint
// remains fully operational when Valkey/Redis is completely unreachable. Since this endpoint
// uses pure functions and does not read from or write to any cache, it should be immune to
// cache failures — validating resilience isolation.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_GenerateCode_ValkeyUnavailable(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	valkeyProxy, useToxiproxy := getValkeyProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for Valkey unavailability simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()
	body := generateCodeRequestBody()

	// Phase 1 (Normal): Verify generate-code endpoint works under normal conditions
	t.Log("Phase 1 (Normal): Verifying generate-code endpoint before Valkey outage...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, respBody, err := cli.Request(reqCtx, "POST", "/v1/templates/generate-code", headers, body)
	require.NoError(t, err, "generate-code should succeed before chaos injection")
	require.Equal(t, 200, code, "generate-code should return 200 before chaos injection")

	var normalResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &normalResponse), "generate-code should return valid JSON before chaos injection")
	t.Log("Phase 1 (Normal): generate-code endpoint is healthy and returns valid JSON")

	// Phase 2 (Inject): Simulate complete Valkey unavailability via Toxiproxy
	t.Log("Phase 2 (Inject): Injecting complete Valkey connection loss via Toxiproxy...")
	err = chaosutil.InjectConnectionLoss(valkeyProxy)
	require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")

	time.Sleep(5 * time.Second)
	t.Log("Phase 2 (Inject): Valkey connection loss injected successfully")

	// Phase 3 (Verify): generate-code endpoint should STILL return 200 with valid JSON
	t.Log("Phase 3 (Verify): Verifying generate-code endpoint remains operational during Valkey outage...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	code, respBody, err = cli.Request(reqCtx3, "POST", "/v1/templates/generate-code", headers, body)
	require.NoError(t, err, "generate-code should respond during Valkey outage (stateless endpoint)")
	require.Equal(t, 200, code, "generate-code should return 200 during Valkey outage")

	var chaosResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &chaosResponse), "generate-code should return valid JSON during Valkey outage")
	assert.JSONEq(t, string(normalResponse), string(chaosResponse), "generate-code response should be identical during Valkey outage (pure function)")
	t.Log("Phase 3 (Verify): generate-code endpoint confirmed operational during Valkey outage")

	// Phase 4 (Restore): Remove all toxics to restore Valkey connectivity
	t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore Valkey connectivity...")
	err = chaosutil.RemoveAllToxics(valkeyProxy)
	require.NoError(t, err, "Failed to remove toxics from Valkey proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after Valkey connectivity restoration")
	t.Log("Phase 4 (Restore): Valkey connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify generate-code still returns correct data after recovery
	t.Log("Phase 5 (Recovery): Verifying generate-code endpoint after Valkey recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, respBody, err = cli.Request(reqCtx5, "POST", "/v1/templates/generate-code", headers, body)
	require.NoError(t, err, "generate-code should succeed after Valkey recovery")
	require.Equal(t, 200, code, "generate-code should return 200 after Valkey recovery")

	var recoveryResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &recoveryResponse), "generate-code should return valid JSON after recovery")
	assert.JSONEq(t, string(normalResponse), string(recoveryResponse), "generate-code response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): generate-code endpoint fully verified after Valkey recovery")
}

// TestChaos_GenerateCode_HighLatency verifies that the generate-code endpoint response
// time is unaffected by high MongoDB latency. Since this endpoint does not query MongoDB
// (it uses pure functions to convert blocks to Pongo2 code), latency on the MongoDB proxy
// should have zero impact on response time.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_GenerateCode_HighLatency(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping latency test - Toxiproxy not available")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()
	body := generateCodeRequestBody()

	// Phase 1 (Normal): Verify generate-code endpoint works and measure baseline response time
	t.Log("Phase 1 (Normal): Verifying generate-code endpoint and measuring baseline response time...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	startNormal := time.Now()

	code, _, err := cli.Request(reqCtx, "POST", "/v1/templates/generate-code", headers, body)
	normalDuration := time.Since(startNormal)

	require.NoError(t, err, "generate-code request should succeed before chaos injection")
	require.Equal(t, 200, code, "generate-code should return 200 before chaos injection")
	t.Logf("Phase 1 (Normal): generate-code responded in %v", normalDuration)

	// Phase 2 (Inject): Add 5000ms latency with 1000ms jitter to MongoDB
	t.Log("Phase 2 (Inject): Injecting 5000ms latency + 1000ms jitter into MongoDB connection...")
	err = chaosutil.InjectLatency(mongoProxy, 5000, 1000)
	require.NoError(t, err, "Failed to inject latency via Toxiproxy")

	time.Sleep(3 * time.Second)
	t.Log("Phase 2 (Inject): MongoDB latency injected successfully")

	// Phase 3 (Verify): generate-code should respond fast (under 500ms) despite 5s MongoDB latency
	t.Log("Phase 3 (Verify): Verifying generate-code response time is unaffected by MongoDB latency...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	startChaos := time.Now()

	code, body3, err := cli.Request(reqCtx3, "POST", "/v1/templates/generate-code", headers, body)
	chaosDuration := time.Since(startChaos)

	require.NoError(t, err, "generate-code should respond even with MongoDB latency (stateless endpoint)")
	require.Equal(t, 200, code, "generate-code should return 200 under MongoDB latency")

	var chaosJSON json.RawMessage
	require.NoError(t, json.Unmarshal(body3, &chaosJSON), "generate-code should return valid JSON under MongoDB latency")

	// Stateless endpoint should respond well under 500ms regardless of MongoDB latency
	assert.Less(t, chaosDuration, 500*time.Millisecond,
		"generate-code response time should stay under 500ms when MongoDB has 5s latency (stateless endpoint, got %v)", chaosDuration)
	t.Logf("Phase 3 (Verify): generate-code responded in %v under MongoDB latency (limit: 500ms)", chaosDuration)

	// Phase 4 (Restore): Remove all toxics to restore normal MongoDB operation
	t.Log("Phase 4 (Restore): Removing latency toxic from MongoDB proxy...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB latency removal")
	t.Log("Phase 4 (Restore): System recovered, MongoDB latency removed")

	// Phase 5 (Recovery): Verify generate-code response time is back to normal
	t.Log("Phase 5 (Recovery): Verifying generate-code response time after MongoDB recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	startRecovery := time.Now()

	code, _, err = cli.Request(reqCtx5, "POST", "/v1/templates/generate-code", headers, body)
	recoveryDuration := time.Since(startRecovery)

	require.NoError(t, err, "generate-code should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "generate-code should return 200 after MongoDB recovery")
	t.Logf("Phase 5 (Recovery): generate-code responded in %v after recovery", recoveryDuration)
}

// validateBlocksRequestBody returns a valid request body for the validate endpoint.
// This is a stateless endpoint that validates block structures and calls GenerateCode.
func validateBlocksRequestBody() map[string]any {
	return map[string]any{
		"blocks": []map[string]any{
			{"type": "text", "content": "chaos test"},
		},
	}
}

// TestChaos_ValidateBlocks_MongoDBConnectionLoss verifies that the validate endpoint
// remains fully operational when MongoDB is completely unreachable. Since this endpoint
// validates block structures using pure functions and calls GenerateCode (also stateless),
// it should be immune to MongoDB failures — validating resilience isolation.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_ValidateBlocks_MongoDBConnectionLoss(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for connection loss simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()
	body := validateBlocksRequestBody()

	// Phase 1 (Normal): Verify validate endpoint works under normal conditions
	t.Log("Phase 1 (Normal): Verifying validate endpoint works before MongoDB connection loss...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, respBody, err := cli.Request(reqCtx, "POST", "/v1/templates/validate", headers, body)
	require.NoError(t, err, "validate request should succeed before chaos injection")
	require.Equal(t, 200, code, "validate should return 200 before chaos injection")

	var normalResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &normalResponse), "validate should return valid JSON before chaos injection")
	t.Log("Phase 1 (Normal): validate endpoint is healthy and returns valid JSON")

	// Phase 2 (Inject): Simulate complete MongoDB connection loss via Toxiproxy
	t.Log("Phase 2 (Inject): Injecting MongoDB connection loss via Toxiproxy...")
	err = chaosutil.InjectConnectionLoss(mongoProxy)
	require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")

	// Wait for the connection loss to take effect on existing connections
	time.Sleep(5 * time.Second)
	t.Log("Phase 2 (Inject): MongoDB connection loss injected successfully")

	// Phase 3 (Verify): validate endpoint should STILL return 200 (stateless endpoint)
	t.Log("Phase 3 (Verify): Verifying validate endpoint remains operational during MongoDB outage...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	code, respBody, err = cli.Request(reqCtx3, "POST", "/v1/templates/validate", headers, body)
	require.NoError(t, err, "validate should respond even when MongoDB is down (stateless endpoint)")
	require.Equal(t, 200, code, "validate should return 200 when MongoDB is down (no DB dependency)")

	var chaosResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &chaosResponse), "validate should return valid JSON during MongoDB outage")
	assert.JSONEq(t, string(normalResponse), string(chaosResponse), "validate response should be identical during MongoDB outage (pure function)")
	t.Log("Phase 3 (Verify): validate endpoint confirmed operational during MongoDB connection loss")

	// Phase 4 (Restore): Remove all toxics to restore MongoDB connectivity
	t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore MongoDB connectivity...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB connectivity restoration")
	t.Log("Phase 4 (Restore): MongoDB connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify validate still returns correct data after recovery
	t.Log("Phase 5 (Recovery): Verifying validate endpoint after MongoDB recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, respBody, err = cli.Request(reqCtx5, "POST", "/v1/templates/validate", headers, body)
	require.NoError(t, err, "validate should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "validate should return 200 after MongoDB recovery")

	var recoveryResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &recoveryResponse), "validate should return valid JSON after recovery")
	assert.JSONEq(t, string(normalResponse), string(recoveryResponse), "validate response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): validate endpoint fully verified after MongoDB recovery")
}

// TestChaos_ValidateBlocks_RabbitMQNetworkPartition verifies that the validate endpoint
// remains fully operational when RabbitMQ is experiencing a network partition (50% packet loss).
// Since validate is a stateless endpoint that does not publish or consume messages, it should
// be completely immune to RabbitMQ failures — validating resilience isolation.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_ValidateBlocks_RabbitMQNetworkPartition(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	rabbitProxy, useToxiproxy := getRabbitMQProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for network partition simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()
	body := validateBlocksRequestBody()

	// Phase 1 (Normal): Verify validate works and capture baseline response
	t.Log("Phase 1 (Normal): Verifying validate endpoint before RabbitMQ partition...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, respBody, err := cli.Request(reqCtx, "POST", "/v1/templates/validate", headers, body)
	require.NoError(t, err, "validate should succeed before chaos injection")
	require.Equal(t, 200, code, "validate should return 200 before chaos injection")

	var normalResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &normalResponse), "validate should return valid JSON before chaos injection")
	t.Log("Phase 1 (Normal): validate endpoint is healthy")

	// Phase 2 (Inject): Simulate network partition on RabbitMQ (50% packet loss)
	t.Log("Phase 2 (Inject): Injecting network partition (50% packet loss) on RabbitMQ...")
	err = chaosutil.InjectPacketLoss(rabbitProxy, 50)
	require.NoError(t, err, "Failed to inject packet loss via Toxiproxy")

	time.Sleep(3 * time.Second)
	t.Log("Phase 2 (Inject): RabbitMQ network partition injected successfully")

	// Phase 3 (Verify): validate should respond fast (under 500ms) despite RabbitMQ partition
	t.Log("Phase 3 (Verify): Verifying validate response during RabbitMQ partition...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	startChaos := time.Now()

	code, respBody, err = cli.Request(reqCtx3, "POST", "/v1/templates/validate", headers, body)
	chaosDuration := time.Since(startChaos)

	require.NoError(t, err, "validate should respond during RabbitMQ partition (no RabbitMQ dependency)")
	require.Equal(t, 200, code, "validate should return 200 during RabbitMQ partition")

	var chaosResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &chaosResponse), "validate should return valid JSON during RabbitMQ partition")
	assert.JSONEq(t, string(normalResponse), string(chaosResponse), "validate response should be identical during RabbitMQ partition")

	assert.Less(t, chaosDuration, 500*time.Millisecond,
		"validate response time should stay under 500ms during RabbitMQ partition (got %v)", chaosDuration)
	t.Logf("Phase 3 (Verify): validate responded in %v during RabbitMQ partition (limit: 500ms)", chaosDuration)

	// Phase 4 (Restore): Remove all toxics to restore RabbitMQ connectivity
	t.Log("Phase 4 (Restore): Removing packet loss toxic from RabbitMQ proxy...")
	err = chaosutil.RemoveAllToxics(rabbitProxy)
	require.NoError(t, err, "Failed to remove toxics from RabbitMQ proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after RabbitMQ partition removal")
	t.Log("Phase 4 (Restore): RabbitMQ connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify validate still works correctly after recovery
	t.Log("Phase 5 (Recovery): Verifying validate after RabbitMQ recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, respBody, err = cli.Request(reqCtx5, "POST", "/v1/templates/validate", headers, body)
	require.NoError(t, err, "validate should succeed after RabbitMQ recovery")
	require.Equal(t, 200, code, "validate should return 200 after RabbitMQ recovery")

	var recoveryResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &recoveryResponse), "validate should return valid JSON after recovery")
	assert.JSONEq(t, string(normalResponse), string(recoveryResponse), "validate response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): validate endpoint verified after RabbitMQ recovery")
}

// TestChaos_ValidateBlocks_ValkeyUnavailable verifies that the validate endpoint
// remains fully operational when Valkey/Redis is completely unreachable. Since this endpoint
// validates block structures using pure functions and does not read from or write to any cache,
// it should be immune to cache failures — validating resilience isolation.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_ValidateBlocks_ValkeyUnavailable(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	valkeyProxy, useToxiproxy := getValkeyProxy()
	if !useToxiproxy {
		t.Skip("Skipping test - Toxiproxy not available (required for Valkey unavailability simulation)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()
	body := validateBlocksRequestBody()

	// Phase 1 (Normal): Verify validate endpoint works under normal conditions
	t.Log("Phase 1 (Normal): Verifying validate endpoint before Valkey outage...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, respBody, err := cli.Request(reqCtx, "POST", "/v1/templates/validate", headers, body)
	require.NoError(t, err, "validate should succeed before chaos injection")
	require.Equal(t, 200, code, "validate should return 200 before chaos injection")

	var normalResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &normalResponse), "validate should return valid JSON before chaos injection")
	t.Log("Phase 1 (Normal): validate endpoint is healthy and returns valid JSON")

	// Phase 2 (Inject): Simulate complete Valkey unavailability via Toxiproxy
	t.Log("Phase 2 (Inject): Injecting complete Valkey connection loss via Toxiproxy...")
	err = chaosutil.InjectConnectionLoss(valkeyProxy)
	require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")

	time.Sleep(5 * time.Second)
	t.Log("Phase 2 (Inject): Valkey connection loss injected successfully")

	// Phase 3 (Verify): validate endpoint should STILL return 200 with valid JSON
	t.Log("Phase 3 (Verify): Verifying validate endpoint remains operational during Valkey outage...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	code, respBody, err = cli.Request(reqCtx3, "POST", "/v1/templates/validate", headers, body)
	require.NoError(t, err, "validate should respond during Valkey outage (stateless endpoint)")
	require.Equal(t, 200, code, "validate should return 200 during Valkey outage")

	var chaosResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &chaosResponse), "validate should return valid JSON during Valkey outage")
	assert.JSONEq(t, string(normalResponse), string(chaosResponse), "validate response should be identical during Valkey outage (pure function)")
	t.Log("Phase 3 (Verify): validate endpoint confirmed operational during Valkey outage")

	// Phase 4 (Restore): Remove all toxics to restore Valkey connectivity
	t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore Valkey connectivity...")
	err = chaosutil.RemoveAllToxics(valkeyProxy)
	require.NoError(t, err, "Failed to remove toxics from Valkey proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after Valkey connectivity restoration")
	t.Log("Phase 4 (Restore): Valkey connectivity restored, system is ready")

	// Phase 5 (Recovery): Verify validate still returns correct data after recovery
	t.Log("Phase 5 (Recovery): Verifying validate endpoint after Valkey recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, respBody, err = cli.Request(reqCtx5, "POST", "/v1/templates/validate", headers, body)
	require.NoError(t, err, "validate should succeed after Valkey recovery")
	require.Equal(t, 200, code, "validate should return 200 after Valkey recovery")

	var recoveryResponse json.RawMessage
	require.NoError(t, json.Unmarshal(respBody, &recoveryResponse), "validate should return valid JSON after recovery")
	assert.JSONEq(t, string(normalResponse), string(recoveryResponse), "validate response should be consistent after recovery")
	t.Log("Phase 5 (Recovery): validate endpoint fully verified after Valkey recovery")
}
