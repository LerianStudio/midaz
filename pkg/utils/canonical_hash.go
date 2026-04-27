// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalHashJSON returns the lowercase hex SHA-256 of the canonical JSON form of v.
// Canonical form: typed model → default application → json.Marshal with sorted map keys → SHA-256.
// Used by both the CRM idempotency guard (Phase 3) and the account-registration saga (Phase 4)
// so equivalent payloads collide on the same hash.
//
// The function first marshals v to JSON (applying any struct-tag defaults and type coercions),
// then unmarshals into a generic tree, walks the tree to sort every map[string]any by key, and
// finally re-marshals the canonicalized tree. This guarantees that two payloads differing only
// in map-key ordering — whether the ordering comes from Go's map iteration or from the source
// document — produce the same hash.
func CanonicalHashJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("canonical hash: marshal input: %w", err)
	}

	var tree any
	// UseNumber preserves numeric precision; otherwise all numbers become float64 and
	// large int64 values would be lossy across the round-trip.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	if err := decoder.Decode(&tree); err != nil {
		return "", fmt.Errorf("canonical hash: decode intermediate: %w", err)
	}

	canonical, err := marshalCanonical(tree)
	if err != nil {
		return "", fmt.Errorf("canonical hash: marshal canonical: %w", err)
	}

	sum := sha256.Sum256(canonical)

	return hex.EncodeToString(sum[:]), nil
}

// marshalCanonical serializes v to JSON with every object's keys emitted in ascending
// byte-wise order. It walks maps and slices recursively and delegates scalars to json.Marshal.
func marshalCanonical(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		var buf bytes.Buffer
		buf.WriteByte('{')

		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}

			keyJSON, err := json.Marshal(k)
			if err != nil {
				return nil, fmt.Errorf("marshal key %q: %w", k, err)
			}

			buf.Write(keyJSON)
			buf.WriteByte(':')

			valJSON, err := marshalCanonical(t[k])
			if err != nil {
				return nil, err
			}

			buf.Write(valJSON)
		}

		buf.WriteByte('}')

		return buf.Bytes(), nil

	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')

		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}

			itemJSON, err := marshalCanonical(item)
			if err != nil {
				return nil, err
			}

			buf.Write(itemJSON)
		}

		buf.WriteByte(']')

		return buf.Bytes(), nil

	default:
		// Scalars (string, bool, json.Number, nil) — json.Marshal handles all of these.
		return json.Marshal(t)
	}
}
