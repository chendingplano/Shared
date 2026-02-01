package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

// SECURITY: Dummy hash for timing-safe comparison when user doesn't exist.
// This prevents timing attacks that could enumerate valid email addresses.
var dummyPasswordHash = func() []byte {
	hash, _ := bcrypt.GenerateFromPassword([]byte("dummy_password_for_timing_safety"), bcrypt.DefaultCost)
	return hash
}()

type User struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type EmailSignupResponse struct {
	Message string `json:"message"`
	LOC     string `json:"loc"`
}

type EmailSignupRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type EmailLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type EmailLoginResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type EmailVerifyRequest struct {
	Token string `json:"token"`
}

type VerifyResponse struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	RedirectURL string `json:"redirect_url"`
	Loc         string `json:"loc,omitempty"`
}

const (
	cookie_timeout_hours = 72
)

func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func HandleEmailLogin(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_EML_073")
	defer rc.Close()
	logger := rc.GetLogger()

	// SECURITY: Rate limiting to prevent brute-force attacks
	clientIP := c.RealIP()
	allowed, remaining, retryAfter := CheckLoginRateLimit(clientIP)
	if !allowed {
		logger.Warn("Rate limit exceeded for login",
			"ip", clientIP,
			"retry_after", retryAfter.String())
		return c.JSON(http.StatusTooManyRequests, map[string]string{
			"status":  "error",
			"message": "Too many login attempts. Please try again later.",
			"loc":     "SHD_EML_RATE_001",
		})
	}
	_ = remaining // Used for X-RateLimit-Remaining header if needed

	// SECURITY: Validate request origin to prevent CSRF attacks
	if !IsSafeOrigin(c) {
		logger.Warn("CSRF protection: rejected cross-origin request",
			"origin", c.Request().Header.Get("Origin"),
			"referer", c.Request().Header.Get("Referer"))
		return c.JSON(http.StatusForbidden, map[string]string{
			"status":  "error",
			"message": "Invalid request origin",
			"loc":     "SHD_EML_CSRF_001",
		})
	}

	body, _ := io.ReadAll(c.Request().Body)
	path := c.Path()
	logger.Info("Handle request", "path", path)
	status_code, msg := HandleEmailLoginBase(rc, body, clientIP)
	c.JSON(status_code, msg)
	return nil
}

// HandleEmailLoginBase processes email login requests.
// It returns (status_code, json).
//   - When success, json = {"status":"ok", "redirect_url": "...", "loc": "..."}.
//   - When failure, json = {"status":"error", "message": "...", "loc": "..."}.
//
// The clientIP parameter is used to reset rate limiting on successful login.
func HandleEmailLoginBase(
	rc ApiTypes.RequestContext,
	body []byte,
	clientIP string) (int, map[string]string) {
	logger := rc.GetLogger()
	logger.Info("HandleEmailLogin called")

	// SECURITY: Generic error message to prevent user enumeration
	// We return the same message whether email doesn't exist or password is wrong

	var req EmailLoginRequest
	if err := json.Unmarshal(body, &req); err != nil {
		error_msg := "invalid request body (SHD_EML_043)"

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_119"})

		logger.Error("invalid request body", "error", err)
		return http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": error_msg,
			"loc":     "SHD_EML_119",
		}
	}

	if !isValidEmail(req.Email) {
		error_msg := "invalid email format (SHD_EML_081)"
		logger.Error("invalid email format", "email", req.Email)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadEmail,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_136"})

		return http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": error_msg,
			"loc":     "SHD_EML_136",
		}
	}

	// SECURITY: Check per-account rate limiting to prevent distributed brute-force attacks.
	// This protects against attackers using multiple IPs to attack a single account.
	accountAllowed, _, accountRetryAfter := CheckAccountLockout(req.Email)
	if !accountAllowed {
		logger.Warn("Account locked due to too many failed attempts",
			"email", req.Email,
			"retry_after", accountRetryAfter.String())

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_AuthFailure,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  func() *string { s := fmt.Sprintf("Account locked due to rate limiting: %s", req.Email); return &s }(),
			CallerLoc:    "SHD_EML_ACCT_LOCK"})

		return http.StatusTooManyRequests, map[string]string{
			"status":  "error",
			"message": "This account is temporarily locked due to too many failed login attempts. Please try again later.",
			"loc":     "SHD_EML_ACCT_LOCK",
		}
	}

	user_info, exist := rc.GetUserInfoByEmail(req.Email)
	if !exist {
		// SECURITY: Perform dummy bcrypt comparison to prevent timing attacks.
		// This ensures response time is similar whether email exists or not,
		// preventing attackers from enumerating valid emails via timing analysis.
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))

		error_msg := "email not found, email"
		logger.Warn("login attempt for non-existent email", "email", req.Email)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_AuthFailure,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_131"})

		// Return generic error to prevent user enumeration
		return http.StatusUnauthorized, map[string]string{
			"status":  "error",
			"message": error_msg,
			"loc":     "SHD_EML_218",
		}
	}

	status, status_code, msg := rc.VerifyUserPassword(user_info, req.Password)
	if !status {
		if status_code == ApiTypes.CustomHttpStatus_PasswordNotSet {
			return status_code, map[string]string{
				"status":  "info",
				"message": msg,
				"loc":     "SHD_EML_147",
			}
		}

		// SECURITY: Return generic error for invalid password
		logger.Warn("login failed: invalid password", "email", req.Email)
		return http.StatusUnauthorized, map[string]string{
			"status":  "error",
			"message": "invalid password",
			"loc":     "SHD_EML_237",
		}
	}

	// SECURITY: Reset both IP and account rate limits on successful login
	if clientIP != "" {
		ResetLoginRateLimits(clientIP, req.Email)
	}

	// Generate Pocketbase auth token (similar to Google OAuth flow)
	auth_token, err := rc.GenerateAuthToken(req.Email)
	if err != nil {
		error_msg := fmt.Sprintf("failed to generate auth token: %v (SHD_EML_272)", err)
		logger.Error("failed generating auth token", "error", err, "email", req.Email)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_282"})

		return http.StatusInternalServerError, map[string]string{
			"status":  "error",
			"message": error_msg,
			"loc":     "SHD_EML_282",
		}
	}

	logger.Info("Generated token", "auth_token", ApiUtils.MaskToken(auth_token))

	// Generate a secure random session ID for logging purposes
	sessionID := ApiUtils.GenerateSecureToken(32)
	expired_time := time.Now().Add(cookie_timeout_hours * time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)

	// Save session in DB for audit logging
	err1 := rc.SaveSession(
		"email_login",
		sessionID,
		auth_token,
		req.Email,
		"email",
		req.Email,
		req.Email,
		expired_time,
		true)

	if err1 != nil {
		logger.Warn("failed saving session", "error", err1, "email", req.Email)
	}

	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:  "email_login",
		SessionID:    sessionID,
		AuthToken:    auth_token,
		Status:       "active",
		UserName:     req.Email,
		UserNameType: "email",
		UserRegID:    req.Email,
		UserEmail:    &req.Email,
		CallerLoc:    "SHD_EML_267",
		ExpiresAt:    &expired_time_str,
	})

	rc.SetCookie(sessionID)

	logger.Info("Email login success",
		"email", user_info.Email,
		"cookie set/session_id", ApiUtils.MaskToken(sessionID))

	// Construct redirect URL with Pocketbase auth token (like Google OAuth)
	user_name := user_info.FirstName + " " + user_info.LastName
	redirect_url := ApiUtils.GetOAuthRedirectURL(rc, auth_token, user_name)
	msg1 := fmt.Sprintf("email login success, email:%s, session_id:%s, redirect_url:%s",
		req.Email, ApiUtils.MaskToken(sessionID), redirect_url)
	logger.Info(
		"Email login success",
		"email", req.Email,
		"session_id", ApiUtils.MaskToken(sessionID),
		"redirect_url", redirect_url,
		"loc", "SHD_EML_316")

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_UserLoginSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg1,
		CallerLoc:    "SHD_EML_324"})

	msg = fmt.Sprintf("email login success, email:%s, redirectURL:%s, loc:(SHD_EML_190)",
		req.Email, redirect_url)
	return http.StatusOK, map[string]string{
		"status":       "ok",
		"redirect_url": redirect_url,
		"loc":          "SHD_EML_190",
	}
}

func sendVerificationEmail(
	rc ApiTypes.RequestContext,
	to string,
	url string) error {
	logger := rc.GetLogger()
	log_id := sysdatastores.NextActivityLogID()
	subject := "Verify your email address"
	htmlBody := fmt.Sprintf(`
        <p>Please click the link below to verify your email (logid:%d):</p>
        <p><a href="%s">%s</a></p>`, log_id, url, url)
	textBody := fmt.Sprintf("Please click the link below to verify your email (logid:%d):\n%s", log_id, url)

	msg := fmt.Sprintf("Sending verification email to %s with URL: %s, logid:%d", to, url, log_id)
	logger.Info(
		"Send verification email",
		"to", to,
		"url", url,
		"log_id", log_id)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		LogID:        log_id,
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_UserLoginSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_351"})

	logger.Info(
		"Send verification email",
		"to", to,
		"url", url,
		"testBody", textBody,
		"htmlBody", htmlBody)

	rc.PushCallFlow("SHD_EML_275")
	err := ApiUtils.SendMail(rc, to, subject, textBody, htmlBody, ApiUtils.EmailTypeVerification)
	rc.PopCallFlow()
	return err
}

func HandleEmailVerify(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_EML_272")
	logger := rc.GetLogger()
	logger.Info("Handle Email Verify (GET)")

	is_post := false
	HandleEmailVerifyCommon(rc, c, is_post)
	return nil
}

func HandleEmailVerifyPost(c echo.Context) error {
	// It supports two URL params:
	//	?token=<token>
	//	?type=<type>
	// Currently, it supports only type:'auth'
	rc := EchoFactory.NewFromEcho(c, "SHD_EML_272")
	logger := rc.GetLogger()
	logger.Info("Handle Email Verify (POST)")

	is_post := true
	HandleEmailVerifyCommon(rc, c, is_post)
	return nil
}

func HandleEmailVerifyCommon(
	rc ApiTypes.RequestContext,
	c echo.Context,
	is_post bool) {

	logger := rc.GetLogger()

	status_code, resp, err := HandleEmailVerifyBase(rc, is_post)
	if err == nil {
		// Verify success. Retrieve the query parm 'type'
		verifyType := c.QueryParam("type") // "auth"
		if verifyType == "auth" {
			// It needs to return the response
			c.JSON(status_code, resp)
			return
		}

		// Success case: redirect to the dashboard
		// Cookie was already set in HandleEmailVerifyBase
		redirectURL := resp["redirect_url"]
		if len(redirectURL) <= 0 {
			redirectURL = os.Getenv("APP_DOMAIN_NAME") + "/login"
			logger.Error("missing redirectURL",
				"status_code", status_code,
				"rediect_url", redirectURL,
				"is_post", is_post)
			c.Redirect(http.StatusSeeOther, redirectURL)
			return
		}

		logger.Info("email verify success, redirecting", "redirect_url", redirectURL)
		c.Redirect(http.StatusSeeOther, redirectURL)
		return
	}

	// Error case: return error message
	logger.Error("failed verify", "error", err)
	c.JSON(status_code, resp)
}

// HandleEmailVerifyBase handles email verification.
// Returns: (status_code, resp, error)
func HandleEmailVerifyBase(
	rc ApiTypes.RequestContext,
	is_post bool) (int, map[string]string, error) {
	logger := rc.GetLogger()
	logger.Info("Handle email verify request")

	var token string
	if is_post {
		// The request is a POST
		var req EmailVerifyRequest
		if err := rc.Bind(&req); err != nil {
			log_id := sysdatastores.NextActivityLogID()
			error_msg := fmt.Sprintf("Failed to bind request body: %v, log_id:%d", err, log_id)
			logger.Error("failed to bind request body", "log_id", log_id)

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				LogID:        log_id,
				ActivityName: ApiTypes.ActivityName_Auth,
				ActivityType: ApiTypes.ActivityType_BadRequest,
				AppName:      ApiTypes.AppName_Auth,
				ModuleName:   ApiTypes.ModuleName_EmailAuth,
				ActivityMsg:  &error_msg,
				CallerLoc:    "SHD_EML_369"})

			e_msg := fmt.Sprintf("invalid request body, log_id:%d (SHD_EML_365)", log_id)
			resp := map[string]string{
				"status":    "failed",
				"error_msg": e_msg,
				"loc":       "SHD_EML_369",
			}
			return http.StatusBadRequest, resp, fmt.Errorf("%s", e_msg)
		}
		token = req.Token
	} else {
		// The request is a GET
		token = rc.QueryParam("token")
		if token == "" {
			log_id := sysdatastores.NextActivityLogID()
			error_msg := fmt.Sprintf("Failed retrieving token, log_id:%d", log_id)
			logger.Error("failed retrieving token", "log_id", log_id)

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				LogID:        log_id,
				ActivityName: ApiTypes.ActivityName_Auth,
				ActivityType: ApiTypes.ActivityType_BadRequest,
				AppName:      ApiTypes.AppName_Auth,
				ModuleName:   ApiTypes.ModuleName_EmailAuth,
				ActivityMsg:  &error_msg,
				CallerLoc:    "SHD_EML_400"})

			e_msg := fmt.Sprintf("failed retrieving token, log_id:%d (SHD_EML_393)", log_id)
			resp := map[string]string{
				"status":    "failed",
				"error_msg": e_msg,
				"loc":       "SHD_EML_395",
			}
			return http.StatusBadRequest, resp, fmt.Errorf("%s", e_msg)
		}
	}

	logger.Info("Handle email verify",
		"token", ApiUtils.MaskToken(token),
		"db_type", ApiTypes.DatabaseInfo.DBType,
		"tablename", ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)

	// SECURITY: Validate token and check expiration
	user_info, exist := rc.GetUserInfoByToken(token)
	if !exist {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieving user by token:%s, log_id:%d", ApiUtils.MaskToken(token), log_id)
		logger.Error("failed retrieving user by token", "token", ApiUtils.MaskToken(token), "log_id", log_id)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InvalidToken,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_429"})

		e_msg := fmt.Sprintf("invalid or expired email verification, log_id:%d (SHD_EML_431)", log_id)
		resp := map[string]string{
			"status":    "failed",
			"error_msg": e_msg,
			"loc":       "SHD_EML_430",
		}
		return http.StatusBadRequest, resp, fmt.Errorf("%s", e_msg)
	}

	// SECURITY: Explicit token expiration check (defense in depth)
	if !user_info.VTokenExpiresAt.IsZero() && time.Now().After(user_info.VTokenExpiresAt) {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("email verification token expired, email:%s, log_id:%d", user_info.Email, log_id)
		logger.Warn("email verification token expired",
			"email", user_info.Email,
			"expired_at", user_info.VTokenExpiresAt,
			"log_id", log_id)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InvalidToken,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_TOKEN_EXP"})

		e_msg := fmt.Sprintf("email verification link has expired, log_id:%d (SHD_EML_TOKEN_EXP)", log_id)
		resp := map[string]string{
			"status":    "failed",
			"error_msg": e_msg,
			"loc":       "SHD_EML_TOKEN_EXP",
		}
		return http.StatusBadRequest, resp, fmt.Errorf("%s", e_msg)
	}

	// Mark user as verified first
	err1 := rc.MarkUserVerified(user_info.Email)
	if err1 != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("mark user failed, error:%v, user_name:%s, log_id:%d",
			err1, user_info.UserName, log_id)
		logger.Error("mark user failed",
			"error", err1,
			"email", user_info.Email,
			"log_id", log_id)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_468"})

		resp := map[string]string{
			"status":    "failed",
			"error_msg": error_msg,
			"loc":       "SHD_EML_458",
		}
		return http.StatusInternalServerError, resp, fmt.Errorf("%s", error_msg)
	}

	// Generate Pocketbase auth token (not session ID)
	authToken, err := rc.GenerateAuthToken(user_info.Email)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to generate auth token, err:%s, log_id:%d", err.Error(), log_id)
		logger.Error("failed generating auth token",
			"error", err,
			"email", user_info.Email,
			"log_id", log_id)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_438"})

		resp := map[string]string{
			"status":    "failed",
			"error_msg": error_msg,
			"loc":       "SHD_EML_485",
		}
		return http.StatusInternalServerError, resp, fmt.Errorf("%s", error_msg)
	}

	// Generate a secure random session ID for logging purposes
	sessionID := ApiUtils.GenerateSecureToken(32)
	expired_time := time.Now().Add(cookie_timeout_hours * time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)

	// Save session in DB for audit logging
	err1 = sysdatastores.SaveSession(
		rc,
		"email_verify",
		sessionID,
		authToken,
		user_info.UserName,
		"email",
		user_info.Email,
		user_info.Email,
		expired_time,
		true)

	if err1 != nil {
		logger.Error("failed saving session (non-critical)", "error", err1)
	}

	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:  "email_verify",
		SessionID:    sessionID,
		Status:       "active",
		UserName:     user_info.UserName,
		UserNameType: "email",
		UserRegID:    user_info.Email,
		UserEmail:    &user_info.Email,
		CallerLoc:    "SHD_EML_435",
		ExpiresAt:    &expired_time_str,
	})

	rc.SetCookie(sessionID)

	logger.Info("Email verification success",
		"email", user_info.Email,
		"cookie set/session_id", sessionID)

	msg1 := fmt.Sprintf("Set cookie, session_id:%s, HttpOnly:true", sessionID)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_SetCookie,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg1,
		CallerLoc:    "SHD_EML_462"})

	redirect_url := GetRedirectURL(rc, user_info.Email, user_info.Admin, false)
	msg := fmt.Sprintf("Email verify success: email:%s, session_id:%s, redirect:%s",
		user_info.Email, sessionID, redirect_url)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_VerifyEmailSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_505"})

	user_info_str, _ := json.Marshal(user_info)
	logger.Info("verify email success",
		"redirect_url", redirect_url,
		"cookie", sessionID,
		"is_admin", user_info.Admin,
		"email", user_info.Email)

	base_url := os.Getenv("APP_DOMAIN_NAME")
	user_name := user_info.FirstName + " " + user_info.LastName
	response := map[string]string{
		"name":         user_name,
		"email":        user_info.Email,
		"redirect_url": redirect_url,
		"user_info":    string(user_info_str),
		"base_url":     base_url,
		"auth_token":   authToken, // Return token to be set by frontend
		"loc":          "SHD_EML_210",
	}
	return http.StatusOK, response, nil
}

func HandleEmailSignup(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_EML_509")
	logger := rc.GetLogger()
	defer rc.Close()

	// SECURITY: Validate request origin to prevent CSRF attacks
	if !IsSafeOrigin(c) {
		logger.Warn("CSRF protection: rejected cross-origin request",
			"origin", c.Request().Header.Get("Origin"),
			"referer", c.Request().Header.Get("Referer"))
		return c.JSON(http.StatusForbidden, EmailSignupResponse{
			Message: "Invalid request origin",
			LOC:     "SHD_EML_CSRF_002",
		})
	}

	logger.Info("Handle email signup")
	ctx := rc.Context()
	call_flow := fmt.Sprintf("%s->SHD_EML_482", ctx.Value(ApiTypes.CallFlowKey))
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, call_flow)
	status_code, resp := HandleEmailSignupBase(new_ctx, rc)
	c.JSON(status_code, resp)
	return nil
}

func HandleEmailSignupBase(
	ctx context.Context,
	rc ApiTypes.RequestContext) (int, EmailSignupResponse) {
	// The request body:
	// {
	//   "first_name": "John",		// Optional
	//   "last_name": "Doe",		// Optional
	//   "email": "xxx",
	//   "password": "yyy"
	// }
	logger := rc.GetLogger()
	logger.Info("Handle email signup request")

	var req EmailSignupRequest
	if err := rc.Bind(&req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request body (SHD_EML_534), log_id:%d, err:%v", log_id, err)
		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_311",
		}

		logger.Error("invalid request body", "log_id", log_id, "error", err)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_548"})

		return http.StatusBadRequest, resp
	}

	logger.Info("Parsing request success")

	if !isValidEmail(req.Email) {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid email format, email:%s, log_id:%d (SHD_EML_547)", req.Email, log_id)
		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_547",
		}

		logger.Warn("invalid email format", "log_id", log_id, "email", req.Email)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InvalidEmail,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_548"})

		return http.StatusBadRequest, resp
	}

	// SECURITY: Validate password strength
	passwordResult := ValidatePasswordDefault(req.Password)
	if !passwordResult.Valid {
		log_id := sysdatastores.NextActivityLogID()
		// Combine all password errors into a user-friendly message
		errorDetails := "Password requirements not met"
		if len(passwordResult.Errors) > 0 {
			errorDetails = passwordResult.Errors[0]
			if len(passwordResult.Errors) > 1 {
				errorDetails += fmt.Sprintf(" (and %d more issues)", len(passwordResult.Errors)-1)
			}
		}
		error_msg := fmt.Sprintf("weak password, email:%s, log_id:%d (SHD_EML_614)", req.Email, log_id)
		resp := EmailSignupResponse{
			Message: errorDetails,
			LOC:     "SHD_EML_617",
		}

		logger.Warn("password validation failed",
			"log_id", log_id,
			"email", req.Email,
			"strength", passwordResult.Strength,
			"error_count", len(passwordResult.Errors))
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_WeakPassword,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_628"})

		return http.StatusBadRequest, resp
	}

	// 1. Check if email already exists
	// if databaseutil.UserExists(req.Email) {
	user_info, exist := rc.GetUserInfoByEmail(req.Email)
	if exist {
		if user_info.Verified {
			error_msg := fmt.Sprintf("email already exist (SHD_EML_588), email:%s", req.Email)
			logger.Warn("email already exists", "email", req.Email)
			resp := EmailSignupResponse{
				Message: error_msg,
				LOC:     "SHD_EML_588",
			}
			return http.StatusConflict, resp
		}
		logger.Info("Email signup: email exists but not verified", "email", req.Email)
	}

	// 3. Generate a verification token and Create a user record with "verified = false"
	token := uuid.NewString()
	var user_name = req.Email
	user_info = new(ApiTypes.UserInfo)
	user_info.UserIdType = "email"
	user_info.UserName = req.Email
	user_info.Email = req.Email
	user_info.AuthType = "email"
	user_info.UserStatus = "active"
	user_info.FirstName = req.FirstName
	user_info.LastName = req.LastName
	user_info.VToken = token
	_, err1 := rc.UpsertUser(user_info,
		req.Password, false, false, false, false, false)

	if err1 != nil {
		error_msg := fmt.Sprintf("failed creating user (SHD_EML_710), error:%v", err1)
		logger.Error("failed creating user account", "error", err1, "email", req.Email)

		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_715",
		}
		return http.StatusInternalServerError, resp
	}

	home_domain := os.Getenv("APP_DOMAIN_NAME")
	if home_domain == "" {
		logger.Error("missing APP_DOMAIN_NAME env var", "email", req.Email)
	}

	// 4. Send verification email
	verificationURL := fmt.Sprintf("%s/auth/email/verify?token=%s", home_domain, token)
	// SECURITY: Do not log full verification URLs or tokens - they allow account takeover
	logger.Info("sending verification email",
		"to", req.Email,
		"token", ApiUtils.MaskToken(token))

	rc.PushCallFlow("SHD_EML_642")
	go sendVerificationEmail(rc, req.Email, verificationURL)

	log_id := sysdatastores.NextActivityLogID()
	resp_msg := fmt.Sprintf("Signup successful! Please check your email:%s to verify your account, log_id:%d.",
		req.Email, log_id)
	resp := EmailSignupResponse{
		Message: resp_msg,
		LOC:     "SHD_EML_399",
	}

	msg := fmt.Sprintf("user signup success, user_name:%s, email:%s, token:%s",
		user_name, req.Email, ApiUtils.MaskToken(token))

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		LogID:        log_id,
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_SignupSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_689"})
	return http.StatusOK, resp
}

func HandleForgotPassword(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_EML_664")
	logger := rc.GetLogger()

	// SECURITY: Rate limiting to prevent abuse
	clientIP := c.RealIP()
	allowed, _, retryAfter := CheckPasswordResetRateLimit(clientIP)
	if !allowed {
		logger.Warn("Rate limit exceeded for password reset", "ip", clientIP)
		return c.JSON(http.StatusTooManyRequests, map[string]string{
			"status":  "error",
			"message": "Too many password reset attempts. Please try again later.",
			"loc":     "SHD_EML_RATE_002",
		})
	}

	// SECURITY: Validate request origin to prevent CSRF attacks
	if !IsSafeOrigin(c) {
		logger.Warn("CSRF protection: rejected cross-origin request",
			"origin", c.Request().Header.Get("Origin"),
			"referer", c.Request().Header.Get("Referer"))
		return c.JSON(http.StatusForbidden, map[string]string{
			"status":  "error",
			"message": "Invalid request origin",
			"loc":     "SHD_EML_CSRF_003",
		})
	}
	_ = retryAfter // Used for Retry-After header if needed

	reqID := rc.ReqID()
	status_code, resp := HandleForgotPasswordBase(rc, reqID)
	c.JSON(status_code, resp)
	return nil
}

/*
func HandleForgotPasswordPocket(p *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(p)
	reqID := rc.ReqID()
	status_code, resp := HandleForgotPasswordBase(rc, reqID)
	p.JSON(status_code, resp)
	return nil
}
*/

func HandleForgotPasswordBase(
	rc ApiTypes.RequestContext,
	reqID string) (int, map[string]string) {
	// Note that this function will report alarms if errors occur. The caller will
	// not generate logs.
	logger := rc.GetLogger()

	logger.Info("Handle forgot password request")
	// The request body:
	// {
	//   "email": "xxx",
	// }
	// Response with JSON:
	//   - When success: {"status":"ok", "message": "...", "loc": "..."}.
	//   - When failure: {"status":"error", "error": "...", "loc": "..."}.
	var req struct {
		Email string `json:"email"`
	}
	if err := rc.Bind(&req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request, log_id:%d, error:%v (SHD_EML_702)",
			log_id, err)
		logger.Error("invalid request", "error", err, "logid", log_id)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_712"})

		return http.StatusBadRequest, map[string]string{
			"status": "error",
			"error":  error_msg,
			"loc":    "SHD_EML_663"}
	}

	if !isValidEmail(req.Email) {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid email:%s, log_id:%d (SHD_EML_653)", req.Email, log_id)
		logger.Error("invalid email", "email", req.Email, "logid", log_id)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InvalidEmail,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_720"})

		return http.StatusBadRequest, map[string]string{
			"status": "error",
			"error":  error_msg,
			"loc":    "SHD_EML_683",
		}
	}

	// SECURITY: Always return the same response to prevent user enumeration.
	// Log internally whether user exists, but don't reveal this to the client.
	successMsg := "If an account exists with this email, a password reset link has been sent."

	user, exist := rc.GetUserInfoByEmail(req.Email)
	if !exist {
		// SECURITY: Log internally but return success to prevent enumeration
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("password reset requested for non-existent email:%s, log_id:%d",
			req.Email, log_id)
		logger.Warn("reset password, but user not found", "email", req.Email)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UserNotFound,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_731"})

		// Return success to prevent user enumeration
		return http.StatusOK, map[string]string{
			"status":  "ok",
			"message": successMsg,
			"loc":     "SHD_EML_027",
		}
	}

	token := uuid.NewString()
	rc.UpdateTokenByEmail(req.Email, token)

	home_domain := os.Getenv("APP_DOMAIN_NAME")
	if home_domain == "" {
		logger.Error("APP_DOMAIN_NAME not set")
		return http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "server error (env var not set)",
			"loc":     "SHD_EML_040",
		}
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", home_domain, token)
	htmlBody := fmt.Sprintf(`
        <p>Hi %s,</p>
        <p>Click the link below to reset your password:</p>
        <p><a href="%s">%s</a></p>
    `, user.UserName, resetURL, resetURL)
	textBody := fmt.Sprintf("Hi %s,\n\nClick the link below to reset your password:\n%s", user.UserName, resetURL)
	rc.PushCallFlow("SHD_EML_786")
	go ApiUtils.SendMail(rc, req.Email, "Password Reset", textBody, htmlBody, ApiUtils.EmailTypePasswordReset)

	log_id := sysdatastores.NextActivityLogID()
	msg := fmt.Sprintf("reset link sent to email:%s", req.Email)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		LogID:        log_id,
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_SentEmail,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_775"})

	return http.StatusOK, map[string]string{
		"status":  "ok",
		"message": successMsg,
		"loc":     "SHD_EML_068",
	}
}

func HandleResetLink(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_EML_796")
	reqID := rc.ReqID()
	status_code, msg := HandleResetLinkBase(rc, reqID)
	c.String(status_code, msg)
	return nil
}

/*
func HandleResetLinkPocket(e *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()
	status_code, msg := HandleResetLinkBase(rc, reqID)
	e.String(status_code, msg)
	return nil
}
*/

func HandleResetLinkBase(
	rc ApiTypes.RequestContext,
	reqID string) (int, string) {
	token := rc.QueryParam("token")
	log.Printf("[req=%s] Handle reset link (SHD_EML_257), token:%s", reqID, ApiUtils.MaskToken(token))
	_, exist := rc.GetUserInfoByToken(token)
	if !exist {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieving user by token:%s (SHD_EML_201).", ApiUtils.MaskToken(token))
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UserNotFound,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_795"})

		return http.StatusBadRequest, "Invalid or expired reset link (SHD_EML_201)."
	}

	// Redirect to frontend reset form
	home_domain := os.Getenv("APP_DOMAIN_NAME")
	if home_domain == "" {
		error_msg := "missing APP_DOMAIN_NAME env var (SHD_EML_808)"
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)
	}
	redirect_url := fmt.Sprintf("%s/reset-password?token=%s", home_domain, token)
	msg := fmt.Sprintf("handle reset, redirect to:%s", redirect_url)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_Redirect,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_808"})
	return http.StatusSeeOther, redirect_url
}

type ResetConfirmRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func HandleResetPasswordConfirm(c echo.Context) error {
	log.Printf("Handle reset password confirm (SHD_EML_820)")
	rc := EchoFactory.NewFromEcho(c, "SHD_EML_859")
	logger := rc.GetLogger()

	// SECURITY: Validate request origin to prevent CSRF attacks
	if !IsSafeOrigin(c) {
		logger.Warn("CSRF protection: rejected cross-origin request",
			"origin", c.Request().Header.Get("Origin"),
			"referer", c.Request().Header.Get("Referer"))
		return c.String(http.StatusForbidden, "Invalid request origin")
	}

	ctx := c.Request().Context()
	call_flow := fmt.Sprintf("%s->SHD_EML_824", ctx.Value(ApiTypes.CallFlowKey))
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, call_flow)
	new_req := c.Request().WithContext(new_ctx)
	c.SetRequest(new_req)

	status_code, msg := HandleResetPasswordConfirmBase(new_ctx, rc)
	c.String(status_code, msg)
	return nil
}

/*
func HandleResetPasswordConfirmPocket(e *core.RequestEvent) error {
	log.Printf("Handle Reset Password Confirm (SHD_EML_827)")
	rc := RequestHandlers.NewFromPocket(e)
	ctx := rc.Context()
	call_flow := fmt.Sprintf("%s->SHD_EML_838", ctx.Value(ApiTypes.CallFlowKey))
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, call_flow)

	// Note that Pocketbase does not offer a way to replace requests
	// in 'e'. We generally do not need to create a new request, anyway.
	// In all function calls, always pass 'ctx' instead of letting functions
	// retrieve ctx (context.Context) from request.
	status_code, msg := HandleResetPasswordConfirmBase(new_ctx, rc)
	e.String(status_code, msg)
	return nil
}
*/

func HandleResetPasswordConfirmBase(
	ctx context.Context,
	rc ApiTypes.RequestContext) (int, string) {

	// The frontend (routes/reset-password/+page.svelte)
	// fetches (POST) this endpoint with Token and Password.
	// It retrieves the Token and Password.
	/*
		user_name, ok := rc.Context().Value(ApiTypes.UserContextKey).(string)
		if !ok {
			log_id := sysdatastores.NextActivityLogID()
			error_msg := fmt.Sprintf("internal error (SHD_EML_693), log_id:%d", log_id)
			log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				LogID:        log_id,
				ActivityName: ApiTypes.ActivityName_Auth,
				ActivityType: ApiTypes.ActivityType_InternalError,
				AppName:      ApiTypes.AppName_Auth,
				ModuleName:   ApiTypes.ModuleName_EmailAuth,
				ActivityMsg:  &error_msg,
				CallerLoc:    "SHD_EML_703"})

			return http.StatusBadRequest, error_msg
		}
	*/

	reqID := rc.ReqID()
	var req ResetConfirmRequest
	if err := json.NewDecoder(rc.GetBody()).Decode(&req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request payload, log_id:%d (SHD_EML_841)", log_id)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_701"})

		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		return http.StatusBadRequest, error_msg
	}

	// SECURITY: Validate password strength before processing
	passwordResult := ValidatePasswordDefault(req.Password)
	if !passwordResult.Valid {
		log_id := sysdatastores.NextActivityLogID()
		errorDetails := "Password requirements not met"
		if len(passwordResult.Errors) > 0 {
			errorDetails = passwordResult.Errors[0]
		}
		error_msg := fmt.Sprintf("weak password in reset, log_id:%d (SHD_EML_PWD)", log_id)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_WeakPassword,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_PWD_001"})

		log.Printf("[req=%s] Password validation failed: %s", reqID, errorDetails)
		return http.StatusBadRequest, errorDetails
	}

	// Validate token and get user
	user_info, exist := rc.GetUserInfoByToken(req.Token)
	if !exist {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("user not found, token:%s, log_id:%d", ApiUtils.MaskToken(req.Token), log_id)
		log.Printf("[req=%s] ***** (SHD_EML_862) Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UserNotFound,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_871"})

		e_msg := fmt.Sprintf("user not found, log_id:%d (SHD_EML_704)", log_id)
		return http.StatusBadRequest, e_msg
	}

	// Hash new password
	status, status_code, msg := rc.UpdatePassword(user_info.Email, req.Password)
	if !status {
		return status_code, msg
	}

	// SECURITY: Clear the reset token to prevent reuse
	if err := sysdatastores.ClearVTokenByEmail(rc, user_info.Email); err != nil {
		log.Printf("[req=%s] Warning: failed to clear reset token: %v", reqID, err)
		// Continue anyway - password was successfully updated
	}

	log.Printf("[req=%s] Update password success (SHD_EML_259), email:%s", reqID, user_info.Email)

	log_id := sysdatastores.NextActivityLogID()
	msg = fmt.Sprintf("reset password success, user_name:%s, log_id:%d", user_info.UserName, log_id)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_InternalError,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_779"})
	return http.StatusOK, "Password has been reset successfully (SHD_EML_263)."
}
