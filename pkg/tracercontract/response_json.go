// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"unicode/utf8"
)

// DecodeReserveResultJSON validates a complete, bounded response before typed
// allocation. Duplicate fields and unknown data cannot conceal incomplete controls.
func DecodeReserveResultJSON(ctx context.Context, raw []byte, maxBodyBytes, maxReservations int) (*ReserveResult, error) {
	if maxReservations <= 0 {
		return nil, invalid("result reservation bound")
	}

	text := &reserveJSONShape{kind: 's', required: true}
	shape := &reserveJSONShape{kind: 'o', fields: map[string]*reserveJSONShape{
		"contractRevision": text, "transactionId": text, "evaluationId": text, "decision": text,
		"controls":       {kind: 'o', required: true, fields: map[string]*reserveJSONShape{"rules": text, "limits": text}},
		"reservationIds": {kind: 'l', required: true, element: text, maxElements: maxReservations},
		"reasons":        {kind: 'l', required: true, element: text, maxElements: 7},
	}}

	var result ReserveResult
	if err := decodeResponseJSON(ctx, raw, maxBodyBytes, shape, &result); err != nil {
		return nil, err
	}

	if err := result.Validate(maxReservations); err != nil {
		return nil, err
	}

	return &result, nil
}

// DecodeTransactionCompletionJSON distinguishes a missing movement count from
// an explicit zero and an absent evaluation from a malformed/null identifier.
func DecodeTransactionCompletionJSON(ctx context.Context, raw []byte, maxBodyBytes int) (*TransactionCompletionResult, error) {
	text := &reserveJSONShape{kind: 's', required: true}
	shape := &reserveJSONShape{kind: 'o', fields: map[string]*reserveJSONShape{
		"contractRevision": text, "transactionId": text, "status": text,
		"flipped": {kind: 'i', required: true}, "evaluationId": {kind: 's'},
	}}

	var result TransactionCompletionResult
	if err := decodeResponseJSON(ctx, raw, maxBodyBytes, shape, &result); err != nil {
		return nil, err
	}

	if err := result.Validate(); err != nil {
		return nil, err
	}

	return &result, nil
}

func decodeResponseJSON(ctx context.Context, raw []byte, maxBodyBytes int, shape *reserveJSONShape, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if maxBodyBytes <= 0 || len(raw) == 0 || len(raw) > maxBodyBytes || !utf8.Valid(raw) || !validJSONSurrogates(raw) {
		return invalid("response body")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	if err := shape.read(ctx, decoder, Limits{}); err != nil {
		return err
	}

	if _, err := decoder.Token(); err != io.EOF {
		return invalid("trailing response JSON")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := json.Unmarshal(raw, result); err != nil {
		return invalid("response JSON fields")
	}

	return nil
}
