// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"crypto/hmac"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

const replayInitialCap = 1024

// Replay reads WAL entries from disk, verifying every frame's HMAC-SHA256 tag
// before surfacing its payload. keys is a non-empty list of candidate HMAC
// keys: the current key first, followed by any previous keys accepted during
// rotation. The first key that verifies a frame wins; if none verify, the
// frame (and everything after it) is treated as corrupt and truncated. When
// obs is non-nil, HMAC failures and truncations are reported as security
// metrics before the file is truncated.
func Replay(path string, keys [][]byte, obs Observer) ([]Entry, error) {
	if path == "" {
		return nil, constant.ErrWALPathEmpty
	}

	if len(keys) == 0 {
		return nil, constant.ErrWALHMACKeyRequired
	}

	for i, k := range keys {
		if len(k) < MinHMACKeyLength {
			return nil, fmt.Errorf("key index=%d length=%d minimum=%d: %w", i, len(k), MinHMACKeyLength, constant.ErrWALHMACKeyRequired)
		}
	}

	// NOTE: O_RDWR is intentional — Replay performs self-healing truncation
	// of corrupt/incomplete trailing frames, which requires write access.
	// O_NOFOLLOW rejects symlinks; Windows treats the flag as zero.
	file, err := os.OpenFile(path, os.O_RDWR|openNoFollowFlag(), 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("open wal file for replay: %w", err)
	}
	defer file.Close()

	entries := make([]Entry, 0, replayInitialCap)
	header := make([]byte, recordHeaderSize)

	var validOffset int64

	for {
		entry, bytesRead, done, loopErr := readNextEntry(file, header, validOffset, keys, obs)
		if loopErr != nil {
			return nil, loopErr
		}

		if done {
			break
		}

		entries = append(entries, entry)
		validOffset += bytesRead
	}

	return entries, nil
}

// readNextEntry reads a single WAL frame from file and returns the entry, number of bytes read,
// whether reading is done, and any error. On corrupt/incomplete frames it truncates to validOffset.
func readNextEntry(file *os.File, header []byte, validOffset int64, keys [][]byte, obs Observer) (Entry, int64, bool, error) {
	_, err := io.ReadFull(file, header)
	if err != nil {
		headerErr := handleHeaderReadErr(file, err, validOffset, obs)
		if headerErr != nil {
			return Entry{}, 0, false, headerErr
		}

		return Entry{}, 0, true, nil
	}

	payloadLength := binary.LittleEndian.Uint32(header[:4])
	expectedHMAC := header[4 : 4+recordHMACSize]

	if payloadLength == 0 {
		if truncateErr := truncateFileWithObs(file, validOffset, "zero_length_frame", obs); truncateErr != nil {
			return Entry{}, 0, false, truncateErr
		}

		return Entry{}, 0, true, nil
	}

	payload := make([]byte, int(payloadLength))

	if _, err := io.ReadFull(file, payload); err != nil {
		payloadErr := handlePayloadReadErr(file, err, validOffset, obs)
		if payloadErr != nil {
			return Entry{}, 0, false, payloadErr
		}

		return Entry{}, 0, true, nil
	}

	if !verifyFrameHMAC(keys, header[:4], payload, expectedHMAC) {
		if obs != nil {
			obs.ObserveWALHMACVerifyFailed(validOffset, "hmac_mismatch")
		}

		if truncateErr := truncateFileWithObs(file, validOffset, "hmac_mismatch", obs); truncateErr != nil {
			return Entry{}, 0, false, truncateErr
		}

		return Entry{}, 0, true, nil
	}

	var entry Entry
	if err := json.Unmarshal(payload, &entry); err != nil {
		if truncateErr := truncateFileWithObs(file, validOffset, "json_unmarshal_failed", obs); truncateErr != nil {
			return Entry{}, 0, false, truncateErr
		}

		return Entry{}, 0, true, nil
	}

	bytesRead := int64(recordHeaderSize) + int64(payloadLength)

	return entry, bytesRead, false, nil
}

// verifyFrameHMAC tries each key in order using constant-time comparison
// (hmac.Equal). Returns true on the first match. This enables zero-downtime
// key rotation: the writer signs with the current key, and the reader accepts
// the current key OR the previous key while both are deployed.
func verifyFrameHMAC(keys [][]byte, lenBytes, payload, expected []byte) bool {
	for _, key := range keys {
		if len(key) < MinHMACKeyLength {
			continue
		}

		if hmac.Equal(computeFrameHMAC(key, lenBytes, payload), expected) {
			return true
		}
	}

	return false
}

// handleHeaderReadErr handles errors from reading the WAL frame header.
// Returns nil when the caller should break, or a wrapped error to propagate.
func handleHeaderReadErr(file *os.File, err error, validOffset int64, obs Observer) error {
	if errors.Is(err, io.EOF) {
		return nil
	}

	if errors.Is(err, io.ErrUnexpectedEOF) {
		if truncateErr := truncateFileWithObs(file, validOffset, "partial_header", obs); truncateErr != nil {
			return truncateErr
		}

		return nil
	}

	return fmt.Errorf("read wal header: %w", err)
}

// handlePayloadReadErr handles errors from reading the WAL frame payload.
// Returns nil when the caller should break, or a wrapped error to propagate.
func handlePayloadReadErr(file *os.File, err error, validOffset int64, obs Observer) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		if truncateErr := truncateFileWithObs(file, validOffset, "partial_payload", obs); truncateErr != nil {
			return truncateErr
		}

		return nil
	}

	return fmt.Errorf("read wal payload: %w", err)
}

// truncateFileWithObs truncates file at offset and emits a security-observable
// audit event so the truncation does not go unnoticed.
func truncateFileWithObs(file *os.File, offset int64, reason string, obs Observer) error {
	if err := file.Truncate(offset); err != nil {
		return fmt.Errorf("truncate wal file at offset %d: %w", offset, err)
	}

	if obs != nil {
		obs.ObserveWALTruncation(offset, reason)
	}

	return nil
}
