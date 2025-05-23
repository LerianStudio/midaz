package mmodel

// Pagination struct for cursor-based pagination.
//
// swagger:model Pagination
// @Description Pagination is the struct designed to store pagination data.
type Pagination struct {
	// Current page number in the pagination
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`

	// Cursor for next page navigation (optional)
	// example: next_cursor_token
	NextCursor *string `json:"nextCursor,omitempty" example:"next_cursor_token"`

	// Cursor for previous page navigation (optional)
	// example: prev_cursor_token
	PrevCursor *string `json:"prevCursor,omitempty" example:"prev_cursor_token"`
}
