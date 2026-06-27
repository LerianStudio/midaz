// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package instrument

import (
	"context"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	encryption "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// FindAll instruments by holder id and filter
func (am *MongoDBRepository) FindAll(ctx context.Context, organizationID string, holderID uuid.UUID, query http.QueryHeader, includeDeleted bool) ([]*mmodel.Instrument, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_all_instruments")
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
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	// Legacy collection name kept; renaming would orphan existing instrument documents.
	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	limit := int64(query.Limit)
	skip := int64(query.Page*query.Limit - query.Limit)
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.D{{Key: "_id", Value: 1}})

	_, spanFind := tracer.Start(ctx, "mongodb.find_all_instruments.find")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	spanFind.SetAttributes(
		attribute.Int("app.request.query.limit", query.Limit),
		attribute.Int("app.request.query.page", query.Page),
		attribute.String("app.request.query.sort_order", query.SortOrder),
		attribute.Bool("app.request.query.has_metadata", query.Metadata != nil),
		attribute.Bool("app.request.query.has_account_id", query.AccountID != nil),
		attribute.Bool("app.request.query.has_ledger_id", query.LedgerID != nil),
		attribute.Bool("app.request.query.has_document", query.Document != nil),
		attribute.Bool("app.request.query.has_related_party_filters", query.InstrumentRelatedPartyDocument != nil || query.InstrumentRelatedPartyRole != nil),
		attribute.Bool("app.request.query.has_banking_details_filters", query.InstrumentBankingDetailsBranch != nil || query.InstrumentBankingDetailsAccount != nil || query.InstrumentBankingDetailsIban != nil),
	)

	filter, err := am.buildInstrumentFilter(ctx, organizationID, query, holderID, includeDeleted)
	if err != nil {
		recordSpanError(spanFind, "Invalid metadata value", err)
		return nil, err
	}

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find instruments", err)

		return nil, err
	}

	defer func() {
		if closeErr := cursor.Close(ctx); closeErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to close cursor", closeErr)
		}
	}()

	var instruments []*MongoDBModel

	for cursor.Next(ctx) {
		var instrument MongoDBModel
		if err := cursor.Decode(&instrument); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode instruments", err)

			return nil, err
		}

		instruments = append(instruments, &instrument)
	}

	if err := cursor.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate instruments", err)

		return nil, err
	}

	results := make([]*mmodel.Instrument, len(instruments))
	for i, instrument := range instruments {
		// Build encryption context for each instrument
		encryptionCtx := encryption.EncryptionContext{
			TenantID:       encryption.ExtractTenantID(ctx),
			OrganizationID: organizationID,
			RecordID:       instrument.ID.String(),
		}

		results[i], err = instrument.ToEntity(ctx, am.FieldEncryptor, encryptionCtx)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to convert instrument to model", err)

			return nil, err
		}
	}

	return results, nil
}

func (am *MongoDBRepository) buildInstrumentFilter(ctx context.Context, organizationID string, query http.QueryHeader, holderID uuid.UUID, includeDeleted bool) (bson.D, error) {
	filter := bson.D{}

	if holderID != uuid.Nil {
		filter = append(filter, bson.E{Key: "holder_id", Value: holderID})
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	filter = am.appendBasicFilters(filter, query)

	searchCtx := encryption.SearchTokenContext{
		TenantID:       encryption.ExtractTenantID(ctx),
		OrganizationID: organizationID,
	}

	encryptedFilter, err := am.appendEncryptedFilters(ctx, filter, query, searchCtx)
	if err != nil {
		return nil, err
	}

	return am.appendMetadataFilters(encryptedFilter, query)
}

// appendBasicFilters adds non-encrypted field filters to the filter.
func (am *MongoDBRepository) appendBasicFilters(filter bson.D, query http.QueryHeader) bson.D {
	if !libCommons.IsNilOrEmpty(query.AccountID) {
		filter = append(filter, bson.E{Key: "account_id", Value: *query.AccountID})
	}

	if !libCommons.IsNilOrEmpty(query.LedgerID) {
		filter = append(filter, bson.E{Key: "ledger_id", Value: *query.LedgerID})
	}

	if !libCommons.IsNilOrEmpty(query.InstrumentBankingDetailsBranch) {
		filter = append(filter, bson.E{Key: "banking_details.branch", Value: *query.InstrumentBankingDetailsBranch})
	}

	if !libCommons.IsNilOrEmpty(query.InstrumentRelatedPartyRole) {
		filter = append(filter, bson.E{Key: "related_parties.role", Value: *query.InstrumentRelatedPartyRole})
	}

	return filter
}

// appendEncryptedFilters adds encrypted field search filters to the filter.
// Uses $in operator with token candidates to support key rotation during searches.
func (am *MongoDBRepository) appendEncryptedFilters(ctx context.Context, filter bson.D, query http.QueryHeader, searchCtx encryption.SearchTokenContext) (bson.D, error) {
	if !libCommons.IsNilOrEmpty(query.Document) {
		searchCtx.FieldName = "document"

		tokens, err := am.FieldEncryptor.GenerateSearchTokenCandidates(ctx, searchCtx, *query.Document)
		if err != nil {
			return nil, err
		}

		filter = append(filter, bson.E{Key: "search.document", Value: bson.M{"$in": tokens}})
	}

	if !libCommons.IsNilOrEmpty(query.InstrumentBankingDetailsAccount) {
		searchCtx.FieldName = "banking_details.account"

		tokens, err := am.FieldEncryptor.GenerateSearchTokenCandidates(ctx, searchCtx, *query.InstrumentBankingDetailsAccount)
		if err != nil {
			return nil, err
		}

		filter = append(filter, bson.E{Key: "search.banking_details_account", Value: bson.M{"$in": tokens}})
	}

	if !libCommons.IsNilOrEmpty(query.InstrumentBankingDetailsIban) {
		searchCtx.FieldName = "banking_details.iban"

		tokens, err := am.FieldEncryptor.GenerateSearchTokenCandidates(ctx, searchCtx, *query.InstrumentBankingDetailsIban)
		if err != nil {
			return nil, err
		}

		filter = append(filter, bson.E{Key: "search.banking_details_iban", Value: bson.M{"$in": tokens}})
	}

	if !libCommons.IsNilOrEmpty(query.InstrumentRegulatoryFieldsParticipantDocument) {
		searchCtx.FieldName = "regulatory_fields.participant_document"

		tokens, err := am.FieldEncryptor.GenerateSearchTokenCandidates(ctx, searchCtx, *query.InstrumentRegulatoryFieldsParticipantDocument)
		if err != nil {
			return nil, err
		}

		filter = append(filter, bson.E{Key: "search.regulatory_fields_participant_document", Value: bson.M{"$in": tokens}})
	}

	if !libCommons.IsNilOrEmpty(query.InstrumentRelatedPartyDocument) {
		searchCtx.FieldName = "related_parties.document"

		tokens, err := am.FieldEncryptor.GenerateSearchTokenCandidates(ctx, searchCtx, *query.InstrumentRelatedPartyDocument)
		if err != nil {
			return nil, err
		}

		filter = append(filter, bson.E{Key: "search.related_party_documents", Value: bson.M{"$in": tokens}})
	}

	return filter, nil
}

// appendMetadataFilters adds metadata filters to the filter.
func (am *MongoDBRepository) appendMetadataFilters(filter bson.D, query http.QueryHeader) (bson.D, error) {
	if query.Metadata == nil {
		return filter, nil
	}

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

	return filter, nil
}

// Count returns the count of instruments for a given holder
func (am *MongoDBRepository) Count(ctx context.Context, organizationID string, holderID uuid.UUID) (int64, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.count_instruments")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return 0, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	_, spanCount := tracer.Start(ctx, "mongodb.count_instruments.count")
	defer spanCount.End()

	spanCount.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "holder_id", Value: holderID},
		{Key: "deleted_at", Value: nil},
	}

	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanCount, "Failed to count aliases by holder", err)

		return 0, err
	}

	return count, nil
}
