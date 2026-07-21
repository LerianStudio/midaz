// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import "time"

// DateTime format constants for API validation.
// RFC3339 is required for all date/time query parameters (startDate, endDate).
const (
	// DateTimeFormat defines the required format for date/time query parameters.
	// RFC3339 format mandates timezone (e.g., 2026-01-28T10:30:00Z or 2026-01-28T10:30:00-03:00).
	// Date-only formats (e.g., 2026-01-28) or timestamps without timezone are rejected.
	DateTimeFormat = time.RFC3339

	// DateTimeFormatExample provides a human-readable example for error messages.
	DateTimeFormatExample = "2026-01-28T10:30:00Z"
)
