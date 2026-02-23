// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRingBufferWriterSyncOnAppendPersistsImmediately(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, writer.Close())
	}()

	err = writer.Append(Entry{
		TransactionID:     "tx-1",
		OrganizationID:    "org-1",
		LedgerID:          "ledger-1",
		Pending:           false,
		TransactionStatus: "CREATED",
	})
	require.NoError(t, err)

	entries, err := Replay(walPath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "tx-1", entries[0].TransactionID)
}
