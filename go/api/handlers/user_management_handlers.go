package handlers

import (
	"net/http"
	"strconv"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/dinglind/mirai/server/api/appdatastores"
	"github.com/labstack/echo/v4"
)

func HandleListUsers(e echo.Context) error {
	rc := EchoFactory.NewFromEcho(e, "ARX_CLN_013")
	defer rc.Close()
	logger := rc.GetLogger()
	path := e.Path()
	logger.Info("Handle request", "path", path)

	user_info := rc.IsAuthenticated()
	if user_info == nil {
		logger.Error("user not logged")
		return e.JSON(http.StatusUnauthorized, map[string]string{
			"error": "user not logged in",
		})
	}
	if !user_info.IsOwner {
		return e.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Owner access required",
		})
	}

	users, err := auth.KratosListAllIdentities(logger)
	if err != nil {
		logger.Error("Failed to list users", "error", err.Error())
		return e.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch users",
		})
	}

	return e.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"users":  users,
	})
}

// HandleToggleAdmin updates a user's admin status
func HandleToggleAdmin(e echo.Context) error {
	rc := EchoFactory.NewFromEcho(e, "ARX_CLN_013")
	defer rc.Close()
	logger := rc.GetLogger()
	path := e.Path()
	logger.Info("Handle request", "path", path)

	user_info := rc.IsAuthenticated()
	if user_info == nil {
		logger.Warn("user not logged in")
		return e.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Owner access required"})
	}

	if !user_info.IsOwner {
		return e.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Owner access required",
		})
	}

	userId := e.Param("id")
	var body struct {
		Admin bool `json:"admin"`
	}
	if err := e.Bind(&body); err != nil {
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Fetch user from Kratos
	user, err := auth.KratosGetIdentityByID(logger, userId)
	if err != nil {
		return e.JSON(http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
	}

	// Store old value for audit log
	oldValue := strconv.FormatBool(user.Admin)

	// Update admin status in Kratos metadata_public
	if err := auth.KratosUpdateIdentity(logger, userId, auth.KratosIdentityUpdate{
		MetadataPublic: map[string]interface{}{"admin": body.Admin},
	}); err != nil {
		logger.Error("Failed to update admin status", "error", err.Error())
		return e.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update user",
		})
	}

	// Log to audit trail
	actionType := appdatastores.ActionGrantAdmin
	if !body.Admin {
		actionType = appdatastores.ActionRevokeAdmin
	}
	if err := appdatastores.LogUserAction(
		rc,
		user_info.UserId,
		user_info.Email,
		actionType,
		userId,
		user.Email,
		appdatastores.FieldAdmin,
		oldValue,
		strconv.FormatBool(body.Admin),
	); err != nil {
		logger.Warn("Failed to log audit action", "error", err.Error())
		// Don't fail the request if audit logging fails
	}

	return e.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"user": map[string]any{
			"id":       user.UserId,
			"email":    user.Email,
			"admin":    body.Admin,
			"is_owner": user.IsOwner,
		},
	})
}

// HandleToggleOwner updates a user's owner status with validations
func HandleToggleOwner(e echo.Context) error {
	rc := EchoFactory.NewFromEcho(e, "ARX_CLN_013")
	defer rc.Close()
	logger := rc.GetLogger()
	path := e.Path()
	logger.Info("Handle request", "path", path)

	user_info := rc.IsAuthenticated()
	if user_info == nil {
		logger.Warn("user not logged in")
		return e.JSON(http.StatusUnauthorized, map[string]string{
			"error": "user not logged in",
		})
	}

	if !user_info.IsOwner {
		return e.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Owner access required",
		})
	}

	userId := e.Param("id")
	var body struct {
		IsOwner bool `json:"is_owner"`
	}
	if err := e.Bind(&body); err != nil {
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Validation 1: Cannot modify your own owner status
	if userId == rc.GetUserID() {
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "Cannot modify your own owner status",
		})
	}

	// Fetch user from Kratos
	user, err := auth.KratosGetIdentityByID(logger, userId)
	if err != nil {
		return e.JSON(http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
	}

	// Store old value for audit log
	oldValue := strconv.FormatBool(user.IsOwner)

	// Validation 2: Must maintain at least one owner (if removing)
	if !body.IsOwner && user.IsOwner {
		allUsers, countErr := auth.KratosListAllIdentities(logger)
		if countErr != nil {
			return e.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to verify owner count",
			})
		}
		ownerCount := 0
		for _, u := range allUsers {
			if u.IsOwner {
				ownerCount++
			}
		}
		if ownerCount <= 1 {
			return e.JSON(http.StatusBadRequest, map[string]string{
				"error": "Must have at least one owner",
			})
		}
	}

	// Update owner status in Kratos metadata_public
	if err := auth.KratosUpdateIdentity(logger, userId, auth.KratosIdentityUpdate{
		MetadataPublic: map[string]interface{}{"is_owner": body.IsOwner},
	}); err != nil {
		logger.Error("Failed to update owner status", "error", err.Error())
		return e.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update user",
		})
	}

	// Log to audit trail
	actionType := appdatastores.ActionGrantOwner
	if !body.IsOwner {
		actionType = appdatastores.ActionRevokeOwner
	}
	if err := appdatastores.LogUserAction(
		rc,
		user_info.UserId,
		user_info.Email,
		actionType,
		userId,
		user.Email,
		appdatastores.FieldIsOwner,
		oldValue,
		strconv.FormatBool(body.IsOwner),
	); err != nil {
		logger.Warn("Failed to log audit action", "error", err.Error())
	}

	return e.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"user": map[string]any{
			"id":       user.UserId,
			"email":    user.Email,
			"admin":    user.Admin,
			"is_owner": body.IsOwner,
		},
	})
}

// HandleToggleStatus updates a user's active/inactive status
func HandleToggleStatus(e echo.Context) error {
	rc := EchoFactory.NewFromEcho(e, "ARX_CLN_013")
	defer rc.Close()
	logger := rc.GetLogger()
	path := e.Path()
	logger.Info("Handle request", "path", path)

	userId := e.Param("id")
	var body struct {
		Status string `json:"status"` // "active" or "inactive"
	}
	if err := e.Bind(&body); err != nil {
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Validate status value
	status := appdatastores.UserStatus(body.Status)
	if !status.IsValid() {
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "Status must be 'active' or 'inactive'",
		})
	}

	// Cannot deactivate yourself
	if userId == rc.GetUserID() && status == appdatastores.UserStatusInactive {
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "Cannot deactivate your own account",
		})
	}

	// Fetch user from Kratos
	user, err := auth.KratosGetIdentityByID(logger, userId)
	if err != nil {
		return e.JSON(http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
	}

	// Map status to active field (active = true, inactive = false)
	active := status == appdatastores.UserStatusActive
	oldActive := user.UserStatus == string(appdatastores.UserStatusActive)

	// Update identity state in Kratos
	stateStr := string(status)
	if err := auth.KratosUpdateIdentity(logger, userId, auth.KratosIdentityUpdate{
		State: &stateStr,
	}); err != nil {
		logger.Error("Failed to update user status", "error", err.Error())
		return e.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update user",
		})
	}

	// If deactivating, invalidate all user sessions via Kratos Admin API
	if !active && oldActive {
		if err := auth.KratosDeleteIdentitySessions(logger, userId); err != nil {
			logger.Warn("Failed to delete sessions for deactivated user", "user_id", userId, "error", err.Error())
		} else {
			logger.Info("User deactivated - all sessions invalidated", "user_id", userId)
		}
	}

	// Log to audit trail
	actionType := appdatastores.ActionActivateUser
	if !active {
		actionType = appdatastores.ActionDeactivateUser
	}
	if err := appdatastores.LogUserAction(
		rc,
		user.UserId,
		user.Email,
		actionType,
		userId,
		user.Email,
		appdatastores.FieldActive,
		strconv.FormatBool(oldActive),
		strconv.FormatBool(active),
	); err != nil {
		logger.Warn("Failed to log audit action", "error", err.Error())
	}

	return e.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"user": map[string]any{
			"id":          user.UserId,
			"email":       user.Email,
			"user_status": string(status),
		},
	})
}
