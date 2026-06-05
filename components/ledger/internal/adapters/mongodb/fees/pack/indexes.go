// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"strings"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"

	mmongoDB "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// EnsureIndexes creates all necessary indexes for optimal package queries
func EnsureIndexes(ctx context.Context, mc *mmongoDB.MongoConnection) error {
	db, err := mc.GetDB(ctx)
	if err != nil {
		return err
	}

	coll := db.Database(strings.ToLower(mc.Database)).Collection(strings.ToLower(constant.PackageCollection))

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "enable", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_org_ledger_enable_deleted"),
		},

		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "enable", Value: 1},
				{Key: "transaction_route", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_org_ledger_enable_route_deleted").
				SetSparse(true),
		},

		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "enable", Value: 1},
				{Key: "segment_id", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_org_ledger_enable_segment_deleted").
				SetSparse(true),
		},

		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "enable", Value: 1},
				{Key: "minimum_amount", Value: 1},
				{Key: "maximum_amount", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_org_ledger_enable_amounts_deleted"),
		},

		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "enable", Value: 1},
				{Key: "transaction_route", Value: 1},
				{Key: "segment_id", Value: 1},
				{Key: "minimum_amount", Value: 1},
				{Key: "maximum_amount", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_fee_calculation_complete").
				SetSparse(true),
		},

		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "deleted_at", Value: 1},
				{Key: "created_at", Value: -1},
			},
			Options: options.Index().
				SetName("idx_org_deleted_created"),
		},

		{
			Keys: bson.D{
				{Key: "_id", Value: 1},
				{Key: "organization_id", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_id_org_deleted"),
		},
	}

	_, err = coll.Indexes().CreateMany(ctx, indexes)

	return err
}
