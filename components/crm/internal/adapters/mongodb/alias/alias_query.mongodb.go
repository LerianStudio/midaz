// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import (
	"context"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// FindAll accounts by holder id and filter
func (am *MongoDBRepository) FindAll(ctx context.Context, organizationID string, holderID uuid.UUID, query http.QueryHeader, includeDeleted bool) ([]*mmodel.Alias, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_all_aliases")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	limit := int64(query.Limit)
	skip := int64(query.Page*query.Limit - query.Limit)
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.D{{Key: "_id", Value: 1}})

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_all_alias.find")

	spanFind.SetAttributes(attributes...)

	spanFind.SetAttributes(
		attribute.Int("app.request.query.limit", query.Limit),
		attribute.Int("app.request.query.page", query.Page),
		attribute.String("app.request.query.sort_order", query.SortOrder),
		attribute.Bool("app.request.query.has_metadata", query.Metadata != nil),
		attribute.Bool("app.request.query.has_account_id", query.AccountID != nil),
		attribute.Bool("app.request.query.has_ledger_id", query.LedgerID != nil),
		attribute.Bool("app.request.query.has_document", query.Document != nil),
		attribute.Bool("app.request.query.has_related_party_filters", query.RelatedPartyDocument != nil || query.RelatedPartyRole != nil),
		attribute.Bool("app.request.query.has_banking_details_filters", query.BankingDetailsBranch != nil || query.BankingDetailsAccount != nil || query.BankingDetailsIban != nil),
	)

	filter, err := am.buildAliasFilter(query, holderID, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanFind, "Invalid metadata value", err)
		return nil, err
	}

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanFind, "Failed to find aliases", err)

		return nil, err
	}
	defer func() {
		if closeErr := cursor.Close(ctx); closeErr != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to close cursor", closeErr)
		}
	}()

	spanFind.End()

	var aliases []*MongoDBModel

	for cursor.Next(ctx) {
		var holder MongoDBModel
		if err := cursor.Decode(&holder); err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to decode aliases", err)

			return nil, err
		}

		aliases = append(aliases, &holder)
	}

	if err := cursor.Err(); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to iterate aliases", err)

		return nil, err
	}

	results := make([]*mmodel.Alias, len(aliases))
	for i, alias := range aliases {
		results[i], err = alias.ToEntity(am.DataSecurity)
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

			return nil, err
		}
	}

	return results, nil
}

func (am *MongoDBRepository) buildAliasFilter(query http.QueryHeader, holderID uuid.UUID, includeDeleted bool) (bson.D, error) {
	filter := bson.D{}

	if holderID != uuid.Nil {
		filter = append(filter, bson.E{Key: "holder_id", Value: holderID})
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	if !libCommons.IsNilOrEmpty(query.AccountID) {
		filter = append(filter, bson.E{Key: "account_id", Value: *query.AccountID})
	}

	if !libCommons.IsNilOrEmpty(query.LedgerID) {
		filter = append(filter, bson.E{Key: "ledger_id", Value: *query.LedgerID})
	}

	if !libCommons.IsNilOrEmpty(query.Document) {
		documentHash := am.DataSecurity.GenerateHash(query.Document)
		filter = append(filter, bson.E{Key: "search.document", Value: documentHash})
	}

	if !libCommons.IsNilOrEmpty(query.BankingDetailsAccount) {
		bankingDetailsAccountHash := am.DataSecurity.GenerateHash(query.BankingDetailsAccount)
		filter = append(filter, bson.E{Key: "search.banking_details_account", Value: bankingDetailsAccountHash})
	}

	if !libCommons.IsNilOrEmpty(query.BankingDetailsIban) {
		bankingDetailsIbanHash := am.DataSecurity.GenerateHash(query.BankingDetailsIban)
		filter = append(filter, bson.E{Key: "search.banking_details_iban", Value: bankingDetailsIbanHash})
	}

	if !libCommons.IsNilOrEmpty(query.BankingDetailsBranch) {
		filter = append(filter, bson.E{Key: "banking_details.branch", Value: *query.BankingDetailsBranch})
	}

	if !libCommons.IsNilOrEmpty(query.RegulatoryFieldsParticipantDocument) {
		participantDocHash := am.DataSecurity.GenerateHash(query.RegulatoryFieldsParticipantDocument)
		filter = append(filter, bson.E{Key: "search.regulatory_fields_participant_document", Value: participantDocHash})
	}

	if !libCommons.IsNilOrEmpty(query.RelatedPartyDocument) {
		relatedPartyDocHash := am.DataSecurity.GenerateHash(query.RelatedPartyDocument)
		filter = append(filter, bson.E{Key: "search.related_party_documents", Value: relatedPartyDocHash})
	}

	if !libCommons.IsNilOrEmpty(query.RelatedPartyRole) {
		filter = append(filter, bson.E{Key: "related_parties.role", Value: *query.RelatedPartyRole})
	}

	if query.Metadata != nil {
		for k, v := range *query.Metadata {
			safeValue, err := http.ValidateMetadataValue(v)
			if err != nil {
				return nil, err
			}

			key := k
			if !strings.HasPrefix(key, "metadata.") {
				key = "metadata." + key
			}

			filter = append(filter, bson.E{Key: key, Value: safeValue})
		}
	}

	return filter, nil
}

func (am *MongoDBRepository) Count(ctx context.Context, organizationID string, holderID uuid.UUID) (int64, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_all_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return 0, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	ctx, spanCount := tracer.Start(ctx, "mongodb.find_all_alias.find")
	defer spanCount.End()

	spanCount.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "holder_id", Value: holderID},
		{Key: "deleted_at", Value: nil},
	}

	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanCount, "Failed to count aliases by holder", err)

		return 0, err
	}

	return count, nil
}
