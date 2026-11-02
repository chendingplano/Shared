// Package emojilib provides the locally hosted emoji catalogue and BM25 search.
package emojilib

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/chendingplano/shared/go/api/emojibuild"
)

var ErrBM25Unavailable = errors.New("emoji BM25 index unavailable")
var ErrNotFound = errors.New("emoji not found")

type Emoji struct {
	ID            string  `json:"unicode_id"`
	Unicode       string  `json:"unicode_value"`
	NameEN        string  `json:"name_en"`
	NameZH        string  `json:"name_zh"`
	CategoryID    string  `json:"category_id"`
	CategoryEN    string  `json:"category_en"`
	CategoryZH    string  `json:"category_zh"`
	KeywordsEN    string  `json:"keywords_en"`
	KeywordsZH    string  `json:"keywords_zh"`
	DescriptionEN string  `json:"description_en"`
	DescriptionZH string  `json:"description_zh"`
	Version       int     `json:"version"`
	Score         float64 `json:"score"`
}
type Page struct {
	Items      []Emoji `json:"items"`
	NextOffset *int    `json:"next_offset,omitempty"`
}
type Category struct {
	ID     string `json:"id"`
	NameEN string `json:"name_en"`
	NameZH string `json:"name_zh"`
}

type Source struct {
	TwemojiVersion string
	UnicodeVersion string
}

const columns = `unicode_id,unicode_value,name_en,name_zh,category_id,category_en,category_zh,keywords_en,keywords_zh,description_en,description_zh,version`

func bounds(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
func scan(rows *sql.Rows) ([]Emoji, error) {
	defer rows.Close()
	out := []Emoji{}
	for rows.Next() {
		var e Emoji
		if err := rows.Scan(&e.ID, &e.Unicode, &e.NameEN, &e.NameZH, &e.CategoryID, &e.CategoryEN, &e.CategoryZH, &e.KeywordsEN, &e.KeywordsZH, &e.DescriptionEN, &e.DescriptionZH, &e.Version, &e.Score); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func hasEmojiSymbol(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.So, r) || r == '\u200d' || r == '\ufe0f' {
			return true
		}
	}
	return false
}

// Ready checks the actual database on every call so a newly created index is
// recognized without restarting the application.
func Ready(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return ErrBM25Unavailable
	}
	var ready bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='shared' AND c.relname='idx_emoji_library_bm25' AND i.indisvalid AND i.indisready)`).Scan(&ready); err != nil || !ready {
		return ErrBM25Unavailable
	}
	return nil
}

// Search uses BM25 for text, an exact lookup for Unicode, or stable paged browse.
func Search(ctx context.Context, db *sql.DB, q, category string, limit, offset int) (Page, error) {
	result := Page{Items: []Emoji{}}
	q = strings.TrimSpace(q)
	if len(q) > 512 {
		return result, errors.New("query too long")
	}
	limit, offset = bounds(limit, offset)
	if q != "" && hasEmojiSymbol(q) {
		rows, err := db.QueryContext(ctx, `SELECT `+columns+`, 0::float8 FROM shared.emoji_library WHERE unicode_value=$1 AND ($2='' OR category_id=$2)`, q, category)
		if err != nil {
			return result, err
		}
		result.Items, err = scan(rows)
		if err != nil || len(result.Items) > 0 {
			return result, err
		}
		hasText := false
		for _, r := range q {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				hasText = true
				break
			}
		}
		if !hasText {
			return result, nil
		}
	}
	if q != "" {
		if err := Ready(ctx, db); err != nil {
			return result, err
		}
		rows, err := db.QueryContext(ctx, `SELECT `+columns+`, pdb.score(id)::float8 FROM shared.emoji_library
    WHERE ($2='' OR category_id=$2) AND (name_en ||| $1 OR name_zh ||| $1 OR keywords_en ||| $1 OR keywords_zh ||| $1 OR category_en ||| $1 OR category_zh ||| $1 OR description_en ||| $1 OR description_zh ||| $1)
    ORDER BY pdb.score(id) DESC, unicode_id LIMIT $3`, q, category, limit)
		if err != nil {
			return result, err
		}
		result.Items, err = scan(rows)
		return result, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+columns+`, 0::float8 FROM shared.emoji_library WHERE ($1='' OR category_id=$1) ORDER BY sort_order,unicode_id LIMIT $2 OFFSET $3`, category, limit+1, offset)
	if err != nil {
		return result, err
	}
	result.Items, err = scan(rows)
	if err != nil {
		return result, err
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		next := offset + limit
		result.NextOffset = &next
	}
	return result, nil
}

func Get(ctx context.Context, db *sql.DB, id string) (Emoji, error) {
	var e Emoji
	err := db.QueryRowContext(ctx, `SELECT `+columns+` FROM shared.emoji_library WHERE unicode_id=$1`, id).Scan(&e.ID, &e.Unicode, &e.NameEN, &e.NameZH, &e.CategoryID, &e.CategoryEN, &e.CategoryZH, &e.KeywordsEN, &e.KeywordsZH, &e.DescriptionEN, &e.DescriptionZH, &e.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}
func SVG(ctx context.Context, db *sql.DB, id string) (string, string, error) {
	var svg, hash string
	err := db.QueryRowContext(ctx, `SELECT svg_body,content_hash FROM shared.emoji_library WHERE unicode_id=$1`, id).Scan(&svg, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return svg, hash, err
}
func Categories(ctx context.Context, db *sql.DB) ([]Category, error) {
	rows, err := db.QueryContext(ctx, `SELECT category_id,min(category_en),min(category_zh) FROM shared.emoji_library GROUP BY category_id ORDER BY min(sort_order),category_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err = rows.Scan(&c.ID, &c.NameEN, &c.NameZH); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Import publishes a fully built catalogue as one transaction and preserves
// historical entries. Unchanged rows are not touched.
func Import(ctx context.Context, db *sql.DB, all []emojibuild.Emoji, source Source) error {
	if err := emojibuild.Validate(all); err != nil {
		return err
	}
	if strings.TrimSpace(source.TwemojiVersion) == "" || strings.TrimSpace(source.UnicodeVersion) == "" {
		return errors.New("missing catalogue source version")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range all {
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		_, err = tx.ExecContext(ctx, `INSERT INTO shared.emoji_library (unicode_id,unicode_value,name_en,name_zh,category_id,category_en,category_zh,keywords_en,keywords_zh,description_en,description_zh,svg_body,sort_order,content_hash,source_version,unicode_version)
   VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
   ON CONFLICT (unicode_id) DO UPDATE SET unicode_value=EXCLUDED.unicode_value,name_en=EXCLUDED.name_en,name_zh=EXCLUDED.name_zh,category_id=EXCLUDED.category_id,category_en=EXCLUDED.category_en,category_zh=EXCLUDED.category_zh,keywords_en=EXCLUDED.keywords_en,keywords_zh=EXCLUDED.keywords_zh,description_en=EXCLUDED.description_en,description_zh=EXCLUDED.description_zh,svg_body=EXCLUDED.svg_body,sort_order=EXCLUDED.sort_order,content_hash=EXCLUDED.content_hash,source_version=EXCLUDED.source_version,unicode_version=EXCLUDED.unicode_version,version=shared.emoji_library.version+1,updated_at=NOW() WHERE shared.emoji_library.content_hash<>EXCLUDED.content_hash OR shared.emoji_library.source_version<>EXCLUDED.source_version OR shared.emoji_library.unicode_version<>EXCLUDED.unicode_version`, e.ID, e.Unicode, e.NameEN, e.NameZH, e.CategoryID, e.CategoryEN, e.CategoryZH, e.KeywordsEN, e.KeywordsZH, e.DescriptionEN, e.DescriptionZH, e.SVG, e.Order, hash, source.TwemojiVersion, source.UnicodeVersion)
		if err != nil {
			return fmt.Errorf("import %s: %w", e.ID, err)
		}
	}
	return tx.Commit()
}
