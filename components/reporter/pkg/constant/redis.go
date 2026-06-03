// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import "time"

type contextKey string

const (
	DataSourceDetailsKeyPrefix = "data_source_details"
	RedisTTL                   = 300

	// IdempotencyKeyPrefix is the Redis key prefix for idempotency locks.
	IdempotencyKeyPrefix = "idempotency"

	// IdempotencyTTL is the time-to-live for idempotency keys (30 seconds).
	IdempotencyTTL = 30 * time.Second

	// IdempotencyKeyCtx is the context key for client-provided Idempotency-Key header values.
	IdempotencyKeyCtx = contextKey("idempotency_key")

	// IdempotencyReplayedCtx is the context key for signaling a replayed idempotent response
	// from the service layer back to the handler.
	IdempotencyReplayedCtx = contextKey("idempotency_replayed")
)
