package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/datastructures"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	authmiddleware "github.com/chendingplano/shared/go/auth-middleware"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type EmailSignupRequest struct {
	FirstName     	string `json:"first_name"`
	LastName     	string `json:"last_name"`
	Email    		string `json:"email"`
	Password 		string `json:"password"`
}

type EmailSignupResponse struct {
	Message 		string `json:"message"`
	LOC 			string `json:"loc"`
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
	Name         string `json:"name"`
	Email        string `json:"email"`
	RedirectURL  string `json:"redirect_url"`
	Loc          string `json:"loc,omitempty"`
}

const (
	cookie_timeout_hours = 24
)

func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func HandleEmailLogin(c echo.Context) error {
	r := c.Request()

	log.Printf("HandleEmailLogin called (SHD_EML_076)")
	var req EmailLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		error_msg := "invalid request body (SHD_EML_043)"

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_119"})

		log.Printf("***** Alarm:%s", error_msg)
		return c.String(http.StatusBadRequest, error_msg)
	}

	if !isValidEmail(req.Email) {
		error_msg := "invalid email format (SHD_EML_081)"
		log.Printf("+++++ Warning:%s, email:%s", error_msg, req.Email)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadEmail,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_136"})

		return c.String(http.StatusBadRequest, error_msg)
	}

	user_name, stored_password, err_msg := sysdatastores.GetUserNameAndPasswordByEmail(req.Email)
	if err_msg != "" {
		// Possible reasons:
		// 1. user not found: user not found
		// 2. user pending verify: 
		// 3. database error: error: xxx
		// 4. invalid user: (status not active)
		if strings.HasPrefix(err_msg, "user not found:") {	
			error_msg := fmt.Sprintf("user not found, email:%s", req.Email)
			log.Printf("+++++Warning:%s (SHD_EML_152)", error_msg)

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				ActivityName: 		ApiTypes.Activity_Auth,
				ActivityType: 		ApiTypes.ActivityType_UserNotFound,
				AppName: 			ApiTypes.AppName_Auth,
				ModuleName: 		ApiTypes.ModuleName_EmailAuth,
				ActivityMsg: 		&error_msg,
				CallerLoc: 			"SHD_EML_160"})

			return c.String(http.StatusNotFound, error_msg)
		}

		if strings.HasPrefix(err_msg, "error:") {	
			error_msg := fmt.Sprintf("user not found, email:%s", req.Email)
			log.Printf("+++++Warning:%s", error_msg)

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				ActivityName: 		ApiTypes.Activity_Auth,
				ActivityType: 		ApiTypes.ActivityType_UserNotFound,
				AppName: 			ApiTypes.AppName_Auth,
				ModuleName: 		ApiTypes.ModuleName_EmailAuth,
				ActivityMsg: 		&error_msg,
				CallerLoc: 			"SHD_EML_176"})

			return c.String(http.StatusInternalServerError, error_msg)
		}

		if strings.HasPrefix(err_msg, "user pending verify:") {	
			error_msg := fmt.Sprintf("user pending verify, email:%s", req.Email)
			log.Printf("+++++Warning:%s", error_msg)

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				ActivityName: 		ApiTypes.Activity_Auth,
				ActivityType: 		ApiTypes.ActivityType_UserPending,
				AppName: 			ApiTypes.AppName_Auth,
				ModuleName: 		ApiTypes.ModuleName_EmailAuth,
				ActivityMsg: 		&error_msg,
				CallerLoc: 			"SHD_EML_192"})

			return c.String(http.StatusMethodNotAllowed, 
				"user pending verify (SHD_EML_185). Click 'Forgot Password' to re-reset your password!")
		}

		error_msg := fmt.Sprintf("failed to get user info: %s (SHD_EML_058)", err_msg)
		log.Printf("Warning %s", error_msg)
		return c.String(http.StatusForbidden, "invalid user (SHD_EML_099). Sign up with your email!")
	}

	if stored_password == nil {
		token := uuid.NewString()
		url := fmt.Sprintf("http://localhost:5173/auth/email/reset?token=%s", token)
		error_msg := fmt.Sprintf("user not set password yet, sent reset password email to:%s, email:%s, token:%s", 
				req.Email, token, url)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_SentEmail,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_216"})

		databaseutil.UpdateVTokenByEmail(ApiTypes.DatabaseInfo.DBType, 
				ApiTypes.LibConfig.SystemTableNames.TableName_Users, req.Email, token)

		subject := "Reset your password"
		body := fmt.Sprintf(`
        	<p>Please click the link below to reset your password:</p>
        	<p><a href="%s">%s</a></p>`, url, url)

		log.Printf("%s (SHD_EML_227)", error_msg)
		ApiUtils.SendMail(req.Email, subject, body)

		msg := fmt.Sprintf("You have not set the password yet. An email has been sent to your email:%s. " +
				"Please check your email and click the link to set your password (SHD_EML_135)", req.Email)
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": msg})
	}

	// Hash password
	err := bcrypt.CompareHashAndPassword([]byte(*stored_password), []byte(req.Password))
	if err != nil {
		error_msg := fmt.Sprintf("invalid password, email:%s", req.Email)
		log.Printf("+++++ Warning:%s (SHD_EML_240)", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_InvalidPassword,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_248"})

		return c.String(http.StatusUnauthorized, error_msg)
	}

	// Generate a secure random session ID
	sessionID := generateSecureToken(32) // e.g., 256-bit random string

	// Save session in DB/cache: map sessionID → user_email (or user_id)
	expired_time := time.Now().Add(cookie_timeout_hours*time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)
	err1 := sysdatastores.SaveSession(
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
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_DatabaseError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_282"})

		return c.String(http.StatusInternalServerError, error_msg)
	}

	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:	"email_login",
    	SessionID:		sessionID,
		Status:			"active",
    	UserName:		req.Email,
    	UserNameType:	"email",
    	UserRegID:		req.Email,
    	UserEmail:		&req.Email,
		CallerLoc:		"SHD_EML_267",
    	ExpiresAt:		&expired_time_str,
	})

	is_secure := authmiddleware.IsSecure()
	cookie := new(http.Cookie)
	cookie.Name = "session_id"
	cookie.Value = sessionID 
	cookie.Path  = "/" 
	cookie.HttpOnly = true 
	cookie.Secure = is_secure
	cookie.SameSite = http.SameSiteStrictMode
	c.SetCookie(cookie)

	// Construct redirect URL
	redirect_url := ApiTypes.DatabaseInfo.HomeURL
	if redirect_url == "" {
		redirect_url = "localhost:5173"

		error_msg := fmt.Sprintf("missing home_url in config, email:%s, session_id:%s, redirect to:%s", 
				req.Email, sessionID, redirect_url)
		log.Printf("***** Alarm:%s (SHD_EML_301)", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_MissHomeURL,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_312"})
	} else {
		msg1 := fmt.Sprintf("email login success, email:%s, session_id:%s, redirect_url:%s",
				req.Email, sessionID, redirect_url)
		log.Printf("%s (SHD_EML_316)", msg1)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_UserLoginSuccess,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&msg1,
			CallerLoc: 			"SHD_EML_324"})
	}

	response := VerifyResponse {
		Name:         user_name,
		Email:        req.Email,
		RedirectURL:  redirect_url,
		Loc:          "SHD_EML_190",
	}
	c.JSON(http.StatusOK, response)
	return nil
}

func sendVerificationEmail(to string, url string) error {
	log_id := sysdatastores.NextActivityLogID()
	subject := "Verify your email address"
	body := fmt.Sprintf(`
        <p>Please click the link below to verify your email (logid:%d):</p>
        <p><a href="%s">%s</a></p>`, log_id, url, url)

	msg := fmt.Sprintf("Sending verification email to %s with URL: %s, logid:%d", to, url, log_id)
	log.Printf("%s (SHD_EML_345)", msg)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		LogID:				log_id,
		ActivityName: 		ApiTypes.Activity_Auth,
		ActivityType: 		ApiTypes.ActivityType_UserLoginSuccess,
		AppName: 			ApiTypes.AppName_Auth,
		ModuleName: 		ApiTypes.ModuleName_EmailAuth,
		ActivityMsg: 		&msg,
		CallerLoc: 			"SHD_EML_351"})

	return ApiUtils.SendMail(to, subject, body) // implement this using smtp.SendMail or a library
}

func HandleEmailVerify(c echo.Context) error {
	log.Printf("Handle email verify request (SHD_EML_192)")
	reqCtx := c.Request().Context()
	user_name, ok := reqCtx.Value(ApiTypes.UserContextKey).(string)
    if !ok {
        // Handle case where "user_name" is not set (e.g., log an error)
        error_msg := "user_name not found in context (SHD_EML_363)"
        log.Printf("+++++ WARNING:%s", error_msg)
		user_name = "no-user-name-found"
    }

	var req EmailVerifyRequest
	if err := c.Bind(&req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("Failed to bind request body: %v, log_id:%d, user_name:%s", 
				err, log_id, user_name)
		log.Printf("%s (SHD_EML_361)", error_msg)
		
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_369"})

		e_msg := fmt.Sprintf("invalid request body, log_id:%d, user_name:%s (SHD_EML_361)", log_id, user_name)
		return c.String(http.StatusBadRequest, e_msg)
	}

	token := req.Token
	if token == "" {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("Failed retrieving token, user_name:%s, log_id:%d", user_name, log_id)
		log.Printf("***** Alarm:%s (SHD_EML_205)", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_400"})

		e_msg := fmt.Sprintf("failed retrieving token, log_id:%d (SHD_EML_393)", log_id)
		return c.String(http.StatusBadRequest, e_msg)
	}

	log.Printf("Handle email verify (SHD_EML_193), token:%s, dbtype:%s, tablename:%s",
		token, ApiTypes.DatabaseInfo.DBType, ApiTypes.LibConfig.SystemTableNames.TableName_Sessions)

	// TODO (Chen Ding, 2025/11/03)
	// Add timeout or rate-limiting to prevent abuse of this endpoint.
	// Validate token format early (e.g., check length, character set).
	// Use HTTPS in production — tokens in request bodies are still sensitive.

	user_info, err := sysdatastores.LookupUserByToken(token)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieving user by token:%s, error_msg:%v, log_id:%d", token, err, log_id)
		log.Printf("***** Alarm:%s (SHD_EML_420)", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_InvalidToken,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_429"})

		e_msg := fmt.Sprintf("invalid or expired email verification, log_id:%d (SHD_EML_431)", log_id)
		return c.String(http.StatusBadRequest, e_msg)
	}

	// Generate a secure random session ID
	sessionID := generateSecureToken(32) // e.g., 256-bit random string

	// Save session in DB/cache: map sessionID → user_email (or user_id)
	expired_time := time.Now().Add(cookie_timeout_hours*time.Hour)
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
		log.Printf("***** Alarm:%s (SHD_EML_429)", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_DatabaseError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_438"})
		return c.String(http.StatusInternalServerError, error_msg)
	}

	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:	"email_signup",
    	SessionID:		sessionID,
		Status:			"active",
    	UserName:		user_info.UserName,
    	UserNameType:	"email",
    	UserRegID:		user_info.Email,
    	UserEmail:		&user_info.Email,
		CallerLoc:		"SHD_EML_435",
    	ExpiresAt:		&expired_time_str,
	})

	err1 = sysdatastores.MarkUserVerified(user_info.UserName)
	if err1 != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("mark user failed, error:%v, user_name:%s, log_id:%d", 
				err1, user_info.UserName, log_id)
		log.Printf("***** Alarm:%s (SHD_EML_460)", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_DatabaseError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_468"})
	} else {
		log.Printf("Email signup success (SHD_EML_253): email:%s, session_id:%s", user_info.Email, sessionID)
	}

	is_secure := authmiddleware.IsSecure()
	cookie := new(http.Cookie)
	cookie.Name = "session_id"
	cookie.Value = sessionID 
	cookie.Path  = "/" 
	cookie.HttpOnly = true 
	cookie.Secure = is_secure
	cookie.SameSite = http.SameSiteStrictMode
	c.SetCookie(cookie)

	redirect_url := ApiTypes.DatabaseInfo.HomeURL
	if redirect_url == "" {
		redirect_url = "localhost:5173"
		error_msg := fmt.Sprintf("missing home_url config, email:%s, session_id:%s, default to:%s", 
				user_info.Email, sessionID, redirect_url)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_ConfigError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_481"})
	} else {
		msg := fmt.Sprintf("Email verify success: email:%s, session_id:%s, redirect:%s", 
				user_info.Email, sessionID, redirect_url)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_VerifyEmailSuccess,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&msg,
			CallerLoc: 			"SHD_EML_505"})
	}

	response := map[string]string{
   		"name": user_info.UserName, // or user.Email if that's what you use
   		"email": user_info.Email,
		"redirect_url": redirect_url,
		"loc": "SHD_EML_210",
	}
	w := c.Response()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
	return nil
}

func HandleEmailSignup(c echo.Context) error {
	// The request body:
	// {
	//   "first_name": "John",		// Optional
	//   "last_name": "Doe",		// Optional
	//   "email": "xxx",
	//   "password": "yyy"
	// }
	log.Printf("Handle email signup request (SHD_EML_300)")
	var req EmailSignupRequest
	if err := c.Bind(&req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request body, log_id:%d (SHD_EML_534)", log_id)
		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_311",
		}

		log.Printf("***** Alarm Handle Email Signup failed:%s", resp.Message)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_548"})

		return c.JSON(http.StatusBadRequest, resp)
	}

	if !isValidEmail(req.Email) {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid email format, email:%s, log_id:%d (SHD_EML_547)", req.Email, log_id)
		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_547",
		}

		log.Printf("+++++ Warning:%s", error_msg)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_InvalidEmail,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_548"})

		return c.JSON(http.StatusBadRequest, resp)
	}

	// 1. Check if email already exists
	// if databaseutil.UserExists(req.Email) {
	user_status := sysdatastores.GetUserStatus(req.Email)
	log.Printf("user status (SHD_EML_218), email:%s, status:%s", req.Email, user_status)
	if user_status != "" {
		if strings.HasPrefix(user_status, "error:") {
			log_id := sysdatastores.NextActivityLogID()
			error_msg := fmt.Sprintf("check user failed, status:%s, " +
						"email:%s, log_id%d (SHD_EML_565)", user_status, req.Email, log_id)
			log.Printf("***** Alarm:%s (SHD_EML_565)", error_msg)
			resp := EmailSignupResponse{
				Message: error_msg,
				LOC:     "SHD_EML_347",
			}

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				LogID:				log_id,
				ActivityName: 		ApiTypes.Activity_Auth,
				ActivityType: 		ApiTypes.ActivityType_InternalError,
				AppName: 			ApiTypes.AppName_Auth,
				ModuleName: 		ApiTypes.ModuleName_EmailAuth,
				ActivityMsg: 		&error_msg,
				CallerLoc: 			"SHD_EML_596"})

			return c.JSON(http.StatusInternalServerError, resp)
		}

		if user_status == "user not found" {
			log_id := sysdatastores.NextActivityLogID()
			error_msg := fmt.Sprintf("user already exists, status:%s, email:%s, log_id:%d (SHD_EML_357)",
						user_status, req.Email, log_id)
			log.Printf("***** Alarm:%s", error_msg)
			resp := EmailSignupResponse{
				Message: error_msg,
				LOC:     "SHD_EML_357",
			}

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				LogID:				log_id,
				ActivityName: 		ApiTypes.Activity_Auth,
				ActivityType: 		ApiTypes.ActivityType_UserExist,
				AppName: 			ApiTypes.AppName_Auth,
				ModuleName: 		ApiTypes.ModuleName_EmailAuth,
				ActivityMsg: 		&error_msg,
				CallerLoc: 			"SHD_EML_618"})

			return c.JSON(http.StatusOK, resp)
		}	
	}

	// 2. Hash password
	hashedPwd, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)

	// 3. Create a user record with "verified = false"
	var user_name = req.Email
	var user_type string = "email"
	var user_id_type = "email"

	// 4. Generate a verification token (e.g., UUID)
	token := uuid.NewString()

	user_info := datastructures.UserInfo{
		UserName:    user_name,
		Password:    databaseutil.StrPtr(string(hashedPwd)),
		UserIdType:  user_id_type,
		Email:       req.Email,
		FirstName:   databaseutil.StrPtr(req.FirstName),
		LastName:    databaseutil.StrPtr(req.LastName),
		UserType:    user_type,
		VToken:      databaseutil.StrPtr(token),
		Status:      "pending_verify",
	}
	status, err1 := sysdatastores.AddUserNew(user_info)
	if !status {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("Failed creating user, user_name:%s, email:%s, err:%s, log_id:%d", 
						user_name, req.Email, err1, log_id)
		log.Printf("***** Alarm %s (SHD_EML_393)", error_msg)
		resp := EmailSignupResponse{
			Message: error_msg,
			LOC:     "SHD_EML_393",
		}

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_DatabaseError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_664"})

		return c.JSON(http.StatusConflict, resp)
	}

	// 5. Send verification email
	verificationURL := fmt.Sprintf("http://localhost:5173/verifyemail?token=%s", token)
	go sendVerificationEmail(req.Email, verificationURL)

	log_id := sysdatastores.NextActivityLogID()
	resp_msg := fmt.Sprintf("Signup successful! Please check your email:%s to verify your account, log_id:%d.", 
				req.Email, log_id)
	log.Printf("%s (SHD_EML_399)", resp_msg)
	resp := EmailSignupResponse{
		Message: resp_msg,
		LOC:     "SHD_EML_399",
	}

	msg := fmt.Sprintf("user signup success, user_name:%s, password_hash:%s, email:%s",
			user_name, hashedPwd, req.Email)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		LogID:				log_id,
		ActivityName: 		ApiTypes.Activity_Auth,
		ActivityType: 		ApiTypes.ActivityType_SignupSuccess,
		AppName: 			ApiTypes.AppName_Auth,
		ModuleName: 		ApiTypes.ModuleName_EmailAuth,
		ActivityMsg: 		&msg,
		CallerLoc: 			"SHD_EML_689"})
	return c.JSON(http.StatusOK, resp)
}

func HandleForgotPassword(c echo.Context) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieve email, log_id:%d (SHD_EML_702)", log_id)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_712"})

		return c.JSON(http.StatusBadRequest, map[string]string{"error": error_msg})
	}

	if !isValidEmail(req.Email) {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid email:%s, log_id:%d (SHD_EML_710)", req.Email, log_id)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_InvalidEmail,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_720"})

		return c.JSON(http.StatusBadRequest, map[string]string{"error": error_msg})
	}

	// 1. Check if email already exists
	user, err := sysdatastores.GetUserInfoByEmail(req.Email)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieving user, error:%v, email:%s, log_id:%d (SHD_EML_710)", 
				err, req.Email, log_id)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_UserNotFound,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_731"})

		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found (SHD_EML_179)"})
	}

	token := uuid.NewString()
	databaseutil.UpdateVTokenByEmail(ApiTypes.DatabaseInfo.DBType, 
					ApiTypes.LibConfig.SystemTableNames.TableName_Users, req.Email, token)

	resetURL := fmt.Sprintf("http://localhost:8080/auth/email/reset?token=%s", token)
	go ApiUtils.SendMail(req.Email, "Password Reset", fmt.Sprintf(`
        <p>Hi %s,</p>
        <p>Click the link below to reset your password:</p>
        <p><a href="%s">%s</a></p>
    `, user.UserName, resetURL, resetURL))

	log_id := sysdatastores.NextActivityLogID()
	msg := fmt.Sprintf("reset link sent to email:%s, log_id:%d (SHD_EML_765)", req.Email, log_id)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		LogID:				log_id,
		ActivityName: 		ApiTypes.Activity_Auth,
		ActivityType: 		ApiTypes.ActivityType_UserNotFound,
		AppName: 			ApiTypes.AppName_Auth,
		ModuleName: 		ApiTypes.ModuleName_EmailAuth,
		ActivityMsg: 		&msg,
		CallerLoc: 			"SHD_EML_775"})

	return c.JSON(http.StatusOK, map[string]string{"message": "reset link sent to email (SHD_EML_193)"})
}

func HandleResetLink(c echo.Context) error {
	token := c.QueryParam("token")
	log.Printf("Handle reset link (SHD_EML_257), token:%s", token)
	_, err := sysdatastores.LookupUserByToken(token)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed retrieving user by token:%s (SHD_EML_201).", token)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_UserNotFound,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_795"})

		return c.String(http.StatusBadRequest, "Invalid or expired reset link (SHD_EML_201).")
	}
	// Redirect to frontend reset form
	redirect_url := fmt.Sprintf("http://localhost:5173/reset-password?token=%s", token)
	msg := fmt.Sprintf("handle reset, redirect to:%s", redirect_url)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: 		ApiTypes.Activity_Auth,
		ActivityType: 		ApiTypes.ActivityType_Redirect,
		AppName: 			ApiTypes.AppName_Auth,
		ModuleName: 		ApiTypes.ModuleName_EmailAuth,
		ActivityMsg: 		&msg,
		CallerLoc: 			"SHD_EML_808"})
	return c.Redirect(http.StatusSeeOther, redirect_url)
}

type ResetConfirmRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func HandleResetPasswordConfirm(c echo.Context) error {
	// The frontend (routes/reset-password/+page.svelte)
	// fetches (POST) this endpoint with Token and Password.
	// It retrieves the Token and Password.
	user_name, ok := c.Request().Context().Value(ApiTypes.UserContextKey).(string)
	if !ok {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("internal error (SHD_EML_693), log_id:%d", log_id)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_InternalError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_703"})

		return c.String(http.StatusBadRequest, error_msg)
	}

	var req ResetConfirmRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request payload, log_id:%d, user_name:%s (SHD_EML_693)", 
				log_id, user_name)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_701"})

		log.Printf("***** Alarm:%s", error_msg)

		return c.String(http.StatusBadRequest, error_msg)
	}

	// Validate token and get user
	user, err := sysdatastores.LookupUserByToken(req.Token)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("user not found, user_name:%s, token:%s, log_id:%d",
		 	user_name, req.Token, log_id)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_UserNotFound,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_723"})

		e_msg := fmt.Sprintf("user not found, user_name:%s, log_id:%d (SHD_EML_704)", user_name, log_id)
		return c.String(http.StatusBadRequest, e_msg)
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to hash password, user_name:%s, password:%s, log_id:%d", 
				user_name, req.Password, log_id)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_InternalError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_743"})

		e_msg := fmt.Sprintf("internal error, user_name:%s, log_id:%d (SHD_EML_715)", user_name, log_id)
		return c.String(http.StatusInternalServerError, e_msg)
	}

	log.Printf("Update password (SHD_EML_259), username:%s", user.UserName)

	// Update user password
	err = sysdatastores.UpdatePasswordByUserName(user.UserName, string(hashedPassword))
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed updating database, user_name:%s, error:%v, log_id:%d", 
				user_name, err, log_id)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID: 				log_id,
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_InternalError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_EmailAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_EML_736"})

		e_msg := fmt.Sprintf("internal error, user_name:%s, log_id:%d (SHD_EML_728)", user_name, log_id)
		return c.String(http.StatusInternalServerError, e_msg)
	}

	log_id := sysdatastores.NextActivityLogID()
	msg := fmt.Sprintf("reset password success, user_name:%s, log_id:%d", user.UserName, log_id)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: 		ApiTypes.Activity_Auth,
		ActivityType: 		ApiTypes.ActivityType_InternalError,
		AppName: 			ApiTypes.AppName_Auth,
		ModuleName: 		ApiTypes.ModuleName_EmailAuth,
		ActivityMsg: 		&msg,
		CallerLoc: 			"SHD_EML_779"})
	return c.String(http.StatusOK, "Password has been reset successfully (SHD_EML_263).")
}
