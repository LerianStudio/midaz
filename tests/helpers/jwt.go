package helpers

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testSigningKey is a shared RSA key for test JWT generation.
// In tests, we generate a new key if not set.
var testSigningKey *rsa.PrivateKey

func init() {
	// Generate a test signing key on package initialization
	var err error
	testSigningKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("failed to generate test RSA key: " + err.Error())
	}
}

// TenantClaims represents JWT claims with tenant context.
// The "tenantId" claim is used as the tenant identifier (hardcoded).
type TenantClaims struct {
	jwt.RegisteredClaims

	// Owner is the tenant ID (organization name from Casdoor)
	Owner string `json:"owner,omitempty"`

	// TenantID is an alternative claim key for tenant identification
	TenantID string `json:"tenantId,omitempty"`

	// TenantSlug is an optional human-readable tenant identifier
	TenantSlug string `json:"tenantSlug,omitempty"`

	// UserID is the authenticated user's identifier
	UserID string `json:"sub,omitempty"`

	// Username is the authenticated user's name
	Username string `json:"name,omitempty"`

	// Email is the authenticated user's email
	Email string `json:"email,omitempty"`

	// Roles contains the user's roles for authorization
	Roles []string `json:"roles,omitempty"`
}

// GenerateTestJWT creates a JWT token with the specified tenant context.
// The token includes the tenant ID in the "tenantId" claim (hardcoded claim key).
// This is suitable for testing multi-tenant scenarios.
//
// Parameters:
//   - tenantID: The tenant identifier (organization name)
//   - tenantSlug: Optional human-readable tenant slug
//   - userID: The user identifier (subject claim)
//
// Returns the signed JWT string or an error.
func GenerateTestJWT(tenantID, tenantSlug, userID string) (string, error) {
	now := time.Now()

	claims := TenantClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    getTestIssuer(),
			Subject:   userID,
			Audience:  jwt.ClaimStrings{"midaz"},
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        RandHex(16),
		},
		Owner:      tenantID,
		TenantID:   tenantID,
		TenantSlug: tenantSlug,
		UserID:     userID,
		Username:   "test-user",
		Email:      "test@example.com",
		Roles:      []string{"admin"},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	// Sign with test key
	signedToken, err := token.SignedString(testSigningKey)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

// GenerateTestJWTWithClaims creates a JWT with custom claims for advanced testing.
// This allows full control over the token claims for edge case testing.
func GenerateTestJWTWithClaims(claims TenantClaims) (string, error) {
	// Set defaults if not provided
	now := time.Now()
	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(24 * time.Hour))
	}
	if claims.IssuedAt == nil {
		claims.IssuedAt = jwt.NewNumericDate(now)
	}
	if claims.NotBefore == nil {
		claims.NotBefore = jwt.NewNumericDate(now)
	}
	if claims.Issuer == "" {
		claims.Issuer = getTestIssuer()
	}
	if claims.ID == "" {
		claims.ID = RandHex(16)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	signedToken, err := token.SignedString(testSigningKey)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

// GenerateTestJWTWithoutTenant creates a JWT without tenant claims.
// This is useful for testing error handling when tenant context is missing.
func GenerateTestJWTWithoutTenant(userID string) (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		Issuer:    getTestIssuer(),
		Subject:   userID,
		Audience:  jwt.ClaimStrings{"midaz"},
		ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ID:        RandHex(16),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	signedToken, err := token.SignedString(testSigningKey)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

// GenerateExpiredTestJWT creates an expired JWT for testing token validation.
func GenerateExpiredTestJWT(tenantID, userID string) (string, error) {
	past := time.Now().Add(-24 * time.Hour)

	claims := TenantClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    getTestIssuer(),
			Subject:   userID,
			Audience:  jwt.ClaimStrings{"midaz"},
			ExpiresAt: jwt.NewNumericDate(past),
			IssuedAt:  jwt.NewNumericDate(past.Add(-1 * time.Hour)),
			NotBefore: jwt.NewNumericDate(past.Add(-1 * time.Hour)),
			ID:        RandHex(16),
		},
		Owner:    tenantID,
		TenantID: tenantID,
		UserID:   userID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	signedToken, err := token.SignedString(testSigningKey)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

// GetTestPublicKey returns the public key for verifying test JWTs.
// This can be used to configure test servers to accept test tokens.
func GetTestPublicKey() *rsa.PublicKey {
	return &testSigningKey.PublicKey
}

// getTestIssuer returns the JWT issuer for test tokens.
func getTestIssuer() string {
	if issuer := os.Getenv("TEST_JWT_ISSUER"); issuer != "" {
		return issuer
	}
	return "midaz-test"
}

// IsMultiTenantEnabled checks if multi-tenant mode is enabled via environment.
func IsMultiTenantEnabled() bool {
	return os.Getenv("MULTI_TENANT_ENABLED") == "true"
}

// GetMultiTenantURL returns the tenant manager URL from environment.
func GetMultiTenantURL() string {
	return os.Getenv("MULTI_TENANT_URL")
}
