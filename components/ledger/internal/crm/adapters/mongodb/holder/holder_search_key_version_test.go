// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFieldEncryptorVersion implements FieldEncryptor returning a configurable
// search-token key version, used to assert SearchKeyVersion stamping.
type mockFieldEncryptorVersion struct {
	version uint32
}

func (m *mockFieldEncryptorVersion) EncryptField(_ context.Context, _ encryption.FieldContext, plaintext string) (string, error) {
	return "encrypted-" + plaintext, nil
}

func (m *mockFieldEncryptorVersion) DecryptField(_ context.Context, _ encryption.FieldContext, ciphertext string) (string, error) {
	return ciphertext, nil
}

func (m *mockFieldEncryptorVersion) GenerateSearchToken(_ context.Context, _ encryption.SearchTokenContext, _ string) (string, uint32, error) {
	return "token", m.version, nil
}

func (m *mockFieldEncryptorVersion) GenerateSearchTokenCandidates(_ context.Context, _ encryption.SearchTokenContext, _ string) ([]string, error) {
	return []string{"token"}, nil
}

func TestMongoDBModel_FromEntity_StampsSearchKeyVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	holderID := uuid.New()

	tests := []struct {
		name        string
		version     uint32
		wantVersion uint32
	}{
		{
			name:        "envelope write stamps non-zero version",
			version:     7,
			wantVersion: 7,
		},
		{
			name:        "legacy write leaves version unset",
			version:     0,
			wantVersion: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fe := &mockFieldEncryptorVersion{version: tt.version}

			holder := &mmodel.Holder{
				ID:        &holderID,
				Type:      testutils.Ptr("NATURAL_PERSON"),
				Name:      testutils.Ptr("John Doe"),
				Document:  testutils.Ptr("12345678901"),
				CreatedAt: now,
				UpdatedAt: now,
			}

			encryptionCtx := testEncryptionContext(holderID.String())

			var model MongoDBModel
			err := model.FromEntity(ctx, holder, fe, encryptionCtx)
			require.NoError(t, err)

			assert.Equal(t, tt.wantVersion, model.SearchKeyVersion)
		})
	}
}
