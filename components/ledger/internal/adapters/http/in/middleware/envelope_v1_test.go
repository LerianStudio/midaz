// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The v3 reference bodies below are the contract. They are asserted as exact
// bytes, not as decoded maps, because key order and the exact key SET are both
// part of what a v3 client saw — a decoded comparison would pass on an envelope
// that reordered keys or grew one.
func TestRenderLegacyV1(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		problem string
		want    string
	}{
		{
			// The bug report. ValidationError: v4 carries entityType, v3 did not,
			// because WithError zeroed it before rendering.
			name:   "400 validation error drops entityType and matches the bug report",
			status: 400,
			problem: `{"type":"https://errors.lerian.studio/v1/0065","title":"Invalid Path Parameter",` +
				`"status":400,"detail":"One or more path parameters are in an incorrect format. Please check ` +
				`the following parameters organization_id and ensure they meet the required format before ` +
				`trying again.","code":"0065","entityType":"Ledger"}`,
			want: `{"title":"Invalid Path Parameter","message":"One or more path parameters are in an ` +
				`incorrect format. Please check the following parameters organization_id and ensure they ` +
				`meet the required format before trying again.","code":"0065"}`,
		},
		{
			name:   "400 known-fields error keeps entityType and rebuilds fields",
			status: 400,
			problem: `{"type":"https://errors.lerian.studio/v1/0018","title":"Missing Fields in Request",` +
				`"status":400,"detail":"Your request is missing fields.","code":"0018","entityType":"Account",` +
				`"errors":[{"message":"name is required","location":"name"}]}`,
			want: `{"entityType":"Account","title":"Missing Fields in Request","message":"Your request is ` +
				`missing fields.","code":"0018","fields":{"name":"name is required"}}`,
		},
		{
			name:   "400 unknown-fields error rebuilds fields from value",
			status: 400,
			problem: `{"type":"https://errors.lerian.studio/v1/0053","title":"Unexpected Fields",` +
				`"status":400,"detail":"The request contains unexpected fields.","code":"0053",` +
				`"entityType":"Account","errors":[{"message":"unexpected field","location":"nickname",` +
				`"value":"bob"}]}`,
			want: `{"entityType":"Account","title":"Unexpected Fields","message":"The request contains ` +
				`unexpected fields.","code":"0053","fields":{"nickname":"bob"}}`,
		},
		{
			// fiber.Map class: exactly three keys, alphabetical, no entityType.
			name:   "404 renders the flat envelope and drops entityType",
			status: 404,
			problem: `{"type":"https://errors.lerian.studio/v1/0007","title":"Entity Not Found",` +
				`"status":404,"detail":"No entity was found for the given ID.","code":"0007",` +
				`"entityType":"Ledger"}`,
			want: `{"code":"0007","message":"No entity was found for the given ID.","title":"Entity Not Found"}`,
		},
		{
			name:   "422 renders the flat envelope",
			status: 422,
			problem: `{"type":"https://errors.lerian.studio/v1/0018","title":"Insufficient Funds",` +
				`"status":422,"detail":"The account does not have enough funds.","code":"0018"}`,
			want: `{"code":"0018","message":"The account does not have enough funds.","title":"Insufficient Funds"}`,
		},
		{
			name:   "409 renders the flat envelope",
			status: 409,
			problem: `{"type":"https://errors.lerian.studio/v1/0004","title":"Duplicate Ledger",` +
				`"status":409,"detail":"A ledger with this name already exists.","code":"0004"}`,
			want: `{"code":"0004","message":"A ledger with this name already exists.","title":"Duplicate Ledger"}`,
		},
		{
			name:   "401 renders the flat envelope",
			status: 401,
			problem: `{"type":"https://errors.lerian.studio/v1/0042","title":"Invalid Token",` +
				`"status":401,"detail":"The provided token is invalid.","code":"0042"}`,
			want: `{"code":"0042","message":"The provided token is invalid.","title":"Invalid Token"}`,
		},
		{
			name:   "403 renders the flat envelope",
			status: 403,
			problem: `{"type":"https://errors.lerian.studio/v1/0043","title":"Forbidden",` +
				`"status":403,"detail":"You are not allowed to do that.","code":"0043"}`,
			want: `{"code":"0043","message":"You are not allowed to do that.","title":"Forbidden"}`,
		},
		{
			// 5xx: the deferral left registry text on the body, so it rides through
			// to the v3 message field exactly as v3 emitted it.
			name:   "500 carries the registry text through unchanged",
			status: 500,
			problem: `{"type":"https://errors.lerian.studio/v1/0046","title":"Internal Server Error",` +
				`"status":500,"detail":"The server encountered an unexpected error. Please try again later ` +
				`or contact support.","code":"0046"}`,
			want: `{"code":"0046","message":"The server encountered an unexpected error. Please try again ` +
				`later or contact support.","title":"Internal Server Error"}`,
		},
		{
			name:   "503 carries the registry text through unchanged",
			status: 503,
			problem: `{"type":"https://errors.lerian.studio/v1/0099","title":"Service Unavailable",` +
				`"status":503,"detail":"The service is unavailable.","code":"0099"}`,
			want: `{"code":"0099","message":"The service is unavailable.","title":"Service Unavailable"}`,
		},
		{
			// ResponseError is pinned to 400 by HumaProblem, so it lands in the
			// struct class. It has no errors[], so entityType drops.
			name:   "malformed body error renders as the struct class",
			status: 400,
			problem: `{"type":"https://errors.lerian.studio/v1/0094","title":"Malformed Request",` +
				`"status":400,"detail":"The request body could not be parsed.","code":"0094"}`,
			want: `{"title":"Malformed Request","message":"The request body could not be parsed.","code":"0094"}`,
		},
		{
			// 413 and 504 did not exist in v3. They take the flat class, and the
			// top-level `message` member v4 adds for them is deliberately ignored:
			// `detail` carries the same text and is the field every other class uses.
			name:   "413 renders the flat envelope",
			status: 413,
			problem: `{"type":"https://errors.lerian.studio/v1/0143","title":"Payload Too Large",` +
				`"status":413,"detail":"The request payload is too large.","code":"0143",` +
				`"message":"The request payload is too large."}`,
			want: `{"code":"0143","message":"The request payload is too large.","title":"Payload Too Large"}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := renderLegacyV1([]byte(testCase.problem), testCase.status)

			require.True(t, ok, "renderer must recognize this shape")
			assert.JSONEq(t, testCase.want, string(got))
			assert.Equal(t, testCase.want, string(got), "key order is part of the v3 contract")
		})
	}
}

func TestRenderLegacyV1_EmitsExactlyTheV3KeySet(t *testing.T) {
	t.Run("flat class always carries all three keys", func(t *testing.T) {
		got, ok := renderLegacyV1([]byte(`{"status":404,"code":"0007","title":"","detail":"","entityType":"Ledger"}`), 404)
		require.True(t, ok)

		assert.Equal(t, []string{"code", "message", "title"}, keysOf(t, got))
	})

	t.Run("struct class omits empty optionals", func(t *testing.T) {
		got, ok := renderLegacyV1([]byte(`{"status":400,"code":"0065","title":"Invalid Path Parameter","detail":"bad"}`), 400)
		require.True(t, ok)

		assert.Equal(t, []string{"code", "message", "title"}, sortedKeysOf(t, got))
		assert.NotContains(t, string(got), "entityType")
		assert.NotContains(t, string(got), "fields")
	})
}

// The gap is asserted so it stays a recorded decision. If a future change makes
// the concrete error class reachable here, this test is the one that should fail.
func TestRenderLegacyV1_KnownGap_EmptyFieldsLosesEntityType(t *testing.T) {
	got, ok := renderLegacyV1([]byte(
		`{"status":400,"code":"0018","title":"Missing Fields in Request","detail":"missing","entityType":"Account"}`,
	), 400)
	require.True(t, ok)

	assert.NotContains(t, string(got), "entityType",
		"documented gap: a known-fields error with no fields is indistinguishable from a ValidationError here")
}

func TestRenderLegacyV1_RefusesShapesItCannotConvert(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "not json", status: 400, body: `this is not json`},
		{name: "json but not an object", status: 400, body: `["nope"]`},
		{name: "no status member", status: 400, body: `{"code":"0065","title":"Invalid Path Parameter","detail":"bad"}`},
		{name: "empty body", status: 400, body: ``},
		{
			name:   "body status disagrees with the response status",
			status: 400,
			body:   `{"status":500,"code":"0046","title":"Internal Server Error","detail":"internal error"}`,
		},
		{
			name:   "a 4xx payload that is not a problem document",
			status: 409,
			body:   `{"id":"abc","status":{"code":"ACTIVE","description":"active"}}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := renderLegacyV1([]byte(testCase.body), testCase.status)

			assert.False(t, ok, "unconvertible shapes must pass through untouched")
			assert.Nil(t, got)
		})
	}
}

func TestErrorsToFields(t *testing.T) {
	t.Run("nil for no details", func(t *testing.T) {
		assert.Nil(t, errorsToFields(nil))
		assert.Nil(t, errorsToFields([]problemError{}))
	})

	t.Run("nil when every detail lacks a location", func(t *testing.T) {
		assert.Nil(t, errorsToFields([]problemError{{Message: "orphan"}}))
	})

	t.Run("value wins over message", func(t *testing.T) {
		fields := errorsToFields([]problemError{
			{Location: "nickname", Message: "unexpected field", Value: "bob"},
			{Location: "name", Message: "name is required"},
		})

		assert.Equal(t, map[string]any{"nickname": "bob", "name": "name is required"}, fields)
	})
}

func keysOf(t *testing.T, body []byte) []string {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(body))

	token, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), token)

	var keys []string

	for decoder.More() {
		key, keyErr := decoder.Token()
		require.NoError(t, keyErr)

		keys = append(keys, key.(string))

		var discard any
		require.NoError(t, decoder.Decode(&discard))
	}

	return keys
}

func sortedKeysOf(t *testing.T, body []byte) []string {
	t.Helper()

	keys := keysOf(t, body)
	slices.Sort(keys)

	return keys
}
