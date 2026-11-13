package rdap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const domainJSON = `{
  "objectClassName": "domain",
  "ldhName": "EXAMPLE.COM",
  "status": ["client delete prohibited", "client transfer prohibited", "active"],
  "events": [
    {"eventAction": "registration", "eventDate": "1995-08-14T04:00:00Z"},
    {"eventAction": "expiration", "eventDate": "2027-08-13T04:00:00.123Z"},
    {"eventAction": "last changed", "eventDate": "2026-08-14T07:01:39Z"}
  ],
  "entities": [
    {"roles": ["registrant"], "vcardArray": ["vcard", [["fn", {}, "text", "Not Me"]]]},
    {"roles": ["registrar"], "vcardArray": ["vcard", [["version", {}, "text", "4.0"], ["fn", {}, "text", "RESERVED-IANA"]]]}
  ],
  "secureDNS": {"delegationSigned": true}
}`

func newServer(t *testing.T, bootstrapHits *atomic.Int32) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		bootstrapHits.Add(1)
		fmt.Fprintf(w, `{"services":[[["com","net"],["http://insecure.invalid/","%s/rdap/"]],[["co.uk"],["%s/rdap"]]]}`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/rdap/domain/example.com", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rdap+json")
		w.Write([]byte(domainJSON))
	})
	mux.HandleFunc("/rdap/domain/", http.NotFound)
	srv = httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLookup(t *testing.T) {
	var hits atomic.Int32
	srv := newServer(t, &hits)
	c := &Client{HTTP: srv.Client(), BootstrapURL: srv.URL + "/bootstrap"}

	d, err := c.Lookup(context.Background(), "www.Example.com.")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "example.com" || d.Registrar != "RESERVED-IANA" {
		t.Errorf("Domain = %+v", d)
	}
	if d.Registered.Year() != 1995 || d.Expires.Year() != 2027 || d.Updated.Year() != 2026 {
		t.Errorf("dates = %v %v %v", d.Registered, d.Expires, d.Updated)
	}
	if len(d.Status) != 3 || d.DNSSEC == nil || !*d.DNSSEC {
		t.Errorf("Status = %v, DNSSEC = %v", d.Status, d.DNSSEC)
	}
}

func TestLookupErrors(t *testing.T) {
	var hits atomic.Int32
	srv := newServer(t, &hits)
	c := &Client{HTTP: srv.Client(), BootstrapURL: srv.URL + "/bootstrap"}

	if _, err := c.Lookup(context.Background(), "nope.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unregistered: err = %v, want ErrNotFound", err)
	}
	if _, err := c.Lookup(context.Background(), "example.zz"); !errors.Is(err, ErrNoServer) {
		t.Errorf("unknown TLD: err = %v, want ErrNoServer", err)
	}
}

func TestBootstrapCache(t *testing.T) {
	var hits atomic.Int32
	srv := newServer(t, &hits)
	dir := t.TempDir()
	c := &Client{HTTP: srv.Client(), BootstrapURL: srv.URL + "/bootstrap", CacheDir: dir}

	for range 2 {
		if _, err := c.Lookup(context.Background(), "example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("bootstrap fetched %d times, want 1 (cached)", hits.Load())
	}

	// Expired cache and unreachable registry: fall back to the stale copy.
	c.CacheTTL = time.Nanosecond
	c.BootstrapURL = srv.URL + "/missing"
	if _, err := c.Lookup(context.Background(), "example.com"); err != nil {
		t.Errorf("stale cache fallback failed: %v", err)
	}
}
