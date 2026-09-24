// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"time"
)

const reserveFingerprintDomain = "tracer/context-reserve/1"

// Fingerprint hashes validated facts and resolved identity for durable replay.
// It is a content digest, not an authentication signature. Framing is uint32
// big-endian byte lengths/counts, UTF-8 strings, raw UUID bytes and 0/1 booleans.
// Array order/multiplicity remain observable to CEL and are never sorted here.
// Decimals and timestamps normalize exactly; policy revisions, execution
// deadlines and resource configuration are deliberately not part of identity.
// Callers must not mutate the request concurrently while it is being read.
func (r ReserveRequest) Fingerprint(ctx context.Context, scope ReserveScope, limits Limits) ([sha256.Size]byte, error) {
	if err := ctx.Err(); err != nil {
		return [sha256.Size]byte{}, err
	}

	if err := scope.validate(limits); err != nil {
		return [sha256.Size]byte{}, err
	}

	if err := r.Validate(ctx, scope.AssetNamespace, limits); err != nil {
		return [sha256.Size]byte{}, err
	}

	w := reserveHashWriter{hash: sha256.New()}
	w.text(reserveFingerprintDomain)
	w.text(scope.TenantID)
	w.text(scope.IntegrationID)
	w.text(scope.AssetNamespace)
	w.text(r.ContractRevision)
	w.bytes(r.TransactionID[:])
	w.bytes(r.RequestID[:])
	w.text(r.ContextID)
	w.text(string(r.ValidationMode))
	w.text(r.TransactionTimestamp.UTC().Format(time.RFC3339Nano))
	w.boolean(*r.LongLived)

	if err := w.amount(ctx, r.Amount, limits); err != nil {
		return [sha256.Size]byte{}, err
	}

	w.asset(r.Asset)
	w.count(len(r.Context.Accounts))

	for _, account := range r.Context.Accounts {
		if err := ctx.Err(); err != nil {
			return [sha256.Size]byte{}, err
		}

		w.bytes(account.ID[:])
		w.text(account.Type)
		w.text(account.Status)
		w.boolean(*account.Blocked)
		w.asset(account.Asset)
	}

	w.count(len(r.Context.Entries))

	for _, entry := range r.Context.Entries {
		w.boolean(entry.External)

		if !entry.External {
			w.bytes(entry.AccountID[:])
		}

		w.text(string(entry.Direction))

		if err := w.amount(ctx, entry.Amount, limits); err != nil {
			return [sha256.Size]byte{}, err
		}

		w.asset(entry.Asset)
	}

	if err := ctx.Err(); err != nil {
		return [sha256.Size]byte{}, err
	}

	var result [sha256.Size]byte
	w.hash.Sum(result[:0])

	return result, nil
}

// Stream directly into SHA-256 rather than retaining a second full envelope.
// Only validated values enter this writer; every length fits uint32.
type reserveHashWriter struct {
	hash hash.Hash
}

func (w reserveHashWriter) bytes(value []byte) {
	// hash.Hash.Write is documented to never return an error.
	_, _ = w.hash.Write(value)
}

func (w reserveHashWriter) count(value int) {
	var prefix [4]byte
	// #nosec G115 -- lengths are bounded by validateReserveLimits and Validate.
	binary.BigEndian.PutUint32(prefix[:], uint32(value))
	w.bytes(prefix[:])
}

func (w reserveHashWriter) text(value string) {
	w.count(len(value))
	w.bytes([]byte(value))
}

func (w reserveHashWriter) boolean(value bool) {
	var encoded byte
	if value {
		encoded = 1
	}

	w.bytes([]byte{encoded})
}

func (w reserveHashWriter) asset(value AssetRef) {
	w.text(value.Namespace)
	w.text(value.ID)
	w.text(value.Code)
}

func (w reserveHashWriter) amount(ctx context.Context, value Amount, limits Limits) error {
	amount, err := value.Decimal(ctx, limits)
	if err != nil {
		return err
	}

	w.text(amount.String())

	return nil
}
