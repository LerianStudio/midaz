// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeBehindEnvelopeFixture(t testing.TB) TransactionWriteBehindEnvelope {
	t.Helper()

	payload, result := recoveryContractFixture(t)

	return TransactionWriteBehindEnvelope{
		FormatVersion:    TransactionWriteBehindFormatVersion,
		ApplicationState: TransactionApplicationConfirmed,
		ReplayState:      TransactionReplayReconstructible,
		DurabilityState:  TransactionDurabilityPending,
		Record:           recoveryContractEnvelope(t, payload, result),
		Dependencies:     []TransactionEvidenceReference{},
	}
}

func TestTransactionWriteBehindEnvelopeRoundTrip(t *testing.T) {
	envelope := writeBehindEnvelopeFixture(t)

	encoded, err := EncodeTransactionWriteBehindEnvelope(envelope)
	require.NoError(t, err)

	decoded, err := DecodeTransactionWriteBehindEnvelope(encoded)
	require.NoError(t, err)
	decodedJSON, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(decodedJSON))
}

func TestTransactionWriteBehindEnvelopeReadsLegacyCompletionRecord(t *testing.T) {
	envelope := writeBehindEnvelopeFixture(t)
	legacy, err := EncodeTransactionCompletionRecord(envelope.Record)
	require.NoError(t, err)

	decoded, err := DecodeTransactionWriteBehindEnvelope(legacy)
	require.NoError(t, err)
	assert.Equal(t, TransactionWriteBehindFormatVersion, decoded.FormatVersion)
	assert.Equal(t, TransactionApplicationConfirmed, decoded.ApplicationState)
	assert.Equal(t, TransactionReplayReconstructible, decoded.ReplayState)
	assert.Equal(t, TransactionDurabilityPending, decoded.DurabilityState)
	expectedRecord, err := json.Marshal(envelope.Record)
	require.NoError(t, err)
	actualRecord, err := json.Marshal(decoded.Record)
	require.NoError(t, err)
	require.JSONEq(t, string(expectedRecord), string(actualRecord))
	assert.Empty(t, decoded.Dependencies)
}

func TestTransactionWriteBehindEnvelopeDependencies(t *testing.T) {
	envelope := writeBehindEnvelopeFixture(t)
	predecessorExecutionID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	envelope.Dependencies = []TransactionEvidenceReference{{
		Kind: TransactionDependencyPredecessor, TenantID: envelope.Record.TenantID,
		OrganizationID: envelope.Record.OrganizationID, LedgerID: envelope.Record.LedgerID,
		TransactionID: envelope.Record.TransactionID, ExecutionID: predecessorExecutionID,
	}}

	encoded, err := EncodeTransactionWriteBehindEnvelope(envelope)
	require.NoError(t, err)
	decoded, err := DecodeTransactionWriteBehindEnvelope(encoded)
	require.NoError(t, err)
	assert.Equal(t, envelope.Dependencies, decoded.Dependencies)
}

func TestTransactionWriteBehindEnvelopeRequiresCorrelatedOrigin(t *testing.T) {
	envelope := writeBehindEnvelopeFixture(t)
	payload, err := DecodeTransactionCompletionPlan([]byte(envelope.Record.Payload))
	require.NoError(t, err)

	originID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	payload.ParentTransactionID = &originID
	payload.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(*payload))
	require.NoError(t, err)
	envelope.Record = recoveryContractEnvelope(t, *payload, envelope.Record.Result)

	_, err = EncodeTransactionWriteBehindEnvelope(envelope)
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)

	envelope.Dependencies = []TransactionEvidenceReference{{
		Kind: TransactionDependencyOrigin, TenantID: envelope.Record.TenantID,
		OrganizationID: envelope.Record.OrganizationID, LedgerID: envelope.Record.LedgerID,
		TransactionID: originID, ExecutionID: uuid.MustParse("33333333-3333-4333-8333-333333333333"),
	}}
	_, err = EncodeTransactionWriteBehindEnvelope(envelope)
	require.NoError(t, err)
}

func TestTransactionWriteBehindEnvelopeRejectsInvalidDependencies(t *testing.T) {
	base := writeBehindEnvelopeFixture(t)
	valid := TransactionEvidenceReference{
		Kind: TransactionDependencyPredecessor, TenantID: base.Record.TenantID,
		OrganizationID: base.Record.OrganizationID, LedgerID: base.Record.LedgerID,
		TransactionID: base.Record.TransactionID,
		ExecutionID:   uuid.MustParse("11111111-1111-4111-8111-111111111111"),
	}

	tests := []struct {
		name   string
		mutate func(*TransactionWriteBehindEnvelope)
	}{
		{"scope", func(envelope *TransactionWriteBehindEnvelope) { envelope.Dependencies[0].LedgerID = uuid.New() }},
		{"cycle", func(envelope *TransactionWriteBehindEnvelope) {
			envelope.Dependencies[0].ExecutionID = envelope.Record.ExecutionID
		}},
		{"correlation", func(envelope *TransactionWriteBehindEnvelope) { envelope.Dependencies[0].TransactionID = uuid.New() }},
		{"duplicate", func(envelope *TransactionWriteBehindEnvelope) {
			envelope.Dependencies = append(envelope.Dependencies, envelope.Dependencies[0])
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := base
			envelope.Dependencies = []TransactionEvidenceReference{valid}
			tt.mutate(&envelope)
			_, err := EncodeTransactionWriteBehindEnvelope(envelope)
			require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
		})
	}
}

func TestTransactionWriteBehindEnvelopeRejectsUnknownAndDuplicateJSON(t *testing.T) {
	envelope := writeBehindEnvelopeFixture(t)
	encoded, err := EncodeTransactionWriteBehindEnvelope(envelope)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	fields["unknown"] = json.RawMessage(`true`)
	unknown, err := json.Marshal(fields)
	require.NoError(t, err)
	_, err = DecodeTransactionWriteBehindEnvelope(unknown)
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)

	duplicate := append([]byte(`{"formatVersion":1,`), encoded[1:]...)
	_, err = DecodeTransactionWriteBehindEnvelope(duplicate)
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
}

func TestTransactionEvidenceIndexRoundTripAndValidation(t *testing.T) {
	envelope := writeBehindEnvelopeFixture(t)
	index := TransactionEvidenceIndex{
		FormatVersion: TransactionEvidenceIndexFormatVersion, TenantID: envelope.Record.TenantID,
		OrganizationID: envelope.Record.OrganizationID, LedgerID: envelope.Record.LedgerID,
		TransactionID: envelope.Record.TransactionID, ExecutionID: envelope.Record.ExecutionID,
		Action: "CREATE", ApplicationState: TransactionApplicationConfirmed,
		ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityPending,
		RecoveryField: envelope.Record.TransactionID.String() + ":" + envelope.Record.ExecutionID.String(),
		ReceiptField:  envelope.Record.ExecutionID.String(), Dependencies: []TransactionEvidenceReference{},
	}

	encoded, err := EncodeTransactionEvidenceIndex(index)
	require.NoError(t, err)
	decoded, err := DecodeTransactionEvidenceIndex(encoded)
	require.NoError(t, err)
	assert.Equal(t, index, *decoded)

	index.RecoveryField = uuid.NewString()
	_, err = EncodeTransactionEvidenceIndex(index)
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)

	duplicate := append([]byte(`{"formatVersion":1,`), encoded[1:]...)
	_, err = DecodeTransactionEvidenceIndex(duplicate)
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
}

func TestTransactionWriteBehindDecodersNormalizeOnlyLegacyEmptyDependenciesObject(t *testing.T) {
	envelope := writeBehindEnvelopeFixture(t)
	index := TransactionEvidenceIndex{
		FormatVersion: TransactionEvidenceIndexFormatVersion, TenantID: envelope.Record.TenantID,
		OrganizationID: envelope.Record.OrganizationID, LedgerID: envelope.Record.LedgerID,
		TransactionID: envelope.Record.TransactionID, ExecutionID: envelope.Record.ExecutionID,
		Action: "CREATE", ApplicationState: TransactionApplicationConfirmed,
		ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityComplete,
		RecoveryField: envelope.Record.TransactionID.String() + ":" + envelope.Record.ExecutionID.String(),
		ReceiptField:  envelope.Record.ExecutionID.String(), Dependencies: []TransactionEvidenceReference{},
	}

	encodedEnvelope, err := EncodeTransactionWriteBehindEnvelope(envelope)
	require.NoError(t, err)
	encodedIndex, err := EncodeTransactionEvidenceIndex(index)
	require.NoError(t, err)

	tests := []struct {
		name   string
		data   []byte
		decode func([]byte) ([]TransactionEvidenceReference, error)
	}{
		{
			name: "envelope", data: encodedEnvelope,
			decode: func(data []byte) ([]TransactionEvidenceReference, error) {
				decoded, decodeErr := DecodeTransactionWriteBehindEnvelope(data)
				if decodeErr != nil {
					return nil, decodeErr
				}
				return decoded.Dependencies, nil
			},
		},
		{
			name: "index", data: encodedIndex,
			decode: func(data []byte) ([]TransactionEvidenceReference, error) {
				decoded, decodeErr := DecodeTransactionEvidenceIndex(data)
				if decodeErr != nil {
					return nil, decodeErr
				}
				return decoded.Dependencies, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacy := bytes.Replace(tt.data, []byte(`"dependencies":[]`), []byte(`"dependencies":{}`), 1)
			require.NotEqual(t, tt.data, legacy)
			dependencies, decodeErr := tt.decode(legacy)
			require.NoError(t, decodeErr)
			require.NotNil(t, dependencies)
			require.Empty(t, dependencies)

			invalid := bytes.Replace(tt.data, []byte(`"dependencies":[]`), []byte(`"dependencies":{"kind":"origin"}`), 1)
			require.NotEqual(t, tt.data, invalid)
			_, decodeErr = tt.decode(invalid)
			require.ErrorIs(t, decodeErr, ErrInvalidTransactionCompletionRecord)
		})
	}
}
