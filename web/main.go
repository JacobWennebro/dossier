// Command web serves dossier output as HTML, with the terminal colors kept.
//
//	GET /                 the output for the requested hostname, or DEFAULT_DOMAIN (example.com unless set)
//	GET /{domain}         the output of "dossier {domain}" as colored HTML
//	GET /og/{domain}.png  a 1200×630 preview image of the output, for og:image
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
	lookups := newCache(30*time.Minute, 10000)

	// showResult sends the top of the page right away (the browser shows the
	// form and, if the lookup is slow, a loading line), then streams the
	// output into the same response when it is ready.
	showResult := func(w http.ResponseWriter, r *http.Request, d string) {
		res := lookups.get(d)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		// Link-preview bots read the meta tags but run no scripts, so they
		// wait for the result to get the right theme color in <head>.
		if isPreviewBot(r.UserAgent()) {
			select {
			case <-res.done:
			case <-r.Context().Done():
				return
			}
		}
		theme := defaultThemeColor
		if res.finished() {
			theme = res.keyColor(d)
		}
		execute(w, r, "head", d, theme)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-res.done:
		case <-r.Context().Done():
			return
		}
		io.WriteString(w, `<pre class="out">`+ansiToHTML(string(res.out))+"</pre>\n")
		if c := res.keyColor(d); c != theme {
			// c is always "#rrggbb" from our own palette, so it is safe to inline.
			io.WriteString(w, `<script>document.querySelector('meta[name="theme-color"]').content="`+c+`"</script>`+"\n")
		}
		execute(w, r, "tail", d, "")
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
	// Preview image for link unfurls (Slack, Discord, X, iMessage, ...),
	// referenced from the og:image meta tag.
	mux.HandleFunc("GET /og/{file}", func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutSuffix(r.PathValue("file"), ".png")
		d, valid := normalizeDomain(name)
		if !ok || !valid {
			http.NotFound(w, r)
			return
		}
		res := lookups.get(d)
		select {
		case <-res.done:
		case <-r.Context().Done():
			return
		}
		png, err := res.image(d)
		if err != nil {
			log.Printf("og image %s: %v", d, err)
			http.Error(w, "could not render image", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(png)
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
	ThemeColor  string   // for <meta name="theme-color">: the CLI's key color for this domain
	Version     string   // dossier version this server was built from
	VersionURL  string   // release page for Version, or "" for development builds
	PageURL     string   // absolute URL of this page, for og:url
	ImageURL    string   // absolute URL of the preview image, for og:image
	OS          string   // "Windows", "Mac", "Linux" or "" when unknown
	OtherOS     []string // the platforms not detected, for the "also for" line
	ReleasesURL string
}

var platforms = []string{"Windows", "Mac", "Linux"}

func execute(w http.ResponseWriter, r *http.Request, name, domain, theme string) {
	w.Header().Add("Vary", "User-Agent")
	plat := detectOS(r.UserAgent())
	data := pageData{Domain: domain, ThemeColor: theme, OS: plat, ReleasesURL: releasesURL, Version: cli.Version()}
	if releaseTagRE.MatchString(data.Version) {
		data.VersionURL = releasesURL + "/tag/" + data.Version
	}
	if domain != "" {
		base := baseURL(r)
		data.PageURL = base + r.URL.Path
		data.ImageURL = base + "/og/" + domain + ".png"
	}
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

	ogOnce sync.Once // the preview image is rendered on first request
	og     []byte
	ogErr  error
}

func (r *result) finished() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// defaultThemeColor is used until a lookup's key color is known: the page
// background.
const defaultThemeColor = "#0d1117"

// keyColor returns the color the CLI used for the domain title and keys,
// which comes from the detected provider's logo, as "#rrggbb". Call it only
// after done is closed.
func (r *result) keyColor(domain string) string {
	segs := parseANSI(string(r.out))
	for _, want := range []string{domain, "Took"} { // the title, else the last key
		for _, s := range segs {
			if s.Text == want && s.Bold && s.FG != "" {
				return s.FG
			}
		}
	}
	return defaultThemeColor
}

// image returns the result's preview PNG, rendering it once. Call it only
// after done is closed.
func (r *result) image(domain string) ([]byte, error) {
	r.ogOnce.Do(func() { r.og, r.ogErr = renderOGImage(r.out, domain) })
	return r.og, r.ogErr
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

// releaseTagRE matches the tags the release workflow creates.
var releaseTagRE = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// isPreviewBot reports whether ua belongs to a link-preview crawler.
func isPreviewBot(ua string) bool {
	ua = strings.ToLower(ua)
	for _, s := range []string{"bot", "facebookexternalhit", "embedly", "whatsapp", "slack", "discord", "telegram", "skype", "vkshare", "pinterest", "iframely"} {
		if strings.Contains(ua, s) {
			return true
		}
	}
	return false
}

// baseURL returns the scheme and host the request was made to. Behind a TLS
// proxy the scheme comes from X-Forwarded-Proto; without one, https is
// assumed except for localhost and IP addresses.
func baseURL(r *http.Request) string {
	scheme := "https"
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	switch p := r.Header.Get("X-Forwarded-Proto"); {
	case p == "http" || p == "https":
		scheme = p
	case r.TLS == nil && (host == "localhost" || net.ParseIP(strings.Trim(host, "[]")) != nil):
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
