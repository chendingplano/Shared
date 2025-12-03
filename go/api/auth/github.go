package auth

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
	"github.com/chendingplano/shared/go/api/sysdatastores"
	authmiddleware "github.com/chendingplano/shared/go/auth-middleware"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var githubOauthConfig = &oauth2.Config{
	ClientID:     os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
	ClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
	RedirectURL:  "http://localhost:8080/auth/github/callback",
	Scopes:       []string{"user:email"},
	Endpoint:     github.Endpoint,
}

var githubOauthStateString = "random-github-state"

func HandleGitHubLogin(c echo.Context) error {
	url := githubOauthConfig.AuthCodeURL(githubOauthStateString)
	msg := fmt.Sprintf("Github Login, url:%s", url)
	log.Printf("%s (SHD_GHB_032)", msg)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: 		ApiTypes.ActivityName_Auth,
		ActivityType: 		ApiTypes.ActivityType_GitHubAuth,
		AppName: 			ApiTypes.AppName_Auth,
		ModuleName: 		ApiTypes.ModuleName_GitHubAuth,
		ActivityMsg: 		&msg,
		CallerLoc: 			"SHD_GHB_041"})

	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func HandleGitHubCallback(c echo.Context) error {
	log.Printf("Github Login Callback (MID_GHB_032)")
	req := c.Request()
	state := req.FormValue("state")
	if state != githubOauthStateString {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid oauth state:%s, log_id:%d (MID_GHB_034)", state, log_id)
		log.Printf("***** Alarm %s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.ActivityName_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GHB_061"})

		return c.String(http.StatusBadRequest, error_msg)
	}
	code := req.FormValue("code")
	token, err := githubOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("code exchange failed, code:%s, log_id:%d (MID_GHB_042)", code, log_id)
		log.Printf("***** Alarm %s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.ActivityName_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GHB_080"})

		return c.String(http.StatusInternalServerError, error_msg)
	}

	client := githubOauthConfig.Client(context.Background(), token)
	rr, err := client.Get("https://api.github.com/user")
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to get user info, log_id:%d (MID_GHB_051)", log_id)
		log.Printf("***** Alarm %s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.ActivityName_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GHB_080"})

		return c.String(http.StatusInternalServerError, error_msg)
	}
	defer rr.Body.Close()

	var user_info struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := json.NewDecoder(rr.Body).Decode(&user_info); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to decode user info, log_id:%d (MID_GHB_059)", log_id)
		log.Printf("***** Alarm %s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.ActivityName_Auth,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GHB_125"})

		return c.String(http.StatusInternalServerError, error_msg)
	}

	// Generate a secure random session ID
	sessionID := generateSecureToken(32) // e.g., 256-bit random string

	expired_time := time.Now().Add(cookie_timeout_hours*time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)
	err1 := sysdatastores.SaveSession(
				"github_login",
				sessionID, 
				user_info.Name,
				"github",
				user_info.Login,
				user_info.Email,
				expired_time)
	if err1 != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to save session, error:%v, log_id:%d (MID_GHB_094)", err1, log_id)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:				log_id,
			ActivityName: 		ApiTypes.ActivityName_Auth,
			ActivityType: 		ApiTypes.ActivityType_DatabaseError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GHB_154"})

		return c.String(http.StatusInternalServerError, error_msg)
	}

	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:	"github_login",
    	SessionID:		sessionID,
		Status:			"active",
    	UserName:		user_info.Email,
    	UserNameType:	"email",
    	UserRegID:		user_info.Email,
    	UserEmail:		&user_info.Email,
		CallerLoc:		"SHD_GHB_171",
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
		error_msg := fmt.Sprintf("missing home_url config, default to:%s", redirect_url)
		log.Printf("***** Alarm:%s (MID_GHB_104)", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.ActivityName_Auth,
			ActivityType: 		ApiTypes.ActivityType_ConfigError,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_GHB_182"})
	}

	// Redirect to the home URL
	redirectURL := fmt.Sprintf("%s?name=%s", redirect_url, url.QueryEscape(user_info.Name))

	msg := fmt.Sprintf("User %s (%s) logged in successfully, set cookie:%s, redirect to:%s",
	 			user_info.Name, user_info.Email, sessionID, redirectURL)
	log.Printf("%s (SHD_GHB_129)", msg)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: 		ApiTypes.ActivityName_Auth,
		ActivityType: 		ApiTypes.ActivityType_AuthSuccess,
		AppName: 			ApiTypes.AppName_Auth,
		ModuleName: 		ApiTypes.ModuleName_GitHubAuth,
		ActivityMsg: 		&msg,
		CallerLoc: 			"SHD_GHB_198"})

	return c.String(http.StatusSeeOther, redirectURL)
}
