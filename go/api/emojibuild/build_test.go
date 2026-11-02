package emojibuild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPreservesSequencesAndRequiresBilingualMetadata(t *testing.T) {
	dir := t.TempDir()
	must := func(name, value string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	must("emoji-test.txt", "# group: Smileys & Emotion\n1F389 ; fully-qualified # 🎉 E0.6 party popper\n")
	must("svg/1f389.svg", `<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`)
	must("en.xml", `<ldml><annotations><annotation cp="🎉">celebration | party</annotation><annotation cp="🎉" type="tts">party popper</annotation></annotations></ldml>`)
	must("zh.xml", `<ldml><annotations><annotation cp="🎉">庆祝 | 礼花</annotation><annotation cp="🎉" type="tts">拉炮</annotation></annotations></ldml>`)
	opt := Options{Inventory: filepath.Join(dir, "emoji-test.txt"), SVGDir: filepath.Join(dir, "svg"), English: []string{filepath.Join(dir, "en.xml")}, Chinese: []string{filepath.Join(dir, "zh.xml")}}
	result, err := Build(opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Unicode != "🎉" || !strings.Contains(result[0].KeywordsZH, "庆祝") {
		t.Fatalf("unexpected result: %+v", result)
	}
	must("zh.xml", `<ldml><annotations><annotation cp="🎉" type="tts">party popper</annotation></annotations></ldml>`)
	if _, err := Build(opt); err == nil {
		t.Fatal("expected missing Chinese keywords to fail publication")
	}
}

func TestUnicodeIDPreservesJoinersAndVariationSelectors(t *testing.T) {
	if got := UnicodeID("👩\u200d❤️\u200d👩"); got != "1f469-200d-2764-fe0f-200d-1f469" {
		t.Fatal(got)
	}
}

func TestReviewedChineseNameOverride(t *testing.T) {
	if got := chineseName("1f44c", "OK", map[string]string{"1f44c": "OK手势"}); got != "OK手势" {
		t.Fatalf("override not applied: %q", got)
	}
}

func TestReviewedDescriptionOverride(t *testing.T) {
	if got := description("1f389", "party popper", map[string]string{"1f389": "celebration party popper"}); got != "celebration party popper" {
		t.Fatal(got)
	}
}

func TestRejectUnsafeSVG(t *testing.T) {
	for _, input := range []string{`<svg><script>alert(1)</script></svg>`, `<svg onload="alert(1)"></svg>`, `<svg><image href="https://example.org/x"/></svg>`} {
		if err := validateSVG(input); err == nil {
			t.Fatalf("accepted unsafe SVG: %s", input)
		}
	}
}

func TestValidateRejectsIncompleteRecord(t *testing.T) {
	if err := Validate([]Emoji{{ID: "1f389", Unicode: "🎉"}}); err == nil {
		t.Fatal("incomplete record was accepted")
	}
}

func TestReviewedChineseKeywordOverride(t *testing.T) {
	if got := chineseName("1f389", "", map[string]string{"1f389": "庆祝 礼花"}); got != "庆祝 礼花" {
		t.Fatal(got)
	}
}
