package auth

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	jwtKey     []byte
	jwtKeyOnce sync.Once
	jwtKeyErr  error
)

// initJWTKey loads the JWT signing key from the environment.
// SECURITY: The key MUST be set via JWT_SECRET_KEY environment variable.
// Minimum recommended key length is 32 bytes (256 bits).
func initJWTKey() error {
	jwtKeyOnce.Do(func() {
		secret := os.Getenv("JWT_SECRET_KEY")
		if secret == "" {
			jwtKeyErr = fmt.Errorf("JWT_SECRET_KEY environment variable is not set")
			return
		}
		if len(secret) < 32 {
			jwtKeyErr = fmt.Errorf("JWT_SECRET_KEY must be at least 32 characters (got %d)", len(secret))
			return
		}
		jwtKey = []byte(secret)
	})
	return jwtKeyErr
}

// getJWTKey returns the JWT signing key, initializing it if necessary.
// Returns an error if the key is not properly configured.
func getJWTKey() ([]byte, error) {
	if err := initJWTKey(); err != nil {
		return nil, err
	}
	return jwtKey, nil
}

// IsValid validates a JWT token string.
// Returns true only if:
// - The token signature is valid
// - The token has not expired
// - The token was issued in the past (not future-dated)
func IsValid(tokenStr string) bool {
	key, err := getJWTKey()
	if err != nil {
		return false
	}

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return key, nil
	})

	if err != nil {
		return false
	}

	// Check claims for expiration
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		// Verify expiration
		if exp, ok := claims["exp"].(float64); ok {
			if time.Now().Unix() > int64(exp) {
				return false // Token expired
			}
		}
		// Verify issued-at is not in the future
		if iat, ok := claims["iat"].(float64); ok {
			if time.Now().Unix() < int64(iat)-60 { // 60 second clock skew allowance
				return false // Token issued in future
			}
		}
		return true
	}

	return false
}

// SessionTokenExpiry is the default expiry for session tokens (72 hours).
// SECURITY: This should match the cookie MaxAge in SetCookie.
const SessionTokenExpiry = 72 * time.Hour

// GenerateToken creates a new JWT token with the given claims.
// Automatically adds iat (issued at) and exp (expiration) claims.
// Default expiration is 72 hours (aligned with session cookie).
func GenerateToken(claims map[string]interface{}, expiration time.Duration) (string, error) {
	key, err := getJWTKey()
	if err != nil {
		return "", err
	}

	if expiration == 0 {
		expiration = SessionTokenExpiry // 72 hours, aligned with cookie
	}

	now := time.Now()
	jwtClaims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(expiration).Unix(),
	}

	// Add custom claims
	for k, v := range claims {
		jwtClaims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	return token.SignedString(key)
}

// ParseToken parses and validates a JWT token, returning the claims if valid.
func ParseToken(tokenStr string) (jwt.MapClaims, error) {
	key, err := getJWTKey()
	if err != nil {
		return nil, err
	}

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return key, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}
