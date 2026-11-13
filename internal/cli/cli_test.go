package cli

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"dossier/internal/rdap"
	"dossier/internal/web"
)

type fakeResolver struct{}

func (fakeResolver) LookupNS(context.Context, string) ([]*net.NS, error) {
	return []*net.NS{{Host: "b.ns.cloudflare.com."}, {Host: "a.ns.cloudflare.com."}}, nil
}
func (fakeResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return []*net.MX{{Host: "aspmx.l.google.com.", Pref: 1}, {Host: "alt1.aspmx.l.google.com.", Pref: 5}}, nil
}
func (fakeResolver) LookupCNAME(_ context.Context, host string) (string, error) {
	return host + ".", nil
}
func (fakeResolver) LookupIP(_ context.Context, network, _ string) ([]net.IP, error) {
	if network == "ip6" {
		return nil, &net.DNSError{Err: "no such host", IsNotFound: true}
	}
	return []net.IP{net.ParseIP("192.0.2.1")}, nil
}

func TestRun(t *testing.T) {
	resolver = fakeResolver{}
	origFetch := fetchPage
	fetchPage = func(_ context.Context, domain string) (*web.Page, error) {
		if domain == "down.example" {
			return nil, errors.New("connection refused")
		}
		return &web.Page{
			URL: "https://www.example.com/", Redirects: 1, Status: "200 OK", Proto: "HTTP/2",
			Server: "nginx", TLSVersion: "TLS 1.3", CertIssuer: "Let's Encrypt (R11)",
			CertExpiry: time.Now().Add(50*24*time.Hour + time.Hour),
			Meta: web.Meta{
				Title: "Example",
				JSONLD: []web.Entity{
					{Types: []string{"Organization"}, Name: "Example Inc", SameAs: []string{"https://x.com/ex"}},
					{Types: []string{"WebSite"}},
				},
			},
		}, nil
	}
	origRDAP := lookupRDAP
	lookupRDAP = func(_ context.Context, domain string) (*rdap.Domain, error) {
		if domain == "down.example" {
			return nil, rdap.ErrNotFound
		}
		signed := true
		return &rdap.Domain{
			Name: domain, Registrar: "Example Registrar, Inc.",
			Registered: time.Now().AddDate(-31, 0, -5),
			Expires:    time.Now().Add(273*24*time.Hour + time.Hour),
			Status:     []string{"client transfer prohibited", "server delete prohibited", "client delete prohibited", "active"},
			DNSSEC:     &signed,
		}, nil
	}
	t.Cleanup(func() { resolver = nil; fetchPage = origFetch; lookupRDAP = origRDAP })

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout []string
		wantStderr string
	}{
		{name: "version", args: []string{"-version"}, wantStdout: []string{"dossier dev"}},
		{name: "help", args: []string{"-h"}, wantStderr: "Usage:"},
		{name: "no args", args: nil, wantCode: 2, wantStderr: "Usage:"},
		{name: "unknown flag", args: []string{"-nope"}, wantCode: 2, wantStderr: "flag provided but not defined"},
		{name: "bad color", args: []string{"-color", "pink", "example.com"}, wantCode: 2, wantStderr: "invalid -color"},
		{name: "bad logo", args: []string{"-logo", "nope", "example.com"}, wantCode: 2, wantStderr: "unknown logo"},
		{
			name: "domain",
			args: []string{"example.com"},
			wantStdout: []string{
				"Registrar: Example Registrar, Inc.\n",
				"(31 years ago)",
				"Expires: ",
				"(in 273 days)",
				"Status: active · locked: delete, transfer\n",
				"DNSSEC: signed",
				"DNS Provider: Cloudflare",
				"Nameservers: a.ns.cloudflare.com",
				"IPv4: 192.0.2.1\n",
				"IPv6: ✗ no AAAA record\n",
				"Mail: Google (Gmail)\n",
				"HTTP: 200 OK · HTTP/2\n",
				"→ https://www.example.com/",
				"Server: nginx",
				"TLS: TLS 1.3 · expires ",
				"(in 50 days)",
				"Let's Encrypt (R11)",
				"Title: Example",
				"JSON-LD: Organization (Example Inc)\n",
				"WebSite\n",
				"Social: https://x.com/ex",
				"Took:",
				`""""`, // Cloudflare logo
			},
		},
		{name: "http down", args: []string{"down.example"}, wantStdout: []string{"HTTP: unreachable (connection refused)", "Registration: not registered"}},
		{name: "no http", args: []string{"-no-http", "-logo", "none", "example.com"}, wantStdout: []string{"Mail: Google (Gmail)\nTook:"}},
		{name: "no rdap", args: []string{"-no-rdap", "-logo", "none", "example.com"}, wantStdout: []string{"-----------\nDNS Provider:"}},
		{name: "no logo", args: []string{"-logo", "none", "example.com"}, wantStdout: []string{"example.com\n-----------\nRegistrar:"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, tt.wantCode, stderr.String())
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("stdout = %q, want substring %q", stdout.String(), want)
				}
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
			if strings.Contains(stdout.String(), "\x1b[") {
				t.Error("stdout to a non-terminal should not contain ANSI escapes")
			}
		})
	}
}
