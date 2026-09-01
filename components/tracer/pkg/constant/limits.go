// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// CounterRetentionDays defines how long expired usage counters are kept before cleanup.
// After a counter's period expires, it's retained for this duration for auditing purposes.
// This applies to both periodic limits (DAILY/WEEKLY/MONTHLY) and CUSTOM period limits.
//
// Usage:
//   - For DAILY/WEEKLY/MONTHLY: expiresAt = resetAt + CounterRetentionDays
//   - For CUSTOM: expiresAt = customEndDate + CounterRetentionDays
const CounterRetentionDays = 90

// MaxMetadataEntries defines the maximum number of metadata key-value pairs allowed
// in context objects (Segment, Portfolio, Merchant) for validation requests.
const MaxMetadataEntries = 50

// MaxMetadataKeyLength defines the maximum length of a metadata key string
// in context objects for validation requests.
const MaxMetadataKeyLength = 64

// GlobalScopeKey is the scope key used when a limit has no scopes defined.
const GlobalScopeKey = "global"
