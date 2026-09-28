// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

// TestReconcileTransactionGroups_WithoutACommandDoesNothing pins that the pass
// is inert when the runner carries no use case.
func TestReconcileTransactionGroups_WithoutACommandDoesNothing(t *testing.T) {
	runner := &RedisQueueConsumer{Logger: recoveryQuietLogger{}}

	assert.NotPanics(t, func() { runner.reconcileTransactionGroups(context.Background()) })
}

// TestReconcileTransactionGroups_WithoutAGroupSurfaceDoesNothing pins the same
// for a use case wired without the transaction-group repository.
func TestReconcileTransactionGroups_WithoutAGroupSurfaceDoesNothing(t *testing.T) {
	runner := &RedisQueueConsumer{Logger: recoveryQuietLogger{}, Command: &command.UseCase{}}

	assert.NotPanics(t, func() { runner.reconcileTransactionGroups(context.Background()) })
}
