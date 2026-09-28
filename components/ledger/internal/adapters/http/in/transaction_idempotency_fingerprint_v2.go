// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

const v2IdempotencyRequestFingerprintDomain = "midaz.transaction.request.v2\x00"

// v2IdempotencyFingerprint identifies a singular v2 create request inside its
// idempotency slot, so a reused X-Idempotency key replays only the request that
// claimed it. It is canonical — whitespace and property order do not change it —
// so an honest retry that re-serializes the same body still replays. The action
// is folded in because the endpoint, not the body, carries it. The domain label
// keeps it apart from the atomic batch and /v1 fingerprints.
//
// It never names the slot: the key a request without X-Idempotency lands on
// stays derived from v2IdempotencyHashSource.
func v2IdempotencyFingerprint(rawBody []byte, pending bool, operationTypeOverride string) (string, error) {
	canonical, err := canonicalV2IdempotencyRequest(rawBody)
	if err != nil {
		return "", fmt.Errorf("canonicalize v2 idempotency request: %w", err)
	}

	digest := sha256.New()
	_, _ = digest.Write([]byte(v2IdempotencyRequestFingerprintDomain))
	_, _ = digest.Write([]byte(idempotencyActionDiscriminator(pending, operationTypeOverride)))
	_, _ = digest.Write([]byte(command.IdempotencyDiscriminatorSep))
	_, _ = digest.Write(canonical)

	return hex.EncodeToString(digest.Sum(nil)), nil
}

// canonicalV2IdempotencyRequest re-encodes the body with sorted object keys and
// no insignificant whitespace. UseNumber keeps every number literal intact, so
// two amounts float64 would round together stay distinct; array order is kept
// because leg order is part of the request.
func canonicalV2IdempotencyRequest(rawBody []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.UseNumber()

	var body any
	if err := decoder.Decode(&body); err != nil {
		return nil, err
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after the request body")
	}

	return json.Marshal(body)
}
