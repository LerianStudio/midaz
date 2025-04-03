package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
)

// \1 represents an entity
type Auth struct {
	Factory *factory.Factory
}

// func (r *Auth) AuthenticateWithCredentials( performs an operation
func (r *Auth) AuthenticateWithCredentials(
	username, password string) (*model.TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", r.Factory.Env.ClientID)
	data.Set("client_secret", r.Factory.Env.ClientSecret)
	data.Set("username", username)
	data.Set("password", password)

	resp, err := http.PostForm(
		r.Factory.Env.URLAPIAuth+"/api/login/oauth/access_token", data)

	if err != nil {
		return nil, errors.New("error request: " + err.Error())
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, errors.New("error when reading the answer: " + err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var tokenResponse model.TokenResponse

	if err = json.Unmarshal(body, &tokenResponse); err != nil {
		return nil, errors.New("error processing the answer: " + err.Error())
	}

	return &tokenResponse, nil
}

// func (r *Auth) ExchangeToken(code string) (*model.TokenResponse, error) { performs an operation
func (r *Auth) ExchangeToken(code string) (*model.TokenResponse, error) {
	redirectURI := "http://localhost:9000/callback"

	data := url.Values{}
	data.Set("client_id", r.Factory.Env.ClientID)
	data.Set("client_secret", r.Factory.Env.ClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	resp, err := http.PostForm(
		r.Factory.Env.URLAPIAuth+"/api/login/oauth/access_token", data)

	if err != nil {
		return nil, errors.New("request error: " + err.Error())
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, errors.New("request error: " + err.Error())
	}

	var tokenResponse model.TokenResponse

	if err = json.Unmarshal(body, &tokenResponse); err != nil {
		return nil, errors.New("error processing the answer: " + err.Error())
	}

	return &tokenResponse, nil
}

// \1 performs an operation
func NewAuth(f *factory.Factory) *Auth {
	return &Auth{f}
}
