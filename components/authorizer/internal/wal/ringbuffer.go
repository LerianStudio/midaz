// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

const (
	// Record frame layout (little-endian):
	//   offset  0..4   — payload length (uint32)
	//   offset  4..36  — HMAC-SHA256(key, lenBytes || payload) (32 bytes)
	//   offset 36..40  — reserved, zero-filled (reserved for future frame flags / seq)
	//   offset 40..    — payload (JSON-encoded Entry)
	//
	// The HMAC covers the length prefix so truncation that changes the
	// declared payload size is detected on replay.
	recordHMACSize     = sha256.Size
	recordReservedSize = 4
	recordHeaderSize   = 4 + recordHMACSize + recordReservedSize

	// MinHMACKeyLength is the minimum acceptable length (in bytes) for the WAL
	// HMAC key. 32 bytes = 256 bits, matching the block size of SHA-256.
	MinHMACKeyLength = 32

	// walDirPerm is the permission mode for the WAL directory.
	walDirPerm = 0o700

	// walFilePerm is the permission mode for the WAL file.
	walFilePerm = 0o600

	// walWriterBufShift is the bit shift for the buffered writer size (1<<20 = 1 MiB).
	walWriterBufShift = 20
)

// WALParticipant identifies an authorizer instance that participated in a cross-shard transaction.
type WALParticipant struct {
	InstanceAddr string `json:"instanceAddr"`
	PreparedTxID string `json:"preparedTxId"`
	IsLocal      bool   `json:"isLocal"`
}

// Entry is a persisted authorization decision.
//
// JSON omitempty convention: core fields that existed in the original WAL schema
// (TransactionID through CreatedAt, plus Pending) never use omitempty so their
// zero values are always serialized. Extension fields added later (Mutations,
// CrossShard, Participants) use omitempty so entries written before those fields
// existed remain byte-compatible when round-tripped through marshal/unmarshal --
// the keys are simply absent rather than present with zero values.
//
// LEGACY FIELDS (unread on replay): Pending, TransactionStatus and Operations
// are persisted for historical compatibility and external offline tooling but
// are NOT consumed by Engine.ReplayEntries — which only reconstructs balance
// state from Mutations. Do not rely on them for in-process recovery; they are
// kept in the schema solely so older WAL files remain parseable.
type Entry struct {
	TransactionID     string                           `json:"transactionId"`
	OrganizationID    string                           `json:"organizationId"`
	LedgerID          string                           `json:"ledgerId"`
	Pending           bool                             `json:"pending"`           // legacy: unread on replay
	TransactionStatus string                           `json:"transactionStatus"` // legacy: unread on replay
	Operations        []*authorizerv1.BalanceOperation `json:"operations"`        // legacy: unread on replay
	Mutations         []BalanceMutation                `json:"mutations,omitempty"`
	CrossShard        bool                             `json:"crossShard,omitempty"`
	Participants      []WALParticipant                 `json:"participants,omitempty"`
	CreatedAt         time.Time                        `json:"createdAt"`

	// PreparedIntent, when non-nil, marks this entry as a prepared-but-not-yet-
	// committed 2PC intent (D1 audit finding #2). Replay distinguishes prepared
	// intents from committed mutations by the presence of this field: Mutations
	// is nil on prepared intents and set on commits. When replay encounters a
	// prepared intent without a matching subsequent committed entry, the
	// authorizer re-issues PrepareAuthorize to rebuild the in-memory lock and
	// the prepStore map entry — closing the double-spend window where a
	// coordinator's post-restart CommitPrepared would otherwise find nothing.
	PreparedIntent *PreparedIntent `json:"preparedIntent,omitempty"`
}

// PreparedIntent records the minimum state needed to re-prepare a transaction
// after an authorizer restart. The embedded AuthorizeRequest is the original
// client payload (proto-serialized JSON via protojson) so the replay path can
// pass it verbatim to PrepareAuthorize and obtain an identical set of locks
// and validation results as the pre-crash prepare did.
type PreparedIntent struct {
	// PreparedTxID is the engine-assigned identifier returned by the original
	// PrepareAuthorize call. Coordinators use it to call CommitPrepared /
	// AbortPrepared; replay MUST restore the same ID so post-restart RPCs
	// from the coordinator find the re-prepared entry.
	PreparedTxID string `json:"preparedTxId"`

	// WorkerIDs lists the shard router worker IDs that hold locks for this
	// prepared transaction. Stored for operational visibility — replay does
	// not consume this directly because re-prepare deterministically
	// re-resolves the same workers via the router.
	WorkerIDs []int `json:"workerIds,omitempty"`

	// LockedBalanceKeys lists the lookup keys of every balance held under
	// the prepared transaction's locks (organizationID:ledgerID:alias:key).
	// Operational / forensic metadata; not consumed on replay.
	LockedBalanceKeys []string `json:"lockedBalanceKeys,omitempty"`

	// RequestJSON is the protojson-serialized AuthorizeRequest that produced
	// this prepare. Replay decodes it back into an AuthorizeRequest and
	// re-invokes PrepareAuthorize.
	RequestJSON []byte `json:"requestJson,omitempty"`

	// PreparedAt is the wall-clock time of the original prepare. Replay uses
	// this to detect prepared intents that are already past the prepare
	// timeout — those should be auto-aborted rather than restored.
	PreparedAt time.Time `json:"preparedAt"`
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

// Observer receives WAL operational metrics.
type Observer interface {
	ObserveWALQueueDepth(depth int)
	ObserveWALAppendDropped(err error)
	ObserveWALWriteError(stage string, err error)
	ObserveWALFsyncLatency(latency time.Duration)
	// ObserveWALHMACVerifyFailed is emitted when a WAL frame fails HMAC
	// verification on replay. This typically indicates tampering, key
	// rotation without the previous key configured, or on-disk corruption.
	ObserveWALHMACVerifyFailed(offset int64, reason string)
	// ObserveWALTruncation is emitted when replay truncates trailing bytes of
	// the WAL file (corruption, partial write, HMAC mismatch). offset is the
	// byte offset at which the file was truncated; reason is a short label
	// identifying why truncation was performed.
	ObserveWALTruncation(offset int64, reason string)
}

type noopWriter struct{}

// NewNoopWriter returns a writer that drops all entries.
func NewNoopWriter() Writer {
	return noopWriter{}
}

// Append is a no-op implementation that discards the entry.
func (noopWriter) Append(_ Entry) error {
	return nil
}

// Close is a no-op implementation that always returns nil.
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
	hmacKey      []byte

	entries chan Entry
	stop    chan struct{}
	done    chan struct{}

	mu      sync.Mutex
	errMu   sync.RWMutex
	lastErr error
}

// NewRingBufferWriter creates a file-backed WAL writer.
// The hmacKey must be at least MinHMACKeyLength bytes; shorter keys are rejected.
func NewRingBufferWriter(path string, bufferSize int, flushInterval time.Duration, hmacKey []byte) (*RingBufferWriter, error) {
	return NewRingBufferWriterWithOptions(path, bufferSize, flushInterval, false, nil, hmacKey)
}

// NewRingBufferWriterWithObserver creates a file-backed WAL writer with observability hooks.
func NewRingBufferWriterWithObserver(path string, bufferSize int, flushInterval time.Duration, observer Observer, hmacKey []byte) (*RingBufferWriter, error) {
	return NewRingBufferWriterWithOptions(path, bufferSize, flushInterval, false, observer, hmacKey)
}

// NewRingBufferWriterWithOptions creates a file-backed WAL writer with optional synchronous append semantics.
// The hmacKey is used to sign every appended frame with HMAC-SHA256; it MUST be >= MinHMACKeyLength.
func NewRingBufferWriterWithOptions(
	path string,
	bufferSize int,
	flushInterval time.Duration,
	syncOnAppend bool,
	observer Observer,
	hmacKey []byte,
) (*RingBufferWriter, error) {
	if path == "" {
		return nil, constant.ErrWALPathEmpty
	}

	if len(hmacKey) < MinHMACKeyLength {
		return nil, fmt.Errorf("hmac key length=%d minimum=%d: %w", len(hmacKey), MinHMACKeyLength, constant.ErrWALHMACKeyRequired)
	}

	if bufferSize <= 0 {
		bufferSize = 65536
	}

	if flushInterval <= 0 {
		flushInterval = time.Millisecond
	}

	if err := os.MkdirAll(filepath.Dir(path), walDirPerm); err != nil {
		return nil, fmt.Errorf("create wal directory: %w", err)
	}

	// O_NOFOLLOW (POSIX): refuse to open the WAL file if it is a symlink.
	// Prevents an attacker with filesystem access from swapping the WAL path
	// for a symlink pointing at an arbitrary victim file.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY|openNoFollowFlag(), walFilePerm)
	if err != nil {
		return nil, fmt.Errorf("open wal file: %w", err)
	}

	// Defensive copy of the key so the caller cannot mutate signing material
	// after construction.
	keyCopy := make([]byte, len(hmacKey))
	copy(keyCopy, hmacKey)

	r := &RingBufferWriter{
		file:         file,
		writer:       bufio.NewWriterSize(file, 1<<walWriterBufShift),
		flush:        time.NewTicker(flushInterval),
		obs:          observer,
		syncOnAppend: syncOnAppend,
		hmacKey:      keyCopy,
		entries:      make(chan Entry, bufferSize),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}

	go r.run()

	return r, nil
}

// Append enqueues a WAL entry for persistence.
func (r *RingBufferWriter) Append(entry Entry) error {
	if err := r.getError(); err != nil {
		return err
	}

	select {
	case <-r.stop:
		return constant.ErrWALWriterClosing
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
		r.observeQueueDepth(len(r.entries))
		r.observeAppendDropped(constant.ErrWALBufferFull)

		return constant.ErrWALBufferFull
	}
}

func (r *RingBufferWriter) appendSync(entry Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.writer == nil || r.file == nil {
		return constant.ErrWALWriterNotAvailable
	}

	frame, err := encodeEntryFrame(entry, r.hmacKey)
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

// Close drains remaining entries and closes the WAL file.
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

			return fmt.Errorf("flush wal writer on close: %w", err)
		}
	}

	if r.file != nil {
		syncStart := time.Now()

		if err := r.file.Sync(); err != nil {
			r.observeWriteError("close_sync", err)
			r.setError(fmt.Errorf("sync wal file on close: %w", err))

			return fmt.Errorf("sync wal file on close: %w", err)
		}

		r.observeFsyncLatency(time.Since(syncStart))

		if err := r.file.Close(); err != nil {
			r.observeWriteError("close_file", err)
			r.setError(fmt.Errorf("close wal file: %w", err))

			return fmt.Errorf("close wal file: %w", err)
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

	frame, err := encodeEntryFrame(entry, r.hmacKey)
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

// computeFrameHMAC returns HMAC-SHA256(key, lenBytes || payload).
// Including the length prefix in the MAC input binds the declared payload
// size to the payload bytes, so a tampered header that lies about length
// cannot pass verification even if the payload is re-signed separately.
func computeFrameHMAC(key, lenBytes, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(lenBytes)
	mac.Write(payload)

	return mac.Sum(nil)
}

// encodeEntryFrame serializes entry as a length-prefixed frame authenticated
// by HMAC-SHA256. The reserved trailing 4 bytes of the header are zero-filled;
// they are reserved for a future monotonic sequence number or frame flags and
// are covered neither by the HMAC nor by any current reader (they MUST stay
// zero so older readers that ignore them continue to verify correctly if the
// layout is extended later).
func encodeEntryFrame(entry Entry, hmacKey []byte) ([]byte, error) {
	if len(hmacKey) < MinHMACKeyLength {
		return nil, fmt.Errorf("hmac key length=%d minimum=%d: %w", len(hmacKey), MinHMACKeyLength, constant.ErrWALHMACKeyRequired)
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("marshal wal entry: %w", err)
	}

	if len(payload) > math.MaxUint32 {
		return nil, fmt.Errorf("%w: %d bytes exceeds uint32 max", constant.ErrWALPayloadTooLarge, len(payload))
	}

	frame := make([]byte, recordHeaderSize+len(payload))
	binary.LittleEndian.PutUint32(frame[:4], uint32(len(payload))) //nolint:gosec // length validated above against MaxUint32

	mac := computeFrameHMAC(hmacKey, frame[:4], payload)
	copy(frame[4:4+recordHMACSize], mac)
	// frame[4+recordHMACSize : recordHeaderSize] is the reserved zero-filled region.
	copy(frame[recordHeaderSize:], payload)

	return frame, nil
}
