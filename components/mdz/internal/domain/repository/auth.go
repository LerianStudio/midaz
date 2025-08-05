package repository

import "github.com/LerianStudio/midaz/v3/components/mdz/internal/model"

type Auth interface {
	AuthenticateWithCredentials(username, password string) (*model.TokenResponse, error)
	ExchangeToken(code string) (*model.TokenResponse, error)
}
