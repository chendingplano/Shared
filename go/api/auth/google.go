// server/api/Auth/google.go
package auth

// server/api/Auth/google.go
import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func getGoogleOauthConfig() *oauth2.Config {
	// IMPORTANT: any time when you change your domain names, use need to configure
	// Google to allow your domains:
	// 	Link: https://console.cloud.google.com/apis/dashboard
	//		- Select "Credentials" from the last panel
	//		- Click Web Login link under "OAuth 2.0 Client IDs"
	//		- Find "Authorized redirect URLs", which is a list of allowed URLS
	//		- Add your URLs.
	redirectURL := os.Getenv("GOOGLE_OAUTH_REDIRECT_URL")
	if redirectURL == "" {
		error_msg := "missing GOOGLE_OAUTH_REDIRECT_URL env var (SHD_GGL_003)"
		log.Printf("***** Alarm: %s", error_msg)
	}
	return &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  redirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}
}

var oauthStateNonce = "random-string" // Base nonce for CSRF protection

// oauthState represents the data encoded in the OAuth state parameter.
// This carries data through the OAuth flow while maintaining CSRF protection.
type oauthState struct {
	Nonce     string `json:"n"` // CSRF nonce
	ReturnURL string `json:"r"` // Optional return URL after login
}

// encodeOAuthState encodes the state struct to a base64 string
func encodeOAuthState(state oauthState) string {
	data, err := json.Marshal(state)
	if err != nil {
		return oauthStateNonce // Fallback to simple nonce
	}
	return base64.URLEncoding.EncodeToString(data)
}

// decodeOAuthState decodes the base64 state string back to struct
func decodeOAuthState(encoded string) (oauthState, error) {
	// Handle legacy simple string state
	if encoded == oauthStateNonce {
		return oauthState{Nonce: oauthStateNonce}, nil
	}

	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return oauthState{}, err
	}

	var state oauthState
	if err := json.Unmarshal(data, &state); err != nil {
		return oauthState{}, err
	}
	return state, nil
}

func HandleGoogleLogin(c echo.Context) error {
	config := getGoogleOauthConfig()

	// Capture and validate returnUrl from query params
	returnURL := c.QueryParam("returnUrl")
	if returnURL != "" && !ApiUtils.IsSafeReturnURL(returnURL) {
		log.Printf("HandleGoogleLogin: rejected unsafe returnUrl: %s (SHD_GGL_081)", returnURL)
		returnURL = "" // Reject unsafe URLs
	}

	// Encode state with nonce and optional returnUrl
	state := oauthState{
		Nonce:     oauthStateNonce,
		ReturnURL: returnURL,
	}
	stateStr := encodeOAuthState(state)

	authURL := config.AuthCodeURL(stateStr, oauth2.AccessTypeOffline)
	log.Printf("HandleGoogleLogin called (MID_GGL_043), returnUrl:%s, redirect to:%s", returnURL, authURL)
	return c.Redirect(http.StatusTemporaryRedirect, authURL)
}

func HandleGoogleCallback(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_GGL_067")
	defer rc.Close()
	logger := rc.GetLogger()
	logger.Info("handle google callback")
	status_code, redirect_url := HandleGoogleCallbackBase(rc)
	if status_code == http.StatusSeeOther {
		return c.Redirect(http.StatusSeeOther, redirect_url)
	}
	return c.String(status_code, redirect_url)
}

func HandleGoogleCallbackBase(
	rc ApiTypes.RequestContext) (int, string) {
	logger := rc.GetLogger()
	logger.Info("HandleGoogleCallback called")

	// Decode and validate OAuth state
	stateStr := rc.FormValue("state")
	state, err := decodeOAuthState(stateStr)
	if err != nil || state.Nonce != oauthStateNonce {
		error_msg := fmt.Sprintf("invalid oauth state: got %s, error: %v",
			stateStr, err)
		logger.Error("invalid oauth state",
			"expecting_nonce", oauthStateNonce,
			"actual", stateStr,
			"error", err)
		var record = ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_AuthFailure,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GGL_043"}
		sysdatastores.AddActivityLog(record)

		return http.StatusBadRequest, "invalid oauth state (MID_GGL_043)"
	}

	// Extract and re-validate returnUrl from state (defense-in-depth)
	returnURL := ""
	if state.ReturnURL != "" && ApiUtils.IsSafeReturnURL(state.ReturnURL) {
		returnURL = state.ReturnURL
		logger.Info("OAuth callback with returnUrl", "returnUrl", returnURL)
	}

	code := rc.FormValue("code")
	if code == "" {
		error_msg := "code not found in request (SHD_GGL_060)"
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_AuthFailure,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GGL_068"})
		return http.StatusBadRequest, error_msg
	}

	// Retrieve user info from Google OAuth
	googleUserInfo, err := getGoogleUserInfo(rc, code)
	if err != nil {
		error_msg := fmt.Sprintf("failed to get user info, error:%v, code:%s", err, code)
		logger.Error("failed to get user info",
			"error", err,
			"code", code)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_AuthFailure,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GGL_068"})
		return http.StatusInternalServerError, "failed to get user info (MID_GGL_054): " + err.Error()
	}

	if !googleUserInfo.VerifiedEmail {
		error_msg := fmt.Sprintf("***** Alarm Unverified email login attempt, email:%s (MID_GGL_118)", googleUserInfo.Email)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UnverifiedEmail,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GGL_126"})
		return http.StatusUnauthorized, error_msg
	}

	// Generate auth token
	auth_token, err := rc.GenerateAuthToken(googleUserInfo.Email)
	if err != nil {
		error_msg := fmt.Sprintf("failed to generate auth token: %v (SHD_EML_272)", err)
		logger.Error("failed to generate auth token",
			"error", err,
			"email", googleUserInfo.Email)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_EML_282"})

		return http.StatusInternalServerError, error_msg
	}

	user_info, found := rc.GetUserInfoByEmail(googleUserInfo.Email)
	if !found {
		// Add user to database by rc.
		user_info.UserId = ApiUtils.GenerateUUID()
		user_info.UserIdType = "google"
		user_info.VToken = auth_token
		user_info.UserName = googleUserInfo.Email
		user_info.Email = googleUserInfo.Email
		user_info.AuthType = "google"
		user_info.UserStatus = "active"
		user_info.FirstName = googleUserInfo.GivenName
		user_info.LastName = googleUserInfo.FamilyName
		user_info.Avatar = googleUserInfo.Picture
		logger.Info("google avatar", "avatar", googleUserInfo.Picture)
	} else {
		if user_info.FirstName == "" {
			user_info.FirstName = googleUserInfo.GivenName
		}

		if user_info.LastName == "" {
			user_info.LastName = googleUserInfo.FamilyName
		}

		if user_info.Avatar == "" {
			user_info.Avatar = googleUserInfo.Picture
		}

		if user_info.AuthType == "" {
			user_info.AuthType = "google"
		}
	}

	user_info.VToken = auth_token
	user_info, err = rc.UpsertUser(user_info, "", true, false, false, true, false)
	logger.Info("upsert user", "found", found, "auth_token", auth_token)
	if err != nil {
		error_msg := fmt.Sprintf("failed creating user, email:%s, err:%s (SHD_GGL_125)", googleUserInfo.Email, err)
		logger.Error("failed creating user",
			"error", err,
			"email", googleUserInfo.Email)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GGL_155"})
		return http.StatusInternalServerError, error_msg
	}

	// Generate a secure random session ID
	sessionID := ApiUtils.GenerateSecureToken(32) // e.g., 256-bit random string

	// Save session in DB/cache: map sessionID → user_email (or user_id)
	expired_time := time.Now().Add(cookie_timeout_hours * time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)

	// Save the session to the session table through 'rc'. 'rc' is database agnostic.
	// Currently, it supports PostgreSQL, MySQL and Pocketbase.
	err1 := rc.SaveSession(
		"google_login",
		sessionID,
		auth_token,
		googleUserInfo.Email,
		"email",
		googleUserInfo.Email,
		googleUserInfo.Email,
		expired_time,
		false)
	if err1 != nil {
		error_msg := fmt.Sprintf("failed to save session: %s (MID_GGL_076)", err1)
		logger.Error("failed to save session",
			"error", err1,
			"email", googleUserInfo.Email)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GGL_113"})
		return http.StatusInternalServerError, "failed to save session (MID_GGL_068): " + err1.Error()
	}

	// Log the activity.
	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:  "google_login",
		SessionID:    sessionID,
		AuthToken:    auth_token,
		Status:       "active",
		UserName:     googleUserInfo.Email,
		UserNameType: "email",
		UserRegID:    googleUserInfo.Email,
		UserEmail:    &googleUserInfo.Email,
		CallerLoc:    "SHD_GGL_123",
		ExpiresAt:    &expired_time_str,
	})

	msg := fmt.Sprintf("User registered, email:%s, name:%s %s, picture:%s, locale:%s",
		googleUserInfo.Email,
		googleUserInfo.GivenName,
		googleUserInfo.FamilyName,
		googleUserInfo.Picture,
		googleUserInfo.Locale)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_UserCreated,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_GoogleAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_GGL_172"})

	// Generate a cookie.
	rc.SetCookie(sessionID)

	msg1 := fmt.Sprintf("Set cookie, session_id:%s, HttpOnly:true", sessionID)
	logger.Info("set cookie",
		"session_id", sessionID,
		"user_id", user_info.UserId,
		"is_admin", user_info.Admin,
		"http_only", "true")
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_SetCookie,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_GoogleAuth,
		ActivityMsg:  &msg1,
		CallerLoc:    "SHD_GGL_179"})

	// Important: it should redirect to oauth/callback to let appAuthStore
	// check auth/me.
	user_name := user_info.FirstName + " " + user_info.LastName
	redirect_url := ApiUtils.GetOAuthRedirectURL(rc, auth_token, user_name)
	msg2 := fmt.Sprintf("google login success, email:%s, session_id:%s, redirect_url:%s",
		user_info.Email, sessionID, redirect_url)
	logger.Info(
		"Google login success",
		"email", user_info.Email,
		"session_id", sessionID,
		"redirect_url", redirect_url,
		"loc", "SHD_EML_316")

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_UserLoginSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_EmailAuth,
		ActivityMsg:  &msg2,
		CallerLoc:    "SHD_EML_324"})

	// Redirect to the home URL, including returnUrl if present
	redirectURL := fmt.Sprintf("%s?name=%s", redirect_url, url.QueryEscape(googleUserInfo.Name))
	if returnURL != "" {
		redirectURL = fmt.Sprintf("%s&returnUrl=%s", redirectURL, url.QueryEscape(returnURL))
	}

	logger.Info("Google login success",
		"is_admin", user_info.Admin,
		"redirectURL", redirectURL,
		"returnURL", returnURL,
		"email", googleUserInfo.Email,
		"user_name", googleUserInfo.Name,
		"http_only", "true")

	msg3 := fmt.Sprintf("User %s (%s) logged in successfully, redirect to:%s, returnUrl:%s",
		googleUserInfo.Name, googleUserInfo.Email, redirectURL, returnURL)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_AuthSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_GoogleAuth,
		ActivityMsg:  &msg3,
		CallerLoc:    "SHD_GGL_201"})

	return http.StatusSeeOther, redirectURL
}

// userInfo, returned by Google Oauth
type userInfoResp struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale,omitempty"`
}

// getGoogleUserInfo: use oauth2.Config.Exchange to get token，then use config.Client to parse JSON
// Upon success, it returns an instance of 'userInfoResp'.
func getGoogleUserInfo(rc ApiTypes.RequestContext, code string) (*userInfoResp, error) {
	ctx := rc.Context()
	logger := rc.GetLogger()
	logger.Info("google getGoogleUserInfo")
	config := getGoogleOauthConfig()
	token, err := config.Exchange(ctx, code)
	if err != nil {
		error_msg := fmt.Errorf("code exchange failed (MID_GGL_122): %w", err)
		logger.Error("Failed Google auth", "error", err, "code", code)
		return nil, error_msg
	}

	// Use oauth2 client（will automatically attach access token）
	client := config.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		error_msg := fmt.Errorf("failed to get userinfo (MID_GGL_121): %w", err)
		logger.Error("failed to get UserInfo", "error", err, "token", token)
		return nil, error_msg
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		error_msg := fmt.Errorf("userinfo endpoint returned status %d (MID_GGL_126)", resp.StatusCode)
		logger.Error("non-ok status code returned", "status_code", resp.StatusCode)
		return nil, error_msg
	}

	var ui userInfoResp
	if err := json.NewDecoder(resp.Body).Decode(&ui); err != nil {
		error_msg := fmt.Errorf("failed to decode userinfo: %w (MID_GGL_131)", err)
		logger.Error("failed to decode user info", "error", err)
		return nil, error_msg
	}

	logger.Info("Google auth successful")
	return &ui, nil
}
