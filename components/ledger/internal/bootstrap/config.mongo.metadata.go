// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"strings"
	"time"

	libMongo "github.com/LerianStudio/lib-commons/v7/commons/mongo"
	libLog "github.com/LerianStudio/lib-observability/v4/log"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// onboardingMetadataEntities and transactionMetadataEntities are the entities whose metadata each
// module keeps, one collection per entity named by its lowercased entity name.
var (
	onboardingMetadataEntities = []string{
		constant.EntityOrganization, constant.EntityLedger, constant.EntitySegment, constant.EntityAccount,
		constant.EntityPortfolio, constant.EntityAsset, constant.EntityAccountType,
	}
	transactionMetadataEntities = []string{
		constant.EntityOperation, constant.EntityTransaction, constant.EntityOperationRoute,
		constant.EntityTransactionRoute, constant.EntityAssetRate,
	}
)

// ensureMetadataIndexes gives each single-tenant metadata collection that lacks one a unique
// entity_id index; tenant databases get it from the Mongo migrations shipped to the Tenant Manager.
// It never drops an index: a non-unique one is left to the backfill runner; nothing stops the boot.
func ensureMetadataIndexes(conn *libMongo.Client, logger libLog.Logger, entities []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := conn.Database(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to resolve the metadata database for its indexes", libLog.Err(err))

		return
	}

	for _, entity := range entities {
		collection := strings.ToLower(entity)

		unique, err := mongodb.EnsureUniqueEntityIDIndex(ctx, db.Collection(collection))

		switch {
		case err != nil:
			logger.Log(ctx, libLog.LevelWarn, "Failed to ensure indexes for collection", libLog.String("collection", collection), libLog.Err(err))
		case !unique:
			logger.Log(ctx, libLog.LevelWarn, "Metadata index is not unique; run the backfill runner", libLog.String("collection", collection))
		}
	}
}
