package web

import (
	"encoding/json"
	"io"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Meta is metadata extracted from an HTML document.
type Meta struct {
	Title       string
	Description string
	SiteName    string // og:site_name
	Generator   string // e.g. "WordPress 6.6"
	JSONLD      []Entity
}

// Entity is a top-level JSON-LD node (including nodes inside @graph).
type Entity struct {
	Types  []string
	Name   string
	SameAs []string
}

// ParseHTML streams through an HTML document collecting metadata. It never
// builds a DOM, so memory stays proportional to the largest single token.
func ParseHTML(r io.Reader) (Meta, error) {
	var (
		m           Meta
		z           = html.NewTokenizer(r)
		inTitle     bool
		inLD        bool
		ld          strings.Builder
		ogDesc      string
		description string
	)
	for {
		switch z.Next() {
		case html.ErrorToken:
			if z.Err() == io.EOF {
				if description == "" {
					description = ogDesc
				}
				m.Description = clean(description)
				return m, nil
			}
			return m, z.Err()

		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			attrs := map[string]string{}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				attrs[string(k)] = string(v)
			}
			switch atom.Lookup(name) {
			case atom.Title:
				inTitle = m.Title == ""
			case atom.Script:
				if strings.EqualFold(strings.TrimSpace(attrs["type"]), "application/ld+json") {
					inLD = true
					ld.Reset()
				}
			case atom.Meta:
				content := attrs["content"]
				switch strings.ToLower(attrs["name"] + attrs["property"]) {
				case "description":
					description = content
				case "og:description":
					ogDesc = content
				case "og:site_name":
					m.SiteName = clean(content)
				case "generator":
					m.Generator = clean(content)
				}
			}

		case html.TextToken:
			if inTitle {
				m.Title = clean(string(z.Text()))
			} else if inLD {
				ld.Write(z.Text())
			}

		case html.EndTagToken:
			name, _ := z.TagName()
			switch atom.Lookup(name) {
			case atom.Title:
				inTitle = false
			case atom.Script:
				if inLD {
					inLD = false
					var v any
					if json.Unmarshal([]byte(ld.String()), &v) == nil {
						collect(v, &m.JSONLD)
					}
				}
			}
		}
	}
}

// collect walks a decoded JSON-LD value and appends typed top-level nodes.
func collect(v any, out *[]Entity) {
	switch v := v.(type) {
	case []any:
		for _, e := range v {
			collect(e, out)
		}
	case map[string]any:
		if g, ok := v["@graph"]; ok {
			collect(g, out)
		}
		types := strings_(v["@type"])
		if len(types) == 0 {
			return
		}
		name := first(strings_(v["name"]))
		if name == "" {
			name = first(strings_(v["headline"]))
		}
		*out = append(*out, Entity{Types: types, Name: clean(name), SameAs: strings_(v["sameAs"])})
	}
}

// strings_ accepts a JSON string or array of strings.
func strings_(v any) []string {
	switch v := v.(type) {
	case string:
		if v = strings.TrimSpace(v); v != "" {
			return []string{v}
		}
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return slices.Compact(out)
	}
	return nil
}

func first(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// clean collapses whitespace so values fit on one line.
func clean(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}
