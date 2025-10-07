// Package model provides data models for the MDZ CLI internal layer.
//
// This package contains domain models used by the CLI for data transfer
// and error handling.
package model

// Error represents a standardized error structure for CLI operations.
//
// This struct is used for error responses and internal error handling,
// providing consistent error information across the CLI.
type Error struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}
