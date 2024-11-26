package mpostgres

// Pagination is a struct designed to encapsulate pagination response payload data.
//
// swagger:model Pagination
// @Description Pagination is the struct designed to store the pagination data of an entity list.
type Pagination struct {
	Items any `json:"items"`
	Page  int `json:"page" example:"1"`
	Limit int `json:"limit" example:"10"`
} // @name Pagination

// SetItems set an array of any struct in items.
func (p *Pagination) SetItems(items any) {
	p.Items = items
}
