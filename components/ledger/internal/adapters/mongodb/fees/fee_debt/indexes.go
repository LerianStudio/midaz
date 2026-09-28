// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee_debt

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	mmongoDB "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
)

// EnsureIndexes creates the listing indexes on the static fee database. Recording
// depends only on the unique _id index, which every database has.
func EnsureIndexes(ctx context.Context, mc *mmongoDB.MongoConnection) error {
	client, err := mc.GetDB(ctx)
	if err != nil {
		return err
	}

	coll := client.Database(strings.ToLower(mc.Database)).Collection(feeconstant.FeeDebtCollection)

	_, err = coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "_id", Value: 1},
			},
			Options: options.Index().SetName("idx_fd_org_ledger_id"),
		},
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "debtor_balance_ref", Value: 1},
				{Key: "seq", Value: 1},
			},
			Options: options.Index().SetName("idx_fd_org_ledger_debtor_seq"),
		},
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "debtor_balance_ref", Value: 1},
				{Key: "remaining", Value: 1},
			},
			Options: options.Index().SetName("idx_fd_org_ledger_debtor_remaining"),
		},
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "ledger_id", Value: 1},
				{Key: "remaining", Value: 1},
			},
			Options: options.Index().SetName("idx_fd_org_ledger_remaining"),
		},
	})

	return err
}
