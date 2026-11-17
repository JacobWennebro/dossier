// Command web serves dossier output as HTML, with the terminal colors kept.
//
//	GET /                 the output for the requested hostname, or DEFAULT_DOMAIN (example.com unless set)
//	GET /{domain}         the output of "dossier {domain}" as colored HTML
//	GET /raw/{domain}     the raw ANSI output of "dossier -color always {domain}"
package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"dossier/internal/cli"
)

//go:embed index.html
var indexHTML string

//go:embed favicon.svg
var faviconSVG string

var page = template.Must(template.New("index").Parse(indexHTML))

// domainRE accepts DNS hostnames with at least two labels and an alphabetic
// (or punycode) TLD, which also rules out IP literals and bare names such as
// "localhost".
var domainRE = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+(?:[a-z]{2,63}|xn--[a-z0-9-]{1,59})$`)

func normalizeDomain(s string) (string, bool) {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
	if len(d) > 253 || !domainRE.MatchString(d) {
		return "", false
	}
	return d, true
}

func main() {
	addr := ":" + envOr("PORT", "8080")

	mux := http.NewServeMux()
	serveFavicon := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		io.WriteString(w, faviconSVG)
	}
	mux.HandleFunc("GET /favicon.svg", serveFavicon)
	mux.HandleFunc("GET /favicon.ico", serveFavicon) // for clients that ignore the <link>; modern browsers accept SVG here
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("User-agent: *\nDisallow: /raw/\n"))
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	lookups := newCache(2*time.Minute, 10000)

	// showResult sends the top of the page right away (the browser shows the
	// form and, if the lookup is slow, a loading line), then streams the
	// output into the same response when it is ready.
	showResult := func(w http.ResponseWriter, r *http.Request, d string) {
		res := lookups.get(d)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		execute(w, r, "head", d)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-res.done:
		case <-r.Context().Done():
			return
		}
		io.WriteString(w, `<pre class="out">`+ansiToHTML(string(res.out))+"</pre>\n")
		execute(w, r, "tail", d)
	}

	// The root page shows the lookup for the hostname it was requested on,
	// or DEFAULT_DOMAIN when that is not a public domain (localhost, an IP).
	defaultDomain, ok := normalizeDomain(envOr("DEFAULT_DOMAIN", "example.com"))
	if !ok {
		log.Fatalf("invalid DEFAULT_DOMAIN %q", os.Getenv("DEFAULT_DOMAIN"))
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		showResult(w, r, rootDomain(r, defaultDomain))
	})

	mux.HandleFunc("GET /{domain}", func(w http.ResponseWriter, r *http.Request) {
		d, ok := normalizeDomain(r.PathValue("domain"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		if d != r.PathValue("domain") { // e.g. /Google.com. → /google.com
			lookups.get(d) // start now so the result is ready when the redirect lands
			http.Redirect(w, r, "/"+d, http.StatusFound)
			return
		}
		showResult(w, r, d)
	})
	mux.HandleFunc("GET /go", func(w http.ResponseWriter, r *http.Request) {
		if d, ok := normalizeDomain(r.FormValue("d")); ok {
			lookups.get(d)
			http.Redirect(w, r, "/"+d, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("GET /raw/{domain}", func(w http.ResponseWriter, r *http.Request) {
		d, ok := normalizeDomain(r.PathValue("domain"))
		if !ok {
			http.Error(w, "invalid domain", http.StatusBadRequest)
			return
		}
		res := lookups.get(d)
		select {
		case <-res.done:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(res.out)
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

const releasesURL = "https://github.com/JacobWennebro/dossier/releases"

// pageData is what the templates see.
type pageData struct {
	Domain      string
	OS          string   // "Windows", "Mac", "Linux" or "" when unknown
	OtherOS     []string // the platforms not detected, for the "also for" line
	ReleasesURL string
}

var platforms = []string{"Windows", "Mac", "Linux"}

func execute(w http.ResponseWriter, r *http.Request, name, domain string) {
	w.Header().Add("Vary", "User-Agent")
	plat := detectOS(r.UserAgent())
	data := pageData{Domain: domain, OS: plat, ReleasesURL: releasesURL}
	for _, p := range platforms {
		if p != plat {
			data.OtherOS = append(data.OtherOS, p)
		}
	}
	if err := page.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
	}
}

// detectOS maps a User-Agent to a desktop platform dossier has builds for.
// Phones, tablets and anything unrecognized yield "". iPads that request
// desktop sites report themselves as Macs and cannot be told apart.
func detectOS(ua string) string {
	switch {
	case strings.Contains(ua, "Windows NT") && !strings.Contains(ua, "Windows Phone"):
		return "Windows"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"), strings.Contains(ua, "Android"):
		return ""
	case strings.Contains(ua, "Macintosh"), strings.Contains(ua, "Mac OS X"):
		return "Mac"
	case strings.Contains(ua, "Linux"), strings.Contains(ua, "X11"), strings.Contains(ua, "CrOS"):
		return "Linux"
	}
	return ""
}

// cache runs each domain's lookup once, shares it between concurrent
// requests and keeps the output for ttl.
type cache struct {
	mu         sync.Mutex
	m          map[string]*result
	ttl        time.Duration
	maxEntries int
}

type result struct {
	done chan struct{} // closed when out and at are set
	out  []byte
	at   time.Time
}

func newCache(ttl time.Duration, maxEntries int) *cache {
	return &cache{m: map[string]*result{}, ttl: ttl, maxEntries: maxEntries}
}

func (r *result) fresh(ttl time.Duration) bool {
	select {
	case <-r.done:
		return time.Since(r.at) < ttl
	default:
		return true // still running
	}
}

// get returns the cached or in-flight lookup for domain, starting one if
// needed. The lookup is not tied to any request, so a visitor who leaves
// early still fills the cache; the CLI's own timeout bounds it.
func (c *cache) get(domain string) *result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := c.m[domain]; ok && r.fresh(c.ttl) {
		return r
	}
	if len(c.m) >= c.maxEntries {
		for k, r := range c.m {
			if !r.fresh(c.ttl) {
				delete(c.m, k)
			}
		}
	}
	r := &result{done: make(chan struct{})}
	c.m[domain] = r
	go func() {
		// stdout and stderr share one buffer, so lookup errors appear below
		// the card just as they would in a terminal.
		var out bytes.Buffer
		cli.Run(context.Background(), []string{"-color", "always", domain}, &out, &out)
		r.out, r.at = out.Bytes(), time.Now()
		close(r.done)
	}()
	return r
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// rootDomain returns the domain the request was addressed to, taken from the
// Host header without any port, or fallback if that is not a valid domain.
func rootDomain(r *http.Request, fallback string) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if d, ok := normalizeDomain(host); ok {
		return d
	}
	return fallback
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
