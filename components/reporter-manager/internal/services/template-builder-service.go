// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"

	"github.com/LerianStudio/reporter/pkg/template_builder"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
)

// ValidateBlocks validates template blocks and returns detailed validation errors.
func (uc *UseCase) ValidateBlocks(ctx context.Context, input *template_builder.ValidateBlocksInput) *template_builder.ValidateBlocksResponse {
	ctx, span := uc.Tracer.Start(ctx, "service.template_builder.validate_blocks")
	defer span.End()

	uc.Logger.Log(ctx, log.LevelInfo, "Validating template blocks")

	if input == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Nil input for block validation", errors.New("input must not be nil"))

		return &template_builder.ValidateBlocksResponse{
			Valid: false,
			Errors: []template_builder.ValidationError{
				{BlockID: "_input", Field: "input", Message: "input must not be nil"},
			},
		}
	}

	resp := template_builder.ValidateBlocks(input.Blocks)

	if resp.Valid {
		uc.Logger.Log(ctx, log.LevelInfo, "Template blocks validation passed")
	} else {
		uc.Logger.Log(ctx, log.LevelInfo, "Template blocks validation failed",
			log.Int("error_count", len(resp.Errors)))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Block validation failed", errors.New("validation errors found"))
	}

	return resp
}

// GenerateCode converts a JSON block structure into Pongo2 template code
// and extracts mapped fields from the block tree.
func (uc *UseCase) GenerateCode(ctx context.Context, input *template_builder.GenerateCodeInput) (*template_builder.GenerateCodeResponse, error) {
	ctx, span := uc.Tracer.Start(ctx, "service.template_builder.generate_code")
	defer span.End()

	uc.Logger.Log(ctx, log.LevelInfo, "Generating Pongo2 code from template blocks")

	if input == nil {
		err := errors.New("input must not be nil")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Nil input for code generation", err)

		return nil, err
	}

	code, err := template_builder.GenerateCode(input.Blocks, input.Format)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate code from blocks", err)
		uc.Logger.Log(ctx, log.LevelError, "Failed to generate code from blocks", log.Err(err))

		return nil, err
	}

	mappedFields := template_builder.ExtractMappedFields(input.Blocks)

	uc.Logger.Log(ctx, log.LevelInfo, "Successfully generated code",
		log.Int("mapped_field_datasources", len(mappedFields)))

	return &template_builder.GenerateCodeResponse{
		Code:         code,
		MappedFields: mappedFields,
	}, nil
}
