// Package repository defines repository interfaces for the MDZ CLI domain layer.
// This file contains the Auth repository interface.
package repository

import "github.com/LerianStudio/midaz/v3/components/mdz/internal/model"

// Auth defines the interface for authentication operations.
//
// This interface abstracts authentication operations, allowing CLI commands
// to authenticate without knowing the underlying HTTP implementation.
type Auth interface {
	AuthenticateWithCredentials(username, password string) (*model.TokenResponse, error)
	ExchangeToken(code string) (*model.TokenResponse, error)
}
