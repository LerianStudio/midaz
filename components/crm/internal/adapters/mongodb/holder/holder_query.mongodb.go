// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// FindAll get all holders that match the query filter
func (hm *MongoDBRepository) FindAll(ctx context.Context, organizationID string, query http.QueryHeader, includeDeleted bool) ([]*mmodel.Holder, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_all_holders")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	}

	span.SetAttributes(attributes...)

	db, err := hm.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("holders_" + organizationID))

	limit := int64(query.Limit)
	skip := int64(query.Page*query.Limit - query.Limit)
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.D{{Key: "_id", Value: 1}})

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_all_holders.find")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	spanFind.SetAttributes(
		attribute.Int("app.request.query.limit", query.Limit),
		attribute.Int("app.request.query.page", query.Page),
		attribute.String("app.request.query.sort_order", query.SortOrder),
		attribute.Bool("app.request.query.has_metadata", query.Metadata != nil),
		attribute.Bool("app.request.query.has_external_id", query.ExternalID != nil),
		attribute.Bool("app.request.query.has_document", query.Document != nil),
	)

	filter, err := hm.buildHolderFilter(query, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanFind, "Invalid metadata value", err)
		return nil, err
	}

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanFind, "Failed to find holder", err)

		return nil, err
	}

	defer func() {
		if closeErr := cursor.Close(ctx); closeErr != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to close cursor", closeErr)
		}
	}()

	var holders []*MongoDBModel

	for cursor.Next(ctx) {
		var holder MongoDBModel
		if err := cursor.Decode(&holder); err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to decode holder", err)

			return nil, err
		}

		holders = append(holders, &holder)
	}

	if err := cursor.Err(); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to iterate holders", err)

		return nil, err
	}

	results := make([]*mmodel.Holder, len(holders))
	for i, holder := range holders {
		results[i], err = holder.ToEntity(hm.DataSecurity)
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

			return nil, err
		}
	}

	return results, nil
}

func (hm *MongoDBRepository) buildHolderFilter(query http.QueryHeader, includeDeleted bool) (bson.D, error) {
	filter := bson.D{}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	if query.ExternalID != nil && *query.ExternalID != "" {
		filter = append(filter, bson.E{Key: "external_id", Value: *query.ExternalID})
	}

	if query.Document != nil && *query.Document != "" {
		documentHash := hm.DataSecurity.GenerateHash(query.Document)
		filter = append(filter, bson.E{Key: "search.document", Value: documentHash})
	}

	// Apply type filter (holder type: NATURAL_PERSON, LEGAL_PERSON)
	if query.FilterType != nil && *query.FilterType != "" {
		filter = append(filter, bson.E{Key: "type", Value: strings.ToUpper(*query.FilterType)})
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
