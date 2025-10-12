package constant

// Order represents the sort order direction for paginated results.
type Order string

// Order constants define the available sort directions for listings.
const (
	// Asc sorts results in ascending order (smallest to largest, A to Z).
	Asc Order = "asc"
	// Desc sorts results in descending order (largest to smallest, Z to A).
	Desc Order = "desc"
)
