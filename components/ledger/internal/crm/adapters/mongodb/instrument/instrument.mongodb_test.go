// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package instrument

import (
	"context"
	"testing"

	encryption "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

func TestMongoDBRepository_buildInstrumentFilter(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	holderID := uuid.New()
	organizationID := "test-org"

	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	tests := []struct {
		name           string
		query          http.QueryHeader
		holderID       uuid.UUID
		includeDeleted bool
		wantKeys       []string
		wantErr        bool
		errContains    string
	}{
		{
			name:           "empty filter with holder",
			query:          http.QueryHeader{},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at"},
			wantErr:        false,
		},
		{
			name:           "include deleted records",
			query:          http.QueryHeader{},
			holderID:       holderID,
			includeDeleted: true,
			wantKeys:       []string{"holder_id"},
			wantErr:        false,
		},
		{
			name:           "nil holder ID excludes holder filter",
			query:          http.QueryHeader{},
			holderID:       uuid.Nil,
			includeDeleted: false,
			wantKeys:       []string{"deleted_at"},
			wantErr:        false,
		},
		{
			name: "filter by account ID",
			query: http.QueryHeader{
				AccountID: testutils.Ptr("account-123"),
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "account_id"},
			wantErr:        false,
		},
		{
			name: "filter by ledger ID",
			query: http.QueryHeader{
				LedgerID: testutils.Ptr("ledger-456"),
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "ledger_id"},
			wantErr:        false,
		},
		{
			name: "filter by document generates hash",
			query: http.QueryHeader{
				Document: testutils.Ptr("12345678901"),
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "search.document"},
			wantErr:        false,
		},
		{
			name: "filter by banking details account",
			query: http.QueryHeader{
				InstrumentBankingDetailsAccount: testutils.Ptr("123456"),
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "search.banking_details_account"},
			wantErr:        false,
		},
		{
			name: "filter by banking details IBAN",
			query: http.QueryHeader{
				InstrumentBankingDetailsIban: testutils.Ptr("BR1234567890123456789012345"),
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "search.banking_details_iban"},
			wantErr:        false,
		},
		{
			name: "filter by banking details branch",
			query: http.QueryHeader{
				InstrumentBankingDetailsBranch: testutils.Ptr("0001"),
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "banking_details.branch"},
			wantErr:        false,
		},
		{
			name: "filter with metadata",
			query: http.QueryHeader{
				Metadata: &bson.M{
					"metadata.custom_key": "custom_value",
				},
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "metadata.custom_key"},
			wantErr:        false,
		},
		{
			name: "combined filters",
			query: http.QueryHeader{
				AccountID:                      testutils.Ptr("account-789"),
				LedgerID:                       testutils.Ptr("ledger-012"),
				InstrumentBankingDetailsBranch: testutils.Ptr("9999"),
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "account_id", "ledger_id", "banking_details.branch"},
			wantErr:        false,
		},
		{
			name: "invalid metadata value",
			query: http.QueryHeader{
				Metadata: &bson.M{
					"metadata.nested": map[string]any{"invalid": "nested"},
				},
			},
			holderID:       holderID,
			includeDeleted: false,
			wantErr:        true,
			errContains:    "0067",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := repo.buildInstrumentFilter(ctx, organizationID, tt.query, tt.holderID, tt.includeDeleted)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, filter)

			// Extract keys from bson.D
			gotKeys := make([]string, 0, len(filter))
			for _, elem := range filter {
				gotKeys = append(gotKeys, elem.Key)
			}

			// Verify all expected keys are present
			for _, wantKey := range tt.wantKeys {
				assert.Contains(t, gotKeys, wantKey, "filter should contain key: %s", wantKey)
			}

			// Verify count matches expected
			assert.Len(t, gotKeys, len(tt.wantKeys), "filter should have exactly %d keys", len(tt.wantKeys))
		})
	}
}

func TestMongoDBRepository_buildInstrumentFilter_HashGeneration(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	holderID := uuid.New()
	organizationID := "test-org"

	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	// Test that document hash is generated consistently
	document := "12345678901"
	expectedHash := testutils.TestLegacySearchToken(document)

	query := http.QueryHeader{
		Document: &document,
	}

	filter, err := repo.buildInstrumentFilter(ctx, organizationID, query, holderID, false)
	require.NoError(t, err)

	// Find the document hash in the filter (now uses $in operator with token candidates)
	var foundTokens []string
	for _, elem := range filter {
		if elem.Key == "search.document" {
			inQuery := elem.Value.(bson.M)
			foundTokens = inQuery["$in"].([]string)
			break
		}
	}

	require.NotEmpty(t, foundTokens, "should have at least one token candidate")
	assert.Contains(t, foundTokens, expectedHash, "token candidates should contain expected hash")
}

func TestMongoDBRepository_buildInstrumentFilter_BankingDetailsHashes(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	holderID := uuid.New()
	organizationID := "test-org"

	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	account := "123456"
	iban := "BR1234567890123456789012345"

	expectedAccountHash := testutils.TestLegacySearchToken(account)
	expectedIbanHash := testutils.TestLegacySearchToken(iban)

	query := http.QueryHeader{
		InstrumentBankingDetailsAccount: &account,
		InstrumentBankingDetailsIban:    &iban,
	}

	filter, err := repo.buildInstrumentFilter(ctx, organizationID, query, holderID, false)
	require.NoError(t, err)

	// Extract token candidates from $in queries
	var foundAccountTokens, foundIbanTokens []string
	for _, elem := range filter {
		switch elem.Key {
		case "search.banking_details_account":
			inQuery := elem.Value.(bson.M)
			foundAccountTokens = inQuery["$in"].([]string)
		case "search.banking_details_iban":
			inQuery := elem.Value.(bson.M)
			foundIbanTokens = inQuery["$in"].([]string)
		}
	}

	require.NotEmpty(t, foundAccountTokens, "should have at least one account token candidate")
	require.NotEmpty(t, foundIbanTokens, "should have at least one IBAN token candidate")
	assert.Contains(t, foundAccountTokens, expectedAccountHash, "account token candidates should contain expected hash")
	assert.Contains(t, foundIbanTokens, expectedIbanHash, "IBAN token candidates should contain expected hash")
}

// TestMongoDBRepository_appendEncryptedFilters_UsesInOperator verifies that all 5 encrypted
// field filters use the $in operator with token candidates for key rotation support.
func TestMongoDBRepository_appendEncryptedFilters_UsesInOperator(t *testing.T) {
	t.Parallel()

	fe := setupTestFieldEncryptor(t)
	ctx := context.Background()
	holderID := uuid.New()
	organizationID := "test-org"

	repo := &MongoDBRepository{
		FieldEncryptor: fe,
	}

	// Setup query with all 5 encrypted fields
	document := "12345678901"
	account := "123456"
	iban := "BR1234567890123456789012345"
	participantDoc := "98765432109876"
	relatedPartyDoc := "55566677788"

	query := http.QueryHeader{
		Document:                                      &document,
		InstrumentBankingDetailsAccount:               &account,
		InstrumentBankingDetailsIban:                  &iban,
		InstrumentRegulatoryFieldsParticipantDocument: &participantDoc,
		InstrumentRelatedPartyDocument:                &relatedPartyDoc,
	}

	filter, err := repo.buildInstrumentFilter(ctx, organizationID, query, holderID, false)
	require.NoError(t, err)

	// Expected token for legacy crypto (single token returned as slice)
	expectedDocumentToken := testutils.TestLegacySearchToken(document)
	expectedAccountToken := testutils.TestLegacySearchToken(account)
	expectedIbanToken := testutils.TestLegacySearchToken(iban)
	expectedParticipantToken := testutils.TestLegacySearchToken(participantDoc)
	expectedRelatedPartyToken := testutils.TestLegacySearchToken(relatedPartyDoc)

	// Verify each encrypted field uses $in operator with token candidates
	encryptedFields := map[string]string{
		"search.document":                               expectedDocumentToken,
		"search.banking_details_account":                expectedAccountToken,
		"search.banking_details_iban":                   expectedIbanToken,
		"search.regulatory_fields_participant_document": expectedParticipantToken,
		"search.related_party_documents":                expectedRelatedPartyToken,
	}

	for _, elem := range filter {
		if expectedToken, ok := encryptedFields[elem.Key]; ok {
			// Value must be bson.M with $in operator
			inQuery, ok := elem.Value.(bson.M)
			require.True(t, ok, "filter for %s should be bson.M with $in operator, got %T", elem.Key, elem.Value)

			tokens, ok := inQuery["$in"]
			require.True(t, ok, "filter for %s should have $in operator", elem.Key)

			tokenSlice, ok := tokens.([]string)
			require.True(t, ok, "filter for %s $in value should be []string, got %T", elem.Key, tokens)

			// With legacy crypto, we expect at least 1 token
			require.NotEmpty(t, tokenSlice, "filter for %s should have at least one token candidate", elem.Key)
			assert.Contains(t, tokenSlice, expectedToken, "filter for %s should contain expected token", elem.Key)
		}
	}
}

// TestMongoDBRepository_appendEncryptedFilters_ErrorPropagation verifies that errors from
// GenerateSearchTokenCandidates are properly propagated.
func TestMongoDBRepository_appendEncryptedFilters_ErrorPropagation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	holderID := uuid.New()
	organizationID := "test-org"

	tests := []struct {
		name        string
		query       http.QueryHeader
		description string
	}{
		{
			name: "document filter error",
			query: http.QueryHeader{
				Document: testutils.Ptr("12345678901"),
			},
			description: "error generating document search tokens",
		},
		{
			name: "banking details account filter error",
			query: http.QueryHeader{
				InstrumentBankingDetailsAccount: testutils.Ptr("123456"),
			},
			description: "error generating banking_details.account search tokens",
		},
		{
			name: "banking details iban filter error",
			query: http.QueryHeader{
				InstrumentBankingDetailsIban: testutils.Ptr("BR1234567890123456789012345"),
			},
			description: "error generating banking_details.iban search tokens",
		},
		{
			name: "regulatory fields participant document filter error",
			query: http.QueryHeader{
				InstrumentRegulatoryFieldsParticipantDocument: testutils.Ptr("98765432109876"),
			},
			description: "error generating regulatory_fields.participant_document search tokens",
		},
		{
			name: "related party document filter error",
			query: http.QueryHeader{
				InstrumentRelatedPartyDocument: testutils.Ptr("55566677788"),
			},
			description: "error generating related_parties.document search tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Use mock that fails on GenerateSearchTokenCandidates
			fe := &failingSearchTokenCandidatesEncryptor{}

			repo := &MongoDBRepository{
				FieldEncryptor: fe,
			}

			_, err := repo.buildInstrumentFilter(ctx, organizationID, tt.query, holderID, false)
			require.Error(t, err, tt.description)
		})
	}
}

// failingSearchTokenCandidatesEncryptor is a mock that fails on GenerateSearchTokenCandidates.
type failingSearchTokenCandidatesEncryptor struct{}

func (f *failingSearchTokenCandidatesEncryptor) EncryptField(_ context.Context, _ encryption.FieldContext, _ string) (string, error) {
	return "encrypted", nil
}

func (f *failingSearchTokenCandidatesEncryptor) DecryptField(_ context.Context, _ encryption.FieldContext, _ string) (string, error) {
	return "decrypted", nil
}

func (f *failingSearchTokenCandidatesEncryptor) GenerateSearchToken(_ context.Context, _ encryption.SearchTokenContext, _ string) (string, uint32, error) {
	return "token", 0, nil
}

func (f *failingSearchTokenCandidatesEncryptor) GenerateSearchTokenCandidates(_ context.Context, _ encryption.SearchTokenContext, _ string) ([]string, error) {
	return nil, assert.AnError
}

// ============================================================================
// Multi-Token Key Rotation Tests
// ============================================================================

// mockFieldEncryptorMultiToken implements FieldEncryptor that returns configurable
// multiple tokens to simulate key rotation scenarios where multiple HMAC keys are active.
type mockFieldEncryptorMultiToken struct {
	tokens []string
}

func (m *mockFieldEncryptorMultiToken) EncryptField(_ context.Context, _ encryption.FieldContext, plaintext string) (string, error) {
	return "encrypted-" + plaintext, nil
}

func (m *mockFieldEncryptorMultiToken) DecryptField(_ context.Context, _ encryption.FieldContext, ciphertext string) (string, error) {
	return ciphertext, nil
}

func (m *mockFieldEncryptorMultiToken) GenerateSearchToken(_ context.Context, _ encryption.SearchTokenContext, _ string) (string, uint32, error) {
	if len(m.tokens) > 0 {
		return m.tokens[0], 0, nil
	}

	return "mock-token", 0, nil
}

func (m *mockFieldEncryptorMultiToken) GenerateSearchTokenCandidates(_ context.Context, _ encryption.SearchTokenContext, _ string) ([]string, error) {
	return m.tokens, nil
}

// TestMongoDBRepository_appendEncryptedFilters_MultiTokenKeyRotation verifies that all 5 encrypted
// fields correctly use $in operator with multiple tokens when key rotation is active.
func TestMongoDBRepository_appendEncryptedFilters_MultiTokenKeyRotation(t *testing.T) {
	t.Parallel()

	// Simulate key rotation with 3 active HMAC keys (old, current, new)
	rotationTokens := []string{"token-old-key", "token-current-key", "token-new-key"}

	mockFE := &mockFieldEncryptorMultiToken{
		tokens: rotationTokens,
	}

	ctx := context.Background()
	holderID := uuid.New()
	organizationID := "test-org"

	repo := &MongoDBRepository{
		FieldEncryptor: mockFE,
	}

	// Setup query with all 5 encrypted fields
	query := http.QueryHeader{
		Document:                                      testutils.Ptr("12345678901"),
		InstrumentBankingDetailsAccount:               testutils.Ptr("123456"),
		InstrumentBankingDetailsIban:                  testutils.Ptr("BR1234567890123456789012345"),
		InstrumentRegulatoryFieldsParticipantDocument: testutils.Ptr("98765432109876"),
		InstrumentRelatedPartyDocument:                testutils.Ptr("55566677788"),
	}

	filter, err := repo.buildInstrumentFilter(ctx, organizationID, query, holderID, false)
	require.NoError(t, err)

	// Map of encrypted search fields to verify
	encryptedFields := []string{
		"search.document",
		"search.banking_details_account",
		"search.banking_details_iban",
		"search.regulatory_fields_participant_document",
		"search.related_party_documents",
	}

	for _, fieldKey := range encryptedFields {
		var foundInFilter bson.M
		for _, elem := range filter {
			if elem.Key == fieldKey {
				foundInFilter, _ = elem.Value.(bson.M)

				break
			}
		}

		require.NotNil(t, foundInFilter, "filter for %s should be present", fieldKey)

		tokens, ok := foundInFilter["$in"].([]string)
		require.True(t, ok, "filter for %s should use $in operator with []string", fieldKey)
		assert.Len(t, tokens, 3, "filter for %s should contain all 3 rotation tokens", fieldKey)
		assert.Contains(t, tokens, "token-old-key", "filter for %s should contain old key token", fieldKey)
		assert.Contains(t, tokens, "token-current-key", "filter for %s should contain current key token", fieldKey)
		assert.Contains(t, tokens, "token-new-key", "filter for %s should contain new key token", fieldKey)
	}
}

// TestMongoDBRepository_appendEncryptedFilters_MultiTokenKeyRotation_IndividualFields verifies
// that each encrypted field independently receives the correct multi-token filter.
func TestMongoDBRepository_appendEncryptedFilters_MultiTokenKeyRotation_IndividualFields(t *testing.T) {
	t.Parallel()

	rotationTokens := []string{"old-hmac-token", "current-hmac-token", "new-hmac-token", "future-hmac-token"}

	mockFE := &mockFieldEncryptorMultiToken{
		tokens: rotationTokens,
	}

	ctx := context.Background()
	holderID := uuid.Nil // No holder filter
	organizationID := "rotation-test-org"

	repo := &MongoDBRepository{
		FieldEncryptor: mockFE,
	}

	tests := []struct {
		name      string
		query     http.QueryHeader
		wantField string
	}{
		{
			name: "document field with multi-token",
			query: http.QueryHeader{
				Document: testutils.Ptr("document-value"),
			},
			wantField: "search.document",
		},
		{
			name: "banking account field with multi-token",
			query: http.QueryHeader{
				InstrumentBankingDetailsAccount: testutils.Ptr("account-value"),
			},
			wantField: "search.banking_details_account",
		},
		{
			name: "banking IBAN field with multi-token",
			query: http.QueryHeader{
				InstrumentBankingDetailsIban: testutils.Ptr("iban-value"),
			},
			wantField: "search.banking_details_iban",
		},
		{
			name: "regulatory participant document with multi-token",
			query: http.QueryHeader{
				InstrumentRegulatoryFieldsParticipantDocument: testutils.Ptr("participant-doc"),
			},
			wantField: "search.regulatory_fields_participant_document",
		},
		{
			name: "related party document with multi-token",
			query: http.QueryHeader{
				InstrumentRelatedPartyDocument: testutils.Ptr("related-party-doc"),
			},
			wantField: "search.related_party_documents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := repo.buildInstrumentFilter(ctx, organizationID, tt.query, holderID, true)
			require.NoError(t, err)

			var foundInFilter bson.M
			for _, elem := range filter {
				if elem.Key == tt.wantField {
					foundInFilter, _ = elem.Value.(bson.M)

					break
				}
			}

			require.NotNil(t, foundInFilter, "filter for %s should be present", tt.wantField)

			tokens, ok := foundInFilter["$in"].([]string)
			require.True(t, ok, "$in value should be []string")
			assert.Len(t, tokens, 4, "should have all 4 rotation tokens")
			assert.Equal(t, rotationTokens, tokens, "tokens should match rotation tokens in order")
		})
	}
}

// TestMongoDBRepository_appendEncryptedFilters_MultiTokenKeyRotation_OrderPreserved verifies
// that token order is preserved in the $in clause for deterministic query execution.
func TestMongoDBRepository_appendEncryptedFilters_MultiTokenKeyRotation_OrderPreserved(t *testing.T) {
	t.Parallel()

	// Token order matters for deterministic query plans and consistent behavior
	orderedTokens := []string{"first", "second", "third", "fourth", "fifth"}

	mockFE := &mockFieldEncryptorMultiToken{
		tokens: orderedTokens,
	}

	ctx := context.Background()
	organizationID := "order-test-org"

	repo := &MongoDBRepository{
		FieldEncryptor: mockFE,
	}

	query := http.QueryHeader{
		Document: testutils.Ptr("test-document"),
	}

	filter, err := repo.buildInstrumentFilter(ctx, organizationID, query, uuid.Nil, true)
	require.NoError(t, err)

	var foundTokens []string
	for _, elem := range filter {
		if elem.Key == "search.document" {
			inFilter := elem.Value.(bson.M)
			foundTokens = inFilter["$in"].([]string)

			break
		}
	}

	require.Len(t, foundTokens, len(orderedTokens))
	assert.Equal(t, orderedTokens, foundTokens, "token order must be preserved for deterministic queries")
}

func TestRepositoryInputAttributes(t *testing.T) {
	const (
		keyHasMetadata         = "app.request.repository_input.has_metadata"
		keyHasBankingDetails   = "app.request.repository_input.has_banking_details"
		keyHasRegulatoryFields = "app.request.repository_input.has_regulatory_fields"
		keyRelatedPartiesCount = "app.request.repository_input.related_parties_count"
	)

	allowedKeys := map[attribute.Key]struct{}{
		attribute.Key(keyHasMetadata):         {},
		attribute.Key(keyHasBankingDetails):   {},
		attribute.Key(keyHasRegulatoryFields): {},
		attribute.Key(keyRelatedPartiesCount): {},
	}

	tests := []struct {
		name              string
		model             *MongoDBModel
		wantHasMetadata   bool
		wantHasBanking    bool
		wantHasRegulatory bool
		wantCount         int64
	}{
		{
			name: "populated model",
			model: &MongoDBModel{
				Metadata:         map[string]any{"k": "v"},
				BankingDetails:   &BankingMongoDBModel{},
				RegulatoryFields: &RegulatoryFieldsMongoDBModel{},
				RelatedParties:   []*RelatedPartyMongoDBModel{{}, {}},
			},
			wantHasMetadata:   true,
			wantHasBanking:    true,
			wantHasRegulatory: true,
			wantCount:         2,
		},
		{
			name:              "zero-value model",
			model:             &MongoDBModel{},
			wantHasMetadata:   false,
			wantHasBanking:    false,
			wantHasRegulatory: false,
			wantCount:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := repositoryInputAttributes(tt.model)

			byKey := make(map[attribute.Key]attribute.Value, len(attrs))
			for _, a := range attrs {
				if _, ok := allowedKeys[a.Key]; !ok {
					t.Fatalf("unexpected attribute key emitted: %q", a.Key)
				}

				if _, dup := byKey[a.Key]; dup {
					t.Fatalf("duplicate attribute key emitted: %q", a.Key)
				}

				byKey[a.Key] = a.Value
			}

			if len(byKey) != len(allowedKeys) {
				t.Fatalf("expected exactly %d keys, got %d: %v", len(allowedKeys), len(byKey), byKey)
			}

			if got := byKey[attribute.Key(keyHasMetadata)].AsBool(); got != tt.wantHasMetadata {
				t.Errorf("%s = %v, want %v", keyHasMetadata, got, tt.wantHasMetadata)
			}

			if got := byKey[attribute.Key(keyHasBankingDetails)].AsBool(); got != tt.wantHasBanking {
				t.Errorf("%s = %v, want %v", keyHasBankingDetails, got, tt.wantHasBanking)
			}

			if got := byKey[attribute.Key(keyHasRegulatoryFields)].AsBool(); got != tt.wantHasRegulatory {
				t.Errorf("%s = %v, want %v", keyHasRegulatoryFields, got, tt.wantHasRegulatory)
			}

			if got := byKey[attribute.Key(keyRelatedPartiesCount)].AsInt64(); got != tt.wantCount {
				t.Errorf("%s = %d, want %d", keyRelatedPartiesCount, got, tt.wantCount)
			}
		})
	}
}
