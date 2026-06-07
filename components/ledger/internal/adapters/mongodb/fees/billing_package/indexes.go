// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"context"
	"strings"

	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"

	mmongoDB "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// EnsureIndexes creates all necessary indexes for optimal billing package queries.
func EnsureIndexes(ctx context.Context, mc *mmongoDB.MongoConnection) error {
	db, err := mc.GetDB(ctx)
	if err != nil {
		return err
	}

	coll := db.Database(strings.ToLower(mc.Database)).Collection(strings.ToLower(feeconstant.BillingPackageCollection))

	indexes := []mongo.IndexModel{
		// Index 1: org + ledger + type + enable + deleted_at (for FindActiveByType)
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "type", Value: 1},
				{Key: "enable", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_bp_org_ledger_type_enable_deleted"),
		},

		// Index 2: org + ledger + event_filter.transaction_route + enable + deleted_at (for FindMatchingPackages)
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "event_filter.transaction_route", Value: 1},
				{Key: "enable", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_bp_org_ledger_route_enable_deleted").
				SetSparse(true),
		},

		// Index 3: org + ledger + deleted_at + created_at DESC (for FindAll pagination)
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "deleted_at", Value: 1},
				{Key: "created_at", Value: -1},
			},
			Options: options.Index().
				SetName("idx_bp_org_ledger_deleted_created"),
		},

		// Index 4: _id + org + deleted_at (for FindByID)
		{
			Keys: bson.D{
				{Key: "_id", Value: 1},
				{Key: "organization_id", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_bp_id_org_deleted"),
		},
	}

	_, err = coll.Indexes().CreateMany(ctx, indexes)

	return err
}
