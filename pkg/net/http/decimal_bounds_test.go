// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func transactionBody(value string) []byte {
	leg := `"amount":{"asset":"BRL","value":"` + value + `"}`

	return []byte(`{"send":{"asset":"BRL","value":"` + value + `",` +
		`"source":{"from":[{"accountAlias":"@a",` + leg + `}]},` +
		`"distribute":{"to":[{"accountAlias":"@b",` + leg + `}]}}}`)
}

func TestDecodeRefusesOutOfBoundDecimalExponent(t *testing.T) {
	t.Parallel()

	value := "1e" + strconv.Itoa(safe.MaxDecimalExponent+1)
	bodies := []struct {
		name   string
		body   []byte
		target any
		field  string
	}{
		{"transaction", transactionBody(value), &mtransaction.Transaction{}, "send.value"},
		{"number token", []byte(`{"send":{"asset":"BRL","value":` + value + `}}`), &mtransaction.Transaction{}, "send.value"},
		{"array element", []byte(`{"send":{"source":{"from":[{"amount":{"value":"` + value + `"}}]}}}`), &mtransaction.Transaction{}, "send.source.from[0].amount.value"},
		{"key in another case", []byte(`{"Send":{"VALUE":"` + value + `"}}`), &mtransaction.Transaction{}, "Send.VALUE"},
		{"map value", []byte(`{"rates":{"cdi":"` + value + `"}}`), &struct {
			Rates map[string]*decimal.Decimal `json:"rates"`
		}{}, "rates.cdi"},
	}

	for _, b := range bodies {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeAndValidate(b.body, b.target)

			var vErr pkg.ValidationKnownFieldsError
			require.ErrorAs(t, err, &vErr)
			require.Equal(t, cn.ErrBadRequest.Error(), vErr.Code)
			require.Equal(t, b.field+" is outside the supported decimal range", vErr.Fields[b.field])
		})
	}
}

func TestDecodeKeepsInBoundDecimals(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"a cent":                            "0.01",
		"an ordinary amount":                "12500.50",
		"thirty digits and eighteen places": "999999999999999999999999999999.999999999999999999",
	}

	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			want := decimal.RequireFromString(value)

			var tx mtransaction.Transaction

			_, err := DecodeAndValidate(transactionBody(value), &tx)
			require.NoError(t, err)
			require.True(t, want.Equal(tx.Send.Value), "got %s", tx.Send.Value)
		})
	}
}

func TestDecodeRefusesLongNumericTokens(t *testing.T) {
	t.Parallel()

	// Leading zeros keep the parsed values small, so only the text length is out of bounds.
	long := strings.Repeat("0", safe.MaxDecimalTextLength) + "1"
	longNumber := "1e+" + strings.Repeat("0", safe.MaxDecimalTextLength-3) + "1"
	leg := func(value string) string {
		return `{"accountAlias":"@a","amount":{"asset":"BRL","value":"` + value + `"}}`
	}

	cases := []struct {
		name  string
		body  []byte
		field string
	}{
		{"number token in a decimal field", []byte(`{"send":{"asset":"BRL","value":` + longNumber + `}}`), "send.value"},
		{"digits and a trailing letter in a decimal field", transactionBody(long + "x"), "send.value"},
		{"digits in an array element", []byte(`{"send":{"source":{"from":[` + leg("1") + `,` + leg(long) + `]}}}`), "send.source.from[1].amount.value"},
		{"digits in a key sent twice", []byte(`{"send":{"value":"` + long + `","value":"1"}}`), "send.value"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, details, err := DecodeAndValidateWithDetails(c.body, &mtransaction.Transaction{})

			var vErr pkg.ValidationKnownFieldsError
			require.ErrorAs(t, err, &vErr)
			require.Equal(t, cn.ErrBadRequest.Error(), vErr.Code)
			require.Contains(t, vErr.Fields, c.field)
			require.NotContains(t, fmt.Sprintf("%+v %+v", vErr, details), long[:64], "the refusal must not echo the value")
		})
	}
}

func TestDecodeKeepsNumericTextOutsideDecimalFields(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("0", safe.MaxDecimalTextLength) + "1"

	var named struct {
		Name string `json:"name"`
	}

	_, err := DecodeAndValidate([]byte(`{"name":"`+long+`"}`), &named)
	require.NoError(t, err)
	require.Equal(t, long, named.Name)

	body := []byte(`{"metadata":{"commit":"12e45678"},"send":{"asset":"BRL","value":"1",` +
		`"source":{"from":[{"accountAlias":"1E2024","amount":{"asset":"BRL","value":"1"}}]},` +
		`"distribute":{"to":[{"accountAlias":"@b","amount":{"asset":"BRL","value":"1"}}]}}}`)

	var tx mtransaction.Transaction

	_, err = DecodeAndValidate(body, &tx)
	require.NoError(t, err)
	require.Equal(t, "12e45678", tx.Metadata["commit"])
	require.Equal(t, "1E2024", tx.Send.Source.From[0].AccountAlias)
}

func TestDecodeRefusesKeysLongerThanTheBound(t *testing.T) {
	t.Parallel()

	key := strings.Repeat("k", safe.MaxDecimalTextLength+1)

	_, err := DecodeAndValidate([]byte(`{"metadata":{"`+key+`":"v"}}`), &mtransaction.Transaction{})

	var vErr pkg.ValidationKnownFieldsError
	require.ErrorAs(t, err, &vErr)
	require.Equal(t, cn.ErrBadRequest.Error(), vErr.Code)
	require.Equal(t, "metadata has a key longer than the supported length", vErr.Fields["metadata"])
	require.NotContains(t, fmt.Sprintf("%+v", vErr), key[:64], "the refusal must not echo the key")

	_, err = DecodeAndValidate([]byte(`{"`+key[:safe.MaxDecimalTextLength]+`":"v"}`), &mtransaction.Transaction{})
	require.NotContains(t, fmt.Sprintf("%+v", err), "longer than the supported length", "a key at the bound passes the scan")
}

func TestDecodeRefusesNestingDeeperThanTheBound(t *testing.T) {
	t.Parallel()

	nested := func(depth int) []byte {
		return []byte(strings.Repeat(`{"a":`, depth-1) + `{}` + strings.Repeat(`}`, depth-1))
	}

	_, err := RefuseOutOfBoundTokens(nested(maxBodyDepth), nil)
	require.NoError(t, err)

	_, err = RefuseOutOfBoundTokens(nested(maxBodyDepth+1), nil)

	var vErr pkg.ValidationKnownFieldsError
	require.ErrorAs(t, err, &vErr)
	require.Equal(t, cn.ErrBadRequest.Error(), vErr.Code)

	field := strings.TrimSuffix(strings.Repeat("a.", maxBodyDepth), ".")
	require.Equal(t, field+" exceeds the supported nesting depth", vErr.Fields[field])
}
