// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package holder

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
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

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_HolderRepo_Create(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-" + uuid.New().String()[:8]
	originalName := "John Doe"
	originalDocument := "12345678901"

	holder := mongotestutil.CreateTestHolderSimple(t, originalName, originalDocument)

	// Act
	result, err := repo.Create(ctx, organizationID, holder)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, result)
	assert.Equal(t, holder.ID, result.ID)
	assert.Equal(t, originalName, *result.Name, "returned name should be decrypted")
	assert.Equal(t, originalDocument, *result.Document, "returned document should be decrypted")

	// Verify via direct query
	collName := strings.ToLower("holders_" + organizationID)
	count := mongotestutil.CountDocuments(t, container.Database, collName, bson.M{"_id": holder.ID})
	assert.Equal(t, int64(1), count, "should have exactly 1 document")
}

func TestIntegration_HolderRepo_Create_EncryptsData(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-encrypt-" + uuid.New().String()[:8]
	originalName := "Encrypted User"
	originalDocument := "99988877766"

	holder := mongotestutil.CreateTestHolderWithContact(t, originalName, originalDocument)

	// Act
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	// Assert - Query raw document to verify encryption
	collName := strings.ToLower("holders_" + organizationID)
	var rawDoc bson.M
	err = container.Database.Collection(collName).FindOne(ctx, bson.M{"_id": holder.ID}).Decode(&rawDoc)
	require.NoError(t, err)

	// Name should be encrypted (not equal to original)
	storedName, ok := rawDoc["name"].(string)
	require.True(t, ok, "name should be stored as string")
	assert.NotEqual(t, originalName, storedName, "name should be encrypted in storage")

	// Document should be encrypted (not equal to original)
	storedDoc, ok := rawDoc["document"].(string)
	require.True(t, ok, "document should be stored as string")
	assert.NotEqual(t, originalDocument, storedDoc, "document should be encrypted in storage")

	// Search hash should be present
	search, ok := rawDoc["search"].(bson.M)
	require.True(t, ok, "search map should exist")
	assert.NotEmpty(t, search["document"], "document hash should be generated")
}

func TestIntegration_HolderRepo_Create_DuplicateDocument(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-dup-" + uuid.New().String()[:8]
	sharedDocument := "11111111111"

	// Create first holder
	holder1 := mongotestutil.CreateTestHolderSimple(t, "First User", sharedDocument)
	_, err := repo.Create(ctx, organizationID, holder1)
	require.NoError(t, err, "first create should succeed")

	// Act - Try to create second holder with same document
	holder2 := mongotestutil.CreateTestHolderSimple(t, "Second User", sharedDocument)
	_, err = repo.Create(ctx, organizationID, holder2)

	// Assert
	require.Error(t, err, "duplicate document should fail")
	var conflictErr pkg.EntityConflictError
	require.ErrorAs(t, err, &conflictErr, "should return EntityConflictError")
	assert.Equal(t, "CRM-0010", conflictErr.Code, "should return ErrDocumentAssociationError code")
}

func TestIntegration_HolderRepo_Create_WithAllFields(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-complete-" + uuid.New().String()[:8]

	holder := mongotestutil.CreateCompleteTestHolder(t, "Complete User", "55566677788")

	// Act
	result, err := repo.Create(ctx, organizationID, holder)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, result)

	// Verify basic fields
	assert.Equal(t, holder.ID, result.ID)
	assert.Equal(t, *holder.Name, *result.Name)
	assert.Equal(t, *holder.Document, *result.Document)
	assert.Equal(t, *holder.ExternalID, *result.ExternalID)

	// Verify addresses
	require.NotNil(t, result.Addresses)
	require.NotNil(t, result.Addresses.Primary)
	assert.Equal(t, holder.Addresses.Primary.Line1, result.Addresses.Primary.Line1)

	// Verify contact (decrypted)
	require.NotNil(t, result.Contact)
	assert.Equal(t, *holder.Contact.PrimaryEmail, *result.Contact.PrimaryEmail)

	// Verify natural person (decrypted)
	require.NotNil(t, result.NaturalPerson)
	assert.Equal(t, *holder.NaturalPerson.MotherName, *result.NaturalPerson.MotherName)
	assert.Equal(t, *holder.NaturalPerson.FatherName, *result.NaturalPerson.FatherName)
}

func TestIntegration_HolderRepo_Create_WithLegalPerson(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-legal-" + uuid.New().String()[:8]

	holder := mongotestutil.CreateTestHolderWithLegalPerson(t, "ACME Corp", "12345678000199")

	// Act
	result, err := repo.Create(ctx, organizationID, holder)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, result)

	// Verify legal person fields
	require.NotNil(t, result.LegalPerson)
	assert.Equal(t, *holder.LegalPerson.TradeName, *result.LegalPerson.TradeName)
	assert.Equal(t, *holder.LegalPerson.FoundingDate, *result.LegalPerson.FoundingDate)

	// Verify representative (decrypted)
	require.NotNil(t, result.LegalPerson.Representative)
	assert.Equal(t, *holder.LegalPerson.Representative.Name, *result.LegalPerson.Representative.Name)
	assert.Equal(t, *holder.LegalPerson.Representative.Document, *result.LegalPerson.Representative.Document)
	assert.Equal(t, *holder.LegalPerson.Representative.Email, *result.LegalPerson.Representative.Email)
}

// ============================================================================
// Find Tests
// ============================================================================

func TestIntegration_HolderRepo_Find(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-find-" + uuid.New().String()[:8]
	originalName := "Find Test User"
	originalDocument := "44455566677"

	holder := mongotestutil.CreateTestHolderWithContact(t, originalName, originalDocument)
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	// Act
	result, err := repo.Find(ctx, organizationID, *holder.ID, false)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, holder.ID, result.ID)
	assert.Equal(t, originalName, *result.Name, "name should be decrypted")
	assert.Equal(t, originalDocument, *result.Document, "document should be decrypted")
	assert.Equal(t, *holder.Contact.PrimaryEmail, *result.Contact.PrimaryEmail, "contact email should be decrypted")
}

func TestIntegration_HolderRepo_Find_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-notfound-" + uuid.New().String()[:8]
	nonExistentID := uuid.New()

	// Act
	result, err := repo.Find(ctx, organizationID, nonExistentID, false)

	// Assert
	require.Error(t, err, "should return error for non-existent holder")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "holder", "should return ErrHolderNotFound")
}

func TestIntegration_HolderRepo_Find_ExcludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-deleted-" + uuid.New().String()[:8]

	holder := mongotestutil.CreateTestHolderSimple(t, "Deleted User", "77788899900")
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	// Soft delete
	err = repo.Delete(ctx, organizationID, *holder.ID, false)
	require.NoError(t, err)

	// Act - Find without includeDeleted
	result, err := repo.Find(ctx, organizationID, *holder.ID, false)

	// Assert
	require.Error(t, err, "should not find soft-deleted holder")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "holder")
}

func TestIntegration_HolderRepo_Find_IncludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-incldel-" + uuid.New().String()[:8]

	holder := mongotestutil.CreateTestHolderSimple(t, "Include Deleted User", "66655544433")
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	// Soft delete
	err = repo.Delete(ctx, organizationID, *holder.ID, false)
	require.NoError(t, err)

	// Act - Find with includeDeleted=true
	result, err := repo.Find(ctx, organizationID, *holder.ID, true)

	// Assert
	require.NoError(t, err, "should find soft-deleted holder with includeDeleted=true")
	require.NotNil(t, result)
	assert.NotNil(t, result.DeletedAt, "deleted_at should be set")
}

// ============================================================================
// FindAll Tests
// ============================================================================

func TestIntegration_HolderRepo_FindAll(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-findall-" + uuid.New().String()[:8]

	// Create multiple holders
	for i := 0; i < 5; i++ {
		holder := mongotestutil.CreateTestHolderSimple(t, fmt.Sprintf("User %d", i), fmt.Sprintf("1111111111%d", i))
		_, err := repo.Create(ctx, organizationID, holder)
		require.NoError(t, err)
	}

	// Act
	filter := http.QueryHeader{Limit: 10, Page: 1}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 5, "should return all 5 holders")
}

func TestIntegration_HolderRepo_FindAll_Pagination(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-page-" + uuid.New().String()[:8]

	// Create 5 holders
	for i := 0; i < 5; i++ {
		holder := mongotestutil.CreateTestHolderSimple(t, fmt.Sprintf("Page User %d", i), fmt.Sprintf("2222222222%d", i))
		_, err := repo.Create(ctx, organizationID, holder)
		require.NoError(t, err)
	}

	// Act - Get page 1 (limit 2)
	page1, err := repo.FindAll(ctx, organizationID, http.QueryHeader{Limit: 2, Page: 1}, false)
	require.NoError(t, err)

	// Act - Get page 2
	page2, err := repo.FindAll(ctx, organizationID, http.QueryHeader{Limit: 2, Page: 2}, false)
	require.NoError(t, err)

	// Act - Get page 3
	page3, err := repo.FindAll(ctx, organizationID, http.QueryHeader{Limit: 2, Page: 3}, false)
	require.NoError(t, err)

	// Assert
	assert.Len(t, page1, 2, "page 1 should have 2 items")
	assert.Len(t, page2, 2, "page 2 should have 2 items")
	assert.Len(t, page3, 1, "page 3 should have 1 item")

	// Verify no duplicates
	allIDs := make(map[uuid.UUID]bool)
	for _, r := range slices.Concat(page1, page2, page3) {
		assert.False(t, allIDs[*r.ID], "should not have duplicates")
		allIDs[*r.ID] = true
	}
	assert.Len(t, allIDs, 5)
}

func TestIntegration_HolderRepo_FindAll_FilterByExternalID(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-filterext-" + uuid.New().String()[:8]
	targetExternalID := "EXT-TARGET-123"

	// Create holders with different external IDs
	holder1 := mongotestutil.CreateTestHolderWithExternalID(t, "Target User", "33344455566", targetExternalID)
	_, err := repo.Create(ctx, organizationID, holder1)
	require.NoError(t, err)

	holder2 := mongotestutil.CreateTestHolderWithExternalID(t, "Other User", "99988877766", "EXT-OTHER-456")
	_, err = repo.Create(ctx, organizationID, holder2)
	require.NoError(t, err)

	// Act - Filter by external_id
	filter := http.QueryHeader{
		Limit:      10,
		Page:       1,
		ExternalID: testutils.Ptr(targetExternalID),
	}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should return only holder with matching external_id")
	assert.Equal(t, targetExternalID, *results[0].ExternalID)
}

func TestIntegration_HolderRepo_FindAll_FilterByDocument(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-filterdoc-" + uuid.New().String()[:8]
	targetDocument := "55566677788"

	// Create holders with different documents
	holder1 := mongotestutil.CreateTestHolderSimple(t, "Target Doc User", targetDocument)
	_, err := repo.Create(ctx, organizationID, holder1)
	require.NoError(t, err)

	holder2 := mongotestutil.CreateTestHolderSimple(t, "Other Doc User", "11122233344")
	_, err = repo.Create(ctx, organizationID, holder2)
	require.NoError(t, err)

	// Act - Filter by document (uses hash matching)
	filter := http.QueryHeader{
		Limit:    10,
		Page:     1,
		Document: testutils.Ptr(targetDocument),
	}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should return only holder with matching document")
	assert.Equal(t, targetDocument, *results[0].Document)
}

func TestIntegration_HolderRepo_FindAll_FilterByMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-filtermeta-" + uuid.New().String()[:8]

	// Create holder with specific metadata
	holder1 := mongotestutil.CreateTestHolderSimple(t, "Metadata User 1", "77788899900")
	holder1.Metadata = map[string]any{"region": "us-east", "priority": "high"}
	_, err := repo.Create(ctx, organizationID, holder1)
	require.NoError(t, err)

	holder2 := mongotestutil.CreateTestHolderSimple(t, "Metadata User 2", "00011122233")
	holder2.Metadata = map[string]any{"region": "eu-west", "priority": "low"}
	_, err = repo.Create(ctx, organizationID, holder2)
	require.NoError(t, err)

	// Act - Filter by metadata.region
	filter := http.QueryHeader{
		Limit: 10,
		Page:  1,
		Metadata: &bson.M{
			"metadata.region": "us-east",
		},
	}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should return only holder with matching metadata")
	assert.Equal(t, holder1.ID, results[0].ID)
}

func TestIntegration_HolderRepo_FindAll_ReturnsEmpty(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-empty-" + uuid.New().String()[:8]

	// Act - Query empty collection
	filter := http.QueryHeader{Limit: 10, Page: 1}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err, "should not error on empty result")
	assert.Empty(t, results)
}

func TestIntegration_HolderRepo_FindAll_ExcludesDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-findalldel-" + uuid.New().String()[:8]

	// Create holders
	holder1 := mongotestutil.CreateTestHolderSimple(t, "Delete Test User 1", "44455566677")
	_, err := repo.Create(ctx, organizationID, holder1)
	require.NoError(t, err)

	holder2 := mongotestutil.CreateTestHolderSimple(t, "Delete Test User 2", "88899900011")
	_, err = repo.Create(ctx, organizationID, holder2)
	require.NoError(t, err)

	// Soft delete one
	err = repo.Delete(ctx, organizationID, *holder1.ID, false)
	require.NoError(t, err)

	// Act
	filter := http.QueryHeader{Limit: 10, Page: 1}
	results, err := repo.FindAll(ctx, organizationID, filter, false)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should exclude soft-deleted holder")
	assert.Equal(t, holder2.ID, results[0].ID)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_HolderRepo_Update(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-update-" + uuid.New().String()[:8]

	holder := mongotestutil.CreateTestHolderSimple(t, "Original Name", "88899900011")
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	// Act - Update metadata
	updatedHolder := &mmodel.Holder{
		Metadata: map[string]any{"updated": true, "version": 2},
	}
	result, err := repo.Update(ctx, organizationID, *holder.ID, updatedHolder, nil)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result.Metadata["updated"])
	assert.EqualValues(t, 2, result.Metadata["version"]) // Use EqualValues for BSON type handling
}

func TestIntegration_HolderRepo_Update_FieldsToRemove(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-remove-" + uuid.New().String()[:8]

	holder := mongotestutil.CreateTestHolderSimple(t, "Remove Fields User", "77766655544")
	holder.Metadata = map[string]any{"key1": "value1", "key2": "value2", "key3": "value3"}
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	// Act - Remove metadata.key1
	result, err := repo.Update(ctx, organizationID, *holder.ID, &mmodel.Holder{}, []string{"metadata.key1"})

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	_, hasKey1 := result.Metadata["key1"]
	assert.False(t, hasKey1, "key1 should be removed")
	assert.Equal(t, "value2", result.Metadata["key2"], "key2 should still exist")
	assert.Equal(t, "value3", result.Metadata["key3"], "key3 should still exist")
}

func TestIntegration_HolderRepo_Update_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-updatenotfound-" + uuid.New().String()[:8]
	nonExistentID := uuid.New()

	// Act - Try to update non-existent holder
	updateData := &mmodel.Holder{
		Metadata: map[string]any{"key": "value"},
	}
	result, err := repo.Update(ctx, organizationID, nonExistentID, updateData, nil)

	// Assert
	require.Error(t, err, "should return error for non-existent holder")
	assert.Nil(t, result, "result should be nil")

	var notFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFoundErr, "should return EntityNotFoundError")
	assert.Equal(t, "CRM-0006", notFoundErr.Code, "should return ErrHolderNotFound code")
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_HolderRepo_Delete_Soft(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-softdel-" + uuid.New().String()[:8]

	holder := mongotestutil.CreateTestHolderSimple(t, "Soft Delete User", "55544433322")
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	// Act - Soft delete
	err = repo.Delete(ctx, organizationID, *holder.ID, false)

	// Assert
	require.NoError(t, err)

	// Verify document still exists with deleted_at set
	result, err := repo.Find(ctx, organizationID, *holder.ID, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.DeletedAt, "deleted_at should be set")
}

func TestIntegration_HolderRepo_Delete_Hard(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-harddel-" + uuid.New().String()[:8]

	holder := mongotestutil.CreateTestHolderSimple(t, "Hard Delete User", "22211100099")
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	// Act - Hard delete
	err = repo.Delete(ctx, organizationID, *holder.ID, true)

	// Assert
	require.NoError(t, err)

	// Verify document is completely removed
	collName := strings.ToLower("holders_" + organizationID)
	count := mongotestutil.CountDocuments(t, container.Database, collName, bson.M{"_id": holder.ID})
	assert.Equal(t, int64(0), count, "document should be removed from collection")
}

func TestIntegration_HolderRepo_Delete_NotFound(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-delnotfound-" + uuid.New().String()[:8]
	nonExistentID := uuid.New()

	// Act
	err := repo.Delete(ctx, organizationID, nonExistentID, false)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holder", "should return ErrHolderNotFound")
}

func TestIntegration_HolderRepo_Delete_AlreadyDeleted(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-delalready-" + uuid.New().String()[:8]

	holder := mongotestutil.CreateTestHolderSimple(t, "Already Deleted User", "99900011122")
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	// Soft delete first time
	err = repo.Delete(ctx, organizationID, *holder.ID, false)
	require.NoError(t, err)

	// Act - Try to soft delete again
	err = repo.Delete(ctx, organizationID, *holder.ID, false)

	// Assert
	require.Error(t, err, "should not be able to delete already deleted record")
	assert.Contains(t, err.Error(), "holder")
}

// ============================================================================
// Encryption Round-Trip Tests
// ============================================================================

func TestIntegration_HolderRepo_EncryptionRoundTrip(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-roundtrip-" + uuid.New().String()[:8]

	// Original values
	originalName := "Round Trip Test User"
	originalDocument := "98765432100"
	originalEmail := "roundtrip@example.com"
	originalPhone := "+5511999888777"
	originalMotherName := "Round Trip Mother"
	originalFatherName := "Round Trip Father"

	holder := mongotestutil.CreateTestHolderSimple(t, originalName, originalDocument)
	holder.Contact = &mmodel.Contact{
		PrimaryEmail: testutils.Ptr(originalEmail),
		MobilePhone:  testutils.Ptr(originalPhone),
	}
	holder.NaturalPerson = &mmodel.NaturalPerson{
		FavoriteName: testutils.Ptr("RT"),
		MotherName:   testutils.Ptr(originalMotherName),
		FatherName:   testutils.Ptr(originalFatherName),
	}

	// Act - Create and retrieve
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	result, err := repo.Find(ctx, organizationID, *holder.ID, false)

	// Assert - All sensitive fields should match original
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, originalName, *result.Name, "name should decrypt correctly")
	assert.Equal(t, originalDocument, *result.Document, "document should decrypt correctly")
	assert.Equal(t, originalEmail, *result.Contact.PrimaryEmail, "email should decrypt correctly")
	assert.Equal(t, originalPhone, *result.Contact.MobilePhone, "phone should decrypt correctly")
	assert.Equal(t, originalMotherName, *result.NaturalPerson.MotherName, "mother name should decrypt correctly")
	assert.Equal(t, originalFatherName, *result.NaturalPerson.FatherName, "father name should decrypt correctly")
}

func TestIntegration_HolderRepo_EncryptionRoundTrip_LegalPerson(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-roundtrip-legal-" + uuid.New().String()[:8]

	// Original values
	originalName := "Legal Round Trip Corp"
	originalDocument := "12345678000199"
	originalRepName := "Legal Representative"
	originalRepDocument := "11122233344"
	originalRepEmail := "rep@legalroundtrip.com"

	holder := mongotestutil.CreateTestHolderWithLegalPerson(t, originalName, originalDocument)
	holder.LegalPerson.Representative.Name = testutils.Ptr(originalRepName)
	holder.LegalPerson.Representative.Document = testutils.Ptr(originalRepDocument)
	holder.LegalPerson.Representative.Email = testutils.Ptr(originalRepEmail)

	// Act - Create and retrieve
	_, err := repo.Create(ctx, organizationID, holder)
	require.NoError(t, err)

	result, err := repo.Find(ctx, organizationID, *holder.ID, false)

	// Assert - All sensitive fields should match original
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, originalName, *result.Name, "name should decrypt correctly")
	assert.Equal(t, originalDocument, *result.Document, "document should decrypt correctly")

	require.NotNil(t, result.LegalPerson)
	require.NotNil(t, result.LegalPerson.Representative)
	assert.Equal(t, originalRepName, *result.LegalPerson.Representative.Name, "rep name should decrypt correctly")
	assert.Equal(t, originalRepDocument, *result.LegalPerson.Representative.Document, "rep document should decrypt correctly")
	assert.Equal(t, originalRepEmail, *result.LegalPerson.Representative.Email, "rep email should decrypt correctly")
}

// ============================================================================
// Index Constraint Tests
// ============================================================================

func TestIntegration_HolderRepo_Create_SameDocumentDifferentOrganizations(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	sharedDocument := "33344455566"

	// Create holder in first organization
	org1 := "org-1-" + uuid.New().String()[:8]
	holder1 := mongotestutil.CreateTestHolderSimple(t, "Org1 User", sharedDocument)
	_, err := repo.Create(ctx, org1, holder1)
	require.NoError(t, err, "first org create should succeed")

	// Act - Create holder with same document in different organization
	org2 := "org-2-" + uuid.New().String()[:8]
	holder2 := mongotestutil.CreateTestHolderSimple(t, "Org2 User", sharedDocument)
	_, err = repo.Create(ctx, org2, holder2)

	// Assert - Should succeed since different collections
	require.NoError(t, err, "same document in different organization should succeed")
}

func TestIntegration_HolderRepo_Create_ReuseSoftDeletedDocument(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()

	organizationID := "org-reuse-" + uuid.New().String()[:8]
	reusedDocument := "77788899900"

	// Create and soft delete first holder
	holder1 := mongotestutil.CreateTestHolderSimple(t, "First User", reusedDocument)
	_, err := repo.Create(ctx, organizationID, holder1)
	require.NoError(t, err)

	err = repo.Delete(ctx, organizationID, *holder1.ID, false)
	require.NoError(t, err)

	// Act - Create new holder with same document
	holder2 := mongotestutil.CreateTestHolderSimple(t, "Second User", reusedDocument)
	_, err = repo.Create(ctx, organizationID, holder2)

	// Assert - Should succeed since first was soft deleted
	// (partial filter expression on index excludes deleted records)
	require.NoError(t, err, "reusing document from soft-deleted holder should succeed")
}
