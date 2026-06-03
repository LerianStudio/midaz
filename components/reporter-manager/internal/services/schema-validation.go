// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/datasource"
	pkgHTTP "github.com/LerianStudio/reporter/pkg/net/http"

	"go.opentelemetry.io/otel/attribute"
)

// ValidateSchemaViaProvider validates mapped fields against data source schemas
// using the DataSourceProvider interface. For each data source referenced in
// mappedFields, it passes the table→fields map to ValidateSchema on the provider.
//
// The provider performs table-aware validation:
//   - DirectProvider: validates table existence, field-per-table, schema ambiguity (PostgreSQL),
//     and plugin CRM organization-scoped collections (MongoDB).
//   - FetcherProvider: flattens to field list and delegates to Fetcher API.
//
// Per D7 decision, unavailable data sources produce warnings (not errors),
// allowing template creation/update to proceed with partial validation.
//
// Returns:
//   - warnings: ValidationWarning entries for unavailable datasources
//   - error: non-nil if any datasource has invalid tables/fields or provider returns a hard error
//
// If DataSourceProvider is nil, validation is skipped (backward compatibility).
func (uc *UseCase) ValidateSchemaViaProvider(ctx context.Context, mappedFields map[string]map[string][]string) ([]datasource.ValidationWarning, error) {
	if uc.DataSourceProvider == nil {
		return nil, nil
	}

	if len(mappedFields) == 0 {
		return nil, nil
	}

	ctx, span := uc.Tracer.Start(ctx, "service.template.validate_schema_via_provider")
	defer span.End()

	span.SetAttributes(
		attribute.Int("app.datasource.count", len(mappedFields)),
	)

	uc.Logger.Log(ctx, log.LevelInfo, "Validating mapped fields via DataSourceProvider",
		log.Int("datasource_count", len(mappedFields)),
	)

	var allWarnings []datasource.ValidationWarning

	for dsID, tables := range mappedFields {
		if len(tables) == 0 {
			continue
		}

		result, err := uc.DataSourceProvider.ValidateSchema(ctx, dsID, tables)
		if err != nil {
			if pkgHTTP.IsBusinessError(err) {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Schema validation failed via provider", err)
			} else {
				libOpentelemetry.HandleSpanError(span, "Schema validation failed via provider", err)
			}

			uc.Logger.Log(ctx, log.LevelError, "Schema validation error from provider",
				log.String("data_source_id", dsID),
				log.Err(err),
			)

			return nil, fmt.Errorf("schema validation for data source %q: %w", dsID, err)
		}

		// Translate structured validation result to business errors
		if !result.Valid {
			if validationErr := translateValidationResult(result, dsID); validationErr != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Schema validation found issues", validationErr)

				uc.Logger.Log(ctx, log.LevelError, "Schema validation failed",
					log.String("data_source_id", dsID),
					log.Err(validationErr),
				)

				return nil, validationErr
			}
		}

		if len(result.Warnings) > 0 {
			allWarnings = append(allWarnings, result.Warnings...)

			uc.Logger.Log(ctx, log.LevelWarn, "Schema validation produced warnings (D7)",
				log.String("data_source_id", dsID),
				log.Int("warning_count", len(result.Warnings)),
			)
		}
	}

	return allWarnings, nil
}

// translateValidationResult converts a structured ValidationResult into the
// appropriate business error, preserving the same error codes that the legacy
// validation used.
func translateValidationResult(result *datasource.ValidationResult, dsID string) error {
	// Schema ambiguity takes priority (PostgreSQL-specific)
	if len(result.Ambiguous) > 0 {
		first := result.Ambiguous[0]

		return pkg.ValidateBusinessError(constant.ErrSchemaAmbiguous, "", first.Table, first.Schemas)
	}

	// Missing fields per table
	if len(result.MissingFields) > 0 {
		var allMissing []string

		for _, mf := range result.MissingFields {
			allMissing = append(allMissing, mf.Fields...)
		}

		return pkg.ValidateBusinessError(constant.ErrMissingTableFields, "", allMissing)
	}

	// Missing tables
	if len(result.MissingTables) > 0 {
		return pkg.ValidateBusinessError(constant.ErrMissingSchemaTable, "", result.MissingTables, dsID)
	}

	// Generic fallback
	return pkg.ValidateBusinessError(constant.ErrSchemaValidationFailed, "", dsID)
}
