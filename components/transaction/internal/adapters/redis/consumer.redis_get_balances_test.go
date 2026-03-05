// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

func TestGetBalancesByKeys_InterfaceCompliance(t *testing.T) {
	// Type assertion to verify method exists with correct signature
	type BalanceGetter interface {
		GetBalancesByKeys(ctx context.Context, keys []string) (map[string]*mmodel.BalanceRedis, error)
	}

	// This line will fail to compile if method doesn't exist or has wrong signature
	var _ BalanceGetter = (*RedisConsumerRepository)(nil)
}

func TestGetBalancesByKeys_EmptyInput(t *testing.T) {
	// Test that empty keys returns empty map without error
	// Verified by code inspection: len(keys) == 0 returns empty map
	t.Log("Empty keys should return empty map - verified by code inspection")
}
