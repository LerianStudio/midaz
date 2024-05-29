package domain

// Pagination is a struct designed to encapsulate pagination response payload data.
type Pagination struct {
	Items interface{} `json:"items"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
}

// SetItems set an array of any struct in items.
func (p *Pagination) SetItems(items interface{}) {
	p.Items = items
}
