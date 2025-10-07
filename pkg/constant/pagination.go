// Package constant provides system-wide constant values used across the Midaz ledger system.
// This file contains pagination-related constants and types.
package constant

// Order is a type that represents the sort ordering direction for paginated list queries.
// It is used throughout the API to specify whether results should be returned in
// ascending or descending order based on the specified sort field.
type Order string

// Sort Order Constants
//
// These constants define the valid values for sort order in paginated queries.
// They are used in conjunction with sort field parameters to control the order
// in which results are returned.
const (
	// Asc represents ascending sort order (A-Z, 0-9, oldest to newest).
	// When applied to:
	//   - Strings: alphabetical order (A to Z)
	//   - Numbers: smallest to largest
	//   - Dates: oldest to newest
	Asc Order = "asc"

	// Desc represents descending sort order (Z-A, 9-0, newest to oldest).
	// When applied to:
	//   - Strings: reverse alphabetical order (Z to A)
	//   - Numbers: largest to smallest
	//   - Dates: newest to oldest
	Desc Order = "desc"
)
