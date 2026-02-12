package api

import (
	"os"

	"github.com/chendingplano/shared/go/api/RequestHandlers"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// RegisterCSRFMiddleware adds CSRF protection to state-changing routes.
// Call this after RegisterRoutes to protect POST/PUT/DELETE endpoints.
// Note: OAuth callbacks and login endpoints are excluded as they have
// their own CSRF protection via state parameter and origin validation.
func RegisterCSRFMiddleware(e *echo.Echo) {
	e.Use(auth.CSRFMiddleware)
}

func RegisterRoutes(e *echo.Echo) {
	var logger = loggerutil.CreateDefaultLogger("SHD_RTR_020")

	// Check if Kratos authentication is enabled
	useKratos := os.Getenv("AUTH_USE_KRATOS") == "true"

	logger.Info("Register /auth/google/login route", "use_kratos", useKratos)
	if useKratos {
		// Use Kratos OIDC for Google login - Kratos handles the full OAuth flow
		e.GET("/auth/google/login", func(c echo.Context) error {
			return auth.HandleGoogleLoginKratos(c)
		})
		// Note: With Kratos OIDC, the callback is handled by Kratos internally
		// at /self-service/methods/oidc/callback/google
		// We still register the legacy callback route for backwards compatibility
		e.GET("/auth/google/callback", func(c echo.Context) error {
			return auth.HandleGoogleCallback(c)
		})
	} else {
		e.GET("/auth/google/login", func(c echo.Context) error {
			return auth.HandleGoogleLogin(c)
		})
		e.GET("/auth/google/callback", func(c echo.Context) error {
			return auth.HandleGoogleCallback(c)
		})
	}

	logger.Info("Register /auth/github/login route")
	e.GET("/auth/github/login", func(c echo.Context) error {
		return auth.HandleGitHubLogin(c)
	})

	logger.Info("Register /auth/github/callback route")
	e.GET("/auth/github/callback", func(c echo.Context) error {
		return auth.HandleGitHubCallback(c)
	})

	logger.Info("Register /auth/email/login route", "use_kratos", useKratos)
	if useKratos {
		e.POST("/auth/email/login", func(c echo.Context) error {
			return auth.HandleEmailLoginKratos(c)
		})
	} else {
		e.POST("/auth/email/login", func(c echo.Context) error {
			return auth.HandleEmailLogin(c)
		})
	}

	logger.Info("Register /auth/me route", "use_kratos", useKratos)
	if useKratos {
		e.GET("/auth/me", func(c echo.Context) error {
			return auth.HandleAuthMeKratos(c)
		})
	} else {
		e.GET("/auth/me", func(c echo.Context) error {
			return auth.HandleAuthMe(c)
		})
	}

	// Register logout endpoint (Kratos-only, as existing logout is handled differently)
	if useKratos {
		logger.Info("Register /auth/logout route (Kratos)")
		e.POST("/auth/logout", func(c echo.Context) error {
			return auth.HandleLogoutKratos(c)
		})

		// Register TOTP verification endpoint for 2FA
		logger.Info("Register /auth/totp/verify route (Kratos)")
		e.POST("/auth/totp/verify", func(c echo.Context) error {
			return auth.HandleTOTPVerifyKratos(c)
		})
	}

	logger.Info("Register /auth/email/signup route", "use_kratos", useKratos)
	if useKratos {
		e.POST("/auth/email/signup", func(c echo.Context) error {
			return auth.HandleEmailSignupKratos(c)
		})
	} else {
		e.POST("/auth/email/signup", func(c echo.Context) error {
			return auth.HandleEmailSignup(c)
		})
	}

	logger.Info("Register /auth/email/reset/confirm route", "use_kratos", useKratos)
	if useKratos {
		e.POST("/auth/email/reset/confirm", func(c echo.Context) error {
			return auth.HandleResetPasswordConfirmKratos(c)
		})
	} else {
		e.POST("/auth/email/reset/confirm", auth.HandleResetPasswordConfirm) // user submits new password
	}

	logger.Info("Register /auth/email/verify route")
	e.POST("/auth/email/verify", auth.HandleEmailVerifyPost)

	logger.Info("Register /auth/email/verify route")
	e.GET("/auth/email/verify", auth.HandleEmailVerify)

	logger.Info("Register /auth/email/forgot route", "use_kratos", useKratos)
	if useKratos {
		e.POST("/auth/email/forgot", func(c echo.Context) error {
			return auth.HandleForgotPasswordKratos(c)
		})
	} else {
		e.POST("/auth/email/forgot", auth.HandleForgotPassword)
	}

	logger.Info("Register /auth/email/reset route", "use_kratos", useKratos)
	if useKratos {
		e.GET("/auth/email/reset", func(c echo.Context) error {
			return auth.HandleResetLinkKratos(c)
		})
	} else {
		e.GET("/auth/email/reset", auth.HandleResetLink) // user clicks link in email
	}

	logger.Info("Register /shared_api/v1/jimo_req")
	e.POST("/shared_api/v1/jimo_req", RequestHandlers.HandleJimoRequestEcho)

	// Icon service routes
	logger.Info("Register /shared_api/v1/icons routes")
	e.GET("/shared_api/v1/icons", RequestHandlers.HandleListIcons)
	e.GET("/shared_api/v1/icons/categories", RequestHandlers.HandleGetCategories)
	e.GET("/shared_api/v1/icons/:id", RequestHandlers.HandleGetIcon)
	e.POST("/shared_api/v1/icons", RequestHandlers.HandleUploadIcon)
	e.DELETE("/shared_api/v1/icons/:id", RequestHandlers.HandleDeleteIcon)
	e.GET("/shared_api/v1/icons/file/:category/:filename", RequestHandlers.HandleServeIconFile)
}
