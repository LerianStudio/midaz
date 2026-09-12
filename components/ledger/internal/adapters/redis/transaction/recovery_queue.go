// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"fmt"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

// RecoveryQueueSource identifies the Redis hash that owns a recovery record.
// It is carried through reading, completion, and exact acknowledgement so an
// identical field in another hash remains independent.
type RecoveryQueueSource string

const (
	RecoveryQueueSourceLegacyBackup  RecoveryQueueSource = "legacy_backup"
	RecoveryQueueSourceEngineRecover RecoveryQueueSource = "engine_recover"
)

func recoveryQueueKey(source RecoveryQueueSource) (string, error) {
	switch source {
	case RecoveryQueueSourceLegacyBackup:
		return TransactionBackupQueue, nil
	case RecoveryQueueSourceEngineRecover:
		return cachepolicy.EngineRecoverQueue, nil
	default:
		return "", fmt.Errorf("invalid recovery queue source %q", source)
	}
}

func (source RecoveryQueueSource) clearsLegacyAttempts() bool {
	return source == RecoveryQueueSourceLegacyBackup
}
