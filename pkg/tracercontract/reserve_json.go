// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"
)

// DecodeReserveJSON enforces a closed, case-sensitive JSON shape before typed
// decoding. It preserves monetary text and boolean presence, rejects duplicate
// decoded keys and invalid Unicode, and bounds arrays before allocating facts.
// It does not authenticate a namespace, decide policy or apply freshness. The
// admission command must still call ReserveRequest.Validate/Fingerprint with
// the authenticated scope, including on replay.
func DecodeReserveJSON(ctx context.Context, raw []byte, maxBodyBytes int, limits Limits) (ReserveRequest, error) {
	if err := ctx.Err(); err != nil {
		return ReserveRequest{}, err
	}

	if maxBodyBytes <= 0 || len(raw) == 0 || len(raw) > maxBodyBytes {
		return ReserveRequest{}, invalid("reserve body size")
	}

	if err := limits.Validate(); err != nil {
		return ReserveRequest{}, err
	}

	if !utf8.Valid(raw) || !validJSONSurrogates(raw) {
		return ReserveRequest{}, invalid("reserve JSON text")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	if err := reserveWireShape.read(ctx, decoder, limits); err != nil {
		return ReserveRequest{}, err
	}

	if _, err := decoder.Token(); err != io.EOF {
		return ReserveRequest{}, invalid("trailing reserve JSON")
	}

	if err := ctx.Err(); err != nil {
		return ReserveRequest{}, err
	}

	var result ReserveRequest
	if err := json.Unmarshal(raw, &result); err != nil {
		return ReserveRequest{}, invalid("reserve JSON fields")
	}

	return result, nil
}

type reserveJSONShape struct {
	kind        byte
	required    bool
	maxElements int
	fields      map[string]*reserveJSONShape
	element     *reserveJSONShape
}

// Immutable private schema: every accepted path has bounded nesting, and only
// accounts/entries are arrays. Keep exact field spelling aligned with the DTO.
var reserveWireShape = newReserveWireShape()

func newReserveWireShape() *reserveJSONShape {
	text := &reserveJSONShape{kind: 's'}
	boolean := &reserveJSONShape{kind: 'b'}
	asset := &reserveJSONShape{kind: 'o', fields: map[string]*reserveJSONShape{"namespace": text, "id": text, "code": text}}
	account := &reserveJSONShape{kind: 'o', fields: map[string]*reserveJSONShape{"id": text, "type": text, "status": text, "blocked": boolean, "asset": asset}}
	entry := &reserveJSONShape{kind: 'o', fields: map[string]*reserveJSONShape{"accountId": text, "external": boolean, "direction": text, "amount": text, "asset": asset}}
	facts := &reserveJSONShape{kind: 'o', fields: map[string]*reserveJSONShape{"accounts": {kind: 'a', element: account}, "entries": {kind: 'e', element: entry}}}

	return &reserveJSONShape{kind: 'o', fields: map[string]*reserveJSONShape{
		"contractRevision": text, "transactionId": text, "requestId": text, "contextId": text,
		"validationMode": text, "transactionTimestamp": text, "longLived": boolean,
		"amount": text, "asset": asset, "context": facts,
	}}
}

func (s *reserveJSONShape) read(ctx context.Context, d *json.Decoder, limits Limits) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	token, err := d.Token()
	if err != nil {
		return invalid("reserve JSON syntax")
	}

	switch s.kind {
	case 's':
		if _, ok := token.(string); !ok {
			return invalid("reserve JSON string")
		}
	case 'i':
		number, ok := token.(json.Number)
		if !ok {
			return invalid("reserve JSON integer")
		}

		if _, err := number.Int64(); err != nil {
			return invalid("reserve JSON integer")
		}
	case 'b':
		if _, ok := token.(bool); !ok {
			return invalid("reserve JSON boolean")
		}
	case 'o':
		if token != json.Delim('{') {
			return invalid("reserve JSON object")
		}

		return s.readObject(ctx, d, limits)
	case 'a', 'e', 'l':
		if token != json.Delim('[') {
			return invalid("reserve JSON array")
		}

		bound := limits.MaxAccounts
		if s.kind == 'e' {
			bound = limits.MaxEntries
		}

		if s.kind == 'l' {
			bound = s.maxElements
		}

		return s.readArray(ctx, d, limits, bound)
	default:
		return invalid("reserve JSON schema")
	}

	return nil
}

func (s *reserveJSONShape) readObject(ctx context.Context, d *json.Decoder, limits Limits) error {
	seen := make(map[string]struct{}, len(s.fields))

	for d.More() {
		token, err := d.Token()
		if err != nil {
			return invalid("reserve JSON key")
		}

		key, ok := token.(string)
		if !ok {
			return invalid("reserve JSON key")
		}

		child, known := s.fields[key]
		if _, duplicate := seen[key]; !known || duplicate {
			return invalid("unknown or duplicate reserve field")
		}

		seen[key] = struct{}{}

		if err := child.read(ctx, d, limits); err != nil {
			return err
		}
	}

	for key, child := range s.fields {
		if _, present := seen[key]; child.required && !present {
			return invalid("missing reserve JSON field")
		}
	}

	token, err := d.Token()
	if err != nil || token != json.Delim('}') {
		return invalid("reserve JSON object end")
	}

	return nil
}

func (s *reserveJSONShape) readArray(ctx context.Context, d *json.Decoder, limits Limits, bound int) error {
	count := 0
	for d.More() {
		if count >= bound {
			return invalid("reserve JSON array size")
		}

		count++

		if err := s.element.read(ctx, d, limits); err != nil {
			return err
		}
	}

	token, err := d.Token()
	if err != nil || token != json.Delim(']') {
		return invalid("reserve JSON array end")
	}

	return nil
}

// encoding/json replaces unpaired UTF-16 surrogates with U+FFFD. Reject them
// before that lossy conversion, while accepting escaped literal backslashes and
// valid surrogate pairs. JSON syntax/escape validation remains with the decoder.
func validJSONSurrogates(raw []byte) bool {
	quoted := false

	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			quoted = !quoted
			continue
		}

		if !quoted || raw[i] != '\\' {
			continue
		}

		i++
		if i >= len(raw) {
			return false
		}

		if raw[i] != 'u' {
			continue
		}

		if len(raw)-i < 5 {
			return false
		}

		unit, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}

		i += 4

		if unit >= 0xDC00 && unit <= 0xDFFF {
			return false
		}

		if unit < 0xD800 || unit > 0xDBFF {
			continue
		}

		if !validLowSurrogate(raw, i) {
			return false
		}

		i += 6
	}

	return true
}

func validLowSurrogate(raw []byte, lastHigh int) bool {
	if len(raw)-lastHigh < 7 || raw[lastHigh+1] != '\\' || raw[lastHigh+2] != 'u' {
		return false
	}

	low, err := strconv.ParseUint(string(raw[lastHigh+3:lastHigh+7]), 16, 16)

	return err == nil && low >= 0xDC00 && low <= 0xDFFF
}

// DecodeCompletionJSON requires explicit acknowledgement of this contract.
// Empty legacy bodies are handled by the transport's separate legacy lifecycle.
func DecodeCompletionJSON(ctx context.Context, raw []byte, maxBodyBytes int) (CompletionRequest, error) {
	if err := ctx.Err(); err != nil {
		return CompletionRequest{}, err
	}

	if maxBodyBytes <= 0 || len(raw) == 0 || len(raw) > maxBodyBytes || !utf8.Valid(raw) || !validJSONSurrogates(raw) {
		return CompletionRequest{}, invalid("completion body")
	}

	shape := reserveJSONShape{kind: 'o', fields: map[string]*reserveJSONShape{"contractRevision": {kind: 's'}}}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := shape.read(ctx, decoder, Limits{}); err != nil {
		return CompletionRequest{}, err
	}

	if _, err := decoder.Token(); err != io.EOF {
		return CompletionRequest{}, invalid("trailing completion JSON")
	}

	var result CompletionRequest
	if err := json.Unmarshal(raw, &result); err != nil || result.ContractRevision != ReserveContractRevision {
		return CompletionRequest{}, invalid("completion revision")
	}

	return result, nil
}
