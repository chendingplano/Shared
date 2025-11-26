// server/api/Auth/google.go
package auth

// server/api/Auth/google.go
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/datastructures"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	authmiddleware "github.com/chendingplano/shared/go/auth-middleware"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var googleOauthConfig = &oauth2.Config{
	ClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
	ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
	RedirectURL:  "http://localhost:8080/auth/google/callback",
	Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
	Endpoint:     google.Endpoint,
}

var oauthStateString = "random-string" // 开发阶段可用常量，生产环境请生成并验证

func HandleGoogleLogin(c echo.Context) error {
	log.Printf("HandleGoogleLogin called (MID_GGL_010)")
	url := googleOauthConfig.AuthCodeURL(oauthStateString, oauth2.AccessTypeOffline)
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func HandleGoogleCallback(c echo.Context) error {
	req := c.Request()
	log.Printf("HandleGoogleCallback called (MID_GGL_020)")
	if req.FormValue("state") != oauthStateString {
		error_msg := fmt.Sprintf("invalid oauth state: expected %s, got %s", 
				oauthStateString, req.FormValue("state"))
		log.Printf("***** Alarm %s (MID_GGL_042)", error_msg)
		var record = ApiTypes.ActivityLogDef{
				ActivityName: 	ApiTypes.Activity_Auth,
				ActivityType: 	ApiTypes.ActivityType_AuthFailure,
				AppName: 		ApiTypes.AppName_Auth,
				ModuleName: 	ApiTypes.ModuleName_GoogleAuth,
				ActivityMsg:    &error_msg,
				CallerLoc:      "SHD_GGL_043",}
		sysdatastores.AddActivityLog(record)

		return c.String(http.StatusBadRequest, "invalid oauth state (MID_GGL_043)")
	}
	code := req.FormValue("code")
	if code == "" {
		error_msg := "code not found in request (SHD_GGL_060)"
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_AuthFailure,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GGL_068"})
		return c.String(http.StatusBadRequest, error_msg)
	}

	userInfo, err := getUserInfo(req.Context(), code)
	if err != nil {
		error_msg := fmt.Sprintf("failed to get user info: %v (MID_GGL_055)", err)
		log.Printf("***** Alarm %s", error_msg)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_AuthFailure,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GGL_068"})
		return c.String(http.StatusInternalServerError, "failed to get user info (MID_GGL_054): "+err.Error())
	}

	// Generate a secure random session ID
	sessionID := generateSecureToken(32) // e.g., 256-bit random string

	// Save session in DB/cache: map sessionID → user_email (or user_id)
	expired_time := time.Now().Add(cookie_timeout_hours*time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)
	err1 := sysdatastores.SaveSession(
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
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_DatabaseError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GGL_113"})
		return c.String(http.StatusInternalServerError, "failed to save session (MID_GGL_068): "+err1.Error())
	}

	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:	"google_login",
    	SessionID:		sessionID,
		Status:			"active",
    	UserName:		userInfo.Email,
    	UserNameType:	"email",
    	UserRegID:		userInfo.Email,
    	UserEmail:		&userInfo.Email,
		CallerLoc:		"SHD_GGL_123",
    	ExpiresAt:		&expired_time_str,
	})

	if !userInfo.VerifiedEmail {
		error_msg := fmt.Sprintf("***** Alarm Unverified email login attempt, email:%s (MID_GGL_118)", userInfo.Email)
		c.String(http.StatusUnauthorized, error_msg)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_UnverifiedEmail,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GGL_126"})
		return nil
	}

	// Save the user
	user_record := datastructures.UserInfo{
		UserID:    	 databaseutil.StrPtr(userInfo.ID),
		UserName:    userInfo.Email,
		UserIdType:  "email",
		Email:       userInfo.Email,
		UserType:    "google",
		Status:      "active",
		FirstName:   databaseutil.StrPtr(userInfo.GivenName),
		LastName:    databaseutil.StrPtr(userInfo.FamilyName),
		Picture:     databaseutil.StrPtr(userInfo.Picture),
		Locale:      databaseutil.StrPtr(userInfo.Locale),
	}
	status, err1 := sysdatastores.AddUserNew(user_record)
	if !status {
		error_msg := fmt.Sprintf("***** Alarm Failed creating user, email:%s, err:%s (SHD_GGL_125)", userInfo.Email, err1)
		log.Printf("***** Alarm %s", error_msg)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_DatabaseError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GGL_155"})
		return c.String(http.StatusInternalServerError, error_msg)
	} else {
		msg := fmt.Sprintf("User registered, email:%s, name:%s %s, picture:%s, locale:%s",
					userInfo.Email,
					userInfo.GivenName,
					userInfo.FamilyName,
					userInfo.Picture,
					userInfo.Locale)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_UserCreated,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg: 		&msg,
			CallerLoc: 			"SHD_GGL_172"})
	}

	// Generate a cookie.
	is_secure := authmiddleware.IsSecure()
	cookie := new(http.Cookie)
	cookie.Name = "session_id"
	cookie.Value = sessionID 
	cookie.Path  = "/" 
	cookie.HttpOnly = true 
	cookie.Secure = is_secure
	cookie.SameSite = http.SameSiteStrictMode
	c.SetCookie(cookie)

	msg1 := fmt.Sprintf("Set cookie, session_id:%s, HttpOnly:true, Secure:%t", sessionID, is_secure)
	log.Printf("%s (SHD_GGL_197)", msg1)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_SetCookie,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg: 		&msg1,
			CallerLoc: 			"SHD_GGL_179"})

	// Construct redirect URL
	redirect_url := ApiTypes.DatabaseInfo.HomeURL
	if redirect_url == "" {
		log.Printf("***** Alarm missing home_url config (MID_GGL_094)")
		redirect_url = "localhost:5173"
	}

	// Redirect to the home URL
	redirectURL := fmt.Sprintf("%s?name=%s", redirect_url, url.QueryEscape(userInfo.Name))

	msg2 := fmt.Sprintf("User %s (%s) logged in successfully, redirect to:%s",
	 	userInfo.Name, userInfo.Email, redirectURL)
	log.Printf("%s (SHD_GGL_217)", msg2)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_Auth,
			ActivityType: 		ApiTypes.ActivityType_AuthSuccess,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GoogleAuth,
			ActivityMsg: 		&msg2,
			CallerLoc: 			"SHD_GGL_201"})

	return c.Redirect(http.StatusSeeOther, redirectURL)
}


func generateSecureToken(length int) string {
    bytes := make([]byte, length)
    if _, err := rand.Read(bytes); err != nil {
        panic(err)
    }
    return hex.EncodeToString(bytes)
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
	token, err := googleOauthConfig.Exchange(ctx, code)
	if err != nil {
		error_msg := fmt.Errorf("code exchange failed (MID_GGL_122): %w", err)
		log.Printf("***** Alarm Failed Google auth, err:%s", error_msg.Error())
		return nil, error_msg
	}

	// Use oauth2 client（will automatically attach access token）
	client := googleOauthConfig.Client(ctx, token)
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
