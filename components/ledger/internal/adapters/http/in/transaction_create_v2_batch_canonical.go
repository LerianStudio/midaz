// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const atomicTransactionBatchV2RequestFingerprintDomain = "midaz.atomic-transaction-batch.request.v1\x00"

// canonicalizeAtomicTransactionBatchV2Request removes JSON whitespace and
// object-property ordering from a request's identity while retaining the exact
// sequence of every array and the lexical value of JSON numbers. The caller
// runs the strict typed batch decoder first, so this function never admits an
// unknown field or a shape the public contract rejects.
func canonicalizeAtomicTransactionBatchV2Request(rawBody []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.UseNumber()

	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}

	return json.Marshal(document)
}

// fingerprintAtomicTransactionBatchV2Request binds the canonical request to a
// versioned batch-only domain. It returns lowercase SHA-256 hex so idempotency
// records can validate and compare it without carrying request contents.
func fingerprintAtomicTransactionBatchV2Request(canonicalRequest []byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(atomicTransactionBatchV2RequestFingerprintDomain))
	_, _ = digest.Write(canonicalRequest)

	return hex.EncodeToString(digest.Sum(nil))
}
