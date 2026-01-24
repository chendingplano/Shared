package ApiTypes

import (
	"io"
	"time"
)

// IconDef represents an icon record in the database
// Make sure it syncs with shared/svelte/src/lib/types/IconTypes.ts::IconDef
type IconDef struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	FileName    string    `json:"file_name"`
	FilePath    string    `json:"file_path"`
	MimeType    string    `json:"mime_type"`
	FileSize    int64     `json:"file_size"`
	Width       *int      `json:"width,omitempty"`
	Height      *int      `json:"height,omitempty"`
	Tags        []string  `json:"tags"`
	Description *string   `json:"description,omitempty"`
	Creator     string    `json:"creator"`
	Updater     string    `json:"updater"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IconUploadRequest for creating/uploading icons
// Make sure it syncs with shared/svelte/src/lib/types/IconTypes.ts::IconUploadRequest
type IconUploadRequest struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Description *string  `json:"description,omitempty"`
}

// IconUpdateRequest for updating icon metadata
// Make sure it syncs with shared/svelte/src/lib/types/IconTypes.ts::IconUpdateRequest
type IconUpdateRequest struct {
	Name        *string  `json:"name,omitempty"`
	Category    *string  `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Description *string  `json:"description,omitempty"`
}

// IconListRequest for querying icons with pagination
// Make sure it syncs with shared/svelte/src/lib/types/IconTypes.ts::IconListRequest
type IconListRequest struct {
	Category string `json:"category,omitempty"`
	Search   string `json:"search,omitempty"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// Allowed MIME types for icon uploads
var AllowedIconMimeTypes = map[string]bool{
	"image/svg+xml": true,
	"image/png":     true,
	"image/jpeg":    true,
	"image/webp":    true,
	"image/gif":     true,
}

// IsAllowedIconMimeType checks if the given MIME type is allowed for icon uploads
func IsAllowedIconMimeType(mimeType string) bool {
	return AllowedIconMimeTypes[mimeType]
}

// IsAllowedMimeType checks if the given MIME type is allowed for icon uploads
func IsAllowedMimeType(mimeType string) bool {
	return IsAllowedIconMimeType(mimeType)
}

// GetAllowedMimeTypes returns a slice of all allowed MIME types
func GetAllowedMimeTypes() []string {
	types := make([]string, 0, len(AllowedIconMimeTypes))
	for t := range AllowedIconMimeTypes {
		types = append(types, t)
	}
	return types
}

// IconService defines the interface for icon operations
type IconService interface {
	// ListIcons returns a list of icons with optional filters
	ListIcons(rc RequestContext, req IconListRequest) ([]*IconDef, int, error)

	// GetIcon retrieves a single icon by ID
	GetIcon(rc RequestContext, id string) (*IconDef, error)

	// CreateIcon uploads and creates a new icon
	CreateIcon(rc RequestContext, req IconUploadRequest, file io.Reader, filename string, mimeType string, fileSize int64, creator string) (*IconDef, error)

	// UpdateIcon updates an icon's metadata
	UpdateIcon(rc RequestContext, id string, req IconUpdateRequest, updater string) (*IconDef, error)

	// DeleteIcon removes an icon
	DeleteIcon(rc RequestContext, id string) error

	// GetCategories returns all distinct categories
	GetCategories(rc RequestContext) ([]string, error)

	// GetIconFilePath returns the full file path for serving an icon file
	GetIconFilePath(category string, filename string) (string, error)

	// DeleteIconFileByPath deletes icon file using service
	DeleteIconFile(rc RequestContext, category string, fileName string) error
}

// DefaultIconService is the singleton instance (set during initialization)
var DefaultIconService IconService

// SetIconService allows dependency injection (similar to SetEmailSender pattern)
func SetIconService(svc IconService) {
	DefaultIconService = svc
}
