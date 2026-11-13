// Package rdap looks up domain registration data via RDAP (RFC 9082/9083),
// the JSON successor to WHOIS. Servers are found through the IANA bootstrap
// registry (RFC 9224), which is cached on disk.
package rdap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultBootstrapURL = "https://data.iana.org/rdap/dns.json"
	DefaultCacheTTL     = 7 * 24 * time.Hour
	maxBody             = 1 << 20
	bootstrapFile       = "rdap-dns.json"
)

var (
	// ErrNotFound means the registry has no record of the domain.
	ErrNotFound = errors.New("domain not found in registry")
	// ErrNoServer means the TLD has no RDAP service in the IANA registry.
	ErrNoServer = errors.New("no RDAP server for this TLD")
)

// Domain is the registration data that dossier displays.
type Domain struct {
	Name       string
	Registrar  string
	Registered time.Time
	Expires    time.Time
	Updated    time.Time
	Status     []string // EPP-style statuses, e.g. "client transfer prohibited"
	DNSSEC     *bool    // nil when the registry does not say
}

// Client performs RDAP lookups. The zero value is not usable; set HTTP.
type Client struct {
	HTTP         *http.Client
	BootstrapURL string        // defaults to DefaultBootstrapURL
	CacheDir     string        // where the bootstrap file is cached; "" disables caching
	CacheTTL     time.Duration // defaults to DefaultCacheTTL
}

// Lookup returns registration data for domain. For subdomains such as
// www.example.co.uk it walks up the name until the registry recognises it.
func (c *Client) Lookup(ctx context.Context, domain string) (*Domain, error) {
	bs, err := c.bootstrap(ctx)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	for {
		base := bs.serverFor(name)
		if base == "" {
			return nil, ErrNoServer
		}
		d, err := c.query(ctx, base, name)
		if errors.Is(err, ErrNotFound) && strings.Count(name, ".") > 1 {
			name = name[strings.IndexByte(name, '.')+1:]
			continue
		}
		return d, err
	}
}

// --- bootstrap ---

type bootstrap map[string]string // TLD (or suffix) -> base URL

func (b bootstrap) serverFor(name string) string {
	labels := strings.Split(name, ".")
	for i := 1; i < len(labels); i++ { // longest suffix first, excluding the name itself
		if u, ok := b[strings.Join(labels[i:], ".")]; ok {
			return u
		}
	}
	return ""
}

func parseBootstrap(data []byte) (bootstrap, error) {
	var raw struct {
		Services [][][]string `json:"services"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	b := bootstrap{}
	for _, svc := range raw.Services {
		if len(svc) != 2 || len(svc[1]) == 0 {
			continue
		}
		base := svc[1][0]
		for _, u := range svc[1] { // prefer HTTPS
			if strings.HasPrefix(u, "https://") {
				base = u
				break
			}
		}
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		for _, tld := range svc[0] {
			b[strings.ToLower(tld)] = base
		}
	}
	if len(b) == 0 {
		return nil, errors.New("empty bootstrap registry")
	}
	return b, nil
}

func (c *Client) bootstrap(ctx context.Context) (bootstrap, error) {
	ttl := c.CacheTTL
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	var cachePath string
	var stale []byte
	if c.CacheDir != "" {
		cachePath = filepath.Join(c.CacheDir, bootstrapFile)
		if fi, err := os.Stat(cachePath); err == nil {
			if data, err := os.ReadFile(cachePath); err == nil {
				if time.Since(fi.ModTime()) < ttl {
					if b, err := parseBootstrap(data); err == nil {
						return b, nil
					}
				} else {
					stale = data
				}
			}
		}
	}

	u := c.BootstrapURL
	if u == "" {
		u = DefaultBootstrapURL
	}
	data, err := c.get(ctx, u, "application/json")
	if err == nil {
		var b bootstrap
		if b, err = parseBootstrap(data); err == nil {
			if cachePath != "" {
				writeFileAtomic(cachePath, data) // best effort
			}
			return b, nil
		}
	}
	if stale != nil { // registry unreachable: an old copy beats nothing
		if b, perr := parseBootstrap(stale); perr == nil {
			return b, nil
		}
	}
	return nil, err
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// --- domain query ---

type entity struct {
	Roles    []string `json:"roles"`
	VCard    []any    `json:"vcardArray"`
	Entities []entity `json:"entities"`
}

type domainResponse struct {
	LdhName string   `json:"ldhName"`
	Status  []string `json:"status"`
	Events  []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
	Entities  []entity `json:"entities"`
	SecureDNS *struct {
		DelegationSigned *bool `json:"delegationSigned"`
	} `json:"secureDNS"`
}

func (c *Client) query(ctx context.Context, base, name string) (*Domain, error) {
	data, err := c.get(ctx, base+"domain/"+name, "application/rdap+json, application/json;q=0.9")
	if err != nil {
		return nil, err
	}
	var r domainResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("invalid RDAP response: %w", err)
	}
	d := &Domain{Name: strings.ToLower(r.LdhName), Status: r.Status, Registrar: registrar(r.Entities)}
	if d.Name == "" {
		d.Name = name
	}
	for _, e := range r.Events {
		t, err := time.Parse(time.RFC3339, e.Date)
		if err != nil {
			continue
		}
		switch e.Action {
		case "registration":
			d.Registered = t
		case "expiration":
			d.Expires = t
		case "last changed":
			d.Updated = t
		}
	}
	if r.SecureDNS != nil {
		d.DNSSEC = r.SecureDNS.DelegationSigned
	}
	return d, nil
}

func (c *Client) get(ctx context.Context, url, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s returned %s", req.URL.Host, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

// registrar finds the formatted name (vCard "fn") of the registrar entity.
func registrar(entities []entity) string {
	for _, e := range entities {
		for _, role := range e.Roles {
			if role == "registrar" {
				if fn := vcardFN(e.VCard); fn != "" {
					return fn
				}
			}
		}
		if fn := registrar(e.Entities); fn != "" {
			return fn
		}
	}
	return ""
}

// vcardFN extracts "fn" from a jCard: ["vcard", [["fn", {}, "text", "Name"], ...]].
func vcardFN(v []any) string {
	if len(v) < 2 {
		return ""
	}
	props, _ := v[1].([]any)
	for _, p := range props {
		prop, _ := p.([]any)
		if len(prop) >= 4 && prop[0] == "fn" {
			if s, ok := prop[3].(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
