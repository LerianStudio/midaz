// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sync"
	"time"

	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

const recordHeaderSize = 8

// Entry is a persisted authorization decision.
type Entry struct {
	TransactionID     string                           `json:"transactionId"`
	OrganizationID    string                           `json:"organizationId"`
	LedgerID          string                           `json:"ledgerId"`
	Pending           bool                             `json:"pending"`
	TransactionStatus string                           `json:"transactionStatus"`
	Operations        []*authorizerv1.BalanceOperation `json:"operations"`
	Mutations         []BalanceMutation                `json:"mutations,omitempty"`
	CreatedAt         time.Time                        `json:"createdAt"`
}

// BalanceMutation stores post-authorization balance state for WAL replay.
type BalanceMutation struct {
	AccountAlias    string `json:"accountAlias"`
	BalanceKey      string `json:"balanceKey"`
	Available       int64  `json:"available"`
	OnHold          int64  `json:"onHold"`
	PreviousVersion uint64 `json:"previousVersion"`
	NextVersion     uint64 `json:"nextVersion"`
}

// Writer persists authorization entries.
type Writer interface {
	Append(entry Entry) error
	Close() error
}

type noopWriter struct{}

// NewNoopWriter returns a writer that drops all entries.
func NewNoopWriter() Writer {
	return noopWriter{}
}

func (noopWriter) Append(_ Entry) error {
	return nil
}

func (noopWriter) Close() error {
	return nil
}

// RingBufferWriter batches WAL appends in-memory and flushes to disk periodically.
type RingBufferWriter struct {
	file   *os.File
	writer *bufio.Writer
	flush  *time.Ticker

	entries chan Entry
	stop    chan struct{}
	done    chan struct{}

	mu sync.Mutex
}

// NewRingBufferWriter creates a file-backed WAL writer.
func NewRingBufferWriter(path string, bufferSize int, flushInterval time.Duration) (*RingBufferWriter, error) {
	if path == "" {
		return nil, fmt.Errorf("wal path cannot be empty")
	}

	if bufferSize <= 0 {
		bufferSize = 65536
	}

	if flushInterval <= 0 {
		flushInterval = time.Millisecond
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create wal directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open wal file: %w", err)
	}

	r := &RingBufferWriter{
		file:    file,
		writer:  bufio.NewWriterSize(file, 1<<20),
		flush:   time.NewTicker(flushInterval),
		entries: make(chan Entry, bufferSize),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}

	go r.run()

	return r, nil
}

func (r *RingBufferWriter) Append(entry Entry) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	select {
	case r.entries <- entry:
		return nil
	default:
		return fmt.Errorf("wal buffer is full")
	}
}

func (r *RingBufferWriter) Close() error {
	close(r.stop)
	<-r.done

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.flush != nil {
		r.flush.Stop()
	}

	if r.writer != nil {
		if err := r.writer.Flush(); err != nil {
			return err
		}
	}

	if r.file != nil {
		if err := r.file.Sync(); err != nil {
			return err
		}

		return r.file.Close()
	}

	return nil
}

func (r *RingBufferWriter) run() {
	defer close(r.done)

	for {
		select {
		case <-r.stop:
			r.drain()
			return
		case entry := <-r.entries:
			r.writeEntry(entry)
		case <-r.flush.C:
			r.flushNow()
		}
	}
}

func (r *RingBufferWriter) drain() {
	for {
		select {
		case entry := <-r.entries:
			r.writeEntry(entry)
		default:
			r.flushNow()
			return
		}
	}
}

func (r *RingBufferWriter) writeEntry(entry Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	frame, err := encodeEntryFrame(entry)
	if err != nil {
		return
	}

	_, _ = r.writer.Write(frame)
}

func (r *RingBufferWriter) flushNow() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.writer == nil || r.file == nil {
		return
	}

	_ = r.writer.Flush()
	_ = r.file.Sync()
}

func encodeEntryFrame(entry Entry) ([]byte, error) {
	payload, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}

	frame := make([]byte, recordHeaderSize+len(payload))
	binary.LittleEndian.PutUint32(frame[:4], uint32(len(payload)))
	binary.LittleEndian.PutUint32(frame[4:8], crc32.ChecksumIEEE(payload))
	copy(frame[recordHeaderSize:], payload)

	return frame, nil
}
