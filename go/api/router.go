package api

import (
	"log"

	requesthandlers "github.com/chendingplano/shared/go/api/RequestHandlers"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
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
	e.GET("/auth/email/reset", auth.HandleResetLink)      // user clicks link in email

	log.Println("Register /auth/email/confirm route (SHD_RTR_061)")
	e.POST("/auth/email/reset/confirm", auth.HandleResetPasswordConfirm) // user submits new password

	log.Println("Register /shared_api/v1/add_prompt (SHD_RTR_067)")
	e.POST("/shared_api/v1/add_prompt", sysdatastores.AddPromptFromFrontend)

	log.Println("Register /shared_api/v1/jimo_req (SHD_RTR_062)")
	e.POST("/shared_api/v1/jimo_req", requesthandlers.HandleJimoRequest)
}