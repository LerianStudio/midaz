// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package dupkey

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// dupKeyMessage builds a duplicate-key error message in the exact shape the
// MongoDB server returns (mongo-driver v2.6.0). Pinning the format here is the
// guard for the single sanctioned string-touch in dupkey.go: if a driver upgrade
// changes the wording, these fixtures must change too, surfacing the break.
func dupKeyMessage(collection, index, key, value string) string {
	return fmt.Sprintf(
		"E11000 duplicate key error collection: %s index: %s dup key: { %s: \"%s\" }",
		collection, index, key, value,
	)
}

func writeException(code int, message string) mongo.WriteException {
	return mongo.WriteException{
		WriteErrors: mongo.WriteErrors{
			{Index: 0, Code: code, Message: message},
		},
	}
}

func TestClassifyDuplicateKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantName string
		wantOK   bool
	}{
		{
			name:     "holder single document index",
			err:      writeException(11000, dupKeyMessage("crm.holders_org", "search.document_1", "search.document", "123")),
			wantName: "search.document_1",
			wantOK:   true,
		},
		{
			name:     "holder compound document+external_id index",
			err:      writeException(11000, dupKeyMessage("crm.holders_org", "search.document_1_external_id_1", "search.document", "123")),
			wantName: "search.document_1_external_id_1",
			wantOK:   true,
		},
		{
			name:     "instrument account_id index",
			err:      writeException(11000, dupKeyMessage("crm.instruments_org", "account_id_1", "account_id", "acc-1")),
			wantName: "account_id_1",
			wantOK:   true,
		},
		{
			name:     "instrument compound ledger_id+account_id index",
			err:      writeException(11000, dupKeyMessage("crm.instruments_org", "ledger_id_1_account_id_1", "account_id", "acc-1")),
			wantName: "ledger_id_1_account_id_1",
			wantOK:   true,
		},
		{
			name:     "raw _id collision (idempotency path)",
			err:      writeException(11000, dupKeyMessage("crm.holders_org", "_id_", "_id", "uuid-1")),
			wantName: "_id_",
			wantOK:   true,
		},
		{
			name:     "duplicate key on update code 11001",
			err:      writeException(11001, dupKeyMessage("crm.holders_org", "search.document_1", "search.document", "123")),
			wantName: "search.document_1",
			wantOK:   true,
		},
		{
			name:   "non-duplicate write error code",
			err:    writeException(121, "Document failed validation"),
			wantOK: false,
		},
		{
			name:   "non-mongo error",
			err:    errors.New("connection reset"),
			wantOK: false,
		},
		{
			name:   "nil error",
			err:    nil,
			wantOK: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			name, ok := ClassifyDuplicateKey(tc.err)
			require.Equal(t, tc.wantOK, ok)

			if tc.wantOK {
				require.Equal(t, tc.wantName, name)
			} else {
				require.Empty(t, name)
			}
		})
	}
}

func TestClassifyDuplicateKey_WrappedError(t *testing.T) {
	t.Parallel()

	inner := writeException(11000, dupKeyMessage("crm.instruments_org", "account_id_1", "account_id", "acc-1"))
	wrapped := fmt.Errorf("insert failed: %w", inner)

	name, ok := ClassifyDuplicateKey(wrapped)
	require.True(t, ok)
	require.Equal(t, "account_id_1", name)
}

func TestClassifyDuplicateKey_BulkWriteException(t *testing.T) {
	t.Parallel()

	err := mongo.BulkWriteException{
		WriteErrors: []mongo.BulkWriteError{
			{WriteError: mongo.WriteError{
				Index:   0,
				Code:    11000,
				Message: dupKeyMessage("crm.holders_org", "search.document_1", "search.document", "123"),
			}},
		},
	}

	name, ok := ClassifyDuplicateKey(err)
	require.True(t, ok)
	require.Equal(t, "search.document_1", name)
}

// TestClassifyDuplicateKey_PreservesRawIDForIdempotency documents the contract
// create-holder-with-id.go depends on: a raw _id collision is still reported by
// mongo.IsDuplicateKeyError, and ClassifyDuplicateKey returns the _id_ index so
// the adapter can choose NOT to convert it to a typed business error.
func TestClassifyDuplicateKey_PreservesRawIDForIdempotency(t *testing.T) {
	t.Parallel()

	err := writeException(11000, dupKeyMessage("crm.holders_org", "_id_", "_id", "uuid-1"))

	require.True(t, mongo.IsDuplicateKeyError(err), "raw _id collision must still satisfy IsDuplicateKeyError")

	name, ok := ClassifyDuplicateKey(err)
	require.True(t, ok)
	require.Equal(t, "_id_", name, "adapter inspects the name and must treat _id_ as not-business-classified")
}
