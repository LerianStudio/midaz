// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

//go:generate mockgen -source=usage_counter_cleanup_repository.go -destination=mocks/usage_counter_cleanup_repository_mock.go -package=mocks

import (
	"context"
	"time"
)

// UsageCounterCleanupRepository defines the interface for usage counter cleanup operations.
// This is a subset of query.UsageCounterRepository, containing only the methods needed
// for the cleanup worker.
type UsageCounterCleanupRepository interface {
	// DeleteExpiredCounters removes usage counters where expires_at < now.
	// Counters with NULL expires_at are preserved (never deleted).
	// This provides more accurate cleanup based on when counters should actually expire
	// rather than when they were last updated.
	// Returns the number of deleted counters.
	DeleteExpiredCounters(ctx context.Context, now time.Time) (int64, error)
}
