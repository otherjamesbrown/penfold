// Package auth provides shared authentication functionality for Go microservices.
// It supports JWT token validation, claims extraction, and API key authentication.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Common authentication errors.
var (
	// ErrInvalidToken is returned when a token cannot be parsed or validated.
	ErrInvalidToken = errors.New("invalid token")
	// ErrTokenExpired is returned when a token has expired.
	ErrTokenExpired = errors.New("token expired")
	// ErrInvalidClaims is returned when token claims are invalid or malformed.
	ErrInvalidClaims = errors.New("invalid claims")
	// ErrMissingToken is returned when no token is provided.
	ErrMissingToken = errors.New("missing token")
	// ErrInsufficientPermissions is returned when the user lacks required permissions.
	ErrInsufficientPermissions = errors.New("insufficient permissions")
)

// Claims represents the JWT claims for authentication.
type Claims struct {
	// Subject is the user identifier (typically user ID or email).
	Subject string `json:"sub"`
	// Roles are the permissions/roles assigned to the user.
	Roles []string `json:"roles,omitempty"`
	// ExpiresAt is the token expiration time.
	ExpiresAt time.Time `json:"exp"`
	// IssuedAt is the token issue time.
	IssuedAt time.Time `json:"iat,omitempty"`
	// Issuer is the token issuer.
	Issuer string `json:"iss,omitempty"`
	// Custom contains additional custom claims.
	Custom map[string]interface{} `json:"custom,omitempty"`
}

// HasRole checks if the claims include the specified role.
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsExpired checks if the token has expired.
func (c *Claims) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// TokenValidator defines the interface for validating authentication tokens.
type TokenValidator interface {
	// ValidateToken validates a token string and returns the claims.
	ValidateToken(tokenString string) (*Claims, error)
}

// TokenGenerator defines the interface for generating authentication tokens.
type TokenGenerator interface {
	// GenerateToken creates a new token for the given claims with the specified expiry.
	GenerateToken(claims *Claims, expiry time.Duration) (string, error)
}

// jwtClaims wraps Claims for JWT library compatibility.
type jwtClaims struct {
	Subject string                 `json:"sub"`
	Roles   []string               `json:"roles,omitempty"`
	Issuer  string                 `json:"iss,omitempty"`
	Custom  map[string]interface{} `json:"custom,omitempty"`
	jwt.RegisteredClaims
}

// JWTValidator validates JWT tokens using a shared secret.
type JWTValidator struct {
	secretKey []byte
	issuer    string
}

// JWTValidatorOption configures a JWTValidator.
type JWTValidatorOption func(*JWTValidator)

// WithIssuer sets the expected issuer for token validation.
func WithIssuer(issuer string) JWTValidatorOption {
	return func(v *JWTValidator) {
		v.issuer = issuer
	}
}

// NewJWTValidator creates a new JWT validator with the given secret key.
func NewJWTValidator(secretKey string, opts ...JWTValidatorOption) *JWTValidator {
	v := &JWTValidator{
		secretKey: []byte(secretKey),
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// ValidateToken validates a JWT token string and returns the claims.
func (v *JWTValidator) ValidateToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, ErrMissingToken
	}

	token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	jwtClaims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidClaims
	}

	// Validate issuer if configured
	if v.issuer != "" && jwtClaims.Issuer != v.issuer {
		return nil, fmt.Errorf("%w: issuer mismatch", ErrInvalidClaims)
	}

	claims := &Claims{
		Subject: jwtClaims.Subject,
		Roles:   jwtClaims.Roles,
		Issuer:  jwtClaims.Issuer,
		Custom:  jwtClaims.Custom,
	}

	if jwtClaims.ExpiresAt != nil {
		claims.ExpiresAt = jwtClaims.ExpiresAt.Time
	}
	if jwtClaims.IssuedAt != nil {
		claims.IssuedAt = jwtClaims.IssuedAt.Time
	}

	return claims, nil
}

// GenerateToken creates a new JWT token for the given claims.
// This is primarily for testing and internal service-to-service communication.
func (v *JWTValidator) GenerateToken(claims *Claims, expiry time.Duration) (string, error) {
	now := time.Now()
	expiresAt := now.Add(expiry)

	jwtClaims := &jwtClaims{
		Subject: claims.Subject,
		Roles:   claims.Roles,
		Issuer:  claims.Issuer,
		Custom:  claims.Custom,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   claims.Subject,
			Issuer:    claims.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	return token.SignedString(v.secretKey)
}

// Ensure JWTValidator implements both interfaces.
var (
	_ TokenValidator = (*JWTValidator)(nil)
	_ TokenGenerator = (*JWTValidator)(nil)
)
