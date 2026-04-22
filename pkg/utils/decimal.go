// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

// ParseDecimalString normalizes a value that may arrive as string, float64,
// or json.Number into a plain decimal string. Returns defaultVal when v is
// nil or of an unsupported type.
//
// This helper is typically used inside custom JSON unmarshalers where the
// same numeric field can be emitted as a string (e.g. by Lua/Redis), a
// float64 (standard JSON number), or json.Number (when the decoder is
// configured with UseNumber). It preserves the exact textual representation
// for string and json.Number inputs, and converts float64 via
// decimal.NewFromFloat to avoid binary-float surprises in the output.
func ParseDecimalString(v any, defaultVal string) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return decimal.NewFromFloat(t).String()
	case json.Number:
		return t.String()
	default:
		return defaultVal
	}
}
