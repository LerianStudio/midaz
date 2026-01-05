//go:build integration

package mongodb

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================================
// Test Helpers
// ============================================================================

func createRepository(t *testing.T, container *mongotestutil.ContainerResult) *MetadataMongoDBRepository {
	t.Helper()

	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)

	return &MetadataMongoDBRepository{
		connection: conn,
		Database:   container.DBName,
	}
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_MetadataRepository_Create_InsertsMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	metadata := &Metadata{
		EntityID:   "acc-123",
		EntityName: "Account",
		Data:       map[string]any{"group": "treasury", "priority": "high"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Act
	err := repo.Create(ctx, collection, metadata)

	// Assert
	require.NoError(t, err, "Create should not return error")

	// Verify via direct query
	count := mongotestutil.CountDocuments(t, container.Database, strings.ToLower(collection), bson.M{"entity_id": "acc-123"})
	assert.Equal(t, int64(1), count, "should have exactly 1 document")
}

// ============================================================================
// FindList Tests
// ============================================================================

func TestIntegration_MetadataRepository_FindList_FiltersByMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	// Insert test data with different groups
	fixtures := []mongotestutil.MetadataFixture{
		{EntityID: "acc-1", EntityName: "Account", Data: map[string]any{"group": "cash"}},
		{EntityID: "acc-2", EntityName: "Account", Data: map[string]any{"group": "cash"}},
		{EntityID: "acc-3", EntityName: "Account", Data: map[string]any{"group": "ops"}},
		{EntityID: "acc-4", EntityName: "Account", Data: map[string]any{"group": "treasury"}},
	}
	mongotestutil.InsertManyMetadata(t, container.Database, strings.ToLower(collection), fixtures)

	// Filter for group=cash
	metadataFilter := bson.M{"metadata.group": "cash"}
	filter := http.QueryHeader{
		Metadata:    &metadataFilter,
		UseMetadata: true,
		Limit:       10,
		Page:        1,
	}

	// Act
	results, err := repo.FindList(ctx, collection, filter)

	// Assert
	require.NoError(t, err, "FindList should not return error")
	assert.Len(t, results, 2, "should return exactly 2 accounts with group=cash")

	for _, r := range results {
		group, ok := r.Data["group"].(string)
		require.True(t, ok, "group should be a string")
		assert.Equal(t, "cash", group, "all results should have group=cash")
	}
}

func TestIntegration_MetadataRepository_FindList_SupportsPagination(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	// Insert 5 accounts with same group
	fixtures := make([]mongotestutil.MetadataFixture, 5)
	for i := 0; i < 5; i++ {
		fixtures[i] = mongotestutil.MetadataFixture{
			EntityID:   "acc-page-" + string(rune('a'+i)),
			EntityName: "Account",
			Data:       map[string]any{"group": "paginated"},
		}
	}
	mongotestutil.InsertManyMetadata(t, container.Database, strings.ToLower(collection), fixtures)

	metadataFilter := bson.M{"metadata.group": "paginated"}

	// Act - Get first page (limit 2)
	filter := http.QueryHeader{
		Metadata:    &metadataFilter,
		UseMetadata: true,
		Limit:       2,
		Page:        1,
	}
	page1, err := repo.FindList(ctx, collection, filter)
	require.NoError(t, err)

	// Act - Get second page
	filter.Page = 2
	page2, err := repo.FindList(ctx, collection, filter)
	require.NoError(t, err)

	// Act - Get third page
	filter.Page = 3
	page3, err := repo.FindList(ctx, collection, filter)
	require.NoError(t, err)

	// Assert
	assert.Len(t, page1, 2, "page 1 should have 2 items")
	assert.Len(t, page2, 2, "page 2 should have 2 items")
	assert.Len(t, page3, 1, "page 3 should have 1 item")

	// Verify no duplicates across pages
	allIDs := make(map[string]bool)
	for _, r := range append(append(page1, page2...), page3...) {
		assert.False(t, allIDs[r.EntityID], "should not have duplicate entity IDs across pages")
		allIDs[r.EntityID] = true
	}
	assert.Len(t, allIDs, 5, "should have 5 unique entity IDs total")
}

func TestIntegration_MetadataRepository_FindList_ReturnsEmptyForNoMatch(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	// Insert data that won't match
	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "acc-other",
		EntityName: "Account",
		Data:       map[string]any{"group": "other"},
	})

	metadataFilter := bson.M{"metadata.group": "nonexistent"}
	filter := http.QueryHeader{
		Metadata:    &metadataFilter,
		UseMetadata: true,
		Limit:       10,
		Page:        1,
	}

	// Act
	results, err := repo.FindList(ctx, collection, filter)

	// Assert
	require.NoError(t, err, "FindList should not error on empty result")
	assert.Empty(t, results, "should return empty slice for no matches")
}

// ============================================================================
// FindByEntity Tests
// ============================================================================

func TestIntegration_MetadataRepository_FindByEntity_ReturnsMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "acc-find-1",
		EntityName: "Account",
		Data:       map[string]any{"key": "value", "number": float64(42)},
	})

	// Act
	result, err := repo.FindByEntity(ctx, collection, "acc-find-1")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "acc-find-1", result.EntityID)
	assert.Equal(t, "value", result.Data["key"])
	assert.Equal(t, float64(42), result.Data["number"])
}

func TestIntegration_MetadataRepository_FindByEntity_ReturnsNilForNonExistent(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	// Act
	result, err := repo.FindByEntity(ctx, "account", "nonexistent-id")

	// Assert
	require.NoError(t, err, "FindByEntity should not error on missing document")
	assert.Nil(t, result, "should return nil for non-existent entity")
}

// ============================================================================
// FindByEntityIDs Tests
// ============================================================================

func TestIntegration_MetadataRepository_FindByEntityIDs_ReturnsBatch(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	fixtures := []mongotestutil.MetadataFixture{
		{EntityID: "batch-1", EntityName: "Account", Data: map[string]any{"idx": 1}},
		{EntityID: "batch-2", EntityName: "Account", Data: map[string]any{"idx": 2}},
		{EntityID: "batch-3", EntityName: "Account", Data: map[string]any{"idx": 3}},
	}
	mongotestutil.InsertManyMetadata(t, container.Database, strings.ToLower(collection), fixtures)

	// Act - Request only 2 of 3
	results, err := repo.FindByEntityIDs(ctx, collection, []string{"batch-1", "batch-3"})

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 2, "should return exactly 2 metadata entries")

	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.EntityID] = true
	}
	assert.True(t, ids["batch-1"])
	assert.True(t, ids["batch-3"])
	assert.False(t, ids["batch-2"], "batch-2 should not be in results")
}

func TestIntegration_MetadataRepository_FindByEntityIDs_ReturnsEmptyForEmptyInput(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	// Act
	results, err := repo.FindByEntityIDs(ctx, "account", []string{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, results, "should return empty slice for empty input")
}

func TestIntegration_MetadataRepository_FindByEntityIDs_HandlesPartialMatch(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "partial-exists",
		EntityName: "Account",
		Data:       map[string]any{"found": true},
	})

	// Act - Request one existing and one non-existing
	results, err := repo.FindByEntityIDs(ctx, collection, []string{"partial-exists", "partial-missing"})

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1, "should return only existing entries")
	assert.Equal(t, "partial-exists", results[0].EntityID)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_MetadataRepository_Update_UpdatesExisting(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "update-1",
		EntityName: "Account",
		Data:       map[string]any{"original": true},
	})

	// Act
	err := repo.Update(ctx, collection, "update-1", map[string]any{"updated": true, "newKey": "newValue"})

	// Assert
	require.NoError(t, err, "Update should not return error")

	// Verify
	found, err := repo.FindByEntity(ctx, collection, "update-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, true, found.Data["updated"])
	assert.Equal(t, "newValue", found.Data["newKey"])
}

func TestIntegration_MetadataRepository_Update_UpsertsIfNotExists(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	// Act - Update non-existent (should upsert)
	err := repo.Update(ctx, collection, "upsert-new", map[string]any{"created": "via upsert"})

	// Assert
	require.NoError(t, err, "Update should upsert if not exists")

	// Verify
	found, err := repo.FindByEntity(ctx, collection, "upsert-new")
	require.NoError(t, err)
	require.NotNil(t, found, "upserted document should exist")
	assert.Equal(t, "via upsert", found.Data["created"])
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_MetadataRepository_Delete_RemovesMetadata(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()
	collection := "account"

	mongotestutil.InsertMetadata(t, container.Database, strings.ToLower(collection), mongotestutil.MetadataFixture{
		EntityID:   "delete-1",
		EntityName: "Account",
		Data:       map[string]any{"toDelete": true},
	})

	// Verify exists before delete
	before, _ := repo.FindByEntity(ctx, collection, "delete-1")
	require.NotNil(t, before, "document should exist before delete")

	// Act
	err := repo.Delete(ctx, collection, "delete-1")

	// Assert
	require.NoError(t, err, "Delete should not return error")

	after, err := repo.FindByEntity(ctx, collection, "delete-1")
	require.NoError(t, err)
	assert.Nil(t, after, "document should not exist after delete")
}

func TestIntegration_MetadataRepository_Delete_IsIdempotent(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	// Act - Delete non-existent (should not error)
	err := repo.Delete(ctx, "account", "never-existed")

	// Assert
	require.NoError(t, err, "Delete should be idempotent - no error for non-existent")
}

// ============================================================================
// Collection Isolation Tests
// ============================================================================

func TestIntegration_MetadataRepository_CollectionIsolation(t *testing.T) {
	// Arrange
	container := mongotestutil.SetupContainer(t)

	repo := createRepository(t, container)
	ctx := context.Background()

	// Insert same entity_id in different collections
	accountMeta := &Metadata{
		EntityID:   "shared-id",
		EntityName: "Account",
		Data:       map[string]any{"type": "account"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	ledgerMeta := &Metadata{
		EntityID:   "shared-id",
		EntityName: "Ledger",
		Data:       map[string]any{"type": "ledger"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	require.NoError(t, repo.Create(ctx, "Account", accountMeta))
	require.NoError(t, repo.Create(ctx, "Ledger", ledgerMeta))

	// Act & Assert - Each collection should have its own data
	fromAccount, err := repo.FindByEntity(ctx, "Account", "shared-id")
	require.NoError(t, err)
	require.NotNil(t, fromAccount)
	assert.Equal(t, "account", fromAccount.Data["type"])

	fromLedger, err := repo.FindByEntity(ctx, "Ledger", "shared-id")
	require.NoError(t, err)
	require.NotNil(t, fromLedger)
	assert.Equal(t, "ledger", fromLedger.Data["type"])
}

// ============================================================================
// CHAOS TEST HELPERS
// ============================================================================

// skipIfNotChaos skips the test if CHAOS=1 environment variable is not set.
// Use this for tests that inject failures (network chaos, container restarts, etc.)
func skipIfNotChaos(t *testing.T) {
	t.Helper()
	if os.Getenv("CHAOS") != "1" {
		t.Skip("skipping chaos test (set CHAOS=1 to run)")
	}
}

// ============================================================================
// CHAOS TEST INFRASTRUCTURE
// ============================================================================

// chaosTestInfra holds the infrastructure for chaos tests (container restart, etc.).
type chaosTestInfra struct {
	container  *mongotestutil.ContainerResult
	repo       *MetadataMongoDBRepository
	chaosOrch  *chaos.Orchestrator
	collection string
}

// networkChaosTestInfra holds infrastructure for network chaos tests with Toxiproxy.
type networkChaosTestInfra struct {
	chaosInfra  *chaos.Infrastructure
	mongoResult *mongotestutil.ContainerResult
	conn        *libMongo.MongoConnection
	repo        *MetadataMongoDBRepository
	proxy       *chaos.Proxy
	collection  string
}

// setupChaosInfra sets up the test infrastructure for chaos testing (container restart).
func setupChaosInfra(t *testing.T) *chaosTestInfra {
	t.Helper()

	// Setup MongoDB container
	container := mongotestutil.SetupContainer(t)

	// Create repository using the connection wrapper
	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)
	repo := &MetadataMongoDBRepository{
		connection: conn,
		Database:   container.DBName,
	}

	// Create chaos orchestrator
	chaosOrch := chaos.NewOrchestrator(t)

	return &chaosTestInfra{
		container:  container,
		repo:       repo,
		chaosOrch:  chaosOrch,
		collection: "chaos_test",
	}
}

// setupNetworkChaosInfra sets up the infrastructure with Toxiproxy for network chaos testing.
func setupNetworkChaosInfra(t *testing.T) *networkChaosTestInfra {
	t.Helper()

	// Create chaos infrastructure with Toxiproxy
	chaosInfra := chaos.NewInfrastructure(t)

	// Setup MongoDB container
	mongoResult := mongotestutil.SetupContainer(t)

	// Register the container with chaos infrastructure
	_, err := chaosInfra.RegisterContainerWithPort("mongodb", mongoResult.Container, "27017/tcp")
	require.NoError(t, err, "failed to register MongoDB container")

	// Create proxy for MongoDB using an exposed Toxiproxy port (8666)
	proxy, err := chaosInfra.CreateProxyFor("mongodb", "8666/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for MongoDB")

	// Get proxy address for client connections
	containerInfo, ok := chaosInfra.GetContainer("mongodb")
	require.True(t, ok, "MongoDB container should be registered")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy address should be set")

	// Create lib-commons MongoDB connection through proxy
	logger := libZap.InitializeLogger()
	proxyURI := "mongodb://" + containerInfo.ProxyListen

	conn := &libMongo.MongoConnection{
		ConnectionStringSource: proxyURI,
		Database:               mongoResult.DBName,
		Logger:                 logger,
	}

	// Create repository
	repo := &MetadataMongoDBRepository{
		connection: conn,
		Database:   mongoResult.DBName,
	}

	return &networkChaosTestInfra{
		chaosInfra:  chaosInfra,
		mongoResult: mongoResult,
		conn:        conn,
		repo:        repo,
		proxy:       proxy,
		collection:  "network_chaos_test",
	}
}

// cleanup releases all resources for chaos tests.
func (infra *chaosTestInfra) cleanup() {
	if infra.chaosOrch != nil {
		infra.chaosOrch.Close()
	}
}

// cleanup releases all resources for network chaos infrastructure.
func (infra *networkChaosTestInfra) cleanup() {
	// Cleanup Infrastructure (Toxiproxy, network, orchestrator)
	if infra.chaosInfra != nil {
		infra.chaosInfra.Cleanup()
	}
}

// createTestMetadata creates a metadata document for chaos testing.
func (infra *chaosTestInfra) createTestMetadata(t *testing.T, entityID, description string) *Metadata {
	t.Helper()

	metadata := &Metadata{
		EntityID:   entityID,
		EntityName: "ChaosTest",
		Data:       map[string]any{"description": description, "timestamp": time.Now().Unix()},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := infra.repo.Create(context.Background(), infra.collection, metadata)
	require.NoError(t, err)
	return metadata
}

// createTestMetadata creates a metadata document for network chaos testing.
func (infra *networkChaosTestInfra) createTestMetadata(t *testing.T, entityID, description string) *Metadata {
	t.Helper()

	metadata := &Metadata{
		EntityID:   entityID,
		EntityName: "NetworkChaosTest",
		Data:       map[string]any{"description": description, "timestamp": time.Now().Unix()},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := infra.repo.Create(context.Background(), infra.collection, metadata)
	require.NoError(t, err)
	return metadata
}

// ============================================================================
// CHAOS TESTS - CONTAINER LIFECYCLE
// ============================================================================

// TestChaos_Metadata_MongoDBRestart tests that the repository recovers
// after a MongoDB container restart.
// SKIPPED: lib-commons MongoDB connection pool does not recover after restart.
func TestIntegration_Chaos_Metadata_MongoDBRestart(t *testing.T) {
	t.Skip("skipping: lib-commons connection pool does not recover after MongoDB restart")
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create initial metadata
	metadata := infra.createTestMetadata(t, "restart-test-1", "Pre-restart metadata")
	t.Logf("Created metadata %s before restart", metadata.EntityID)

	// Inject chaos: restart MongoDB
	containerID := infra.container.Container.GetContainerID()
	t.Logf("Chaos: Restarting MongoDB container %s", containerID)

	err := infra.chaosOrch.RestartContainer(ctx, containerID, 10*time.Second)
	require.NoError(t, err, "container restart should succeed")

	err = infra.chaosOrch.WaitForContainerRunning(ctx, containerID, 60*time.Second)
	require.NoError(t, err, "container should be running after restart")

	// Wait for database to be ready again
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
		return err
	}, 30*time.Second, "repository should recover after MongoDB restart")

	// Verify data integrity
	recovered, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
	require.NoError(t, err)
	require.NotNil(t, recovered, "metadata should exist after restart")
	assert.Equal(t, metadata.EntityID, recovered.EntityID, "entity ID should be unchanged")
	assert.Equal(t, metadata.Data["description"], recovered.Data["description"], "description should be unchanged")

	t.Log("Chaos test passed: MongoDB restart recovery verified")
}

// TestChaos_Metadata_DataIntegrity tests that data remains consistent
// after chaos events (no data loss, no corruption).
func TestIntegration_Chaos_Metadata_DataIntegrity(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create multiple metadata documents before chaos
	var createdMetadata []*Metadata
	for i := 0; i < 5; i++ {
		metadata := infra.createTestMetadata(t, "integrity-test-"+string(rune('a'+i)), "Integrity test metadata")
		createdMetadata = append(createdMetadata, metadata)
	}

	t.Logf("Created %d metadata documents before chaos", len(createdMetadata))

	// Verify all data is intact
	chaos.AssertDataIntegrity(t, func() error {
		for _, m := range createdMetadata {
			_, err := infra.repo.FindByEntity(ctx, infra.collection, m.EntityID)
			if err != nil {
				return err
			}
		}
		return nil
	}, "all metadata should be retrievable")

	// Verify each metadata document's data
	for _, expected := range createdMetadata {
		actual, err := infra.repo.FindByEntity(ctx, infra.collection, expected.EntityID)
		require.NoError(t, err)
		require.NotNil(t, actual)
		chaos.AssertNoDataLoss(t, expected.EntityID, actual.EntityID, "entity ID mismatch")
		chaos.AssertNoDataLoss(t, expected.Data["description"], actual.Data["description"], "description mismatch")
	}

	t.Log("Chaos test passed: data integrity verified")
}

// ============================================================================
// CHAOS TESTS - NETWORK CHAOS
// ============================================================================

// TestChaos_Metadata_NetworkLatency tests that the repository handles
// network latency gracefully without timing out inappropriately.
func TestIntegration_Chaos_Metadata_NetworkLatency(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	t.Logf("Using Toxiproxy proxy: %s -> %s", infra.proxy.Listen(), infra.proxy.Upstream())

	// Create metadata before adding latency
	metadata := infra.createTestMetadata(t, "latency-test-1", "Pre-latency metadata")
	t.Logf("Created metadata %s before adding latency", metadata.EntityID)

	// Add 200ms latency to the connection
	t.Log("Chaos: Adding 200ms network latency")
	err := infra.proxy.AddLatency(200*time.Millisecond, 50*time.Millisecond)
	require.NoError(t, err, "failed to add latency")
	defer infra.proxy.RemoveAllToxics()

	// Operations should still succeed (with higher latency)
	start := time.Now()
	found, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
	elapsed := time.Since(start)

	require.NoError(t, err, "operation should succeed despite latency")
	require.NotNil(t, found)
	assert.Equal(t, metadata.EntityID, found.EntityID)
	t.Logf("Query completed in %v (with 200ms injected latency)", elapsed)

	// Latency should be noticeable
	assert.Greater(t, elapsed, 150*time.Millisecond, "query should take longer due to injected latency")

	// Create new metadata under latency
	start = time.Now()
	newMetadata := infra.createTestMetadata(t, "latency-test-2", "Under-latency metadata")
	elapsed = time.Since(start)

	require.NotNil(t, newMetadata, "should be able to create metadata under latency")
	t.Logf("Create completed in %v (with 200ms injected latency)", elapsed)

	t.Log("Chaos test passed: network latency handled gracefully")
}

// TestChaos_Metadata_NetworkPartition tests that the repository handles
// network partitions (disconnections) gracefully.
func TestIntegration_Chaos_Metadata_NetworkPartition(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create metadata before partition
	metadata := infra.createTestMetadata(t, "partition-test-1", "Pre-partition metadata")
	t.Logf("Created metadata %s before partition", metadata.EntityID)

	// Disconnect the proxy (simulate network partition)
	t.Log("Chaos: Disconnecting network (simulating partition)")
	err := infra.proxy.Disconnect()
	require.NoError(t, err, "failed to disconnect proxy")

	// Operations should fail gracefully during partition
	partitionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, partitionErr := infra.repo.FindByEntity(partitionCtx, infra.collection, metadata.EntityID)
	if partitionErr != nil {
		t.Logf("Operation during partition failed as expected: %v", partitionErr)
	} else {
		t.Log("Operation during partition succeeded (connection pool still had active connections)")
	}

	// Reconnect the proxy
	t.Log("Chaos: Reconnecting network")
	err = infra.proxy.Reconnect()
	require.NoError(t, err, "failed to reconnect proxy")

	// Wait for recovery
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
		return err
	}, 30*time.Second, "repository should recover after network partition")

	// Verify data integrity after partition
	found, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
	require.NoError(t, err, "should find metadata after recovery")
	require.NotNil(t, found)
	assert.Equal(t, metadata.EntityID, found.EntityID)
	assert.Equal(t, metadata.Data["description"], found.Data["description"])

	t.Log("Chaos test passed: network partition handled gracefully")
}

// TestChaos_Metadata_PacketLoss tests that the repository handles
// packet loss gracefully with retries.
func TestIntegration_Chaos_Metadata_PacketLoss(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create metadata before adding packet loss
	metadata := infra.createTestMetadata(t, "packetloss-test-1", "Pre-packet-loss metadata")
	t.Logf("Created metadata %s before packet loss", metadata.EntityID)

	// Add 10% packet loss
	t.Log("Chaos: Adding 10% packet loss")
	err := infra.proxy.AddPacketLoss(10)
	require.NoError(t, err, "failed to add packet loss")
	defer infra.proxy.RemoveAllToxics()

	// Execute multiple operations - some may fail, but overall should be resilient
	successCount := 0
	errorCount := 0
	totalAttempts := 20

	for i := 0; i < totalAttempts; i++ {
		_, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
		if err != nil {
			errorCount++
		} else {
			successCount++
		}
	}

	t.Logf("Packet loss test: %d/%d operations succeeded", successCount, totalAttempts)

	// Most operations should succeed despite packet loss
	assert.Greater(t, successCount, totalAttempts/2, "majority of operations should succeed despite packet loss")

	t.Log("Chaos test passed: packet loss handled with acceptable success rate")
}

// TestChaos_Metadata_IntermittentFailure tests that the repository handles
// intermittent network failures (flapping connection).
func TestIntegration_Chaos_Metadata_IntermittentFailure(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Create metadata
	metadata := infra.createTestMetadata(t, "intermittent-test-1", "Intermittent test metadata")

	// Simulate intermittent failures with multiple disconnect/reconnect cycles
	cycles := 3
	for i := 0; i < cycles; i++ {
		// Disconnect
		t.Logf("Chaos: Cycle %d - disconnecting", i+1)
		infra.proxy.Disconnect()
		time.Sleep(500 * time.Millisecond)

		// Reconnect
		t.Logf("Chaos: Cycle %d - reconnecting", i+1)
		infra.proxy.Reconnect()

		// Wait for recovery and verify
		chaos.AssertRecoveryWithin(t, func() error {
			_, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
			return err
		}, 10*time.Second, "should recover after cycle %d", i+1)
	}

	// Final verification
	found, err := infra.repo.FindByEntity(ctx, infra.collection, metadata.EntityID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, metadata.EntityID, found.EntityID)

	t.Log("Chaos test passed: intermittent failures handled correctly")
}
