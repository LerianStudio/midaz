// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	mongoUtils "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Repository provides an interface for operations related to alias entities.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=alias.mongodb_mock.go --package=alias . Repository
type Repository interface {
	Create(ctx context.Context, organizationID string, input *mmodel.Alias) (*mmodel.Alias, error)
	Find(ctx context.Context, organizationID string, holderID, id uuid.UUID, includeDeleted bool) (*mmodel.Alias, error)
	Update(ctx context.Context, organizationID string, holderID, id uuid.UUID, input *mmodel.Alias, fieldsToRemove []string) (*mmodel.Alias, error)
	FindAll(ctx context.Context, organizationID string, holderID uuid.UUID, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Alias, error)
	Delete(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error
	DeleteRelatedParty(ctx context.Context, organizationID string, holderID, aliasID, relatedPartyID uuid.UUID) error
	Count(ctx context.Context, organizationID string, holderID uuid.UUID) (int64, error)
	ResolveBankAccount(ctx context.Context, input *mmodel.ResolveBankAccountInput) ([]*mmodel.Alias, error)
	ResolveAccount(ctx context.Context, accountID uuid.UUID) ([]*mmodel.Alias, error)
	BackfillBankAccountIndex(ctx context.Context, dryRun bool) (*mmodel.BankAccountIndexBackfillReport, error)
}

// MongoDBRepository is a MongoDB-specific implementation of Repository
type MongoDBRepository struct {
	connection   *libMongo.Client
	DataSecurity *libCrypto.Crypto
}

// NewMongoDBRepository returns a new instance of MongoDBRepository using the given MongoDB connection.
// In multi-tenant mode, connection may be nil — the per-request tenant context provides the database.
func NewMongoDBRepository(connection *libMongo.Client, dataSecurity *libCrypto.Crypto) (*MongoDBRepository, error) {
	r := &MongoDBRepository{
		DataSecurity: dataSecurity,
	}

	if connection != nil {
		r.connection = connection
	}

	return r, nil
}

// getDatabase resolves the MongoDB database for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific *mongo.Database into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (am *MongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
	if am.connection == nil {
		// Check tenant context when static connection is nil (multi-tenant mode without static fallback)
		if db := tmcore.GetMBContext(ctx); db != nil {
			return db, nil
		}

		return nil, fmt.Errorf("no database connection available: multi-tenant context required but not present, and no static connection configured")
	}

	if db := tmcore.GetMBContext(ctx); db != nil {
		return db, nil
	}

	return am.connection.Database(ctx)
}

func (am *MongoDBRepository) withTransaction(ctx context.Context, db *mongo.Database, fn func(mongo.SessionContext) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	session, err := db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) {
		return nil, fn(sc)
	})

	return err
}

// Create inserts an alias into mongo
func (am *MongoDBRepository) Create(ctx context.Context, organizationID string, alias *mmodel.Alias) (*mmodel.Alias, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", alias.HolderID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	err = createIndexes(ctx, coll)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create indexes", err)

		return nil, err
	}

	ctx, spanCount := tracer.Start(ctx, "mongodb.create_alias.count_existing")
	defer spanCount.End()

	spanCount.SetAttributes(attributes...)

	record := &MongoDBModel{}

	if err := record.FromEntity(alias, am.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

		return nil, err
	}

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_alias.insert")
	defer spanInsert.End()

	spanInsert.SetAttributes(attributes...)

	spanInsert.SetAttributes(
		attribute.Bool("app.request.repository_input.has_metadata", len(record.Metadata) > 0),
		attribute.Bool("app.request.repository_input.has_banking_details", record.BankingDetails != nil),
		attribute.Bool("app.request.repository_input.has_regulatory_fields", record.RegulatoryFields != nil),
		attribute.Int("app.request.repository_input.related_parties_count", len(record.RelatedParties)),
	)

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

		return nil, err
	}

	if err := ensureBankAccountIndexIndexes(ctx, db); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create bank account index indexes", err)

		return nil, err
	}

	err = am.withTransaction(ctx, db, func(sc mongo.SessionContext) error {
		if _, insertErr := coll.InsertOne(sc, record); insertErr != nil {
			if mongo.IsDuplicateKeyError(insertErr) && strings.Contains(insertErr.Error(), "account_id") {
				businessErr := pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, cn.EntityAlias)
				libOpenTelemetry.HandleSpanBusinessErrorEvent(spanInsert, "Alias account already associated", businessErr)

				return businessErr
			}

			libOpenTelemetry.HandleSpanError(spanInsert, "Failed to insert alias", insertErr)

			return insertErr
		}

		if indexErr := am.replaceBankAccountIndex(sc, db, organizationID, result); indexErr != nil {
			if mongo.IsDuplicateKeyError(indexErr) {
				businessErr := pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, cn.EntityAlias)
				libOpenTelemetry.HandleSpanBusinessErrorEvent(spanInsert, "Bank account identity already associated", businessErr)

				return businessErr
			}

			libOpenTelemetry.HandleSpanError(spanInsert, "Failed to replace alias bank account index", indexErr)

			return indexErr
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Find an alias by holder and alias id
func (am *MongoDBRepository) Find(ctx context.Context, organizationID string, holderID, id uuid.UUID, includeDeleted bool) (*mmodel.Alias, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	var record MongoDBModel

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "holder_id", Value: holderID},
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	err = coll.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to find account", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, cn.EntityAlias)
		}

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

		return nil, err
	}

	return result, nil
}

//nolint:gocognit,gocyclo // Transaction-backed update must keep source mutation and resolver-index replacement in one callback.
func (am *MongoDBRepository) Update(ctx context.Context, organizationID string, holderID, id uuid.UUID, alias *mmodel.Alias, fieldsToRemove []string) (*mmodel.Alias, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.StringSlice("app.request.fields_to_remove", fieldsToRemove),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))
	if err := createIndexes(ctx, coll); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create alias indexes", err)

		return nil, err
	}

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_alias.update_by_id")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	spanUpdate.SetAttributes(
		attribute.Bool("app.request.repository_input.has_metadata", alias != nil && alias.Metadata != nil),
		attribute.Bool("app.request.repository_input.has_banking_details", alias != nil && alias.BankingDetails != nil),
		attribute.Bool("app.request.repository_input.has_regulatory_fields", alias != nil && alias.RegulatoryFields != nil),
		attribute.Int("app.request.repository_input.related_parties_count", aliasRelatedPartiesCount(alias)),
	)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "holder_id", Value: holderID},
		{Key: "deleted_at", Value: nil},
	}

	aliasToUpdate := &MongoDBModel{}

	if err := aliasToUpdate.FromEntity(alias, am.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to convert alias to model", err)

		return nil, err
	}

	bsonData, err := bson.Marshal(aliasToUpdate)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to marshal alias", err)

		return nil, err
	}

	var updateDocument bson.M
	if err := bson.Unmarshal(bsonData, &updateDocument); err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to unmarshal alias", err)

		return nil, err
	}

	update := mongoUtils.BuildDocumentToPatch(updateDocument, fieldsToRemove)

	if err := ensureBankAccountIndexIndexes(ctx, db); err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to create bank account index indexes", err)

		return nil, err
	}

	ctx, spanFind := tracer.Start(ctx, "mongodb.update_alias.find_by_id")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	var result *mmodel.Alias

	err = am.withTransaction(ctx, db, func(sc mongo.SessionContext) error {
		var beforeRecord MongoDBModel
		if findErr := coll.FindOne(sc, filter).Decode(&beforeRecord); findErr != nil {
			if errors.Is(findErr, mongo.ErrNoDocuments) {
				businessErr := pkg.ValidateBusinessError(cn.ErrAliasNotFound, cn.EntityAlias)
				libOpenTelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "Alias not found", businessErr)

				return businessErr
			}

			libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to find alias before update", findErr)

			return findErr
		}

		beforeAlias, toBeforeErr := beforeRecord.ToEntity(am.DataSecurity)
		if toBeforeErr != nil {
			libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to convert alias before update", toBeforeErr)

			return toBeforeErr
		}

		candidateAlias := mergeAliasForBankAccountIndex(beforeAlias, alias, fieldsToRemove)
		if conflictErr := am.preflightBankAccountIdentityConflict(sc, organizationID, candidateAlias); conflictErr != nil {
			libOpenTelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "Bank account identity conflict", conflictErr)

			return conflictErr
		}

		updateResult, updateErr := coll.UpdateOne(sc, filter, update)
		if updateErr != nil {
			if mongo.IsDuplicateKeyError(updateErr) {
				businessErr := pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, cn.EntityAlias)
				libOpenTelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "Alias account already associated", businessErr)

				return businessErr
			}

			libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to update alias", updateErr)

			return updateErr
		}

		if updateResult.MatchedCount == 0 {
			businessErr := pkg.ValidateBusinessError(cn.ErrAliasNotFound, cn.EntityAlias)
			libOpenTelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "Alias not found", businessErr)

			return businessErr
		}

		var record MongoDBModel
		if findErr := coll.FindOne(sc, filter).Decode(&record); findErr != nil {
			libOpenTelemetry.HandleSpanError(spanFind, "Failed to find alias after update", findErr)

			return findErr
		}

		resolved, toEntityErr := record.ToEntity(am.DataSecurity)
		if toEntityErr != nil {
			libOpenTelemetry.HandleSpanError(spanFind, "Failed to convert alias to model", toEntityErr)

			return toEntityErr
		}

		indexAlias := candidateAlias
		indexAlias.UpdatedAt = resolved.UpdatedAt

		if indexErr := am.replaceBankAccountIndex(sc, db, organizationID, indexAlias); indexErr != nil {
			if mongo.IsDuplicateKeyError(indexErr) {
				businessErr := pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, cn.EntityAlias)
				libOpenTelemetry.HandleSpanBusinessErrorEvent(spanFind, "Bank account identity already associated", businessErr)

				return businessErr
			}

			libOpenTelemetry.HandleSpanError(spanFind, "Failed to replace alias bank account index", indexErr)

			return indexErr
		}

		result = resolved

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Delete remove an alias
func (am *MongoDBRepository) Delete(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	opts := options.Delete()

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))
	if err := createIndexes(ctx, coll); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create alias indexes", err)

		return err
	}

	ctx, spanDelete := tracer.Start(ctx, "mongodb.delete_alias.delete_one")
	defer spanDelete.End()

	spanDelete.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "holder_id", Value: holderID},
		{Key: "deleted_at", Value: nil},
	}

	if err := ensureBankAccountIndexIndexes(ctx, db); err != nil {
		libOpenTelemetry.HandleSpanError(spanDelete, "Failed to create bank account index indexes", err)

		return err
	}

	err = am.withTransaction(ctx, db, func(sc mongo.SessionContext) error {
		if hardDelete {
			deleted, deleteErr := coll.DeleteOne(sc, filter, opts)
			if deleteErr != nil {
				libOpenTelemetry.HandleSpanError(spanDelete, "Failed to delete alias", deleteErr)

				return deleteErr
			}

			if deleted.DeletedCount == 0 {
				businessErr := pkg.ValidateBusinessError(cn.ErrAliasNotFound, cn.EntityAlias)
				libOpenTelemetry.HandleSpanBusinessErrorEvent(spanDelete, "Alias not found", businessErr)

				return businessErr
			}
		} else {
			update := bson.D{
				{Key: "$set", Value: bson.D{
					{Key: "deleted_at", Value: time.Now()},
				}},
			}

			updateResult, updateErr := coll.UpdateOne(sc, filter, update)
			if updateErr != nil {
				libOpenTelemetry.HandleSpanError(spanDelete, "Failed to delete alias", updateErr)

				return updateErr
			}

			if updateResult.MatchedCount == 0 {
				businessErr := pkg.ValidateBusinessError(cn.ErrAliasNotFound, cn.EntityAlias)
				libOpenTelemetry.HandleSpanBusinessErrorEvent(spanDelete, "Alias not found", businessErr)

				return businessErr
			}
		}

		if indexErr := am.deleteBankAccountIndexWithDB(sc, db, id, hardDelete); indexErr != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to delete alias bank account index", indexErr)

			return indexErr
		}

		return nil
	})
	if err != nil {
		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintln("Deleted a document with id: ", id.String(), " (hard delete: ", hardDelete, ")"))

	return nil
}

func mergeAliasForBankAccountIndex(existingAlias, patch *mmodel.Alias, fieldsToRemove []string) *mmodel.Alias {
	if existingAlias == nil {
		return patch
	}

	merged := *existingAlias
	if patch == nil {
		return &merged
	}

	if patch.Document != nil {
		merged.Document = patch.Document
	}

	if patch.Type != nil {
		merged.Type = patch.Type
	}

	if patch.LedgerID != nil {
		merged.LedgerID = patch.LedgerID
	}

	if patch.AccountID != nil {
		merged.AccountID = patch.AccountID
	}

	if patch.HolderID != nil {
		merged.HolderID = patch.HolderID
	}

	if patch.BankingDetails != nil {
		merged.BankingDetails = mergeBankingDetailsForBankAccountIndex(merged.BankingDetails, patch.BankingDetails)
	}

	for _, field := range fieldsToRemove {
		clearRemovedBankAccountIndexField(&merged, field)
	}

	return &merged
}

func clearRemovedBankAccountIndexField(alias *mmodel.Alias, field string) {
	if alias == nil {
		return
	}

	switch strings.TrimSpace(field) {
	case "document":
		alias.Document = nil
	case "type":
		alias.Type = nil
	case "ledgerID", "ledgerId", "ledger_id":
		alias.LedgerID = nil
	case "accountID", "accountId", "account_id":
		alias.AccountID = nil
	case "holderID", "holderId", "holder_id":
		alias.HolderID = nil
	case "bankingDetails", "banking_details":
		alias.BankingDetails = nil
	case "bankingDetails.bankId", "banking_details.bank_id":
		clearBankingDetailsFieldForBankAccountIndex(alias, func(bankingDetails *mmodel.BankingDetails) {
			bankingDetails.BankID = nil
		})
	case "bankingDetails.branch", "banking_details.branch":
		clearBankingDetailsFieldForBankAccountIndex(alias, func(bankingDetails *mmodel.BankingDetails) {
			bankingDetails.Branch = nil
		})
	case "bankingDetails.account", "banking_details.account":
		clearBankingDetailsFieldForBankAccountIndex(alias, func(bankingDetails *mmodel.BankingDetails) {
			bankingDetails.Account = nil
		})
	case "bankingDetails.type", "banking_details.type":
		clearBankingDetailsFieldForBankAccountIndex(alias, func(bankingDetails *mmodel.BankingDetails) {
			bankingDetails.Type = nil
		})
	}
}

func clearBankingDetailsFieldForBankAccountIndex(alias *mmodel.Alias, clearField func(*mmodel.BankingDetails)) {
	if alias.BankingDetails == nil {
		return
	}

	bankingDetails := *alias.BankingDetails
	clearField(&bankingDetails)
	alias.BankingDetails = &bankingDetails
}

func mergeBankingDetailsForBankAccountIndex(existing, patch *mmodel.BankingDetails) *mmodel.BankingDetails {
	if existing == nil {
		return patch
	}

	if patch == nil {
		return existing
	}

	merged := *existing
	if patch.BankID != nil {
		merged.BankID = patch.BankID
	}

	if patch.Branch != nil {
		merged.Branch = patch.Branch
	}

	if patch.Account != nil {
		merged.Account = patch.Account
	}

	if patch.Type != nil {
		merged.Type = patch.Type
	}

	if patch.OpeningDate != nil {
		merged.OpeningDate = patch.OpeningDate
	}

	if patch.ClosingDate != nil {
		merged.ClosingDate = patch.ClosingDate
	}

	if patch.IBAN != nil {
		merged.IBAN = patch.IBAN
	}

	if patch.CountryCode != nil {
		merged.CountryCode = patch.CountryCode
	}

	return &merged
}

func aliasRelatedPartiesCount(alias *mmodel.Alias) int {
	if alias == nil {
		return 0
	}

	return len(alias.RelatedParties)
}
