// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const revisedAtomicTransactionBatchV2RequestFingerprintDomain = "midaz.atomic-transaction-batch.request.v2\x00"

// canonicalizeRevisedAtomicTransactionBatchV2Request builds the revised batch
// identity after the strict revised decoder accepted the request. It sorts only
// the outer transaction list by its validated explicit order. Each item's raw
// JSON is decoded with UseNumber so metadata numbers and every nested array keep
// their semantic identity.
func canonicalizeRevisedAtomicTransactionBatchV2Request(
	rawBody []byte,
	orderedItems []revisedDecodedAtomicTransactionBatchV2Item,
) ([]byte, error) {
	var wrapper struct {
		Transactions []json.RawMessage `json:"transactions"`
	}
	if err := json.Unmarshal(rawBody, &wrapper); err != nil {
		return nil, err
	}
	if len(wrapper.Transactions) != len(orderedItems) {
		return nil, fmt.Errorf("transaction count does not match validated items")
	}

	transactions := make([]any, len(orderedItems))
	for executionIndex, item := range orderedItems {
		if item.originalIndex < 0 || item.originalIndex >= len(wrapper.Transactions) {
			return nil, fmt.Errorf("validated transaction has invalid original index %d", item.originalIndex)
		}

		decoder := json.NewDecoder(bytes.NewReader(wrapper.Transactions[item.originalIndex]))
		decoder.UseNumber()
		if err := decoder.Decode(&transactions[executionIndex]); err != nil {
			return nil, fmt.Errorf("decode transaction at original index %d: %w", item.originalIndex, err)
		}
	}

	return json.Marshal(map[string]any{"transactions": transactions})
}

// fingerprintRevisedAtomicTransactionBatchV2Request keeps the revised wire
// contract separate from the original array-order-based contract. The explicit
// idempotency namespace remains unchanged; this domain only prevents the two
// canonicalization rules from sharing an identity by accident.
func fingerprintRevisedAtomicTransactionBatchV2Request(canonicalRequest []byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(revisedAtomicTransactionBatchV2RequestFingerprintDomain))
	_, _ = digest.Write(canonicalRequest)

	return hex.EncodeToString(digest.Sum(nil))
}
