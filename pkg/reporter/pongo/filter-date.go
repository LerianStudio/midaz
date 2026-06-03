// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"fmt"
	"time"

	"github.com/flosch/pongo2/v6"
)

// dateFilterWithStringSupport extends the built-in |date filter to accept
// string inputs in addition to time.Time.
//
// In direct mode, the PostgreSQL pq driver returns time.Time natively.
// In fetcher mode, json.Unmarshal into map[string]any returns all date fields
// as strings (e.g., "2026-03-28T17:03:05.588Z") because JSON has no date type.
//
// This filter transparently handles both cases: if the input is already
// time.Time it formats directly; if it's a string it parses first using
// the same parseTime() function used by other Pongo2 tags.
func dateFilterWithStringSupport(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	t, isTime := in.Interface().(time.Time)
	if !isTime {
		s, isString := in.Interface().(string)
		if !isString {
			return nil, &pongo2.Error{
				Sender:    "filter:date",
				OrigError: fmt.Errorf("filter input must be time.Time or date string, got %T", in.Interface()),
			}
		}

		parsed := parseTime(s)
		if parsed.IsZero() {
			return nil, &pongo2.Error{
				Sender:    "filter:date",
				OrigError: fmt.Errorf("cannot parse %q as date", s),
			}
		}

		t = parsed
	}

	return pongo2.AsValue(t.Format(param.String())), nil
}
