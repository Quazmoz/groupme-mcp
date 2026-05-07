package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrMissingToken    = errors.New("missing authorization token")
	ErrInvalidToken    = errors.New("invalid token format")
	ErrTokenExpired    = errors.New("token has expired")
	ErrInvalidAudience = errors.New("invalid audience claim")
	ErrMissingSubject  = errors.New("missing subject claim")
)

// JWTClaims represents the claims we expect in Open WebUI JWTs.
type JWTClaims struct {
	jwt.RegisteredClaims
}

// ValidateOpenWebUIJWT validates a JWT from Open WebUI and extracts the user ID.
// It checks:
// - Token signature using the provided secret
// - Audience matches expectedAudience
// - Token is not expired
// - Subject (user ID) is present
func ValidateOpenWebUIJWT(tokenString, jwtSecret, expectedAudience string) (userID string, err error) {
	if tokenString == "" {
		return "", ErrMissingToken
	}

	// Remove "Bearer " prefix if present
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	tokenString = strings.TrimSpace(tokenString)

	if tokenString == "" {
		return "", ErrMissingToken
	}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrTokenExpired
		}
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return "", ErrInvalidToken
	}

	// Check expiration
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return "", ErrTokenExpired
	}

	// Check audience if expectedAudience is specified
	if expectedAudience != "" {
		audienceValid := false
		for _, aud := range claims.Audience {
			if aud == expectedAudience {
				audienceValid = true
				break
			}
		}
		if !audienceValid {
			return "", ErrInvalidAudience
		}
	}

	// Extract subject (user ID)
	if claims.Subject == "" {
		return "", ErrMissingSubject
	}

	return claims.Subject, nil
}

// ExtractBearerToken extracts the token from an Authorization header value.
func ExtractBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	return strings.TrimPrefix(authHeader, "Bearer ")
}
