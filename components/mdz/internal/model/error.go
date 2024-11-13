package model

// Error error struct for other structs to use and universal use
type Error struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}
