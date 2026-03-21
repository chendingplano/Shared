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

	// IP geolocation service (ip66.dev MMDB)
	e.GET("/shared_api/v1/ipdb/lookup", RequestHandlers.HandleIPLookup)
	e.GET("/shared_api/v1/ipdb/sync/status", RequestHandlers.HandleIPSyncStatus)
	e.POST("/shared_api/v1/ipdb/sync/trigger", RequestHandlers.HandleIPSyncTrigger)

	logger.Info("All routes registered", "use_kratos", useKratos)
}
