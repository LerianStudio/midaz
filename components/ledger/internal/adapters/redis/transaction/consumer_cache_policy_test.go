// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

func TestBalanceCachePolicyContract(t *testing.T) {
	t.Parallel()

	if want := cachepolicy.LuaSource(balanceAtomicOperationLuaSource); balanceAtomicOperationLua != want {
		t.Fatal("balance atomic Lua script does not use the rendered cache policy")
	}

	if !strings.Contains(balanceAtomicOperationLua, "local ttl = balance_cache_ttl_seconds") {
		t.Fatal("balance atomic Lua script does not use the policy TTL")
	}

	if !strings.Contains(balanceAtomicOperationLua, "ARGV[i] .. balance_deletion_marker_suffix") {
		t.Fatal("balance atomic Lua script does not use the policy deletion marker suffix")
	}

	if balanceCacheSettingsTTL != cachepolicy.BalanceTTL {
		t.Fatalf("balance settings TTL = %s, want %s", balanceCacheSettingsTTL, cachepolicy.BalanceTTL)
	}

	if balanceCacheSettingsTTL != 24*time.Hour {
		t.Fatalf("balance settings TTL = %s, want %s", balanceCacheSettingsTTL, 24*time.Hour)
	}

	if TransactionBackupQueue != "backup_queue:{transactions}" {
		t.Fatalf("transaction backup queue = %q, want %q", TransactionBackupQueue, "backup_queue:{transactions}")
	}

	if luaArgsPerOperation != 25 {
		t.Fatalf("Lua argument stride = %d, want 25", luaArgsPerOperation)
	}
}
