// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestOperation_AccountTypeStaysOffThePublicJSONWire locks the field out of the
// public operation shape. There is no account_type column, so an operation read
// back from Postgres cannot carry it; emitting it on the create response only
// would make the public contract disagree with itself between a create and a
// later read.
func TestOperation_AccountTypeStaysOffThePublicJSONWire(t *testing.T) {
	raw, err := json.Marshal(&Operation{ID: "op-1", AccountType: constant.ExternalAccountType})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	assert.NotContains(t, decoded, "accountType",
		"AccountType is internal; the public operation shape must not grow a key it cannot fill on reads")
}

// TestOperation_AccountTypeSurvivesMsgpackRoundTrip is load-bearing: the
// operation is built in the HTTP handler and only reaches the lifecycle emit
// after a msgpack hop through the transaction queue. msgpack keys on the Go
// field name and does not read json tags, so `json:"-"` withholds the field
// from the public wire without withholding it from the queue — but that is a
// property of the codec, not of this repository, so it is pinned here.
func TestOperation_AccountTypeSurvivesMsgpackRoundTrip(t *testing.T) {
	raw, err := msgpack.Marshal(&Operation{ID: "op-1", AccountType: constant.ExternalAccountType})
	require.NoError(t, err)

	var decoded Operation
	require.NoError(t, msgpack.Unmarshal(raw, &decoded))

	assert.Equal(t, constant.ExternalAccountType, decoded.AccountType,
		"the account type must survive the transaction queue hop")
}

// TestOperation_AccountTypeSurvivesRedisRoundTrip covers the other carrier: the
// Redis backup envelope, from which the consumer rebuilds operations when it
// replays a transaction whose first pass failed. Without this the replayed
// transaction would emit lifecycle events missing the field.
func TestOperation_AccountTypeSurvivesRedisRoundTrip(t *testing.T) {
	op := &Operation{ID: "op-1", AccountType: constant.ExternalAccountType}

	assert.Equal(t, constant.ExternalAccountType, op.ToRedis().AccountType,
		"ToRedis must carry the account type into the backup envelope")

	restored := OperationFromRedis(op.ToRedis())
	require.NotNil(t, restored)
	assert.Equal(t, constant.ExternalAccountType, restored.AccountType,
		"OperationFromRedis must restore the account type")
}
