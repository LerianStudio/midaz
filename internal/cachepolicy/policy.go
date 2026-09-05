// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package cachepolicy defines shared balance-cache and transaction-key policy.
package cachepolicy

import (
	"strconv"
	"time"
)

const (
	BalanceTTL           = 24 * time.Hour
	HashTag              = "{transactions}"
	DeletionMarkerSuffix = ":deleted"
)

// LuaSource prepends the shared cache policy to a Lua script.
func LuaSource(source string) string {
	return "local balance_cache_ttl_seconds = " + strconv.FormatInt(int64(BalanceTTL/time.Second), 10) + "\n" +
		"local balance_deletion_marker_suffix = " + strconv.Quote(DeletionMarkerSuffix) + "\n" +
		"local transaction_hash_tag = " + strconv.Quote(HashTag) + "\n" +
		source
}
