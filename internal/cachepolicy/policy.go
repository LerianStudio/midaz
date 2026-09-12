// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package cachepolicy defines shared balance-cache and transaction-key policy.
package cachepolicy

import (
	"strconv"
	"strings"
	"time"
)

const (
	BalanceTTL                    = 24 * time.Hour
	HashTag                       = "{transactions}"
	BalanceNamespacePrefix        = "balance:" + HashTag + ":"
	DeletionMarkerNamespacePrefix = "balance_delete_marker:" + HashTag + ":"
	// EngineRecoverQueue stores engine recovery envelopes. Schema evolution is
	// identified by each record's formatVersion. The queue remains separate from
	// the legacy transaction backup hash while sharing its Redis Cluster slot.
	EngineRecoverQueue   = "engine:" + HashTag + ":recover"
	DeletionMarkerSuffix = ":deleted"
)

// DeletionMarkerKey moves a balance key into the dedicated marker namespace
// while preserving its tenant prefix, logical identity, and Redis hash slot.
func DeletionMarkerKey(balanceKey string) (string, bool) {
	marker := strings.Replace(balanceKey, BalanceNamespacePrefix, DeletionMarkerNamespacePrefix, 1)

	return marker, marker != balanceKey
}

// LuaSource prepends the shared cache policy to a Lua script.
func LuaSource(source string) string {
	return "local balance_cache_ttl_seconds = " + strconv.FormatInt(int64(BalanceTTL/time.Second), 10) + "\n" +
		"local balance_deletion_marker_suffix = " + strconv.Quote(DeletionMarkerSuffix) + "\n" +
		"local balance_cache_namespace_prefix = " + strconv.Quote(BalanceNamespacePrefix) + "\n" +
		"local balance_deletion_marker_namespace_prefix = " + strconv.Quote(DeletionMarkerNamespacePrefix) + "\n" +
		"local transaction_hash_tag = " + strconv.Quote(HashTag) + "\n" +
		source
}
