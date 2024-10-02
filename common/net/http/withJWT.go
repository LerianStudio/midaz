package http

import (
	"context"
	"errors"
	"fmt"
	"github.com/LerianStudio/midaz/common/mcasdoor"
	"strings"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt"
	"github.com/lestrrat-go/jwx/jwk"
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
type OAuth2JWTToken struct {
	Token    *jwt.Token
	Claims   jwt.MapClaims
	Groups   []string
	Sub      string
	Username *string
	Domain   string
	Scope    string
	ScopeSet map[string]bool
}

type TokenParser struct {
	ParseToken func(*jwt.Token) (*OAuth2JWTToken, error)
}

type CasdoorTokenParser struct{}

func (p *CasdoorTokenParser) ParseToken(token *jwt.Token) (*OAuth2JWTToken, error) {
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		t := &OAuth2JWTToken{
			Token:  token,
			Claims: claims,
			Sub:    claims["sub"].(string),
		}
		if username, found := claims["name"].(string); found {
			t.Username = &username
		}

		if domain, found := claims["signupApplication"].(string); found {
			t.Domain = domain
		}

		if scope, found := claims["scope"].(string); found {
			t.Scope = scope
			t.ScopeSet = make(map[string]bool)

			for _, s := range strings.Split(scope, " ") {
				t.ScopeSet[s] = true
			}
		}

		if groups, found := claims["groups"].([]any); found {
			t.Groups = convertGroups(groups)
		}

		return t, nil
	}
	return nil, errors.New("invalid JWT token")
}

func TokenFromContext(c *fiber.Ctx, parser TokenParser) (*OAuth2JWTToken, error) {
	if tokenValue := c.Locals(string(TokenContextValue("token"))); tokenValue != nil {
		if token, ok := tokenValue.(*jwt.Token); ok {
			return parser.ParseToken(token)
		}
	}
	return nil, errors.New("invalid JWT token")
}

func convertGroups(groups []any) []string {
	newGroups := make([]string, 0)

	for _, g := range groups {
		if v, ok := g.(string); ok {
			newGroups = append(newGroups, v)
		}
	}

	return newGroups
}

func getTokenHeader(c *fiber.Ctx) string {
	splitToken := strings.Split(c.Get(fiber.HeaderAuthorization), "Bearer")
	if len(splitToken) == 2 {
		return strings.TrimSpace(splitToken[1])
	}

	return ""
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
	connection *mcasdoor.CasdoorConnection
	JWK        *JWKProvider
}

// NewJWTMiddleware create an instance of JWTMiddleware
// It uses JWK cache duration of 1 hour.
func NewJWTMiddleware(cc *mcasdoor.CasdoorConnection) *JWTMiddleware {
	c := &JWTMiddleware{
		connection: cc,
		JWK: &JWKProvider{
			URI:           cc.JWKUri,
			CacheDuration: jwkDefaultDuration,
		},
	}

	_, err := c.connection.GetClient()
	if err != nil {
		panic("Failed to connect on Casddor")
	}

	return c
}

// Protect protects any endpoint using JWT tokens.
func (m *JWTMiddleware) Protect() fiber.Handler {
	return func(c *fiber.Ctx) error {
		l := mlog.NewLoggerFromContext(c.UserContext())
		l.Debug("JWTMiddleware:Protect")

		l.Debug("Read token from header")

		tokenString := getTokenHeader(c)

		if len(tokenString) == 0 {
			return Unauthorized(c, "INVALID_REQUEST", "Must provide a token")
		}

		// TODO: Need to be cached
		l.Debugf("Get JWK keys using %s", m.JWK.URI)

		keySet, err := m.JWK.Fetch(context.Background())
		if err != nil {
			msg := fmt.Sprint("Couldn't now load JWK keys from source: ", err.Error())
			l.Error(msg)

			return InternalServerError(c, msg)
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}

			kid, ok := token.Header["kid"].(string)
			if !ok {
				return nil, errors.New("kid header not found")
			}

			key, ok := keySet.LookupKeyID(kid)
			if !ok {
				return nil, errors.New("the provided token doesn't belongs to the required trusted issuer, check the identity server you logged in")
			}

			var raw any

			if err := key.Raw(&raw); err != nil {
				return nil, err
			}

			return raw, nil
		})
		if err != nil {
			l.Error(err.Error())
			return Unauthorized(c, "AUTH_SERVER_ERROR", err.Error())
		}

		if token.Valid {
			// Check if the token is expired
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if exp, ok := claims["exp"].(float64); ok {
					if time.Unix(int64(exp), 0).Before(time.Now()) {
						return Unauthorized(c, "INVALID_TOKEN", "Token is expired")
					}
				}
			}

			l.Debug("Token ok")
			c.Locals(string(TokenContextValue("token")), token)

			return c.Next()
		}

		return Unauthorized(c, "INVALID_TOKEN", "Invalid token")
	}
}

// WithScope verify if a requester has the required scope to access an endpoint.
func (m *JWTMiddleware) WithScope(scopes []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		parser := TokenParser{
			ParseToken: (&CasdoorTokenParser{}).ParseToken,
		}
		t, err := TokenFromContext(c, parser)
		if err != nil {
			return Unauthorized(c, "INVALID_SCOPE", "Unauthorized")
		}

		authorized := false

		for _, s := range scopes {
			if _, found := t.ScopeSet[s]; found {
				authorized = true
				break
			}
		}

		if authorized || len(scopes) == 0 {
			return c.Next()
		}

		return Forbidden(c, "Insufficient privileges")
	}
}

// WithPermission verify if a requester has the required permission to access an endpoint.
func (jwtm *JWTMiddleware) WithPermission(resource string) fiber.Handler {
	client, err := jwtm.connection.GetClient()
	if err != nil {
		panic("Failed to connect on Casddor")
	}

	return func(c *fiber.Ctx) error {
		parser := TokenParser{
			ParseToken: (&CasdoorTokenParser{}).ParseToken,
		}
		t, err := TokenFromContext(c, parser)
		if err != nil {
			return Unauthorized(c, "INVALID_PERMISSION", "Unauthorized")
		}

		println(t.Sub)
		authorized, err := client.Enforce("", "", "", "", "", nil)
		if err != nil {
			panic("Failed to connect on Casddor")
		}

		if authorized || len(resource) == 0 {

			return c.Next()
		}

		return Forbidden(c, "Insufficient privileges")
	}
}
