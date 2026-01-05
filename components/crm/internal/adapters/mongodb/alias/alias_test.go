package alias

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/testutils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMongoDBModel_FromEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	aliasID := uuid.New()
	holderID := uuid.New()

	tests := []struct {
		name    string
		alias   *mmodel.Alias
		wantErr bool
	}{
		{
			name: "minimal alias",
			alias: &mmodel.Alias{
				ID:        &aliasID,
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
			alias: &mmodel.Alias{
				ID:        &aliasID,
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
			alias: &mmodel.Alias{
				ID:        &aliasID,
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
			alias: &mmodel.Alias{
				ID:        &aliasID,
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
			alias: &mmodel.Alias{
				ID:        &aliasID,
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
			name: "alias with participant document",
			alias: &mmodel.Alias{
				ID:                  &aliasID,
				Document:            testutils.Ptr("99900011122"),
				Type:                testutils.Ptr("NATURAL_PERSON"),
				LedgerID:            testutils.Ptr("ledger-part"),
				AccountID:           testutils.Ptr("account-part"),
				ParticipantDocument: testutils.Ptr("12345678901234"),
				CreatedAt:           now,
				UpdatedAt:           now,
			},
			wantErr: false,
		},
		{
			name: "alias with closing date",
			alias: &mmodel.Alias{
				ID:          &aliasID,
				Document:    testutils.Ptr("44455566677"),
				Type:        testutils.Ptr("NATURAL_PERSON"),
				LedgerID:    testutils.Ptr("ledger-close"),
				AccountID:   testutils.Ptr("account-close"),
				ClosingDate: &now,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			wantErr: false,
		},
		{
			name: "alias with all fields",
			alias: &mmodel.Alias{
				ID:        &aliasID,
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
					IBAN:        testutils.Ptr("US12345678901234567890"),
					CountryCode: testutils.Ptr("US"),
					BankID:      testutils.Ptr("BANK123"),
				},
				Metadata: map[string]any{
					"complete": true,
				},
				ParticipantDocument: testutils.Ptr("98765432109876"),
				ClosingDate:         &now,
				CreatedAt:           now,
				UpdatedAt:           now,
				DeletedAt:           &now,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var model MongoDBModel
			err := model.FromEntity(tt.alias, crypto)

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
			assert.Equal(t, tt.alias.ClosingDate, model.ClosingDate)

			// Encrypted fields should not match original
			if tt.alias.Document != nil {
				assert.NotNil(t, model.Document)
				assert.NotEqual(t, *tt.alias.Document, *model.Document, "Document should be encrypted")
			}

			if tt.alias.ParticipantDocument != nil {
				assert.NotNil(t, model.ParticipantDocument)
				assert.NotEqual(t, *tt.alias.ParticipantDocument, *model.ParticipantDocument, "ParticipantDocument should be encrypted")
			}

			// Verify search hash is generated for document
			if tt.alias.Document != nil && *tt.alias.Document != "" {
				assert.NotEmpty(t, model.Search["document"], "Document hash should be generated")
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
				if tt.alias.BankingDetails.Account != nil && *tt.alias.BankingDetails.Account != "" {
					assert.NotEmpty(t, model.Search["banking_details_account"], "Account hash should be generated")
				}
				if tt.alias.BankingDetails.IBAN != nil && *tt.alias.BankingDetails.IBAN != "" {
					assert.NotEmpty(t, model.Search["banking_details_iban"], "IBAN hash should be generated")
				}
			}

			// Verify metadata
			assert.NotNil(t, model.Metadata, "Metadata should never be nil")
		})
	}
}

func TestMongoDBModel_ToEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	aliasID := uuid.New()
	holderID := uuid.New()

	// First create a model from an entity, then convert back
	originalAlias := &mmodel.Alias{
		ID:        &aliasID,
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
			IBAN:        testutils.Ptr("BR9876543210987654321098765"),
			CountryCode: testutils.Ptr("BR"),
			BankID:      testutils.Ptr("341"),
		},
		Metadata: map[string]any{
			"testKey": "testValue",
		},
		ParticipantDocument: testutils.Ptr("11223344556677"),
		ClosingDate:         &now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	var model MongoDBModel
	err := model.FromEntity(originalAlias, crypto)
	require.NoError(t, err)

	// Now convert back to entity
	resultAlias, err := model.ToEntity(crypto)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, originalAlias.ID, resultAlias.ID)
	assert.Equal(t, *originalAlias.Document, *resultAlias.Document)
	assert.Equal(t, originalAlias.Type, resultAlias.Type)
	assert.Equal(t, originalAlias.LedgerID, resultAlias.LedgerID)
	assert.Equal(t, originalAlias.AccountID, resultAlias.AccountID)
	assert.Equal(t, originalAlias.HolderID, resultAlias.HolderID)
	assert.Equal(t, originalAlias.Metadata, resultAlias.Metadata)
	assert.Equal(t, *originalAlias.ParticipantDocument, *resultAlias.ParticipantDocument)
	assert.Equal(t, originalAlias.ClosingDate, resultAlias.ClosingDate)
	assert.Equal(t, originalAlias.CreatedAt, resultAlias.CreatedAt)
	assert.Equal(t, originalAlias.UpdatedAt, resultAlias.UpdatedAt)

	// Verify banking details (decrypted)
	require.NotNil(t, resultAlias.BankingDetails)
	assert.Equal(t, originalAlias.BankingDetails.Branch, resultAlias.BankingDetails.Branch)
	assert.Equal(t, *originalAlias.BankingDetails.Account, *resultAlias.BankingDetails.Account)
	assert.Equal(t, originalAlias.BankingDetails.Type, resultAlias.BankingDetails.Type)
	assert.Equal(t, originalAlias.BankingDetails.OpeningDate, resultAlias.BankingDetails.OpeningDate)
	assert.Equal(t, *originalAlias.BankingDetails.IBAN, *resultAlias.BankingDetails.IBAN)
	assert.Equal(t, originalAlias.BankingDetails.CountryCode, resultAlias.BankingDetails.CountryCode)
	assert.Equal(t, originalAlias.BankingDetails.BankID, resultAlias.BankingDetails.BankID)
}

func TestMongoDBModel_ToEntity_NilBankingDetails(t *testing.T) {
	crypto := testutils.SetupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	aliasID := uuid.New()

	originalAlias := &mmodel.Alias{
		ID:             &aliasID,
		Document:       testutils.Ptr("99988877766"),
		Type:           testutils.Ptr("LEGAL_PERSON"),
		LedgerID:       testutils.Ptr("ledger-no-bank"),
		AccountID:      testutils.Ptr("account-no-bank"),
		BankingDetails: nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	var model MongoDBModel
	err := model.FromEntity(originalAlias, crypto)
	require.NoError(t, err)

	resultAlias, err := model.ToEntity(crypto)
	require.NoError(t, err)

	assert.Equal(t, *originalAlias.Document, *resultAlias.Document)
	assert.Nil(t, resultAlias.BankingDetails)
}

func TestMongoDBModel_ToEntity_WithDeletedAt(t *testing.T) {
	crypto := testutils.SetupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	aliasID := uuid.New()

	originalAlias := &mmodel.Alias{
		ID:        &aliasID,
		Document:  testutils.Ptr("66677788899"),
		Type:      testutils.Ptr("NATURAL_PERSON"),
		LedgerID:  testutils.Ptr("ledger-deleted"),
		AccountID: testutils.Ptr("account-deleted"),
		CreatedAt: now,
		UpdatedAt: now,
		DeletedAt: &now,
	}

	var model MongoDBModel
	err := model.FromEntity(originalAlias, crypto)
	require.NoError(t, err)

	resultAlias, err := model.ToEntity(crypto)
	require.NoError(t, err)

	require.NotNil(t, resultAlias.DeletedAt)
	assert.Equal(t, *originalAlias.DeletedAt, *resultAlias.DeletedAt)
}
