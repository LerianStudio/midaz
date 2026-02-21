// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// Replay reads WAL entries from disk.
func Replay(path string) ([]Entry, error) {
	if path == "" {
		return nil, fmt.Errorf("wal path cannot be empty")
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}
	defer file.Close()

	entries := make([]Entry, 0, 1024)
	header := make([]byte, recordHeaderSize)

	var validOffset int64

	for {
		_, err := io.ReadFull(file, header)
		if err != nil {
			if err == io.EOF {
				break
			}

			if err == io.ErrUnexpectedEOF {
				if truncateErr := file.Truncate(validOffset); truncateErr != nil {
					return nil, truncateErr
				}

				break
			}

			return nil, err
		}

		payloadLength := binary.LittleEndian.Uint32(header[:4])
		expectedChecksum := binary.LittleEndian.Uint32(header[4:8])

		if payloadLength == 0 {
			if truncateErr := file.Truncate(validOffset); truncateErr != nil {
				return nil, truncateErr
			}

			break
		}

		payload := make([]byte, int(payloadLength))
		if _, err := io.ReadFull(file, payload); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				if truncateErr := file.Truncate(validOffset); truncateErr != nil {
					return nil, truncateErr
				}

				break
			}

			return nil, err
		}

		if crc32.ChecksumIEEE(payload) != expectedChecksum {
			if truncateErr := file.Truncate(validOffset); truncateErr != nil {
				return nil, truncateErr
			}

			break
		}

		var entry Entry
		if err := json.Unmarshal(payload, &entry); err != nil {
			if truncateErr := file.Truncate(validOffset); truncateErr != nil {
				return nil, truncateErr
			}

			break
		}

		entries = append(entries, entry)
		validOffset += int64(recordHeaderSize) + int64(payloadLength)
	}

	return entries, nil
}
