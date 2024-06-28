package http

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/common/mauth"

	"github.com/coreos/go-oidc"
	"github.com/gofiber/fiber/v2"
	"github.com/patrickmn/go-cache"
)

const (
	jwkDefaultDuration = time.Hour * 1
	superOrgScope      = "*"
)

// TokenContextValue is a wrapper type used to keep Context.Locals safe.
type TokenContextValue string

// ProfileID is the profileID type of a member.
type ProfileID string

// OAuth2JWTToken represents a self-contained way for securely transmitting information between parties as a JSON object
// https://tools.ietf.org/html/rfc7519
// type OAuth2JWTToken struct {
// 	Token    *jwt.Token
// 	Claims   jwt.MapClaims
// 	Groups   []string
// 	Sub      string
// 	Username *string
// 	Scope    string
// 	ScopeSet map[string]bool
// }

func NewAuthnMiddleware(app *fiber.App, authClient *mauth.AuthClient) {

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, authClient.Endpoint)
	if err != nil {
		panic(err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: authClient.ClientID, SkipClientIDCheck: true})

	app.Use(func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return Unauthorized(c, "INVALID_REQUEST", "Authorization header required")
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		idToken, err := verifier.Verify(ctx, token)
		if err != nil {
			return Unauthorized(c, "INVALID_REQUEST", "Invalid token")
		}

		var claims map[string]interface{}
		if err := idToken.Claims(&claims); err != nil {
			return Unauthorized(c, "INVALID_REQUEST", "Unable to parse claims")
		}

		c.Locals("claims", claims)
		return c.Next()
	})
}

func WithScope(requiredScopes []string) fiber.Handler {
	return func(c *fiber.Ctx) error {

		token := c.Get("Authorization")
		scopes, err := GetScopes(c, token)
		if err != nil {
			return InternalServerError(c, "Insufficient scopes")
		}
		scopeMap := make(map[string]bool)

		for _, scope := range scopes {
			scopeMap[scope] = true
		}

		for _, requiredScope := range requiredScopes {
			if !scopeMap[requiredScope] {
				return Forbidden(c, "Insufficient scopes")
			}
		}
		return c.Next()
	}
}

// Dummy function to get user scopes (replace with actual implementation)
func GetScopes(c *fiber.Ctx, token string) ([]string, error) {
	// Example: Get scopes from user token or session
	return []string{"organization:create", "organization:view"}, nil
}

// JWKProvider manages cryptographic public keys issued by an authorization server
// See https://tools.ietf.org/html/rfc7517
// It's used to verify JSON Web Tokens which was signed using RS256 signing algorithm.
type JWKProvider struct {
	URI           string
	CacheDuration time.Duration
	cache         *cache.Cache
	once          sync.Once
}

// Fetch fetches (JWKS) JSON Web Key Set from authorization server and cache it
//
//nolint:ireturn
// func (p *JWKProvider) Fetch(ctx context.Context) (jwk.Set, error) {
// 	p.once.Do(func() {
// 		p.cache = cache.New(p.CacheDuration, p.CacheDuration)
// 	})

// 	if set, found := p.cache.Get(p.URI); found {
// 		return set.(jwk.Set), nil
// 	}

// 	set, err := jwk.Fetch(ctx, p.URI)
// 	if err != nil {
// 		return nil, err
// 	}

// 	p.cache.Set(p.URI, set, p.CacheDuration)

// 	return set, nil
// }

// JWTMiddleware represents a middleware which protects endpoint using JWT tokens.
type JWTMiddleware struct {
	JWK *JWKProvider
}
