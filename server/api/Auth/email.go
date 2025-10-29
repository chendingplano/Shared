package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/chendingplano/Shared/server/api/ApiUtils"
	"github.com/chendingplano/Shared/server/api/databaseutil"
	"github.com/chendingplano/Shared/server/api/datastructures"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// var mu sync.Mutex

type EmailSignupRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type EmailLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type EmailLoginResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type EmailSignupResponse struct {
	Message string `json:"message"`
}

func HandleEmailLogin(w http.ResponseWriter, r *http.Request) {
	var req EmailLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		error_msg := "invalid request body (MID_EML_043)"
		log.Printf("***** Alarm Handle Email Login failed:%s", error_msg)
		http.Error(w, error_msg, http.StatusBadRequest)
		return
	}

	stored_password := databaseutil.GetUserPasswordByEmail(AuthInfo.DBType, AuthInfo.UsersTableName, req.Email)
	if stored_password == "" {
		error_msg := fmt.Sprintf("Email does not exist:%s (MID_EML_060)", req.Email)
		log.Printf("+++++ Warning %s", error_msg)
		http.Error(w, error_msg, http.StatusUnauthorized)
		return
	}

	// Hash password
	err := bcrypt.CompareHashAndPassword([]byte(stored_password), []byte(req.Password))
	if err != nil {
		error_msg := "invalid password (MID_EML_052)"
		log.Printf("+++++ Warning Invalid password:%s, password:%s, retrieved:%s", error_msg, req.Password, stored_password)
		http.Error(w, error_msg, http.StatusUnauthorized)
		return
	}

	// Generate a secure random session ID
	sessionID := generateSecureToken(32) // e.g., 256-bit random string

	// Save session in DB/cache: map sessionID → user_email (or user_id)
	err1 := databaseutil.SaveSession(
		AuthInfo.DBType,
		AuthInfo.SessionTableName,
		"google_login",
		sessionID,
		req.Email,
		"email",
		req.Email,
		time.Now().Add(24*time.Hour))
	if err1 != nil {
		http.Error(w, "failed to save session (MID_EML_086): "+err1.Error(), http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // require HTTPS in production
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("Email login success (MID_EML_071): email:%s, session_id:%s", req.Email, sessionID)

	resp := EmailLoginResponse{
		Name:  "DeepDoc User",
		Email: req.Email,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func sendVerificationEmail(to string, url string) error {
	subject := "Verify your email address"
	body := fmt.Sprintf(`
        <p>Welcome to DeepDocs!</p>
        <p>Please click the link below to verify your email:</p>
        <p><a href="%s">%s</a></p>
    `, url, url)

	return ApiUtils.SendMail(to, subject, body) // implement this using smtp.SendMail or a library
}

func HandleEmailVerify(c echo.Context) error {
	token := c.QueryParam("token")
	user_info, err := databaseutil.LookupUserByToken(AuthInfo.DBType, AuthInfo.SessionTableName, token)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid or expired verification link (MID_EML_119).")
	}

	err1 := databaseutil.MarkUserVerified(AuthInfo.DBType, AuthInfo.SessionTableName, user_info.UserName)
	if err1 == nil {
		return c.String(http.StatusOK, "Your email has been verified (MID_EML_124)! You can now log in.")
	}

	return c.String(http.StatusInternalServerError, "Database failed (MID_EML_127)")

}

func HandleEmailSignup(c echo.Context) error {
	var req EmailSignupRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request (MID_EML_099)"})
	}

	// 1. Check if email already exists
	// if databaseutil.UserExists(req.Email) {
	user_status := databaseutil.GetUserStatus(AuthInfo.DBType, AuthInfo.UsersTableName, req.Email)
	if user_status == "pending_varified" {
		msg := fmt.Sprintf("An email has sent to '%s'. Please check the email and click the link to activate your account (MID_EML_142)", req.Email)
		return c.JSON(http.StatusOK, map[string]string{"message": msg})
	}

	// 2. Hash password
	hashedPwd, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)

	// 3. Create a user record with "verified = false"
	var user_name string
	var user_type string = "email"
	var user_id_type string
	if req.Name == "" {
		user_name = req.Email
		user_id_type = "user_name"
	} else {
		user_name = req.Email
		user_id_type = "email"
	}

	// 4. Generate a verification token (e.g., UUID)
	token := uuid.NewString()

	user_info := datastructures.UserInfo{
		UserName:    user_name,
		Password:    string(hashedPwd),
		UserIdType:  user_id_type,
		RealName:    "",
		Email:       req.Email,
		PhoneNumber: "",
		UserType:    user_type,
		VToken:      token,
		Status:      "pending_varified",
	}
	status, err1 := databaseutil.AddUser(AuthInfo.DBType, AuthInfo.UsersTableName, user_info)
	if !status {
		log.Printf("***** Alarm Failed creating user, user_info:%s, err:%s", user_info, err1)
		return c.JSON(http.StatusConflict, map[string]string{"error": "Failed creating user (MID_EML_140)"})
	}

	// 5. Send verification email
	verificationURL := fmt.Sprintf("http://localhost:8080/auth/email/verify?token=%s", token)
	go sendVerificationEmail(req.Email, verificationURL)

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Signup successful! Please check your email to verify your account (MID_EML_167).",
	})
}

func HandleForgotPassword(c echo.Context) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request (MID_EML_174)"})
	}

	user, err := databaseutil.GetUserByEmail(AuthInfo.DBType, AuthInfo.UsersTableName, req.Email)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found (MID_EML_179)"})
	}

	token := uuid.NewString()
	databaseutil.UpdateVTokenByEmail(AuthInfo.DBType, AuthInfo.SessionTableName, req.Email, token)

	resetURL := fmt.Sprintf("http://localhost:8080/auth/email/reset?token=%s", token)
	go ApiUtils.SendMail(req.Email, "Password Reset", fmt.Sprintf(`
        <p>Hi %s,</p>
        <p>Click the link below to reset your password:</p>
        <p><a href="%s">%s</a></p>
    `, user.UserName, resetURL, resetURL))

	return c.JSON(http.StatusOK, map[string]string{
		"message": "reset link sent to email (MID_EML_193)",
	})
}

func HandleResetLink(c echo.Context) error {
	token := c.QueryParam("token")
	_, err := databaseutil.LookupUserByToken(AuthInfo.DBType, AuthInfo.SessionTableName, token)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid or expired reset link (MID_EML_201).")
	}
	// Redirect to frontend reset form
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("http://localhost:5173/reset-password?token=%s", token))
}

type ResetConfirmRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func HandleResetPasswordConfirm(c echo.Context) error {
	// The frontend (routes/reset-password/+page.svelte)
	// fetches (POST) this endpoint with Token and Password.
	// It retrieves the Token and Password.
	var req ResetConfirmRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid request payload")
	}

	// Validate token and get user
	user, err := databaseutil.LookupUserByToken(AuthInfo.DBType, AuthInfo.SessionTableName, req.Token)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid or expired token (MID_RST_001)")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to hash password")
	}

	log.Printf("Update password (MID_EML_259), password:%s, encoded:%s, username:%s", req.Password, string(hashedPassword), user.UserName)

	// Update user password
	err = databaseutil.UpdatePasswordByUserName(AuthInfo.DBType, AuthInfo.UsersTableName, user.UserName, string(hashedPassword))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed updating database (MID_EML_260)")
	}

	return c.String(http.StatusOK, "Password has been reset successfully (MID_EML_263).")
}
