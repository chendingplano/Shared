package authmiddleware

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
)

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
		user_info, err := IsAuthenticated(rc)
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

func IsValidSessionPG(
	rc ApiTypes.RequestContext,
	session_id string) (*ApiTypes.UserInfo, bool, error) {
	// This function checks whether 'session_id' is valid in the sessions table.
	// If valid, return user_name.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions
	logger := rc.GetLogger()

	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		query = fmt.Sprintf("SELECT user_email FROM %s WHERE session_id = ? AND expires_at > NOW() LIMIT 1", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("SELECT user_email FROM %s WHERE session_id = $1 AND expires_at > NOW() LIMIT 1", table_name)

	default:
		error_msg := fmt.Errorf("unsupported database type (SHD_DBS_234): %s", db_type)
		logger.Error(
			"unsupported database type",
			"database type", db_type)
		return nil, false, error_msg
	}

	var user_email sql.NullString
	err := db.QueryRow(query, session_id).Scan(&user_email)
	if err != nil || !user_email.Valid {
		if err == sql.ErrNoRows {
			error_msg := fmt.Sprintf("user not found:%s (SHD_DBS_333)", user_email.String)
			logger.Warn(
				"user not found",
				"error", error_msg)
			return nil, false, nil

		}

		error_msg := fmt.Errorf("failed to validate session (SHD_DBS_268): %w", err)
		logger.Error(error_msg.Error())
		return nil, false, error_msg
	}

	logger.Info("Check session",
		"stmt", query,
		"user_email", user_email.String)

	user_info, err := sysdatastores.GetUserInfoByEmail(rc, user_email.String)
	if err == nil {
		logger.Info("valid session",
			"email", user_email,
			"is_admin", user_info.Admin,
			"status", user_info.UserStatus,
			"user_id", user_info.UserId)
		return user_info, true, nil
	}

	logger.Error("failed retrieving user", "error", err, "email", user_email)
	return user_info, false, err
	/*
		const selected_fields = "id, name, user_id_type, first_name, last_name," +
			"email, user_mobile, user_address, verified, admin, " +
			"email_visibility, user_status, avatar, locale"

		table_name = ApiTypes.LibConfig.SystemTableNames.TableNameUsers
		switch db_type {
		case ApiTypes.MysqlName:
			db = ApiTypes.MySql_DB_miner
			query = fmt.Sprintf("SELECT %s FROM %s WHERE email = ? LIMIT 1",
				selected_fields, table_name)

		case ApiTypes.PgName:
			db = ApiTypes.PG_DB_miner
			query = fmt.Sprintf("SELECT %s FROM %s WHERE email = $1 LIMIT 1",
				selected_fields, table_name)

		default:
			error_msg := fmt.Errorf("unsupported database type (SHD_DBS_234): %s", db_type)
			logger.Error("unsupported database type",
				"database_type", db_type)
			return ApiTypes.UserInfo{}, false, error_msg
		}

		var user_info ApiTypes.UserInfo
		var user_mobile, user_address, user_id, user_name, user_id_type,
			first_name, last_name, avatar, locale,
			email, verified, admin, emailVisibility,
			user_status sql.NullString
		err = db.QueryRow(query, user_email).Scan(
			&user_id,
			&user_name,
			&user_id_type,
			&first_name,
			&last_name,
			&email,
			&user_mobile,
			&user_address,
			&verified,
			&admin,
			&emailVisibility,
			&user_status,
			&avatar,
			&locale)

		if err != nil {
			if err == sql.ErrNoRows {
				logger.Warn("No sessions found",
					"session_id", session_id,
					"user_email", user_email.String,
					"loc", "SHD_UTL_329")
				return ApiTypes.UserInfo{}, false, nil
			}

			error_msg := fmt.Errorf("failed to validate session (SHD_DBS_248): %w, stmt:%s", err, query)
			logger.Error("failed to validate session",
				"error", err,
				"stmt", query)
			return ApiTypes.UserInfo{}, false, error_msg
		}

		if user_id.Valid {
			user_info.UserId = user_id.String
		}

		if user_name.Valid {
			user_info.UserName = user_name.String
		}

		if user_id_type.Valid {
			user_info.UserIdType = user_id_type.String
		}

		if first_name.Valid {
			user_info.FirstName = first_name.String
		}

		if last_name.Valid {
			user_info.LastName = last_name.String
		}

		if user_mobile.Valid {
			user_info.UserMobile = user_mobile.String
		}

		if user_address.Valid {
			user_info.UserAddress = user_address.String
		}

		if avatar.Valid {
			user_info.Avatar = avatar.String
		}

		if locale.Valid {
			user_info.Locale = locale.String
		}

		if email.Valid {
			user_info.Email = email.String
		}

		if verified.Valid {
			user_info.Verified = verified.String == "true"
		}

		if admin.Valid {
			user_info.Admin = admin.String == "true"
		}

		if emailVisibility.Valid {
			user_info.EmailVisibility = emailVisibility.String == "true"
		}

		if user_status.Valid {
			user_info.UserStatus = user_status.String
		}
	*/

}

// isAuthenticated checks if the request is from an authenticated user
// It retrieves the cookie from 'c'. If no cookie is found, it returns
// 'no cookie found' and false.
// If cookie is found, it checks whether the cookie is valid. If yes,
// it returns user_name and true.
// If the cookie is invalid, it removes the cookie.
func IsAuthenticated(rc ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
	logger := rc.GetLogger()

	cookie := rc.GetCookie("session_id")
	logger.Info("isAuthenticated invoked", "cookie", cookie)
	if cookie != "" {
		user_info, valid, _ := IsValidSessionPG(rc, cookie)
		if valid {
			logger.Info("Cookie valid", "email", user_info.Email)
			return user_info, nil
		}

		// Cookie exists but is invalid → delete it
		logger.Info("Cookie invalid, remove cookie", "cookie", cookie)
		/*
			rc.SetCookie(&http.Cookie{
				Name:   "session_id",
				Value:  "",
				Path:   "/",
				MaxAge: -1,
				// Match original cookie attributes:
				HttpOnly: true,
				Secure:   ApiUtils.IsSecure(), // e.g., true in prod, false in dev
			})
		*/
		rc.SetCookie(cookie)
		return nil, nil
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

	logger.Warn("isAuthenticated failed")
	return nil, nil
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
