package Auth

import (
	"log"

	"github.com/labstack/echo/v4"
)

func RegisterRoutes(e *echo.Echo) {
	log.Println("Registering Auth routes (MID_RTR_010)")
	e.GET("/auth/google/login", func(c echo.Context) error {
		HandleGoogleLogin(c.Response(), c.Request())
		return nil
	})

	e.GET("/auth/google/callback", func(c echo.Context) error {
		HandleGoogleCallback(c.Response(), c.Request())
		return nil
	})

	e.GET("/auth/github/login", func(c echo.Context) error {
		HandleGitHubLogin(c.Response(), c.Request())
		return nil
	})

	e.GET("/auth/github/callback", func(c echo.Context) error {
		HandleGitHubCallback(c.Response(), c.Request())
		return nil
	})

	e.POST("/auth/email/login", func(c echo.Context) error {
		HandleEmailLogin(c.Response(), c.Request())
		return nil
	})

	e.POST("/auth/email/signup", func(c echo.Context) error {
		HandleEmailSignup(c)
		return nil
	})

	e.GET("/auth/email/verify", HandleEmailVerify)

	e.POST("/auth/email/forgot", HandleForgotPassword)
	e.GET("/auth/email/reset", HandleResetLink)      // user clicks link in email
	e.POST("/auth/email/reset/confirm", HandleResetPasswordConfirm) // user submits new password
}