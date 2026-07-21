// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package instrument

import (
	"context"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteRelatedParty removes a related party from an instrument by ID (hard delete)
func (am *MongoDBRepository) DeleteRelatedParty(ctx context.Context, organizationID string, holderID, instrumentID, relatedPartyID uuid.UUID) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_related_party")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", instrumentID.String()),
		attribute.String("app.request.related_party_id", relatedPartyID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	// Match on instrument identity only (not related_parties._id) so a missing
	// related party reaches the ModifiedCount==0 branch instead of being
	// misreported as a missing instrument. The $pull below selects the element.
	filter := bson.D{
		{Key: "_id", Value: instrumentID},
		{Key: "holder_id", Value: holderID},
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
		libOpentelemetry.HandleSpanError(span, "Failed to delete related party", err)
		return err
	}

	if result.MatchedCount == 0 {
		businessErr := pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, cn.EntityInstrument)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Instrument not found", businessErr)

		return businessErr
	}

	if result.ModifiedCount == 0 {
		businessErr := pkg.ValidateBusinessError(cn.ErrRelatedPartyNotFound, cn.EntityRelatedParty)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Related party not found", businessErr)

		return businessErr
	}

	return nil
}

// indexModels returns the index definitions for the instrument collection.
func indexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
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
}

// ensureIndexes ensures indexes exist for the alias collection.
// Uses per-collection tracking to handle multi-tenant/per-org collections correctly.
// Retries on failure — indexes are only marked as done after successful creation.
func ensureIndexes(ctx context.Context, collection *mongo.Collection) error {
	key := collection.Database().Name() + ":" + collection.Name()

	return globalIndexTracker.ensureOnce(key, func() error {
		return createIndexes(ctx, collection)
	})
}

// createIndexes creates indexes for specific fields, if it not exists.
func createIndexes(ctx context.Context, collection *mongo.Collection) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateMany(ctx, indexModels())

	return err
}
