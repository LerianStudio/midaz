// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package instrument

import (
	"context"
	"testing"
	"time"

	encryption "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestFieldEncryptor creates a FieldEncryptorAdapter wrapping an EncryptionService
// with lib-commons crypto for testing. This matches production behavior when KMS vendor is none.
func setupTestFieldEncryptor(t *testing.T) encryption.FieldEncryptor {
	t.Helper()

	// Use lib-commons crypto directly, matching the production path when KMS vendor is none
	crypto := testutils.SetupCrypto(t)

	resolver := encryption.NewProtectionStateResolver(nil, encryption.NewProtectionMetrics(nil))
	svc := encryption.NewEncryptionService(resolver, nil, nil, crypto, encryption.NewProtectionMetrics(nil))

	return encryption.NewFieldEncryptorAdapter(svc)
}

// fixedTestTime is a deterministic, second-aligned UTC timestamp used across
// instrument adapter tests so setup and assertions share the same value.
var fixedTestTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// testEncryptionContext returns a standard encryption context for tests.
func testEncryptionContext(instrumentID uuid.UUID) encryption.EncryptionContext {
	return encryption.EncryptionContext{
		TenantID:       "default",
		OrganizationID: "test-org",
		RecordID:       instrumentID.String(),
	}
}

func TestMongoDBModel_FromEntity(t *testing.T) {
	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	now := fixedTestTime
	instrumentID := uuid.New()
	holderID := uuid.New()

	tests := []struct {
		name    string
		alias   *mmodel.Instrument
		wantErr bool
	}{
		{
			name: "minimal alias",
			alias: &mmodel.Instrument{
				ID:        &instrumentID,
				Document:  testutils.Ptr("12345678901"),
				Type:      testutils.Ptr("NATURAL_PERSON"),
				LedgerID:  testutils.Ptr("ledger-123"),
				AccountID: testutils.Ptr("account-456"),
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "alias with holder",
			alias: &mmodel.Instrument{
				ID:        &instrumentID,
				Document:  testutils.Ptr("98765432100"),
				Type:      testutils.Ptr("LEGAL_PERSON"),
				LedgerID:  testutils.Ptr("ledger-789"),
				AccountID: testutils.Ptr("account-012"),
				HolderID:  &holderID,
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "alias with banking details",
			alias: &mmodel.Instrument{
				ID:        &instrumentID,
				Document:  testutils.Ptr("11122233344"),
				Type:      testutils.Ptr("NATURAL_PERSON"),
				LedgerID:  testutils.Ptr("ledger-456"),
				AccountID: testutils.Ptr("account-789"),
				BankingDetails: &mmodel.BankingDetails{
					Branch:      testutils.Ptr("0001"),
					Account:     testutils.Ptr("123456"),
					Type:        testutils.Ptr("CACC"),
					OpeningDate: testutils.Ptr("2025-01-01"),
					IBAN:        testutils.Ptr("BR1234567890123456789012345"),
					CountryCode: testutils.Ptr("BR"),
					BankID:      testutils.Ptr("001"),
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "alias with metadata",
			alias: &mmodel.Instrument{
				ID:        &instrumentID,
				Document:  testutils.Ptr("55566677788"),
				Type:      testutils.Ptr("NATURAL_PERSON"),
				LedgerID:  testutils.Ptr("ledger-meta"),
				AccountID: testutils.Ptr("account-meta"),
				Metadata: map[string]any{
					"key1": "value1",
					"key2": 123,
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "alias with nil metadata initializes empty map",
			alias: &mmodel.Instrument{
				ID:        &instrumentID,
				Document:  testutils.Ptr("77788899900"),
				Type:      testutils.Ptr("LEGAL_PERSON"),
				LedgerID:  testutils.Ptr("ledger-nil"),
				AccountID: testutils.Ptr("account-nil"),
				Metadata:  nil,
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "alias with regulatory fields",
			alias: &mmodel.Instrument{
				ID:        &instrumentID,
				Document:  testutils.Ptr("99900011122"),
				Type:      testutils.Ptr("NATURAL_PERSON"),
				LedgerID:  testutils.Ptr("ledger-part"),
				AccountID: testutils.Ptr("account-part"),
				RegulatoryFields: &mmodel.RegulatoryFields{
					ParticipantDocument: testutils.Ptr("12345678901234"),
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "alias with related parties",
			alias: &mmodel.Instrument{
				ID:        &instrumentID,
				Document:  testutils.Ptr("88877766655"),
				Type:      testutils.Ptr("NATURAL_PERSON"),
				LedgerID:  testutils.Ptr("ledger-rp"),
				AccountID: testutils.Ptr("account-rp"),
				RelatedParties: []*mmodel.RelatedParty{
					{
						ID:        &holderID,
						Document:  "11122233344",
						Name:      "Related Person 1",
						Role:      "PRIMARY_HOLDER",
						StartDate: mmodel.Date{Time: now},
					},
					{
						ID:        &instrumentID,
						Document:  "55566677788",
						Name:      "Related Person 2",
						Role:      "LEGAL_REPRESENTATIVE",
						StartDate: mmodel.Date{Time: now},
						EndDate:   &mmodel.Date{Time: now},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "alias with closing date",
			alias: &mmodel.Instrument{
				ID:        &instrumentID,
				Document:  testutils.Ptr("44455566677"),
				Type:      testutils.Ptr("NATURAL_PERSON"),
				LedgerID:  testutils.Ptr("ledger-close"),
				AccountID: testutils.Ptr("account-close"),
				BankingDetails: &mmodel.BankingDetails{
					ClosingDate: &mmodel.Date{Time: now},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "alias with all fields",
			alias: &mmodel.Instrument{
				ID:        &instrumentID,
				Document:  testutils.Ptr("11111111111"),
				Type:      testutils.Ptr("LEGAL_PERSON"),
				LedgerID:  testutils.Ptr("ledger-full"),
				AccountID: testutils.Ptr("account-full"),
				HolderID:  &holderID,
				BankingDetails: &mmodel.BankingDetails{
					Branch:      testutils.Ptr("9999"),
					Account:     testutils.Ptr("999999"),
					Type:        testutils.Ptr("SVGS"),
					OpeningDate: testutils.Ptr("2020-06-15"),
					ClosingDate: &mmodel.Date{Time: now},
					IBAN:        testutils.Ptr("US12345678901234567890"),
					CountryCode: testutils.Ptr("US"),
					BankID:      testutils.Ptr("BANK123"),
				},
				Metadata: map[string]any{
					"complete": true,
				},
				RegulatoryFields: &mmodel.RegulatoryFields{
					ParticipantDocument: testutils.Ptr("98765432109876"),
				},
				RelatedParties: []*mmodel.RelatedParty{
					{
						ID:        &holderID,
						Document:  "99988877766",
						Name:      "Full Test Party",
						Role:      "RESPONSIBLE_PARTY",
						StartDate: mmodel.Date{Time: now},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
				DeletedAt: &now,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encryptionCtx := testEncryptionContext(*tt.alias.ID)

			var model MongoDBModel
			err := model.FromEntity(ctx, tt.alias, fe, encryptionCtx)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.alias.ID, model.ID)
			assert.Equal(t, tt.alias.Type, model.Type)
			assert.Equal(t, tt.alias.LedgerID, model.LedgerID)
			assert.Equal(t, tt.alias.AccountID, model.AccountID)
			assert.Equal(t, tt.alias.HolderID, model.HolderID)

			// Encrypted fields should not match original
			if tt.alias.Document != nil {
				assert.NotNil(t, model.Document)
				assert.NotEqual(t, *tt.alias.Document, *model.Document, "Document should be encrypted")
			}

			// Verify regulatory fields encryption
			if tt.alias.RegulatoryFields != nil && tt.alias.RegulatoryFields.ParticipantDocument != nil {
				require.NotNil(t, model.RegulatoryFields)
				assert.NotNil(t, model.RegulatoryFields.ParticipantDocument)
				assert.NotEqual(t, *tt.alias.RegulatoryFields.ParticipantDocument, *model.RegulatoryFields.ParticipantDocument, "ParticipantDocument should be encrypted")

				// Verify search hash for participant document
				if *tt.alias.RegulatoryFields.ParticipantDocument != "" {
					require.NotNil(t, model.Search)
					assert.NotNil(t, model.Search.RegulatoryFieldsParticipantDocument, "ParticipantDocument hash should be generated")
				}
			}

			// Verify related parties encryption
			if len(tt.alias.RelatedParties) > 0 {
				require.Len(t, model.RelatedParties, len(tt.alias.RelatedParties))
				require.NotNil(t, model.Search)
				assert.Len(t, model.Search.RelatedPartyDocuments, len(tt.alias.RelatedParties), "Should have hash for each related party")

				for i, rp := range tt.alias.RelatedParties {
					assert.NotNil(t, model.RelatedParties[i].Document)
					assert.NotEqual(t, rp.Document, *model.RelatedParties[i].Document, "RelatedParty document should be encrypted")
					assert.Equal(t, rp.Name, model.RelatedParties[i].Name)
					assert.Equal(t, rp.Role, model.RelatedParties[i].Role)
				}
			}

			// Verify search hash is generated for document
			if tt.alias.Document != nil && *tt.alias.Document != "" {
				require.NotNil(t, model.Search)
				assert.NotNil(t, model.Search.Document, "Document hash should be generated")
			}

			// Verify banking details encryption
			if tt.alias.BankingDetails != nil {
				require.NotNil(t, model.BankingDetails)

				if tt.alias.BankingDetails.Account != nil {
					assert.NotNil(t, model.BankingDetails.Account)
					assert.NotEqual(t, *tt.alias.BankingDetails.Account, *model.BankingDetails.Account, "Account should be encrypted")
				}

				if tt.alias.BankingDetails.IBAN != nil {
					assert.NotNil(t, model.BankingDetails.IBAN)
					assert.NotEqual(t, *tt.alias.BankingDetails.IBAN, *model.BankingDetails.IBAN, "IBAN should be encrypted")
				}

				// Non-encrypted banking fields should match
				assert.Equal(t, tt.alias.BankingDetails.Branch, model.BankingDetails.Branch)
				assert.Equal(t, tt.alias.BankingDetails.Type, model.BankingDetails.Type)
				assert.Equal(t, tt.alias.BankingDetails.OpeningDate, model.BankingDetails.OpeningDate)
				assert.Equal(t, tt.alias.BankingDetails.CountryCode, model.BankingDetails.CountryCode)
				assert.Equal(t, tt.alias.BankingDetails.BankID, model.BankingDetails.BankID)

				// Verify search hashes for banking details
				require.NotNil(t, model.Search)
				if tt.alias.BankingDetails.Account != nil && *tt.alias.BankingDetails.Account != "" {
					assert.NotNil(t, model.Search.BankingDetailsAccount, "Account hash should be generated")
				}
				if tt.alias.BankingDetails.IBAN != nil && *tt.alias.BankingDetails.IBAN != "" {
					assert.NotNil(t, model.Search.BankingDetailsIBAN, "IBAN hash should be generated")
				}
			}

			// Verify metadata
			assert.NotNil(t, model.Metadata, "Metadata should never be nil")
		})
	}
}

func TestMongoDBModel_ToEntity(t *testing.T) {
	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	now := fixedTestTime
	instrumentID := uuid.New()
	holderID := uuid.New()
	relatedPartyID := uuid.New()

	// First create a model from an entity, then convert back
	originalAlias := &mmodel.Instrument{
		ID:        &instrumentID,
		Document:  testutils.Ptr("33344455566"),
		Type:      testutils.Ptr("NATURAL_PERSON"),
		LedgerID:  testutils.Ptr("ledger-roundtrip"),
		AccountID: testutils.Ptr("account-roundtrip"),
		HolderID:  &holderID,
		BankingDetails: &mmodel.BankingDetails{
			Branch:      testutils.Ptr("1234"),
			Account:     testutils.Ptr("567890"),
			Type:        testutils.Ptr("CACC"),
			OpeningDate: testutils.Ptr("2023-06-15"),
			ClosingDate: &mmodel.Date{Time: now},
			IBAN:        testutils.Ptr("BR9876543210987654321098765"),
			CountryCode: testutils.Ptr("BR"),
			BankID:      testutils.Ptr("341"),
		},
		Metadata: map[string]any{
			"testKey": "testValue",
		},
		RegulatoryFields: &mmodel.RegulatoryFields{
			ParticipantDocument: testutils.Ptr("11223344556677"),
		},
		RelatedParties: []*mmodel.RelatedParty{
			{
				ID:        &relatedPartyID,
				Document:  "99988877766",
				Name:      "Related Test Party",
				Role:      "PRIMARY_HOLDER",
				StartDate: mmodel.Date{Time: now},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	encryptionCtx := testEncryptionContext(instrumentID)

	var model MongoDBModel
	err := model.FromEntity(ctx, originalAlias, fe, encryptionCtx)
	require.NoError(t, err)

	// Now convert back to entity
	resultAlias, err := model.ToEntity(ctx, fe, encryptionCtx)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, originalAlias.ID, resultAlias.ID)
	assert.Equal(t, *originalAlias.Document, *resultAlias.Document)
	assert.Equal(t, originalAlias.Type, resultAlias.Type)
	assert.Equal(t, originalAlias.LedgerID, resultAlias.LedgerID)
	assert.Equal(t, originalAlias.AccountID, resultAlias.AccountID)
	assert.Equal(t, originalAlias.HolderID, resultAlias.HolderID)
	assert.Equal(t, originalAlias.Metadata, resultAlias.Metadata)
	assert.Equal(t, originalAlias.CreatedAt, resultAlias.CreatedAt)
	assert.Equal(t, originalAlias.UpdatedAt, resultAlias.UpdatedAt)

	// Verify regulatory fields (decrypted)
	require.NotNil(t, resultAlias.RegulatoryFields)
	assert.Equal(t, *originalAlias.RegulatoryFields.ParticipantDocument, *resultAlias.RegulatoryFields.ParticipantDocument)

	// Verify related parties (decrypted)
	require.Len(t, resultAlias.RelatedParties, 1)
	assert.Equal(t, originalAlias.RelatedParties[0].ID, resultAlias.RelatedParties[0].ID)
	assert.Equal(t, originalAlias.RelatedParties[0].Document, resultAlias.RelatedParties[0].Document)
	assert.Equal(t, originalAlias.RelatedParties[0].Name, resultAlias.RelatedParties[0].Name)
	assert.Equal(t, originalAlias.RelatedParties[0].Role, resultAlias.RelatedParties[0].Role)
	assert.Equal(t, originalAlias.RelatedParties[0].StartDate, resultAlias.RelatedParties[0].StartDate)

	// Verify banking details (decrypted)
	require.NotNil(t, resultAlias.BankingDetails)
	assert.Equal(t, originalAlias.BankingDetails.Branch, resultAlias.BankingDetails.Branch)
	assert.Equal(t, *originalAlias.BankingDetails.Account, *resultAlias.BankingDetails.Account)
	assert.Equal(t, originalAlias.BankingDetails.Type, resultAlias.BankingDetails.Type)
	assert.Equal(t, originalAlias.BankingDetails.OpeningDate, resultAlias.BankingDetails.OpeningDate)
	assert.Equal(t, originalAlias.BankingDetails.ClosingDate.Time, resultAlias.BankingDetails.ClosingDate.Time)
	assert.Equal(t, *originalAlias.BankingDetails.IBAN, *resultAlias.BankingDetails.IBAN)
	assert.Equal(t, originalAlias.BankingDetails.CountryCode, resultAlias.BankingDetails.CountryCode)
	assert.Equal(t, originalAlias.BankingDetails.BankID, resultAlias.BankingDetails.BankID)
}

func TestMongoDBModel_ToEntity_NilBankingDetails(t *testing.T) {
	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	now := fixedTestTime
	instrumentID := uuid.New()

	originalAlias := &mmodel.Instrument{
		ID:             &instrumentID,
		Document:       testutils.Ptr("99988877766"),
		Type:           testutils.Ptr("LEGAL_PERSON"),
		LedgerID:       testutils.Ptr("ledger-no-bank"),
		AccountID:      testutils.Ptr("account-no-bank"),
		BankingDetails: nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	encryptionCtx := testEncryptionContext(instrumentID)

	var model MongoDBModel
	err := model.FromEntity(ctx, originalAlias, fe, encryptionCtx)
	require.NoError(t, err)

	resultAlias, err := model.ToEntity(ctx, fe, encryptionCtx)
	require.NoError(t, err)

	assert.Equal(t, *originalAlias.Document, *resultAlias.Document)
	assert.Nil(t, resultAlias.BankingDetails)
}

func TestMongoDBModel_ToEntity_WithDeletedAt(t *testing.T) {
	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	now := fixedTestTime
	instrumentID := uuid.New()

	originalAlias := &mmodel.Instrument{
		ID:        &instrumentID,
		Document:  testutils.Ptr("66677788899"),
		Type:      testutils.Ptr("NATURAL_PERSON"),
		LedgerID:  testutils.Ptr("ledger-deleted"),
		AccountID: testutils.Ptr("account-deleted"),
		CreatedAt: now,
		UpdatedAt: now,
		DeletedAt: &now,
	}

	encryptionCtx := testEncryptionContext(instrumentID)

	var model MongoDBModel
	err := model.FromEntity(ctx, originalAlias, fe, encryptionCtx)
	require.NoError(t, err)

	resultAlias, err := model.ToEntity(ctx, fe, encryptionCtx)
	require.NoError(t, err)

	require.NotNil(t, resultAlias.DeletedAt)
	assert.Equal(t, *originalAlias.DeletedAt, *resultAlias.DeletedAt)
}

func TestMongoDBModel_ToEntity_NilRegulatoryFieldsAndRelatedParties(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	now := fixedTestTime
	instrumentID := uuid.New()

	originalAlias := &mmodel.Instrument{
		ID:               &instrumentID,
		Document:         testutils.Ptr("55544433322"),
		Type:             testutils.Ptr("NATURAL_PERSON"),
		LedgerID:         testutils.Ptr("ledger-no-extras"),
		AccountID:        testutils.Ptr("account-no-extras"),
		RegulatoryFields: nil,
		RelatedParties:   nil,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	encryptionCtx := testEncryptionContext(instrumentID)

	var model MongoDBModel
	err := model.FromEntity(ctx, originalAlias, fe, encryptionCtx)
	require.NoError(t, err)

	resultAlias, err := model.ToEntity(ctx, fe, encryptionCtx)
	require.NoError(t, err)

	assert.Equal(t, *originalAlias.Document, *resultAlias.Document)
	assert.Nil(t, resultAlias.RegulatoryFields)
	assert.Empty(t, resultAlias.RelatedParties)
}

func TestMongoDBModel_FromEntity_RoundTrip_NilOptionalEncryptedFields(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	now := fixedTestTime
	instrumentID := uuid.New()
	holderID := uuid.New()

	originalAlias := &mmodel.Instrument{
		ID:        &instrumentID,
		Document:  testutils.Ptr("12312312399"),
		Type:      testutils.Ptr("NATURAL_PERSON"),
		LedgerID:  testutils.Ptr("ledger-nil-optionals"),
		AccountID: testutils.Ptr("account-nil-optionals"),
		HolderID:  &holderID,
		BankingDetails: &mmodel.BankingDetails{
			Branch: testutils.Ptr("0001"),
			Type:   testutils.Ptr("CACC"),
			// Account and IBAN intentionally nil.
		},
		RegulatoryFields: &mmodel.RegulatoryFields{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	encryptionCtx := testEncryptionContext(instrumentID)

	var model MongoDBModel
	err := model.FromEntity(ctx, originalAlias, fe, encryptionCtx)
	require.NoError(t, err)

	require.NotNil(t, model.BankingDetails)
	assert.Nil(t, model.BankingDetails.Account)
	assert.Nil(t, model.BankingDetails.IBAN)
	require.NotNil(t, model.RegulatoryFields)
	assert.Nil(t, model.RegulatoryFields.ParticipantDocument)
	require.NotNil(t, model.Search)
	assert.Nil(t, model.Search.BankingDetailsAccount)
	assert.Nil(t, model.Search.BankingDetailsIBAN)
	assert.Nil(t, model.Search.RegulatoryFieldsParticipantDocument)

	resultAlias, err := model.ToEntity(ctx, fe, encryptionCtx)
	require.NoError(t, err)

	require.NotNil(t, resultAlias.BankingDetails)
	assert.Nil(t, resultAlias.BankingDetails.Account)
	assert.Nil(t, resultAlias.BankingDetails.IBAN)
	require.NotNil(t, resultAlias.RegulatoryFields)
	assert.Nil(t, resultAlias.RegulatoryFields.ParticipantDocument)
}

func TestMongoDBModel_ToEntity_InvalidOptionalCiphertextReturnsError(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	instrumentID := uuid.New()

	// Encrypt a valid document using the FieldEncryptor
	encryptionCtx := testEncryptionContext(instrumentID)
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
		FieldName:      "document",
	}
	encryptedDoc, err := fe.EncryptField(ctx, fieldCtx, "12345678901")
	require.NoError(t, err)

	// Build a model with valid encrypted document but invalid banking details ciphertext
	model := &MongoDBModel{
		ID:       &instrumentID,
		Document: &encryptedDoc,
		BankingDetails: &BankingMongoDBModel{
			Account: testutils.Ptr("not-a-valid-ciphertext"),
		},
	}

	_, err = model.ToEntity(ctx, fe, encryptionCtx)
	require.Error(t, err)
}

// failingFieldEncryptor is a mock that fails on encryption to test error paths.
type failingFieldEncryptor struct {
	failOnEncrypt bool
	failOnDecrypt bool
}

func (f *failingFieldEncryptor) EncryptField(_ context.Context, _ encryption.FieldContext, _ string) (string, error) {
	if f.failOnEncrypt {
		return "", assert.AnError
	}
	return "encrypted", nil
}

func (f *failingFieldEncryptor) DecryptField(_ context.Context, _ encryption.FieldContext, _ string) (string, error) {
	if f.failOnDecrypt {
		return "", assert.AnError
	}
	return "decrypted", nil
}

func (f *failingFieldEncryptor) GenerateSearchToken(_ context.Context, _ encryption.SearchTokenContext, _ string) (string, uint32, error) {
	return "token", 0, nil
}

func (f *failingFieldEncryptor) GenerateSearchTokenCandidates(_ context.Context, _ encryption.SearchTokenContext, _ string) ([]string, error) {
	return []string{"token1", "token2"}, nil
}

func TestMongoDBModel_FromEntity_EncryptOptionalFailureReturnsError(t *testing.T) {
	t.Parallel()

	// Use mock that returns errors to test encryption failure path
	fe := &failingFieldEncryptor{failOnEncrypt: true}
	ctx := context.Background()
	instrumentID := uuid.New()
	now := fixedTestTime

	alias := &mmodel.Instrument{
		ID:        &instrumentID,
		Document:  testutils.Ptr("12345678901"),
		Type:      testutils.Ptr("NATURAL_PERSON"),
		LedgerID:  testutils.Ptr("ledger-1"),
		AccountID: testutils.Ptr("account-1"),
		BankingDetails: &mmodel.BankingDetails{
			Account: testutils.Ptr(""),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	encryptionCtx := testEncryptionContext(instrumentID)

	var model MongoDBModel
	err := model.FromEntity(ctx, alias, fe, encryptionCtx)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Related Party AAD Binding Tests (ID-based, not index-based)
// ---------------------------------------------------------------------------

// TestRelatedPartyAAD_UsesIDNotIndex verifies that related party encryption
// binds AAD to the related party ID, not its array index. This ensures that
// deleting a related party (which shifts array indexes) does not break
// decryption of remaining related parties.
func TestRelatedPartyAAD_UsesIDNotIndex(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	now := fixedTestTime
	instrumentID := uuid.New()

	// Create three related parties with distinct IDs
	rpID1 := uuid.New()
	rpID2 := uuid.New()
	rpID3 := uuid.New()

	parties := []*mmodel.RelatedParty{
		{
			ID:        &rpID1,
			Document:  "11111111111",
			Name:      "Party One",
			Role:      "PRIMARY_HOLDER",
			StartDate: mmodel.Date{Time: now},
		},
		{
			ID:        &rpID2,
			Document:  "22222222222",
			Name:      "Party Two",
			Role:      "LEGAL_REPRESENTATIVE",
			StartDate: mmodel.Date{Time: now},
		},
		{
			ID:        &rpID3,
			Document:  "33333333333",
			Name:      "Party Three",
			Role:      "RESPONSIBLE_PARTY",
			StartDate: mmodel.Date{Time: now},
		},
	}

	encryptionCtx := encryption.EncryptionContext{
		TenantID:       "default",
		OrganizationID: "test-org",
		RecordID:       instrumentID.String(),
	}

	// Encrypt all three related parties
	encryptedModels, _, _, err := mapRelatedPartiesFromEntity(ctx, fe, encryptionCtx, parties)
	require.NoError(t, err)
	require.Len(t, encryptedModels, 3)

	// Verify each encrypted document is different (unique AAD per party)
	assert.NotEqual(t, *encryptedModels[0].Document, *encryptedModels[1].Document)
	assert.NotEqual(t, *encryptedModels[1].Document, *encryptedModels[2].Document)

	// SIMULATE DELETE: Remove the middle related party (index 1)
	// This is what happens when MongoDB $pull removes an element
	remainingModels := []*RelatedPartyMongoDBModel{
		encryptedModels[0], // Was index 0, stays index 0
		encryptedModels[2], // Was index 2, NOW index 1 (shifted!)
	}

	// Decrypt remaining parties - should succeed because AAD uses ID, not index
	decryptedParties, err := mapRelatedPartiesToEntity(ctx, fe, encryptionCtx, remainingModels)
	require.NoError(t, err, "decryption should succeed despite index shift because AAD is bound to ID")
	require.Len(t, decryptedParties, 2)

	// Verify decrypted values match originals
	assert.Equal(t, "11111111111", decryptedParties[0].Document, "Party One should decrypt correctly")
	assert.Equal(t, "Party One", decryptedParties[0].Name)

	assert.Equal(t, "33333333333", decryptedParties[1].Document, "Party Three should decrypt correctly despite index shift")
	assert.Equal(t, "Party Three", decryptedParties[1].Name)
}

// TestRelatedPartyAAD_DifferentIDsProduceDifferentCiphertexts verifies that
// related parties with the same document but different IDs produce different
// ciphertexts due to ID-based AAD binding.
func TestRelatedPartyAAD_DifferentIDsProduceDifferentCiphertexts(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	now := fixedTestTime
	instrumentID := uuid.New()

	// Two related parties with SAME document but DIFFERENT IDs
	rpID1 := uuid.New()
	rpID2 := uuid.New()
	sameDocument := "99999999999"

	parties := []*mmodel.RelatedParty{
		{
			ID:        &rpID1,
			Document:  sameDocument,
			Name:      "Party With ID 1",
			Role:      "PRIMARY_HOLDER",
			StartDate: mmodel.Date{Time: now},
		},
		{
			ID:        &rpID2,
			Document:  sameDocument, // Same document value
			Name:      "Party With ID 2",
			Role:      "LEGAL_REPRESENTATIVE",
			StartDate: mmodel.Date{Time: now},
		},
	}

	encryptionCtx := encryption.EncryptionContext{
		TenantID:       "default",
		OrganizationID: "test-org",
		RecordID:       instrumentID.String(),
	}

	encryptedModels, _, _, err := mapRelatedPartiesFromEntity(ctx, fe, encryptionCtx, parties)
	require.NoError(t, err)
	require.Len(t, encryptedModels, 2)

	// Even though document values are identical, ciphertexts should differ
	// because AAD includes the related party ID
	assert.NotEqual(t, *encryptedModels[0].Document, *encryptedModels[1].Document,
		"same plaintext with different IDs should produce different ciphertexts due to ID-based AAD")
}
