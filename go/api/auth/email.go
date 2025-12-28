package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/RequestHandlers"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/bcrypt"
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
	status_code, msg := HandleEmailLoginBase(rc, reqID, body)
	c.String(status_code, msg)
	return nil
}

func HandleEmailLoginPocket(e *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()
	body, _ := io.ReadAll(rc.GetBody())
	status_code, msg := HandleEmailLoginBase(rc, reqID, body)
	e.String(status_code, msg)
	return nil
}

func HandleEmailLoginBase(
	rc RequestHandlers.RequestContext,
	reqID string,
	body []byte) (int, string) {
	log.Printf("[req=%s] HandleEmailLogin called (SHD_EML_076)", reqID)
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
		return http.StatusBadRequest, error_msg
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

		return http.StatusBadRequest, error_msg
	}

	user_info, exist := rc.GetUserInfoByEmail(reqID, req.Email)
	if !exist {
		// The user (email) already exists.
		return http.StatusNotFound, "email not found (SHD_EML_131)"
	}

	if user_info.Password == "" {
		token := uuid.NewString()
		url := fmt.Sprintf("http://localhost:5173/auth/email/reset?token=%s", token)
		error_msg := fmt.Sprintf("user not set password yet, sent reset password email to:%s, email:%s, token:%s",
			req.Email, token, url)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_SentEmail,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_216"})

		databaseutil.UpdateVTokenByEmail(ApiTypes.DatabaseInfo.DBType,
			ApiTypes.LibConfig.SystemTableNames.TableNameUsers, req.Email, token)

		subject := "Reset your password"
		body := fmt.Sprintf(`
        	<p>Please click the link below to reset your password:</p>
        	<p><a href="%s">%s</a></p>`, url, url)

		log.Printf("[req=%s] %s (SHD_EML_227)", reqID, error_msg)
		ApiUtils.SendMail(reqID, req.Email, subject, body, "SHD_EML_154")

		msg := fmt.Sprintf("You have not set the password yet. An email has been sent to your email:%s. "+
			"Please check your email and click the link to set your password (SHD_EML_135)", req.Email)
		return http.StatusUnauthorized, msg
	}

	// Hash password
	err := bcrypt.CompareHashAndPassword([]byte(user_info.Password), []byte(req.Password))
	if err != nil {
		error_msg := fmt.Sprintf("invalid password, email:%s", req.Email)
		log.Printf("[req=%s] +++++ Warning:%s (SHD_EML_240)", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InvalidPassword,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_248"})

		return http.StatusUnauthorized, error_msg
	}

	// Generate a secure random session ID
	sessionID := ApiUtils.GenerateSecureToken(32) // e.g., 256-bit random string

	// Save session in DB/cache: map sessionID → user_email (or user_id)
	expired_time := time.Now().Add(cookie_timeout_hours * time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)
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
		error_msg := fmt.Sprintf("failed saving session:%v, email:%s, session_id:%s (SHD_EML_272)",
			err1, req.Email, sessionID)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_282"})

		return http.StatusInternalServerError, error_msg
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

	rc.SetCookie(sessionID)

	// Construct redirect URL
	redirect_url := ApiTypes.DatabaseInfo.HomeURL
	if redirect_url == "" {
		redirect_url = "localhost:5173"

		error_msg := fmt.Sprintf("missing home_url in config, email:%s, session_id:%s, redirect to:%s",
			req.Email, sessionID, redirect_url)
		log.Printf("[req=%s] ***** Alarm:%s (SHD_EML_301)", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_MissHomeURL,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_312"})
	} else {
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
	}

	// response := VerifyResponse{
	// 	Name:        user_name,
	// 	Email:       req.Email,
	// 	RedirectURL: redirect_url,
	// 	Loc:         "SHD_EML_190",
	msg := fmt.Sprintf("email login success, email:%s, redirectURL:%s, loc:(SHD_EML_190)",
		req.Email, redirect_url)
	return http.StatusOK, msg
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
		e.JSON(status_code, resp)
	} else {
		e.String(status_code, msg)
	}
	return nil
}

func HandleEmailVerifyBase(
	rc RequestHandlers.RequestContext,
	reqID string) (int, string, map[string]string) {
	log.Printf("[req=%s] Handle email verify request (SHD_EML_192)", reqID)
	reqCtx := rc.Context()
	user_name, ok := reqCtx.Value(ApiTypes.UserContextKey).(string)
	if !ok {
		// Handle case where "user_name" is not set (e.g., log an error)
		error_msg := "user_name not found in context (SHD_EML_363)"
		log.Printf("[req=%s] +++++ WARNING:%s", reqID, error_msg)
		user_name = "no-user-name-found"
	}

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
		error_msg := fmt.Sprintf("Failed retrieving token, user_name:%s, log_id:%d", user_name, log_id)
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
		reqID, token, ApiTypes.DatabaseInfo.DBType, ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)

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

	// Generate a secure random session ID
	sessionID := ApiUtils.GenerateSecureToken(32) // e.g., 256-bit random string

	// Save session in DB/cache: map sessionID → user_email (or user_id)
	expired_time := time.Now().Add(cookie_timeout_hours * time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)
	err1 := sysdatastores.SaveSession(
		"email",
		sessionID,
		user_info.UserName,
		"email",
		user_info.Email,
		user_info.Email,
		expired_time)

	if err1 != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to save session, err:%s, log_id:%d", err1.Error(), log_id)
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

	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:  "email_signup",
		SessionID:    sessionID,
		Status:       "active",
		UserName:     user_info.UserName,
		UserNameType: "email",
		UserRegID:    user_info.Email,
		UserEmail:    &user_info.Email,
		CallerLoc:    "SHD_EML_435",
		ExpiresAt:    &expired_time_str,
	})

	err1 = sysdatastores.MarkUserVerified(reqID, user_info.UserName)
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
	} else {
		log.Printf("[req=%s] Email signup success (SHD_EML_253): email:%s, session_id:%s", reqID, user_info.Email, sessionID)
	}

	rc.SetCookie(sessionID)

	redirect_url := ApiTypes.DatabaseInfo.HomeURL
	if redirect_url == "" {
		redirect_url = "localhost:5173"
		error_msg := fmt.Sprintf("missing home_url config, email:%s, session_id:%s, default to:%s",
			user_info.Email, sessionID, redirect_url)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_ConfigError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_481"})
	} else {
		msg := fmt.Sprintf("Email verify success: email:%s, session_id:%s, redirect:%s",
			user_info.Email, sessionID, redirect_url)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_VerifyEmailSuccess,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &msg,
			CallerLoc:    "SHD_EML_505"})
	}

	response := map[string]string{
		"name":         user_info.UserName, // or user.Email if that's what you use
		"email":        user_info.Email,
		"redirect_url": redirect_url,
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

	// Read body once
	body, err := io.ReadAll(rc.GetBody())
	if err != nil {
		// handle read error
	}

	// Log it (temporarily)
	log.Printf("REQUEST BODY: %s", string(body))

	// Restore body
	// rc.Request().Body = io.NopCloser(bytes.NewReader(body))

	var req EmailSignupRequest
	// req := map[string]interface{}{}
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
	} else {
		log.Printf("[req=%s] Parsing request success (SHD_EML_627), req:%v", reqID, req)
	}

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
	_, exist := rc.GetUserInfoByEmail(reqID, req.Email)
	if exist {
		error_msg := fmt.Sprintf("email already exist (SHD_EML_588), email:%s", req.Email)
		log.Printf("[req=%s] +++++ WARN %s", reqID, error_msg)

		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_588",
		}
		return http.StatusConflict, resp
	}

	// 2. Hash password
	hashedPwd, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)

	// 3. Generate a verification token and Create a user record with "verified = false"
	token := uuid.NewString()
	var user_name = req.Email
	err1 := rc.UpsertUser(reqID,
		"email", user_name, string(hashedPwd), req.Email, "email", "pending_verify",
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

	// 4. Send verification email
	verificationURL := fmt.Sprintf("http://localhost:5173/auth/email/verify?token=%s", token)
	go sendVerificationEmail(reqID, req.Email, verificationURL)

	log_id := sysdatastores.NextActivityLogID()
	resp_msg := fmt.Sprintf("Signup successful! Please check your email:%s to verify your account, log_id:%d.",
		req.Email, log_id)
	log.Printf("[req=%s] %s (SHD_EML_399)", reqID, resp_msg)
	resp := EmailSignupResponse{
		Message: resp_msg,
		LOC:     "SHD_EML_399",
	}

	msg := fmt.Sprintf("user signup success, user_name:%s, password_hash:%s, email:%s",
		user_name, hashedPwd, req.Email)

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
	var req struct {
		Email string `json:"email"`
	}
	if err := rc.Bind(reqID, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieve email, log_id:%d (SHD_EML_702)", log_id)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_712"})

		return http.StatusBadRequest, map[string]string{"error": error_msg}
	}

	if !isValidEmail(req.Email) {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid email:%s, log_id:%d (SHD_EML_710)", req.Email, log_id)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InvalidEmail,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_720"})

		return http.StatusBadRequest, map[string]string{"error": error_msg}
	}

	// 1. Check if email already exists
	user, err := sysdatastores.GetUserInfoByEmail(reqID, req.Email)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieving user, error:%v, email:%s, log_id:%d (SHD_EML_710)",
			err, req.Email, log_id)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UserNotFound,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_731"})

		return http.StatusNotFound, map[string]string{"error": "user not found (SHD_EML_179)"}
	}

	token := uuid.NewString()
	databaseutil.UpdateVTokenByEmail(ApiTypes.DatabaseInfo.DBType,
		ApiTypes.LibConfig.SystemTableNames.TableNameUsers, req.Email, token)

	resetURL := fmt.Sprintf("http://localhost:8080/auth/email/reset?token=%s", token)
	go ApiUtils.SendMail(reqID, req.Email, "Password Reset", fmt.Sprintf(`
        <p>Hi %s,</p>
        <p>Click the link below to reset your password:</p>
        <p><a href="%s">%s</a></p>
    `, user.UserName, resetURL, resetURL), "SHD_EML_732")

	log_id := sysdatastores.NextActivityLogID()
	msg := fmt.Sprintf("reset link sent to email:%s, log_id:%d (SHD_EML_765)", req.Email, log_id)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		LogID:        log_id,
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_UserNotFound,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_775"})

	return http.StatusOK, map[string]string{"message": "reset link sent to email (SHD_EML_193)"}
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
	redirect_url := fmt.Sprintf("http://localhost:5173/reset-password?token=%s", token)
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

	var req ResetConfirmRequest
	if err := json.NewDecoder(rc.GetBody()).Decode(&req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request payload, log_id:%d, user_name:%s (SHD_EML_693)",
			log_id, user_name)

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
	user, err := sysdatastores.LookupUserByToken(reqID, req.Token)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("user not found, user_name:%s, token:%s, log_id:%d",
			user_name, req.Token, log_id)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UserNotFound,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_723"})

		e_msg := fmt.Sprintf("user not found, user_name:%s, log_id:%d (SHD_EML_704)", user_name, log_id)
		return http.StatusBadRequest, e_msg
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to hash password, user_name:%s, password:%s, log_id:%d",
			user_name, req.Password, log_id)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InternalError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_743"})

		e_msg := fmt.Sprintf("internal error, user_name:%s, log_id:%d (SHD_EML_715)", user_name, log_id)
		return http.StatusInternalServerError, e_msg
	}

	log.Printf("[req=%s] Update password (SHD_EML_259), username:%s", reqID, user.UserName)

	// Update user password
	err = sysdatastores.UpdatePasswordByUserName(reqID, user.UserName, string(hashedPassword))
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed updating database, user_name:%s, error:%v, log_id:%d",
			user_name, err, log_id)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_InternalError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_736"})

		e_msg := fmt.Sprintf("internal error, user_name:%s, log_id:%d (SHD_EML_728)", user_name, log_id)
		return http.StatusInternalServerError, e_msg
	}

	log_id := sysdatastores.NextActivityLogID()
	msg := fmt.Sprintf("reset password success, user_name:%s, log_id:%d", user.UserName, log_id)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_InternalError,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_EML_779"})
	return http.StatusOK, "Password has been reset successfully (SHD_EML_263)."
}
