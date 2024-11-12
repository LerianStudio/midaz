package mmodel

// Pagination is a struct designed to encapsulate pagination response payload data.
type Pagination struct {
	Items any `json:"items"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}
