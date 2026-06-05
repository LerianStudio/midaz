// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/pongo"

	"github.com/LerianStudio/lib-observability/log"
)

// GetBlocksConfig returns all available block definitions for the template builder.
func (uc *UseCase) GetBlocksConfig(ctx context.Context) *pongo.BlocksConfigResponse {
	ctx, span := uc.Tracer.Start(ctx, "service.template_builder.get_blocks_config")
	defer span.End()

	uc.Logger.Log(ctx, log.LevelInfo, "Retrieving block definitions for template builder")

	blocks := pongo.GetBlockDefinitions()

	uc.Logger.Log(ctx, log.LevelInfo, "Successfully retrieved block definitions",
		log.Int("block_count", len(blocks)))

	return &pongo.BlocksConfigResponse{
		Blocks: blocks,
	}
}

// GetFiltersConfig returns all available filter definitions for the template builder.
func (uc *UseCase) GetFiltersConfig(ctx context.Context) *pongo.FiltersResponse {
	ctx, span := uc.Tracer.Start(ctx, "service.template_builder.get_filters_config")
	defer span.End()

	uc.Logger.Log(ctx, log.LevelInfo, "Retrieving filter definitions for template builder")

	filters := pongo.GetFilterDefinitions()

	uc.Logger.Log(ctx, log.LevelInfo, "Successfully retrieved filter definitions",
		log.Int("filter_count", len(filters)))

	return &pongo.FiltersResponse{
		Filters: filters,
	}
}
