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

	lookbackDays := config.LookbackDays
	if lookbackDays <= 0 {
		lookbackDays = defaultLookbackDays
	}

	maxScan := config.MetadataMaxScan
	if maxScan <= 0 {
		maxScan = defaultMaxScan
	}

	result := &domain.MetadataCheckResult{}

	summaries, missingEntityIDs, duplicateEntityIDs, missingRequiredFields, emptyMetadata, err := c.collectCollectionSummaries(ctx)
	if err != nil {
		return nil, err
	}

	result.CollectionSummaries = summaries
	result.MissingEntityIDs = missingEntityIDs
	result.DuplicateEntityIDs = duplicateEntityIDs
	result.MissingRequiredFields = missingRequiredFields
	result.EmptyMetadata = emptyMetadata

	if c.db != nil && (c.transactionMongo != nil || c.onboardingMongo != nil) {
		outboxRes, err := c.checkOutboxPublished(ctx, lookbackDays, maxScan, config.MaxResults)
		if err != nil {
			return nil, err
		}

		// Important: these counts are for the subset we can actually validate against MongoDB.
		// (Entity types that map to a different MongoDB, or unsupported types, are excluded.)
		result.PostgreSQLCount = int64(outboxRes.ValidatedCount)
		result.MissingCount = int64(outboxRes.MissingCount)
		result.MongoDBCount = int64(outboxRes.FoundCount)
		result.MissingEntities = outboxRes.MissingEntities

		if outboxRes.ScannedCount < outboxRes.TotalCount {
			result.ScanLimited = true
		}
	}

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

	return result, nil
}

func (c *MetadataChecker) collectCollectionSummaries(ctx context.Context) ([]domain.MetadataCollectionSummary, int, int, int, int, error) {
	var (
		summaries                  []domain.MetadataCollectionSummary
		totalMissingEntityIDs      int
		totalDuplicateEntityIDs    int
		totalMissingRequiredFields int
		totalEmptyMetadata         int
	)

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

func (c *MetadataChecker) checkOutboxPublished(ctx context.Context, lookbackDays, maxScan, maxResults int) (outboxPublishedCheckResult, error) {
	if maxResults < 0 {
		maxResults = 0
	}

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
		return outboxPublishedCheckResult{}, fmt.Errorf("metadata outbox query failed: %w", err)
	}
	defer rows.Close()

	type entry struct {
		EntityID   string
		EntityType string
		TotalCount int
	}

	var entries []entry

	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.EntityID, &e.EntityType, &e.TotalCount); err != nil {
			return outboxPublishedCheckResult{}, fmt.Errorf("metadata outbox scan failed: %w", err)
		}

		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return outboxPublishedCheckResult{}, fmt.Errorf("metadata outbox iteration failed: %w", err)
	}

	normalizeType := func(entityType string) string {
		return strings.ToLower(strings.TrimSpace(entityType))
	}

	type mongoTarget struct {
		db         *mongo.Database
		collection string
	}

	// Map outbox entity_type -> MongoDB collection(s). Some entity types have legacy/alias
	// collection names; we check all plausible collections and consider the entity present
	// if it exists in any.
	targetsForType := func(entityType string) []mongoTarget {
		switch normalizeType(entityType) {
		// Onboarding Mongo
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

		// Transaction Mongo
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

	typeIDs := map[string][]string{}
	validatableType := map[string]bool{}

	for _, e := range entries {
		typ := normalizeType(e.EntityType)
		targets := targetsForType(typ)
		available := false

		for _, t := range targets {
			if t.db != nil {
				available = true
				break
			}
		}

		if !available {
			continue
		}

		validatableType[typ] = true
		typeIDs[typ] = append(typeIDs[typ], e.EntityID)
	}

	foundIDsByType := make(map[string]map[string]struct{}, len(typeIDs))
	for typ, ids := range typeIDs {
		targets := targetsForType(typ)
		for _, t := range targets {
			if t.db == nil {
				continue
			}

			found, err := c.findIDs(ctx, t.db, t.collection, ids)
			if err != nil {
				return outboxPublishedCheckResult{}, err
			}

			if _, ok := foundIDsByType[typ]; !ok {
				foundIDsByType[typ] = map[string]struct{}{}
			}

			for id := range found {
				foundIDsByType[typ][id] = struct{}{}
			}
		}
	}

	missingCount := 0
	missingEntities := make([]domain.MetadataMissingEntity, 0, maxResults)
	validatedCount := 0

	for _, e := range entries {
		typ := normalizeType(e.EntityType)
		if !validatableType[typ] {
			// Unsupported entity type, or relevant MongoDB not configured: do not treat as missing.
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
	}, nil
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
