// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
)

func TestDecodeAndValidateAtomicTransactionBatchV2_CanonicalRequestGolden(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"transactions": [{
			"metadata": {"zeta":"last","large":9007199254740993,"fraction":0.125},
			"credits": [{"ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1","organizationId":"00000000-0000-0000-0000-000000000001","alias":"@destination"}],
			"debits": [{"organizationId":"00000000-0000-0000-0000-000000000001","alias":"@source","amount":"1","ledgerId":"00000000-0000-0000-0000-000000000002"}],
			"amount": "1",
			"asset": "USD"
		}]
	}`)

	result, err := decodeAndValidateAtomicTransactionBatchV2(raw, 50)
	require.NoError(t, err)

	wantCanonical := `{"transactions":[{"amount":"1","asset":"USD","credits":[{"alias":"@destination","amount":"1","ledgerId":"00000000-0000-0000-0000-000000000002","organizationId":"00000000-0000-0000-0000-000000000001"}],"debits":[{"alias":"@source","amount":"1","ledgerId":"00000000-0000-0000-0000-000000000002","organizationId":"00000000-0000-0000-0000-000000000001"}],"metadata":{"fraction":0.125,"large":9007199254740993,"zeta":"last"}}]}`
	assert.Equal(t, wantCanonical, string(result.canonicalRequest))
	assert.Equal(t, "9e7056c3aaf54d1fc2ca2f4246cfc25b00a5d488238e43382bd51534ec94867a", result.requestFingerprint)
}

func TestDecodeAndValidateAtomicTransactionBatchV2_ObjectPermutationProperty(t *testing.T) {
	t.Parallel()

	base := []byte(`{
		"transactions":[{
			"asset":"USD",
			"amount":"1",
			"debits":[{"alias":"@source","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}],
			"credits":[{"alias":"@destination","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}],
			"metadata":{"integer":9007199254740993,"decimal":0.100,"exponent":1e3,"label":"stable"},
			"skip":{"fees":false,"tracer":true}
		}]
	}`)
	baseline, err := decodeAndValidateAtomicTransactionBatchV2(base, 50)
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(base))
	decoder.UseNumber()
	var document any
	require.NoError(t, decoder.Decode(&document))

	for seed := int64(0); seed < 64; seed++ {
		seed := seed
		t.Run("seed", func(t *testing.T) {
			raw := marshalJSONWithPermutedObjects(t, document, rand.New(rand.NewSource(seed)))
			if seed%2 == 0 {
				var indented bytes.Buffer
				require.NoError(t, json.Indent(&indented, raw, "", "  "))
				raw = indented.Bytes()
			}

			result, err := decodeAndValidateAtomicTransactionBatchV2(raw, 50)
			require.NoError(t, err)
			assert.Equal(t, baseline.canonicalRequest, result.canonicalRequest)
			assert.Equal(t, baseline.requestFingerprint, result.requestFingerprint)
		})
	}
}

func TestDecodeAndValidateAtomicTransactionBatchV2_ArrayOrderChangesFingerprint(t *testing.T) {
	t.Parallel()

	first := validAtomicBatchV2Request("@source-one", "@destination-one", batchTestLedgerID)
	second := validAtomicBatchV2Request("@source-two", "@destination-two", batchTestLedgerID)
	first.Debits = append(first.Debits, validAtomicBatchV2Leg("@source-extra", batchTestLedgerID))
	first.Credits = append(first.Credits, validAtomicBatchV2Leg("@destination-extra", batchTestLedgerID))

	base := canonicalAtomicBatchV2Result(
		t,
		mustMarshalAtomicBatchV2Item(t, first),
		mustMarshalAtomicBatchV2Item(t, second),
	)

	transactionsReordered := canonicalAtomicBatchV2Result(
		t,
		mustMarshalAtomicBatchV2Item(t, second),
		mustMarshalAtomicBatchV2Item(t, first),
	)
	assert.NotEqual(t, base.requestFingerprint, transactionsReordered.requestFingerprint)

	debitsReordered := first
	debitsReordered.Debits = append([]TransactionV2LegRequest(nil), first.Debits...)
	debitsReordered.Debits[0], debitsReordered.Debits[1] = debitsReordered.Debits[1], debitsReordered.Debits[0]
	debitResult := canonicalAtomicBatchV2Result(
		t,
		mustMarshalAtomicBatchV2Item(t, debitsReordered),
		mustMarshalAtomicBatchV2Item(t, second),
	)
	assert.NotEqual(t, base.requestFingerprint, debitResult.requestFingerprint)

	creditsReordered := first
	creditsReordered.Credits = append([]TransactionV2LegRequest(nil), first.Credits...)
	creditsReordered.Credits[0], creditsReordered.Credits[1] = creditsReordered.Credits[1], creditsReordered.Credits[0]
	creditResult := canonicalAtomicBatchV2Result(
		t,
		mustMarshalAtomicBatchV2Item(t, creditsReordered),
		mustMarshalAtomicBatchV2Item(t, second),
	)
	assert.NotEqual(t, base.requestFingerprint, creditResult.requestFingerprint)
}

func TestDecodeAndValidateAtomicTransactionBatchV2_RejectsUnknownBeforeCanonicalization(t *testing.T) {
	t.Parallel()

	unknownItemField := []byte(`{
		"transactions":[{
			"asset":"USD",
			"amount":"1",
			"debits":[{"alias":"@source","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}],
			"credits":[{"alias":"@destination","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}],
			"unexpected":"rejected"
		}]
	}`)

	result, err := decodeAndValidateAtomicTransactionBatchV2(unknownItemField, 50)
	require.Error(t, err)
	assert.Empty(t, result.canonicalRequest)
	assert.Empty(t, result.requestFingerprint)

	var unknownErr pkg.ValidationUnknownFieldsError
	assert.ErrorAs(t, err, &unknownErr)
}

func canonicalAtomicBatchV2Result(t *testing.T, items ...json.RawMessage) decodedAtomicTransactionBatchV2 {
	t.Helper()

	result, err := decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, items...), 50)
	require.NoError(t, err)

	return result
}

func marshalJSONWithPermutedObjects(t *testing.T, value any, random *rand.Rand) []byte {
	t.Helper()

	var out bytes.Buffer
	writeJSONWithPermutedObjects(t, &out, value, random)

	return out.Bytes()
}

func writeJSONWithPermutedObjects(t *testing.T, out *bytes.Buffer, value any, random *rand.Rand) {
	t.Helper()

	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		random.Shuffle(len(keys), func(i, j int) {
			keys[i], keys[j] = keys[j], keys[i]
		})

		out.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				out.WriteByte(',')
			}
			encodedKey, err := json.Marshal(key)
			require.NoError(t, err)
			out.Write(encodedKey)
			out.WriteByte(':')
			writeJSONWithPermutedObjects(t, out, typed[key], random)
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				out.WriteByte(',')
			}
			writeJSONWithPermutedObjects(t, out, item, random)
		}
		out.WriteByte(']')
	default:
		encoded, err := json.Marshal(typed)
		require.NoError(t, err)
		out.Write(encoded)
	}
}
