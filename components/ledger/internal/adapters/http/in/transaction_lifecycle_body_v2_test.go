// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// These tests cover the OPTIONAL body the /v2 commit and revert grew for the
// single-use account-block exception. The whole point of the shape is backward
// compatibility: the bodiless request those two actions shipped with must keep
// working byte for byte, while a body that IS sent gets the same decode strictness
// every other body on this surface gets.

// TestDecodeLifecycleV2Body_EmptyBodyPresentsNoGrant is the compatibility lock. A
// client integrated against the bodiless commit sends no bytes, and some HTTP
// stacks send an empty string or a bare newline instead — none of those may become
// a decode error.
func TestDecodeLifecycleV2Body_EmptyBodyPresentsNoGrant(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		body []byte
	}{
		{name: "nil body", body: nil},
		{name: "zero-length body", body: []byte{}},
		{name: "whitespace only", body: []byte("  \n\t ")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exceptionID, err := decodeLifecycleV2Body(tt.body)

			require.NoError(t, err, "an empty body is the no-grant case, never an error")
			assert.Nil(t, exceptionID)
		})
	}
}

// TestDecodeLifecycleV2Body_PresentsTheGrant locks the one field the body carries,
// including the explicit-null form: a body that names the key with a null value
// presents no grant rather than failing.
func TestDecodeLifecycleV2Body_PresentsTheGrant(t *testing.T) {
	t.Parallel()

	presented := uuid.New()

	exceptionID, err := decodeLifecycleV2Body([]byte(`{"accountBlockExceptionId":"` + presented.String() + `"}`))
	require.NoError(t, err)
	require.NotNil(t, exceptionID)
	assert.Equal(t, presented, *exceptionID)

	empty, err := decodeLifecycleV2Body([]byte(`{}`))
	require.NoError(t, err)
	assert.Nil(t, empty, "an empty JSON object presents no grant")
}

// TestDecodeLifecycleV2Body_RejectsMalformedAndUnknown proves the optional body is
// not a lenient body. A misspelled field must not be silently ignored — the caller
// would believe it presented a grant and be answered with a plain account-block
// rejection naming nothing.
func TestDecodeLifecycleV2Body_RejectsMalformedAndUnknown(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		body string
	}{
		{name: "malformed uuid", body: `{"accountBlockExceptionId":"not-a-uuid"}`},
		{name: "unknown field", body: `{"accountBlockExceptionID":"` + uuid.New().String() + `"}`},
		{name: "unrelated field", body: `{"skip":{"fees":true}}`},
		{name: "broken json", body: `{"accountBlockExceptionId":`},
		{name: "wrong type", body: `{"accountBlockExceptionId":123}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exceptionID, err := decodeLifecycleV2Body([]byte(tt.body))

			require.Error(t, err)
			assert.Nil(t, exceptionID)
		})
	}
}

// TestDecodeLifecycleV2Body_MalformedUUIDIsAFieldRejection pins WHICH layer
// answers a malformed identifier: the field's `uuid` validate tag, which names the
// field in a 400, rather than the exception lookup further down.
//
// That ordering is the useful one. "accountBlockExceptionId is not a UUID" tells
// the caller what to fix; a 422 saying the exception does not exist would send
// them looking for a grant that was never the problem.
func TestDecodeLifecycleV2Body_MalformedUUIDIsAFieldRejection(t *testing.T) {
	t.Parallel()

	_, err := decodeLifecycleV2Body([]byte(`{"accountBlockExceptionId":"550e8400-e29b-41d4-a716"}`))

	require.Error(t, err)
	assert.NotContains(t, err.Error(), constant.ErrAccountBlockExceptionInvalid.Error(),
		"the decode boundary answers first, so the lookup's code never surfaces here")

	assert.Contains(t, err.Error(), "malformed",
		"the rejection must be the decoder's field-level 400")
}

// =============================================================================
// CREATE-SIDE WIRING
// =============================================================================
// decodeAndBuildV2Transaction is the seam every v2 create action enters through,
// and the exception identifier is its THIRD return — the value createTransactionV2
// hands to the use case. The surface rule lives in mtransaction.Translate, but
// whether this seam carries the identifier out, and whether the hold's rejection
// actually propagates through it, are facts only this function can show.

// v2DirectBodyWithException returns the minimal valid v2 create body with the
// account-block exception field set to the given raw value.
func v2DirectBodyWithException(rawID string) string {
	return `{"description":"v2 direct","asset":"BRL","amount":"100",` +
		`"accountBlockExceptionId":"` + rawID + `",` +
		`"debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],` +
		`"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`
}

// TestDecodeAndBuildV2Transaction_CarriesTheException proves the direct action's
// decode carries the presented identifier out as a parsed UUID.
func TestDecodeAndBuildV2Transaction_CarriesTheException(t *testing.T) {
	t.Parallel()

	presented := uuid.New()

	tran, scope, exceptionID, err := decodeAndBuildV2Transaction(
		[]byte(v2DirectBodyWithException(presented.String())), false, "")
	require.NoError(t, err)

	require.NotNil(t, exceptionID, "the direct action must carry the presented identifier out")
	assert.Equal(t, presented, *exceptionID)

	assert.False(t, tran.Pending)
	assert.NotEmpty(t, scope.OrganizationID, "the scope must still resolve off the legs")
}

// TestDecodeAndBuildV2Transaction_AbsentExceptionIsNil is the additive guarantee at
// the create seam: a body that names no identifier decodes exactly as before.
func TestDecodeAndBuildV2Transaction_AbsentExceptionIsNil(t *testing.T) {
	t.Parallel()

	_, _, exceptionID, err := decodeAndBuildV2Transaction([]byte(v2DirectBody), false, "")

	require.NoError(t, err)
	assert.Nil(t, exceptionID, "no identifier presented means none carried")
}

// TestDecodeAndBuildV2Transaction_HoldRejectsTheException proves D2 propagates
// THROUGH this seam and not merely inside Translate: the hold answers 0509 and
// carries nothing out.
//
// Asserting it here matters because the seam is what the endpoint calls. A future
// refactor that decoded the body and dropped Translate's error — or resolved the
// identifier before the surface check — would leave the unit test on Translate
// green while the hold silently accepted a grant it consumes nothing with.
func TestDecodeAndBuildV2Transaction_HoldRejectsTheException(t *testing.T) {
	t.Parallel()

	body := []byte(v2DirectBodyWithException(uuid.New().String()))

	tran, _, exceptionID, err := decodeAndBuildV2Transaction(body, true, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrAccountBlockExceptionNotSupported.Error(),
		"the hold must reject the field explicitly, not ignore it")
	assert.Nil(t, exceptionID)
	assert.Zero(t, tran.Send.Asset, "a rejected decode must build no transaction")

	// The same body on the direct action is accepted, so the rejection is the
	// SURFACE rule and not a malformed body.
	_, _, accepted, err := decodeAndBuildV2Transaction(body, false, "")
	require.NoError(t, err)
	require.NotNil(t, accepted)
}

// TestDecodeAndBuildV2Transaction_MalformedExceptionIsAFieldRejection pins the
// layer that answers a malformed identifier on the create side too: the field's
// `uuid` validate tag, naming the field in a 400.
func TestDecodeAndBuildV2Transaction_MalformedExceptionIsAFieldRejection(t *testing.T) {
	t.Parallel()

	_, _, exceptionID, err := decodeAndBuildV2Transaction(
		[]byte(v2DirectBodyWithException("not-a-uuid")), false, "")

	require.Error(t, err)
	assert.Nil(t, exceptionID)
	assert.Contains(t, err.Error(), "malformed",
		"the decoder's field-level 400 must answer before the exception lookup")
}

// TestDecodeAndBuildV2Transaction_BlockActionAcceptsTheException covers the two
// remaining create actions, which are direct creates with an operation-type label:
// the label must not interfere with the identifier being carried out.
func TestDecodeAndBuildV2Transaction_BlockActionAcceptsTheException(t *testing.T) {
	t.Parallel()

	presented := uuid.New()

	for _, override := range []string{"BLOCK", "UNBLOCK"} {
		t.Run(override, func(t *testing.T) {
			t.Parallel()

			tran, _, exceptionID, err := decodeAndBuildV2Transaction(
				[]byte(v2DirectBodyWithException(presented.String())), false, override)
			require.NoError(t, err)

			require.NotNil(t, exceptionID)
			assert.Equal(t, presented, *exceptionID)
			assert.Equal(t, override, tran.OperationTypeOverride)
		})
	}
}
