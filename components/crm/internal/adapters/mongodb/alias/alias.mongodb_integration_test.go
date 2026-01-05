//go:build integration

package alias

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================================
// Test Helpers
// ============================================================================

// createRepository creates a MongoDBRepository for integration testing.
func createRepository(t *testing.T, container *mongotestutil.ContainerResult) *MongoDBRepository {
	t.Helper()

	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)
	crypto := testutils.SetupCrypto(t)

	return &MongoDBRepository{
		connection:   conn,
		Database:     container.DBName,
		DataSecurity: crypto,
	}
}

// createTestAlias builds a test alias with default values.
func createTestAlias(holderID uuid.UUID, accountID, document string) *mmodel.Alias {
	aliasID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	return &mmodel.Alias{
		ID:        &aliasID,
		Document:  testutils.Ptr(document),
		Type:      testutils.Ptr("NATURAL_PERSON"),
		LedgerID:  testutils.Ptr("ledger-" + uuid.New().String()[:8]),
		AccountID: testutils.Ptr(accountID),
		HolderID:  &holderID,
		Metadata:  map[string]any{"test": true},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// createTestAliasWithBanking builds a test alias with banking details.
func createTestAliasWithBanking(holderID uuid.UUID, accountID, document string) *mmodel.Alias {
	alias := createTestAlias(holderID, accountID, document)
	alias.BankingDetails = &mmodel.BankingDetails{
		Branch:      testutils.Ptr("0001"),
		Account:     testutils.Ptr("123456"),
		Type:        testutils.Ptr("CACC"),
		OpeningDate: testutils.Ptr("2025-01-01"),
		IBAN:        testutils.Ptr("BR1234567890123456789012345"),
		CountryCode: testutils.Ptr("BR"),
		BankID:      testutils.Ptr("001"),
	}
	return alias
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_AliasRepo_Create(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-" + uuid.New().String()[:8]
	holderID := uuid.New()
	originalDocument := "12345678901"

	alias := createTestAlias(holderID, "account-create-1", originalDocument)

	// Act
	result, err := repo.Create(ctx, organizationID, alias)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, result)
	assert.Equal(t, alias.ID, result.ID)
	assert.Equal(t, originalDocument, *result.Document, "returned document should be decrypted")

	// Verify via direct query - document should be encrypted in storage
	collName := strings.ToLower("aliases_" + organizationID)
	count := mongotestutil.CountDocuments(t, container.Database, collName, bson.M{"_id": alias.ID})
	assert.Equal(t, int64(1), count, "should have exactly 1 document")
}

func TestIntegration_AliasRepo_Create_EncryptsData(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-encrypt-" + uuid.New().String()[:8]
	holderID := uuid.New()
	originalDocument := "99988877766"

	alias := createTestAliasWithBanking(holderID, "account-encrypt-1", originalDocument)

	// Act
	_, err := repo.Create(ctx, organizationID, alias)
	require.NoError(t, err)

	// Assert - Query raw document to verify encryption
	collName := strings.ToLower("aliases_" + organizationID)
	var rawDoc bson.M
	err = container.Database.Collection(collName).FindOne(ctx, bson.M{"_id": alias.ID}).Decode(&rawDoc)
	require.NoError(t, err)

	// Document should be encrypted (not equal to original)
	storedDoc, ok := rawDoc["document"].(string)
	require.True(t, ok, "document should be stored as string")
	assert.NotEqual(t, originalDocument, storedDoc, "document should be encrypted in storage")

	// Search hash should be present
	search, ok := rawDoc["search"].(bson.M)
	require.True(t, ok, "search map should exist")
	assert.NotEmpty(t, search["document"], "document hash should be generated")
}

func TestIntegration_AliasRepo_Create_DuplicateAccount(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-dup-" + uuid.New().String()[:8]
	holderID := uuid.New()
	sharedAccountID := "account-duplicate-test"

	// Create first alias
	alias1 := createTestAlias(holderID, sharedAccountID, "11111111111")
	_, err := repo.Create(ctx, organizationID, alias1)
	require.NoError(t, err, "first create should succeed")

	// Act - Try to create second alias with same account_id
	alias2 := createTestAlias(holderID, sharedAccountID, "22222222222")
	_, err = repo.Create(ctx, organizationID, alias2)

	// Assert
	require.Error(t, err, "duplicate account_id should fail")
	assert.Contains(t, err.Error(), "accountId from ledger can only be associated with a single", "should return ErrAccountAlreadyAssociated")
}

// ============================================================================
// Find Tests
// ============================================================================

func TestIntegration_AliasRepo_Find(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-find-" + uuid.New().String()[:8]
	holderID := uuid.New()
	originalDocument := "44455566677"

	alias := createTestAliasWithBanking(holderID, "account-find-1", originalDocument)
	_, err := repo.Create(ctx, organizationID, alias)
	require.NoError(t, err)

	// Act
	result, err := repo.Find(ctx, organizationID, holderID, *alias.ID, false)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, alias.ID, result.ID)
	assert.Equal(t, originalDocument, *result.Document, "document should be decrypted")
	assert.Equal(t, *alias.BankingDetails.Account, *result.BankingDetails.Account, "banking account should be decrypted")
	assert.Equal(t, *alias.BankingDetails.IBAN, *result.BankingDetails.IBAN, "IBAN should be decrypted")
}

func TestIntegration_AliasRepo_Find_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-notfound-" + uuid.New().String()[:8]
	holderID := uuid.New()
	nonExistentID := uuid.New()

	// Act
	result, err := repo.Find(ctx, organizationID, holderID, nonExistentID, false)

	// Assert
	require.Error(t, err, "should return error for non-existent alias")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "alias ID does not exist", "should return ErrAliasNotFound")
}

func TestIntegration_AliasRepo_Find_ExcludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-deleted-" + uuid.New().String()[:8]
	holderID := uuid.New()

	alias := createTestAlias(holderID, "account-deleted-1", "77788899900")
	_, err := repo.Create(ctx, organizationID, alias)
	require.NoError(t, err)

	// Soft delete
	err = repo.Delete(ctx, organizationID, holderID, *alias.ID, false)
	require.NoError(t, err)

	// Act - Find without includeDeleted
	result, err := repo.Find(ctx, organizationID, holderID, *alias.ID, false)

	// Assert
	require.Error(t, err, "should not find soft-deleted alias")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "alias ID does not exist")
}

func TestIntegration_AliasRepo_Find_IncludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-incldel-" + uuid.New().String()[:8]
	holderID := uuid.New()

	alias := createTestAlias(holderID, "account-incldel-1", "66655544433")
	_, err := repo.Create(ctx, organizationID, alias)
	require.NoError(t, err)

	// Soft delete
	err = repo.Delete(ctx, organizationID, holderID, *alias.ID, false)
	require.NoError(t, err)

	// Act - Find with includeDeleted=true
	result, err := repo.Find(ctx, organizationID, holderID, *alias.ID, true)

	// Assert
	require.NoError(t, err, "should find soft-deleted alias with includeDeleted=true")
	require.NotNil(t, result)
	assert.NotNil(t, result.DeletedAt, "deleted_at should be set")
}

// ============================================================================
// FindAll Tests
// ============================================================================

func TestIntegration_AliasRepo_FindAll(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-findall-" + uuid.New().String()[:8]
	holderID := uuid.New()

	// Create multiple aliases
	for i := 0; i < 5; i++ {
		alias := createTestAlias(holderID, "account-findall-"+string(rune('a'+i)), "1111111111"+string(rune('0'+i)))
		_, err := repo.Create(ctx, organizationID, alias)
		require.NoError(t, err)
	}

	// Act
	filter := http.QueryHeader{Limit: 10, Page: 1}
	results, err := repo.FindAll(ctx, organizationID, holderID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 5, "should return all 5 aliases")
}

func TestIntegration_AliasRepo_FindAll_Pagination(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-page-" + uuid.New().String()[:8]
	holderID := uuid.New()

	// Create 5 aliases
	for i := 0; i < 5; i++ {
		alias := createTestAlias(holderID, "account-page-"+string(rune('a'+i)), "2222222222"+string(rune('0'+i)))
		_, err := repo.Create(ctx, organizationID, alias)
		require.NoError(t, err)
	}

	// Act - Get page 1 (limit 2)
	page1, err := repo.FindAll(ctx, organizationID, holderID, http.QueryHeader{Limit: 2, Page: 1}, false)
	require.NoError(t, err)

	// Act - Get page 2
	page2, err := repo.FindAll(ctx, organizationID, holderID, http.QueryHeader{Limit: 2, Page: 2}, false)
	require.NoError(t, err)

	// Act - Get page 3
	page3, err := repo.FindAll(ctx, organizationID, holderID, http.QueryHeader{Limit: 2, Page: 3}, false)
	require.NoError(t, err)

	// Assert
	assert.Len(t, page1, 2, "page 1 should have 2 items")
	assert.Len(t, page2, 2, "page 2 should have 2 items")
	assert.Len(t, page3, 1, "page 3 should have 1 item")

	// Verify no duplicates
	allIDs := make(map[uuid.UUID]bool)
	for _, r := range append(append(page1, page2...), page3...) {
		assert.False(t, allIDs[*r.ID], "should not have duplicates")
		allIDs[*r.ID] = true
	}
	assert.Len(t, allIDs, 5)
}

func TestIntegration_AliasRepo_FindAll_FilterByDocument(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-filterdoc-" + uuid.New().String()[:8]
	holderID := uuid.New()
	targetDocument := "33344455566"

	// Create aliases with different documents
	alias1 := createTestAlias(holderID, "account-filterdoc-1", targetDocument)
	_, err := repo.Create(ctx, organizationID, alias1)
	require.NoError(t, err)

	alias2 := createTestAlias(holderID, "account-filterdoc-2", "99988877766")
	_, err = repo.Create(ctx, organizationID, alias2)
	require.NoError(t, err)

	// Act - Filter by document
	filter := http.QueryHeader{
		Limit:    10,
		Page:     1,
		Document: testutils.Ptr(targetDocument),
	}
	results, err := repo.FindAll(ctx, organizationID, holderID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should return only alias with matching document")
	assert.Equal(t, targetDocument, *results[0].Document)
}

func TestIntegration_AliasRepo_FindAll_FilterByAccountID(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-filteracc-" + uuid.New().String()[:8]
	holderID := uuid.New()
	targetAccountID := "account-target-xyz"

	// Create aliases with different account IDs
	alias1 := createTestAlias(holderID, targetAccountID, "11122233344")
	_, err := repo.Create(ctx, organizationID, alias1)
	require.NoError(t, err)

	alias2 := createTestAlias(holderID, "account-other-abc", "55566677788")
	_, err = repo.Create(ctx, organizationID, alias2)
	require.NoError(t, err)

	// Act
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		AccountID: testutils.Ptr(targetAccountID),
	}
	results, err := repo.FindAll(ctx, organizationID, holderID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, targetAccountID, *results[0].AccountID)
}

func TestIntegration_AliasRepo_FindAll_ReturnsEmpty(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-empty-" + uuid.New().String()[:8]
	holderID := uuid.New()

	// Act - Query empty collection
	filter := http.QueryHeader{Limit: 10, Page: 1}
	results, err := repo.FindAll(ctx, organizationID, holderID, filter, false)

	// Assert
	require.NoError(t, err, "should not error on empty result")
	assert.Empty(t, results)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_AliasRepo_Update(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-update-" + uuid.New().String()[:8]
	holderID := uuid.New()

	alias := createTestAlias(holderID, "account-update-1", "88899900011")
	_, err := repo.Create(ctx, organizationID, alias)
	require.NoError(t, err)

	// Act - Update document and type
	updatedAlias := &mmodel.Alias{
		Document: testutils.Ptr("11100099988"),
		Type:     testutils.Ptr("LEGAL_PERSON"),
	}
	result, err := repo.Update(ctx, organizationID, holderID, *alias.ID, updatedAlias, nil)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "11100099988", *result.Document)
	assert.Equal(t, "LEGAL_PERSON", *result.Type)
}

func TestIntegration_AliasRepo_Update_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-upnotfound-" + uuid.New().String()[:8]
	holderID := uuid.New()
	nonExistentID := uuid.New()

	// Act
	updatedAlias := &mmodel.Alias{Document: testutils.Ptr("00000000000")}
	result, err := repo.Update(ctx, organizationID, holderID, nonExistentID, updatedAlias, nil)

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "alias ID does not exist", "should return ErrAliasNotFound")
}

func TestIntegration_AliasRepo_Update_FieldsToRemove(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-remove-" + uuid.New().String()[:8]
	holderID := uuid.New()

	alias := createTestAliasWithBanking(holderID, "account-remove-1", "77766655544")
	alias.Metadata = map[string]any{"key1": "value1", "key2": "value2"}
	_, err := repo.Create(ctx, organizationID, alias)
	require.NoError(t, err)

	// Act - Remove metadata.key1
	result, err := repo.Update(ctx, organizationID, holderID, *alias.ID, &mmodel.Alias{}, []string{"metadata.key1"})

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	_, hasKey1 := result.Metadata["key1"]
	assert.False(t, hasKey1, "key1 should be removed")
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_AliasRepo_Delete_Soft(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-softdel-" + uuid.New().String()[:8]
	holderID := uuid.New()

	alias := createTestAlias(holderID, "account-softdel-1", "55544433322")
	_, err := repo.Create(ctx, organizationID, alias)
	require.NoError(t, err)

	// Act - Soft delete
	err = repo.Delete(ctx, organizationID, holderID, *alias.ID, false)

	// Assert
	require.NoError(t, err)

	// Verify document still exists with deleted_at set
	result, err := repo.Find(ctx, organizationID, holderID, *alias.ID, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.DeletedAt, "deleted_at should be set")
}

func TestIntegration_AliasRepo_Delete_Hard(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-harddel-" + uuid.New().String()[:8]
	holderID := uuid.New()

	alias := createTestAlias(holderID, "account-harddel-1", "22211100099")
	_, err := repo.Create(ctx, organizationID, alias)
	require.NoError(t, err)

	// Act - Hard delete
	err = repo.Delete(ctx, organizationID, holderID, *alias.ID, true)

	// Assert
	require.NoError(t, err)

	// Verify document is completely removed
	collName := strings.ToLower("aliases_" + organizationID)
	count := mongotestutil.CountDocuments(t, container.Database, collName, bson.M{"_id": alias.ID})
	assert.Equal(t, int64(0), count, "document should be removed from collection")
}

func TestIntegration_AliasRepo_Delete_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-delnotfound-" + uuid.New().String()[:8]
	holderID := uuid.New()
	nonExistentID := uuid.New()

	// Act
	err := repo.Delete(ctx, organizationID, holderID, nonExistentID, false)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alias ID does not exist", "should return ErrAliasNotFound")
}

// ============================================================================
// Count Tests
// ============================================================================

func TestIntegration_AliasRepo_Count(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-count-" + uuid.New().String()[:8]
	holderID := uuid.New()

	// Create 3 aliases
	for i := 0; i < 3; i++ {
		alias := createTestAlias(holderID, "account-count-"+string(rune('a'+i)), "4444444444"+string(rune('0'+i)))
		_, err := repo.Create(ctx, organizationID, alias)
		require.NoError(t, err)
	}

	// Act
	count, err := repo.Count(ctx, organizationID, holderID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestIntegration_AliasRepo_Count_ExcludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-countdel-" + uuid.New().String()[:8]
	holderID := uuid.New()

	// Create 3 aliases
	aliases := make([]*mmodel.Alias, 3)
	for i := 0; i < 3; i++ {
		alias := createTestAlias(holderID, "account-countdel-"+string(rune('a'+i)), "5555555555"+string(rune('0'+i)))
		_, err := repo.Create(ctx, organizationID, alias)
		require.NoError(t, err)
		aliases[i] = alias
	}

	// Soft delete one
	err := repo.Delete(ctx, organizationID, holderID, *aliases[0].ID, false)
	require.NoError(t, err)

	// Act
	count, err := repo.Count(ctx, organizationID, holderID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "count should exclude soft-deleted alias")
}

func TestIntegration_AliasRepo_Count_ReturnsZero(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-countzero-" + uuid.New().String()[:8]
	holderID := uuid.New()

	// Act - Count empty collection
	count, err := repo.Count(ctx, organizationID, holderID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

// ============================================================================
// Encryption Round-Trip Tests
// ============================================================================

func TestIntegration_AliasRepo_EncryptionRoundTrip(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-roundtrip-" + uuid.New().String()[:8]
	holderID := uuid.New()

	// Original values
	originalDocument := "98765432100"
	originalAccount := "987654"
	originalIBAN := "US9876543210987654321098765"
	originalParticipantDoc := "11223344556677"

	alias := createTestAliasWithBanking(holderID, "account-roundtrip-1", originalDocument)
	alias.BankingDetails.Account = testutils.Ptr(originalAccount)
	alias.BankingDetails.IBAN = testutils.Ptr(originalIBAN)
	alias.ParticipantDocument = testutils.Ptr(originalParticipantDoc)

	// Act - Create and retrieve
	_, err := repo.Create(ctx, organizationID, alias)
	require.NoError(t, err)

	result, err := repo.Find(ctx, organizationID, holderID, *alias.ID, false)

	// Assert - All sensitive fields should match original
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, originalDocument, *result.Document, "document should decrypt correctly")
	assert.Equal(t, originalAccount, *result.BankingDetails.Account, "banking account should decrypt correctly")
	assert.Equal(t, originalIBAN, *result.BankingDetails.IBAN, "IBAN should decrypt correctly")
	assert.Equal(t, originalParticipantDoc, *result.ParticipantDocument, "participant document should decrypt correctly")
}
