package alias

import (
	"testing"
	"time"

	libCrypto "github.com/LerianStudio/lib-commons/v2/commons/crypto"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupCrypto creates a Crypto instance for testing
func setupCrypto(t *testing.T) *libCrypto.Crypto {
	t.Helper()

	logger := &libLog.GoLogger{Level: libLog.InfoLevel}

	// Keys must be hex-encoded 32-byte (64 hex chars) values
	hashKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encryptKey := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	crypto := &libCrypto.Crypto{
		HashSecretKey:    hashKey,
		EncryptSecretKey: encryptKey,
		Logger:           logger,
	}

	err := crypto.InitializeCipher()
	require.NoError(t, err)

	return crypto
}

func ptr[T any](v T) *T {
	return &v
}

func TestMongoDBModel_FromEntity(t *testing.T) {
	crypto := setupCrypto(t)
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
				Document:  ptr("12345678901"),
				Type:      ptr("NATURAL_PERSON"),
				LedgerID:  ptr("ledger-123"),
				AccountID: ptr("account-456"),
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "alias with holder",
			alias: &mmodel.Alias{
				ID:        &aliasID,
				Document:  ptr("98765432100"),
				Type:      ptr("LEGAL_PERSON"),
				LedgerID:  ptr("ledger-789"),
				AccountID: ptr("account-012"),
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
				Document:  ptr("11122233344"),
				Type:      ptr("NATURAL_PERSON"),
				LedgerID:  ptr("ledger-456"),
				AccountID: ptr("account-789"),
				BankingDetails: &mmodel.BankingDetails{
					Branch:      ptr("0001"),
					Account:     ptr("123456"),
					Type:        ptr("CACC"),
					OpeningDate: ptr("2025-01-01"),
					IBAN:        ptr("BR1234567890123456789012345"),
					CountryCode: ptr("BR"),
					BankID:      ptr("001"),
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
				Document:  ptr("55566677788"),
				Type:      ptr("NATURAL_PERSON"),
				LedgerID:  ptr("ledger-meta"),
				AccountID: ptr("account-meta"),
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
				Document:  ptr("77788899900"),
				Type:      ptr("LEGAL_PERSON"),
				LedgerID:  ptr("ledger-nil"),
				AccountID: ptr("account-nil"),
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
				Document:            ptr("99900011122"),
				Type:                ptr("NATURAL_PERSON"),
				LedgerID:            ptr("ledger-part"),
				AccountID:           ptr("account-part"),
				ParticipantDocument: ptr("12345678901234"),
				CreatedAt:           now,
				UpdatedAt:           now,
			},
			wantErr: false,
		},
		{
			name: "alias with closing date",
			alias: &mmodel.Alias{
				ID:          &aliasID,
				Document:    ptr("44455566677"),
				Type:        ptr("NATURAL_PERSON"),
				LedgerID:    ptr("ledger-close"),
				AccountID:   ptr("account-close"),
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
				Document:  ptr("11111111111"),
				Type:      ptr("LEGAL_PERSON"),
				LedgerID:  ptr("ledger-full"),
				AccountID: ptr("account-full"),
				HolderID:  &holderID,
				BankingDetails: &mmodel.BankingDetails{
					Branch:      ptr("9999"),
					Account:     ptr("999999"),
					Type:        ptr("SVGS"),
					OpeningDate: ptr("2020-06-15"),
					IBAN:        ptr("US12345678901234567890"),
					CountryCode: ptr("US"),
					BankID:      ptr("BANK123"),
				},
				Metadata: map[string]any{
					"complete": true,
				},
				ParticipantDocument: ptr("98765432109876"),
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
	crypto := setupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	aliasID := uuid.New()
	holderID := uuid.New()

	// First create a model from an entity, then convert back
	originalAlias := &mmodel.Alias{
		ID:        &aliasID,
		Document:  ptr("33344455566"),
		Type:      ptr("NATURAL_PERSON"),
		LedgerID:  ptr("ledger-roundtrip"),
		AccountID: ptr("account-roundtrip"),
		HolderID:  &holderID,
		BankingDetails: &mmodel.BankingDetails{
			Branch:      ptr("1234"),
			Account:     ptr("567890"),
			Type:        ptr("CACC"),
			OpeningDate: ptr("2023-06-15"),
			IBAN:        ptr("BR9876543210987654321098765"),
			CountryCode: ptr("BR"),
			BankID:      ptr("341"),
		},
		Metadata: map[string]any{
			"testKey": "testValue",
		},
		ParticipantDocument: ptr("11223344556677"),
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
	crypto := setupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	aliasID := uuid.New()

	originalAlias := &mmodel.Alias{
		ID:             &aliasID,
		Document:       ptr("99988877766"),
		Type:           ptr("LEGAL_PERSON"),
		LedgerID:       ptr("ledger-no-bank"),
		AccountID:      ptr("account-no-bank"),
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
	crypto := setupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	aliasID := uuid.New()

	originalAlias := &mmodel.Alias{
		ID:        &aliasID,
		Document:  ptr("66677788899"),
		Type:      ptr("NATURAL_PERSON"),
		LedgerID:  ptr("ledger-deleted"),
		AccountID: ptr("account-deleted"),
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
