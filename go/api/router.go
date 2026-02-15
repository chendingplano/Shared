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
	logger := loggerutil.CreateDefaultLogger("SHD_RTR_020")
	useKratos := os.Getenv("AUTH_USE_KRATOS") == "true"

	// Google OAuth
	googleLogin := echo.HandlerFunc(auth.HandleGoogleLogin)
	if useKratos {
		googleLogin = auth.HandleGoogleLoginKratos
	}
	e.GET("/auth/google/login", googleLogin)
	e.GET("/auth/google/callback", auth.HandleGoogleCallback)

	// GitHub OAuth
	e.GET("/auth/github/login", auth.HandleGitHubLogin)
	e.GET("/auth/github/callback", auth.HandleGitHubCallback)

	// Email auth
	emailLogin := echo.HandlerFunc(auth.HandleEmailLogin)
	emailSignup := echo.HandlerFunc(auth.HandleEmailSignup)
	authMe := echo.HandlerFunc(auth.HandleAuthMe)
	if useKratos {
		emailLogin = auth.HandleEmailLoginKratos
		emailSignup = auth.HandleEmailSignupKratos
		authMe = auth.HandleAuthMeKratos
	}
	e.POST("/auth/email/login", emailLogin)
	e.POST("/auth/email/signup", emailSignup)
	e.GET("/auth/me", authMe)

	// Email verification (shared across both auth modes)
	e.POST("/auth/email/verify", auth.HandleEmailVerifyPost)
	e.GET("/auth/email/verify", auth.HandleEmailVerify)

	// Kratos-only routes
	if useKratos {
		e.POST("/auth/logout", auth.HandleLogoutKratos)
		e.POST("/auth/totp/verify", auth.HandleTOTPVerifyKratos)
		e.GET("/auth/verification/flow", auth.HandleVerificationFlowKratos)
		e.POST("/auth/verification", auth.HandleVerificationSubmitKratos)
		e.POST("/auth/recovery", auth.HandleRecoverySubmitKratos)
		e.GET("/auth/recovery/settings", auth.HandleSettingsFlowKratos)
		e.POST("/auth/recovery/settings", auth.HandleSettingsSubmitKratos)
	}

	// Shared API
	e.POST("/shared_api/v1/jimo_req", RequestHandlers.HandleJimoRequestEcho)

	// Icon service
	e.GET("/shared_api/v1/icons", RequestHandlers.HandleListIcons)
	e.GET("/shared_api/v1/icons/categories", RequestHandlers.HandleGetCategories)
	e.GET("/shared_api/v1/icons/:id", RequestHandlers.HandleGetIcon)
	e.POST("/shared_api/v1/icons", RequestHandlers.HandleUploadIcon)
	e.DELETE("/shared_api/v1/icons/:id", RequestHandlers.HandleDeleteIcon)
	e.GET("/shared_api/v1/icons/file/:category/:filename", RequestHandlers.HandleServeIconFile)

	logger.Info("All routes registered", "use_kratos", useKratos)
}
