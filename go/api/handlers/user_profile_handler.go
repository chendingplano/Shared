package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// UserProfileResponse represents the user profile in API responses
type UserProfileResponse struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Avatar     string `json:"avatar"`
	Admin      bool   `json:"admin"`
	IsOwner    bool   `json:"is_owner"`
	UserName   string `json:"user_name"`
	UserStatus string `json:"user_status"`
}

// userInfoToProfileResponse converts ApiTypes.UserInfo to UserProfileResponse
func userInfoToProfileResponse(u *ApiTypes.UserInfo) UserProfileResponse {
	return UserProfileResponse{
		ID:         u.UserId,
		Email:      u.Email,
		FirstName:  u.FirstName,
		LastName:   u.LastName,
		Avatar:     u.Avatar,
		Admin:      u.Admin,
		IsOwner:    u.IsOwner,
		UserName:   u.UserName,
		UserStatus: u.UserStatus,
	}
}

// getAvatarStoragePath returns the directory path where user avatars are stored.
// The pattern is: <DATA_HOME>/<FILE_DIR>/<user_id>/
// Example: /var/data/uploads/documents/abc123/
func getAvatarStoragePath(userID string) string {
	// Get the base data directory from environment or use default
	dataHome := os.Getenv("DATA_HOME")
	if dataHome == "" {
		dataHome = "uploads"
	}

	fileDir := os.Getenv("FILE_DIR")
	if fileDir == "" {
		fileDir = "documents"
	}

	return filepath.Join(dataHome, fileDir, userID)
}

// HandleUpdateUserProfile handles PUT /api/v1/users/:id/profile
// Updates user's firstName, lastName, and avatar
func HandleUpdateUserProfile(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "ARX_UPH_062")
	defer rc.Close()
	logger := rc.GetLogger()

	// Authenticate the user
	currentUser := rc.IsAuthenticated()
	if currentUser == nil {
		logger.Error("authentication failed")
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
			"loc":   "ARX_UPH_072",
		})
	}

	// Get user ID from path parameter
	userID := c.Param("id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "User ID is required",
			"loc":   "ARX_UPH_081",
		})
	}

	// Users can only update their own profile (unless they're admin or owner)
	if currentUser.UserId != userID && !currentUser.Admin && !currentUser.IsOwner {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "You can only update your own profile",
			"loc":   "ARX_UPH_089",
		})
	}

	// Parse multipart form (max 10MB for avatar)
	if err := c.Request().ParseMultipartForm(10 << 20); err != nil {
		logger.Error("failed to parse multipart form", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Failed to parse form data",
			"loc":   "ARX_UPH_098",
		})
	}

	// Get form values
	firstName := c.FormValue("firstName")
	lastName := c.FormValue("lastName")
	removeAvatar := c.FormValue("removeAvatar") == "true"

	// Validate required fields
	if strings.TrimSpace(firstName) == "" || strings.TrimSpace(lastName) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "First name and last name are required",
			"loc":   "ARX_UPH_111",
		})
	}

	// Get current user info from Kratos to preserve existing avatar if not changed
	existingUser, err := auth.KratosGetIdentityByID(logger, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "User not found",
			"loc":   "ARX_UPH_120",
		})
	}

	avatarFilename := existingUser.Avatar

	// Handle avatar file upload
	file, header, err := c.Request().FormFile("avatar")
	if err == nil {
		defer file.Close()

		// Validate file type
		contentType := header.Header.Get("Content-Type")
		if !isValidImageType(contentType) {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Invalid file type. Only JPEG, PNG, GIF, and WebP images are allowed",
				"loc":   "ARX_UPH_135",
			})
		}

		// Generate unique filename
		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = getExtensionFromContentType(contentType)
		}
		avatarFilename = fmt.Sprintf("avatar_%s_%d%s", uuid.New().String()[:8], time.Now().Unix(), ext)

		// Create storage directory
		storagePath := getAvatarStoragePath(userID)
		if err := os.MkdirAll(storagePath, 0755); err != nil {
			logger.Error("failed to create storage directory", "error", err, "path", storagePath)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to save avatar",
				"loc":   "ARX_UPH_150",
			})
		}

		// Delete old avatar file if exists
		if existingUser.Avatar != "" {
			oldAvatarPath := filepath.Join(storagePath, existingUser.Avatar)
			if err := os.Remove(oldAvatarPath); err != nil && !os.IsNotExist(err) {
				logger.Warn("failed to delete old avatar", "error", err, "path", oldAvatarPath)
			}
		}

		// Save new avatar file
		avatarPath := filepath.Join(storagePath, avatarFilename)
		dst, err := os.Create(avatarPath)
		if err != nil {
			logger.Error("failed to create avatar file", "error", err, "path", avatarPath)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to save avatar",
				"loc":   "ARX_UPH_168",
			})
		}
		defer dst.Close()

		if _, err := io.Copy(dst, file); err != nil {
			logger.Error("failed to write avatar file", "error", err, "path", avatarPath)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to save avatar",
				"loc":   "ARX_UPH_177",
			})
		}

		logger.Info("avatar saved successfully", "path", avatarPath, "user_id", userID)
	} else if removeAvatar {
		// User wants to remove avatar
		if existingUser.Avatar != "" {
			storagePath := getAvatarStoragePath(userID)
			oldAvatarPath := filepath.Join(storagePath, existingUser.Avatar)
			if err := os.Remove(oldAvatarPath); err != nil && !os.IsNotExist(err) {
				logger.Warn("failed to delete avatar", "error", err, "path", oldAvatarPath)
			}
		}
		avatarFilename = ""
	}

	// Update user profile in Kratos (traits for name, metadata_public for avatar)
	if err := auth.KratosUpdateIdentity(logger, userID, auth.KratosIdentityUpdate{
		Traits: map[string]interface{}{
			"name": map[string]interface{}{
				"first": firstName,
				"last":  lastName,
			},
		},
		MetadataPublic: map[string]interface{}{
			"avatar": avatarFilename,
		},
	}); err != nil {
		logger.Error("failed to update user profile", "error", err, "user_id", userID)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update profile",
			"loc":   "ARX_UPH_199",
		})
	}

	// Fetch updated user from Kratos for the response
	updatedUser, err := auth.KratosGetIdentityByID(logger, userID)
	if err != nil {
		logger.Error("failed to fetch updated user", "error", err, "user_id", userID)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Profile updated but failed to fetch result",
			"loc":   "ARX_UPH_210",
		})
	}

	logger.Info("user profile updated successfully",
		"user_id", userID,
		"first_name", firstName,
		"last_name", lastName,
		"avatar", avatarFilename)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"user":   userInfoToProfileResponse(updatedUser),
	})
}

// isValidImageType checks if the content type is a valid image type
func isValidImageType(contentType string) bool {
	validTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}
	return validTypes[contentType]
}

// getExtensionFromContentType returns file extension based on content type
func getExtensionFromContentType(contentType string) string {
	extensions := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
		"image/webp": ".webp",
	}
	if ext, ok := extensions[contentType]; ok {
		return ext
	}
	return ".jpg" // default
}
