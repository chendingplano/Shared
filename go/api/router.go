package api

import (
	"log"

	"github.com/chendingplano/shared/go/api/auth"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func RegisterRoutes(e *echo.Echo) {
	log.Println("Register /auth/google/login route (SHD_RTR_010)")
	e.GET("/auth/google/login", func(c echo.Context) error {
		return auth.HandleGoogleLogin(c)
	})

	log.Println("Register /auth/google/callback route (SHD_RTR_016)")
	e.GET("/auth/google/callback", func(c echo.Context) error {
		return auth.HandleGoogleCallback(c)
	})

	log.Println("Register /auth/github/login route (SHD_RTR_022)")
	e.GET("/auth/github/login", func(c echo.Context) error {
		return auth.HandleGitHubLogin(c)
	})

	log.Println("Register /auth/github/callback route (SHD_RTR_028)")
	e.GET("/auth/github/callback", func(c echo.Context) error {
		return auth.HandleGitHubCallback(c)
	})

	log.Println("Register /auth/email/login route (SHD_RTR_034)")
	e.POST("/auth/email/login", func(c echo.Context) error {
		return auth.HandleEmailLogin(c)
	})

	log.Println("Register /auth/me route (SHD_RTR_040)")
	e.GET("/auth/me", func(c echo.Context) error {
		return auth.HandleAuthMe(c)
	})

	log.Println("Register /auth/email/signup route (SHD_RTR_046)")
	e.POST("/auth/email/signup", func(c echo.Context) error {
		return auth.HandleEmailSignup(c)
	})

	log.Println("Register /auth/email/verify route (SHD_RTR_052)")
	e.POST("/auth/email/verify", auth.HandleEmailVerify)

	log.Println("Register /auth/email/forgot route (SHD_RTR_055)")
	e.POST("/auth/email/forgot", auth.HandleForgotPassword)

	log.Println("Register /auth/email/reset route (SHD_RTR_058)")
	e.GET("/auth/email/reset", auth.HandleResetLink) // user clicks link in email

	log.Println("Register /auth/email/confirm route (SHD_RTR_061)")
	e.POST("/auth/email/reset/confirm", auth.HandleResetPasswordConfirm) // user submits new password

	// log.Println("Register /shared_api/v1/add_prompt (SHD_RTR_067)")
	// e.POST("/shared_api/v1/add_prompt", sysdatastores.AddPromptFromFrontend)

	// log.Println("Register /shared_api/v1/jimo_req (SHD_RTR_062)")
	// e.POST("/shared_api/v1/jimo_req", RequestHandlers.HandleJimoRequest)
}

func RegisterRoutesPocket(router *router.Router[*core.RequestEvent]) {
	log.Println("Register /auth/google/login route (SHD_RTR_068)")
	router.GET("/auth/google/login", func(e *core.RequestEvent) error {
		return auth.HandleGoogleLoginPocketbase(e)
	})

	log.Println("Register /auth/google/callback route (SHD_RTR_073)")
	router.GET("/auth/google/callback", func(e *core.RequestEvent) error {
		return auth.HandleGoogleCallbackPocket(e)
	})

	log.Println("Register /auth/github/login route (SHD_RTR_078)")
	router.GET("/auth/github/login", func(e *core.RequestEvent) error {
		return auth.HandleGitHubLoginPocket(e)
	})

	log.Println("Register /auth/github/callback route (SHD_RTR_083)")
	router.GET("/auth/github/callback", func(e *core.RequestEvent) error {
		return auth.HandleGitHubCallbackPocket(e)
	})

	log.Println("Register /auth/email/login route (SHD_RTR_088)")
	router.POST("/auth/email/login", func(e *core.RequestEvent) error {
		return auth.HandleEmailLoginPocket(e)
	})

	log.Println("Register /auth/me route (SHD_RTR_093)")
	router.GET("/auth/me", func(e *core.RequestEvent) error {
		return auth.HandleAuthMePocket(e)
	})

	log.Println("Register /auth/email/signup route (SHD_RTR_098)")
	router.POST("/auth/email/signup", func(e *core.RequestEvent) error {
		return auth.HandleEmailSignupPocket(e)
	})

	log.Println("Register /auth/email/verify (GET) route (SHD_RTR_103)")
	router.GET("/auth/email/verify", func(e *core.RequestEvent) error {
		return auth.HandleEmailVerifyPocket(e)
	})

	log.Println("Register /auth/email/verify (POST) route (SHD_RTR_108)")
	router.POST("/auth/email/verify", func(e *core.RequestEvent) error {
		return auth.HandleEmailVerifyPocket(e)
	})

	log.Println("Register /auth/email/forgot route (SHD_RTR_113)")
	router.POST("/auth/email/forgot", func(e *core.RequestEvent) error {
		return auth.HandleForgotPasswordPocket(e)
	})

	log.Println("Register /auth/email/reset route (SHD_RTR_117)")
	router.GET("/auth/email/reset", func(e *core.RequestEvent) error {
		return auth.HandleResetLinkPocket(e) // user clicks link in email
	})

	log.Println("Register /auth/email/confirm route (SHD_RTR_118)")
	router.POST("/auth/email/reset/confirm", func(e *core.RequestEvent) error {
		return auth.HandleResetPasswordConfirmPocket(e) // user submits new password
	})
}
