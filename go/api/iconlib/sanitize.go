package iconlib

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Elements and attributes allowed in a library icon. Anything else (script,
// foreignObject, style, event handlers, external references) is dropped.
var allowedElems = map[string]bool{
	"svg": true, "g": true, "path": true, "circle": true, "ellipse": true, "line": true,
	"polyline": true, "polygon": true, "rect": true, "defs": true, "clipPath": true,
	"mask": true, "title": true, "desc": true,
}

var allowedAttrs = map[string]bool{
	"d": true, "x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true,
	"cx": true, "cy": true, "r": true, "rx": true, "ry": true, "width": true, "height": true,
	"points": true, "viewBox": true, "fill": true, "stroke": true, "stroke-width": true,
	"stroke-linecap": true, "stroke-linejoin": true, "stroke-miterlimit": true,
	"stroke-dasharray": true, "stroke-dashoffset": true, "stroke-opacity": true,
	"fill-opacity": true, "fill-rule": true, "clip-rule": true, "opacity": true,
	"transform": true, "id": true, "clip-path": true, "mask": true, "xmlns": true,
	"preserveAspectRatio": true,
}

// SanitizeSVG returns a re-serialised copy of svg containing only allowed
// elements and attributes. Attribute values that reference external resources
// (url(http…), javascript:, data:) are removed.
func SanitizeSVG(svg string) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(svg))
	dec.Strict = true
	var out bytes.Buffer
	var stack []bool // true when the element was emitted
	depth, sawRoot := 0, false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("invalid svg: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !sawRoot && t.Name.Local != "svg" {
				return "", errors.New("root element must be <svg>")
			}
			sawRoot = true
			keep := allowedElems[t.Name.Local] && (len(stack) == 0 || stack[len(stack)-1])
			stack = append(stack, keep)
			if !keep {
				continue
			}
			depth++
			out.WriteString("<" + t.Name.Local)
			if t.Name.Local == "svg" && depth == 1 {
				out.WriteString(` xmlns="http://www.w3.org/2000/svg"`)
			}
			for _, a := range t.Attr {
				name := a.Name.Local
				if a.Name.Space != "" && a.Name.Space != "xmlns" || name == "xmlns" || !allowedAttrs[name] || unsafeValue(a.Value) {
					continue
				}
				fmt.Fprintf(&out, ` %s="%s"`, name, escapeAttr(a.Value))
			}
			out.WriteString(">")
		case xml.EndElement:
			keep := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if keep {
				depth--
				out.WriteString("</" + t.Name.Local + ">")
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] {
				xml.EscapeText(&out, t)
			}
		}
	}
	if !sawRoot || depth != 0 {
		return "", errors.New("invalid svg: no root element")
	}
	return out.String(), nil
}

func unsafeValue(v string) bool {
	l := strings.ToLower(strings.ReplaceAll(v, " ", ""))
	return strings.Contains(l, "javascript:") || strings.Contains(l, "data:") ||
		strings.Contains(l, "url(http") || strings.Contains(l, "url('http") || strings.Contains(l, `url("http`) ||
		strings.Contains(l, "//")
}

func escapeAttr(v string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(v))
	return strings.ReplaceAll(b.String(), `"`, "&#34;")
}
