package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const page = `<!doctype html><html><head>
<title>  Example
  Corp </title>
<meta name="description" content="We make &amp; sell things.">
<meta property="og:site_name" content="Example">
<meta name="generator" content="WordPress 6.6">
<script type="application/ld+json">
{"@context":"https://schema.org","@graph":[
  {"@type":"Organization","name":"Example Corp","sameAs":["https://x.com/example","https://github.com/example"]},
  {"@type":["WebSite","Thing"],"name":"Example"}
]}
</script>
<script type="application/ld+json">{"@type":"Article","headline":"Hello"}</script>
<script type="application/ld+json">{ not json </script>
</head><body><title>ignored</title></body></html>`

func TestParseHTML(t *testing.T) {
	m, err := ParseHTML(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Example Corp" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.Description != "We make & sell things." {
		t.Errorf("Description = %q", m.Description)
	}
	if m.SiteName != "Example" || m.Generator != "WordPress 6.6" {
		t.Errorf("SiteName = %q, Generator = %q", m.SiteName, m.Generator)
	}
	if len(m.JSONLD) != 3 {
		t.Fatalf("JSONLD = %+v, want 3 entities", m.JSONLD)
	}
	if e := m.JSONLD[0]; e.Types[0] != "Organization" || e.Name != "Example Corp" || len(e.SameAs) != 2 {
		t.Errorf("JSONLD[0] = %+v", e)
	}
	if e := m.JSONLD[1]; len(e.Types) != 2 || e.Name != "Example" {
		t.Errorf("JSONLD[1] = %+v", e)
	}
	if e := m.JSONLD[2]; e.Types[0] != "Article" || e.Name != "Hello" {
		t.Errorf("JSONLD[2] = %+v", e)
	}
}

func TestParseHTMLOGDescriptionFallback(t *testing.T) {
	m, _ := ParseHTML(strings.NewReader(`<meta property="og:description" content="og">`))
	if m.Description != "og" {
		t.Errorf("Description = %q, want og fallback", m.Description)
	}
}

func TestFetch(t *testing.T) {
	var gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.UserAgent()
		w.Header().Set("Server", "test-server")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(page))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := NewClient("dossier/test")
	c.Transport.(*uaTransport).base = srv.Client().Transport

	p, err := Fetch(context.Background(), c, srv.URL+"/old")
	if err != nil {
		t.Fatal(err)
	}
	if gotUA != "dossier/test" {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if p.Status != "200 OK" || p.Server != "test-server" || p.Redirects != 1 {
		t.Errorf("Page = %+v", p)
	}
	if p.TLSVersion == "" || p.CertExpiry.IsZero() {
		t.Errorf("missing TLS details: %+v", p)
	}
	if p.Title != "Example Corp" {
		t.Errorf("Title = %q", p.Title)
	}
}

func TestFetchTooManyRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := Fetch(context.Background(), NewClient("t"), srv.URL+"/"); err == nil {
		t.Error("expected redirect limit error")
	}
}
