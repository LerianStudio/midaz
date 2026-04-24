// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package canonicaljson produces a stable, whitespace-free JSON encoding of arbitrary Go
// values and derives a lowercase-hex SHA-256 hash over that encoding. The output is
// suitable for request fingerprinting (for example, to detect replay vs conflict on an
// idempotency key): two values that are semantically equal produce the same bytes and
// therefore the same hash, regardless of Go map iteration order or original JSON
// whitespace.
//
// Canonicalization rules (JCS-style, subset sufficient for Midaz payloads):
//
//   - Object keys are emitted in lexicographic byte order.
//   - No insignificant whitespace between tokens.
//   - Numbers are emitted via Go's default json.Marshal (IEEE-754 float64 surface).
//   - Strings are emitted via Go's default json.Marshal escape rules.
//   - nil Go maps and nil slices encode the same as a JSON null, matching
//     encoding/json behavior. Empty (non-nil) maps/slices encode as "{}"/"[]"
//     respectively; they are NOT treated as null.
//
// The encoder works by round-tripping the input through encoding/json so struct tags,
// MarshalJSON implementations, and json.RawMessage are honored. The resulting any-typed
// tree is then walked and re-emitted with sorted keys. This is deliberately simple:
// the goal is a stable fingerprint for request bodies that already round-trip through
// encoding/json in the HTTP layer.
package canonicaljson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// Canonicalize returns the canonical JSON byte encoding of v.
//
// The value is first marshaled with encoding/json (to honor custom MarshalJSON
// implementations and struct tags), then re-encoded with sorted object keys and no
// insignificant whitespace. Two inputs that differ only in map-key order or JSON
// whitespace produce identical output.
func Canonicalize(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonicaljson: initial marshal failed: %w", err)
	}

	// Handle the top-level "null" fast-path. json.Unmarshal into *any decodes it as nil,
	// and encodeValue emits nil as "null" — but skipping the decode saves an allocation
	// and makes the behavior obvious.
	if bytes.Equal(raw, []byte("null")) {
		return []byte("null"), nil
	}

	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("canonicaljson: decode for canonicalization failed: %w", err)
	}

	var buf bytes.Buffer
	if err := encodeValue(&buf, tree); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Hash returns the lowercase hex SHA-256 of Canonicalize(v).
//
// Intended for request fingerprinting: use together with an idempotency key to detect
// whether a replayed request carries the same body (safe replay) or a different body
// (conflict to reject).
func Hash(v any) (string, error) {
	canonical, err := Canonicalize(v)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(canonical)

	return hex.EncodeToString(sum[:]), nil
}

// encodeValue writes the canonical JSON encoding of v into buf. v must be the output of
// json.Unmarshal into *any — that is, one of: nil, bool, float64, string,
// []any, or map[string]any. json.Number is also accepted when the caller has opted in
// via a json.Decoder with UseNumber; our Canonicalize path does not, but we handle it
// defensively for completeness.
func encodeValue(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
		return nil
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}

		return nil
	case float64:
		return encodeFloat(buf, val)
	case json.Number:
		buf.WriteString(val.String())
		return nil
	case string:
		return encodeString(buf, val)
	case []any:
		return encodeArray(buf, val)
	case map[string]any:
		return encodeObject(buf, val)
	default:
		return fmt.Errorf("canonicaljson: unsupported intermediate type %T", v)
	}
}

func encodeArray(buf *bytes.Buffer, arr []any) error {
	buf.WriteByte('[')

	for i, item := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}

		if err := encodeValue(buf, item); err != nil {
			return err
		}
	}

	buf.WriteByte(']')

	return nil
}

func encodeObject(buf *bytes.Buffer, obj map[string]any) error {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	buf.WriteByte('{')

	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}

		if err := encodeString(buf, k); err != nil {
			return err
		}

		buf.WriteByte(':')

		if err := encodeValue(buf, obj[k]); err != nil {
			return err
		}
	}

	buf.WriteByte('}')

	return nil
}

func encodeString(buf *bytes.Buffer, s string) error {
	// Reuse encoding/json for string escaping so we inherit its \u escape policy and
	// HTML-escape behavior would normally be toggled with SetEscapeHTML, but since
	// json.Marshal enables HTML escaping by default and we want determinism, we use a
	// json.Encoder with escaping disabled and trim the trailing newline it appends.
	var tmp bytes.Buffer

	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("canonicaljson: string encode failed: %w", err)
	}

	out := tmp.Bytes()
	// json.Encoder appends a '\n'; strip it.
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}

	buf.Write(out)

	return nil
}

func encodeFloat(buf *bytes.Buffer, f float64) error {
	// Defer to encoding/json for number formatting. This guarantees byte-for-byte
	// parity with what the standard library would produce (integer-valued floats emit
	// without a trailing ".0"; tiny values use exponent notation) and absorbs any
	// future Go-stdlib changes for free.
	b, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("canonicaljson: float encode failed: %w", err)
	}

	buf.Write(b)

	return nil
}
