// Package metadata provides metadata validation adapters for reconciliation.
package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Metadata check threshold constants.
const (
	metadataWarningThreshold = 10
	defaultLookbackDays      = 7
	defaultMaxScan           = 5000
)

// MetadataChecker validates metadata integrity across MongoDB and the outbox.
type MetadataChecker struct {
	db               *sql.DB
	onboardingMongo  *mongo.Database
	transactionMongo *mongo.Database
	findIDs          func(ctx context.Context, db *mongo.Database, collection string, ids []string) (map[string]struct{}, error)
}

// NewMetadataChecker creates a new metadata checker.
func NewMetadataChecker(db *sql.DB, onboardingMongo, transactionMongo *mongo.Database) *MetadataChecker {
	return &MetadataChecker{
		db:               db,
		onboardingMongo:  onboardingMongo,
		transactionMongo: transactionMongo,
		findIDs:          mongoFindEntityIDs,
	}
}

// Name returns the unique name of this checker.
func (c *MetadataChecker) Name() string {
	return postgres.CheckerNameMetadata
}

// Check validates metadata integrity.
func (c *MetadataChecker) Check(ctx context.Context, config postgres.CheckerConfig) (postgres.CheckResult, error) {
	if c.onboardingMongo == nil && c.transactionMongo == nil {
		return &domain.MetadataCheckResult{Status: domain.StatusSkipped}, nil
	}

	lookbackDays, maxScan := c.normalizeConfig(config)
	result := &domain.MetadataCheckResult{}

	if err := c.collectSummaries(ctx, result); err != nil {
		return nil, err
	}

	if err := c.checkOutboxIfNeeded(ctx, result, lookbackDays, maxScan, config.MaxResults); err != nil {
		return nil, err
	}

	c.determineStatus(result)

	return result, nil
}

// normalizeConfig returns normalized lookback days and max scan values.
func (c *MetadataChecker) normalizeConfig(config postgres.CheckerConfig) (int, int) {
	lookbackDays := config.LookbackDays
	if lookbackDays <= 0 {
		lookbackDays = defaultLookbackDays
	}

	maxScan := config.MetadataMaxScan
	if maxScan <= 0 {
		maxScan = defaultMaxScan
	}

	return lookbackDays, maxScan
}

// collectSummaries collects collection summaries and populates the result.
func (c *MetadataChecker) collectSummaries(ctx context.Context, result *domain.MetadataCheckResult) error {
	summaries, missingEntityIDs, duplicateEntityIDs, missingRequiredFields, emptyMetadata, err := c.collectCollectionSummaries(ctx)
	if err != nil {
		return err
	}

	result.CollectionSummaries = summaries
	result.MissingEntityIDs = missingEntityIDs
	result.DuplicateEntityIDs = duplicateEntityIDs
	result.MissingRequiredFields = missingRequiredFields
	result.EmptyMetadata = emptyMetadata

	return nil
}

// checkOutboxIfNeeded checks outbox published entries if the database is available.
func (c *MetadataChecker) checkOutboxIfNeeded(ctx context.Context, result *domain.MetadataCheckResult, lookbackDays, maxScan, maxResults int) error {
	if c.db == nil || (c.transactionMongo == nil && c.onboardingMongo == nil) {
		return nil
	}

	outboxRes, err := c.checkOutboxPublished(ctx, lookbackDays, maxScan, maxResults)
	if err != nil {
		return err
	}

	result.PostgreSQLCount = int64(outboxRes.ValidatedCount)
	result.MissingCount = int64(outboxRes.MissingCount)
	result.MongoDBCount = int64(outboxRes.FoundCount)
	result.MissingEntities = outboxRes.MissingEntities

	if outboxRes.ScannedCount < outboxRes.TotalCount {
		result.ScanLimited = true
	}

	return nil
}

// determineStatus sets the result status based on issue counts.
func (c *MetadataChecker) determineStatus(result *domain.MetadataCheckResult) {
	issueCount := result.MissingCount +
		int64(result.MissingEntityIDs) +
		int64(result.DuplicateEntityIDs) +
		int64(result.MissingRequiredFields)

	result.Status = postgres.DetermineStatus(int(issueCount), postgres.StatusThresholds{
		WarningThreshold: metadataWarningThreshold,
	})

	if result.MissingCount > 0 {
		result.Status = domain.StatusCritical
	}
}

func (c *MetadataChecker) collectCollectionSummaries(ctx context.Context) ([]domain.MetadataCollectionSummary, int, int, int, int, error) {
	requiredFields := []string{"entity_id", "entity_name", "created_at", "updated_at"}

	type collectionInfo struct {
		db   *mongo.Database
		name string
	}

	collections := []collectionInfo{
		{db: c.onboardingMongo, name: "organization"},
		{db: c.onboardingMongo, name: "ledger"},
		{db: c.onboardingMongo, name: "asset"},
		{db: c.onboardingMongo, name: "account"},
		{db: c.onboardingMongo, name: "portfolio"},
		{db: c.onboardingMongo, name: "segment"},
		{db: c.onboardingMongo, name: "accounttype"},
		{db: c.onboardingMongo, name: "account_type"},
		{db: c.transactionMongo, name: "transaction"},
		{db: c.transactionMongo, name: "operation"},
		{db: c.transactionMongo, name: "transactionroute"},
		{db: c.transactionMongo, name: "transaction_route"},
		{db: c.transactionMongo, name: "operationroute"},
		{db: c.transactionMongo, name: "operation_route"},
	}

	var (
		summaries                  = make([]domain.MetadataCollectionSummary, 0, len(collections))
		totalMissingEntityIDs      int
		totalDuplicateEntityIDs    int
		totalMissingRequiredFields int
		totalEmptyMetadata         int
	)

	for _, coll := range collections {
		if coll.db == nil {
			continue
		}

		mongoColl := coll.db.Collection(coll.name)

		totalDocs, err := mongoColl.CountDocuments(ctx, bson.M{})
		if err != nil {
			return nil, 0, 0, 0, 0, fmt.Errorf("metadata collection count failed: %w", err)
		}

		emptyMetadata, err := mongoColl.CountDocuments(ctx, bson.M{
			"$or": []bson.M{
				{"metadata": bson.M{"$exists": false}},
				{"metadata": bson.M{"$eq": nil}},
				{"metadata": bson.M{"$eq": bson.M{}}},
			},
		})
		if err != nil {
			return nil, 0, 0, 0, 0, fmt.Errorf("metadata empty check failed: %w", err)
		}

		missingEntityIDs, err := mongoColl.CountDocuments(ctx, bson.M{
			"$or": []bson.M{
				{"entity_id": bson.M{"$exists": false}},
				{"entity_id": ""},
				{"entity_id": bson.M{"$eq": nil}},
			},
		})
		if err != nil {
			return nil, 0, 0, 0, 0, fmt.Errorf("metadata entity_id check failed: %w", err)
		}

		duplicateEntityIDs, err := c.countDuplicateEntityIDs(ctx, mongoColl)
		if err != nil {
			return nil, 0, 0, 0, 0, err
		}

		missingRequired := make(map[string]int64)
		missingRequiredTotal := 0

		for _, field := range requiredFields {
			count, err := mongoColl.CountDocuments(ctx, bson.M{
				field: bson.M{"$exists": false},
			})
			if err != nil {
				return nil, 0, 0, 0, 0, fmt.Errorf("metadata required field check failed: %w", err)
			}

			if count > 0 {
				missingRequired[field] = count
				missingRequiredTotal += int(count)
			}
		}

		summary := domain.MetadataCollectionSummary{
			Collection:            coll.name,
			TotalDocuments:        totalDocs,
			EmptyMetadata:         emptyMetadata,
			MissingEntityIDs:      missingEntityIDs,
			DuplicateEntityIDs:    duplicateEntityIDs,
			MissingRequiredFields: missingRequired,
		}

		summaries = append(summaries, summary)
		totalMissingEntityIDs += int(missingEntityIDs)
		totalDuplicateEntityIDs += int(duplicateEntityIDs)
		totalMissingRequiredFields += missingRequiredTotal
		totalEmptyMetadata += int(emptyMetadata)
	}

	return summaries, totalMissingEntityIDs, totalDuplicateEntityIDs, totalMissingRequiredFields, totalEmptyMetadata, nil
}

func (c *MetadataChecker) countDuplicateEntityIDs(ctx context.Context, coll *mongo.Collection) (int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{"_id": "$entity_id", "count": bson.M{"$sum": 1}}}},
		{{Key: "$match", Value: bson.M{"count": bson.M{"$gt": 1}}}},
		{{Key: "$count", Value: "duplicate_count"}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return 0, fmt.Errorf("metadata duplicate aggregation failed: %w", err)
	}
	defer cursor.Close(ctx)

	var result struct {
		DuplicateCount int64 `bson:"duplicate_count"`
	}

	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return 0, fmt.Errorf("metadata duplicate decode failed: %w", err)
		}
	}

	return result.DuplicateCount, nil
}

type outboxPublishedCheckResult struct {
	ScannedCount    int
	ValidatedCount  int
	FoundCount      int
	TotalCount      int
	MissingCount    int
	MissingEntities []domain.MetadataMissingEntity
}

type outboxEntry struct {
	EntityID   string
	EntityType string
	TotalCount int
}

type mongoTarget struct {
	db         *mongo.Database
	collection string
}

func (c *MetadataChecker) checkOutboxPublished(ctx context.Context, lookbackDays, maxScan, maxResults int) (outboxPublishedCheckResult, error) {
	if maxResults < 0 {
		maxResults = 0
	}

	entries, err := c.fetchOutboxEntries(ctx, lookbackDays, maxScan)
	if err != nil {
		return outboxPublishedCheckResult{}, err
	}

	typeIDs, validatableType := c.groupEntriesByType(entries)

	foundIDsByType, err := c.findIDsInMongo(ctx, typeIDs)
	if err != nil {
		return outboxPublishedCheckResult{}, err
	}

	return c.computeMissingEntities(entries, validatableType, foundIDsByType, maxResults), nil
}

func (c *MetadataChecker) fetchOutboxEntries(ctx context.Context, lookbackDays, maxScan int) ([]outboxEntry, error) {
	query := `
		SELECT entity_id, entity_type,
		       (SELECT COUNT(*)
		          FROM metadata_outbox mo2
		         WHERE mo2.status = 'PUBLISHED'
		           AND mo2.created_at >= NOW() - INTERVAL '1 day' * $1) AS total_count
		FROM metadata_outbox
		WHERE status = 'PUBLISHED'
		  AND created_at >= NOW() - INTERVAL '1 day' * $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := c.db.QueryContext(ctx, query, lookbackDays, maxScan)
	if err != nil {
		return nil, fmt.Errorf("metadata outbox query failed: %w", err)
	}
	defer rows.Close()

	var entries []outboxEntry

	for rows.Next() {
		var e outboxEntry
		if err := rows.Scan(&e.EntityID, &e.EntityType, &e.TotalCount); err != nil {
			return nil, fmt.Errorf("metadata outbox scan failed: %w", err)
		}

		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metadata outbox iteration failed: %w", err)
	}

	return entries, nil
}

func (c *MetadataChecker) groupEntriesByType(entries []outboxEntry) (map[string][]string, map[string]bool) {
	typeIDs := map[string][]string{}
	validatableType := map[string]bool{}

	for _, e := range entries {
		typ := normalizeEntityType(e.EntityType)
		targets := c.targetsForType(typ)

		if !hasAvailableTarget(targets) {
			continue
		}

		validatableType[typ] = true
		typeIDs[typ] = append(typeIDs[typ], e.EntityID)
	}

	return typeIDs, validatableType
}

func hasAvailableTarget(targets []mongoTarget) bool {
	for _, t := range targets {
		if t.db != nil {
			return true
		}
	}

	return false
}

func (c *MetadataChecker) findIDsInMongo(ctx context.Context, typeIDs map[string][]string) (map[string]map[string]struct{}, error) {
	foundIDsByType := make(map[string]map[string]struct{}, len(typeIDs))

	for typ, ids := range typeIDs {
		targets := c.targetsForType(typ)

		for _, t := range targets {
			if t.db == nil {
				continue
			}

			found, err := c.findIDs(ctx, t.db, t.collection, ids)
			if err != nil {
				return nil, err
			}

			if _, ok := foundIDsByType[typ]; !ok {
				foundIDsByType[typ] = map[string]struct{}{}
			}

			for id := range found {
				foundIDsByType[typ][id] = struct{}{}
			}
		}
	}

	return foundIDsByType, nil
}

func (c *MetadataChecker) computeMissingEntities(entries []outboxEntry, validatableType map[string]bool, foundIDsByType map[string]map[string]struct{}, maxResults int) outboxPublishedCheckResult {
	missingCount := 0
	missingEntities := make([]domain.MetadataMissingEntity, 0, maxResults)
	validatedCount := 0

	for _, e := range entries {
		typ := normalizeEntityType(e.EntityType)
		if !validatableType[typ] {
			continue
		}

		validatedCount++

		if _, ok := foundIDsByType[typ][e.EntityID]; ok {
			continue
		}

		missingCount++

		if maxResults > 0 && len(missingEntities) < maxResults {
			missingEntities = append(missingEntities, domain.MetadataMissingEntity{
				EntityID:   e.EntityID,
				EntityType: e.EntityType,
				Reason:     "missing_in_mongo",
			})
		}
	}

	totalCount := 0
	if len(entries) > 0 {
		totalCount = entries[0].TotalCount
	}

	foundCount := validatedCount - missingCount
	if foundCount < 0 {
		foundCount = 0
	}

	return outboxPublishedCheckResult{
		ScannedCount:    len(entries),
		ValidatedCount:  validatedCount,
		FoundCount:      foundCount,
		TotalCount:      totalCount,
		MissingCount:    missingCount,
		MissingEntities: missingEntities,
	}
}

func normalizeEntityType(entityType string) string {
	return strings.ToLower(strings.TrimSpace(entityType))
}

// targetsForType maps outbox entity_type -> MongoDB collection(s). Some entity types have legacy/alias
// collection names; we check all plausible collections and consider the entity present if it exists in any.
func (c *MetadataChecker) targetsForType(entityType string) []mongoTarget {
	if targets := c.onboardingTargets(entityType); targets != nil {
		return targets
	}

	return c.transactionTargets(entityType)
}

// onboardingTargets returns MongoDB targets for onboarding entity types.
func (c *MetadataChecker) onboardingTargets(entityType string) []mongoTarget {
	switch entityType {
	case "organization":
		return []mongoTarget{{db: c.onboardingMongo, collection: "organization"}}
	case "ledger":
		return []mongoTarget{{db: c.onboardingMongo, collection: "ledger"}}
	case "asset":
		return []mongoTarget{{db: c.onboardingMongo, collection: "asset"}}
	case "account":
		return []mongoTarget{{db: c.onboardingMongo, collection: "account"}}
	case "portfolio":
		return []mongoTarget{{db: c.onboardingMongo, collection: "portfolio"}}
	case "segment":
		return []mongoTarget{{db: c.onboardingMongo, collection: "segment"}}
	case "accounttype", "account_type":
		return []mongoTarget{
			{db: c.onboardingMongo, collection: "accounttype"},
			{db: c.onboardingMongo, collection: "account_type"},
		}
	default:
		return nil
	}
}

// transactionTargets returns MongoDB targets for transaction entity types.
func (c *MetadataChecker) transactionTargets(entityType string) []mongoTarget {
	switch entityType {
	case "transaction":
		return []mongoTarget{{db: c.transactionMongo, collection: "transaction"}}
	case "operation":
		return []mongoTarget{{db: c.transactionMongo, collection: "operation"}}
	case "transactionroute", "transaction_route":
		return []mongoTarget{
			{db: c.transactionMongo, collection: "transactionroute"},
			{db: c.transactionMongo, collection: "transaction_route"},
		}
	case "operationroute", "operation_route":
		return []mongoTarget{
			{db: c.transactionMongo, collection: "operationroute"},
			{db: c.transactionMongo, collection: "operation_route"},
		}
	default:
		return nil
	}
}

func mongoFindEntityIDs(ctx context.Context, db *mongo.Database, collection string, ids []string) (map[string]struct{}, error) {
	if db == nil {
		return map[string]struct{}{}, nil
	}

	if len(ids) == 0 {
		return map[string]struct{}{}, nil
	}

	coll := db.Collection(collection)

	cursor, err := coll.Find(ctx, bson.M{"entity_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"entity_id": 1}))
	if err != nil {
		return nil, fmt.Errorf("metadata mongo lookup failed: %w", err)
	}
	defer cursor.Close(ctx)

	found := make(map[string]struct{}, len(ids))

	for cursor.Next(ctx) {
		var doc struct {
			EntityID string `bson:"entity_id"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("metadata mongo decode failed: %w", err)
		}

		if doc.EntityID != "" {
			found[doc.EntityID] = struct{}{}
		}
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("metadata mongo cursor failed: %w", err)
	}

	return found, nil
}
