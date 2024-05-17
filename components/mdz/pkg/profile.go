package pkg

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/zitadel/oidc/v2/pkg/client/rp"
	"github.com/zitadel/oidc/v2/pkg/oidc"
	"golang.org/x/oauth2"
)

// ErrInvalidAuthentication is an error that occurs when the authentication token is invalid.
type ErrInvalidAuthentication struct {
	err error
}

// Error returns the error message for ErrInvalidAuthentication.
//
// No parameters.
// Returns a string.
func (e ErrInvalidAuthentication) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying error of ErrInvalidAuthentication.
//
// No parameters.
// Returns an error.
func (e ErrInvalidAuthentication) Unwrap() error {
	return e.err
}

// Is checks if the provided error is of type ErrInvalidAuthentication.
func (e ErrInvalidAuthentication) Is(err error) bool {
	_, ok := err.(*ErrInvalidAuthentication)
	return ok
}

// IsInvalidAuthentication checks if the provided error is an instance of ErrInvalidAuthentication.
func IsInvalidAuthentication(err error) bool {
	return errors.Is(err, &ErrInvalidAuthentication{})
}

func newErrInvalidAuthentication(err error) *ErrInvalidAuthentication {
	return &ErrInvalidAuthentication{
		err: err,
	}
}

// AuthClient is the name of the OIDC client.
const AuthClient = "mdz"

type persistedProfile struct {
	MembershipURI       string                    `json:"membershipURI"`
	Token               *oidc.AccessTokenResponse `json:"token"`
	DefaultOrganization string                    `json:"defaultOrganization"`
}

// Profile represents a user profile.
type Profile struct {
	membershipURI       string
	token               *oidc.AccessTokenResponse
	defaultOrganization string
	config              *Config
}

// UpdateToken updates the token for the Profile.
//
// token *oidc.AccessTokenResponse - The new access token to be set.
func (p *Profile) UpdateToken(token *oidc.AccessTokenResponse) {
	p.token = token
}

// SetMembershipURI sets the membership URI for the Profile.
//
// Takes a string parameter.
func (p *Profile) SetMembershipURI(v string) {
	p.membershipURI = v
}

// MarshalJSON generates the JSON encoding for the Profile struct.
//
// No parameters.
// Returns a byte slice and an error.
func (p *Profile) MarshalJSON() ([]byte, error) {
	return json.Marshal(persistedProfile{
		MembershipURI:       p.membershipURI,
		Token:               p.token,
		DefaultOrganization: p.defaultOrganization,
	})
}

// UnmarshalJSON parses the JSON-encoded data and stores the result in the Profile struct.
//
// It takes a byte slice data as a parameter.
// Returns an error.
func (p *Profile) UnmarshalJSON(data []byte) error {
	cfg := &persistedProfile{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return err
	}

	*p = Profile{
		membershipURI:       cfg.MembershipURI,
		token:               cfg.Token,
		defaultOrganization: cfg.DefaultOrganization,
	}

	return nil
}

// GetMembershipURI returns the membership URI of the Profile.
//
// No parameters.
// Returns a string.
func (p *Profile) GetMembershipURI() string {
	return p.membershipURI
}

// GetDefaultOrganization returns the default organization for the Profile.
//
// No parameters.
// Returns a string.
func (p *Profile) GetDefaultOrganization() string {
	return p.defaultOrganization
}

// GetToken retrieves and refreshes the OAuth2 token if needed.
//
// Parameters:
//   - ctx: the context for the HTTP request
//   - httpClient: the HTTP client to use for making HTTP requests
//
// Returns:
//   - *oauth2.Token: the OAuth2 token retrieved or refreshed
//   - error: an error if any occurred during the token retrieval or refresh
func (p *Profile) GetToken(ctx context.Context, httpClient *http.Client) (*oauth2.Token, error) {
	if p.token == nil {
		return nil, errors.New("not authenticated")
	}

	if p.token != nil {
		claims := &oidc.AccessTokenClaims{}

		if _, err := oidc.ParseToken(p.token.AccessToken, claims); err != nil {
			return nil, newErrInvalidAuthentication(errors.Wrap(err, "parsing token"))
		}

		if claims.Expiration.AsTime().Before(time.Now()) {
			relyingParty, err := GetAuthRelyingParty(httpClient, p.membershipURI)
			if err != nil {
				return nil, err
			}

			newToken, err := rp.RefreshAccessToken(relyingParty, p.token.RefreshToken, "", "")
			if err != nil {
				return nil, newErrInvalidAuthentication(errors.Wrap(err, "refreshing token"))
			}

			p.UpdateToken(&oidc.AccessTokenResponse{
				AccessToken:  newToken.AccessToken,
				TokenType:    newToken.TokenType,
				RefreshToken: newToken.RefreshToken,
				IDToken:      newToken.Extra("id_token").(string),
			})

			if err := p.config.Persist(); err != nil {
				return nil, err
			}
		}
	}

	claims := &oidc.AccessTokenClaims{}
	if _, err := oidc.ParseToken(p.token.AccessToken, claims); err != nil {
		return nil, newErrInvalidAuthentication(err)
	}

	return &oauth2.Token{
		AccessToken:  p.token.AccessToken,
		TokenType:    p.token.TokenType,
		RefreshToken: p.token.RefreshToken,
		Expiry:       claims.Expiration.AsTime(),
	}, nil
}

// GetClaims returns the jwt claims and an error.
//
// No parameters.
// Returns jwt.MapClaims and error.
func (p *Profile) GetClaims() (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	parser := jwt.Parser{}

	if _, _, err := parser.ParseUnverified(p.token.AccessToken, claims); err != nil {
		return nil, err
	}

	return claims, nil
}

func (p *Profile) getUserInfo() (*userClaims, error) {
	claims := &userClaims{}
	if p.token != nil && p.token.IDToken != "" {
		_, err := oidc.ParseToken(p.token.IDToken, claims)
		if err != nil {
			return nil, err
		}
	}

	return claims, nil
}

// SetDefaultOrganization sets the default organization
func (p *Profile) SetDefaultOrganization(o string) {
	p.defaultOrganization = o
}

// IsConnected is connected
func (p *Profile) IsConnected() bool {
	return p.token != nil
}

// CurrentProfile profile
type CurrentProfile Profile

// ListProfiles generates a list of profiles based on the toComplete string.
//
// Parameters:
//
//	cmd: *cobra.Command - the command object
//	toComplete: string - the string to complete
//
// Return type:
//
//	[]string: list of profile strings
//	error: any error that occurred
func ListProfiles(cmd *cobra.Command, toComplete string) ([]string, error) {
	config, err := GetConfig(cmd)
	if err != nil {
		return []string{}, err
	}

	ret := make([]string, 0)

	for p := range config.GetProfiles() {
		if strings.HasPrefix(p, toComplete) {
			ret = append(ret, p)
		}
	}

	sort.Strings(ret)

	return ret, nil
}
