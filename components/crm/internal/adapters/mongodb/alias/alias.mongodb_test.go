package alias

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/tests/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestMongoDBRepository_buildAliasFilter(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	holderID := uuid.New()

	repo := &MongoDBRepository{
		DataSecurity: crypto,
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
				BankingDetailsAccount: testutils.Ptr("123456"),
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "search.banking_details_account"},
			wantErr:        false,
		},
		{
			name: "filter by banking details IBAN",
			query: http.QueryHeader{
				BankingDetailsIban: testutils.Ptr("BR1234567890123456789012345"),
			},
			holderID:       holderID,
			includeDeleted: false,
			wantKeys:       []string{"holder_id", "deleted_at", "search.banking_details_iban"},
			wantErr:        false,
		},
		{
			name: "filter by banking details branch",
			query: http.QueryHeader{
				BankingDetailsBranch: testutils.Ptr("0001"),
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
				AccountID:            testutils.Ptr("account-789"),
				LedgerID:             testutils.Ptr("ledger-012"),
				BankingDetailsBranch: testutils.Ptr("9999"),
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

			filter, err := repo.buildAliasFilter(tt.query, tt.holderID, tt.includeDeleted)

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

func TestMongoDBRepository_buildAliasFilter_HashGeneration(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	holderID := uuid.New()

	repo := &MongoDBRepository{
		DataSecurity: crypto,
	}

	// Test that document hash is generated consistently
	document := "12345678901"
	expectedHash := crypto.GenerateHash(&document)

	query := http.QueryHeader{
		Document: &document,
	}

	filter, err := repo.buildAliasFilter(query, holderID, false)
	require.NoError(t, err)

	// Find the document hash in the filter
	var foundHash string
	for _, elem := range filter {
		if elem.Key == "search.document" {
			foundHash = elem.Value.(string)
			break
		}
	}

	assert.Equal(t, expectedHash, foundHash, "document hash should match expected")
}

func TestMongoDBRepository_buildAliasFilter_BankingDetailsHashes(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	holderID := uuid.New()

	repo := &MongoDBRepository{
		DataSecurity: crypto,
	}

	account := "123456"
	iban := "BR1234567890123456789012345"

	expectedAccountHash := crypto.GenerateHash(&account)
	expectedIbanHash := crypto.GenerateHash(&iban)

	query := http.QueryHeader{
		BankingDetailsAccount: &account,
		BankingDetailsIban:    &iban,
	}

	filter, err := repo.buildAliasFilter(query, holderID, false)
	require.NoError(t, err)

	var foundAccountHash, foundIbanHash string
	for _, elem := range filter {
		switch elem.Key {
		case "search.banking_details_account":
			foundAccountHash = elem.Value.(string)
		case "search.banking_details_iban":
			foundIbanHash = elem.Value.(string)
		}
	}

	assert.Equal(t, expectedAccountHash, foundAccountHash, "account hash should match")
	assert.Equal(t, expectedIbanHash, foundIbanHash, "IBAN hash should match")
}
