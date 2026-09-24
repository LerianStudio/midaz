// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCompletionJSONRequiresExplicitContract(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", `{"contractRevision":"context-reserve-1"}`, true},
		{"missing", `{}`, false},
		{"legacy empty", ``, false},
		{"null", `{"contractRevision":null}`, false},
		{"case", `{"ContractRevision":"context-reserve-1"}`, false},
		{"unknown", `{"contractRevision":"context-reserve-1","transactionId":"injected"}`, false},
		{"duplicate", `{"contractRevision":"context-reserve-1","contract\u0052evision":"context-reserve-1"}`, false},
		{"unsupported", `{"contractRevision":"context-reserve-2"}`, false},
		{"trailing", `{"contractRevision":"context-reserve-1"}{}`, false},
		{"surrogate", `{"contractRevision":"\ud800"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeCompletionJSON(t.Context(), []byte(tc.body), 1024)
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, ReserveContractRevision, got.ContractRevision)
			} else {
				require.Error(t, err)
				require.Empty(t, got.ContractRevision)
			}
		})
	}
	body := []byte(`{"contractRevision":"context-reserve-1"}`)
	_, err := DecodeCompletionJSON(t.Context(), body, len(body)-1)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = DecodeCompletionJSON(ctx, body, 1024)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCompletionResultDoesNotInventCapacity(t *testing.T) {
	id := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	result := TransactionCompletionResult{ContractRevision: ReserveContractRevision, TransactionID: id, Status: "RELEASED"}
	require.NoError(t, result.Validate())
	result.Flipped = 1
	require.Error(t, result.Validate())
	result.EvaluationID = &id
	require.NoError(t, result.Validate())
	result.Flipped = -1
	require.Error(t, result.Validate())
	result.Flipped = 0
	result.Status = "OPEN"
	require.Error(t, result.Validate())
	result.Status = "CONFIRMED"
	result.ContractRevision = ""
	require.Error(t, result.Validate())
}
