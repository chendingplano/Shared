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
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var domain_name = os.Getenv("APP_DOMAIN_NAME")
var githubOauthConfig = &oauth2.Config{
	ClientID:     os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
	ClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
	RedirectURL:  domain_name + "/auth/github/callback",
	Scopes:       []string{"user:email"},
	Endpoint:     github.Endpoint,
}

var githubOauthStateString = "random-github-state"

func HandleGitHubLogin(c echo.Context) error {
	url := githubOauthConfig.AuthCodeURL(githubOauthStateString)
	msg := fmt.Sprintf("Github Login, url:%s", url)
	log.Printf("%s (SHD_GHB_032)", msg)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_GitHubAuth,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_GitHubAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_GHB_041"})

	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func HandleGitHubLoginPocket(e echo.Context) error {
	rc := EchoFactory.NewFromEcho(e, "ARX_GHB_050")
	defer rc.Close()
	logger := rc.GetLogger()
	path := e.Path()
	logger.Info("Handle request", "path", path)

	url := githubOauthConfig.AuthCodeURL(githubOauthStateString)
	msg := fmt.Sprintf("Github Login, url:%s", url)
	log.Printf("%s (SHD_GHB_032)", msg)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_GitHubAuth,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_GitHubAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_GHB_041"})

	http.Redirect(e.Response(), e.Request(), url, http.StatusTemporaryRedirect)
	return nil
}

func HandleGitHubCallback(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_GHB_068")
	reqID := rc.ReqID()
	status_code, msg := HandleGitHubCallbackBase(rc, reqID)
	c.String(status_code, msg)
	return nil
}

/*
func HandleGitHubCallbackPocket(e *core.RequestEvent) error {
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()
	status_code, msg := HandleGitHubCallbackBase(rc, reqID)
	e.String(status_code, msg)
	return nil
}
*/

func HandleGitHubCallbackBase(
	rc ApiTypes.RequestContext,
	reqID string) (int, string) {
	log.Printf("Github Login Callback (MID_GHB_032)")
	state := rc.FormValue("state")
	if state != githubOauthStateString {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid oauth state:%s, log_id:%d (MID_GHB_034)", state, log_id)
		log.Printf("***** Alarm %s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GHB_061"})

		return http.StatusBadRequest, error_msg
	}
	code := rc.FormValue("code")
	token, err := githubOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("code exchange failed, code:%s, log_id:%d (MID_GHB_042)", code, log_id)
		log.Printf("***** Alarm %s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GHB_080"})

		return http.StatusInternalServerError, error_msg
	}

	client := githubOauthConfig.Client(context.Background(), token)
	rr, err := client.Get("https://api.github.com/user")
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to get user info, log_id:%d (MID_GHB_051)", log_id)
		log.Printf("***** Alarm %s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GHB_080"})

		return http.StatusInternalServerError, error_msg
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
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GHB_125"})

		return http.StatusInternalServerError, error_msg
	}

	// Generate a secure random session ID
	sessionID := ApiUtils.GenerateSecureToken(32) // e.g., 256-bit random string

	expired_time := time.Now().Add(cookie_timeout_hours * time.Hour)
	customLayout := "2006-01-02 15:04:05"
	expired_time_str := expired_time.Format(customLayout)
	authToken, err := rc.GenerateAuthToken(user_info.Email)
	err1 := sysdatastores.SaveSession(
		rc,
		"github_login",
		sessionID,
		authToken,
		user_info.Name,
		"github",
		user_info.Login,
		user_info.Email,
		expired_time,
		true)
	if err1 != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed to save session, error:%v, log_id:%d (MID_GHB_094)", err1, log_id)
		log.Printf("***** Alarm:%s", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GHB_154"})

		return http.StatusInternalServerError, error_msg
	}

	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:  "github_login",
		SessionID:    sessionID,
		Status:       "active",
		UserName:     user_info.Email,
		UserNameType: "email",
		UserRegID:    user_info.Email,
		UserEmail:    &user_info.Email,
		CallerLoc:    "SHD_GHB_171",
		ExpiresAt:    &expired_time_str,
	})

	rc.SetCookie(sessionID)

	// Construct redirect URL
	redirect_url := ApiUtils.GetDefahotHomeURL()
	if redirect_url == "" {
		error_msg := "missing home_url config"
		log.Printf("***** Alarm:%s (MID_GHB_104)", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_ConfigError,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_GitHubAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_GHB_182"})
	}

	// Redirect to the home URL
	redirectURL := fmt.Sprintf("%s?name=%s", redirect_url, url.QueryEscape(user_info.Name))

	msg := fmt.Sprintf("User %s (%s) logged in successfully, set cookie:%s, redirect to:%s",
		user_info.Name, user_info.Email, sessionID, redirectURL)
	log.Printf("%s (SHD_GHB_129)", msg)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Auth,
		ActivityType: ApiTypes.ActivityType_AuthSuccess,
		AppName:      ApiTypes.AppName_Auth,
		ModuleName:   ApiTypes.ModuleName_GitHubAuth,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_GHB_198"})

	return http.StatusSeeOther, redirectURL
}
