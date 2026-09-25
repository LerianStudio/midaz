// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReserveResponseJSONRequiresCompleteDecision(t *testing.T) {
	valid := `{"contractRevision":"context-reserve-1","transactionId":"11111111-1111-4111-8111-111111111111","evaluationId":"22222222-2222-4222-8222-222222222222","decision":"ALLOW","controls":{"rules":"EVALUATED","limits":"EVALUATED"},"reservationIds":[],"reasons":["LIMITS_SATISFIED"]}`
	result, err := DecodeReserveResultJSON(t.Context(), []byte(valid), 65536, 100)
	require.NoError(t, err)
	require.NotNil(t, result.ReservationIDs)
	for _, body := range []string{
		strings.Replace(valid, `"decision":"ALLOW"`, `"decision":"ALLOW","decision":"DENY"`, 1),
		strings.Replace(valid, `"rules":"EVALUATED"`, `"rules":"EVALUATED","rules":"NOT_REQUESTED"`, 1),
		strings.Replace(valid, `"reservationIds":[]`, `"reservationIds":null`, 1),
		strings.Replace(valid, `"reservationIds":[],`, "", 1),
		strings.Replace(valid, `"decision":"ALLOW"`, `"denied":false`, 1),
		valid + `{}`,
	} {
		result, err := DecodeReserveResultJSON(t.Context(), []byte(body), 65536, 100)
		require.Error(t, err)
		require.Nil(t, result)
	}
}

func TestCompletionResponseJSONRequiresMovementCount(t *testing.T) {
	valid := `{"contractRevision":"context-reserve-1","transactionId":"11111111-1111-4111-8111-111111111111","status":"RELEASED","flipped":0}`
	result, err := DecodeTransactionCompletionJSON(t.Context(), []byte(valid), 65536)
	require.NoError(t, err)
	require.Nil(t, result.EvaluationID)
	for _, body := range []string{
		strings.Replace(valid, `,"flipped":0`, "", 1),
		strings.Replace(valid, `"flipped":0`, `"flipped":null`, 1),
		strings.Replace(valid, `"flipped":0`, `"flipped":0.1`, 1),
		strings.Replace(valid, `"flipped":0`, `"flipped":"0"`, 1),
		strings.Replace(valid, `"flipped":0`, `"flipped":1`, 1),
		strings.Replace(valid, `"flipped":0`, `"flipped":0,"evaluationId":null`, 1),
		strings.Replace(valid, `"flipped":0`, `"flipped":0,"unknown":false`, 1),
	} {
		result, err := DecodeTransactionCompletionJSON(t.Context(), []byte(body), 65536)
		require.Error(t, err)
		require.Nil(t, result)
	}
}
