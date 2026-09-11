// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cachepolicy_test

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

func TestPolicyConstants(t *testing.T) {
	t.Parallel()

	if cachepolicy.BalanceTTL != 24*time.Hour {
		t.Fatalf("BalanceTTL = %s, want 24h", cachepolicy.BalanceTTL)
	}

	if cachepolicy.HashTag != "{transactions}" {
		t.Fatalf("HashTag = %q, want %q", cachepolicy.HashTag, "{transactions}")
	}

	if cachepolicy.EngineRecoverQueue != "engine:{transactions}:recover:v2" {
		t.Fatalf("EngineRecoverQueue = %q, want %q", cachepolicy.EngineRecoverQueue, "engine:{transactions}:recover:v2")
	}

	if cachepolicy.DeletionMarkerSuffix != ":deleted" {
		t.Fatalf("DeletionMarkerSuffix = %q, want %q", cachepolicy.DeletionMarkerSuffix, ":deleted")
	}

	marker, ok := cachepolicy.DeletionMarkerKey("tenant:a:balance:{transactions}:org:ledger:@source#default")
	if !ok || marker != "tenant:a:balance_delete_marker:{transactions}:org:ledger:@source#default" {
		t.Fatalf("DeletionMarkerKey() = %q, %v", marker, ok)
	}
}

func TestLuaSource(t *testing.T) {
	t.Parallel()

	source := "return {ARGV[1], '\u2603'}\n"
	want := "local balance_cache_ttl_seconds = 86400\n" +
		"local balance_deletion_marker_suffix = \":deleted\"\n" +
		"local balance_cache_namespace_prefix = \"balance:{transactions}:\"\n" +
		"local balance_deletion_marker_namespace_prefix = \"balance_delete_marker:{transactions}:\"\n" +
		"local transaction_hash_tag = \"{transactions}\"\n" +
		source

	first := cachepolicy.LuaSource(source)
	second := cachepolicy.LuaSource(source)

	if first != want {
		t.Fatalf("LuaSource() = %q, want %q", first, want)
	}

	if second != first {
		t.Fatalf("LuaSource() is not deterministic: first %q, second %q", first, second)
	}
}
