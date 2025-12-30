// server/api/Auth/google.go
package auth

// server/api/Auth/google.go
import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/RequestHandlers"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func getGoogleOauthConfig() *oauth2.Config {
	redirectURL := os.Getenv("GOOGLE_OAUTH_REDIRECT_URL")
	if redirectURL == "" {
		// Default to port 8090 for Pocketbase, 8080 for Echo
		redirectURL = "http://localhost:8090/auth/google/callback"
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
	log.Printf("HandleGoogleLogin called (MID_GGL_043)")
	config := getGoogleOauthConfig()
	url := config.AuthCodeURL(oauthStateString, oauth2.AccessTypeOffline)
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func HandleGoogleLoginPocketbase(e *core.RequestEvent) error {
	log.Printf("HandleGoogleLoginPocket called (MID_GGL_050)")
	config := getGoogleOauthConfig()
	log.Printf("OAuth Config - ClientID: %s, RedirectURL: %s (MID_GGL_051)", config.ClientID, config.RedirectURL)
	url := config.AuthCodeURL(oauthStateString, oauth2.AccessTypeOffline)
	log.Printf("Redirecting to Google OAuth URL (MID_GGL_052)")
	http.Redirect(e.Response, e.Request, url, http.StatusTemporaryRedirect)
	return nil
}

func HandleGoogleCallback(c echo.Context) error {
	rc := RequestHandlers.NewFromEcho(c)
	// body, _ := io.ReadAll(c.Request().Body)
	reqID := rc.ReqID()
	status_code, redirect_url := HandleGoogleCallbackBase(rc, reqID)
	if status_code == http.StatusSeeOther {
		return c.Redirect(http.StatusSeeOther, redirect_url)
	}
	return c.String(status_code, redirect_url)
}

func HandleGoogleCallbackPocket(e *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()

	log.Printf("[req=%s] HandleGoogleCallbackPocket called (SHD_GGL_071)", reqID)

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
	googleUserInfo, err := getUserInfo(rc.Context(), code)
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

	err = rc.UpsertUser(reqID,
		"google", "", "", googleUserInfo.Email, "google",
		"active", googleUserInfo.GivenName,
		googleUserInfo.FamilyName, "", googleUserInfo.Picture)
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
	// The frontend will receive the token and set it in the Pocketbase authStore
	// Note: Using /oauth/callback instead of /auth/callback to avoid the (auth) layout
	/*
		redirect_url := "http://localhost:5173/oauth/callback"
		redirectURL := fmt.Sprintf("%s?token=%s&name=%s",
			redirect_url,
			url.QueryEscape(token),
			url.QueryEscape(googleUserInfo.Name))
	*/
	redirectURL := rc.GetRedirectURL(reqID, token, googleUserInfo.Name)
	http.Redirect(e.Response, e.Request, redirectURL, http.StatusSeeOther)
	return nil
}

func HandleGoogleCallbackBase(
	rc RequestHandlers.RequestContext,
	reqID string) (int, string) {
	log.Printf("HandleGoogleCallback called (MID_GGL_020)")
	if rc.FormValue("state") != oauthStateString {
		error_msg := fmt.Sprintf("invalid oauth state: expected %s, got %s",
			oauthStateString, rc.FormValue("state"))
		log.Printf("***** Alarm %s (MID_GGL_042)", error_msg)
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
	userInfo, err := getUserInfo(rc.Context(), code)
	if err != nil {
		error_msg := fmt.Sprintf("failed to get user info: %v (MID_GGL_055)", err)
		log.Printf("***** Alarm %s", error_msg)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_AuthFailure,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GGL_068"})
		return http.StatusInternalServerError, "failed to get user info (MID_GGL_054): " + err.Error()
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
		reqID,
		"google_login",
		sessionID,
		userInfo.Email,
		"email",
		userInfo.Email,
		userInfo.Email,
		expired_time)
	if err1 != nil {
		error_msg := fmt.Sprintf("failed to save session: %s (MID_GGL_076)", err1)
		log.Printf("***** Alarm %s", error_msg)
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
		Status:       "active",
		UserName:     userInfo.Email,
		UserNameType: "email",
		UserRegID:    userInfo.Email,
		UserEmail:    &userInfo.Email,
		CallerLoc:    "SHD_GGL_123",
		ExpiresAt:    &expired_time_str,
	})

	if !userInfo.VerifiedEmail {
		error_msg := fmt.Sprintf("***** Alarm Unverified email login attempt, email:%s (MID_GGL_118)", userInfo.Email)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UnverifiedEmail,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GGL_126"})
		return http.StatusUnauthorized, error_msg
	}

	// Add user to database by rc.
	err = rc.UpsertUser(
		reqID, "google", userInfo.Email, "", userInfo.Email,
		"google", "active", userInfo.GivenName, userInfo.FamilyName,
		"", userInfo.Picture)

	if err != nil {
		error_msg := fmt.Sprintf("failed creating user, email:%s, err:%s (SHD_GGL_125)", userInfo.Email, err1)
		log.Printf("[req=%s] ***** Alarm %s", reqID, error_msg)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GGL_155"})
		return http.StatusInternalServerError, error_msg
	}

	msg := fmt.Sprintf("User registered, email:%s, name:%s %s, picture:%s, locale:%s",
		userInfo.Email,
		userInfo.GivenName,
		userInfo.FamilyName,
		userInfo.Picture,
		userInfo.Locale)

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
	log.Printf("%s (SHD_GGL_197)", msg1)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_SetCookie,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_GoogleAuth,
		ActivityMsg:  &msg1,
		CallerLoc:    "SHD_GGL_179"})

	// Construct redirect URL
	// redirect_url := ApiTypes.DatabaseInfo.HomeURL
	redirect_url := "http://localhost:8090/admin"
	if redirect_url == "" {
		log.Printf("***** Alarm missing home_url config (MID_GGL_094)")
		redirect_url = "localhost:8090"
	}

	// Redirect to the home URL
	redirectURL := fmt.Sprintf("%s?name=%s", redirect_url, url.QueryEscape(userInfo.Name))

	msg2 := fmt.Sprintf("User %s (%s) logged in successfully, redirect to:%s",
		userInfo.Name, userInfo.Email, redirectURL)
	log.Printf("%s (SHD_GGL_217)", msg2)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_AuthSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_GoogleAuth,
		ActivityMsg:  &msg2,
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

// getUserInfo: use oauth2.Config.Exchange to get token，then use config.Client to parse JSON
// Upon success, it returns an instance of 'userInfoResp'.
func getUserInfo(ctx context.Context, code string) (*userInfoResp, error) {
	log.Printf("google getUserInfo (MID_GGL_119)")
	config := getGoogleOauthConfig()
	token, err := config.Exchange(ctx, code)
	if err != nil {
		error_msg := fmt.Errorf("code exchange failed (MID_GGL_122): %w", err)
		log.Printf("***** Alarm Failed Google auth, err:%s", error_msg.Error())
		return nil, error_msg
	}

	// Use oauth2 client（will automatically attach access token）
	client := config.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		error_msg := fmt.Errorf("failed to get userinfo (MID_GGL_121): %w", err)
		log.Printf("***** Alarm %s", error_msg.Error())
		return nil, error_msg
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		error_msg := fmt.Errorf("userinfo endpoint returned status %d (MID_GGL_126)", resp.StatusCode)
		log.Printf("***** Alarm :%s", error_msg.Error())
		return nil, error_msg
	}

	var ui userInfoResp
	if err := json.NewDecoder(resp.Body).Decode(&ui); err != nil {
		error_msg := fmt.Errorf("failed to decode userinfo: %w (MID_GGL_131)", err)
		log.Printf("***** %s", error_msg.Error())
		return nil, error_msg
	}

	log.Printf("Google auth successful (MID_GGL_150)")
	return &ui, nil
}
