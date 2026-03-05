// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWALEntryBackwardCompatDeserializesWithoutCrossShardFields(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer-backcompat.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil)
	require.NoError(t, err)

	// Write an entry WITHOUT CrossShard/Participants (simulating old format).
	err = writer.Append(Entry{
		TransactionID:     "tx-old-format",
		OrganizationID:    "org-1",
		LedgerID:          "ledger-1",
		Pending:           false,
		TransactionStatus: "CREATED",
	})
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	entries, err := Replay(walPath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "tx-old-format", entries[0].TransactionID)

	// Verify zero-value defaults: CrossShard=false, Participants=nil.
	require.False(t, entries[0].CrossShard)
	require.Nil(t, entries[0].Participants)
}

func TestWALEntryCrossShardRoundTrip(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer-crossshard.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil)
	require.NoError(t, err)

	participants := []WALParticipant{
		{InstanceAddr: "localhost:9090", PreparedTxID: "ptx-local-1", IsLocal: true},
		{InstanceAddr: "peer-1:9090", PreparedTxID: "ptx-remote-1", IsLocal: false},
		{InstanceAddr: "peer-2:9090", PreparedTxID: "ptx-remote-2", IsLocal: false},
	}

	err = writer.Append(Entry{
		TransactionID:     "tx-cross-shard",
		OrganizationID:    "org-1",
		LedgerID:          "ledger-1",
		Pending:           false,
		TransactionStatus: "CREATED",
		CrossShard:        true,
		Participants:      participants,
	})
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	entries, err := Replay(walPath)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	entry := entries[0]
	require.Equal(t, "tx-cross-shard", entry.TransactionID)
	require.True(t, entry.CrossShard)
	require.Len(t, entry.Participants, 3)

	require.Equal(t, "localhost:9090", entry.Participants[0].InstanceAddr)
	require.Equal(t, "ptx-local-1", entry.Participants[0].PreparedTxID)
	require.True(t, entry.Participants[0].IsLocal)

	require.Equal(t, "peer-1:9090", entry.Participants[1].InstanceAddr)
	require.Equal(t, "ptx-remote-1", entry.Participants[1].PreparedTxID)
	require.False(t, entry.Participants[1].IsLocal)

	require.Equal(t, "peer-2:9090", entry.Participants[2].InstanceAddr)
	require.Equal(t, "ptx-remote-2", entry.Participants[2].PreparedTxID)
	require.False(t, entry.Participants[2].IsLocal)
}

func TestWALEntryMixedOldAndNewFormatsReplay(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "authorizer-mixed.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil)
	require.NoError(t, err)

	// Write an old-format entry (no cross-shard fields).
	err = writer.Append(Entry{
		TransactionID:     "tx-local-only",
		OrganizationID:    "org-1",
		LedgerID:          "ledger-1",
		TransactionStatus: "CREATED",
	})
	require.NoError(t, err)

	// Write a new-format entry (with cross-shard fields).
	err = writer.Append(Entry{
		TransactionID:     "tx-cross-shard",
		OrganizationID:    "org-1",
		LedgerID:          "ledger-1",
		TransactionStatus: "CREATED",
		CrossShard:        true,
		Participants: []WALParticipant{
			{InstanceAddr: "localhost:9090", PreparedTxID: "ptx-1", IsLocal: true},
		},
	})
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	entries, err := Replay(walPath)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	// First entry: old format, zero-value cross-shard fields.
	require.Equal(t, "tx-local-only", entries[0].TransactionID)
	require.False(t, entries[0].CrossShard)
	require.Nil(t, entries[0].Participants)

	// Second entry: new format with cross-shard metadata.
	require.Equal(t, "tx-cross-shard", entries[1].TransactionID)
	require.True(t, entries[1].CrossShard)
	require.Len(t, entries[1].Participants, 1)
	require.Equal(t, "ptx-1", entries[1].Participants[0].PreparedTxID)
}

// TestWALEntryDeserializesFromLegacyJSON validates backward compatibility by
// unmarshaling a hardcoded JSON string captured from the pre-cross-shard WAL
// schema. This catches accidental JSON key renames that a round-trip test
// (marshal then unmarshal with current code) would silently miss.
func TestWALEntryDeserializesFromLegacyJSON(t *testing.T) {
	// Frozen legacy payload: no crossShard, participants, or mutations keys.
	const legacyJSON = `{
		"transactionId": "tx-legacy-001",
		"organizationId": "org-legacy",
		"ledgerId": "ledger-legacy",
		"pending": false,
		"transactionStatus": "CREATED",
		"operations": [],
		"createdAt": "2026-01-15T10:30:00Z"
	}`

	var entry Entry

	err := json.Unmarshal([]byte(legacyJSON), &entry)
	require.NoError(t, err)

	require.Equal(t, "tx-legacy-001", entry.TransactionID)
	require.Equal(t, "org-legacy", entry.OrganizationID)
	require.Equal(t, "ledger-legacy", entry.LedgerID)
	require.False(t, entry.Pending)
	require.Equal(t, "CREATED", entry.TransactionStatus)

	// Extension fields must default to zero values when absent from legacy JSON.
	require.False(t, entry.CrossShard)
	require.Nil(t, entry.Participants)
	require.Nil(t, entry.Mutations)
}

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
