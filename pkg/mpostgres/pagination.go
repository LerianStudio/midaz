package mpostgres

import "time"

// Pagination is a struct designed to encapsulate pagination response payload data.
//
// swagger:model Pagination
// @Description Pagination is the struct designed to store the pagination data of an entity list.
type Pagination struct {
	Items      any       `json:"items"`
	Page       int       `json:"page,omitempty" example:"1"`
	PrevCursor string    `json:"prev_cursor,omitempty" example:"MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA=="`
	NextCursor string    `json:"next_cursor,omitempty" example:"MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA=="`
	Limit      int       `json:"limit" example:"10"`
	SortOrder  string    `json:"-" example:"asc"`
	StartDate  time.Time `json:"-" example:"2021-01-01"`
	EndDate    time.Time `json:"-" example:"2021-12-31"`
} // @name Pagination

// SetItems set an array of any struct in items.
func (p *Pagination) SetItems(items any) {
	p.Items = items
}

// SetCursor set the next and previous cursor.
func (p *Pagination) SetCursor(next, prev string) {
	p.NextCursor = next
	p.PrevCursor = prev
}
