package iconlib

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
)

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,126}$`)

// ErrExists is returned when an icon of that name is already in the library.
var ErrExists = errors.New("icon name already exists")

// CustomIcon is a user-supplied SVG icon.
type CustomIcon struct {
	Name          string `json:"name"`
	Category      string `json:"category"`
	Tags          string `json:"tags"`
	DescriptionEN string `json:"description_en"`
	DescriptionZH string `json:"description_zh"`
	SVG           string `json:"svg"`
}

// AddCustom sanitises and stores a custom SVG icon (source_set 'custom').
// Embeddings are filled later by icon-enrich --embed-only.
func AddCustom(ctx context.Context, db *sql.DB, ic CustomIcon, creator string) error {
	if !nameRE.MatchString(ic.Name) {
		return fmt.Errorf("name must be lowercase letters, digits and hyphens (max 127)")
	}
	svg, err := SanitizeSVG(ic.SVG)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `INSERT INTO shared.icon_library
		(name, source_set, category_en, tags_en, description_en, description_zh, svg_body, creator)
		VALUES ($1,'custom',$2,$3,$4,$5,$6,$7) ON CONFLICT (name) DO NOTHING`,
		ic.Name, ic.Category, ic.Tags, ic.DescriptionEN, ic.DescriptionZH, svg, creator)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrExists
	}
	return nil
}
