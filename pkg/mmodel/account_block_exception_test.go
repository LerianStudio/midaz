// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateAccountBlockExceptionsInput_WireShape locks the REQUEST field names.
// The route is the control-plane half of a contract whose other half is the
// transaction body, so a rename here silently breaks every caller that mints a
// grant.
func TestCreateAccountBlockExceptionsInput_WireShape(t *testing.T) {
	t.Parallel()

	const body = `{"exceptions":[{"accountAlias":"@fraud_account","amount":"150.00","ttl":300}]}`

	var got CreateAccountBlockExceptionsInput
	require.NoError(t, json.Unmarshal([]byte(body), &got))

	require.Len(t, got.Exceptions, 1)
	assert.Equal(t, "@fraud_account", got.Exceptions[0].AccountAlias)
	assert.Equal(t, "150.00", got.Exceptions[0].Amount)
	require.NotNil(t, got.Exceptions[0].TTL)
	assert.Equal(t, 300, *got.Exceptions[0].TTL)
}

// TestCreateAccountBlockExceptionInput_TTLAbsentIsNil pins that an omitted ttl is
// distinguishable from an explicit zero: the default is applied only when the
// field is ABSENT, and an explicit 0 must be a rejection rather than a silent
// fallback to 300s.
func TestCreateAccountBlockExceptionInput_TTLAbsentIsNil(t *testing.T) {
	t.Parallel()

	var absent CreateAccountBlockExceptionInput
	require.NoError(t, json.Unmarshal([]byte(`{"accountAlias":"@a","amount":"1"}`), &absent))
	assert.Nil(t, absent.TTL, "an omitted ttl must stay nil so the default is applied")

	var zero CreateAccountBlockExceptionInput
	require.NoError(t, json.Unmarshal([]byte(`{"accountAlias":"@a","amount":"1","ttl":0}`), &zero))
	require.NotNil(t, zero.TTL, "an explicit zero must be observable, not indistinguishable from absent")
	assert.Equal(t, 0, *zero.TTL)
}

// TestAccountBlockExceptions_WireShape locks the RESPONSE key set and the
// identifier's spelling — `accountBlockExceptionId`, the exact name the
// transaction body will carry.
func TestAccountBlockExceptions_WireShape(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, 9, 8, 12, 30, 0, 0, time.UTC)

	raw, err := json.Marshal(AccountBlockExceptions{
		Exceptions: []AccountBlockException{{
			AccountBlockExceptionID: "018f2c1e-6a3b-7c4d-8e5f-0a1b2c3d4e5f",
			AccountAlias:            "@fraud_account",
			Amount:                  "150.00",
			ExpiresAt:               expiresAt,
		}},
	})
	require.NoError(t, err)

	var decoded map[string][]map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	items, ok := decoded["exceptions"]
	require.True(t, ok, "the envelope key must be 'exceptions'")
	require.Len(t, items, 1)

	assert.Equal(t, map[string]any{
		"accountBlockExceptionId": "018f2c1e-6a3b-7c4d-8e5f-0a1b2c3d4e5f",
		"accountAlias":            "@fraud_account",
		"amount":                  "150.00",
		"expiresAt":               "2026-09-08T12:30:00Z",
	}, items[0], "the response item must carry exactly these four keys, expiresAt in RFC 3339")
}

// TestAccountBlockExceptionRedis_CamelCaseCasing locks the CACHE casing contract.
// The blob is decoded by the transaction Lua script with cjson, which reads the
// literal key names, so a Go-side tag rename would round-trip cleanly here while
// breaking consumption at runtime — hence the assertion on the raw bytes.
func TestAccountBlockExceptionRedis_CamelCaseCasing(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(AccountBlockExceptionRedis{Alias: "@fraud_account", Amount: "150"})
	require.NoError(t, err)

	var decoded map[string]string
	require.NoError(t, json.Unmarshal(raw, &decoded))

	assert.Equal(t, map[string]string{"Alias": "@fraud_account", "Amount": "150"}, decoded,
		"the cached blob must use CamelCase keys, matching the balance blobs the same script reads")
}

// TestAccountBlockExceptionBounds pins the three documented numbers, which are
// part of the published contract (the OpenAPI schema advertises the batch cap and
// the ttl range) and not free to drift silently.
func TestAccountBlockExceptionBounds(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 100, AccountBlockExceptionMaxBatchSize)
	assert.Equal(t, 300, AccountBlockExceptionDefaultTTLSeconds)
	assert.Equal(t, 86400, AccountBlockExceptionMaxTTLSeconds)
	assert.Less(t, AccountBlockExceptionDefaultTTLSeconds, AccountBlockExceptionMaxTTLSeconds,
		"the default must sit inside the accepted range")
}
