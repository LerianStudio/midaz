// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

type Order string

// Order is a type that represents the ordering of a list.
const (
	Asc  Order = "asc"
	Desc Order = "desc"
)

// Pagination limits for list endpoints.
const (
	// DefaultPaginationLimit is the default number of items per page when not specified.
	DefaultPaginationLimit = 10

	// MinPaginationLimit is the minimum allowed items per page.
	MinPaginationLimit = 1

	// MaxPaginationLimit is the maximum allowed items per page.
	MaxPaginationLimit = 100
)
