// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"errors"
	"strings"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
)

// sectionFailure records a per-database (section) data-retrieval failure with its
// canonical error code (E9). Used to classify a report as PARTIAL when only some
// sections fail.
type sectionFailure struct {
	database  string
	errorCode string
}

// classifyReporterErrorCode extracts the canonical numeric error code carried by a
// typed reporter error (E9: classified code, never raw text). It falls back to the
// generic internal-server code when the error is untyped.
func classifyReporterErrorCode(err error) string {
	var (
		validationErr   pkgErr.ValidationError
		preconditionErr pkgErr.FailedPreconditionError
		notFoundErr     pkgErr.EntityNotFoundError
		unauthorizedErr pkgErr.UnauthorizedError
		internalErr     pkgErr.InternalServerError
	)

	switch {
	case errors.As(err, &validationErr):
		return validationErr.Code
	case errors.As(err, &preconditionErr):
		return preconditionErr.Code
	case errors.As(err, &notFoundErr):
		return notFoundErr.Code
	case errors.As(err, &unauthorizedErr):
		return unauthorizedErr.Code
	case errors.As(err, &internalErr):
		return internalErr.Code
	default:
		return cnErr.ErrInternalServer.Error()
	}
}

// decideReportStatus classifies the terminal report status from the number of
// attempted sections and the accumulated per-section failures:
//
//	all sections ok -> FinishedStatus (no failure metadata)
//	all sections failed -> ErrorStatus (with per-section codes)
//	mixed -> PartialStatus (with per-section codes for the failed sections)
//
// When failures exist, the returned metadata carries a "sections" map keyed by
// database name, each value an {"error_code": <canonical>} entry (E9).
func decideReportStatus(attempted int, failures []sectionFailure) (string, map[string]any) {
	if len(failures) == 0 {
		return constant.FinishedStatus, nil
	}

	sections := make(map[string]any, len(failures))
	for _, f := range failures {
		sections[f.database] = map[string]any{"error_code": f.errorCode}
	}

	metadata := map[string]any{"sections": sections}

	if len(failures) >= attempted {
		metadata["error"] = "All report data sections failed"
		metadata["error_code"] = cnErr.ErrExtractionJobFailed.Error()

		return constant.ErrorStatus, metadata
	}

	metadata["error"] = "Some report data sections failed"
	metadata["error_code"] = cnErr.ErrExtractionJobFailed.Error()

	return constant.PartialStatus, metadata
}

// getTableFilters extracts filters for a specific table/collection
// Supports multiple table name formats:
// - "schema__table" (Pongo2 format)
// - "schema.table" (qualified format)
// - "table" (simple format, will try with "public." prefix)
func getTableFilters(databaseFilters map[string]map[string]model.FilterCondition, tableName string) map[string]model.FilterCondition {
	if databaseFilters == nil {
		return nil
	}

	// Try exact match first
	if filters, ok := databaseFilters[tableName]; ok {
		return filters
	}

	// Try alternative formats
	var alternativeKeys []string

	if strings.Contains(tableName, "__") {
		// Pongo2 format: schema__table -> try schema.table
		alternativeKeys = append(alternativeKeys, strings.Replace(tableName, "__", ".", 1))
	} else if strings.Contains(tableName, ".") {
		// Qualified format: schema.table -> try schema__table
		alternativeKeys = append(alternativeKeys, strings.Replace(tableName, ".", "__", 1))
	} else {
		// Simple table name without schema -> try with public schema
		// This handles the case where template has "organization" but filter has "public.organization"
		alternativeKeys = append(alternativeKeys, "public."+tableName)
		alternativeKeys = append(alternativeKeys, "public__"+tableName)
	}

	for _, altKey := range alternativeKeys {
		if filters, ok := databaseFilters[altKey]; ok {
			return filters
		}
	}

	return nil
}
