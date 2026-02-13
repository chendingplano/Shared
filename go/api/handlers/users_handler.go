package handlers

import (
	"net/http"
	"strconv"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/dinglind/mirai/server/api/appdatastores"
	"github.com/labstack/echo/v4"
)

// HandleGetAdminUsers returns all admin users for the consultant selector
func HandleGetAdminUsers(e echo.Context) error {
	rc := EchoFactory.NewFromEcho(e, "ARX_USH_020")
	defer rc.Close()
	logger := rc.GetLogger()
	path := e.Path()
	logger.Info("Handle request", "path", path)

	// Retrieve the query parm 'is_admin'
	isAdminStr := e.QueryParam("is_admin") // "true" or "false"
	if isAdminStr == "" {
		logger.Error("missing 'is_admin'")
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "missing 'is_admin'",
			"loc":   "ARX_USH_028",
		})
	}
	isAdmin, err := strconv.ParseBool(isAdminStr)
	if err != nil {
		logger.Error("invalid 'is_admin': must be 'true' or 'false'", "is_admin", isAdminStr)
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "is_admin must be true or false",
			"loc":   "ARX_USH_037",
		})
	}

	// Get all identities from Kratos and filter by admin flag
	allUsers, err := auth.KratosListAllIdentities(logger)
	if err != nil {
		logger.Error("failed retrieving identities", "error", err, "is_admin", isAdminStr)
		return e.JSON(http.StatusInternalServerError, map[string]string{
			"status": "error",
			"loc":    "USR_GAU_010",
			"error":  "Failed to retrieve admin users",
		})
	}

	// Filter by admin flag
	users := make([]*ApiTypes.UserInfo, 0)
	for _, user := range allUsers {
		if user.Admin == isAdmin {
			users = append(users, user)
		}
	}

	// Check whether it is for consultant
	forConsultantStr := e.QueryParam("consultant") // "true" or "false"
	logger.Info("Handle get admin users", "is_admin", isAdminStr, "consultant", forConsultantStr)

	if forConsultantStr != "" {
		forConsultant, err := strconv.ParseBool(forConsultantStr)
		if err != nil {
			logger.Error("invalid 'consultant' parm: must be 'true' or 'false'", "consultant", forConsultantStr)
			return e.JSON(http.StatusBadRequest, map[string]string{
				"error": "'consultant' param must be true or false",
				"loc":   "ARX_USH_059",
			})
		}

		if forConsultant {
			// Get existing consultant user IDs to filter them out
			existingUserIDs, err := appdatastores.GetAllConsultantUserIDs(rc)
			if err != nil {
				logger.Error("failed check user IDs", "error", err, "is_admin", isAdminStr)
				return e.JSON(http.StatusInternalServerError, map[string]string{
					"status": "error",
					"loc":    "USR_GAU_015",
					"error":  "Failed to check existing consultants",
				})
			}

			// Create a set for quick lookup
			existingSet := make(map[string]bool)
			for _, id := range existingUserIDs {
				existingSet[id] = true
			}

			// Build response, filtering out users who are already consultants
			logger.Info("Handle get admin users")
			response := make([]ApiTypes.UserInfo, 0, len(users))
			for _, user := range users {
				userID := user.UserId
				// Skip users who are already consultants
				if existingSet[userID] {
					continue
				}

				response = append(response, *user)
			}

			logger.Info("Retrieved users", "total", len(response), "is_admin", isAdminStr)
			return e.JSON(http.StatusOK, map[string]interface{}{
				"status": "ok",
				"loc":    "USR_GAU_089",
				"users":  response,
			})
		}
	}

	logger.Info("Retrieved users", "total", len(users), "is_admin", isAdminStr)
	return e.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"loc":    "USR_GAU_089",
		"users":  users,
	})
}

// HandleGetUser returns a single user by ID
// Used for fetching author details in client notes, etc.
func HandleGetUser(e echo.Context) error {
	rc := EchoFactory.NewFromEcho(e, "ARX_USH_085")
	defer rc.Close()
	logger := rc.GetLogger()

	// Authenticate the user (only admins can fetch user details)
	currentUser := rc.IsAuthenticated()
	if currentUser == nil {
		logger.Warn("user not authenticated")
		return e.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
			"loc":   "ARX_USH_093",
		})
	}

	if !currentUser.Admin && !currentUser.IsOwner {
		return e.JSON(http.StatusForbidden, map[string]string{
			"error": "Admin access required",
			"loc":   "ARX_USH_100",
		})
	}

	// Get user ID from path parameter
	userID := e.Param("id")
	if userID == "" {
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "User ID is required",
			"loc":   "ARX_USH_108",
		})
	}

	// Fetch user from Kratos
	user, err := auth.KratosGetIdentityByID(logger, userID)
	if err != nil {
		logger.Warn("user not found", "user_id", userID, "error", err)
		return e.JSON(http.StatusNotFound, map[string]string{
			"error": "User not found",
			"loc":   "ARX_USH_118",
		})
	}

	logger.Info("User fetched successfully", "user_id", userID)
	return e.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"user":   user,
	})
}
