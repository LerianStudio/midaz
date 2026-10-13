// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"cmp"
	"context"
	"slices"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// metadataListBatchSize is how many matching entity ids one metadata-store read answers.
const metadataListBatchSize = 500

// metadataListWindow describes one offset-paginated, metadata-filtered onboarding listing: which
// metadata collection holds the match, and how to read, identify and enrich the entity rows.
type metadataListWindow[T any] struct {
	collection string
	// batchSize bounds each metadata-store read; zero means metadataListBatchSize.
	batchSize int
	findAll   func(ctx context.Context, filter http.QueryHeader) ([]T, error)
	entityID  func(T) string
	attach    func(T, map[string]any)
}

// listMetadataWindow resolves the page of entities whose metadata matches the filter, ordered by
// entity id in filter.SortOrder.
//
// The metadata store answers the matching entity ids in batches, in that order. Each batch is read
// from the entity store, which applies the route scope (the organization and ledger of the path),
// every other filter of the route and the soft delete, so an id with no live entity row of the
// route takes no slot. The rows are put back in the batch's order, the first (Page-1)*Limit of them
// are skipped and the next Limit form the page. Reading stops once the page is full or the metadata
// store has no more ids. An id that is not a UUID is skipped and counted. Metadata is then read only
// for the returned page. No match yields an empty, non-nil page.
func listMetadataWindow[T any](ctx context.Context, span trace.Span, logger libLog.Logger, uc *UseCase, filter http.QueryHeader, window metadataListWindow[T]) ([]T, error) {
	batchSize := window.batchSize
	if batchSize <= 0 {
		batchSize = metadataListBatchSize
	}

	if filter.Limit <= 0 {
		return []T{}, nil
	}

	skip := max(filter.Page-1, 0) * filter.Limit
	page := make([]T, 0, filter.Limit)
	after := ""
	batches := 0
	invalidIDs := 0

	for len(page) < filter.Limit {
		if err := ctx.Err(); err != nil {
			libOpentelemetry.HandleSpanError(span, "Metadata listing interrupted", err)

			return nil, err
		}

		ids, err := uc.OnboardingMetadataRepo.FindEntityIDs(ctx, window.collection, filter, after, batchSize)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to get metadata entity ids on repo", err)
			logger.Log(ctx, libLog.LevelError, "Error getting metadata entity ids on repo",
				libLog.String("entity_name", window.collection), libLog.Err(err))

			return nil, err
		}

		batches++

		if len(ids) == 0 {
			break
		}

		batch, invalid := parseEntityIDs(ids)
		invalidIDs += invalid

		if len(batch) > 0 {
			rows, err := window.readBatch(ctx, filter, batch)
			if err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to get entities on repo", err)
				logger.Log(ctx, libLog.LevelError, "Error getting entities on repo",
					libLog.String("entity_name", window.collection), libLog.Err(err))

				return nil, err
			}

			page = appendWindow(page, rows, &skip, filter.Limit)
		}

		if len(ids) < batchSize {
			break
		}

		after = ids[len(ids)-1]
	}

	span.SetAttributes(
		attribute.Int("app.metadata_list.batches", batches),
		attribute.Int("db.rows_returned", len(page)),
	)

	if invalidIDs > 0 {
		logger.Log(ctx, libLog.LevelWarn, "Skipped metadata entity ids that are not UUIDs",
			libLog.String("entity_name", window.collection), libLog.Int("skipped_count", invalidIDs))
	}

	if err := window.attachPageMetadata(ctx, span, logger, uc, page); err != nil {
		return nil, err
	}

	return page, nil
}

// readBatch reads the live entity rows of one batch of ids on a single page sized to it, every other
// filter of the request applied, in the batch's order.
func (window metadataListWindow[T]) readBatch(ctx context.Context, filter http.QueryHeader, batch []uuid.UUID) ([]T, error) {
	entityFilter := filter
	entityFilter.EntityIDs = batch
	entityFilter.Page = 1
	entityFilter.Limit = len(batch)

	rows, err := window.findAll(ctx, entityFilter)
	if err != nil {
		return nil, err
	}

	orderByEntityIDs(rows, batch, window.entityID)

	return rows, nil
}

// attachPageMetadata loads and attaches the metadata of the page's entities. An empty page performs
// no lookup.
func (window metadataListWindow[T]) attachPageMetadata(ctx context.Context, span trace.Span, logger libLog.Logger, uc *UseCase, page []T) error {
	if len(page) == 0 {
		return nil
	}

	pageIDs := make([]string, len(page))
	for i := range page {
		pageIDs[i] = window.entityID(page[i])
	}

	metadataMap, err := uc.findPageMetadata(ctx, span, logger, window.collection, pageIDs)
	if err != nil {
		return err
	}

	for i := range page {
		if data, ok := metadataMap[pageIDs[i]]; ok {
			window.attach(page[i], data)
		}
	}

	return nil
}

// parseEntityIDs parses ids in order, dropping the ones that are not UUIDs and counting them.
func parseEntityIDs(ids []string) ([]uuid.UUID, int) {
	parsed := make([]uuid.UUID, 0, len(ids))
	invalid := 0

	for _, id := range ids {
		u, err := uuid.Parse(id)
		if err != nil {
			invalid++

			continue
		}

		parsed = append(parsed, u)
	}

	return parsed, invalid
}

// appendWindow appends rows to page after consuming *skip of them, stopping once page holds limit.
func appendWindow[T any](page, rows []T, skip *int, limit int) []T {
	for _, row := range rows {
		if len(page) == limit {
			break
		}

		if *skip > 0 {
			*skip--

			continue
		}

		page = append(page, row)
	}

	return page
}

// orderByEntityIDs sorts rows into the order of ids. A row whose id is not in ids keeps its relative
// order after the ones that are.
func orderByEntityIDs[T any](rows []T, ids []uuid.UUID, entityID func(T) string) {
	position := make(map[string]int, len(ids))
	for i, id := range ids {
		position[id.String()] = i
	}

	rank := func(row T) int {
		if pos, ok := position[entityID(row)]; ok {
			return pos
		}

		return len(ids)
	}

	slices.SortStableFunc(rows, func(a, b T) int {
		return cmp.Compare(rank(a), rank(b))
	})
}
