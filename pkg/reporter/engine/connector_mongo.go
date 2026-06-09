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

// sampleFields reads one document from a collection to enumerate its top-level
// field names. MongoDB is schemaless, so a single sample is the same shallow
// discovery the reporter's existing path uses for projection validation.
func (c *mongoConnector) sampleFields(ctx context.Context, collection string) ([]string, error) {
	var doc bson.M

	err := c.db.Collection(collection).FindOne(ctx, bson.M{}).Decode(&doc)

	switch {
	case err == nil:
	case strings.Contains(err.Error(), mongo.ErrNoDocuments.Error()):
		return nil, nil
	default:
		return nil, classifyQueryError(ctx, "mongo field sampling failed", err)
	}

	fields := make([]string, 0, len(doc))
	for field := range doc {
		fields = append(fields, field)
	}

	return fields, nil
}

// QueryStream opens a mongo cursor per selected collection and streams documents
// lazily. It reads Fields for this datasource's config name, projecting only the
// selected fields (nested-field-aware, mirroring the reporter's projection). It
// primes the first collection's cursor so query errors surface synchronously.
func (c *mongoConnector) QueryStream(ctx context.Context, request fetcher.ExtractionRequest) (fetcher.RowCursor, error) {
	if err := rejectUnsupportedFilters(c.configName, request); err != nil {
		return nil, err
	}

	selection := request.MappedFields[c.configName]

	collections := make([]string, 0, len(selection))
	for collection := range selection {
		collections = append(collections, collection)
	}

	sort.Strings(collections)

	cursor := &mongoCursor{
		connector:   c,
		ctx:         ctx,
		selection:   selection,
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

		result, execErr := c.connector.breaker.Execute(c.connector.configName, func() (any, error) {
			return c.connector.db.Collection(collection).Find(c.ctx, bson.M{}, findOpts)
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
