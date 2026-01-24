package RequestHandlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
)

// HandleListIcons handles GET /shared_api/v1/icons
func HandleListIcons(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_ICH_016")
	defer rc.Close()
	log := rc.GetLogger()

	// Check authentication
	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Authentication required",
			Loc:      "SHD_ICH_025",
		})
	}

	// Parse query parameters
	category := c.QueryParam("category")
	search := c.QueryParam("search")
	pageStr := c.QueryParam("page")
	pageSizeStr := c.QueryParam("page_size")

	page := 0
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p >= 0 {
			page = p
		}
	}

	pageSize := 50
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	req := ApiTypes.IconListRequest{
		Category: category,
		Search:   search,
		Page:     page,
		PageSize: pageSize,
	}

	icons, total, err := sysdatastores.ListIcons(rc, req)
	if err != nil {
		log.Error("failed to list icons", "error", err)
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Failed to list icons",
			Loc:      "SHD_ICH_060",
		})
	}

	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{
		Status:     true,
		ResultType: "json_array",
		NumRecords: total,
		Results:    icons,
		Loc:        "SHD_ICH_068",
	})
}

// HandleGetIcon handles GET /shared_api/v1/icons/:id
func HandleGetIcon(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_ICH_074")
	defer rc.Close()
	log := rc.GetLogger()

	// Check authentication
	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Authentication required",
			Loc:      "SHD_ICH_083",
		})
	}

	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Icon ID is required",
			Loc:      "SHD_ICH_092",
		})
	}

	icon, err := sysdatastores.GetIconByID(rc, id)
	if err != nil {
		log.Error("failed to get icon", "error", err, "id", id)
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Failed to get icon",
			Loc:      "SHD_ICH_102",
		})
	}

	if icon == nil {
		return c.JSON(http.StatusNotFound, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Icon not found",
			Loc:      "SHD_ICH_110",
		})
	}

	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{
		Status:     true,
		ResultType: "json",
		NumRecords: 1,
		Results:    icon,
		Loc:        "SHD_ICH_119",
	})
}

// HandleUploadIcon handles POST /shared_api/v1/icons (multipart/form-data)
func HandleUploadIcon(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_ICH_125")
	defer rc.Close()
	log := rc.GetLogger()

	// Check authentication and admin status
	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Authentication required",
			Loc:      "SHD_ICH_134",
		})
	}

	if !userInfo.Admin {
		return c.JSON(http.StatusForbidden, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Admin access required",
			Loc:      "SHD_ICH_142",
		})
	}

	// Parse multipart form (max 5MB for icons)
	if err := c.Request().ParseMultipartForm(5 << 20); err != nil {
		log.Error("failed to parse multipart form", "error", err)
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Failed to parse form data",
			Loc:      "SHD_ICH_152",
		})
	}

	// Get file
	file, header, err := c.Request().FormFile("file")
	if err != nil {
		log.Error("failed to get file from form", "error", err)
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "File is required",
			Loc:      "SHD_ICH_163",
		})
	}
	defer file.Close()

	// Get metadata
	name := c.FormValue("name")
	category := c.FormValue("category")
	tagsJSON := c.FormValue("tags")
	description := c.FormValue("description")

	if name == "" {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Name is required",
			Loc:      "SHD_ICH_178",
		})
	}

	if category == "" {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Category is required",
			Loc:      "SHD_ICH_186",
		})
	}

	// Parse tags
	var tags []string
	if tagsJSON != "" {
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			log.Warn("failed to parse tags JSON, using empty array", "error", err, "tags", tagsJSON)
			tags = []string{}
		}
	}

	// Get content type from header
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Validate MIME type
	if !ApiTypes.IsAllowedMimeType(contentType) {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Invalid file type. Allowed types: SVG, PNG, JPEG, WebP, GIF",
			Loc:      "SHD_ICH_210",
		})
	}

	// Check service is initialized
	if ApiTypes.DefaultIconService == nil {
		log.Error("icon service not initialized")
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Icon service not initialized",
			Loc:      "SHD_ICH_220",
		})
	}

	// Build request
	var desc *string
	if description != "" {
		desc = &description
	}
	req := ApiTypes.IconUploadRequest{
		Name:        name,
		Category:    category,
		Tags:        tags,
		Description: desc,
	}

	// Create icon file
	icon, err := ApiTypes.DefaultIconService.CreateIcon(rc, req, file, header.Filename, contentType, header.Size, userInfo.Email)
	if err != nil {
		log.Error("failed to create icon file", "error", err)
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Failed to save icon file",
			Loc:      "SHD_ICH_243",
		})
	}

	// Insert into database
	savedIcon, err := sysdatastores.InsertIcon(rc, icon)
	if err != nil {
		log.Error("failed to insert icon to database", "error", err)
		// Try to clean up the file
		ApiTypes.DefaultIconService.DeleteIconFile(rc, icon.Category, icon.FileName)
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Failed to save icon metadata",
			Loc:      "SHD_ICH_256",
		})
	}

	log.Info("Icon uploaded successfully",
		"id", savedIcon.ID,
		"name", savedIcon.Name,
		"category", savedIcon.Category)

	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{
		Status:     true,
		ResultType: "json",
		NumRecords: 1,
		Results:    savedIcon,
		Loc:        "SHD_ICH_269",
	})
}

// HandleDeleteIcon handles DELETE /shared_api/v1/icons/:id
func HandleDeleteIcon(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_ICH_275")
	defer rc.Close()
	log := rc.GetLogger()

	// Check authentication and admin status
	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Authentication required",
			Loc:      "SHD_ICH_284",
		})
	}

	if !userInfo.Admin {
		return c.JSON(http.StatusForbidden, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Admin access required",
			Loc:      "SHD_ICH_292",
		})
	}

	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Icon ID is required",
			Loc:      "SHD_ICH_301",
		})
	}

	// Get icon to get file info before deletion
	icon, err := sysdatastores.GetIconByID(rc, id)
	if err != nil {
		log.Error("failed to get icon for deletion", "error", err, "id", id)
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Failed to get icon",
			Loc:      "SHD_ICH_312",
		})
	}

	if icon == nil {
		return c.JSON(http.StatusNotFound, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Icon not found",
			Loc:      "SHD_ICH_320",
		})
	}

	// Delete from database first
	err = sysdatastores.DeleteIcon(rc, id)
	if err != nil {
		log.Error("failed to delete icon from database", "error", err, "id", id)
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Failed to delete icon",
			Loc:      "SHD_ICH_331",
		})
	}

	if ApiTypes.DefaultIconService == nil {
		log.Error("icon service not initialized")
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Icon service not initialized",
			Loc:      "SHD_ICH_348",
		})
	}

	// Delete file from disk (best effort, don't fail if file deletion fails)
	if err := ApiTypes.DefaultIconService.DeleteIconFile(rc, icon.Category, icon.FileName); err != nil {
		log.Warn("failed to delete icon file", "error", err, "path", icon.FilePath)
	}

	log.Info("Icon deleted", "id", id, "name", icon.Name)

	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{
		Status:     true,
		ResultType: "json",
		Results:    map[string]string{"deleted_id": id},
		Loc:        "SHD_ICH_346",
	})
}

// HandleGetCategories handles GET /shared_api/v1/icons/categories
func HandleGetCategories(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_ICH_352")
	defer rc.Close()
	log := rc.GetLogger()

	// Check authentication
	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Authentication required",
			Loc:      "SHD_ICH_361",
		})
	}

	categories, err := sysdatastores.GetDistinctCategories(rc)
	if err != nil {
		log.Error("failed to get categories", "error", err)
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Failed to get categories",
			Loc:      "SHD_ICH_371",
		})
	}

	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{
		Status:     true,
		ResultType: "json_array",
		NumRecords: len(categories),
		Results:    categories,
		Loc:        "SHD_ICH_380",
	})
}

// HandleServeIconFile handles GET /shared_api/v1/icons/file/:category/:filename
// This serves the actual icon file (requires authentication)
func HandleServeIconFile(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_ICH_387")
	defer rc.Close()
	log := rc.GetLogger()

	// Check authentication
	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Authentication required",
			Loc:      "SHD_ICH_396",
		})
	}

	category := c.Param("category")
	filename := c.Param("filename")

	if category == "" || filename == "" {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Category and filename are required",
			Loc:      "SHD_ICH_407",
		})
	}

	if ApiTypes.DefaultIconService == nil {
		log.Error("icon service not initialized")
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Icon service not initialized",
			Loc:      "SHD_ICH_435",
		})
	}

	filePath, err := ApiTypes.DefaultIconService.GetIconFilePath(category, filename)
	if err != nil {
		log.Warn("icon file not found", "category", category, "filename", filename)
		return c.JSON(http.StatusNotFound, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Icon not found",
			Loc:      "SHD_ICH_426",
		})
	}

	// Set cache headers for better performance
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")

	return c.File(filePath)
}
