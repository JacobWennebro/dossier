// Package web fetches a domain's homepage and extracts lightweight metadata:
// HTTP/TLS details, title, description and JSON-LD structured data.
package web

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	maxBody      = 1 << 20 // read at most 1 MiB of HTML
	maxRedirects = 5

	// Short connect/handshake limits so a dead HTTPS port fails fast and
	// leaves time for the plain HTTP fallback within the overall deadline.
	dialTimeout      = 2 * time.Second
	handshakeTimeout = 2 * time.Second
)

// Page is what was learned from fetching a homepage.
type Page struct {
	URL       string // final URL after redirects
	Redirects int
	Status    string // e.g. "200 OK"
	Proto     string // e.g. "HTTP/2"
	Server    string

	HTTPSError string // set when HTTPS failed and the page was fetched over HTTP
	TLSVersion string // empty for plain HTTP
	CertIssuer string
	CertExpiry time.Time

	Meta
}

// NewClient returns an HTTP client suitable for Fetch.
func NewClient(userAgent string) *http.Client {
	return &http.Client{
		Transport: &uaTransport{ua: userAgent, base: newTransport()},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
}

// FetchDomain fetches https://domain/, falling back to http:// if the HTTPS
// request fails outright (not on HTTP error statuses).
func FetchDomain(ctx context.Context, c *http.Client, domain string) (*Page, error) {
	p, err := Fetch(ctx, c, "https://"+domain+"/")
	if err == nil || ctx.Err() != nil {
		return p, err
	}
	if p2, err2 := Fetch(ctx, c, "http://"+domain+"/"); err2 == nil {
		p2.HTTPSError = err.Error()
		return p2, nil
	}
	return nil, err
}

// Fetch GETs rawURL and parses the response.
func Fetch(ctx context.Context, c *http.Client, rawURL string) (*Page, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")

	resp, err := c.Do(req)
	if err != nil {
		var uerr interface{ Unwrap() error }
		if errors.As(err, &uerr) {
			err = uerr.Unwrap() // drop the repetitive `Get "https://...":` prefix
		}
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			err = errors.New("timed out")
		}
		return nil, err
	}
	defer resp.Body.Close()

	p := &Page{
		URL:    resp.Request.URL.String(),
		Status: resp.Status,
		Proto:  strings.TrimSuffix(resp.Proto, ".0"),
		Server: resp.Header.Get("Server"),
	}
	for r := resp.Request; r.Response != nil; r = r.Response.Request {
		p.Redirects++
	}
	if resp.TLS != nil {
		p.TLSVersion = tls.VersionName(resp.TLS.Version)
		if certs := resp.TLS.PeerCertificates; len(certs) > 0 {
			p.CertIssuer = issuer(certs[0].Issuer.Organization, certs[0].Issuer.CommonName)
			p.CertExpiry = certs[0].NotAfter
		}
	}

	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt == "text/html" || mt == "application/xhtml+xml" {
		meta, err := ParseHTML(io.LimitReader(resp.Body, maxBody))
		if err != nil {
			return p, nil // metadata is best effort; the HTTP details still stand
		}
		p.Meta = meta
	}
	return p, nil
}

func newTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	t.TLSHandshakeTimeout = handshakeTimeout
	return t
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func issuer(org []string, cn string) string {
	switch {
	case len(org) > 0 && cn != "":
		return org[0] + " (" + cn + ")"
	case len(org) > 0:
		return org[0]
	default:
		return cn
	}
}

type uaTransport struct {
	ua   string
	base http.RoundTripper
}

func (t *uaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", t.ua)
	return t.base.RoundTrip(r)
}
