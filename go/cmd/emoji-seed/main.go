// emoji-seed builds and validates a local catalogue before importing it.
package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chendingplano/shared/go/api/emojibuild"
	"github.com/chendingplano/shared/go/api/emojilib"
	_ "github.com/lib/pq"
)

//go:embed zh-name-overrides.json
var reviewedNames []byte

func main() {
	input := flag.String("input", "", "directory with emoji-test.txt, cldr/ and twemoji/assets/svg")
	output := flag.String("output", "", "write validated catalogue JSON to this path")
	overrides := flag.String("overrides", "", "reviewed JSON translations for gaps, keyed by Unicode ID")
	importDB := flag.Bool("import", false, "atomically import the validated catalogue using standard PG environment variables")
	flag.Parse()
	if *input == "" {
		fmt.Fprintln(os.Stderr, "--input is required")
		os.Exit(2)
	}
	if err := verifySources(*input); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p := func(s string) string { return filepath.Join(*input, s) }
	var names map[string]string
	if err := json.Unmarshal(reviewedNames, &names); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var extra struct {
		Names        map[string]string `json:"names_zh"`
		Keywords     map[string]string `json:"keywords_zh"`
		Descriptions map[string]string `json:"descriptions_zh"`
	}
	if *overrides != "" {
		raw, e := os.ReadFile(*overrides)
		if e == nil {
			e = json.Unmarshal(raw, &extra)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
	}
	for id, name := range extra.Names {
		names[id] = name
	}
	if extra.Descriptions == nil {
		extra.Descriptions = map[string]string{}
	}
	if extra.Descriptions["1f389"] == "" {
		extra.Descriptions["1f389"] = "用于庆祝的拉炮和彩带"
	}
	all, err := emojibuild.Build(emojibuild.Options{Inventory: p("emoji-test.txt"), SVGDir: p("twemoji/assets/svg"), English: []string{p("cldr/annotations-en.xml"), p("cldr/annotationsDerived-en.xml")}, Chinese: []string{p("cldr/annotations-zh.xml"), p("cldr/annotationsDerived-zh.xml")}, ChineseNames: names, ChineseKeywords: extra.Keywords, DescriptionsEN: map[string]string{"1f389": "A party popper for celebration"}, DescriptionsZH: extra.Descriptions})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *output != "" {
		data, e := json.Marshal(struct {
			Manifest json.RawMessage    `json:"source_manifest"`
			Emojis   []emojibuild.Emoji `json:"emojis"`
		}{manifest, all})
		if e == nil {
			e = os.WriteFile(*output, data, 0644)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
	}
	if *importDB {
		var versions struct {
			Unicode string `json:"unicode_version"`
			Twemoji struct {
				Version string `json:"version"`
			} `json:"twemoji"`
		}
		if err := json.Unmarshal(manifest, &versions); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		db, err := sql.Open("postgres", "")
		if err == nil {
			defer db.Close()
			err = emojilib.Import(context.Background(), db, all, emojilib.Source{TwemojiVersion: versions.Twemoji.Version, UnicodeVersion: versions.Unicode})
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "import failed:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("validated %d complete bilingual emoji\n", len(all))
}
