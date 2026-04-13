// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
)

// TestLuaScript_ContainsScheduleLogic is a structural verification test that checks
// if the Lua script contains the expected scheduling tokens (ZADD, KEYS[3], dueAt).
//
// IMPORTANT: This test only asserts string presence, not runtime behavior.
// It protects against accidental removal of scheduling logic during refactors.
// Actual scheduling behavior is tested in integration tests (balance.worker_integration_test.go).
func TestLuaScript_ContainsScheduleLogic(t *testing.T) {
	// Verify the script has ZADD for scheduling
	assert.True(t, strings.Contains(balanceAtomicOperationLua, "ZADD"),
		"Lua script should contain ZADD for balance scheduling")

	// Verify the script uses scheduleKey from KEYS[3]
	assert.True(t, strings.Contains(balanceAtomicOperationLua, "KEYS[3]"),
		"Lua script should reference schedule key from KEYS[3]")

	// Verify the script calculates dueAt for pre-expiry warning
	assert.True(t, strings.Contains(balanceAtomicOperationLua, "dueAt"),
		"Lua script should calculate dueAt for scheduling")
}

// TestScheduleKeyConstant verifies the schedule key constant matches
// what the BalanceSyncWorker expects.
func TestScheduleKeyConstant(t *testing.T) {
	expectedKey := "schedule:{transactions}:balance-sync"
	assert.Equal(t, expectedKey, utils.BalanceSyncScheduleKey,
		"Schedule key constant should match expected format")
}
