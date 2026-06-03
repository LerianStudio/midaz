// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

// Pagination is a struct designed to encapsulate pagination response payload data.
//
// swagger:model Pagination
//
//	@Description	Pagination is the struct designed to store the pagination data of an entity list.
type Pagination struct {
	Items any `json:"items"`
	Page  int `json:"page,omitempty" example:"1"`
	Limit int `json:"limit" example:"10"`
	Total int `json:"total" example:"10"`
} //	@name	Pagination

// SetItems set an array of any struct in items.
func (p *Pagination) SetItems(items any) {
	p.Items = items
}

// SetTotal set the total of items.
func (p *Pagination) SetTotal(total int) {
	p.Total = total
}
