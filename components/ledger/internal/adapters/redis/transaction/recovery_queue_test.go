// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

func TestRecoveryQueueSourceResolvesOnlyClosedOrigins(t *testing.T) {
	t.Parallel()

	legacy, err := recoveryQueueKey(RecoveryQueueSourceLegacyBackup)
	require.NoError(t, err)
	require.Equal(t, TransactionBackupQueue, legacy)
	recoverKey, err := recoveryQueueKey(RecoveryQueueSourceEngineRecover)
	require.NoError(t, err)
	require.Equal(t, cachepolicy.EngineRecoverQueue, recoverKey)
	_, err = recoveryQueueKey(RecoveryQueueSource("arbitrary-key"))
	require.Error(t, err)
}
