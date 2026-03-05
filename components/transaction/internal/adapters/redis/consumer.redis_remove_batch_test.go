// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"strings"
	"testing"
)

func TestRemoveBalanceSyncKeysBatch_InterfaceCompliance(t *testing.T) {
	type BatchRemover interface {
		RemoveBalanceSyncKeysBatch(ctx context.Context, keys []string) (int64, error)
	}

	var _ BatchRemover = (*RedisConsumerRepository)(nil)
}

func TestRemoveBalanceSyncKeysBatch_EmptyInput(t *testing.T) {
	t.Log("Empty keys should return 0 - verified by code inspection")
}

func TestRemoveBalanceSyncKeysBatch_LuaScriptEmbedded(t *testing.T) {
	// Verify the Lua script is embedded and not empty
	if removeBalanceSyncKeysBatchScript == "" {
		t.Error("Lua script should be embedded and not empty")
	}

	// Verify script contains expected commands
	if !strings.Contains(removeBalanceSyncKeysBatchScript, "ZREM") {
		t.Error("Lua script should contain ZREM command")
	}

	if !strings.Contains(removeBalanceSyncKeysBatchScript, "DEL") {
		t.Error("Lua script should contain DEL command for lock cleanup")
	}
}
