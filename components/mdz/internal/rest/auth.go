package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
)

type Auth struct {
	Factory *factory.Factory
}

func (r *Auth) AuthenticateWithCredentials(username, password string) (*model.TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", r.Factory.Env.ClientID)
	data.Set("client_secret", r.Factory.Env.ClientSecret)
	data.Set("username", username)
	data.Set("password", password)

	resp, err := http.PostForm(
		r.Factory.Env.UrlApistring+"/api/login/oauth/access_token", data)
	if err != nil {
		return nil, fmt.Errorf("Error request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error when reading the answer: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Error: status %d - %w", resp.StatusCode, err)
	}

	var tokenResponse model.TokenResponse
	err = json.Unmarshal(body, &tokenResponse)
	if err != nil {
		return nil, fmt.Errorf("Error processing the answer: %w", err)
	}

	return &tokenResponse, nil
}

func (r *Auth) ExchangeToken(code string) (*model.TokenResponse, error) {
	redirectURI := "http://localhost:9000/callback"

	data := url.Values{}
	data.Set("client_id", r.Factory.Env.ClientID)
	data.Set("client_secret", r.Factory.Env.ClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	resp, err := http.PostForm(
		r.Factory.Env.UrlApistring+"/api/login/oauth/access_token", data)
	if err != nil {
		return nil, fmt.Errorf("Request error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Request error: %w", err)
	}

	var tokenResponse model.TokenResponse
	err = json.Unmarshal(body, &tokenResponse)
	if err != nil {
		return nil, fmt.Errorf("Error processing the answer: %w", err)
	}

	return &tokenResponse, nil
}

func NewAuth(f *factory.Factory) *Auth {
	return &Auth{f}
}
