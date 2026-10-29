// icon-seed imports vector icons into shared.icon_library.
//
// Lucide is the base set; a Tabler (outline) icon is added only when no icon of
// that name exists. Re-running with unchanged sources changes nothing.
//
//	icon-seed --lucide <lucide-static dir> --tabler <@tabler/icons dir> [--dry-run]
//
// Connection settings come from the PG_* environment variables (never flags).
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/lib/pq"
)

type icon struct {
	name, set, srcVersion, category, tags, svg string
}

func main() {
	lucideDir := flag.String("lucide", "", "lucide-static package dir (contains icons/ and tags.json)")
	tablerDir := flag.String("tabler", "", "@tabler/icons package dir (contains icons.json and icons/outline)")
	dryRun := flag.Bool("dry-run", false, "parse and report only; do not write")
	flag.Parse()
	if *lucideDir == "" && *tablerDir == "" {
		fmt.Fprintln(os.Stderr, "usage: icon-seed --lucide DIR [--tabler DIR] [--dry-run]")
		os.Exit(2)
	}

	var all []icon
	seen := map[string]bool{}
	if *lucideDir != "" {
		icons, err := loadLucide(*lucideDir)
		fail(err)
		for _, ic := range icons {
			seen[ic.name] = true
			all = append(all, ic)
		}
	}
	if *tablerDir != "" {
		icons, err := loadTabler(*tablerDir)
		fail(err)
		skipped := 0
		for _, ic := range icons {
			if seen[ic.name] {
				skipped++
				continue
			}
			all = append(all, ic)
		}
		fmt.Printf("tabler: %d icons, %d skipped (name exists in an earlier set)\n", len(icons), skipped)
	}
	fmt.Printf("total to import: %d\n", len(all))
	if *dryRun {
		return
	}

	db, err := openDB()
	fail(err)
	defer db.Close()
	inserted, updated, same := 0, 0, 0
	for _, ic := range all {
		var ver int
		var oldSvg, oldTags, oldCat, oldSet string
		err := db.QueryRow(`SELECT version, svg_body, tags_en, category_en, source_set
			FROM shared.icon_library WHERE name=$1`, ic.name).Scan(&ver, &oldSvg, &oldTags, &oldCat, &oldSet)
		switch {
		case err == sql.ErrNoRows:
			_, err = db.Exec(`INSERT INTO shared.icon_library
				(name, source_set, source_version, category_en, tags_en, svg_body, creator)
				VALUES ($1,$2,$3,$4,$5,$6,'icon-seed')`,
				ic.name, ic.set, ic.srcVersion, ic.category, ic.tags, ic.svg)
			fail(err)
			inserted++
		case err != nil:
			fail(err)
		case oldSet != ic.set:
			same++ // a custom icon of the same name wins; never overwritten
		case oldSvg == ic.svg && oldTags == ic.tags && oldCat == ic.category:
			same++
		default:
			_, err = db.Exec(`UPDATE shared.icon_library SET source_version=$2, category_en=$3,
				tags_en=$4, svg_body=$5, version=version+1, updated_at=NOW() WHERE name=$1`,
				ic.name, ic.srcVersion, ic.category, ic.tags, ic.svg)
			fail(err)
			updated++
		}
	}
	fmt.Printf("inserted=%d updated=%d unchanged=%d\n", inserted, updated, same)
	report(db)
}

func report(db *sql.DB) {
	var total, noTags, noDescEN, noDescZH, noEmb int
	fail(db.QueryRow(`SELECT count(*),
		count(*) FILTER (WHERE tags_en=''),
		count(*) FILTER (WHERE description_en=''),
		count(*) FILTER (WHERE description_zh='' OR tags_zh='' OR category_zh=''),
		count(*) FILTER (WHERE embedding IS NULL) FROM shared.icon_library`).
		Scan(&total, &noTags, &noDescEN, &noDescZH, &noEmb))
	fmt.Printf("library: %d icons; missing tags=%d, en description=%d, zh fields=%d, embedding=%d\n",
		total, noTags, noDescEN, noDescZH, noEmb)
}

func loadLucide(dir string) ([]icon, error) {
	ver := pkgVersion(filepath.Join(dir, "package.json"))
	raw, err := os.ReadFile(filepath.Join(dir, "tags.json"))
	if err != nil {
		return nil, err
	}
	tags := map[string][]string{}
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "icons", "*.svg"))
	if err != nil {
		return nil, err
	}
	var out []icon
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".svg")
		svg, err := readSVG(f)
		if err != nil {
			return nil, err
		}
		out = append(out, icon{name: name, set: "lucide", srcVersion: ver,
			tags: strings.Join(tags[name], " "), svg: svg})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func loadTabler(dir string) ([]icon, error) {
	ver := pkgVersion(filepath.Join(dir, "package.json"))
	raw, err := os.ReadFile(filepath.Join(dir, "icons.json"))
	if err != nil {
		return nil, err
	}
	meta := map[string]struct {
		Category string `json:"category"`
		Tags     []any  `json:"tags"`
	}{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	var out []icon
	for name, m := range meta {
		f := filepath.Join(dir, "icons", "outline", name+".svg")
		if _, err := os.Stat(f); err != nil {
			continue // filled-only icon: library is outline only
		}
		svg, err := readSVG(f)
		if err != nil {
			return nil, err
		}
		out = append(out, icon{name: name, set: "tabler", srcVersion: ver,
			category: m.Category, tags: joinTags(m.Tags), svg: svg})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// readSVG returns the file with the licence comment removed.
func readSVG(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	for strings.HasPrefix(s, "<!--") {
		end := strings.Index(s, "-->")
		if end < 0 {
			break
		}
		s = strings.TrimSpace(s[end+3:])
	}
	if !strings.HasPrefix(s, "<svg") {
		return "", fmt.Errorf("%s: not an svg", path)
	}
	return s, nil
}

func pkgVersion(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var p struct{ Version string }
	_ = json.Unmarshal(b, &p)
	return p.Version
}

func openDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("PG_HOST"), os.Getenv("PG_PORT"), os.Getenv("PG_USER_NAME"),
		os.Getenv("PG_PASSWORD"), os.Getenv("PG_DB_NAME"))
	return sql.Open("postgres", dsn)
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func joinTags(tags []any) string {
	parts := make([]string, 0, len(tags))
	for _, t := range tags {
		parts = append(parts, fmt.Sprint(t))
	}
	return strings.Join(parts, " ")
}
