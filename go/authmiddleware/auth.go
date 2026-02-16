package authmiddleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// KratosAuthenticator is an optional fallback authenticator for Kratos sessions.
// Set this from libmanager.InitLib() when AUTH_USE_KRATOS=true to enable
// Kratos session validation as a fallback when the old session_id cookie is not found.
// This uses the function-pointer pattern (same as DefaultAuthenticator) to avoid
// circular imports between authmiddleware and auth packages.
var KratosAuthenticator func(rc ApiTypes.RequestContext) (*ApiTypes.UserInfo, error)

func Init() {
	// Register the authenticator with EchoFactory to break the import cycle.
	// EchoFactory can now call IsAuthenticated without importing auth-middleware.
	EchoFactory.DefaultAuthenticator = IsAuthenticated
}

// AuthMiddleware protects routes that require authentication
func AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// 🔑 Generate a unique request ID
		// reqID := generateRequestID()
		// 🧵 Create a new context with the request ID
		// ctx := context.WithValue(c.Request().Context(), ApiTypes.RequestIDKey, reqID)

		// 🔄 Replace the request context
		// c.SetRequest(c.Request().WithContext(ctx))
		rc := EchoFactory.NewFromEcho(c, "SHD_ATH_026")
		logger := rc.GetLogger()
		defer rc.Close()
		ctx := c.Request().Context()

		path := c.Request().URL.Path
		if isStaticAsset(path) {
			return next(c)
		}

		logger.Info("route path", "path", path)
		user_info, err := IsAuthenticated(rc)
		if err != nil || user_info == nil {
			clientIP := c.RealIP()
			if IsHTMLRequest(c) {
				logger.Warn("auth failed, redirect",
					"error", err,
					"path", path,
					"ip", clientIP)
				return c.Redirect(http.StatusFound, "/")
			}

			logger.Warn("auth failed, unauthorized API call",
				"error", err,
				"path", path,
				"ip", clientIP,
				"method", c.Request().Method)
			return c.JSON(http.StatusUnauthorized, map[string]any{
				"error": "Authentication required",
			})
		}

		// Attach UserContextKey to context
		user_name := user_info.UserName
		ctx = context.WithValue(c.Request().Context(), ApiTypes.UserContextKey, user_name)
		c.SetRequest(c.Request().WithContext(ctx))
		logger.Info("User authenticated, proceed", "path", path)
		return next(c)
	}
}

// isStaticAsset returns true if the path likely refers to a public static asset.
// Static assets never require authentication. We may want to add these to
// a config file.
func isStaticAsset(path string) bool {
	// Common static asset patterns
	if strings.Contains(path, ".") {
		return true
	}
	// SvelteKit/Vite dev server internal paths
	if strings.HasPrefix(path, "/@vite/") ||
		strings.HasPrefix(path, "/@id/") ||
		strings.HasPrefix(path, "/@fs/") ||
		strings.HasPrefix(path, "/node_modules/") ||
		strings.HasPrefix(path, "/_app/") ||
		strings.HasPrefix(path, "/__data__/") {
		return true
	}
	return false
}

// IsAuthenticated checks if the request is from an authenticated user.
// Validates the session via Kratos.
// Returns:
//   - (user_info, nil) on success
//   - (nil, error) when auth fails or no valid session exists
func IsAuthenticated(rc ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
	logger := rc.GetLogger()

	// Clean up any stale legacy session_id cookies from before Kratos migration
	if cookie := rc.GetCookie("session_id"); cookie != "" {
		rc.DeleteCookie("session_id")
	}

	// Validate session via Kratos
	if KratosAuthenticator != nil {
		user_info, err := KratosAuthenticator(rc)
		if err != nil {
			logger.Warn("Kratos auth failed", "error", err)
			return nil, fmt.Errorf("kratos auth error: %w", err)
		}
		if user_info != nil {
			return user_info, nil
		}
	}

	return nil, fmt.Errorf("no valid session found")
}

// isHTMLRequest checks if the client expects an HTML response (browser)
func IsHTMLRequest(c echo.Context) bool {
	accept := c.Request().Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html")
}
