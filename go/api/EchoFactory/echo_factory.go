// //////////////////////////////////////////////////////////////
// EchoFactory provides the Echo implementation of RequestContext.
// This package exists to break the import cycle between
// RequestHandlers and auth-middleware.
//
// Created: 2026/01/14 by Chen Ding
// //////////////////////////////////////////////////////////////

package EchoFactory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

// AuthenticatorFunc is a function type for authentication.
// This allows dependency injection to avoid import cycles.
type AuthenticatorFunc func(rc ApiTypes.RequestContext) (*ApiTypes.UserInfo, error)

// DefaultAuthenticator is set by auth-middleware package during init
// (shared/go/api/authmiddleware/auth.go)
// This breaks the import cycle by using runtime registration instead of compile-time imports.
var DefaultAuthenticator AuthenticatorFunc

type echoContext struct {
	c            echo.Context
	ctx          context.Context
	logger       *loggerutil.JimoLogger
	call_flow    []string
	user_info    *ApiTypes.UserInfo
	user_checked bool
	is_admin     bool
}

func NewRCAsAdmin(loc string) ApiTypes.RequestContext {
	// This RC is used internally!
	ctx := context.Background()
	logger := loggerutil.CreateLogger(ctx, loggerutil.LogHandlerTypeDefault)
	ee := &echoContext{
		call_flow:    []string{loc},
		ctx:          ctx,
		logger:       logger,
		user_checked: false,
		is_admin:     true,
	}

	ee.PushCallFlow(loc)
	return ee
}

func NewFromEcho(c echo.Context, loc string) ApiTypes.RequestContext {
	ctx := c.Request().Context()
	logger := loggerutil.CreateLogger(ctx, loggerutil.LogHandlerTypeDefault)
	ee := &echoContext{
		c:         c,
		call_flow: []string{loc},
		ctx:       ctx,
		logger:    logger,
		is_admin:  false,
	}

	ee.PushCallFlow(loc)
	return ee
}

func (e *echoContext) Context() context.Context {
	return e.c.Request().Context()
}

func (e *echoContext) GetRequest() *http.Request {
	return e.c.Request()
}

func (e *echoContext) GetBody() io.ReadCloser {
	return e.c.Request().Body
}

func (e *echoContext) Close() {
	e.logger.Close()
}

func (e *echoContext) FormValue(name string) string {
	return e.c.FormValue(name)
}

func (e *echoContext) GetUserID() string {
	if e.user_info != nil {
		return e.user_info.UserId
	}

	if e.user_checked {
		return ""
	}

	e.user_info = e.IsAuthenticated()
	e.user_checked = true
	if e.user_info != nil {
		return e.user_info.UserId
	}
	return ""
}

func (e *echoContext) GetCookie(name string) string {
	cookie, err := e.c.Cookie(name)
	if err == nil {
		return cookie.Value
	}
	return ""
}

func (e *echoContext) QueryParam(key string) string {
	return e.c.QueryParam(key)
}

// func (e *echoContext) SetCookie(cookie *http.Cookie) {
func (e *echoContext) SetCookie(session_id string) {
	is_secure := ApiUtils.IsSecure()
	cookie := new(http.Cookie)
	cookie.Name = "session_id"
	cookie.Value = session_id
	cookie.Path = "/"
	cookie.HttpOnly = true
	cookie.Secure = is_secure
	cookie.SameSite = http.SameSiteStrictMode
	e.c.SetCookie(cookie)
}

func (e *echoContext) ReqID() string {
	if id, ok := e.c.Get(ApiTypes.RequestIDKey).(string); ok && id != "" {
		return id
	}
	// Generate and store
	id := ApiUtils.GenerateRequestID("e")
	e.c.Set(ApiTypes.RequestIDKey, id)
	return id
}

func (e *echoContext) SetReqID(reqID string) {
	e.c.Set(ApiTypes.RequestIDKey, reqID)
}

func (e *echoContext) GetLogger() *loggerutil.JimoLogger {
	return e.logger
}

func (e *echoContext) Bind(v interface{}) error {
	return e.c.Bind(v)
}

func (e *echoContext) GenerateAuthToken(email string) (string, error) {
	// For Echo/PostgreSQL/MySQL, we don't use Pocketbase auth tokens
	// Instead, we return a session-based token or JWT
	// This is a placeholder - implement based on your auth strategy
	token := ApiUtils.GenerateSecureToken(32)
	return token, nil
}

func (e *echoContext) UpdatePassword(
	email string,
	plaintextPassword string) (bool, int, string) {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcrypt.DefaultCost)
	if err != nil {
		error_msg := fmt.Sprintf("failed to hash password, email:%s, err:%v", email, err)
		e.logger.Error("failed to hash password", "email", email, "error", err)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_PasswordUpdateFailure,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RCP_218"})

		return false, http.StatusInternalServerError, error_msg
	}

	err = sysdatastores.UpdatePasswordByEmail(e, email, string(hashedPassword))
	if err != nil {
		error_msg := fmt.Sprintf("failed to update password in database, email:%s, err:%v", email, err)
		e.logger.Error("failed to update password", "email", email, "error", err)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_PasswordUpdateFailure,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RCP_238"})

		return false, http.StatusInternalServerError, error_msg
	}

	return true, 0, ""
}

func (e *echoContext) VerifyUserPassword(
	email string,
	password string) (bool, int, string) {

	user_info, found := e.GetUserInfoByEmail(email)
	logger := e.logger
	if !found {
		logger.Warn("No user found", "email", email)
		return false, http.StatusNotFound, "email not found (SHD_RCE_131)"
	}

	if user_info.Password == "" {
		home_domain := os.Getenv("APP_DOMAIN_NAME")
		if home_domain == "" {
			logger.Error("missing APP_DOMAIN_NAME env var")
		}
		token := uuid.NewString()
		url := fmt.Sprintf("%s/reset-password?token=%s", home_domain, token)
		logger.Warn("user not set password yet. Sent reset password email",
			"email", email,
			"token", token,
			"url", url)
		error_msg := fmt.Sprintf("user not set password yet, sent reset password email to:%s, email:%s, token:%s",
			email, token, url)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_SentEmail,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RCE_216"})

		e.UpdateTokenByEmail(email, token)

		subject := "Reset your password"
		htmlBody := fmt.Sprintf(`
        	<p>Please click the link below to reset your password:</p>
        	<p><a href="%s">%s</a></p>`, url, url)
		textBody := fmt.Sprintf("Please click the link below to reset your password:\n%s", url)

		e.logger.Info("user not set password yet",
			"sent email to", email,
			"token", token,
			"url", url)
		e.PushCallFlow("SHD_RCP_192")
		ApiUtils.SendMail(e, email, subject, textBody, htmlBody, ApiUtils.EmailTypePasswordReset)
		e.PopCallFlow()

		msg := fmt.Sprintf("You have not set the password yet. An email has been sent to your email: %s. "+
			"Please check your email and click the link to set your password (SHD_EML_135)", email)
		return false, ApiTypes.CustomHttpStatus_PasswordNotSet, msg
	}

	// Hash password
	err := bcrypt.CompareHashAndPassword([]byte(user_info.Password), []byte(password))
	if err != nil {
		error_msg := fmt.Sprintf("invalid password, email:%s", email)
		logger.Error("invalid password", "email", email)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InvalidPassword,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_248"})

		return false, http.StatusUnauthorized, error_msg
	}

	logger.Info("varify user password success", "email", email)
	return true, 0, ""
}

func (e *echoContext) GetUserInfoByToken(token string) (*ApiTypes.UserInfo, bool) {
	if e.user_info != nil {
		return e.user_info, true
	}

	user_info, err := sysdatastores.GetUserInfoByToken(e, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No user found with that email
			e.logger.Error("No user found", "token", token)
			return user_info, false
		}

		// Real database error
		e.logger.Error("failed to get user by token", "error", err, "token", token)
		return nil, false
	}

	e.user_info = user_info
	return user_info, true
}

func (e *echoContext) GetUserInfoByEmail(email string) (*ApiTypes.UserInfo, bool) {
	if e.user_info != nil {
		return e.user_info, true
	}
	user_info, err := sysdatastores.GetUserInfoByEmail(e, email)
	if err != nil {
		// Possible reasons:
		// 1. user not found: user not found
		// 2. user pending verify:
		// 3. database error: error: xxx
		// 4. invalid user: (status not active)
		if errors.Is(err, sql.ErrNoRows) {
			// No user found with that email
			e.logger.Warn("No user found", "email", email)
			return nil, false
		}

		// Real database error
		e.logger.Error("failed get user by email", "error", err, "email", email)
		return nil, false
	}

	e.user_info = user_info
	return e.user_info, true
}

func (e *echoContext) GetUserInfoByUserID(user_id string) (*ApiTypes.UserInfo, bool) {
	if e.user_info != nil {
		return e.user_info, true
	}

	user_info, err := sysdatastores.GetUserInfoByUserID(e, user_id)
	if err != nil {
		// Possible reasons:
		// 1. user not found: user not found
		// 2. user pending verify:
		// 3. database error: error: xxx
		// 4. invalid user: (status not active)
		if errors.Is(err, sql.ErrNoRows) {
			// No user found with that email
			e.logger.Warn("No user found", "user_id", user_id)
			return nil, false
		}

		// Real database error
		e.logger.Error("failed get user by user id", "user_id", user_id, "error", err)
		return nil, false
	}

	e.user_info = user_info
	return e.user_info, true
}

func (e *echoContext) SaveSession(
	login_method string,
	session_id string,
	auth_token string,
	user_name string,
	user_name_type string,
	user_reg_id string,
	user_email string,
	expiry time.Time,
	need_update_user bool) error {
	return sysdatastores.SaveSession(e, login_method, session_id, auth_token,
		user_name, user_name_type, user_reg_id,
		user_email, expiry, need_update_user)
}

func (e *echoContext) MarkUserVerified(email string) error {
	return sysdatastores.MarkUserVerified(e, email)
}

func (e *echoContext) UpdateTokenByEmail(email string, token string) error {
	return databaseutil.UpdateVTokenByEmail(e, ApiTypes.DatabaseInfo.DBType,
		ApiTypes.LibConfig.SystemTableNames.TableNameUsers, email, token)
}

func (e *echoContext) UpsertUser(
	user_info *ApiTypes.UserInfo,
	plain_password string,
	verified bool,
	admin bool,
	is_owner bool,
	email_visibility bool,
	need_read bool) (*ApiTypes.UserInfo, error) {

	logger := e.logger
	logger.Trace("upsert user")
	var is_dirty bool = false
	if need_read {
		// Check if user exists
		user_info_found, found := e.GetUserInfoByEmail(user_info.Email)
		if !found {
			logger.Error("user not found", "email", user_info.Email)
			if plain_password != "" {
				hashedPwd, _ := bcrypt.GenerateFromPassword([]byte(plain_password), bcrypt.DefaultCost)
				user_info.Password = string(hashedPwd)
				is_dirty = true
			}
		} else {
			// Update existing user info
			if user_info.UserName == "" {
				user_info.UserName = user_info_found.UserName
			} else if user_info.UserName != user_info_found.UserName {
				is_dirty = true
			}

			// Immutable once set
			if user_info.UserIdType == "" {
				user_info.UserIdType = user_info_found.UserIdType
			}

			if user_info.FirstName == "" {
				user_info.FirstName = user_info_found.FirstName
			} else if user_info.FirstName != user_info_found.FirstName {
				is_dirty = true
			}

			if user_info.LastName == "" {
				user_info.LastName = user_info_found.LastName
			} else if user_info.LastName != user_info_found.LastName {
				is_dirty = true
			}

			if user_info.Email == "" {
				user_info.Email = user_info_found.Email
			} else if user_info.Email != user_info_found.Email {
				is_dirty = true
			}

			// Immutable once set
			if user_info.AuthType == "" {
				user_info.AuthType = user_info_found.AuthType
			}

			if user_info.UserStatus == "" {
				user_info.UserStatus = user_info_found.UserStatus
			} else if user_info.UserStatus != user_info_found.UserStatus {
				is_dirty = true
			}

			if plain_password != "" {
				hashedPwd, _ := bcrypt.GenerateFromPassword([]byte(plain_password), bcrypt.DefaultCost)
				user_info.Password = string(hashedPwd)
				is_dirty = true
			}

			if user_info.VToken == "" {
				user_info.VToken = user_info_found.VToken
			} else if user_info.VToken != user_info_found.VToken {
				is_dirty = true
			}

			if user_info.Avatar == "" {
				user_info.Avatar = user_info_found.Avatar
			} else if user_info.Avatar != user_info_found.Avatar {
				is_dirty = true
			}

			if user_info.UserMobile == "" {
				user_info.UserMobile = user_info_found.UserMobile
			} else if user_info.UserMobile != user_info_found.UserMobile {
				is_dirty = true
			}

			if user_info.UserAddress == "" {
				user_info.UserAddress = user_info_found.UserAddress
			} else if user_info.UserAddress != user_info_found.UserAddress {
				is_dirty = true
			}

			if user_info.Locale == "" {
				user_info.Locale = user_info_found.Locale
			} else if user_info.Locale != user_info_found.Locale {
				is_dirty = true
			}

			if user_info.OutlookAccessToken == "" {
				user_info.OutlookAccessToken = user_info_found.OutlookAccessToken
			} else if user_info.OutlookAccessToken != user_info_found.OutlookAccessToken {
				is_dirty = true
			}

			if user_info.OutlookRefreshToken == "" {
				user_info.OutlookRefreshToken = user_info_found.OutlookRefreshToken
			} else if user_info.OutlookRefreshToken != user_info_found.OutlookRefreshToken {
				is_dirty = true
			}

			if user_info.OutlookTokenExpiresAt.IsZero() {
				user_info.OutlookTokenExpiresAt = user_info_found.OutlookTokenExpiresAt
			} else if user_info.OutlookTokenExpiresAt != user_info_found.OutlookTokenExpiresAt {
				is_dirty = true
			}

			if user_info.OutlookSubExpiresAt.IsZero() {
				user_info.OutlookSubExpiresAt = user_info_found.OutlookSubExpiresAt
			} else if user_info.OutlookSubExpiresAt != user_info_found.OutlookSubExpiresAt {
				is_dirty = true
			}

			if user_info.OutlookSubID == "" {
				user_info.OutlookSubID = user_info_found.OutlookSubID
			} else if user_info.OutlookSubID != user_info_found.OutlookSubID {
				is_dirty = true
			}

			if user_info_found.Verified != verified {
				user_info.Verified = verified
			} else if user_info.Verified != user_info_found.Verified {
				is_dirty = true
			}

			if user_info_found.Admin != admin {
				user_info.Admin = admin
			} else if user_info.Admin != user_info_found.Admin {
				is_dirty = true
			}

			if user_info_found.IsOwner != is_owner {
				user_info.IsOwner = is_owner
			} else if user_info.IsOwner != user_info_found.IsOwner {
				is_dirty = true
			}

			if user_info_found.EmailVisibility != email_visibility {
				user_info.EmailVisibility = email_visibility
				is_dirty = true
			} else if user_info.EmailVisibility != user_info_found.EmailVisibility {
				is_dirty = true
			}
		}
	} else {
		is_dirty = true
	}

	if !is_dirty {
		e.logger.Info("No changes for user",
			"email", user_info.Email,
			"need_read", need_read)
		e.user_info = user_info
		return e.user_info, nil
	}

	err := sysdatastores.UpsertUser(e, user_info)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("Failed creating user, user_name:%s, email:%s, err:%s, log_id:%d",
			user_info.UserName, user_info.Email, err, log_id)
		e.logger.Error("failed creating user",
			"email", user_info.Email,
			"error", err)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_664"})

		return user_info, fmt.Errorf("%s", error_msg)
	}

	e.user_info = user_info
	return e.user_info, nil
}

func (e *echoContext) IsAuthenticated() *ApiTypes.UserInfo {
	if e.user_info != nil {
		return e.user_info
	}

	if e.user_checked {
		return nil
	}

	logger := e.logger
	if DefaultAuthenticator == nil {
		logger.Error("Default authenticator not set - auth middleware not initalized")
		return nil
	}

	// Note: DefaultAuthenticator(...) is a function pointer.
	// It is set to auth.go::IsAuthenticated(...) (due to circular importing)
	// 	- user_info not null: user logged in
	//	- user_info is null and err == nil: user not logged in
	//	- user_info is null and err != nil: error
	user_info, err := DefaultAuthenticator(e)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("auth failed, err:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_AuthFailure,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RHD_065"})

		e.logger.Error("auth failed", "error", err, "log_id", log_id)
		return nil
	}

	if user_info == nil {
		logger.Warn("user not logged in")
	}

	e.user_info = user_info
	e.user_checked = true
	return e.user_info
}

func (e *echoContext) SendHTMLResp(errorHTML string) error {
	e.c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	e.c.Response().WriteHeader(http.StatusBadRequest)
	_, err := e.c.Response().Write([]byte(errorHTML))
	return err
}

func (e *echoContext) Redirect(redirect_url string, status_code int) error {
	http.Redirect(e.c.Response(), e.c.Request(), redirect_url, status_code)
	return nil
}

func (e *echoContext) SendJSONResp(status_code int, json_resp map[string]interface{}) error {
	return e.c.JSON(status_code, json_resp)
}

func (e *echoContext) JSON(status_code int, json_resp map[string]interface{}) error {
	return e.c.JSON(status_code, json_resp)
}

func (e *echoContext) IsAuthed() bool {
	// Temporarily, return true
	return true
}

func (e *echoContext) GetCallFlow() string {
	if len(e.call_flow) <= 0 {
		return ""
	}

	// No call flow
	return strings.Join(e.call_flow, "->")
}

func (e *echoContext) PushCallFlow(loc string) string {
	e.call_flow = append(e.call_flow, loc)
	return strings.Join(e.call_flow, "->")
}

func (e *echoContext) PopCallFlow() string {
	if len(e.call_flow) <= 0 {
		return ""
	}
	e.call_flow = e.call_flow[:len(e.call_flow)-1]
	return strings.Join(e.call_flow, "->")
}
