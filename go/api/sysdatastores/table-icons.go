package sysdatastores

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

const IconsTableName = "icons"

var Icons_selected_field_names = "id, " +
	"name, category, file_name, file_path, " +
	"mime_type, file_size, width, height, tags, " +
	"description, creator, updater, created_at, updated_at"

var Icons_insert_field_names = "name, category, file_name, file_path, " +
	"mime_type, file_size, width, height, tags, " +
	"description, creator, updater"

func CreateIconsTable(
	logger ApiTypes.JimoLogger,
	db *sql.DB,
	db_type string,
	table_name string) error {

	logger.Info("Create table", "table_name", table_name)

	var stmt string
	fields :=
		"id              VARCHAR(40) PRIMARY KEY DEFAULT gen_random_uuid()::text, " +
			"name            VARCHAR(128) NOT NULL, " +
			"category        VARCHAR(64) NOT NULL, " +
			"file_name       VARCHAR(255) NOT NULL, " +
			"file_path       VARCHAR(512) NOT NULL, " +
			"mime_type       VARCHAR(64) NOT NULL, " +
			"file_size       BIGINT NOT NULL DEFAULT 0, " +
			"width           INTEGER DEFAULT NULL, " +
			"height          INTEGER DEFAULT NULL, " +
			"tags            JSONB DEFAULT '[]', " +
			"description     TEXT DEFAULT NULL, " +
			"creator         VARCHAR(64) NOT NULL, " +
			"updater         VARCHAR(64) NOT NULL, " +
			"created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(), " +
			"updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(), " +
			"CONSTRAINT uq_icon_category_name UNIQUE (category, name), " +
			"CONSTRAINT chk_mime_type CHECK (mime_type IN ('image/svg+xml', 'image/png', 'image/jpeg', 'image/webp', 'image/gif'))"

	switch db_type {
	case ApiTypes.MysqlName:
		err := fmt.Errorf("mysql not supported for icons table yet (SHD_ICN_055)")
		logger.Error("mysql not supported yet")
		return err

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields + ")"

	default:
		err := fmt.Errorf("database type not supported:%s (SHD_ICN_062)", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		error_msg := fmt.Errorf("failed creating table (SHD_ICN_068), err: %w, stmt:%s", err, stmt)
		logger.Error("failed creating table", "error", err, "stmt", stmt)
		return error_msg
	}

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_icons_category ON ` + table_name + ` (category);`
		databaseutil.ExecuteStatement(db, idx1)

		idx2 := `CREATE INDEX IF NOT EXISTS idx_icons_name ON ` + table_name + ` (name);`
		databaseutil.ExecuteStatement(db, idx2)

		idx3 := `CREATE INDEX IF NOT EXISTS idx_icons_created_at ON ` + table_name + ` (created_at);`
		databaseutil.ExecuteStatement(db, idx3)
	}

	logger.Info("Create table success", "table_name", table_name)
	return nil
}

func scanIconRecord(row *sql.Row, icon *ApiTypes.IconDef) error {
	var tagsJSON []byte
	var width, height sql.NullInt64
	var description sql.NullString

	err := row.Scan(
		&icon.ID,
		&icon.Name,
		&icon.Category,
		&icon.FileName,
		&icon.FilePath,
		&icon.MimeType,
		&icon.FileSize,
		&width,
		&height,
		&tagsJSON,
		&description,
		&icon.Creator,
		&icon.Updater,
		&icon.CreatedAt,
		&icon.UpdatedAt,
	)
	if err != nil {
		return err
	}

	// Parse tags JSON
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &icon.Tags); err != nil {
			icon.Tags = []string{}
		}
	} else {
		icon.Tags = []string{}
	}

	// Handle nullable fields
	if width.Valid {
		w := int(width.Int64)
		icon.Width = &w
	}
	if height.Valid {
		h := int(height.Int64)
		icon.Height = &h
	}
	if description.Valid {
		icon.Description = &description.String
	}

	return nil
}

func scanIconRecordFromRows(rows *sql.Rows, icon *ApiTypes.IconDef) error {
	var tagsJSON []byte
	var width, height sql.NullInt64
	var description sql.NullString

	err := rows.Scan(
		&icon.ID,
		&icon.Name,
		&icon.Category,
		&icon.FileName,
		&icon.FilePath,
		&icon.MimeType,
		&icon.FileSize,
		&width,
		&height,
		&tagsJSON,
		&description,
		&icon.Creator,
		&icon.Updater,
		&icon.CreatedAt,
		&icon.UpdatedAt,
	)
	if err != nil {
		return err
	}

	// Parse tags JSON
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &icon.Tags); err != nil {
			icon.Tags = []string{}
		}
	} else {
		icon.Tags = []string{}
	}

	// Handle nullable fields
	if width.Valid {
		w := int(width.Int64)
		icon.Width = &w
	}
	if height.Valid {
		h := int(height.Int64)
		icon.Height = &h
	}
	if description.Valid {
		icon.Description = &description.String
	}

	return nil
}

// InsertIcon inserts a new icon record and returns the created icon
func InsertIcon(
	rc ApiTypes.RequestContext,
	icon *ApiTypes.IconDef) (*ApiTypes.IconDef, error) {
	logger := rc.GetLogger()
	var db *sql.DB
	var insert_stmt string
	db_type := ApiTypes.DatabaseInfo.DBType

	switch db_type {
	case ApiTypes.MysqlName:
		err := fmt.Errorf("mysql not supported yet (SHD_ICN_185)")
		logger.Error("mysql not supported yet")
		return nil, err

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		insert_stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES ("+
			"$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) "+
			"RETURNING %s",
			IconsTableName, Icons_insert_field_names, Icons_selected_field_names)

	default:
		err := fmt.Errorf("unsupported database type (SHD_ICN_196): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return nil, err
	}

	// Convert tags to JSON
	tagsJSON, err := json.Marshal(icon.Tags)
	if err != nil {
		logger.Error("failed to marshal tags", "error", err)
		return nil, fmt.Errorf("failed to marshal tags (SHD_ICN_204): %w", err)
	}

	// Handle nullable width/height
	var width, height interface{}
	if icon.Width != nil {
		width = *icon.Width
	}
	if icon.Height != nil {
		height = *icon.Height
	}

	args := []interface{}{
		icon.Name,
		icon.Category,
		icon.FileName,
		icon.FilePath,
		icon.MimeType,
		icon.FileSize,
		width,
		height,
		tagsJSON,
		icon.Description,
		icon.Creator,
		icon.Updater,
	}

	row := db.QueryRow(insert_stmt, args...)
	newIcon := new(ApiTypes.IconDef)
	err = scanIconRecord(row, newIcon)
	if err != nil {
		logger.Error("failed to insert icon",
			"error", err,
			"name", icon.Name,
			"category", icon.Category)
		return nil, fmt.Errorf("failed to insert icon (SHD_ICN_235): %w", err)
	}

	logger.Info("Icon inserted",
		"id", newIcon.ID,
		"name", newIcon.Name,
		"category", newIcon.Category)
	return newIcon, nil
}

// GetIconByID retrieves an icon by its ID
func GetIconByID(
	rc ApiTypes.RequestContext,
	id string) (*ApiTypes.IconDef, error) {
	logger := rc.GetLogger()
	var db *sql.DB
	var query string
	db_type := ApiTypes.DatabaseInfo.DBType

	switch db_type {
	case ApiTypes.MysqlName:
		err := fmt.Errorf("mysql not supported yet (SHD_ICN_255)")
		logger.Error("mysql not supported yet")
		return nil, err

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("SELECT %s FROM %s WHERE id = $1", Icons_selected_field_names, IconsTableName)

	default:
		err := fmt.Errorf("unsupported database type (SHD_ICN_263): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return nil, err
	}

	row := db.QueryRow(query, id)
	icon := new(ApiTypes.IconDef)
	err := scanIconRecord(row, icon)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn("icon not found", "id", id)
			return nil, nil
		}
		logger.Error("failed to scan icon record", "error", err, "id", id)
		return nil, err
	}

	logger.Info("Icon retrieved", "id", icon.ID, "name", icon.Name)
	return icon, nil
}

// GetIconByFileName retrieves an icon by category and file name
func GetIconByFileName(
	rc ApiTypes.RequestContext,
	category string,
	fileName string) (*ApiTypes.IconDef, error) {
	logger := rc.GetLogger()
	var db *sql.DB
	var query string
	db_type := ApiTypes.DatabaseInfo.DBType

	switch db_type {
	case ApiTypes.MysqlName:
		err := fmt.Errorf("mysql not supported yet (SHD_ICN_293)")
		logger.Error("mysql not supported yet")
		return nil, err

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("SELECT %s FROM %s WHERE category = $1 AND file_name = $2",
			Icons_selected_field_names, IconsTableName)

	default:
		err := fmt.Errorf("unsupported database type (SHD_ICN_302): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return nil, err
	}

	row := db.QueryRow(query, category, fileName)
	icon := new(ApiTypes.IconDef)
	err := scanIconRecord(row, icon)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn("icon not found", "category", category, "fileName", fileName)
			return nil, nil
		}
		logger.Error("failed to scan icon record", "error", err)
		return nil, err
	}

	logger.Info("Icon retrieved", "id", icon.ID, "name", icon.Name)
	return icon, nil
}

// ListIcons retrieves icons with optional filters and pagination
func ListIcons(
	rc ApiTypes.RequestContext,
	req ApiTypes.IconListRequest) ([]*ApiTypes.IconDef, int, error) {
	logger := rc.GetLogger()
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType

	switch db_type {
	case ApiTypes.MysqlName:
		err := fmt.Errorf("mysql not supported yet (SHD_ICN_333)")
		logger.Error("mysql not supported yet")
		return nil, 0, err

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_ICN_340): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return nil, 0, err
	}

	// Build WHERE clause
	var whereClauses []string
	var args []interface{}
	paramIndex := 1

	if req.Category != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("category = $%d", paramIndex))
		args = append(args, req.Category)
		paramIndex++
	}

	if req.Search != "" {
		// Search in name and tags
		whereClauses = append(whereClauses,
			fmt.Sprintf("(name ILIKE $%d OR tags::text ILIKE $%d)", paramIndex, paramIndex+1))
		searchPattern := "%" + req.Search + "%"
		args = append(args, searchPattern, searchPattern)
		paramIndex += 2
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count total records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", IconsTableName, whereClause)
	var total int
	err := db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		logger.Error("failed to count icons", "error", err)
		return nil, 0, fmt.Errorf("failed to count icons (SHD_ICN_375): %w", err)
	}

	// Get paginated results
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	offset := req.Page * pageSize

	query := fmt.Sprintf("SELECT %s FROM %s %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		Icons_selected_field_names, IconsTableName, whereClause, paramIndex, paramIndex+1)
	args = append(args, pageSize, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		logger.Error("failed to query icons", "error", err)
		return nil, 0, fmt.Errorf("failed to query icons (SHD_ICN_394): %w", err)
	}
	defer rows.Close()

	var iconsList []*ApiTypes.IconDef
	for rows.Next() {
		icon := new(ApiTypes.IconDef)
		err := scanIconRecordFromRows(rows, icon)
		if err != nil {
			logger.Error("failed to scan icon record", "error", err)
			return nil, 0, fmt.Errorf("failed to scan icon record (SHD_ICN_404): %w", err)
		}
		iconsList = append(iconsList, icon)
	}

	if err := rows.Err(); err != nil {
		logger.Error("error iterating rows", "error", err)
		return nil, 0, fmt.Errorf("error iterating rows (SHD_ICN_411): %w", err)
	}

	logger.Info("Icons retrieved", "count", len(iconsList), "total", total)
	return iconsList, total, nil
}

// UpdateIcon updates an icon's metadata
func UpdateIcon(
	rc ApiTypes.RequestContext,
	id string,
	req ApiTypes.IconUpdateRequest,
	updater string) (*ApiTypes.IconDef, error) {
	logger := rc.GetLogger()
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType

	switch db_type {
	case ApiTypes.MysqlName:
		err := fmt.Errorf("mysql not supported yet (SHD_ICN_430)")
		logger.Error("mysql not supported yet")
		return nil, err

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_ICN_437): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return nil, err
	}

	// Build SET clause dynamically
	var setClauses []string
	var args []interface{}
	paramIndex := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", paramIndex))
		args = append(args, *req.Name)
		paramIndex++
	}
	if req.Category != nil {
		setClauses = append(setClauses, fmt.Sprintf("category = $%d", paramIndex))
		args = append(args, *req.Category)
		paramIndex++
	}
	if req.Tags != nil {
		tagsJSON, err := json.Marshal(req.Tags)
		if err != nil {
			logger.Error("failed to marshal tags", "error", err)
			return nil, fmt.Errorf("failed to marshal tags (SHD_ICN_460): %w", err)
		}
		setClauses = append(setClauses, fmt.Sprintf("tags = $%d", paramIndex))
		args = append(args, tagsJSON)
		paramIndex++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", paramIndex))
		args = append(args, *req.Description)
		paramIndex++
	}

	if len(setClauses) == 0 {
		// Nothing to update, just return current icon
		return GetIconByID(rc, id)
	}

	// Always update updater and updated_at
	setClauses = append(setClauses, fmt.Sprintf("updater = $%d", paramIndex))
	args = append(args, updater)
	paramIndex++

	setClauses = append(setClauses, "updated_at = NOW()")

	// Add ID for WHERE clause
	args = append(args, id)

	updateStmt := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d RETURNING %s",
		IconsTableName,
		strings.Join(setClauses, ", "),
		paramIndex,
		Icons_selected_field_names)

	row := db.QueryRow(updateStmt, args...)
	icon := new(ApiTypes.IconDef)
	err := scanIconRecord(row, icon)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn("icon not found for update", "id", id)
			return nil, nil
		}
		logger.Error("failed to update icon", "error", err, "id", id)
		return nil, fmt.Errorf("failed to update icon (SHD_ICN_502): %w", err)
	}

	logger.Info("Icon updated", "id", icon.ID, "name", icon.Name)
	return icon, nil
}

// DeleteIcon deletes an icon by ID
func DeleteIcon(
	rc ApiTypes.RequestContext,
	id string) error {
	logger := rc.GetLogger()
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType

	switch db_type {
	case ApiTypes.MysqlName:
		err := fmt.Errorf("mysql not supported yet (SHD_ICN_520)")
		logger.Error("mysql not supported yet")
		return err

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("DELETE FROM %s WHERE id = $1", IconsTableName)

	default:
		err := fmt.Errorf("unsupported database type (SHD_ICN_528): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return err
	}

	result, err := db.Exec(stmt, id)
	if err != nil {
		logger.Error("failed to delete icon", "error", err, "id", id)
		return fmt.Errorf("failed to delete icon (SHD_ICN_536): %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Error("failed to get rows affected", "error", err)
		return fmt.Errorf("failed to get rows affected (SHD_ICN_542): %w", err)
	}

	if rowsAffected == 0 {
		logger.Warn("no icon found to delete", "id", id)
		return fmt.Errorf("icon not found (SHD_ICN_547): %s", id)
	}

	logger.Info("Icon deleted", "id", id)
	return nil
}

// GetDistinctCategories returns all unique categories
func GetDistinctCategories(
	rc ApiTypes.RequestContext) ([]string, error) {
	logger := rc.GetLogger()
	var db *sql.DB
	var query string
	db_type := ApiTypes.DatabaseInfo.DBType

	switch db_type {
	case ApiTypes.MysqlName:
		err := fmt.Errorf("mysql not supported yet (SHD_ICN_564)")
		logger.Error("mysql not supported yet")
		return nil, err

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("SELECT DISTINCT category FROM %s ORDER BY category", IconsTableName)

	default:
		err := fmt.Errorf("unsupported database type (SHD_ICN_572): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return nil, err
	}

	rows, err := db.Query(query)
	if err != nil {
		logger.Error("failed to query categories", "error", err)
		return nil, fmt.Errorf("failed to query categories (SHD_ICN_580): %w", err)
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			logger.Error("failed to scan category", "error", err)
			return nil, fmt.Errorf("failed to scan category (SHD_ICN_589): %w", err)
		}
		categories = append(categories, category)
	}

	if err := rows.Err(); err != nil {
		logger.Error("error iterating rows", "error", err)
		return nil, fmt.Errorf("error iterating rows (SHD_ICN_596): %w", err)
	}

	logger.Info("Categories retrieved", "count", len(categories))
	return categories, nil
}
