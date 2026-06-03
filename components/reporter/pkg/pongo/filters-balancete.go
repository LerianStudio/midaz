// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// dayFieldName is the field injected by add_day on each item.
const dayFieldName = "dia"

// signedFieldName is the field injected by add_signed on each item.
const signedFieldName = "signed"

// directionFieldName is the operation field that carries the accounting side.
const directionFieldName = "direction"

// directionDebit is the operation direction treated as negative.
const directionDebit = "debit"

// addDayFilter materializes a numeric YYYYMMDD day field ("dia") on each item,
// derived from a timestamp field. It exists because the grouping/aggregation
// tags (group_by, sum_by) read a raw field name and never transform per item,
// so a timestamp like created_at cannot be truncated to a day inline. Once "dia"
// exists as a real field, group_by "dia,route_code" can enumerate days and
// "dia <= l.dia" can drive running-balance sums.
//
// The day is an int (e.g. 20260501) so template comparisons (== and <=) resolve
// numerically instead of as broken string/date comparisons.
//
// Syntax: {{ collection|add_day:"created_at" }}
// Handles both time.Time (direct/Postgres mode) and string (fetcher/JSON mode)
// via the shared parseTime() helper. Items whose date field is missing or
// unparseable are returned unchanged (no "dia" injected).
func addDayFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	list, ok := toMapSlice(in.Interface())
	if !ok {
		return nil, &pongo2.Error{
			Sender:    "add_day",
			OrigError: fmt.Errorf("expected array of maps, got %T", in.Interface()),
		}
	}

	dateField := strings.TrimSpace(param.String())
	if dateField == "" {
		return nil, &pongo2.Error{
			Sender:    "add_day",
			OrigError: fmt.Errorf("add_day requires a date field name, e.g. add_day:\"created_at\""),
		}
	}

	for _, item := range list {
		val, found := getNestedField(item, dateField)
		if !found {
			continue
		}

		t := parseTime(val)
		if t.IsZero() {
			continue
		}

		dia, err := strconv.Atoi(t.Format("20060102"))
		if err != nil {
			continue
		}

		item[dayFieldName] = dia
	}

	return pongo2.AsValue(list), nil
}

// addSignedFilter materializes a signed amount field ("signed") on each item,
// applying the accounting sign from the item's "direction": debit becomes
// negative, credit (or anything else) stays positive. The operation.amount
// column is always positive in Midaz — the sign lives only in direction — so a
// single sum_by cannot produce a net (credit - debit) value without this
// pre-computed signed field.
//
// Syntax: {{ collection|add_signed:"amount" }}
// The signed value is stored as a decimal string (matching how amount flows),
// so sum_by ... by "signed" sums it natively. Items without a parseable amount
// are returned unchanged (no "signed" injected).
func addSignedFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	list, ok := toMapSlice(in.Interface())
	if !ok {
		return nil, &pongo2.Error{
			Sender:    "add_signed",
			OrigError: fmt.Errorf("expected array of maps, got %T", in.Interface()),
		}
	}

	amountField := strings.TrimSpace(param.String())
	if amountField == "" {
		return nil, &pongo2.Error{
			Sender:    "add_signed",
			OrigError: fmt.Errorf("add_signed requires an amount field name, e.g. add_signed:\"amount\""),
		}
	}

	for _, item := range list {
		raw, found := getNestedField(item, amountField)
		if !found {
			continue
		}

		amount, ok := toDecimal(raw)
		if !ok {
			continue
		}

		if dir, ok := getNestedField(item, directionFieldName); ok {
			if strings.EqualFold(fmt.Sprintf("%v", dir), directionDebit) {
				amount = amount.Neg()
			}
		}

		item[signedFieldName] = amount.String()
	}

	return pongo2.AsValue(list), nil
}
