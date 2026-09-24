// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package reservationlock defines the shared account advisory-lock namespace.
package reservationlock

import (
	"hash/fnv"
	"slices"

	"github.com/google/uuid"
)

// AccountKey preserves the historical FNV-1a key over the UUID's sixteen bytes.
// Collisions only serialize unrelated accounts within the same tenant database.
func AccountKey(id uuid.UUID) int64 {
	h := fnv.New64a()
	_, _ = h.Write(id[:])

	return int64(h.Sum64()) // #nosec G115 -- all 64 hash bits are an advisory lock key
}

// AccountKeys sorts and deduplicates physical lock keys, so even hash collisions
// cannot reverse lock order between requests containing overlapping accounts.
func AccountKeys(ids []uuid.UUID) []int64 {
	keys := make([]int64, len(ids))
	for i, id := range ids {
		keys[i] = AccountKey(id)
	}

	slices.Sort(keys)

	return slices.Compact(keys)
}
