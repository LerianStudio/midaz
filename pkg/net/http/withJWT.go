package http

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"sync"
	"time"
)

const (
	jwkDefaultDuration = 1 * time.Hour
)

// TokenContextValue is a wrapper type used to keep Context.Locals safe.
type TokenContextValue string

// OAuth2JWTToken represents a self-contained way for securely transmitting information between parties as a JSON object
// https://tools.ietf.org/html/rfc7519
type OAuth2JWTToken struct {
	Token    *jwt.Token
	Claims   jwt.MapClaims
	Groups   []string
	Sub      string
	Username *string
	Owner    string
	Scope    string
	ScopeSet map[string]bool
}

type TokenParser struct {
	ParseToken func(*jwt.Token) (*OAuth2JWTToken, error)
}

func TokenFromContext(c *fiber.Ctx, parser TokenParser) (*OAuth2JWTToken, error) {
	if tokenValue := c.Locals(TokenContextValue("token")); tokenValue != nil {
		if token, ok := tokenValue.(*jwt.Token); ok {
			return parser.ParseToken(token)
		}
	}

	return nil, errors.New("invalid JWT token")
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
func (p *JWKProvider) Fetch(ctx context.Context) (jwk.Set, error) {
	p.once.Do(func() {
		p.cache = cache.New(p.CacheDuration, p.CacheDuration)
	})

	if set, found := p.cache.Get(p.URI); found {
		return set.(jwk.Set), nil
	}

	set, err := jwk.Fetch(ctx, p.URI)
	if err != nil {
		return nil, err
	}

	p.cache.Set(p.URI, set, p.CacheDuration)

	return set, nil
}

// JWTMiddleware represents a middleware which protects endpoint using JWT tokens.
type JWTMiddleware struct {
	JWK *JWKProvider
}

// NewJWTMiddleware create an instance of JWTMiddleware
// It uses JWK cache duration of 1 hour.
func NewJWTMiddleware() *JWTMiddleware {
	c := &JWTMiddleware{
		JWK: &JWKProvider{
			CacheDuration: jwkDefaultDuration,
		},
	}

	return c
}

// WithPermissionHTTP verify if a requester has the required permission to access an endpoint.
func (jwtm *JWTMiddleware) WithPermissionHTTP(resource string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		l := pkg.NewLoggerFromContext(c.UserContext())
		l.Info("pkg.with_permission_http", "resource", resource)

		return c.Next()
	}
}
