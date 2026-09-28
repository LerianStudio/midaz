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

// TestReconcileAccountClosings_WithoutACommandDoesNothing pins that the pass is
// inert when the runner carries no use case, so a cycle cannot fail on the
// reconciliation of a wiring that does not exist.
func TestReconcileAccountClosings_WithoutACommandDoesNothing(t *testing.T) {
	runner := &RedisQueueConsumer{Logger: recoveryQuietLogger{}}

	assert.NotPanics(t, func() { runner.reconcileAccountClosings(context.Background()) })
}

// TestReconcileAccountClosings_WithoutAProtectionSurfaceDoesNothing pins the same
// for a use case wired without the repositories the protection lives on: the pass
// scans nothing rather than reaching for a surface it does not have.
func TestReconcileAccountClosings_WithoutAProtectionSurfaceDoesNothing(t *testing.T) {
	runner := &RedisQueueConsumer{Logger: recoveryQuietLogger{}, Command: &command.UseCase{}}

	assert.NotPanics(t, func() { runner.reconcileAccountClosings(context.Background()) })
}
