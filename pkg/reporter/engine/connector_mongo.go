// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/hex"
	"sort"
	"strings"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoFactory builds MongoDB connectors. Build resolves the per-tenant database
// but opens no connection — the resolved *mongo.Database is lazily connected by
// the underlying client, and explicit connectivity is checked in
// Connector.TestConnection per the engine factory contract.
type mongoFactory struct {
	resolver TenantResolver
	breaker  CircuitBreaker
}

// Compile-time check that mongoFactory satisfies the engine port.
var _ fetcher.ConnectorFactory = (*mongoFactory)(nil)

// Build resolves the per-tenant MongoDB database from the tenant ID stamped in
// the descriptor's HostAttributes and constructs a connector over it.
func (f *mongoFactory) Build(ctx context.Context, descriptor fetcher.ConnectionDescriptor) (fetcher.Connector, error) {
	if descriptor.ConfigName == "" {
		return nil, NewEngineValidationError("connection descriptor is missing config name")
	}

	tenantID := tenantIDFromDescriptor(descriptor)

	handle, err := f.resolver.ResolveMongo(ctx, tenantID, descriptor.ConfigName)
	if err != nil {
		return nil, err
	}

	if handle.db == nil {
		return nil, NewEngineUnavailableError("resolved mongo database is nil", nil)
	}

	return &mongoConnector{
		configName: descriptor.ConfigName,
		db:         handle.db,
		breaker:    f.breaker,
	}, nil
}

// mongoConnector is a single-flight MongoDB connector over an already resolved
// (tenant-scoped) database. It reuses the host-owned mongo client pool.
type mongoConnector struct {
	configName string
	db         *mongo.Database
	breaker    CircuitBreaker
}

// Compile-time check that mongoConnector satisfies the engine port.
var _ fetcher.Connector = (*mongoConnector)(nil)

// TestConnection runs a ping against the resolved database's client through the
// per-datasource circuit breaker.
func (c *mongoConnector) TestConnection(ctx context.Context) error {
	_, err := c.breaker.Execute(c.configName, func() (any, error) {
		return nil, c.db.Client().Ping(ctx, nil)
	})
	if err != nil {
		return NewEngineConnectError("mongo connectivity check failed", err)
	}

	return nil
}

// DiscoverSchema lists the database's collections and samples one document per
// collection to enumerate field names, returning a secret-free snapshot.
func (c *mongoConnector) DiscoverSchema(ctx context.Context) (fetcher.SchemaSnapshot, error) {
	result, err := c.breaker.Execute(c.configName, func() (any, error) {
		return c.discoverSchema(ctx)
	})
	if err != nil {
		return fetcher.SchemaSnapshot{}, err
	}

	snapshot, _ := result.(fetcher.SchemaSnapshot)

	return snapshot, nil
}

func (c *mongoConnector) discoverSchema(ctx context.Context) (fetcher.SchemaSnapshot, error) {
	names, err := c.db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return fetcher.SchemaSnapshot{}, classifyQueryError(ctx, "mongo collection listing failed", err)
	}

	tables := make(map[string][]string, len(names))

	for _, name := range names {
		fields, err := c.sampleFields(ctx, name)
		if err != nil {
			return fetcher.SchemaSnapshot{}, err
		}

		tables[name] = fields
	}

	return buildSnapshot(c.configName, tables), nil
}

// sampleFields enumerates a collection's top-level field names across the whole
// collection (sampling large collections), matching the legacy reporter's
// discoverAllFieldsWithAggregation. MongoDB is schemaless: a single-document
// sample would miss root fields absent from the first doc but present in others,
// which would wrongly reject a valid filter field. Aggregating $objectToArray
// keys across documents reproduces the legacy known-field set, so filter-field
// validation accepts the same fields the legacy worker did.
func (c *mongoConnector) sampleFields(ctx context.Context, collection string) ([]string, error) {
	coll := c.db.Collection(collection)

	count, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, classifyQueryError(ctx, "mongo field sampling failed", err)
	}

	if count == 0 {
		return nil, nil
	}

	cursor, err := coll.Aggregate(ctx, fieldDiscoveryPipeline(count))
	if err != nil {
		return nil, classifyQueryError(ctx, "mongo field sampling failed", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	if !cursor.Next(ctx) {
		if curErr := cursor.Err(); curErr != nil {
			return nil, classifyQueryError(ctx, "mongo field sampling failed", curErr)
		}

		return nil, nil
	}

	var result struct {
		AllKeys []string `bson:"allkeys"`
	}

	if err := cursor.Decode(&result); err != nil {
		return nil, NewEngineInternalError("failed to decode mongo field discovery result", err)
	}

	return result.AllKeys, nil
}

// fieldDiscoveryPipeline builds the $objectToArray/$addToSet aggregation that
// collects the set of top-level field names, mirroring the legacy reporter's
// discoverAllFieldsWithAggregation: large collections are sampled with $sample,
// smaller ones are bounded with $limit.
func fieldDiscoveryPipeline(count int64) []bson.D {
	var first bson.D

	if count > constant.MongoLargeCollectionThreshold {
		first = bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: sampleSizeForCount(count)}}}}
	} else {
		limit := count
		if limit > constant.MongoSmallCollectionLimit {
			limit = constant.MongoSmallCollectionLimit
		}

		first = bson.D{{Key: "$limit", Value: limit}}
	}

	return []bson.D{
		first,
		{{Key: "$project", Value: bson.D{{Key: "arrayofkeyvalue", Value: bson.D{{Key: "$objectToArray", Value: "$$ROOT"}}}}}},
		{{Key: "$unwind", Value: "$arrayofkeyvalue"}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "allkeys", Value: bson.D{{Key: "$addToSet", Value: "$arrayofkeyvalue.k"}}},
		}}},
	}
}

// sampleSizeForCount mirrors the legacy calculateOptimalSampleSize tiering so
// large-collection field discovery samples the same number of documents.
func sampleSizeForCount(count int64) int64 {
	switch {
	case count <= constant.MongoSmallCollectionDocLimit:
		return count
	case count <= constant.MongoMediumCollectionDocLimit:
		return constant.MongoDefaultSampleSize
	case count <= constant.MongoLargeCollectionDocLimit:
		return constant.MongoMediumSampleSize
	case count <= constant.MongoVeryLargeCollectionDocLimit:
		return constant.MongoLargeSampleSize
	default:
		return constant.MongoMaxSampleSize
	}
}

// QueryStream opens a mongo cursor per selected collection and streams documents
// lazily. It reads Fields for this datasource's config name, projecting only the
// selected fields (nested-field-aware, mirroring the reporter's projection). It
// primes the first collection's cursor so query errors surface synchronously.
func (c *mongoConnector) QueryStream(ctx context.Context, request fetcher.ExtractionRequest) (fetcher.RowCursor, error) {
	filters, err := filtersForDatasource(c.configName, request.Filters)
	if err != nil {
		return nil, err
	}

	selection := request.MappedFields[c.configName]

	collections := make([]string, 0, len(selection))
	for collection := range selection {
		collections = append(collections, collection)
	}

	sort.Strings(collections)

	// Discover the schema once so per-collection filter field references can be
	// validated against the sampled fields; an unknown field must error loudly
	// rather than silently widen the result. Only paid when filters are present.
	var snapshot fetcher.SchemaSnapshot
	if len(filters) > 0 {
		snapshot, err = c.DiscoverSchema(ctx)
		if err != nil {
			return nil, err
		}
	}

	cursor := &mongoCursor{
		connector:   c,
		ctx:         ctx,
		selection:   selection,
		filters:     filters,
		schema:      snapshot,
		collections: collections,
	}

	if err := cursor.openNextCollection(); err != nil {
		_ = cursor.Close(ctx)
		return nil, err
	}

	return cursor, nil
}

// Close releases the connector. The mongo client pool is host-owned, so Close is
// a no-op; it is idempotent and double-close safe.
func (c *mongoConnector) Close(_ context.Context) error { return nil }

// mongoCursor streams documents across the request's selected collections, one
// document at a time, opening each collection's driver cursor only when the
// previous collection is exhausted. Single-flight, matching the RowCursor
// contract.
type mongoCursor struct {
	connector   *mongoConnector
	ctx         context.Context
	selection   fetcher.FieldSelection
	filters     datasourceFilters
	schema      fetcher.SchemaSnapshot
	collections []string
	collIdx     int

	cursor      *mongo.Cursor
	currentColl string
	currentRow  map[string]any
	hasRow      bool
	exhausted   bool
	err         error
}

// Compile-time check that mongoCursor satisfies the engine port.
var _ fetcher.RowCursor = (*mongoCursor)(nil)

// Next advances to the next document, rolling over to the next selected
// collection when the current driver cursor is exhausted. It honors context
// cancellation.
func (c *mongoCursor) Next(ctx context.Context) bool {
	if c.err != nil || c.exhausted {
		return false
	}

	if ctx.Err() != nil {
		c.fail(classifyQueryError(ctx, "mongo stream canceled", ctx.Err()))
		return false
	}

	for {
		if c.cursor == nil {
			c.exhausted = true
			c.hasRow = false

			return false
		}

		if c.cursor.Next(ctx) {
			row, err := c.decodeRow()
			if err != nil {
				c.fail(err)
				return false
			}

			c.currentRow = row
			c.hasRow = true

			return true
		}

		if err := c.cursor.Err(); err != nil {
			c.fail(classifyQueryError(ctx, "mongo cursor iteration failed", err))
			return false
		}

		_ = c.cursor.Close(ctx)
		c.cursor = nil

		if err := c.openNextCollection(); err != nil {
			c.fail(err)
			return false
		}
	}
}

// Row returns the current (collection, document) pair, valid only after Next
// returned true.
func (c *mongoCursor) Row() (string, map[string]any) {
	if !c.hasRow {
		return "", nil
	}

	return c.currentColl, c.currentRow
}

// Err returns the first non-EOF error encountered, or nil for a clean drain.
func (c *mongoCursor) Err() error { return c.err }

// Close releases the open driver cursor. Idempotent and double-close safe.
func (c *mongoCursor) Close(ctx context.Context) error {
	if c.cursor != nil {
		err := c.cursor.Close(ctx)
		c.cursor = nil

		if err != nil && c.err == nil {
			return err
		}
	}

	c.hasRow = false
	c.exhausted = true

	return nil
}

// openNextCollection opens the driver cursor for the next selected collection,
// skipping empty selections. It sets c.cursor to nil and returns nil when no
// collections remain.
func (c *mongoCursor) openNextCollection() error {
	for c.collIdx < len(c.collections) {
		collection := c.collections[c.collIdx]
		c.collIdx++

		fields := c.selection[collection]
		if len(fields) == 0 {
			continue
		}

		findOpts := mongoProjection(fields)

		mongoFilter, err := buildMongoFilter(collection, c.filters.tableFilters(collection), c.schema)
		if err != nil {
			return err
		}

		result, execErr := c.connector.breaker.Execute(c.connector.configName, func() (any, error) {
			return c.connector.db.Collection(collection).Find(c.ctx, mongoFilter, findOpts)
		})
		if execErr != nil {
			return classifyQueryError(c.ctx, "mongo find failed", execErr)
		}

		cur, _ := result.(*mongo.Cursor)

		c.cursor = cur
		c.currentColl = collection

		return nil
	}

	c.cursor = nil

	return nil
}

// decodeRow decodes the current document into a column->value map, converting
// BSON types to their Go equivalents the same way the reporter's existing
// decode path does so embedded extraction produces identical row shapes.
func (c *mongoCursor) decodeRow() (map[string]any, error) {
	var doc bson.M
	if err := c.cursor.Decode(&doc); err != nil {
		return nil, NewEngineInternalError("failed to decode mongo document", err)
	}

	return convertBSON(doc), nil
}

func (c *mongoCursor) fail(err error) {
	if c.err == nil {
		c.err = err
	}

	c.exhausted = true
	c.hasRow = false

	if c.cursor != nil {
		_ = c.cursor.Close(c.ctx)
		c.cursor = nil
	}
}

// buildMongoFilter translates per-field FilterCondition criteria for a
// collection into the bson.M passed to Collection.Find, reusing the exact
// operator semantics of pkg/reporter/mongodb convertFilterConditionToMongoFilter:
// $eq/$in (single vs multi Equals), $gt/$gte/$lt/$lte, Between -> $gte+$lte, and
// $nin. Every filter field is validated against the discovered schema fields; an
// unknown field is a loud error so a mis-referenced filter can never silently
// widen the result. An empty/nil filter set yields an empty bson.M (match-all).
func buildMongoFilter(collection string, filters map[string]model.FilterCondition, schema fetcher.SchemaSnapshot) (bson.M, error) {
	mongoFilter := bson.M{}
	if len(filters) == 0 {
		return mongoFilter, nil
	}

	validFields := validColumnSet(schema, collection)

	// An existing-but-empty collection samples no fields, so validFields is empty
	// even though the collection is real. The legacy validateCollectionAndFields
	// short-circuited on count==0 and applied the filter (yielding zero rows);
	// rejecting every filter field here would turn a successful empty extraction
	// into a hard report failure. Only skip field validation when the collection
	// is present in the discovered schema but carries no fields — a collection
	// absent from the snapshot does not exist and is still rejected.
	skipFieldValidation := len(validFields) == 0 && collectionInSnapshot(schema, collection)

	fields := make([]string, 0, len(filters))
	for field := range filters {
		fields = append(fields, field)
	}

	sort.Strings(fields)

	for _, field := range fields {
		condition := filters[field]
		if isMongoFilterEmpty(condition) {
			continue
		}

		// Charset whitelist for parity with the postgres gate. Mongo is not
		// string-injectable (the field is a bson.M key, not interpolated SQL), but
		// rejecting a malformed field at every gate keeps the contract uniform and
		// applies even when field validation is skipped for an empty collection.
		if err := validateFilterField(field); err != nil {
			return nil, err
		}

		if !skipFieldValidation {
			if _, ok := validFields[rootField(field)]; !ok {
				return nil, NewEngineValidationError("unknown filter field " + field + " for collection " + collection)
			}
		}

		if err := validateFilterCondition(field, condition); err != nil {
			return nil, err
		}

		fieldFilter, err := mongoFieldFilter(field, condition)
		if err != nil {
			return nil, err
		}

		if len(fieldFilter) > 0 {
			mongoFilter[field] = fieldFilter
		}
	}

	return mongoFilter, nil
}

// mongoFieldFilter builds the per-field operator sub-document, mirroring
// pkg/reporter/mongodb convertFilterConditionToMongoFilter, including its
// conflict checks between Between/GtOrEq/LtOrEq and In/multi-Equals.
func mongoFieldFilter(field string, condition model.FilterCondition) (map[string]any, error) {
	fieldFilter := make(map[string]any)

	if len(condition.Equals) == 1 {
		fieldFilter["$eq"] = condition.Equals[0]
	} else if len(condition.Equals) > 1 {
		fieldFilter["$in"] = condition.Equals
	}

	if len(condition.GreaterThan) > 0 {
		fieldFilter["$gt"] = condition.GreaterThan[0]
	}

	if len(condition.GreaterOrEqual) > 0 {
		fieldFilter["$gte"] = condition.GreaterOrEqual[0]
	}

	if len(condition.LessThan) > 0 {
		fieldFilter["$lt"] = condition.LessThan[0]
	}

	if len(condition.LessOrEqual) > 0 {
		fieldFilter["$lte"] = condition.LessOrEqual[0]
	}

	if len(condition.Between) == constant.BetweenOperatorValues {
		if _, exists := fieldFilter["$gte"]; exists {
			return nil, NewEngineValidationError("conflicting operators for field " + field + ": between conflicts with gte")
		}

		if _, exists := fieldFilter["$lte"]; exists {
			return nil, NewEngineValidationError("conflicting operators for field " + field + ": between conflicts with lte")
		}

		fieldFilter["$gte"] = condition.Between[0]
		fieldFilter["$lte"] = condition.Between[1]
	}

	if len(condition.In) > 0 {
		if _, exists := fieldFilter["$in"]; exists {
			return nil, NewEngineValidationError("conflicting operators for field " + field + ": in conflicts with multi-value equals")
		}

		fieldFilter["$in"] = condition.In
	}

	if len(condition.NotIn) > 0 {
		fieldFilter["$nin"] = condition.NotIn
	}

	return fieldFilter, nil
}

// isMongoFilterEmpty reports whether a FilterCondition carries no active
// operator, mirroring the legacy isFilterConditionEmpty.
func isMongoFilterEmpty(condition model.FilterCondition) bool {
	return len(condition.Equals) == 0 &&
		len(condition.GreaterThan) == 0 &&
		len(condition.GreaterOrEqual) == 0 &&
		len(condition.LessThan) == 0 &&
		len(condition.LessOrEqual) == 0 &&
		len(condition.Between) == 0 &&
		len(condition.In) == 0 &&
		len(condition.NotIn) == 0
}

// mongoProjection builds find options projecting only the selected fields. A "*"
// selection projects all fields (no projection set). Nested fields are filtered
// so a parent and its child are never both projected, matching the reporter's
// FilterNestedFields behavior that avoids MongoDB projection conflicts.
func mongoProjection(fields []string) *options.FindOptionsBuilder {
	findOpts := options.Find()

	for _, f := range fields {
		if f == "*" {
			return findOpts
		}
	}

	projection := bson.M{}
	for _, field := range filterNestedFields(fields) {
		projection[field] = 1
	}

	if len(projection) > 0 {
		findOpts.SetProjection(projection)
	}

	return findOpts
}

// filterNestedFields removes a field when any ancestor path is already selected,
// so a parent and its nested child are never both projected (a MongoDB
// projection conflict). It mirrors pkg/reporter/mongodb.FilterNestedFields.
func filterNestedFields(fields []string) []string {
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}

	out := make([]string, 0, len(fields))

	for _, field := range fields {
		if hasSelectedAncestor(field, set) {
			continue
		}

		out = append(out, field)
	}

	sort.Strings(out)

	return out
}

func hasSelectedAncestor(field string, set map[string]struct{}) bool {
	parts := strings.Split(field, ".")
	for i := 1; i < len(parts); i++ {
		if _, ok := set[strings.Join(parts[:i], ".")]; ok {
			return true
		}
	}

	return false
}

// convertBSON converts a bson.M into a plain map[string]any, recursively
// normalizing nested documents and arrays. It is the engine-adapter equivalent
// of the reporter's convertBsonToMap.
func convertBSON(doc bson.M) map[string]any {
	out := make(map[string]any, len(doc))
	for k, v := range doc {
		out[k] = convertBSONValue(v)
	}

	return out
}

func convertBSONValue(value any) any {
	switch v := value.(type) {
	case bson.M:
		return convertBSON(v)
	case bson.D:
		out := make(map[string]any, len(v))
		for _, e := range v {
			out[e.Key] = convertBSONValue(e.Value)
		}

		return out
	case bson.A:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = convertBSONValue(e)
		}

		return out
	case bson.DateTime:
		return v.Time()
	case bson.ObjectID:
		return v.Hex()
	case bson.Binary:
		// A 16-byte Binary is a UUID (the common encoding for UUID-keyed
		// account/transaction/operation documents); decode it to its canonical
		// string. Any other Binary becomes hex. This mirrors the reporter's
		// convertBsonValue so embedded extraction emits identical row shapes.
		if len(v.Data) == constant.MongoUUIDByteLength {
			if u, err := uuid.FromBytes(v.Data); err == nil {
				return u.String()
			}
		}

		return hex.EncodeToString(v.Data)
	case nil:
		return nil
	default:
		return v
	}
}
