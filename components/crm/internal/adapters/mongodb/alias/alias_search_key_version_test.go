// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
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

// mockSequencedVersionEncryptor returns a different key version on each successive
// GenerateSearchToken call (draining the configured sequence, then 0). It exercises the
// "stamp once / first-non-zero-wins" guard, which a fixed-version mock cannot reach.
type mockSequencedVersionEncryptor struct {
	versions []uint32
	calls    int
}

func (m *mockSequencedVersionEncryptor) EncryptField(_ context.Context, _ encryption.FieldContext, plaintext string) (string, error) {
	return "encrypted-" + plaintext, nil
}

func (m *mockSequencedVersionEncryptor) DecryptField(_ context.Context, _ encryption.FieldContext, ciphertext string) (string, error) {
	return ciphertext, nil
}

func (m *mockSequencedVersionEncryptor) GenerateSearchToken(_ context.Context, _ encryption.SearchTokenContext, _ string) (string, uint32, error) {
	var v uint32
	if m.calls < len(m.versions) {
		v = m.versions[m.calls]
	}

	m.calls++

	return "token", v, nil
}

func (m *mockSequencedVersionEncryptor) GenerateSearchTokenCandidates(_ context.Context, _ encryption.SearchTokenContext, _ string) ([]string, error) {
	return []string{"token"}, nil
}

func TestMongoDBModel_FromEntity_StampsSearchKeyVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	aliasID := uuid.New()
	relatedPartyID := uuid.New()

	// Alias exercises multiple GenerateSearchToken call sites (document, banking
	// account/iban, regulatory participant document, related parties); the version
	// must be captured once per document, not per field.
	newAlias := func() *mmodel.Alias {
		return &mmodel.Alias{
			ID:        &aliasID,
			Document:  testutils.Ptr("12345678901"),
			Type:      testutils.Ptr("NATURAL_PERSON"),
			LedgerID:  testutils.Ptr("ledger-1"),
			AccountID: testutils.Ptr("account-1"),
			BankingDetails: &mmodel.BankingDetails{
				Account: testutils.Ptr("123456"),
				IBAN:    testutils.Ptr("BR1234567890123456789012345"),
			},
			RegulatoryFields: &mmodel.RegulatoryFields{
				ParticipantDocument: testutils.Ptr("98765432109876"),
			},
			RelatedParties: []*mmodel.RelatedParty{
				{
					ID:        &relatedPartyID,
					Document:  "55566677788",
					Name:      "Related Person",
					Role:      "PRIMARY_HOLDER",
					StartDate: mmodel.Date{Time: now},
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	tests := []struct {
		name        string
		version     uint32
		wantVersion uint32
	}{
		{
			name:        "envelope write stamps non-zero version once per document",
			version:     9,
			wantVersion: 9,
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

			encryptionCtx := encryption.EncryptionContext{
				TenantID:       "default",
				OrganizationID: "test-org",
				RecordID:       aliasID.String(),
			}

			var model MongoDBModel
			err := model.FromEntity(ctx, newAlias(), fe, encryptionCtx)
			require.NoError(t, err)

			assert.Equal(t, tt.wantVersion, model.SearchKeyVersion)
		})
	}
}

// TestMongoDBModel_FromEntity_StampsSearchKeyVersion_FirstNonZeroWins verifies that
// across the alias's multiple tokenized fields the FIRST non-zero version observed is
// persisted and never overwritten by later fields. The sequenced encryptor returns
// 9 (document), then 5 (first banking field), then 0 (subsequent fields) so a regression
// that overwrote on every field would yield 5 or 0 instead of 9.
func TestMongoDBModel_FromEntity_StampsSearchKeyVersion_FirstNonZeroWins(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	aliasID := uuid.New()
	relatedPartyID := uuid.New()

	alias := &mmodel.Alias{
		ID:        &aliasID,
		Document:  testutils.Ptr("12345678901"),
		Type:      testutils.Ptr("NATURAL_PERSON"),
		LedgerID:  testutils.Ptr("ledger-1"),
		AccountID: testutils.Ptr("account-1"),
		BankingDetails: &mmodel.BankingDetails{
			Account: testutils.Ptr("123456"),
			IBAN:    testutils.Ptr("BR1234567890123456789012345"),
		},
		RegulatoryFields: &mmodel.RegulatoryFields{
			ParticipantDocument: testutils.Ptr("98765432109876"),
		},
		RelatedParties: []*mmodel.RelatedParty{
			{
				ID:        &relatedPartyID,
				Document:  "55566677788",
				Name:      "Related Person",
				Role:      "PRIMARY_HOLDER",
				StartDate: mmodel.Date{Time: now},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// document=9, first banking field=5, all later fields=0.
	fe := &mockSequencedVersionEncryptor{versions: []uint32{9, 5, 0}}

	encryptionCtx := encryption.EncryptionContext{
		TenantID:       "default",
		OrganizationID: "test-org",
		RecordID:       aliasID.String(),
	}

	var model MongoDBModel
	err := model.FromEntity(ctx, alias, fe, encryptionCtx)
	require.NoError(t, err)

	assert.Equal(t, uint32(9), model.SearchKeyVersion,
		"first non-zero version must be kept and never overwritten by later fields")
}
