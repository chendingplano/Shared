package icons

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
)

var logger = loggerutil.CreateDefaultLogger()

// iconServiceImpl is the concrete implementation using local filesystem
type iconServiceImpl struct {
	dataHomeDir string
}

// NewIconService creates a new IconService instance
// dataHomeDir: base directory for icon storage (from DATA_HOME_DIR env var)
func NewIconService(dataHomeDir string) ApiTypes.IconService {
	return &iconServiceImpl{
		dataHomeDir: dataHomeDir,
	}
}

// InitIconService initializes the icon service with the data home directory
// This should be called during application startup
func InitIconService(rc ApiTypes.RequestContext) error {
	logger := rc.GetLogger()
	if ApiTypes.LibConfig.IconServiceConf.EnableIconService == "disabled" {
		logger.Warn("icon service is disabled")
		return nil
	}

	dataHomeDir := ApiTypes.LibConfig.DataHomeDir
	if dataHomeDir == "" {
		logger.Error("Missing data_home_dir config")
		return fmt.Errorf("Missing data_home_dir config (SHD_ISV_042)")
	}

	iconDataDir := ApiTypes.LibConfig.IconServiceConf.IconDataDir
	if iconDataDir == "" {
		logger.Error("Missing [icon_service]:icon_data_dir config item")
		return fmt.Errorf("Missing [icon_service]:icon_data_dir config item (SHD_ISV_048)")
	}

	iconHomeDir := fmt.Sprintf("%s/%s", dataHomeDir, iconDataDir)

	// Ensure base icons directory exists
	if err := os.MkdirAll(iconHomeDir, 0755); err != nil {
		logger.Error("Failed create icon home directory", "path", iconHomeDir)
		return fmt.Errorf("failed to create icons directory (SHD_ISV_056): %w", err)
	}

	ApiTypes.DefaultIconService = NewIconService(iconHomeDir)
	logger.Info("Icon service initialized", "dataHomeDir", iconHomeDir)
	return nil
}

// getIconsDir returns the icons directory path
func (s *iconServiceImpl) getIconsDir() string {
	return filepath.Join(s.dataHomeDir, "icons")
}

// getCategoryDir returns the directory path for a specific category
func (s *iconServiceImpl) getCategoryDir(category string) string {
	// Sanitize category name to prevent path traversal
	sanitized := sanitizePath(category)
	return filepath.Join(s.getIconsDir(), sanitized)
}

// sanitizePath removes potentially dangerous characters from path components
func sanitizePath(input string) string {
	// Remove any path separators and dots that could be used for traversal
	cleaned := strings.ReplaceAll(input, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, "\\", "_")
	cleaned = strings.ReplaceAll(cleaned, "..", "_")
	return cleaned
}

// ListIcons returns a list of icons with optional filters
func (s *iconServiceImpl) ListIcons(rc ApiTypes.RequestContext, req ApiTypes.IconListRequest) ([]*ApiTypes.IconDef, int, error) {
	// Import from sysdatastores would create circular dependency
	// This will be handled via the handlers calling sysdatastores directly
	return nil, 0, fmt.Errorf("not implemented - use sysdatastores.ListIcons directly (SHD_ICN_SVC_100)")
}

// GetIcon retrieves a single icon by ID
func (s *iconServiceImpl) GetIcon(rc ApiTypes.RequestContext, id string) (*ApiTypes.IconDef, error) {
	return nil, fmt.Errorf("not implemented - use sysdatastores.GetIconByID directly (SHD_ICN_SVC_105)")
}

// CreateIcon uploads and creates a new icon
func (s *iconServiceImpl) CreateIcon(
	rc ApiTypes.RequestContext,
	req ApiTypes.IconUploadRequest,
	file io.Reader,
	filename string,
	mimeType string,
	fileSize int64,
	creator string) (*ApiTypes.IconDef, error) {

	log := rc.GetLogger()

	// Validate MIME type
	if !ApiTypes.IsAllowedMimeType(mimeType) {
		return nil, fmt.Errorf("invalid MIME type: %s (SHD_ICN_SVC_120)", mimeType)
	}

	// Sanitize category
	category := sanitizePath(req.Category)
	if category == "" {
		return nil, fmt.Errorf("category is required (SHD_ICN_SVC_125)")
	}

	// Ensure category directory exists
	categoryDir := s.getCategoryDir(category)
	if err := os.MkdirAll(categoryDir, 0755); err != nil {
		log.Error("failed to create category directory", "error", err, "category", category)
		return nil, fmt.Errorf("failed to create category directory (SHD_ICN_SVC_132): %w", err)
	}

	// Generate unique filename
	ext := filepath.Ext(filename)
	if ext == "" {
		// Infer extension from MIME type
		switch mimeType {
		case "image/svg+xml":
			ext = ".svg"
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/webp":
			ext = ".webp"
		case "image/gif":
			ext = ".gif"
		default:
			ext = ".png"
		}
	}
	uniqueID := uuid.New().String()[:8]
	newFileName := fmt.Sprintf("icon_%s%s", uniqueID, ext)
	filePath := filepath.Join(categoryDir, newFileName)
	relPath := filepath.Join("icons", category, newFileName)

	// Read file content into memory for dimension detection
	content, err := io.ReadAll(file)
	if err != nil {
		log.Error("failed to read file content", "error", err)
		return nil, fmt.Errorf("failed to read file content (SHD_ICN_SVC_160): %w", err)
	}

	// Write file to disk
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		log.Error("failed to write icon file", "error", err, "path", filePath)
		return nil, fmt.Errorf("failed to write icon file (SHD_ICN_SVC_166): %w", err)
	}

	// Try to get image dimensions (skip for SVG)
	var width, height *int
	if mimeType != "image/svg+xml" {
		if img, _, err := image.DecodeConfig(strings.NewReader(string(content))); err == nil {
			w, h := img.Width, img.Height
			width = &w
			height = &h
		}
	}

	// Ensure tags is not nil
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	// Create icon record
	icon := &ApiTypes.IconDef{
		Name:        req.Name,
		Category:    category,
		FileName:    newFileName,
		FilePath:    relPath,
		MimeType:    mimeType,
		FileSize:    int64(len(content)),
		Width:       width,
		Height:      height,
		Tags:        tags,
		Description: req.Description,
		Creator:     creator,
		Updater:     creator,
	}

	log.Info("Icon file created",
		"fileName", newFileName,
		"category", category,
		"size", len(content))

	return icon, nil
}

// UpdateIcon updates an icon's metadata
func (s *iconServiceImpl) UpdateIcon(
	rc ApiTypes.RequestContext,
	id string,
	req ApiTypes.IconUpdateRequest,
	updater string) (*ApiTypes.IconDef, error) {
	return nil, fmt.Errorf("not implemented - use sysdatastores.UpdateIcon directly (SHD_ICN_SVC_210)")
}

// DeleteIcon removes an icon file from disk
func (s *iconServiceImpl) DeleteIcon(rc ApiTypes.RequestContext, id string) error {
	return fmt.Errorf("not implemented - icon file deletion handled in handlers (SHD_ICN_SVC_215)")
}

// DeleteIconFile removes an icon file from disk by its path
func (s *iconServiceImpl) DeleteIconFile(
	rc ApiTypes.RequestContext,
	category string,
	fileName string) error {
	filePath := filepath.Join(s.getCategoryDir(category), fileName)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logger.Warn("icon file not found for deletion", "path", filePath)
		return nil // File already doesn't exist, consider it deleted
	}

	if err := os.Remove(filePath); err != nil {
		logger.Error("failed to delete icon file", "error", err, "path", filePath)
		return fmt.Errorf("failed to delete icon file (SHD_ICN_SVC_230): %w", err)
	}

	logger.Info("Icon file deleted", "path", filePath)
	return nil
}

// GetCategories returns all distinct categories
func (s *iconServiceImpl) GetCategories(rc ApiTypes.RequestContext) ([]string, error) {
	return nil, fmt.Errorf("not implemented - use sysdatastores.GetDistinctCategories directly (SHD_ICN_SVC_239)")
}

// GetIconFilePath returns the full file path for serving an icon file
func (s *iconServiceImpl) GetIconFilePath(category string, filename string) (string, error) {
	sanitizedCategory := sanitizePath(category)
	sanitizedFilename := sanitizePath(filename)

	filePath := filepath.Join(s.getCategoryDir(sanitizedCategory), sanitizedFilename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("icon file not found (SHD_ICN_SVC_251): %s/%s", category, filename)
	}

	return filePath, nil
}

/*
// DeleteIconFileByPath is a helper to delete icon file using service
func DeleteIconFileByPath(category string, fileName string) error {
	if ApiTypes.DefaultIconService == nil {
		return fmt.Errorf("icon service not initialized (SHD_ICN_SVC_260)")
	}

	if impl, ok := ApiTypes.DefaultIconService.(*iconServiceImpl); ok {
		return impl.DeleteIconFile(category, fileName)
	}

	return fmt.Errorf("icon service does not support file deletion (SHD_ICN_SVC_267)")
}
*/
