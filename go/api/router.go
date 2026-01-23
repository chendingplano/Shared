package api

import (
	"github.com/chendingplano/shared/go/api/RequestHandlers"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var logger = loggerutil.CreateDefaultLogger()

func RegisterRoutes(e *echo.Echo) {
	logger.Info("Register /auth/google/login route")
	e.GET("/auth/google/login", func(c echo.Context) error {
		return auth.HandleGoogleLogin(c)
	})

	logger.Info("Register /auth/google/callback route")
	e.GET("/auth/google/callback", func(c echo.Context) error {
		return auth.HandleGoogleCallback(c)
	})

	logger.Info("Register /auth/github/login route")
	e.GET("/auth/github/login", func(c echo.Context) error {
		return auth.HandleGitHubLogin(c)
	})

	logger.Info("Register /auth/github/callback route")
	e.GET("/auth/github/callback", func(c echo.Context) error {
		return auth.HandleGitHubCallback(c)
	})

	logger.Info("Register /auth/email/login route")
	e.POST("/auth/email/login", func(c echo.Context) error {
		return auth.HandleEmailLogin(c)
	})

	logger.Info("Register /auth/me route")
	e.GET("/auth/me", func(c echo.Context) error {
		return auth.HandleAuthMe(c)
	})

	logger.Info("Register /auth/email/signup route")
	e.POST("/auth/email/signup", func(c echo.Context) error {
		return auth.HandleEmailSignup(c)
	})

	logger.Info("Register /auth/email/verify route")
	e.POST("/auth/email/verify", auth.HandleEmailVerifyPost)

	logger.Info("Register /auth/email/verify route")
	e.GET("/auth/email/verify", auth.HandleEmailVerify)

	logger.Info("Register /auth/email/forgot route")
	e.POST("/auth/email/forgot", auth.HandleForgotPassword)

	logger.Info("Register /auth/email/reset route")
	e.GET("/auth/email/reset", auth.HandleResetLink) // user clicks link in email

	logger.Info("Register /auth/email/confirm route")
	e.POST("/auth/email/reset/confirm", auth.HandleResetPasswordConfirm) // user submits new password

	logger.Info("Register /shared_api/v1/jimo_req")
	e.POST("/shared_api/v1/jimo_req", RequestHandlers.HandleJimoRequestEcho)
}
