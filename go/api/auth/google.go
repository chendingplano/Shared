// server/api/Auth/google.go
package auth

// server/api/Auth/google.go
import (
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

var oauthStateString = "random-string" // 开发阶段可用常量，生产环境请生成并验证

func HandleGoogleLogin(c echo.Context) error {
	config := getGoogleOauthConfig()
	url := config.AuthCodeURL(oauthStateString, oauth2.AccessTypeOffline)
	log.Printf("HandleGoogleLogin called (MID_GGL_043), redirect to:%s", url)
	return c.Redirect(http.StatusTemporaryRedirect, url)
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

/*
func HandleGoogleCallbackPocket(e *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(e)
	logger := rc.GetLogger()
	reqID := rc.ReqID()

	logger.Info("HandleGoogleCallbackPocket called")

	// Get the OAuth code from the callback
	if rc.FormValue("state") != oauthStateString {
		error_msg := fmt.Sprintf("invalid oauth state: expected %s, got %s",
			oauthStateString, rc.FormValue("state"))
		log.Printf("[req=%s] ***** Alarm %s (SHD_GGL_077)", reqID, error_msg)
		e.String(http.StatusBadRequest, "invalid oauth state")
		return nil
	}

	code := rc.FormValue("code")
	if code == "" {
		error_msg := "code not found in request (SHD_GGL_084)"
		log.Printf("[req=%s] ***** Alarm %s", reqID, error_msg)
		e.String(http.StatusBadRequest, error_msg)
		return nil
	}

	// Get user info from Google
	googleUserInfo, err := getGoogleUserInfo(rc.Context(), code)
	if err != nil {
		error_msg := fmt.Sprintf("failed to get user info: %v (SHD_GGL_092)", err)
		log.Printf("[req=%s] ***** Alarm %s", reqID, error_msg)
		e.String(http.StatusInternalServerError, "failed to get user info")
		return nil
	}

	if !googleUserInfo.VerifiedEmail {
		error_msg := fmt.Sprintf("unverified email login attempt, email:%s (SHD_GGL_099)",
			googleUserInfo.Email)
		log.Printf("[req=%s] ***** Alarm %s", reqID, error_msg)
		e.String(http.StatusUnauthorized, "email not verified")
		return nil
	}

	// Find or create the user in Pocketbase
	usersCollection, err := e.App.FindCollectionByNameOrId("users")
	if err != nil {
		error_msg := fmt.Sprintf("failed to find users collection: %v (SHD_GGL_108)", err)
		log.Printf("[req=%s] ***** Alarm %s", reqID, error_msg)
		e.String(http.StatusInternalServerError, "internal error")
		return nil
	}

	// Will generate a password since Pocketbase does not allow empty password
	password := ApiUtils.GeneratePassword(rc, 12)

	var user_info ApiTypes.UserInfo
	user_info.UserIdType = "google"
	user_info.UserName = googleUserInfo.Email
	user_info.Email = googleUserInfo.Email
	user_info.AuthType = "google"
	user_info.UserStatus = "active"
	user_info.FirstName = googleUserInfo.GivenName
	user_info.LastName = googleUserInfo.FamilyName
	user_info.Avatar = googleUserInfo.Picture
	user_info, err = rc.UpsertUser(user_info,
		password, false, false, false, false)

	if err != nil {
		error_msg := fmt.Sprintf("failed upsert user (SHD_GGL_173), err: %v", err)
		log.Printf("[req=%s] ***** Alarm %s", reqID, error_msg)
		e.String(http.StatusInternalServerError, "failed to generate token")
		return nil
	}

	userRecord, err := e.App.FindAuthRecordByEmail(usersCollection.Id, googleUserInfo.Email)
	if err != nil {
		error_msg := fmt.Sprintf("internal error (SHD_GGL_182), err: %v", err)
		log.Printf("[req=%s] ***** Alarm %s", reqID, error_msg)
		e.String(http.StatusInternalServerError, "failed to generate token")
		return nil
	}

	// Generate Pocketbase auth token
	token, tokenErr := userRecord.NewAuthToken()
	if tokenErr != nil {
		error_msg := fmt.Sprintf("failed to generate auth token: %v (SHD_GGL_167)", tokenErr)
		log.Printf("[req=%s] ***** Alarm %s", reqID, error_msg)
		e.String(http.StatusInternalServerError, "failed to generate token")
		return nil
	}

	log.Printf("[req=%s] Successfully authenticated user %s via Google OAuth (SHD_GGL_176)",
		reqID, googleUserInfo.Email)

	// Redirect to frontend OAuth callback page with token
	// Note: Using /oauth/callback instead of /auth/callback to avoid the (auth) layout
	redirectURL := ApiUtils.GetOAuthRedirectURL(rc, token, googleUserInfo.Name)
	http.Redirect(e.Response, e.Request, redirectURL, http.StatusSeeOther)
	return nil
}
*/

func HandleGoogleCallbackBase(
	rc ApiTypes.RequestContext) (int, string) {
	logger := rc.GetLogger()
	logger.Info("HandleGoogleCallback called")
	if rc.FormValue("state") != oauthStateString {
		error_msg := fmt.Sprintf("invalid oauth state: expected %s, got %s",
			oauthStateString, rc.FormValue("state"))
		logger.Error("invalid oauth state",
			"expecting", oauthStateString,
			"actual", rc.FormValue("state"))
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

	// Redirect to the home URL
	redirectURL := fmt.Sprintf("%s?name=%s", redirect_url, url.QueryEscape(googleUserInfo.Name))

	logger.Info("Google login success",
		"is_admin", user_info.Admin,
		"redirectURL", redirectURL,
		"email", googleUserInfo.Email,
		"user_name", googleUserInfo.Name,
		"http_only", "true")

	msg3 := fmt.Sprintf("User %s (%s) logged in successfully, redirect to:%s",
		googleUserInfo.Name, googleUserInfo.Email, redirectURL)

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
