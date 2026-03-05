// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

const replayInitialCap = 1024

// Replay reads WAL entries from disk.
func Replay(path string) ([]Entry, error) {
	if path == "" {
		return nil, constant.ErrWALPathEmpty
	}

	// NOTE: O_RDWR is intentional — Replay performs self-healing truncation
	// of corrupt/incomplete trailing frames, which requires write access.
	file, err := os.OpenFile(path, os.O_RDWR, 0)
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
		entry, bytesRead, done, loopErr := readNextEntry(file, header, validOffset)
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
func readNextEntry(file *os.File, header []byte, validOffset int64) (Entry, int64, bool, error) {
	_, err := io.ReadFull(file, header)
	if err != nil {
		headerErr := handleHeaderReadErr(file, err, validOffset)
		if headerErr != nil {
			return Entry{}, 0, false, headerErr
		}

		return Entry{}, 0, true, nil
	}

	payloadLength := binary.LittleEndian.Uint32(header[:4])
	expectedChecksum := binary.LittleEndian.Uint32(header[4:8])

	if payloadLength == 0 {
		if truncateErr := truncateFile(file, validOffset); truncateErr != nil {
			return Entry{}, 0, false, truncateErr
		}

		return Entry{}, 0, true, nil
	}

	payload := make([]byte, int(payloadLength))

	if _, err := io.ReadFull(file, payload); err != nil {
		payloadErr := handlePayloadReadErr(file, err, validOffset)
		if payloadErr != nil {
			return Entry{}, 0, false, payloadErr
		}

		return Entry{}, 0, true, nil
	}

	if crc32.ChecksumIEEE(payload) != expectedChecksum {
		if truncateErr := truncateFile(file, validOffset); truncateErr != nil {
			return Entry{}, 0, false, truncateErr
		}

		return Entry{}, 0, true, nil
	}

	var entry Entry
	if err := json.Unmarshal(payload, &entry); err != nil {
		if truncateErr := truncateFile(file, validOffset); truncateErr != nil {
			return Entry{}, 0, false, truncateErr
		}

		return Entry{}, 0, true, nil
	}

	bytesRead := int64(recordHeaderSize) + int64(payloadLength)

	return entry, bytesRead, false, nil
}

// handleHeaderReadErr handles errors from reading the WAL frame header.
// Returns nil when the caller should break, or a wrapped error to propagate.
func handleHeaderReadErr(file *os.File, err error, validOffset int64) error {
	if errors.Is(err, io.EOF) {
		return nil
	}

	if errors.Is(err, io.ErrUnexpectedEOF) {
		if truncateErr := truncateFile(file, validOffset); truncateErr != nil {
			return truncateErr
		}

		return nil
	}

	return fmt.Errorf("read wal header: %w", err)
}

// handlePayloadReadErr handles errors from reading the WAL frame payload.
// Returns nil when the caller should break, or a wrapped error to propagate.
func handlePayloadReadErr(file *os.File, err error, validOffset int64) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		if truncateErr := truncateFile(file, validOffset); truncateErr != nil {
			return truncateErr
		}

		return nil
	}

	return fmt.Errorf("read wal payload: %w", err)
}

// truncateFile wraps os.File.Truncate with error context.
func truncateFile(file *os.File, offset int64) error {
	if err := file.Truncate(offset); err != nil {
		return fmt.Errorf("truncate wal file at offset %d: %w", offset, err)
	}

	return nil
}
