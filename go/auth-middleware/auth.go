package authmiddleware

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
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
			log.Printf("path is a static asset (SHD_MAT_023):%s", path)
			return next(c)
		}

		log.Printf("============= path is not a static asset (SHD_MAT_027):%s", path)
		user_name, err := IsAuthenticated(c, "SHD_MAT_019")
		if err != nil {
			if IsHTMLRequest(c) {
				// It is an HTML request. Redirect the request to "/"
				log.Printf("auth failed, err:%v, redirect (SHD_MAT_033):%s", err, path)
				return c.Redirect(http.StatusFound, "/")
			}

			// It is an API call. It should block the call since the requested
			// is not a static asset, which means it requires login to access the asset,
			// and the user is not logged in. Reject it.
			log.Printf("Not an HTML Request, not authenicated, unauthorized (SHD_MAT_040):%s", path)
			return c.JSON(http.StatusUnauthorized, map[string]any{
				"error": "Authentication required",
			})
		}

		// ✅ Attach UserContextKey to context
		ctx := context.WithValue(c.Request().Context(), ApiTypes.UserContextKey, user_name)
		c.SetRequest(c.Request().WithContext(ctx))
		log.Printf("User authenicated, proceed (SHD_MAT_045):%s", path)
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
			loc string) (string, error) {
	log.Printf("Check IsAuthenticated (SHD_MAT_083), loc:%s", loc)
	cookie, err := c.Cookie("session_id")
	if err == nil {
		log.Printf("Found cookie (SHD_MAT_036):%s", cookie)
		user_name, valid, _ := IsValidSession(cookie.Value)
		if valid {
			log.Printf("Cookie valid (SHD_MAT_039)")
			return user_name, nil
		}

		// Cookie exists but is invalid → delete it
		log.Printf("Cookie invalid, remove cookie:%s (SHD_MAT_044)", cookie)
		c.SetCookie(&http.Cookie{
			Name:   "session_id",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
			// Match original cookie attributes:
			HttpOnly: true,
			Secure:   IsSecure(), // e.g., true in prod, false in dev
		})
		return user_name, fmt.Errorf("cookie invalid, cookie removed (SHD_MAT_112)")
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

	log.Printf("isAuthenticated failed (SHD_MAT_054), err: %v, loc:%s", err, loc)
	return "", fmt.Errorf("user not logged in (SHD_MAT_131)")
}

// isHTMLRequest checks if the client expects an HTML response (browser)
func IsHTMLRequest(c echo.Context) bool {
	accept := c.Request().Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html")
}

// IsSecure returns true if the app is running in production (HTTPS expected)
func IsSecure() bool {
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

func IsValidSession(session_id string) (string, bool, error) {
    // This function checks whether 'session_id' is valid in the sessions table.
    // If valid, return user_name.
    var query string
    var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableName_Sessions
    log.Printf("Check IsValidSession (SHD_DBS_251), db_type:%s", db_type)
    switch db_type {
    case ApiTypes.MysqlName:
         db = ApiTypes.MySql_DB_miner
         query = fmt.Sprintf("SELECT user_name FROM %s WHERE session_id = ? AND expires_at > NOW() LIMIT 1", table_name)

    case ApiTypes.PgName:
         db = ApiTypes.PG_DB_miner
         query = fmt.Sprintf("SELECT user_name FROM %s WHERE session_id = $1 AND expires_at > NOW() LIMIT 1", table_name)

    default:
         error_msg := fmt.Errorf("unsupported database type (SHD_DBS_234): %s", db_type)
         log.Printf("***** Alarm %s:", error_msg.Error())
         return "", false, error_msg
    }

    var user_name string
    err := db.QueryRow(query, session_id).Scan(&user_name)
    if err != nil {
        if err == sql.ErrNoRows {   
            error_msg := fmt.Sprintf("user not found:%s (SHD_DBS_333)", user_name)
            log.Printf("%s", error_msg)
            return "", false, nil

        }

        error_msg := fmt.Errorf("failed to validate session (SHD_DBS_240): %w", err)
        log.Printf("***** Alarm:%s", error_msg)
        return "", false, error_msg
    }
    log.Printf("Check session (SHD_DBS_271), stmt: %s, user_name:%s", query, user_name)
    return user_name, user_name != "", nil
}