// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
)

// TransactionFactRecorder records the System Facts fact of a transaction write
// inside the SQL transaction that commits it, so the fact and the state change
// commit or roll back together.
type TransactionFactRecorder interface {
	// RecordTransactionCommitted is called once per transaction row the Store
	// created or transitioned in tx, before tx commits. It returns nil when the
	// fact was recorded, when the write is not a fact (hold, cancel) and when the
	// fact was refused by validation (the implementation logs and counts the
	// refusal; the business write still commits). Any other error means the
	// fact could not be recorded and the caller must roll back.
	RecordTransactionCommitted(ctx context.Context, tx *sql.Tx, record TransactionWriteSet) error
}
