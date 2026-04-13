// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// balanceRedisList.UnmarshalJSON
// =============================================================================

func TestBalanceRedisList_UnmarshalJSON_StandardArray(t *testing.T) {
	input := `[
		{"id":"b1","alias":"@src","accountId":"a1","available":"100","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1},
		{"id":"b2","alias":"@dst","accountId":"a2","available":"200","onHold":"0","version":2,"accountType":"deposit","allowSending":1,"allowReceiving":1}
	]`

	var list balanceRedisList
	err := json.Unmarshal([]byte(input), &list)

	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "b1", list[0].ID)
	assert.Equal(t, "@src", list[0].Alias)
	assert.Equal(t, "b2", list[1].ID)
	assert.Equal(t, "@dst", list[1].Alias)
}

func TestBalanceRedisList_UnmarshalJSON_SingleElementArray(t *testing.T) {
	input := `[{"id":"b1","alias":"@src","accountId":"a1","available":"100","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}]`

	var list balanceRedisList
	err := json.Unmarshal([]byte(input), &list)

	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "b1", list[0].ID)
}

func TestBalanceRedisList_UnmarshalJSON_EmptyArray(t *testing.T) {
	var list balanceRedisList
	err := json.Unmarshal([]byte(`[]`), &list)

	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestBalanceRedisList_UnmarshalJSON_ArrayWithNulls(t *testing.T) {
	input := `[null, {"id":"b1","alias":"@src","accountId":"a1","available":"100","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}, null]`

	var list balanceRedisList
	err := json.Unmarshal([]byte(input), &list)

	require.NoError(t, err)
	require.Len(t, list, 1, "null entries should be skipped")
	assert.Equal(t, "b1", list[0].ID)
}

func TestBalanceRedisList_UnmarshalJSON_Null(t *testing.T) {
	var list balanceRedisList
	err := json.Unmarshal([]byte(`null`), &list)

	require.NoError(t, err)
	assert.Nil(t, list)
}

func TestBalanceRedisList_UnmarshalJSON_EmptyBytes(t *testing.T) {
	// Empty input is not valid JSON. json.Unmarshal returns an error before
	// our custom UnmarshalJSON is called, so we expect a parse error.
	var list balanceRedisList
	err := json.Unmarshal([]byte(``), &list)

	assert.Error(t, err, "empty bytes should fail JSON parsing")
}

// cjson quirk: empty Lua table encoded as {} instead of [].
func TestBalanceRedisList_UnmarshalJSON_EmptyObject(t *testing.T) {
	var list balanceRedisList
	err := json.Unmarshal([]byte(`{}`), &list)

	require.NoError(t, err)
	assert.Nil(t, list, "empty object should be treated as empty array")
}

// cjson quirk: single balance returned as bare object instead of 1-element array.
func TestBalanceRedisList_UnmarshalJSON_SingleObject(t *testing.T) {
	input := `{"id":"b1","alias":"@src","accountId":"a1","available":"500","onHold":"10","version":3,"accountType":"deposit","allowSending":1,"allowReceiving":0}`

	var list balanceRedisList
	err := json.Unmarshal([]byte(input), &list)

	require.NoError(t, err)
	require.Len(t, list, 1, "bare object should be treated as single-element list")
	assert.Equal(t, "b1", list[0].ID)
	assert.Equal(t, "@src", list[0].Alias)
	assert.Equal(t, int64(3), list[0].Version)
}

// cjson quirk: Lua array-table encoded as {"1":{...},"2":{...}}.
func TestBalanceRedisList_UnmarshalJSON_NestedNumericKeys(t *testing.T) {
	input := `{
		"1": {"id":"b1","alias":"@src","accountId":"a1","available":"100","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1},
		"2": {"id":"b2","alias":"@dst","accountId":"a2","available":"200","onHold":"0","version":2,"accountType":"deposit","allowSending":1,"allowReceiving":1}
	}`

	var list balanceRedisList
	err := json.Unmarshal([]byte(input), &list)

	require.NoError(t, err)
	require.Len(t, list, 2, "numeric-keyed object should produce 2 balances")

	ids := map[string]bool{list[0].ID: true, list[1].ID: true}
	assert.True(t, ids["b1"], "should contain b1")
	assert.True(t, ids["b2"], "should contain b2")
}

func TestBalanceRedisList_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var list balanceRedisList
	err := json.Unmarshal([]byte(`not json`), &list)

	assert.Error(t, err)
}

func TestBalanceRedisList_UnmarshalJSON_UnexpectedToken(t *testing.T) {
	var list balanceRedisList
	err := json.Unmarshal([]byte(`"just a string"`), &list)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected JSON token")
}

// =============================================================================
// balanceAtomicResponse.UnmarshalJSON
// =============================================================================

func TestBalanceAtomicResponse_UnmarshalJSON_StandardArrays(t *testing.T) {
	input := `{
		"before": [{"id":"b1","alias":"@src","accountId":"a1","available":"1000","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}],
		"after":  [{"id":"b1","alias":"@src","accountId":"a1","available":"900","onHold":"0","version":2,"accountType":"deposit","allowSending":1,"allowReceiving":1}]
	}`

	var resp balanceAtomicResponse
	err := json.Unmarshal([]byte(input), &resp)

	require.NoError(t, err)
	require.Len(t, resp.Before, 1)
	require.Len(t, resp.After, 1)
	assert.Equal(t, "b1", resp.Before[0].ID)
	assert.Equal(t, "b1", resp.After[0].ID)
	assert.Equal(t, int64(1), resp.Before[0].Version)
	assert.Equal(t, int64(2), resp.After[0].Version)
}

// cjson quirk: empty result encoded as {"before":{},"after":{}}.
func TestBalanceAtomicResponse_UnmarshalJSON_EmptyObjects(t *testing.T) {
	input := `{"before":{},"after":{}}`

	var resp balanceAtomicResponse
	err := json.Unmarshal([]byte(input), &resp)

	require.NoError(t, err)
	assert.Empty(t, resp.Before)
	assert.Empty(t, resp.After)
}

// cjson quirk: empty arrays as proper JSON.
func TestBalanceAtomicResponse_UnmarshalJSON_EmptyArrays(t *testing.T) {
	input := `{"before":[],"after":[]}`

	var resp balanceAtomicResponse
	err := json.Unmarshal([]byte(input), &resp)

	require.NoError(t, err)
	assert.Empty(t, resp.Before)
	assert.Empty(t, resp.After)
}

// Mixed: before is a proper array, after is a cjson empty object.
func TestBalanceAtomicResponse_UnmarshalJSON_MixedArrayAndEmptyObject(t *testing.T) {
	input := `{
		"before": [{"id":"b1","alias":"@src","accountId":"a1","available":"500","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}],
		"after":  {}
	}`

	var resp balanceAtomicResponse
	err := json.Unmarshal([]byte(input), &resp)

	require.NoError(t, err)
	require.Len(t, resp.Before, 1)
	assert.Empty(t, resp.After)
}

func TestBalanceAtomicResponse_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var resp balanceAtomicResponse
	err := json.Unmarshal([]byte(`{invalid`), &resp)

	assert.Error(t, err)
}

// Multiple balances in before/after (N-to-N transaction scenario).
func TestBalanceAtomicResponse_UnmarshalJSON_MultipleBalances(t *testing.T) {
	input := `{
		"before": [
			{"id":"b1","alias":"@src","accountId":"a1","available":"1000","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1},
			{"id":"b2","alias":"@dst","accountId":"a2","available":"0","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}
		],
		"after": [
			{"id":"b1","alias":"@src","accountId":"a1","available":"500","onHold":"0","version":2,"accountType":"deposit","allowSending":1,"allowReceiving":1},
			{"id":"b2","alias":"@dst","accountId":"a2","available":"500","onHold":"0","version":2,"accountType":"deposit","allowSending":1,"allowReceiving":1}
		]
	}`

	var resp balanceAtomicResponse
	err := json.Unmarshal([]byte(input), &resp)

	require.NoError(t, err)
	require.Len(t, resp.Before, 2)
	require.Len(t, resp.After, 2)
}
