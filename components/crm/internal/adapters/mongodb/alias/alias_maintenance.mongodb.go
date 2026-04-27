// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// CloseByID marks an alias closed by setting both banking_details.closing_date
// and deleted_at to closingAt in a single UpdateOne.
//
// Semantics versus Delete(soft=true):
//
//   - Delete is a logical removal; consumers that respect the soft-delete
//     convention will no longer see the alias.
//   - Close records a BUSINESS event (the underlying banking relationship
//     ended). The closing_date carries downstream signalling used by banking
//     integrations. Close implies soft-delete but soft-delete does NOT imply
//     close — that is why we keep them as distinct operations.
//
// Naturally idempotent: if closing_date is already set, the caller receives
// the existing alias unchanged (status==200 "already closed"). See
// close-alias.go in the services layer for the full state machine.
func (am *MongoDBRepository) CloseByID(ctx context.Context, organizationID string, holderID, id uuid.UUID, closingAt time.Time) (*mmodel.Alias, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.close_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
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

	// Load the current state. Fetching before updating lets us short-circuit
	// on the "already closed" branch without mutating anything and also
	// guarantees the caller receives the full entity in all response paths.
	existingFilter := bson.D{
		{Key: "_id", Value: id},
		{Key: "holder_id", Value: holderID},
	}

	var existing MongoDBModel
	if err := coll.FindOne(ctx, existingFilter).Decode(&existing); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())
		}

		libOpenTelemetry.HandleSpanError(span, "Failed to load alias for close", err)

		return nil, err
	}

	// If the alias is soft-deleted but NOT closed, still allow close semantics
	// to record the banking event. A soft-delete without close loses the
	// banking signal; close remedies that.
	if existing.BankingDetails != nil && existing.BankingDetails.ClosingDate != nil {
		logger.Log(ctx, libLog.LevelInfo, "Alias already closed; returning existing record",
			libLog.String("alias_id", id.String()))

		return existing.ToEntity(am.DataSecurity)
	}

	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "banking_details.closing_date", Value: closingAt},
			{Key: "deleted_at", Value: closingAt},
			{Key: "updated_at", Value: closingAt},
		}},
	}

	result, err := coll.UpdateOne(ctx, existingFilter, update)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to close alias", err)
		return nil, err
	}

	if result.MatchedCount == 0 {
		return nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())
	}

	// Re-read to return the persisted state.
	var updated MongoDBModel
	if err := coll.FindOne(ctx, existingFilter).Decode(&updated); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to reload alias after close", err)
		return nil, err
	}

	entity, err := updated.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert alias to entity after close", err)
		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, "Closed alias", libLog.String("alias_id", id.String()))

	return entity, nil
}

// DeleteRelatedParty removes a related party from an alias by ID (hard delete)
func (am *MongoDBRepository) DeleteRelatedParty(ctx context.Context, organizationID string, holderID, aliasID, relatedPartyID uuid.UUID) error {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_related_party")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", aliasID.String()),
		attribute.String("app.request.related_party_id", relatedPartyID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	filter := bson.D{
		{Key: "_id", Value: aliasID},
		{Key: "holder_id", Value: holderID},
		{Key: "related_parties._id", Value: relatedPartyID},
		{Key: "deleted_at", Value: nil},
	}

	update := bson.D{
		{Key: "$pull", Value: bson.D{
			{Key: "related_parties", Value: bson.D{
				{Key: "_id", Value: relatedPartyID},
			}},
		}},
		{Key: "$set", Value: bson.D{
			{Key: "updated_at", Value: time.Now()},
		}},
	}

	result, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to delete related party", err)
		return err
	}

	if result.MatchedCount == 0 {
		return pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())
	}

	if result.ModifiedCount == 0 {
		return pkg.ValidateBusinessError(cn.ErrRelatedPartyNotFound, reflect.TypeOf(mmodel.RelatedParty{}).Name())
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Deleted related party with id %s from alias %s", relatedPartyID.String(), aliasID.String()))

	return nil
}

// createIndexes creates indexes for specific fields, if it not exists.
func createIndexes(ctx context.Context, collection *mongo.Collection) error {
	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "_id", Value: 1},
				{Key: "holder_id", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{{Key: "holder_id", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "ledger_id", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "account_id", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "search.document", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "search.banking_details_account", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "search.banking_details_iban", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{
				{Key: "ledger_id", Value: 1},
				{Key: "account_id", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "search.regulatory_fields_participant_document", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "search.related_party_documents", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "related_parties.role", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateMany(ctx, indexModels)

	return err
}
