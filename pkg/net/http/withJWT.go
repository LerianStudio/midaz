package http

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"strings"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	cn "github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mcasdoor"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
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
	Owner    string
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

		if owner, found := claims["owner"].(string); found {
			t.Owner = owner
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

func getTokenHeaderFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	authHeader, ok := md["authorization"]
	if !ok || len(authHeader) == 0 {
		return ""
	}

	return strings.TrimPrefix(authHeader[0], "Bearer ")
}

func parseToken(tokenString string, keySet jwk.Set) (*jwt.Token, error) {
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

		var tokenString any

		if err := key.Raw(&tokenString); err != nil {
			return nil, err
		}

		return tokenString, nil
	})
	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return token, nil
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

// ProtectHTTP protects any endpoint using JWT tokens.
func (jwtm *JWTMiddleware) ProtectHTTP() fiber.Handler {
	return func(c *fiber.Ctx) error {
		l := pkg.NewLoggerFromContext(c.UserContext())
		l.Info("JWTMiddleware:ProtectHTTP")

		l.Info("Read token from header")

		tokenString := GetTokenHeader(c)

		if pkg.IsNilOrEmpty(&tokenString) {
			msg := errors.Wrap(errors.New("token not found in context"), "No token found in context")
			l.Error(msg.Error())

			err := pkg.ValidateBusinessError(cn.ErrTokenMissing, "JWT Token")

			return WithError(c, err)
		}

		l.Infof("Get JWK keys using %s", jwtm.JWK.URI)

		keySet, err := jwtm.JWK.Fetch(c.Context())
		if err != nil {
			msg := errors.Wrap(err, "Couldn't load JWK keys from source")
			l.Error(msg.Error())

			err := pkg.ValidateBusinessError(cn.ErrJWKFetch, "JWT Token")

			return WithError(c, err)
		}

		token, err := parseToken(tokenString, keySet)
		if err != nil {
			msg := errors.Wrap(err, "Couldn't parse token")
			l.Error(msg.Error())

			err := pkg.ValidateBusinessError(cn.ErrInvalidToken, "JWT Token")

			return WithError(c, err)
		}

		l.Info("Token ok")
		c.Locals(string(TokenContextValue("token")), token)

		return c.Next()
	}
}

// WithScope verify if a requester has the required scope to access an endpoint.
func (jwtm *JWTMiddleware) WithScope(scopes []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		l := pkg.NewLoggerFromContext(c.UserContext())
		l.Info("JWTMiddleware:WithScope")

		parser := TokenParser{
			ParseToken: (&CasdoorTokenParser{}).ParseToken,
		}

		t, err := TokenFromContext(c, parser)
		if err != nil {
			msg := errors.Wrap(err, "Couldn't parse token")
			l.Error(msg.Error())

			err := pkg.ValidateBusinessError(cn.ErrInvalidToken, "JWT Token")

			return WithError(c, err)
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

		err = pkg.ValidateBusinessError(cn.ErrInsufficientPrivileges, "JWT Token")

		return WithError(c, err)
	}
}

// WithPermissionHTTP verify if a requester has the required permission to access an endpoint.
func (jwtm *JWTMiddleware) WithPermissionHTTP(resource string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		l := pkg.NewLoggerFromContext(c.UserContext())
		l.Info("JWTMiddleware:WithPermissionHTTP")

		client, err := jwtm.connection.GetClient()
		if err != nil {
			l.Error(err.Error())
			panic("Failed to connect on Casdoor")
		}

		parser := TokenParser{
			ParseToken: (&CasdoorTokenParser{}).ParseToken,
		}

		t, err := TokenFromContext(c, parser)
		if err != nil {
			msg := errors.Wrap(err, "Couldn't parse token")
			l.Error(msg.Error())

			err = pkg.ValidateBusinessError(cn.ErrInvalidToken, "JWT Token")

			return WithError(c, err)
		}

		model := fmt.Sprintf("%s/%s", t.Owner, jwtm.connection.ModelName)
		usr := fmt.Sprintf("%s/%s", t.Owner, *t.Username)

		toValidate := casdoorsdk.CasbinRequest{
			usr,
			resource,
			strings.ToLower(c.Method()), // Always send the method as lowercase
		}

		authorized, err := client.Enforce("", model, "", "", "", toValidate)
		if err != nil {
			msg := errors.Wrap(err, "Failed to enforce permission")
			l.Error(msg.Error())

			err = pkg.ValidateBusinessError(cn.ErrPermissionEnforcement, "JWT Token")

			return WithError(c, err)
		}

		if authorized || len(resource) == 0 {
			return c.Next()
		}

		l.Info("Unauthorized")

		err = pkg.ValidateBusinessError(cn.ErrInsufficientPrivileges, "JWT Token")

		return WithError(c, err)
	}
}

// ProtectGrpc protects any gRPC endpoint using JWT tokens.
func (jwtm *JWTMiddleware) ProtectGrpc() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		l := pkg.NewLoggerFromContext(ctx)
		l.Info("JWTMiddleware:ProtectGrpc")

		tokenString := getTokenHeaderFromContext(ctx)

		if len(tokenString) == 0 {
			msg := errors.Wrap(errors.New("token not found in context"), "No token found in context")
			l.Error(msg.Error())

			e := pkg.ValidateBusinessError(cn.ErrTokenMissing, "JWT Token")

			return nil, jwtm.errorHandlingGrpc(codes.Unauthenticated, e)
		}

		l.Infof("Get JWK keys using %s", jwtm.JWK.URI)

		keySet, err := jwtm.JWK.Fetch(context.Background())
		if err != nil {
			msg := errors.Wrap(err, "Couldn't load JWK keys from source")
			l.Error(msg.Error())

			e := pkg.ValidateBusinessError(cn.ErrJWKFetch, "JWT Token")

			return nil, jwtm.errorHandlingGrpc(codes.FailedPrecondition, e)
		}

		token, err := parseToken(tokenString, keySet)
		if err != nil {
			msg := errors.Wrap(err, "Couldn't parse token")
			l.Error(msg.Error())

			e := pkg.ValidateBusinessError(cn.ErrInvalidToken, "JWT Token")

			return nil, jwtm.errorHandlingGrpc(codes.Unauthenticated, e)
		}

		ctx = context.WithValue(ctx, TokenContextValue("token"), token)

		return handler(ctx, req)
	}
}

// WithPermissionGrpc verify if a requester has the required permission to access an endpoint.
func (jwtm *JWTMiddleware) WithPermissionGrpc() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		l := pkg.NewLoggerFromContext(ctx)
		l.Info("JWTMiddleware:WithPermissionGrpc")

		client, err := jwtm.connection.GetClient()
		if err != nil {
			l.Error(err.Error())
			panic("Failed to connect on Casdoor")
		}

		t, err := jwtm.getTokenFromContext(ctx)
		if err != nil {
			msg := errors.Wrap(err, "Couldn't parse token")
			l.Error(msg.Error())

			e := pkg.ValidateBusinessError(cn.ErrInvalidToken, "JWT Token")

			return nil, jwtm.errorHandlingGrpc(codes.Unauthenticated, e)
		}

		model := fmt.Sprintf("%s/%s", t.Owner, jwtm.connection.ModelName)
		usr := fmt.Sprintf("%s/%s", t.Owner, *t.Username)

		toValidate := casdoorsdk.CasbinRequest{
			usr,
			jwtm.extractMethod(info.FullMethod),
			strings.ToLower("*"), // Always send the method as lowercase
		}

		authorized, err := client.Enforce("", model, "", "", "", toValidate)
		if err != nil {
			msg := errors.Wrap(err, "Failed to enforce permission")
			l.Error(msg.Error())

			e := pkg.ValidateBusinessError(cn.ErrPermissionEnforcement, "JWT Token")

			return nil, jwtm.errorHandlingGrpc(codes.FailedPrecondition, e)
		}

		if !authorized {
			l.Info("Unauthorized")

			e := pkg.ValidateBusinessError(cn.ErrInsufficientPrivileges, "JWT Token")

			return nil, jwtm.errorHandlingGrpc(codes.PermissionDenied, e)
		}

		return handler(ctx, req)
	}
}

func (jwtm *JWTMiddleware) getTokenFromContext(ctx context.Context) (*OAuth2JWTToken, error) {
	tokenValue := ctx.Value(TokenContextValue("token"))
	if tokenValue == nil {
		return nil, errors.New("token not found in context")
	}

	token, ok := tokenValue.(*jwt.Token)
	if !ok {
		return nil, errors.New("invalid token type")
	}

	parser := TokenParser{
		ParseToken: (&CasdoorTokenParser{}).ParseToken,
	}

	return parser.ParseToken(token)
}

func (jwtm *JWTMiddleware) errorHandlingGrpc(code codes.Code, e any) error {
	jsonStringError, err := pkg.StructToJSONString(e)
	if err != nil {
		return status.Error(codes.Internal, "Failed to marshal error response")
	}

	return status.Error(code, jsonStringError)
}

func (jwtm *JWTMiddleware) extractMethod(s string) string {
	method := strings.Split(s, "/")[1]
	return method
}
