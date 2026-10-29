package iconlib

import (
	"strings"
	"testing"
)

func TestSanitizeStripsScriptAndHandlers(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" onload="x()">
<script>alert(1)</script><path d="M0 0h24" onclick="y()" fill="url(http://evil/x)"/>
<foreignObject><div>hi</div></foreignObject><circle cx="1" cy="1" r="1"/></svg>`
	got, err := SanitizeSVG(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"script", "alert", "onload", "onclick", "foreignObject", "evil", "<div"} {
		if strings.Contains(got, bad) {
			t.Errorf("output still contains %q: %s", bad, got)
		}
	}
	for _, good := range []string{`viewBox="0 0 24 24"`, `d="M0 0h24"`, "<circle"} {
		if !strings.Contains(got, good) {
			t.Errorf("output lost %q: %s", good, got)
		}
	}
}

func TestSanitizeRejectsNonSVG(t *testing.T) {
	if _, err := SanitizeSVG(`<html></html>`); err == nil {
		t.Fatal("expected error for non-svg root")
	}
	if _, err := SanitizeSVG(`not xml`); err == nil {
		t.Fatal("expected error for non-xml")
	}
}

func TestSanitizeKeepsLucideIcon(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 0 1 9-9"/></svg>`
	got, err := SanitizeSVG(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `xmlns="http://www.w3.org/2000/svg"`) {
		t.Fatalf("root lost xmlns: %s", got)
	}
	if !strings.Contains(got, `stroke="currentColor"`) || !strings.Contains(got, "<path") {
		t.Fatalf("lost content: %s", got)
	}
}
