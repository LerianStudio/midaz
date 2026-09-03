// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/google/uuid"
)

// unblockedAccountsSource is the blocked-accounts source of truth the harnesses
// in this package hand to the transaction Redis repository.
//
// It reports no blocked account, which is simply the truth for a ledger these
// tests never block. It is NOT a way to switch the atomic block gate off: the
// repair path it feeds is additive (SADD, never DEL), so a block written by a
// real block command mid-test survives a rebuild from this source and still
// denies. What it removes is the cold-start failure — without a source the gate
// cannot repair an index it finds unhydrated, and refuses every transaction.
type unblockedAccountsSource struct{}

func (unblockedAccountsSource) ListBlockedAccountIDs(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
