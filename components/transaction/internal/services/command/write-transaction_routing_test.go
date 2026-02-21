// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"strconv"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestResolveBTORoutingKey(t *testing.T) {
	baseKey := "transaction.transaction_balance_operation.key"
	txID := uuid.MustParse("8d661968-915f-4e90-b9ba-63f3f5d0bb49")
	router := shard.NewRouter(8)

	t.Run("sharding disabled keeps legacy routing key", func(t *testing.T) {
		got := resolveBTORoutingKey(baseKey, txID, router, false)

		assert.Equal(t, baseKey, got)
	})

	t.Run("enabled but router missing keeps legacy routing key", func(t *testing.T) {
		got := resolveBTORoutingKey(baseKey, txID, nil, true)

		assert.Equal(t, baseKey, got)
	})

	t.Run("enabled appends deterministic shard suffix", func(t *testing.T) {
		expectedShardID := router.Resolve(txID.String())
		expectedKey := baseKey + ".shard_" + strconv.Itoa(expectedShardID)

		gotA := resolveBTORoutingKey(baseKey, txID, router, true)
		gotB := resolveBTORoutingKey(baseKey, txID, router, true)

		assert.Equal(t, expectedKey, gotA)
		assert.Equal(t, gotA, gotB, "routing key must be deterministic")
	})
}
