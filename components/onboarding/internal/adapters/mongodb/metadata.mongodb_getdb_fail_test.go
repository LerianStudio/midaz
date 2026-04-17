// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

type stderrLogger struct{}

func (stderrLogger) Info(_ ...any)                                     {}
func (stderrLogger) Infof(_ string, _ ...any)                          {}
func (stderrLogger) Infoln(_ ...any)                                   {}
func (stderrLogger) Error(_ ...any)                                    {}
func (stderrLogger) Errorf(_ string, _ ...any)                         {}
func (stderrLogger) Errorln(_ ...any)                                  {}
func (stderrLogger) Warn(_ ...any)                                     {}
func (stderrLogger) Warnf(_ string, _ ...any)                          {}
func (stderrLogger) Warnln(_ ...any)                                   {}
func (stderrLogger) Debug(_ ...any)                                    {}
func (stderrLogger) Debugf(_ string, _ ...any)                         {}
func (stderrLogger) Debugln(_ ...any)                                  {}
func (stderrLogger) Fatal(_ ...any)                                    {}
func (stderrLogger) Fatalf(_ string, _ ...any)                         {}
func (stderrLogger) Fatalln(_ ...any)                                  {}
func (stderrLogger) WithFields(_ ...any) libLog.Logger                 { return stderrLogger{} }
func (stderrLogger) WithDefaultMessageTemplate(_ string) libLog.Logger { return stderrLogger{} }
func (stderrLogger) Sync() error                                       { return nil }

// brokenRepoMongo returns a repository whose connection will fail on every
// GetDB call because the URI is invalid and there is no pre-seeded client.
func brokenRepoMongo() *MetadataMongoDBRepository {
	return &MetadataMongoDBRepository{
		Database: "onboarding",
		connection: &libMongo.MongoConnection{
			ConnectionStringSource: "mongodb://127.0.0.1:1/?connectTimeoutMS=1&serverSelectionTimeoutMS=1",
			Database:               "onboarding",
			Logger:                 stderrLogger{},
			MaxPoolSize:            1,
		},
	}
}

func TestMongoRepository_GetDBFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()

		err := brokenRepoMongo().Create(ctx, "segment", &Metadata{
			ID:       primitive.NewObjectID(),
			EntityID: "e",
			Data:     JSON{"k": "v"},
		})
		require.Error(t, err)
	})
	t.Run("FindList", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepoMongo().FindList(ctx, "segment", http.QueryHeader{
			Limit: 1, Page: 1, SortOrder: "asc", Metadata: &map[string]any{},
		})
		require.Error(t, err)
	})
	t.Run("FindByEntity", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepoMongo().FindByEntity(ctx, "segment", "e")
		require.Error(t, err)
	})
	t.Run("FindByEntityIDs", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepoMongo().FindByEntityIDs(ctx, "segment", []string{"e"})
		require.Error(t, err)
	})
	t.Run("Update", func(t *testing.T) {
		t.Parallel()

		err := brokenRepoMongo().Update(ctx, "segment", "e", map[string]any{"k": "v"})
		require.Error(t, err)
	})
	t.Run("Delete", func(t *testing.T) {
		t.Parallel()

		err := brokenRepoMongo().Delete(ctx, "segment", "e")
		require.Error(t, err)
	})
	t.Run("CreateIndex", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepoMongo().CreateIndex(ctx, "segment", &mmodel.CreateMetadataIndexInput{
			MetadataKey: "tier",
		})
		require.Error(t, err)
	})
	t.Run("FindAllIndexes", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepoMongo().FindAllIndexes(ctx, "segment")
		require.Error(t, err)
	})
	t.Run("DeleteIndex", func(t *testing.T) {
		t.Parallel()

		err := brokenRepoMongo().DeleteIndex(ctx, "segment", "idx")
		require.Error(t, err)
	})
}
