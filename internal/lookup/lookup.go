// Package lookup queries DNS records for a domain.
package lookup

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
)

// Resolver is the subset of *net.Resolver used here, so tests can fake it.
type Resolver interface {
	LookupNS(ctx context.Context, name string) ([]*net.NS, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

type MX struct {
	Host string
	Pref uint16
}

// Result holds the records found for a domain. A record type that does not
// exist is left empty; other failures are collected in Errors.
type Result struct {
	Domain string
	CNAME  string // canonical name, empty if the domain is not an alias
	A      []string
	AAAA   []string
	NS     []string
	MX     []MX
	Errors map[string]error
}

// Domain runs all lookups concurrently and returns once they all finish or ctx is done.
func Domain(ctx context.Context, r Resolver, domain string) *Result {
	if r == nil {
		r = net.DefaultResolver
	}
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	res := &Result{Domain: domain, Errors: map[string]error{}}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	fail := func(kind string, err error) {
		if err == nil || isNotFound(err) {
			return
		}
		mu.Lock()
		res.Errors[kind] = err
		mu.Unlock()
	}

	// Each goroutine writes to its own field, so only Errors needs the mutex.
	wg.Go(func() {
		cname, err := r.LookupCNAME(ctx, domain)
		fail("CNAME", err)
		if c := strings.TrimSuffix(cname, "."); c != "" && !strings.EqualFold(c, domain) {
			res.CNAME = c
		}
	})
	wg.Go(func() {
		ips, err := r.LookupIP(ctx, "ip4", domain)
		fail("A", err)
		res.A = ipStrings(ips)
	})
	wg.Go(func() {
		ips, err := r.LookupIP(ctx, "ip6", domain)
		fail("AAAA", err)
		res.AAAA = ipStrings(ips)
	})
	wg.Go(func() {
		ns, err := r.LookupNS(ctx, domain)
		fail("NS", err)
		for _, n := range ns {
			res.NS = append(res.NS, strings.TrimSuffix(n.Host, "."))
		}
		slices.Sort(res.NS)
	})
	wg.Go(func() {
		mx, err := r.LookupMX(ctx, domain)
		fail("MX", err)
		for _, m := range mx {
			res.MX = append(res.MX, MX{Host: strings.TrimSuffix(m.Host, "."), Pref: m.Pref})
		}
	})

	wg.Wait()
	return res
}

func ipStrings(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	slices.Sort(out)
	return out
}

func isNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}
