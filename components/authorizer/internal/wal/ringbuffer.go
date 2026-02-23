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

type Observer interface {
	ObserveWALQueueDepth(depth int)
	ObserveWALAppendDropped(err error)
	ObserveWALWriteError(stage string, err error)
	ObserveWALFsyncLatency(latency time.Duration)
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
	file         *os.File
	writer       *bufio.Writer
	flush        *time.Ticker
	obs          Observer
	syncOnAppend bool

	entries chan Entry
	stop    chan struct{}
	done    chan struct{}

	mu      sync.Mutex
	errMu   sync.RWMutex
	lastErr error
}

// NewRingBufferWriter creates a file-backed WAL writer.
func NewRingBufferWriter(path string, bufferSize int, flushInterval time.Duration) (*RingBufferWriter, error) {
	return NewRingBufferWriterWithOptions(path, bufferSize, flushInterval, false, nil)
}

// NewRingBufferWriterWithObserver creates a file-backed WAL writer with observability hooks.
func NewRingBufferWriterWithObserver(path string, bufferSize int, flushInterval time.Duration, observer Observer) (*RingBufferWriter, error) {
	return NewRingBufferWriterWithOptions(path, bufferSize, flushInterval, false, observer)
}

// NewRingBufferWriterWithOptions creates a file-backed WAL writer with optional synchronous append semantics.
func NewRingBufferWriterWithOptions(
	path string,
	bufferSize int,
	flushInterval time.Duration,
	syncOnAppend bool,
	observer Observer,
) (*RingBufferWriter, error) {
	if path == "" {
		return nil, fmt.Errorf("wal path cannot be empty")
	}

	if bufferSize <= 0 {
		bufferSize = 65536
	}

	if flushInterval <= 0 {
		flushInterval = time.Millisecond
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create wal directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open wal file: %w", err)
	}

	r := &RingBufferWriter{
		file:         file,
		writer:       bufio.NewWriterSize(file, 1<<20),
		flush:        time.NewTicker(flushInterval),
		obs:          observer,
		syncOnAppend: syncOnAppend,
		entries:      make(chan Entry, bufferSize),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}

	go r.run()

	return r, nil
}

func (r *RingBufferWriter) Append(entry Entry) error {
	if err := r.getError(); err != nil {
		return err
	}

	select {
	case <-r.stop:
		return fmt.Errorf("wal writer is closing")
	default:
	}

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	if r.syncOnAppend {
		return r.appendSync(entry)
	}

	select {
	case r.entries <- entry:
		r.observeQueueDepth(len(r.entries))
		return nil
	default:
		err := fmt.Errorf("wal buffer is full")
		r.observeQueueDepth(len(r.entries))
		r.observeAppendDropped(err)

		return err
	}
}

func (r *RingBufferWriter) appendSync(entry Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.writer == nil || r.file == nil {
		return fmt.Errorf("wal writer is not available")
	}

	frame, err := encodeEntryFrame(entry)
	if err != nil {
		r.observeWriteError("encode", err)
		err = fmt.Errorf("encode wal entry: %w", err)
		r.setError(err)

		return err
	}

	if _, err := r.writer.Write(frame); err != nil {
		r.observeWriteError("write", err)
		err = fmt.Errorf("write wal frame: %w", err)
		r.setError(err)

		return err
	}

	flushStart := time.Now()
	if err := r.writer.Flush(); err != nil {
		r.observeWriteError("flush", err)
		err = fmt.Errorf("flush wal writer: %w", err)
		r.setError(err)

		return err
	}

	if err := r.file.Sync(); err != nil {
		r.observeWriteError("sync", err)
		err = fmt.Errorf("sync wal file: %w", err)
		r.setError(err)

		return err
	}

	r.observeFsyncLatency(time.Since(flushStart))
	r.observeQueueDepth(len(r.entries))

	return nil
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
			r.observeWriteError("close_flush", err)
			r.setError(fmt.Errorf("flush wal writer on close: %w", err))

			return err
		}
	}

	if r.file != nil {
		syncStart := time.Now()
		if err := r.file.Sync(); err != nil {
			r.observeWriteError("close_sync", err)
			r.setError(fmt.Errorf("sync wal file on close: %w", err))

			return err
		}
		r.observeFsyncLatency(time.Since(syncStart))

		if err := r.file.Close(); err != nil {
			r.observeWriteError("close_file", err)
			r.setError(fmt.Errorf("close wal file: %w", err))

			return err
		}
	}

	return r.getError()
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
			r.observeQueueDepth(len(r.entries))
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
		r.observeWriteError("encode", err)
		r.setError(fmt.Errorf("encode wal entry: %w", err))

		return
	}

	if _, err := r.writer.Write(frame); err != nil {
		r.observeWriteError("write", err)
		r.setError(fmt.Errorf("write wal frame: %w", err))
	}
}

func (r *RingBufferWriter) flushNow() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.writer == nil || r.file == nil {
		return
	}

	flushStart := time.Now()
	if err := r.writer.Flush(); err != nil {
		r.observeWriteError("flush", err)
		r.setError(fmt.Errorf("flush wal writer: %w", err))

		return
	}

	if err := r.file.Sync(); err != nil {
		r.observeWriteError("sync", err)
		r.setError(fmt.Errorf("sync wal file: %w", err))

		return
	}

	r.observeFsyncLatency(time.Since(flushStart))
}

func (r *RingBufferWriter) setError(err error) {
	if err == nil {
		return
	}

	r.errMu.Lock()
	defer r.errMu.Unlock()

	if r.lastErr != nil {
		return
	}

	r.lastErr = err
}

func (r *RingBufferWriter) getError() error {
	r.errMu.RLock()
	defer r.errMu.RUnlock()

	if r.lastErr == nil {
		return nil
	}

	return r.lastErr
}

func (r *RingBufferWriter) observeQueueDepth(depth int) {
	if r == nil || r.obs == nil {
		return
	}

	r.obs.ObserveWALQueueDepth(depth)
}

func (r *RingBufferWriter) observeAppendDropped(err error) {
	if r == nil || r.obs == nil {
		return
	}

	r.obs.ObserveWALAppendDropped(err)
}

func (r *RingBufferWriter) observeWriteError(stage string, err error) {
	if r == nil || r.obs == nil {
		return
	}

	r.obs.ObserveWALWriteError(stage, err)
}

func (r *RingBufferWriter) observeFsyncLatency(latency time.Duration) {
	if r == nil || r.obs == nil {
		return
	}

	r.obs.ObserveWALFsyncLatency(latency)
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
