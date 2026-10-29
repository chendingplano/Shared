// Package iconlib searches and serves the vector icon catalogue stored in
// shared.icon_library. See shared/openspec/changes/shared-icon-library.
package iconlib

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Embedder turns a query into a vector. Applications register one with
// SetQueryEmbedder; without it, search uses BM25 only.
type Embedder func(ctx context.Context, text string) ([]float32, error)

// Icon is a search result. SVG bodies are served separately by GetSVG.
type Icon struct {
	Name          string  `json:"name"`
	SourceSet     string  `json:"source_set"`
	CategoryEN    string  `json:"category_en"`
	CategoryZH    string  `json:"category_zh"`
	TagsEN        string  `json:"tags_en"`
	TagsZH        string  `json:"tags_zh"`
	DescriptionEN string  `json:"description_en"`
	DescriptionZH string  `json:"description_zh"`
	Version       int     `json:"version"`
	Score         float64 `json:"score"`
}

var (
	embedMu  sync.RWMutex
	embedder Embedder
)

// SetQueryEmbedder registers the query embedder (nil clears it).
func SetQueryEmbedder(e Embedder) {
	embedMu.Lock()
	embedder = e
	embedMu.Unlock()
}

func getEmbedder() Embedder {
	embedMu.RLock()
	defer embedMu.RUnlock()
	return embedder
}

const (
	cols       = `name, source_set, category_en, category_zh, tags_en, tags_zh, description_en, description_zh, version`
	candidates = 60
	rrfK       = 60.0
)

func scan(rows *sql.Rows) (Icon, error) {
	var ic Icon
	err := rows.Scan(&ic.Name, &ic.SourceSet, &ic.CategoryEN, &ic.CategoryZH, &ic.TagsEN, &ic.TagsZH,
		&ic.DescriptionEN, &ic.DescriptionZH, &ic.Version)
	return ic, err
}

// Search returns up to limit icons for a keyword or short-description query in
// English or Chinese. BM25 (ParadeDB) and vector ranks are fused by reciprocal
// rank; a missing embedder, pg_search or pgvector degrades gracefully.
func Search(ctx context.Context, db *sql.DB, query, category string, limit int) ([]Icon, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	if query == "" {
		return list(ctx, db, category, limit)
	}

	var ranked [][]Icon
	if hasBM25(ctx, db) {
		r, err := bm25(ctx, db, query, category)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, r)
	}
	if emb := getEmbedder(); emb != nil {
		if vec, err := emb(ctx, query); err == nil {
			// An embedding failure only removes the vector leg.
			if r, err := nearest(ctx, db, vec, category); err == nil {
				ranked = append(ranked, r)
			}
		}
	}
	if len(ranked) == 0 {
		return like(ctx, db, query, category, limit)
	}
	return fuse(ranked, limit), nil
}

func fuse(lists [][]Icon, limit int) []Icon {
	score := map[string]float64{}
	byName := map[string]Icon{}
	for _, l := range lists {
		for i, ic := range l {
			score[ic.Name] += 1.0 / (rrfK + float64(i+1))
			byName[ic.Name] = ic
		}
	}
	out := make([]Icon, 0, len(score))
	for n, s := range score {
		ic := byName[n]
		ic.Score = s
		out = append(out, ic)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func collect(rows *sql.Rows, err error) ([]Icon, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Icon
	for rows.Next() {
		ic, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ic)
	}
	return out, rows.Err()
}

func bm25(ctx context.Context, db *sql.DB, q, category string) ([]Icon, error) {
	return collect(db.QueryContext(ctx, `SELECT `+cols+` FROM shared.icon_library
		WHERE ($2 = '' OR category_en = $2)
		  AND (name ||| $1 OR tags_en ||| $1 OR tags_zh ||| $1 OR category_en ||| $1 OR category_zh ||| $1
		       OR description_en ||| $1 OR description_zh ||| $1)
		ORDER BY pdb.score(id) DESC LIMIT $3`, q, category, candidates))
}

func nearest(ctx context.Context, db *sql.DB, vec []float32, category string) ([]Icon, error) {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, x := range vec {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", x)
	}
	sb.WriteByte(']')
	return collect(db.QueryContext(ctx, `SELECT `+cols+` FROM shared.icon_library
		WHERE embedding IS NOT NULL AND ($2 = '' OR category_en = $2)
		ORDER BY embedding <=> $1::public.vector LIMIT $3`, sb.String(), category, candidates))
}

func like(ctx context.Context, db *sql.DB, q, category string, limit int) ([]Icon, error) {
	p := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	return collect(db.QueryContext(ctx, `SELECT `+cols+` FROM shared.icon_library
		WHERE ($2 = '' OR category_en = $2)
		  AND (name ILIKE $1 OR tags_en ILIKE $1 OR tags_zh ILIKE $1 OR description_en ILIKE $1 OR description_zh ILIKE $1)
		ORDER BY name LIMIT $3`, p, category, limit))
}

func list(ctx context.Context, db *sql.DB, category string, limit int) ([]Icon, error) {
	return collect(db.QueryContext(ctx, `SELECT `+cols+` FROM shared.icon_library
		WHERE ($1 = '' OR category_en = $1) ORDER BY name LIMIT $2`, category, limit))
}

var (
	bm25Once sync.Once
	bm25Ok   bool
)

func hasBM25(ctx context.Context, db *sql.DB) bool {
	bm25Once.Do(func() {
		var n int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_indexes
			WHERE schemaname='shared' AND indexname='idx_icon_library_bm25'`).Scan(&n)
		bm25Ok = err == nil && n > 0
	})
	return bm25Ok
}

// ErrNotFound is returned by GetSVG for an unknown icon name.
var ErrNotFound = errors.New("icon not found")

// GetSVG returns the SVG body and version of the named icon.
func GetSVG(ctx context.Context, db *sql.DB, name string) (svg string, version int, err error) {
	err = db.QueryRowContext(ctx, `SELECT svg_body, version FROM shared.icon_library WHERE name=$1`, name).
		Scan(&svg, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	return svg, version, err
}
