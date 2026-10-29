// icon-enrich fills the Chinese/English search text and embeddings of
// shared.icon_library rows. It is resumable: only rows still missing a field
// are processed, so a failed run can simply be started again.
//
//	icon-enrich --models ChenWeb/.models.toml --llm deepseek-flash-processor \
//	            --embed bge-m3-llama-cpp [--limit N] [--text-only|--embed-only]
//
// Database settings come from PG_* environment variables; API keys are read
// from the models file, never from flags.
package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/pelletier/go-toml/v2"
)

//go:embed prompt-icon-metadata-v1.md
var metadataPrompt string

type modelDef struct {
	ModelName string `toml:"model_name"`
	APIKey    string `toml:"api_key"`
	BaseURL   string `toml:"base_url"`
	Timeout   int    `toml:"timeout_sec"`
	Dimension int    `toml:"dimension"`
}

type row struct {
	id                   int64
	name, tags, category string
}

type outIcon struct {
	Name          string `json:"name"`
	CategoryEN    string `json:"category_en"`
	CategoryZH    string `json:"category_zh"`
	TagsZH        string `json:"tags_zh"`
	DescriptionEN string `json:"description_en"`
	DescriptionZH string `json:"description_zh"`
}

func main() {
	models := flag.String("models", "", ".models.toml path")
	llmName := flag.String("llm", "", "chat model section name")
	embName := flag.String("embed", "", "embedding model section name")
	limit := flag.Int("limit", 0, "process at most N icons (0 = all)")
	batch := flag.Int("batch", 40, "icons per LLM call")
	workers := flag.Int("workers", 6, "concurrent LLM calls")
	textOnly := flag.Bool("text-only", false, "skip embeddings")
	embedOnly := flag.Bool("embed-only", false, "skip LLM text")
	flag.Parse()
	if *models == "" {
		fmt.Fprintln(os.Stderr, "usage: icon-enrich --models FILE --llm NAME --embed NAME [--limit N]")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*models)
	fail(err)
	defs := map[string]modelDef{}
	fail(toml.Unmarshal(raw, &defs))

	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("PG_HOST"), os.Getenv("PG_PORT"), os.Getenv("PG_USER_NAME"),
		os.Getenv("PG_PASSWORD"), os.Getenv("PG_DB_NAME")))
	fail(err)
	defer db.Close()

	if !*embedOnly {
		def, ok := defs[*llmName]
		if !ok {
			fail(fmt.Errorf("llm section %q not found", *llmName))
		}
		fail(enrichText(db, def, *limit, *batch, *workers))
	}
	if !*textOnly {
		def, ok := defs[*embName]
		if !ok {
			fail(fmt.Errorf("embed section %q not found", *embName))
		}
		fail(embedAll(db, def, *limit))
	}
}

func enrichText(db *sql.DB, def modelDef, limit, batch, workers int) error {
	q := `SELECT id, name, tags_en, category_en FROM shared.icon_library
	      WHERE description_en='' OR description_zh='' OR tags_zh='' OR category_zh='' ORDER BY id`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.Query(q)
	if err != nil {
		return err
	}
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.name, &r.tags, &r.category); err != nil {
			return err
		}
		todo = append(todo, r)
	}
	rows.Close()
	fmt.Printf("text: %d icons to enrich\n", len(todo))

	jobs := make(chan []row)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done, failed := 0, 0
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for b := range jobs {
				n, err := enrichBatch(db, def, b)
				mu.Lock()
				done += n
				if err != nil {
					failed += len(b) - n
					fmt.Fprintln(os.Stderr, "batch failed:", err)
				}
				fmt.Printf("\rtext: %d done, %d failed", done, failed)
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < len(todo); i += batch {
		end := min(i+batch, len(todo))
		jobs <- todo[i:end]
	}
	close(jobs)
	wg.Wait()
	fmt.Println()
	return nil
}

func enrichBatch(db *sql.DB, def modelDef, b []row) (int, error) {
	type inIcon struct {
		Name     string `json:"name"`
		Tags     string `json:"tags,omitempty"`
		Category string `json:"category,omitempty"`
	}
	in := make([]inIcon, len(b))
	for i, r := range b {
		in[i] = inIcon{r.name, r.tags, r.category}
	}
	payload, _ := json.Marshal(in)
	body, _ := json.Marshal(map[string]any{
		"model":           def.ModelName,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": metadataPrompt},
			{"role": "user", "content": string(payload)},
		},
	})
	var resp struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := post(def, "/chat/completions", body, &resp); err != nil {
		return 0, err
	}
	if len(resp.Choices) == 0 {
		return 0, fmt.Errorf("empty response")
	}
	var out struct {
		Icons []outIcon `json:"icons"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &out); err != nil {
		return 0, fmt.Errorf("bad json: %w", err)
	}
	byName := map[string]outIcon{}
	for _, o := range out.Icons {
		byName[o.Name] = o
	}
	n := 0
	for _, r := range b {
		o, ok := byName[r.name]
		if !ok || o.DescriptionEN == "" || o.DescriptionZH == "" || o.TagsZH == "" || o.CategoryZH == "" {
			continue
		}
		cat := r.category
		if cat == "" {
			cat = o.CategoryEN
		}
		if _, err := db.Exec(`UPDATE shared.icon_library SET category_en=$2, category_zh=$3, tags_zh=$4,
			description_en=$5, description_zh=$6, embedding=NULL, updated_at=NOW() WHERE id=$1`,
			r.id, cat, o.CategoryZH, o.TagsZH, o.DescriptionEN, o.DescriptionZH); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func embedAll(db *sql.DB, def modelDef, limit int) error {
	q := `SELECT id, name || ' ' || tags_en || ' ' || tags_zh || ' ' || category_en || ' ' || category_zh ||
	      ' ' || description_en || ' ' || description_zh FROM shared.icon_library
	      WHERE embedding IS NULL AND description_en <> '' ORDER BY id`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.Query(q)
	if err != nil {
		return err
	}
	type item struct {
		id   int64
		text string
	}
	var todo []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.text); err != nil {
			return err
		}
		todo = append(todo, it)
	}
	rows.Close()
	fmt.Printf("embed: %d icons\n", len(todo))
	const size = 32
	for i := 0; i < len(todo); i += size {
		chunk := todo[i:min(i+size, len(todo))]
		texts := make([]string, len(chunk))
		for j, it := range chunk {
			texts[j] = it.text
		}
		vecs, err := Embed(def, texts)
		if err != nil {
			return err
		}
		for j, it := range chunk {
			if _, err := db.Exec(`UPDATE shared.icon_library SET embedding=$2::public.vector WHERE id=$1`,
				it.id, vecLiteral(vecs[j])); err != nil {
				return err
			}
		}
		fmt.Printf("\rembed: %d/%d", min(i+size, len(todo)), len(todo))
	}
	fmt.Println()
	return nil
}

// Embed returns one vector per text from an OpenAI-compatible /v1/embeddings endpoint.
func Embed(def modelDef, texts []string) ([][]float64, error) {
	body, _ := json.Marshal(map[string]any{"model": def.ModelName, "input": texts})
	var resp struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := post(def, "/v1/embeddings", body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("embedding count %d != %d", len(resp.Data), len(texts))
	}
	out := make([][]float64, len(texts))
	for _, d := range resp.Data {
		if def.Dimension > 0 && len(d.Embedding) != def.Dimension {
			return nil, fmt.Errorf("embedding dimension %d != %d", len(d.Embedding), def.Dimension)
		}
		out[d.Index] = d.Embedding
	}
	return out, nil
}

func vecLiteral(v []float64) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", x)
	}
	sb.WriteByte(']')
	return sb.String()
}

func post(def modelDef, path string, body []byte, v any) error {
	timeout := time.Duration(max(def.Timeout, 30)) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(def.BaseURL, "/")+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if def.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+def.APIKey)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return fmt.Errorf("%s: HTTP %d: %.200s", path, res.StatusCode, b)
	}
	return json.Unmarshal(b, v)
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
