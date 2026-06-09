// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit"
	mongokit "github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/mongodb"
)

// mongoManagerFake routes each tenant ID to its own *mongo.Database, mirroring
// the lib-commons tenant-manager mongo path.
type mongoManagerFake struct {
	dbs map[string]*mongo.Database
}

func (m *mongoManagerFake) GetDatabaseForTenant(_ context.Context, tenantID string) (*mongo.Database, error) {
	db, ok := m.dbs[tenantID]
	if !ok {
		return nil, fmt.Errorf("no database for tenant %q", tenantID)
	}

	return db, nil
}

func startMongo(t *testing.T) (*mongo.Client, func()) {
	t.Helper()

	ctx := context.Background()

	infra := mongokit.NewMongoDBInfra(mongokit.MongoDBConfig{Name: "engine"})

	suite, err := itestkit.New(t).WithInfra(infra).Build(ctx)
	require.NoError(t, err)

	uri, err := infra.URI()
	require.NoError(t, err)

	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	require.NoError(t, client.Ping(connectCtx, nil))

	teardown := func() {
		_ = client.Disconnect(context.Background())
		_ = suite.Terminate(context.Background())
	}

	return client, teardown
}

func seedCollection(t *testing.T, db *mongo.Database, collection string, docs []bson.M) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	anyDocs := make([]any, len(docs))
	for i, d := range docs {
		anyDocs[i] = d
	}

	_, err := db.Collection(collection).InsertMany(ctx, anyDocs)
	require.NoError(t, err)
}

func TestIntegration_MongoConnector_StreamsAndProjects(t *testing.T) {
	client, teardown := startMongo(t)
	defer teardown()

	ctx := context.Background()
	db := client.Database("tenant_default")

	seedCollection(t, db, "holders", []bson.M{
		{"_id": "h-1", "name": "Alice", "ssn": "secret"},
		{"_id": "h-2", "name": "Bob", "ssn": "secret"},
		{"_id": "h-3", "name": "Carol", "ssn": "secret"},
	})

	resolver := NewMultiTenantResolver(nil, &mongoManagerFake{dbs: map[string]*mongo.Database{"tenant-default": db}}, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypeMongo)

	connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "crm", Type: DatasourceTypeMongo,
	}, "tenant-default"))
	require.NoError(t, err)
	defer func() { _ = connector.Close(ctx) }()

	require.NoError(t, connector.TestConnection(ctx))

	cursor, err := connector.QueryStream(ctx, fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{"crm": {"holders": {"name"}}},
	})
	require.NoError(t, err)

	count := 0

	for cursor.Next(ctx) {
		collection, row := cursor.Row()
		assert.Equal(t, "holders", collection)
		// Projection keeps _id (mongo default) and name, excludes ssn.
		_, hasSSN := row["ssn"]
		assert.False(t, hasSSN, "ssn must not be projected")
		assert.Contains(t, row, "name")

		count++
	}

	require.NoError(t, cursor.Err())
	require.NoError(t, cursor.Close(ctx))
	assert.Equal(t, 3, count)
}

func TestIntegration_MongoConnector_TenantIsolation(t *testing.T) {
	client, teardown := startMongo(t)
	defer teardown()

	ctx := context.Background()

	dbA := client.Database("tenant_a")
	dbB := client.Database("tenant_b")

	seedCollection(t, dbA, "holders", []bson.M{{"_id": "1", "owner": "tenant-a"}})
	seedCollection(t, dbB, "holders", []bson.M{{"_id": "1", "owner": "tenant-b"}})

	resolver := NewMultiTenantResolver(nil, &mongoManagerFake{dbs: map[string]*mongo.Database{
		"tenant-a": dbA,
		"tenant-b": dbB,
	}}, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypeMongo)

	read := func(tenant string) string {
		connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
			ConfigName: "crm", Type: DatasourceTypeMongo,
		}, tenant))
		require.NoError(t, err)
		defer func() { _ = connector.Close(ctx) }()

		cursor, err := connector.QueryStream(ctx, fetcher.ExtractionRequest{
			MappedFields: map[string]fetcher.FieldSelection{"crm": {"holders": {"owner"}}},
		})
		require.NoError(t, err)
		defer func() { _ = cursor.Close(ctx) }()

		require.True(t, cursor.Next(ctx))
		_, row := cursor.Row()
		require.NoError(t, cursor.Err())

		owner, _ := row["owner"].(string)

		return owner
	}

	assert.Equal(t, "tenant-a", read("tenant-a"))
	assert.Equal(t, "tenant-b", read("tenant-b"))
}

func TestIntegration_MongoConnector_ContextCancelMidStream(t *testing.T) {
	client, teardown := startMongo(t)
	defer teardown()

	ctx := context.Background()
	db := client.Database("tenant_default")

	docs := make([]bson.M, 0, 500)
	for i := 0; i < 500; i++ {
		docs = append(docs, bson.M{"_id": fmt.Sprintf("e-%d", i), "payload": "x"})
	}

	seedCollection(t, db, "events", docs)

	resolver := NewMultiTenantResolver(nil, &mongoManagerFake{dbs: map[string]*mongo.Database{"tenant-default": db}}, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypeMongo)

	connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "crm", Type: DatasourceTypeMongo,
	}, "tenant-default"))
	require.NoError(t, err)
	defer func() { _ = connector.Close(ctx) }()

	streamCtx, cancel := context.WithCancel(ctx)

	cursor, err := connector.QueryStream(streamCtx, fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{"crm": {"events": {"payload"}}},
	})
	require.NoError(t, err)
	defer func() { _ = cursor.Close(ctx) }()

	require.True(t, cursor.Next(streamCtx))

	cancel()

	for cursor.Next(streamCtx) {
		// drain until cancellation observed
	}

	require.Error(t, cursor.Err())

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, cursor.Err(), &engineErr)
	assert.Equal(t, fetcher.CategoryCanceled, engineErr.Category)
}

func TestIntegration_MongoConnector_DiscoverSchema(t *testing.T) {
	client, teardown := startMongo(t)
	defer teardown()

	ctx := context.Background()
	db := client.Database("tenant_default")

	seedCollection(t, db, "holders", []bson.M{{"_id": "1", "name": "Alice", "email": "a@x.io"}})

	resolver := NewMultiTenantResolver(nil, &mongoManagerFake{dbs: map[string]*mongo.Database{"tenant-default": db}}, nil)
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypeMongo)

	connector, err := factory.Build(ctx, WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "crm", Type: DatasourceTypeMongo,
	}, "tenant-default"))
	require.NoError(t, err)
	defer func() { _ = connector.Close(ctx) }()

	snapshot, err := connector.DiscoverSchema(ctx)
	require.NoError(t, err)

	assert.Equal(t, "crm", snapshot.ConfigName)
	require.True(t, snapshot.HasTable("holders"))
}
