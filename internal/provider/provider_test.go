package provider

import "testing"

func TestLookup(t *testing.T) {
	tests := map[string]string{
		"elliott.ns.cloudflare.com.":    "Cloudflare",
		"ns-1234.awsdns-12.org":         "Amazon Route 53",
		"ns-cloud-a1.googledomains.com": "Google Cloud DNS",
		"ns1.google.com":                "Google",
		"ns1-01.azure-dns.com":          "Azure DNS",
		"a1-66.akam.net":                "Akamai",
		"ns0.wixdns.net":                "Wix",
		"ns01.one.com":                  "one.com",
		"kiki.bunny.net":                "bunny.net",
		"ns1.vercel-dns.com":            "Vercel",
		"ns1.digitalocean.com":          "DigitalOcean",
		"helium.ns.hetzner.de":          "Hetzner",
		"ns01.domaincontrol.com":        "GoDaddy",
		"ns1.your-server.de":            "Hetzner",
		"pdns1.ultradns.net":            "UltraDNS",
		"ns1.example.net":               "example.net",
		"NotCloudflare.com":             "notcloudflare.com",
	}
	for host, want := range tests {
		if got := Lookup("example.org", host).Name; got != want {
			t.Errorf("Lookup(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestDetect(t *testing.T) {
	got := Detect("example.org", []string{"ns1.example.net", "a.ns.cloudflare.com", "b.ns.cloudflare.com"})
	if len(got) != 2 || got[0].Name != "Cloudflare" || got[1].Name != "example.net" {
		t.Errorf("Detect = %+v", got)
	}
	got = Detect("example.org", []string{"dns1.p08.nsone.net", "ns-1.awsdns-01.org"})
	if got[0].Name != "Amazon Route 53" {
		t.Errorf("tie should prefer provider with a logo, got %+v", got)
	}
	for _, domain := range []string{"example.net", "www.example.net"} {
		got = Detect(domain, []string{"ns1.example.net", "ns2.example.net."})
		if len(got) != 1 || got[0].Name != "Self-hosted / example.net" {
			t.Errorf("Detect(%q) = %+v, want Self-hosted / example.net", domain, got)
		}
	}
	// Known providers keep their name even on their own domain.
	if got := Detect("google.com", []string{"ns1.google.com"}); got[0].Name != "Google" {
		t.Errorf("Detect(google.com) = %+v", got)
	}
	if got := Detect("example.org", nil); len(got) != 0 {
		t.Errorf("Detect(nil) = %+v, want empty", got)
	}
}

func TestDetectMail(t *testing.T) {
	tests := []struct {
		domain string
		mx     []string
		want   string
	}{
		{"example.com", []string{"aspmx.l.google.com.", "alt1.aspmx.l.google.com."}, "Google (Gmail)"},
		{"github.com", []string{"github-com.mail.protection.outlook.com."}, "Microsoft (Outlook)"},
		{"proton.me", []string{"mail.protonmail.ch", "mailsec.protonmail.ch"}, "Proton"},
		{"example.com", []string{"mail.tutanota.de"}, "Tutanota"},
		{"example.com", []string{"aspmx1.migadu.com"}, "Migadu"},
		{"example.com", []string{"mxext1.mailbox.org"}, "Mailbox.org"},
		{"example.com", []string{"in1-smtp.messagingengine.com"}, "Fastmail"},
		{"example.com", []string{"mail.example.com"}, "Self-hosted"},
		{"google.com", []string{"smtp.google.com"}, "Google (Gmail)"},
		{"example.com", []string{"mx.example.net"}, "example.net"},
	}
	for _, tt := range tests {
		got := DetectMail(tt.domain, tt.mx)
		if len(got) != 1 || got[0].Name != tt.want {
			t.Errorf("DetectMail(%q, %v) = %+v, want %q", tt.domain, tt.mx, got, tt.want)
		}
	}
	if got := DetectMail("example.com", []string{""}); len(got) != 0 {
		t.Errorf("null MX should yield no provider, got %+v", got)
	}
}
