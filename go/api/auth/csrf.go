// Package auth provides CSRF protection for state-changing API endpoints.
// This implements the double-submit cookie pattern, which is effective for
// JSON APIs where requests come from JavaScript.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

const (
	// CSRFCookieName is the name of the cookie containing the CSRF token
	CSRFCookieName = "csrf_token"
	// CSRFHeaderName is the header that must contain the matching token
	CSRFHeaderName = "X-CSRF-Token"
	// CSRFTokenLength is the length of the random token in bytes (256 bits)
	CSRFTokenLength = 32
	// CSRFTokenExpiry is how long the CSRF token is valid
	CSRFTokenExpiry = 24 * time.Hour
)

// GenerateCSRFToken creates a new cryptographically secure CSRF token
func GenerateCSRFToken() (string, error) {
	bytes := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// SetCSRFCookie sets the CSRF token cookie on the response
func SetCSRFCookie(c echo.Context) (string, error) {
	token, err := GenerateCSRFToken()
	if err != nil {
		return "", err
	}

	isSecure := os.Getenv("ENV") == "production"

	cookie := &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // JavaScript must be able to read this
		Secure:   isSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(CSRFTokenExpiry.Seconds()),
	}
	c.SetCookie(cookie)
	return token, nil
}

// ValidateCSRFToken validates that the CSRF header matches the cookie.
// Returns true if the tokens match, false otherwise.
// Uses constant-time comparison to prevent timing attacks.
func ValidateCSRFToken(c echo.Context) bool {
	// Get token from cookie
	cookie, err := c.Cookie(CSRFCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	// Get token from header
	headerToken := c.Request().Header.Get(CSRFHeaderName)
	if headerToken == "" {
		return false
	}

	// Constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) == 1
}

// CSRFMiddleware provides CSRF protection for routes.
// It generates a token on GET requests and validates it on POST/PUT/DELETE.
// Safe methods (GET, HEAD, OPTIONS) are always allowed.
func CSRFMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		method := c.Request().Method

		// Safe methods - generate token if not present
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			// Check if CSRF cookie exists, if not generate one
			if _, err := c.Cookie(CSRFCookieName); err != nil {
				SetCSRFCookie(c)
			}
			return next(c)
		}

		// State-changing methods - validate token
		if method == http.MethodPost || method == http.MethodPut ||
			method == http.MethodDelete || method == http.MethodPatch {
			if !ValidateCSRFToken(c) {
				return c.JSON(http.StatusForbidden, map[string]string{
					"status":  "error",
					"message": "CSRF token validation failed",
					"loc":     "SHD_CSRF_001",
				})
			}
		}

		return next(c)
	}
}

// CSRFProtectedHandler wraps a handler with CSRF validation.
// Use this for individual handlers instead of middleware.
func CSRFProtectedHandler(
	rc ApiTypes.RequestContext,
	headerToken string,
	cookieToken string,
) bool {
	if headerToken == "" || cookieToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) == 1
}

// IsSafeOrigin checks if the request origin is from a trusted domain.
// This is an additional layer of CSRF protection.
//
// SECURITY: This function is strict about requiring Origin or Referer headers
// for state-changing requests. Requests without these headers are only allowed
// if they appear to be same-origin (checking other indicators).
func IsSafeOrigin(c echo.Context) bool {
	origin := c.Request().Header.Get("Origin")
	referer := c.Request().Header.Get("Referer")

	// Get allowed domain from environment
	appDomain := os.Getenv("APP_DOMAIN_NAME")
	if appDomain == "" {
		// If no domain configured, reject all cross-origin requests in production
		if os.Getenv("ENV") == "production" {
			return false
		}
		// In development without domain configured, be permissive
		return true
	}

	// If we have an Origin header, validate it
	if origin != "" {
		return isOriginAllowed(origin, appDomain)
	}

	// No Origin header - check Referer as fallback
	if referer != "" {
		return isOriginAllowed(referer, appDomain)
	}

	// No Origin or Referer header
	// This could be:
	// 1. Same-origin request (browsers don't always send Origin for same-origin)
	// 2. Non-browser client (curl, Postman, etc.)
	// 3. Privacy extension blocking headers
	//
	// SECURITY: In production, we're stricter - check for other same-origin indicators
	if os.Getenv("ENV") == "production" {
		// Check if this looks like a fetch/XHR request (has typical headers)
		// Same-origin requests from modern browsers typically have these
		contentType := c.Request().Header.Get("Content-Type")
		accept := c.Request().Header.Get("Accept")

		// If it's a JSON API request without origin headers, it's suspicious
		if strings.Contains(contentType, "application/json") ||
			strings.Contains(accept, "application/json") {
			// Require at least one origin indicator for JSON requests
			return false
		}
	}

	// In development, allow requests without origin headers
	return os.Getenv("ENV") != "production"
}

// isOriginAllowed checks if the given origin matches the allowed domain
func isOriginAllowed(origin string, appDomain string) bool {
	// Strip protocol for comparison
	origin = strings.TrimPrefix(origin, "http://")
	origin = strings.TrimPrefix(origin, "https://")
	appDomain = strings.TrimPrefix(appDomain, "http://")
	appDomain = strings.TrimPrefix(appDomain, "https://")

	// Origin might include port and path
	if strings.HasPrefix(origin, appDomain) {
		return true
	}

	// Allow localhost in development
	if os.Getenv("ENV") != "production" {
		if strings.HasPrefix(origin, "localhost") || strings.HasPrefix(origin, "127.0.0.1") {
			return true
		}
	}

	return false
}
