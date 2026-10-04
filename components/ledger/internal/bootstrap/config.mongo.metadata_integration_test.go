//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"sync"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

// warnLogger records the message and collection of every Warn entry.
type warnLogger struct {
	libLog.Logger

	mu       sync.Mutex
	warnings []string
}

func (l *warnLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	if level != libLog.LevelWarn {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for _, field := range fields {
		if f, ok := field.(libLog.Field); ok && f.Key == "collection" {
			l.warnings = append(l.warnings, fmt.Sprintf("%s: %v", msg, f.Value))
		}
	}
}

// Boot creates entity_id unique where no index exists, leaves a unique one alone, and reports a
// non-unique one without touching it.
func TestIntegration_EnsureMetadataIndexes_NeverDropsAnIndex(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)
	ctx := context.Background()
	db := container.Database
	keys := bson.D{{Key: "entity_id", Value: 1}}

	_, err := db.Collection("transaction").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys})
	require.NoError(t, err)

	_, err = db.Collection("assetrate").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys, Options: options.Index().SetUnique(true)})
	require.NoError(t, err)

	logger := &warnLogger{Logger: libLog.NewNop()}

	ensureMetadataIndexes(conn, logger, transactionMetadataEntities)

	t.Run("no index: created unique", func(t *testing.T) {
		assert.True(t, entityIDIndexUnique(t, db.Collection("operation")))
	})

	t.Run("unique index: left alone", func(t *testing.T) {
		assert.True(t, entityIDIndexUnique(t, db.Collection("assetrate")))
	})

	t.Run("non-unique index: kept and reported once", func(t *testing.T) {
		assert.False(t, entityIDIndexUnique(t, db.Collection("transaction")))
		assert.Equal(t, []string{"Metadata index is not unique; run the backfill runner: transaction"}, logger.warnings)
	})
}

func entityIDIndexUnique(t *testing.T, coll *mongo.Collection) bool {
	t.Helper()

	specs, err := coll.Indexes().ListSpecifications(context.Background())
	require.NoError(t, err)

	for _, spec := range specs {
		if spec.Name == "entity_id_1" {
			return spec.Unique != nil && *spec.Unique
		}
	}

	require.Failf(t, "entity_id index missing", "collection %s", coll.Name())

	return false
}
