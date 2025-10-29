// server/middleware/auth.go
package authmiddleware

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/chendingplano/Shared/server/api/auth/tokens"
	"github.com/chendingplano/Shared/server/api/databaseutil"
	"github.com/labstack/echo/v4"
)

// AuthMiddleware protects routes that require authentication
func AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// TODO: Replace with your real auth logic
		// Example: check session, JWT, or API key
		path := c.Request().URL.Path
		if isStaticAsset(path) {
			// Let the request proceed without auth
			log.Printf("path is a static asset (MID_MAT_023):%s", path)
			return next(c)
		}

		log.Printf("path is not a static asset (MID_MAT_027):%s", path)
		db_type := databaseutil.CurrentDatabaseInfo.DBType
		table_name := databaseutil.CurrentDatabaseInfo.SessionsTableName
		user_name, authenticased := IsAuthenticated(c, db_type, table_name, "MID_MAT_019")
		if !authenticased {
			if IsHTMLRequest(c) {
				// It is an HTML request. Redirect the request to "/"
				log.Printf("HTML Request, not authenicated, redirect (MID_MAT_033):%s", path)
				return c.Redirect(http.StatusFound, "/")
			}

			// It is an API call. It should block the call since the requested
			// is not a static asset, which means it requires login to access the asset,
			// and the user is not logged in. Reject it.
			log.Printf("Not an HTML Request, not authenicated, unauthorized (MID_MAT_040):%s", path)
			return c.JSON(http.StatusUnauthorized, map[string]any{
				"error": "Authentication required",
			})
		}

		// ✅ Attach userID to context
		ctx := context.WithValue(c.Request().Context(), "userID", user_name)
		c.SetRequest(c.Request().WithContext(ctx))
		log.Printf("User authenicated, proceed (MID_MAT_045):%s", path)
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

// isAuthenticated checks if the request is from an authenticated user
// Replace this with your actual auth validation (e.g., session store, JWT parsing)
func IsAuthenticated(
			c echo.Context, 
			db_type string,
			table_name string, 
			loc string) (string, bool) {
	log.Printf("Check IsAuthenticated (MID_MAT_035), loc:%s", loc)
	cookie, err := c.Cookie("session_id")
	if err == nil {
		log.Printf("Found cookie (MID_MAT_036):%s", cookie)
		if user_name, valid, _ := databaseutil.IsValidSession(db_type, table_name, cookie.Value); valid {
			log.Printf("Cookie valid (MID_MAT_039)")
			return user_name, true
		}

		// Cookie exists but is invalid → delete it
		log.Printf("To remove cookie:%s (MID_MAT_044)", cookie)
		c.SetCookie(&http.Cookie{
			Name:   "session_id",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
			// Match original cookie attributes:
			HttpOnly: true,
			Secure:   isSecure(), // e.g., true in prod, false in dev
		})
	}

	// 2. Try token-based auth (for API clients)
	// It is not implemented yet. We do not have to implement this unless
	// we want to support API clients.
	authHeader := c.Request().Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if tokens.IsValid(token) { // ← you implement this
			log.Printf("Token is valid (MID_MAT_049): %s", token)
			return "", true
		}
	}

	log.Printf("isAuthenticated failed (MID_MAT_054), cookie: %s, loc:%s", cookie, loc)
	return "", false
}

// isHTMLRequest checks if the client expects an HTML response (browser)
func IsHTMLRequest(c echo.Context) bool {
	accept := c.Request().Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html")
}

// isSecure returns true if the app is running in production (HTTPS expected)
func isSecure() bool {
	// Adjust based on your deployment
	return os.Getenv("ENV") == "production"
}

/*
func deleteCookieHandler(w http.ResponseWriter, r *http.Request) {
    // Create a cookie with the same name, path, and domain
    cookie := &http.Cookie{
        Name:   "session_token",     // Replace with your cookie name
        Value:  "",                  // Optional: can be empty
        Path:   "/",                 // Must match the original cookie's path
        MaxAge: -1,                 // Tells the browser to delete it
        // Domain: "example.com",   // Uncomment if original cookie had a domain
        Secure: true,             // Should match original if used
        HttpOnly: true,           // Should match original if used
    }
    http.SetCookie(w, cookie)

    // Optional: send a response
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Cookie deleted"))
}
*/