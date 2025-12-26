package query

// paginateSlice returns the slice window for the requested page/limit.
// It keeps ordering intact and returns an empty slice when out of range.
func paginateSlice[T any](items []T, page, limit int) []T {
	if len(items) == 0 {
		return items
	}

	if limit <= 0 {
		return items
	}

	if page <= 0 {
		page = 1
	}

	start := (page - 1) * limit
	if start >= len(items) {
		return items[:0]
	}

	end := start + limit
	if end > len(items) {
		end = len(items)
	}

	return items[start:end]
}
