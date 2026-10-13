// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMergeMetadataDocuments(t *testing.T) {
	early := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)
	oldest, newer := bson.NewObjectIDFromTimestamp(early), bson.NewObjectIDFromTimestamp(late)

	tests := []struct {
		name       string
		collection string
		docs       []MetadataMongoDBModel
		want       MetadataMongoDBModel
	}{
		{
			name:       "the latest update wins a client key and every key survives",
			collection: "transaction",
			docs: []MetadataMongoDBModel{
				{ID: oldest, EntityID: "e", Data: JSON{"k": "old", "a": 1}, CreatedAt: early, UpdatedAt: late},
				{ID: newer, EntityID: "e", Data: JSON{"k": "new", "b": 2}, CreatedAt: late, UpdatedAt: early},
			},
			want: MetadataMongoDBModel{ID: oldest, EntityID: "e", Data: JSON{"k": "old", "a": 1, "b": 2}, CreatedAt: early, UpdatedAt: late},
		},
		{
			name:       "a completed document's reserved key beats a later client copy and lends its name",
			collection: "transaction",
			docs: []MetadataMongoDBModel{
				{ID: oldest, EntityID: "e", Data: JSON{"feeApplied": "client", "purpose": "patched"}, CreatedAt: late, UpdatedAt: late},
				{ID: newer, EntityID: "e", EntityName: "Transaction", Data: JSON{"feeApplied": "true", "purpose": "frozen"}, CreatedAt: early, UpdatedAt: early},
			},
			want: MetadataMongoDBModel{
				ID: oldest, EntityID: "e", EntityName: "Transaction", Data: JSON{"feeApplied": "true", "purpose": "patched"}, CreatedAt: early, UpdatedAt: late,
			},
		},
		{
			name:       "a reserved key only an uncompleted document holds survives",
			collection: "transaction",
			docs: []MetadataMongoDBModel{
				{ID: oldest, EntityID: "e", EntityName: "Transaction", Data: JSON{"purpose": "frozen"}, CreatedAt: early, UpdatedAt: early},
				{ID: newer, EntityID: "e", Data: JSON{"feeApplied": "client"}, CreatedAt: late, UpdatedAt: late},
			},
			want: MetadataMongoDBModel{
				ID: oldest, EntityID: "e", EntityName: "Transaction", Data: JSON{"purpose": "frozen", "feeApplied": "client"}, CreatedAt: early, UpdatedAt: late,
			},
		},
		{
			name:       "outside transactions and operations a reserved name is a client key the latest update wins",
			collection: "organization",
			docs: []MetadataMongoDBModel{
				{ID: oldest, EntityID: "e", EntityName: "Organization", Data: JSON{"feeExemption": "old"}, CreatedAt: early, UpdatedAt: early},
				{ID: newer, EntityID: "e", Data: JSON{"feeExemption": "new"}, CreatedAt: late, UpdatedAt: late},
			},
			want: MetadataMongoDBModel{
				ID: oldest, EntityID: "e", EntityName: "Organization", Data: JSON{"feeExemption": "new"}, CreatedAt: early, UpdatedAt: late,
			},
		},
		{
			name:       "a duplicate a PATCH upserted without created_at lends none",
			collection: "transaction",
			docs: []MetadataMongoDBModel{
				{ID: oldest, EntityID: "e", Data: JSON{"client": "patched"}, UpdatedAt: late},
				{ID: newer, EntityID: "e", EntityName: "Transaction", Data: JSON{"purpose": "frozen"}, CreatedAt: early, UpdatedAt: early},
			},
			want: MetadataMongoDBModel{
				ID: oldest, EntityID: "e", EntityName: "Transaction", Data: JSON{"client": "patched", "purpose": "frozen"}, CreatedAt: early, UpdatedAt: late,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mergeMetadataDocuments(tt.collection, tt.docs))
		})
	}
}
