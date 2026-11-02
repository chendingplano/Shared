// Package emojibuild constructs a complete catalogue from pinned Unicode,
// CLDR and Twemoji source files. It does not access the network.
package emojibuild

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/chendingplano/shared/go/api/iconlib"
)

type Options struct {
	Inventory, SVGDir              string
	English, Chinese               []string
	ChineseNames                   map[string]string
	ChineseKeywords                map[string]string
	DescriptionsEN, DescriptionsZH map[string]string
}
type Emoji struct {
	ID            string `json:"unicode_id"`
	Unicode       string `json:"unicode_value"`
	NameEN        string `json:"name_en"`
	NameZH        string `json:"name_zh"`
	CategoryID    string `json:"category_id"`
	CategoryEN    string `json:"category_en"`
	CategoryZH    string `json:"category_zh"`
	KeywordsEN    string `json:"keywords_en"`
	KeywordsZH    string `json:"keywords_zh"`
	DescriptionEN string `json:"description_en"`
	DescriptionZH string `json:"description_zh"`
	SVG           string `json:"svg_body"`
	Order         int    `json:"sort_order"`
}
type annotation struct{ Name, Keywords string }
type xmlFile struct {
	Items []struct {
		CP   string `xml:"cp,attr"`
		Type string `xml:"type,attr"`
		Text string `xml:",chardata"`
	} `xml:"annotations>annotation"`
}

func UnicodeID(s string) string {
	p := make([]string, 0, len(s))
	for _, r := range s {
		p = append(p, fmt.Sprintf("%x", r))
	}
	return strings.Join(p, "-")
}
func annotationKey(s string) string { return strings.ReplaceAll(s, "\ufe0f", "") }
func loadAnnotations(paths []string) (map[string]annotation, error) {
	out := map[string]annotation{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var f xmlFile
		if err = xml.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, v := range f.Items {
			k := annotationKey(v.CP)
			a := out[k]
			if v.Type == "tts" {
				a.Name = strings.TrimSpace(v.Text)
			} else if v.Type == "" {
				a.Keywords = strings.Join(strings.Split(strings.TrimSpace(v.Text), " | "), " ")
			}
			out[k] = a
		}
	}
	return out, nil
}

var categories = map[string][2]string{
	"Smileys & Emotion": {"smileys-emotion", "笑脸与情感"}, "People & Body": {"people-body", "人物与身体"},
	"Animals & Nature": {"animals-nature", "动物与自然"}, "Food & Drink": {"food-drink", "食物与饮料"},
	"Travel & Places": {"travel-places", "旅行与地点"}, "Activities": {"activities", "活动"},
	"Objects": {"objects", "物品"}, "Symbols": {"symbols", "符号"}, "Flags": {"flags", "旗帜"},
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func chineseName(id, original string, overrides map[string]string) string {
	if name := strings.TrimSpace(overrides[id]); name != "" {
		return name
	}
	return original
}

func description(id, original string, overrides map[string]string) string {
	if value := strings.TrimSpace(overrides[id]); value != "" {
		return value
	}
	return original
}

func validateSVG(s string) error {
	d := xml.NewDecoder(strings.NewReader(s))
	for {
		t, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if element, ok := t.(xml.StartElement); ok {
			name := strings.ToLower(element.Name.Local)
			if name == "script" || name == "foreignobject" || name == "image" || name == "style" {
				return fmt.Errorf("unsafe SVG element %s", name)
			}
			for _, a := range element.Attr {
				key, value := strings.ToLower(a.Name.Local), strings.ToLower(strings.TrimSpace(a.Value))
				if strings.HasPrefix(key, "on") || key == "href" || strings.Contains(value, "javascript:") || strings.Contains(value, "url(http") || strings.Contains(value, "data:") {
					return fmt.Errorf("unsafe SVG attribute %s", key)
				}
			}
		}
	}
}

// Validate rejects incomplete or unsafe records before a transaction starts.
func Validate(all []Emoji) error {
	if len(all) == 0 {
		return fmt.Errorf("empty catalogue")
	}
	seen := map[string]bool{}
	for _, e := range all {
		if e.ID == "" || e.ID != UnicodeID(e.Unicode) || seen[e.ID] {
			return fmt.Errorf("invalid or duplicate Unicode ID %q", e.ID)
		}
		seen[e.ID] = true
		for _, v := range []string{e.NameEN, e.NameZH, e.CategoryID, e.CategoryEN, e.CategoryZH, e.KeywordsEN, e.KeywordsZH, e.DescriptionEN, e.DescriptionZH, e.SVG} {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("%s: incomplete metadata", e.ID)
			}
		}
		if !containsHan(e.NameZH) || !containsHan(e.KeywordsZH) || !containsHan(e.DescriptionZH) || !containsHan(e.CategoryZH) {
			return fmt.Errorf("%s: missing Chinese translation", e.ID)
		}
		if err := validateSVG(e.SVG); err != nil {
			return fmt.Errorf("%s: %w", e.ID, err)
		}
	}
	return nil
}

// Build validates every fully-qualified inventory entry before returning any data.
func Build(o Options) ([]Emoji, error) {
	en, err := loadAnnotations(o.English)
	if err != nil {
		return nil, err
	}
	zh, err := loadAnnotations(o.Chinese)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(o.Inventory)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Emoji
	var problems []string
	var group string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# group: ") {
			group = strings.TrimSpace(strings.TrimPrefix(line, "# group: "))
			continue
		}
		pieces := strings.SplitN(line, ";", 2)
		if len(pieces) != 2 || !strings.HasPrefix(strings.TrimSpace(pieces[1]), "fully-qualified ") {
			continue
		}
		var chars strings.Builder
		for _, hex := range strings.Fields(pieces[0]) {
			v, e := strconv.ParseInt(hex, 16, 32)
			if e != nil {
				return nil, e
			}
			chars.WriteRune(rune(v))
		}
		value := chars.String()
		id := UnicodeID(value)
		c, ok := categories[group]
		if !ok {
			problems = append(problems, id+": unknown category "+group)
			continue
		}
		a, b := en[annotationKey(value)], zh[annotationKey(value)]
		b.Name = chineseName(id, b.Name, o.ChineseNames)
		b.Keywords = chineseName(id, b.Keywords, o.ChineseKeywords)
		if a.Name == "" || a.Keywords == "" || b.Name == "" || b.Keywords == "" || !containsHan(b.Name) || !containsHan(b.Keywords) {
			problems = append(problems, id+": missing bilingual annotations")
			continue
		}
		path := filepath.Join(o.SVGDir, id+".svg")
		data, e := os.ReadFile(path)
		if os.IsNotExist(e) {
			path = filepath.Join(o.SVGDir, UnicodeID(annotationKey(value))+".svg")
			data, e = os.ReadFile(path)
		}
		if e != nil {
			problems = append(problems, id+": missing artwork")
			continue
		}
		if e := validateSVG(string(data)); e != nil {
			problems = append(problems, id+": unsafe SVG")
			continue
		}
		svg, e := iconlib.SanitizeSVG(string(data))
		if e != nil {
			problems = append(problems, id+": invalid SVG")
			continue
		}
		out = append(out, Emoji{ID: id, Unicode: value, NameEN: a.Name, NameZH: b.Name, CategoryID: c[0], CategoryEN: group, CategoryZH: c[1], KeywordsEN: a.Keywords, KeywordsZH: b.Keywords, DescriptionEN: description(id, a.Name, o.DescriptionsEN), DescriptionZH: description(id, b.Name, o.DescriptionsZH), SVG: svg, Order: len(out)})
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("catalogue incomplete (%d): %s", len(problems), strings.Join(problems, ", "))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty catalogue")
	}
	if err := Validate(out); err != nil {
		return nil, err
	}
	return out, nil
}
