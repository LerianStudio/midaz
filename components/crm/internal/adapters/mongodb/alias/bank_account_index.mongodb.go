// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

var bankAccountIndexEnsureCache sync.Map

const (
	bankAccountIndexReportAliasIDLimit  = 500
	bankAccountIndexDryRunIdentityLimit = 10000
)

func createBankAccountIndexIndexes(ctx context.Context, collection *mongo.Collection) error {
	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "search.document", Value: 1},
				{Key: "banking_details.bank_id", Value: 1},
				{Key: "banking_details.branch_canonical", Value: 1},
				{Key: "search.banking_details_account", Value: 1},
				{Key: "banking_details.type", Value: 1},
			},
			Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.D{
				{Key: "deleted_at", Value: nil},
				{Key: "search.document", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$type", Value: "string"}}},
				{Key: "banking_details.bank_id", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$type", Value: "string"}}},
				{Key: "banking_details.branch_canonical", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$type", Value: "string"}}},
				{Key: "search.banking_details_account", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$type", Value: "string"}}},
				{Key: "banking_details.type", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$type", Value: "string"}}},
			}),
		},
		{
			Keys:    bson.D{{Key: "account_id", Value: 1}, {Key: "deleted_at", Value: 1}},
			Options: options.Index(),
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateMany(ctx, indexModels)

	return err
}

func ensureBankAccountIndexIndexes(ctx context.Context, db *mongo.Database) error {
	cacheKey := fmt.Sprintf("%p.%s.%s", db.Client(), db.Name(), bankAccountIndexCollection)
	if _, ok := bankAccountIndexEnsureCache.Load(cacheKey); ok {
		return nil
	}

	if err := createBankAccountIndexIndexes(ctx, db.Collection(bankAccountIndexCollection)); err != nil {
		return err
	}

	bankAccountIndexEnsureCache.Store(cacheKey, struct{}{})

	return nil
}

func (am *MongoDBRepository) upsertBankAccountIndex(ctx context.Context, organizationID string, alias *mmodel.Alias) error {
	_, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.upsert_bank_account_index")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID),
		attribute.Bool("app.request.has_banking_details", alias != nil && alias.BankingDetails != nil),
	)

	if alias == nil || alias.ID == nil {
		return nil
	}

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	if err := ensureBankAccountIndexIndexes(ctx, db); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create bank account index indexes", err)
		return err
	}

	err = am.replaceBankAccountIndex(ctx, db, organizationID, alias)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			businessErr := pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, cn.EntityAlias)
			libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Bank account identity already associated", businessErr)

			return businessErr
		}

		libOpenTelemetry.HandleSpanError(span, "Failed to upsert bank account index", err)

		return err
	}

	return nil
}

func (am *MongoDBRepository) replaceBankAccountIndex(ctx context.Context, db *mongo.Database, organizationID string, alias *mmodel.Alias) error {
	if alias == nil || alias.ID == nil {
		return nil
	}

	model, err := bankAccountIndexModelFromAlias(organizationID, alias, am.DataSecurity)
	if err != nil {
		return err
	}

	_, err = db.Collection(bankAccountIndexCollection).ReplaceOne(ctx, bson.M{"_id": alias.ID}, model, options.Replace().SetUpsert(true))

	return err
}

func (am *MongoDBRepository) preflightBankAccountIdentityConflict(ctx context.Context, organizationID string, alias *mmodel.Alias) error {
	_, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.preflight_bank_account_identity_conflict")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID), attribute.String("app.request.organization_id", organizationID))

	if alias == nil || alias.ID == nil || !hasCompleteBankAccountIdentity(alias.BankingDetails) || alias.Document == nil || strings.TrimSpace(*alias.Document) == "" {
		return nil
	}

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	if err := ensureBankAccountIndexIndexes(ctx, db); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create bank account index indexes", err)
		return err
	}

	documentHash := am.DataSecurity.GenerateHash(alias.Document)
	accountHash := am.DataSecurity.GenerateHash(alias.BankingDetails.Account)
	branchCanonical := canonicalizeBankAccountBranch(*alias.BankingDetails.Branch)
	filter := bson.M{
		"search.document":                  documentHash,
		"banking_details.bank_id":          *alias.BankingDetails.BankID,
		"banking_details.branch_canonical": branchCanonical,
		"search.banking_details_account":   accountHash,
		"banking_details.type":             *alias.BankingDetails.Type,
		"deleted_at":                       nil,
		"_id":                              bson.M{"$ne": alias.ID},
	}

	count, err := db.Collection(bankAccountIndexCollection).CountDocuments(ctx, filter, options.Count().SetLimit(1))
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to preflight bank account identity conflict", err)
		return err
	}

	if count > 0 {
		businessErr := pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, cn.EntityAlias)
		libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Bank account identity already associated", businessErr)

		return businessErr
	}

	return nil
}

func (am *MongoDBRepository) deleteBankAccountIndexWithDB(ctx context.Context, db *mongo.Database, aliasID uuid.UUID, hardDelete bool) error {
	if hardDelete {
		_, err := db.Collection(bankAccountIndexCollection).DeleteOne(ctx, bson.M{"_id": aliasID})
		return err
	}

	_, err := db.Collection(bankAccountIndexCollection).UpdateOne(ctx, bson.M{"_id": aliasID, "deleted_at": nil}, bson.M{"$set": bson.M{"deleted_at": time.Now(), "updated_at": time.Now()}})

	return err
}

func (am *MongoDBRepository) ResolveBankAccount(ctx context.Context, input *mmodel.ResolveBankAccountInput) ([]*mmodel.Alias, error) {
	_, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.resolve_bank_account")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	if input == nil || input.Document == "" || input.BankingDetails.BankID == "" || input.BankingDetails.Branch == "" || input.BankingDetails.Account == "" || input.BankingDetails.Type == "" {
		err := pkg.ValidateBusinessError(cn.ErrMissingFieldsInRequest, cn.EntityAlias, "document, bankingDetails.bankId, bankingDetails.branch, bankingDetails.account, bankingDetails.type")
		libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Invalid bank account resolver input", err)

		return nil, err
	}

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	documentHash := am.DataSecurity.GenerateHash(&input.Document)
	accountHash := am.DataSecurity.GenerateHash(&input.BankingDetails.Account)
	branchCanonical := canonicalizeBankAccountBranch(input.BankingDetails.Branch)

	filter := bson.D{
		{Key: "search.document", Value: documentHash},
		{Key: "banking_details.bank_id", Value: input.BankingDetails.BankID},
		{Key: "banking_details.branch_canonical", Value: branchCanonical},
		{Key: "search.banking_details_account", Value: accountHash},
		{Key: "banking_details.type", Value: input.BankingDetails.Type},
		{Key: "deleted_at", Value: nil},
	}

	return am.findBankAccountIndexAliases(ctx, db.Collection(bankAccountIndexCollection), filter)
}

func (am *MongoDBRepository) ResolveAccount(ctx context.Context, accountID uuid.UUID) ([]*mmodel.Alias, error) {
	_, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.resolve_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.account_id", accountID.String()),
	)

	if accountID == uuid.Nil {
		err := pkg.ValidateBusinessError(cn.ErrInvalidQueryParameter, cn.EntityAlias, "accountId")
		libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account id", err)

		return nil, err
	}

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	filter := bson.D{{Key: "account_id", Value: accountID.String()}, {Key: "deleted_at", Value: nil}}

	return am.findBankAccountIndexAliases(ctx, db.Collection(bankAccountIndexCollection), filter)
}

func (am *MongoDBRepository) findBankAccountIndexAliases(ctx context.Context, coll *mongo.Collection, filter bson.D) ([]*mmodel.Alias, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)
	_ = tracer
	_ = reqID

	cursor, err := coll.Find(ctx, filter, options.Find().SetLimit(2))
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := cursor.Close(ctx); closeErr != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to close bank account index cursor", libLog.Err(closeErr))
		}
	}()

	var records []*BankAccountIndexModel

	for cursor.Next(ctx) {
		var record BankAccountIndexModel
		if err := cursor.Decode(&record); err != nil {
			return nil, err
		}

		records = append(records, &record)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	aliases := make([]*mmodel.Alias, 0, len(records))
	for _, record := range records {
		resolved, err := record.ToAlias(am.DataSecurity)
		if err != nil {
			return nil, err
		}

		aliases = append(aliases, resolved)
	}

	return aliases, nil
}

func (am *MongoDBRepository) BackfillBankAccountIndex(ctx context.Context, dryRun bool) (*mmodel.BankAccountIndexBackfillReport, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.backfill_bank_account_index")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID), attribute.Bool("app.request.dry_run", dryRun))

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	collections, err := db.ListCollectionNames(ctx, bson.M{"name": bson.M{"$regex": "^aliases_"}})
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to list alias collections", err)
		return nil, err
	}

	report := &mmodel.BankAccountIndexBackfillReport{DryRun: dryRun, CollectionsScanned: len(collections)}

	var seen map[string][]string
	if dryRun {
		seen = make(map[string][]string)
	}

	for _, collectionName := range collections {
		if err := am.backfillAliasCollection(ctx, db.Collection(collectionName), strings.TrimPrefix(collectionName, "aliases_"), dryRun, report, seen, logger); err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to backfill alias collection", err)
			return nil, err
		}
	}

	for _, aliasIDs := range seen {
		if len(aliasIDs) > 1 {
			report.Duplicates += len(aliasIDs)
			appendDuplicateAliasIDs(report, aliasIDs...)
		}
	}

	return report, nil
}

//nolint:gocognit,gocyclo // Backfill keeps scan/report/write flow together to preserve resumable accounting semantics.
func (am *MongoDBRepository) backfillAliasCollection(ctx context.Context, coll *mongo.Collection, organizationID string, dryRun bool, report *mmodel.BankAccountIndexBackfillReport, seen map[string][]string, logger libLog.Logger) error {
	cursor, err := coll.Find(ctx, bson.M{"deleted_at": nil}, options.Find().SetBatchSize(100))
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := cursor.Close(ctx); closeErr != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to close alias backfill cursor", libLog.Err(closeErr))
		}
	}()

	for cursor.Next(ctx) {
		var record MongoDBModel
		if err := cursor.Decode(&record); err != nil {
			return err
		}

		report.AliasesScanned++

		entity, err := record.ToEntity(am.DataSecurity)
		if err != nil {
			report.Incomplete++
			if record.ID != nil {
				appendIncompleteAliasID(report, record.ID.String())
			}

			logger.Log(ctx, libLog.LevelWarn, "Skipping malformed alias during bank account index backfill", libLog.Err(err))

			continue
		}

		model, err := bankAccountIndexModelFromAlias(organizationID, entity, am.DataSecurity)
		if err != nil {
			report.Incomplete++
			if entity != nil && entity.ID != nil {
				appendIncompleteAliasID(report, entity.ID.String())
			}

			logger.Log(ctx, libLog.LevelWarn, "Skipping malformed alias during bank account index backfill", libLog.Err(err))

			continue
		}

		if entity != nil && hasAnyBankAccountIdentity(entity.BankingDetails) && !hasCompleteBankAccountIdentity(entity.BankingDetails) {
			report.Incomplete++
			if entity.ID != nil {
				appendIncompleteAliasID(report, entity.ID.String())
			}
		}

		if model == nil {
			report.Incomplete++
			if entity != nil && entity.ID != nil {
				appendIncompleteAliasID(report, entity.ID.String())
			}

			continue
		}

		if seen != nil && model.BankingDetails != nil && model.Search != nil && model.Search.Document != nil && model.Search.BankingDetailsAccount != nil {
			identity := fmt.Sprintf("%s:%s:%s:%s:%s", bankIndexStringValue(model.Search.Document), bankIndexStringValue(model.BankingDetails.BankID), bankIndexStringValue(model.BankingDetails.BranchCanonical), bankIndexStringValue(model.Search.BankingDetailsAccount), bankIndexStringValue(model.BankingDetails.Type))
			if _, ok := seen[identity]; ok || len(seen) < bankAccountIndexDryRunIdentityLimit {
				seen[identity] = append(seen[identity], entity.ID.String())
			} else {
				report.DuplicateAliasIDsTruncated = true
			}
		}

		if dryRun {
			continue
		}

		if err := am.upsertBankAccountIndex(ctx, organizationID, entity); err != nil {
			if mongo.IsDuplicateKeyError(err) || errors.As(err, new(pkg.EntityConflictError)) {
				report.Duplicates++
				appendDuplicateAliasIDs(report, entity.ID.String())

				continue
			}

			return err
		}

		report.Upserted++
	}

	return cursor.Err()
}

func bankIndexStringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func appendIncompleteAliasID(report *mmodel.BankAccountIndexBackfillReport, aliasID string) {
	if len(report.IncompleteAliasIDs) >= bankAccountIndexReportAliasIDLimit {
		report.IncompleteAliasIDsTruncated = true
		return
	}

	report.IncompleteAliasIDs = append(report.IncompleteAliasIDs, aliasID)
}

func appendDuplicateAliasIDs(report *mmodel.BankAccountIndexBackfillReport, aliasIDs ...string) {
	for _, aliasID := range aliasIDs {
		if len(report.DuplicateAliasIDs) >= bankAccountIndexReportAliasIDLimit {
			report.DuplicateAliasIDsTruncated = true
			return
		}

		report.DuplicateAliasIDs = append(report.DuplicateAliasIDs, aliasID)
	}
}
