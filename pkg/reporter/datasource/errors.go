// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import "github.com/LerianStudio/midaz/v4/pkg/constant"

// Error sentinels for datasource operations. Each sentinel re-exports a numeric
// constant from pkg/constant so datasource code can errors.Is against it while
// pkg.ValidateBusinessError maps the same constant to a typed HTTP error.
var (
	// ErrDataSourceNotFound indicates the requested data source ID does not exist
	// in the registered datasource registry. Maps to EntityNotFoundError via
	// ValidateBusinessError.
	ErrDataSourceNotFound = constant.ErrDataSourceNotFound

	// ErrDataSourceUnavailable indicates a data source exists but cannot be reached.
	// Used for D7 warning pattern: unavailability produces a ValidationWarning
	// rather than a hard failure, allowing partial results. Maps to ValidationError
	// via ValidateBusinessError.
	ErrDataSourceUnavailable = constant.ErrDataSourceUnavailable
)

// Warning code constants for ValidationWarning.Code values.
const (
	// WarningCodeDataSourceUnavailable is the warning code used when a data source
	// is temporarily unavailable. Per D7 decision, this produces a warning (not an
	// error) so that report generation can proceed with partial results.
	WarningCodeDataSourceUnavailable = "DATA_SOURCE_UNAVAILABLE"
)
