package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/chendingplano/Shared/server/api/databaseutil"
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

func HandleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	log.Printf("Github Login (MID_GHB_025)")
	url := githubOauthConfig.AuthCodeURL(githubOauthStateString)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	log.Printf("Github Login Callback (MID_GHB_032)")
	if r.FormValue("state") != githubOauthStateString {
		err_msg := "invalid oauth state (MID_GHB_034)"
		log.Printf("***** Alarm %s", err_msg)
		http.Error(w, err_msg, http.StatusBadRequest)
		return
	}
	code := r.FormValue("code")
	token, err := githubOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		err_msg := "code exchange failed (MID_GHB_042)"
		log.Printf("***** Alarm %s", err_msg)
		http.Error(w, err_msg, http.StatusInternalServerError)
		return
	}

	client := githubOauthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		err_msg := "failed to get user info (MID_GHB_051)"
		log.Printf("***** Alarm %s", err_msg)
		http.Error(w, err_msg, http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var user struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		err_msg := "failed to decode user info (MID_GHB_059)"
		log.Printf("***** Alarm %s", err_msg)
		http.Error(w, err_msg, http.StatusInternalServerError)
		return
	}

	// Generate a secure random session ID
	sessionID := generateSecureToken(32) // e.g., 256-bit random string

	// Save session in DB/cache: map sessionID → user_email (or user_id)
	err1 := databaseutil.SaveSession(
				AuthInfo.DBType,
				AuthInfo.SessionTableName,
				"github_login",
				sessionID, 
				user.Name,
				"github",
				user.Login,
				time.Now().Add(24*time.Hour))
	if err1 != nil {
		http.Error(w, "failed to save session (MID_GHB_094): "+err1.Error(), http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,   // require HTTPS in production
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("Github login success (MID_GHB_071), login:%s, name:%s, email:%s", user.Login, user.Name, user.Email)
	redirectURL := fmt.Sprintf("http://localhost:8080/sidebar-01?name=%s", user.Login)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}
