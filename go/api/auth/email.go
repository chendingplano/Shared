package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/RequestHandlers"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/pocketbase/core"
)

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
	cookie_timeout_hours = 24
)

func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func HandleEmailLogin(c echo.Context) error {
	rc := RequestHandlers.NewFromEcho(c)
	reqID := rc.ReqID()
	body, _ := io.ReadAll(c.Request().Body)
	log.Printf("[req=%s] Handle Email Login called (SHD_EML_075)", reqID)
	status_code, msg := HandleEmailLoginBase(rc, reqID, body)
	c.JSON(status_code, msg)
	return nil
}

func HandleEmailLoginPocket(e *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()
	body, _ := io.ReadAll(rc.GetBody())
	log.Printf("[req=%s] Handle Email Login called (SHD_EML_085)", reqID)
	status_code, msg := HandleEmailLoginBase(rc, reqID, body)
	e.JSON(status_code, msg)
	return nil
}

// HandleEmailLoginBase processes email login requests.
// It returns (status_code, json).
//   - When success, json = {"status":"ok", "redirect_url": "...", "loc": "..."}.
//   - When failure, json = {"status":"error", "message": "...", "loc": "..."}.
func HandleEmailLoginBase(
	rc RequestHandlers.RequestContext,
	reqID string,
	body []byte) (int, map[string]string) {
	log.Printf("[req=%s] HandleEmailLogin called (SHD_EML_095)", reqID)
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

		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)
		return http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": error_msg,
			"loc":     "SHD_EML_119",
		}
	}

	if !isValidEmail(req.Email) {
		error_msg := "invalid email format (SHD_EML_081)"
		log.Printf("[req=%s] +++++ Warning:%s, email:%s", reqID, error_msg, req.Email)

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

	user_info, exist := rc.GetUserInfoByEmail(reqID, req.Email)
	if !exist {
		// The user (email) already exists.
		return http.StatusNotFound, map[string]string{
			"status":  "error",
			"message": "email not found (SHD_EML_131)",
			"loc":     "SHD_EML_131",
		}
	}

	status, status_code, msg := rc.VerifyUserPassword(reqID, req.Email, req.Password)
	if !status {
		return status_code, map[string]string{
			"status":  "error",
			"message": msg,
			"loc":     "SHD_EML_150",
		}
	}

	// Generate Pocketbase auth token (similar to Google OAuth flow)
	token, err := rc.GenerateAuthToken(reqID, req.Email)
	if err != nil {
		error_msg := fmt.Sprintf("failed to generate auth token: %v (SHD_EML_272)", err)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

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

	// Generate a secure random session ID for logging purposes
	sessionID := ApiUtils.GenerateSecureToken(32)
	expired_time := time.Now().Add(cookie_timeout_hours * time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)

	// Save session in DB for audit logging
	err1 := rc.SaveSession(
		reqID,
		"email_login",
		sessionID,
		req.Email,
		"email",
		req.Email,
		req.Email,
		expired_time)

	if err1 != nil {
		log.Printf("[req=%s] +++++ Warning: failed saving session (non-critical): %v (SHD_EML_285)", reqID, err1)
	}

	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:  "email_login",
		SessionID:    sessionID,
		Status:       "active",
		UserName:     req.Email,
		UserNameType: "email",
		UserRegID:    req.Email,
		UserEmail:    &req.Email,
		CallerLoc:    "SHD_EML_267",
		ExpiresAt:    &expired_time_str,
	})

	// Construct redirect URL with Pocketbase auth token (like Google OAuth)
	user_name := user_info.FirstName + " " + user_info.LastName
	redirect_url := ApiUtils.GetOAuthRedirectURL(reqID, token, user_name)
	msg1 := fmt.Sprintf("email login success, email:%s, session_id:%s, redirect_url:%s",
		req.Email, sessionID, redirect_url)
	log.Printf("[req=%s] %s (SHD_EML_316)", reqID, msg1)

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

func sendVerificationEmail(reqID string, to string, url string) error {
	log_id := sysdatastores.NextActivityLogID()
	subject := "Verify your email address"
	body := fmt.Sprintf(`
        <p>Please click the link below to verify your email (logid:%d):</p>
        <p><a href="%s">%s</a></p>`, log_id, url, url)

	msg := fmt.Sprintf("Sending verification email to %s with URL: %s, logid:%d", to, url, log_id)
	log.Printf("[req=%s] %s (SHD_EML_345)", reqID, msg)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		LogID:        log_id,
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_UserLoginSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_351"})

	return ApiUtils.SendMail(reqID, to, subject, body, "SHD_EML_284") // implement this using smtp.SendMail or a library
}

func HandleEmailVerify(c echo.Context) error {
	rc := RequestHandlers.NewFromEcho(c)
	reqID := rc.ReqID()

	status_code, msg, resp := HandleEmailVerifyBase(rc, reqID)
	if msg == "" {
		c.JSON(status_code, resp)
	} else {
		c.String(status_code, msg)
	}
	return nil
}

func HandleEmailVerifyPocket(e *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()

	status_code, msg, resp := HandleEmailVerifyBase(rc, reqID)
	if msg == "" {
		// Success case: redirect to frontend callback with token in URL
		// This matches the OAuth flow pattern
		if authToken, ok := resp["auth_token"]; ok {
			// Get the frontend URL from redirect_url (which contains the full domain)
			frontendDomain := GetRedirectURL(reqID, "", false, true) // domain_name_only = true
			callbackURL := fmt.Sprintf("%s/auth/email/verify/callback?token=%s", frontendDomain, authToken)
			if name, ok := resp["name"]; ok && name != "" {
				callbackURL += fmt.Sprintf("&name=%s", name)
			}
			log.Printf("[req=%s] Redirecting to callback (SHD_EML_289): %s", reqID, callbackURL)
			e.Redirect(http.StatusSeeOther, callbackURL)
		} else {
			// Fallback: if no auth_token, return JSON
			e.JSON(status_code, resp)
		}
	} else {
		// Error case: return error message
		e.String(status_code, msg)
	}
	return nil
}

func HandleEmailVerifyBase(
	rc RequestHandlers.RequestContext,
	reqID string) (int, string, map[string]string) {
	log.Printf("[req=%s] Handle email verify request (SHD_EML_192)", reqID)

	/* The following code is for POST.
	var req EmailVerifyRequest
	if err := rc.Bind(reqID, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("Failed to bind request body: %v, log_id:%d, user_name:%s",
			err, log_id, user_name)
		log.Printf("[req=%s] %s (SHD_EML_361)", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_369"})

		e_msg := fmt.Sprintf("invalid request body, log_id:%d, user_name:%s (SHD_EML_361)", log_id, user_name)
		return http.StatusBadRequest, e_msg, nil
	}

	token := req.Token
	*/
	// We assume the link uses GET!!!
	token := rc.QueryParam("token")
	if token == "" {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("Failed retrieving token, log_id:%d", log_id)
		log.Printf("[req=%s] ***** Alarm:%s (SHD_EML_205)", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_400"})

		e_msg := fmt.Sprintf("failed retrieving token, log_id:%d (SHD_EML_393)", log_id)
		return http.StatusBadRequest, e_msg, nil
	}

	log.Printf("[req=%s] Handle email verify (SHD_EML_193), token:%s, dbtype:%s, tablename:%s",
		reqID, token,
		ApiTypes.DatabaseInfo.DBType,
		ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)

	// TODO (Chen Ding, 2025/11/03)
	// Add timeout or rate-limiting to prevent abuse of this endpoint.
	// Validate token format early (e.g., check length, character set).
	// Use HTTPS in production — tokens in request bodies are still sensitive.

	user_info, exist := rc.GetUserInfoByToken(reqID, token)
	if !exist {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieving user by token:%s, log_id:%d", token, log_id)
		log.Printf("[req=%s] ***** Alarm:%s (SHD_EML_420)", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InvalidToken,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_429"})

		e_msg := fmt.Sprintf("invalid or expired email verification, log_id:%d (SHD_EML_431)", log_id)
		return http.StatusBadRequest, e_msg, nil
	}

	// Mark user as verified first
	err1 := rc.MarkUserVerified(reqID, user_info.Email)
	if err1 != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("mark user failed, error:%v, user_name:%s, log_id:%d",
			err1, user_info.UserName, log_id)
		log.Printf("[req=%s] ***** Alarm:%s (SHD_EML_460)", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_468"})
		return http.StatusInternalServerError, error_msg, nil
	}

	// Generate Pocketbase auth token (not session ID)
	authToken, err := rc.GenerateAuthToken(reqID, user_info.Email)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to generate auth token, err:%s, log_id:%d", err.Error(), log_id)
		log.Printf("[req=%s] ***** Alarm:%s (SHD_EML_429)", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_438"})
		return http.StatusInternalServerError, error_msg, nil
	}

	// Generate a secure random session ID for logging purposes
	sessionID := ApiUtils.GenerateSecureToken(32)
	expired_time := time.Now().Add(cookie_timeout_hours * time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)

	// Save session in DB for audit logging
	err1 = sysdatastores.SaveSession(
		"email_verify",
		sessionID,
		user_info.UserName,
		"email",
		user_info.Email,
		user_info.Email,
		expired_time)

	if err1 != nil {
		log.Printf("[req=%s] +++++ Warning: failed saving session (non-critical): %v (SHD_EML_285)", reqID, err1)
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

	log.Printf("[req=%s] Email verification success (SHD_EML_253): email:%s, session_id:%s", reqID, user_info.Email, sessionID)

	// Don't set cookie here - frontend will handle it via pb.authStore.save()
	// This matches the OAuth flow pattern

	redirect_url := GetRedirectURL(reqID, user_info.Email, user_info.Admin, false)
	msg := fmt.Sprintf("Email verify success: email:%s, session_id:%s, redirect:%s",
		user_info.Email, sessionID, redirect_url)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_VerifyEmailSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_505"})

	user_name := user_info.FirstName + " " + user_info.LastName
	response := map[string]string{
		"name":         user_name,
		"email":        user_info.Email,
		"redirect_url": redirect_url,
		"auth_token":   authToken, // Return token to be set by frontend
		"loc":          "SHD_EML_210",
	}
	return http.StatusOK, "", response
	// w := c.Response()
	// w.Header().Set("Content-Type", "application/json")
	// w.WriteHeader(http.StatusOK)
	// json.NewEncoder(w).Encode(response)
	// return nil
}

func HandleEmailSignup(c echo.Context) error {
	rc := RequestHandlers.NewFromEcho(c)
	reqID := rc.ReqID()
	status_code, resp := HandleEmailSignupBase(rc, reqID)
	c.JSON(status_code, resp)
	return nil
}

func HandleEmailSignupPocket(e *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()
	status_code, resp := HandleEmailSignupBase(rc, reqID)
	e.JSON(status_code, resp)
	return nil
}

func HandleEmailSignupBase(
	rc RequestHandlers.RequestContext,
	reqID string) (int, EmailSignupResponse) {
	// The request body:
	// {
	//   "first_name": "John",		// Optional
	//   "last_name": "Doe",		// Optional
	//   "email": "xxx",
	//   "password": "yyy"
	// }
	log.Printf("[req=%s] Handle email signup request (SHD_EML_300)", reqID)

	var req EmailSignupRequest
	if err := rc.Bind(reqID, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request body (SHD_EML_534), log_id:%d, err:%v", log_id, err)
		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_311",
		}

		log.Printf("[req=%s] ***** Alarm Handle Email Signup failed:%s", reqID, resp.Message)
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

	log.Printf("[req=%s] Parsing request success (SHD_EML_627), req:%v, password:%s", reqID, req, req.Password)

	if !isValidEmail(req.Email) {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid email format, email:%s, log_id:%d (SHD_EML_547)", req.Email, log_id)
		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_547",
		}

		log.Printf("[req=%s] +++++ Warning:%s", reqID, error_msg)
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

	// 1. Check if email already exists
	// if databaseutil.UserExists(req.Email) {
	user_info, exist := rc.GetUserInfoByEmail(reqID, req.Email)
	if exist {
		if user_info.Verified {
			error_msg := fmt.Sprintf("email already exist (SHD_EML_588), email:%s", req.Email)
			log.Printf("[req=%s] %s", reqID, error_msg)
			resp := EmailSignupResponse{
				Message: error_msg,
				LOC:     "SHD_EML_588",
			}
			return http.StatusConflict, resp
		}
		slog.Info("[req=%s] Email signup: email exists but not verified, email:%s (SHD_EML_613)", reqID, req.Email)
	}

	// 3. Generate a verification token and Create a user record with "verified = false"
	token := uuid.NewString()
	var user_name = req.Email
	_, err1 := rc.UpsertUser(reqID,
		"email", user_name, req.Password, req.Email, "email", "pending_verify",
		req.FirstName, req.LastName, token, "")

	if err1 != nil {
		error_msg := fmt.Sprintf("failed creating user (SHD_EML_710), error:%v", err1)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_715",
		}
		return http.StatusInternalServerError, resp
	}

	home_domain := os.Getenv("APP_DOMAIN_NAME")
	if home_domain == "" {
		error_msg := "missing APP_DOMAIN_NAME env var (SHD_EML_808)"
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)
	}

	// 4. Send verification email
	verificationURL := fmt.Sprintf("%s/auth/email/verify?token=%s", home_domain, token)
	log.Printf("[req=%s] Sending verification email to:%s, verificationURL:%s (SHD_EML_777)", reqID, req.Email, verificationURL)
	go sendVerificationEmail(reqID, req.Email, verificationURL)

	log_id := sysdatastores.NextActivityLogID()
	resp_msg := fmt.Sprintf("Signup successful! Please check your email:%s to verify your account, log_id:%d.",
		req.Email, log_id)
	log.Printf("[req=%s] %s (SHD_EML_399)", reqID, resp_msg)
	resp := EmailSignupResponse{
		Message: resp_msg,
		LOC:     "SHD_EML_399",
	}

	msg := fmt.Sprintf("user signup success, user_name:%s, email:%s, token:%s",
		user_name, req.Email, token)

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

/*
reqID := rc.ReqID()
body, _ := io.ReadAll(c.Request().Body)
log.Printf("[req=%s] Handle Email Login called (SHD_EML_075)", reqID)
status_code, msg := HandleEmailLoginBase(rc, reqID, body)
c.JSON(status_code, msg)
return nil
*/
func HandleForgotPassword(c echo.Context) error {
	rc := RequestHandlers.NewFromEcho(c)
	reqID := rc.ReqID()
	status_code, resp := HandleForgotPasswordBase(rc, reqID)
	c.JSON(status_code, resp)
	return nil
}

func HandleForgotPasswordPocket(p *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(p)
	reqID := rc.ReqID()
	status_code, resp := HandleForgotPasswordBase(rc, reqID)
	p.JSON(status_code, resp)
	return nil
}

func HandleForgotPasswordBase(
	rc RequestHandlers.RequestContext,
	reqID string) (int, map[string]string) {
	log.Printf("[req=%s] Handle forgot password request (SHD_EML_650)", reqID)
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
	if err := rc.Bind(reqID, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieve email, log_id:%d, error:%v (SHD_EML_702)",
			log_id, err)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

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
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

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

	// 1. Check if email already exists
	user, exist := rc.GetUserInfoByEmail(reqID, req.Email)
	if !exist {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("user not found, email:%s, log_id:%d (SHD_EML_676)",
			req.Email, log_id)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UserNotFound,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_731"})

		return http.StatusNotFound, map[string]string{
			"status": "error",
			"error":  error_msg,
			"loc":    "SHD_EML_710",
		}
	}

	token := uuid.NewString()
	rc.UpdateTokenByEmail(reqID, req.Email, token)

	home_domain := os.Getenv("APP_DOMAIN_NAME")
	if home_domain == "" {
		error_msg := "missing APP_DOMAIN_NAME env var (SHD_EML_808)"
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", home_domain, token)
	go ApiUtils.SendMail(reqID, req.Email, "Password Reset", fmt.Sprintf(`
        <p>Hi %s,</p>
        <p>Click the link below to reset your password:</p>
        <p><a href="%s">%s</a></p>
    `, user.UserName, resetURL, resetURL), "SHD_EML_732")

	log_id := sysdatastores.NextActivityLogID()
	msg := fmt.Sprintf("reset link sent to email:%s", req.Email)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		LogID:        log_id,
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_UserNotFound,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_775"})

	return http.StatusOK, map[string]string{
		"message": msg,
		"loc":     "SHD_EML_742",
		"status":  "ok",
	}
}

func HandleResetLink(c echo.Context) error {
	rc := RequestHandlers.NewFromEcho(c)
	reqID := rc.ReqID()
	status_code, msg := HandleResetLinkBase(rc, reqID)
	c.String(status_code, msg)
	return nil
}

func HandleResetLinkPocket(e *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()
	status_code, msg := HandleResetLinkBase(rc, reqID)
	e.String(status_code, msg)
	return nil
}

func HandleResetLinkBase(
	rc RequestHandlers.RequestContext,
	reqID string) (int, string) {
	token := rc.QueryParam("token")
	log.Printf("[req=%s] Handle reset link (SHD_EML_257), token:%s", reqID, token)
	_, exist := rc.GetUserInfoByToken(reqID, token)
	if !exist {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieving user by token:%s (SHD_EML_201).", token)
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
	rc := RequestHandlers.NewFromEcho(c)
	reqID := rc.ReqID()
	status_code, msg := HandleResetPasswordConfirmBase(rc, reqID)
	c.String(status_code, msg)
	return nil
}

func HandleResetPasswordConfirmPocket(e *core.RequestEvent) error {
	log.Printf("Handle Reset Password Confirm (SHD_EML_827)")
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()
	status_code, msg := HandleResetPasswordConfirmBase(rc, reqID)
	e.String(status_code, msg)
	return nil
}

func HandleResetPasswordConfirmBase(
	rc RequestHandlers.RequestContext,
	reqID string) (int, string) {

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

	// Validate token and get user
	user_info, exist := rc.GetUserInfoByToken(reqID, req.Token)
	if !exist {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("user not found, token:%s, log_id:%d", req.Token, log_id)
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
	status, status_code, msg := rc.UpdatePassword(reqID, user_info.Email, req.Password)
	if !status {
		return status_code, msg
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
