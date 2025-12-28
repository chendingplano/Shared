package authmiddleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/labstack/echo/v4"
)

// AuthMiddleware protects routes that require authentication
func AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// 🔑 Generate a unique request ID
		// reqID := generateRequestID()
		// 🧵 Create a new context with the request ID
		// ctx := context.WithValue(c.Request().Context(), ApiTypes.RequestIDKey, reqID)

		// 🔄 Replace the request context
		// c.SetRequest(c.Request().WithContext(ctx))
		ctx := c.Request().Context()
		reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
		log.Printf("[req=%s] AuthMiddleware (SHD_AUT_028)", reqID)

		path := c.Request().URL.Path
		if isStaticAsset(path) {
			// Let the request proceed without auth
			log.Printf("[req=%s] path is a static asset (SHD_MAT_023):%s", reqID, path)
			return next(c)
		}

		log.Printf("[req=%s] ============= path is not a static asset (SHD_MAT_027):%s", reqID, path)
		user_info, err := IsAuthenticated(c, "SHD_MAT_019")
		if err != nil {
			if IsHTMLRequest(c) {
				// It is an HTML request. Redirect the request to "/"
				log.Printf("[req=%s] auth failed, err:%v, redirect (SHD_MAT_033):%s", reqID, err, path)
				return c.Redirect(http.StatusFound, "/")
			}

			// It is an API call. It should block the call since the requested
			// is not a static asset, which means it requires login to access the asset,
			// and the user is not logged in. Reject it.
			log.Printf("[req=%s] Not an HTML Request, not authenicated, unauthorized (SHD_MAT_040):%s", reqID, path)
			return c.JSON(http.StatusUnauthorized, map[string]any{
				"error": "Authentication required",
			})
		}

		// ✅ Attach UserContextKey to context
		user_name := user_info.UserName
		ctx = context.WithValue(c.Request().Context(), ApiTypes.UserContextKey, user_name)
		c.SetRequest(c.Request().WithContext(ctx))
		log.Printf("[req=%s] User authenicated, proceed (SHD_MAT_045):%s", reqID, path)
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
// It retrieves the cookie from 'c'. If no cookie is found, it returns
// 'no cookie found' and false.
// If cookie is found, it checks whether the cookie is valid. If yes,
// it returns user_name and true.
// If the cookie is invalid, it removes the cookie.
func IsAuthenticated(
	c echo.Context,
	loc string) (ApiTypes.UserInfo, error) {
	ctx := c.Request().Context()
	// reqID, ok := ctx.Value(ApiTypes.RequestIDKey).(string)
	reqID, ok := ctx.Value("reqID").(string)
	if !ok {
		log.Printf("***** Alarm failed retrieving reqID (SHD_AUT_112)")
	}

	cookie, err := c.Cookie("session_id")
	if err == nil {
		user_info, valid, _ := ApiUtils.IsValidSessionPG(reqID, cookie.Value)
		if valid {
			log.Printf("[req=%s] Cookie valid (SHD_MAT_039), email:%s", reqID, user_info.Email)
			return user_info, nil
		}

		// Cookie exists but is invalid → delete it
		log.Printf("[req=%s] Cookie invalid, remove cookie:%s (SHD_MAT_044)", reqID, cookie)
		c.SetCookie(&http.Cookie{
			Name:   "session_id",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
			// Match original cookie attributes:
			HttpOnly: true,
			Secure:   ApiUtils.IsSecure(), // e.g., true in prod, false in dev
		})
		return ApiTypes.UserInfo{}, fmt.Errorf("cookie invalid, cookie removed (SHD_MAT_112)")
	}

	// 2. Try token-based auth (for API clients)
	// It is not implemented yet. We do not have to implement this unless
	// we want to support API clients.
	/*
		Need to import "github.com/chendingplano/Shared/go/api/auth/tokens"
		authHeader := c.Request().Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if tokens.IsValid(token) { // ← you implement this
				log.Printf("Token is valid (SHD_MAT_049): %s", token)
				return "", true
			}
		}
	*/

	log.Printf("[req=%s] isAuthenticated failed (SHD_MAT_054), err: %v, loc:%s", reqID, err, loc)
	return ApiTypes.UserInfo{}, fmt.Errorf("user not logged in (SHD_MAT_131)")
}

// isHTMLRequest checks if the client expects an HTML response (browser)
func IsHTMLRequest(c echo.Context) bool {
	accept := c.Request().Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html")
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
