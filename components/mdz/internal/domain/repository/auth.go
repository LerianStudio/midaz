package repository

import "github.com/LerianStudio/midaz/components/mdz/internal/model"

// \1 represents an entity
type Auth interface {
	AuthenticateWithCredentials(username, password string) (*model.TokenResponse, error)
	ExchangeToken(code string) (*model.TokenResponse, error)
}
