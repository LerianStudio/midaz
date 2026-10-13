// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"
)

// findPageMetadata loads the metadata of one listing page's entities, keyed by entity ID. The
// metadata-filtered listings resolve the matching IDs first and page over the entities, so only the
// returned page's metadata is read. An empty page performs no lookup. A lookup failure is technical.
func (uc *UseCase) findPageMetadata(ctx context.Context, span trace.Span, logger libLog.Logger, collection string, ids []string) (map[string]map[string]any, error) {
	if len(ids) == 0 {
		return map[string]map[string]any{}, nil
	}

	metadata, err := uc.OnboardingMetadataRepo.FindByEntityIDs(ctx, collection, ids)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get page metadata on repo", err)
		logger.Log(ctx, libLog.LevelError, "Error getting page metadata on repo", libLog.Err(err))

		return nil, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	return metadataMap, nil
}
