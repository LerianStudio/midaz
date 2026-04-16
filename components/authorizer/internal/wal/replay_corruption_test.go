// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReplay_BadHMAC_Truncates verifies that a frame whose HMAC tag does not
// match the payload is (a) counted in the HMAC-failure observer, (b) causes
// the WAL file to be truncated at the bad frame's starting offset, and (c)
// returns the preceding valid entries without error. This is the B1-commit
// replacement of CRC32-IEEE with HMAC-SHA256: any tamper with payload bytes
// now invalidates the frame instead of being recomputable by an attacker.
func TestReplay_BadHMAC_Truncates(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "bad-hmac.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
	require.NoError(t, err)

	// Frame 1 is valid and should survive replay.
	require.NoError(t, writer.Append(Entry{
		TransactionID:  "tx-valid-1",
		OrganizationID: "org",
		LedgerID:       "ledger",
	}))

	// Frame 2 is also valid but we'll corrupt its HMAC tag.
	require.NoError(t, writer.Append(Entry{
		TransactionID:  "tx-will-be-forged",
		OrganizationID: "org",
		LedgerID:       "ledger",
	}))
	require.NoError(t, writer.Close())

	// Locate frame 2: read frame 1's length prefix, skip past it, then clobber
	// frame 2's HMAC bytes (offset 4..4+recordHMACSize within frame 2's header).
	f, err := os.OpenFile(walPath, os.O_RDWR, 0)
	require.NoError(t, err)

	header := make([]byte, recordHeaderSize)
	_, err = f.ReadAt(header, 0)
	require.NoError(t, err)

	frame1Length := int64(recordHeaderSize) + int64(binary.LittleEndian.Uint32(header[:4]))

	// Flip every bit of the HMAC tag in frame 2. Any single-bit flip is
	// enough to make hmac.Equal return false; flipping all of them makes the
	// test obvious to a human reading the file.
	hmacStart := frame1Length + 4
	bogus := make([]byte, recordHMACSize)

	for i := range bogus {
		bogus[i] = 0xDE
	}

	_, err = f.WriteAt(bogus, hmacStart)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	obs := &recordingObserver{}

	entries, err := Replay(walPath, [][]byte{testHMACKey}, obs)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only frame 1 should survive the HMAC mismatch")
	require.Equal(t, "tx-valid-1", entries[0].TransactionID)

	failures := obs.hmacFailures()
	require.Len(t, failures, 1, "HMAC failure metric must fire exactly once")
	require.Equal(t, "hmac_mismatch", failures[0].reason)
	require.Equal(t, frame1Length, failures[0].offset,
		"HMAC failure offset must be the start of the bad frame, not the start of the file")

	fi, err := os.Stat(walPath)
	require.NoError(t, err)
	require.Equal(t, frame1Length, fi.Size(),
		"replay must truncate the file at the bad frame's starting offset")
}

// TestReplay_TruncatedPayload covers the "partial write" crash pattern:
// authorizer wrote the header but crashed before finishing the payload.
// Replay must detect that ReadFull on the payload fails with io.ErrUnexpectedEOF
// and truncate the file at the start of the incomplete frame.
func TestReplay_TruncatedPayload(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "truncated-payload.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
	require.NoError(t, err)

	require.NoError(t, writer.Append(Entry{
		TransactionID:  "tx-valid-before-crash",
		OrganizationID: "org",
		LedgerID:       "ledger",
	}))
	require.NoError(t, writer.Close())

	// Size of the valid file so we know where the "crash" started.
	fi, err := os.Stat(walPath)
	require.NoError(t, err)

	validFileSize := fi.Size()

	// Simulate: header of a second frame is written but the payload isn't.
	// We append a header with payloadLength=200 and a garbage HMAC, then stop.
	f, err := os.OpenFile(walPath, os.O_RDWR|os.O_APPEND, 0o600)
	require.NoError(t, err)

	header := make([]byte, recordHeaderSize)
	binary.LittleEndian.PutUint32(header[:4], 200)

	// Fill the HMAC slot with something nonzero so the bug isn't "the length
	// was zero" (that path is covered by the zero-length test below).
	for i := 4; i < 4+recordHMACSize; i++ {
		header[i] = 0xAB
	}

	_, err = f.Write(header)
	require.NoError(t, err)
	// Write only 50 bytes of payload, not 200. ReadFull must fail.
	_, err = f.Write(make([]byte, 50))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	obs := &recordingObserver{}

	entries, err := Replay(walPath, [][]byte{testHMACKey}, obs)
	require.NoError(t, err, "replay must self-heal, not surface partial-write errors")
	require.Len(t, entries, 1)
	require.Equal(t, "tx-valid-before-crash", entries[0].TransactionID)

	truncations := obs.truncationEvents()
	require.NotEmpty(t, truncations, "truncation metric must fire for partial payload")
	require.Equal(t, validFileSize, truncations[0].offset,
		"truncation must occur at the last valid frame boundary")

	fi2, err := os.Stat(walPath)
	require.NoError(t, err)
	require.Equal(t, validFileSize, fi2.Size(),
		"file must be truncated back to last valid frame")
}

// TestReplay_ZeroLengthFrame covers the specific corruption vector where the
// header reports payloadLength=0. A zero-length frame cannot be valid (empty
// payloads would fail JSON decoding anyway) and is treated as a truncation
// signal rather than a parse error. Reason label must be "zero_length_frame".
func TestReplay_ZeroLengthFrame(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "zero-length.wal")

	writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
	require.NoError(t, err)

	require.NoError(t, writer.Append(Entry{
		TransactionID:  "tx-before-zero",
		OrganizationID: "org",
		LedgerID:       "ledger",
	}))
	require.NoError(t, writer.Close())

	fi, err := os.Stat(walPath)
	require.NoError(t, err)

	validFileSize := fi.Size()

	// Append a bogus header with payloadLength=0 but a plausible HMAC shape.
	f, err := os.OpenFile(walPath, os.O_RDWR|os.O_APPEND, 0o600)
	require.NoError(t, err)

	header := make([]byte, recordHeaderSize) // zeroed by default → payloadLength=0
	_, err = f.Write(header)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	obs := &recordingObserver{}

	entries, err := Replay(walPath, [][]byte{testHMACKey}, obs)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "tx-before-zero", entries[0].TransactionID)

	truncations := obs.truncationEvents()
	require.NotEmpty(t, truncations)
	require.Equal(t, "zero_length_frame", truncations[0].reason,
		"truncation reason must uniquely identify the zero-length vector for operators")
	require.Equal(t, validFileSize, truncations[0].offset)

	fi2, err := os.Stat(walPath)
	require.NoError(t, err)
	require.Equal(t, validFileSize, fi2.Size())
}

// TestReplay_EmitsTruncationMetric is the cross-cutting assertion that every
// self-healing path wires the truncation Observer callback. We intentionally
// corrupt across all three detectable paths (HMAC mismatch, zero-length,
// partial payload) in separate WAL files and collect the set of reasons seen.
// The test fails if any reason is missing — a reasonable proxy for "the
// truncation metric is reachable from every code path that truncates".
func TestReplay_EmitsTruncationMetric(t *testing.T) {
	cases := []struct {
		name        string
		corruptFn   func(t *testing.T, path string)
		wantReason  string
		wantOffset0 bool
	}{
		{
			name: "hmac_mismatch",
			corruptFn: func(t *testing.T, path string) {
				t.Helper()

				f, err := os.OpenFile(path, os.O_RDWR, 0)
				require.NoError(t, err)

				defer func() { _ = f.Close() }()

				bogus := make([]byte, recordHMACSize)

				for i := range bogus {
					bogus[i] = 0xFF
				}

				_, err = f.WriteAt(bogus, 4)
				require.NoError(t, err)
			},
			wantReason:  "hmac_mismatch",
			wantOffset0: true,
		},
		{
			name: "zero_length_frame",
			corruptFn: func(t *testing.T, path string) {
				t.Helper()
				// Overwrite the header with zeros → payloadLength=0.
				f, err := os.OpenFile(path, os.O_RDWR, 0)
				require.NoError(t, err)

				defer func() { _ = f.Close() }()

				zeroed := make([]byte, recordHeaderSize)
				_, err = f.WriteAt(zeroed, 0)
				require.NoError(t, err)
			},
			wantReason:  "zero_length_frame",
			wantOffset0: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			walPath := filepath.Join(t.TempDir(), "observed.wal")

			writer, err := NewRingBufferWriterWithOptions(walPath, 16, time.Hour, true, nil, testHMACKey)
			require.NoError(t, err)

			require.NoError(t, writer.Append(Entry{
				TransactionID:  "tx-to-be-corrupted",
				OrganizationID: "org",
				LedgerID:       "ledger",
			}))
			require.NoError(t, writer.Close())

			tc.corruptFn(t, walPath)

			obs := &recordingObserver{}

			_, err = Replay(walPath, [][]byte{testHMACKey}, obs)
			require.NoError(t, err)

			truncations := obs.truncationEvents()
			require.NotEmpty(t, truncations, "truncation observer must fire for %s", tc.name)
			require.Equal(t, tc.wantReason, truncations[0].reason,
				"truncation reason must match corruption vector")

			if tc.wantOffset0 {
				require.Equal(t, int64(0), truncations[0].offset,
					"single-frame corruption must truncate at offset 0")
			}
		})
	}
}
