// Package provider identifies DNS hosting and mail providers from
// nameserver and MX hostnames.
package provider

import (
	"slices"
	"strings"
)

// Provider is a DNS or mail provider. Logo names an ASCII logo in package
// render; it is empty when no dedicated logo exists.
type Provider struct {
	Name string
	Logo string
}

type rule struct {
	provider Provider
	suffixes []string // matched against the end of the hostname, on a label boundary
	contains []string // matched anywhere in the hostname
}

var dnsRules = []rule{
	{provider: Provider{"Cloudflare", "cloudflare"}, suffixes: []string{"cloudflare.com"}},
	{provider: Provider{"Amazon Route 53", "aws"}, contains: []string{".awsdns-"}},
	{provider: Provider{"Google Cloud DNS", "google"}, suffixes: []string{"googledomains.com"}},
	{provider: Provider{"Google", "google"}, suffixes: []string{"google.com"}},
	{provider: Provider{"Azure DNS", "azure"}, contains: []string{".azure-dns."}},
	{provider: Provider{"bunny.net", "bunny"}, suffixes: []string{"bunny.net"}},
	{provider: Provider{"Vercel", "vercel"}, suffixes: []string{"vercel-dns.com"}},
	{provider: Provider{"DigitalOcean", "digitalocean"}, suffixes: []string{"digitalocean.com"}},
	{provider: Provider{"Hetzner", "hetzner"}, suffixes: []string{"hetzner.com", "hetzner.de", "your-server.de", "second-ns.com", "second-ns.de"}},
	{provider: Provider{"GoDaddy", "godaddy"}, suffixes: []string{"godaddy.com", "domaincontrol.com"}},
	{provider: Provider{"Namecheap", "namecheap"}, suffixes: []string{"registrar-servers.com"}},
	{provider: Provider{"NS1", ""}, suffixes: []string{"nsone.net"}},
	{provider: Provider{"Akamai", "akamai"}, suffixes: []string{"akam.net", "linode.com"}},
	{provider: Provider{"UltraDNS", ""}, contains: []string{".ultradns."}},
	{provider: Provider{"Oracle Dyn", ""}, suffixes: []string{"dynect.net"}},
	{provider: Provider{"DNSimple", ""}, suffixes: []string{"dnsimple.com"}},
	{provider: Provider{"OVHcloud", ""}, suffixes: []string{"ovh.net"}},
	{provider: Provider{"Gandi", ""}, suffixes: []string{"gandi.net"}},
	{provider: Provider{"Porkbun", ""}, suffixes: []string{"porkbun.com"}},
	{provider: Provider{"Wix", "wix"}, suffixes: []string{"wixdns.net"}},
	{provider: Provider{"one.com", "onecom"}, suffixes: []string{"one.com"}},
	{provider: Provider{"Weebly", "weebly"}, suffixes: []string{"weebly.com"}},
	{provider: Provider{"IONOS", ""}, suffixes: []string{"ui-dns.com", "ui-dns.de", "ui-dns.org", "ui-dns.biz"}},
	{provider: Provider{"Hostinger", ""}, suffixes: []string{"dns-parking.com"}},
}

var mailRules = []rule{
	{provider: Provider{Name: "Google (Gmail)"}, suffixes: []string{"google.com", "googlemail.com"}},
	{provider: Provider{Name: "Microsoft (Outlook)"}, suffixes: []string{"outlook.com", "hotmail.com"}},
	{provider: Provider{Name: "Proton"}, suffixes: []string{"protonmail.ch", "proton.me"}},
	{provider: Provider{Name: "Tutanota"}, suffixes: []string{"tutanota.de", "tutanota.com", "tuta.com", "tuta.io"}},
	{provider: Provider{Name: "Migadu"}, suffixes: []string{"migadu.com"}},
	{provider: Provider{Name: "Mailbox.org"}, suffixes: []string{"mailbox.org"}},
	{provider: Provider{Name: "Fastmail"}, suffixes: []string{"messagingengine.com", "fastmail.com"}},
	{provider: Provider{Name: "Apple (iCloud Mail)"}, suffixes: []string{"icloud.com"}},
	{provider: Provider{Name: "Zoho Mail"}, suffixes: []string{"zoho.com", "zoho.eu", "zoho.in", "zoho.com.au", "zohomail.com"}},
	{provider: Provider{Name: "Yahoo"}, suffixes: []string{"yahoodns.net"}},
	{provider: Provider{Name: "Yandex"}, suffixes: []string{"yandex.net", "yandex.ru"}},
	{provider: Provider{Name: "Posteo"}, suffixes: []string{"posteo.de"}},
	{provider: Provider{Name: "Runbox"}, suffixes: []string{"runbox.com"}},
	{provider: Provider{Name: "Infomaniak"}, suffixes: []string{"infomaniak.ch"}},
	{provider: Provider{Name: "Cloudflare Email Routing"}, suffixes: []string{"mx.cloudflare.net"}},
	{provider: Provider{Name: "ImprovMX"}, suffixes: []string{"improvmx.com"}},
	{provider: Provider{Name: "Forward Email"}, suffixes: []string{"forwardemail.net"}},
	{provider: Provider{Name: "Amazon SES"}, suffixes: []string{"amazonaws.com", "amazonses.com"}},
	{provider: Provider{Name: "Mailgun"}, suffixes: []string{"mailgun.org"}},
	{provider: Provider{Name: "SendGrid"}, suffixes: []string{"sendgrid.net"}},
	{provider: Provider{Name: "Mimecast"}, suffixes: []string{"mimecast.com", "mimecast.co.za"}},
	{provider: Provider{Name: "Proofpoint"}, suffixes: []string{"pphosted.com", "ppe-hosted.com"}},
	{provider: Provider{Name: "Barracuda"}, suffixes: []string{"barracudanetworks.com"}},
	{provider: Provider{Name: "GoDaddy"}, suffixes: []string{"secureserver.net"}},
	{provider: Provider{Name: "Namecheap"}, suffixes: []string{"registrar-servers.com", "privateemail.com"}},
	{provider: Provider{Name: "OVHcloud"}, suffixes: []string{"ovh.net"}},
	{provider: Provider{Name: "Gandi"}, suffixes: []string{"gandi.net"}},
	{provider: Provider{Name: "IONOS"}, suffixes: []string{"ionos.com", "ionos.de", "ionos.co.uk", "1and1.com"}},
	{provider: Provider{Name: "Hostinger"}, suffixes: []string{"hostinger.com"}},
}

// Lookup returns the DNS provider for a single nameserver hostname. Unknown
// nameservers yield a provider named after the hostname's last two labels,
// prefixed with "Self-hosted / " when those match the queried domain's.
func Lookup(domain, host string) Provider {
	if p, ok := match(dnsRules, host); ok {
		return p
	}
	sld := lastLabels(normalize(host), 2)
	if d := normalize(domain); d != "" && sld == lastLabels(d, 2) {
		return Provider{Name: "Self-hosted / " + sld}
	}
	return Provider{Name: sld}
}

// LookupMail returns the mail provider for a single MX hostname. Unknown MX
// hosts inside domain are reported as self-hosted.
func LookupMail(domain, host string) Provider {
	if p, ok := match(mailRules, host); ok {
		return p
	}
	h, d := normalize(host), normalize(domain)
	if d != "" && (h == d || strings.HasSuffix(h, "."+d)) {
		return Provider{Name: "Self-hosted"}
	}
	return Provider{Name: lastLabels(h, 2)}
}

// Detect returns the distinct DNS providers behind a set of nameservers, most
// common first. Ties prefer providers with a logo, then first appearance.
func Detect(domain string, nameservers []string) []Provider {
	return detect(nameservers, func(h string) Provider { return Lookup(domain, h) })
}

// DetectMail returns the distinct mail providers behind a domain's MX hosts.
func DetectMail(domain string, mxHosts []string) []Provider {
	return detect(mxHosts, func(h string) Provider { return LookupMail(domain, h) })
}

func normalize(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func match(rules []rule, host string) (Provider, bool) {
	host = normalize(host)
	for _, r := range rules {
		for _, s := range r.suffixes {
			if host == s || strings.HasSuffix(host, "."+s) {
				return r.provider, true
			}
		}
		for _, c := range r.contains {
			if strings.Contains(host, c) {
				return r.provider, true
			}
		}
	}
	return Provider{}, false
}

func detect(hosts []string, lookup func(string) Provider) []Provider {
	var (
		order  []Provider
		counts = map[Provider]int{}
	)
	for _, h := range hosts {
		if normalize(h) == "" {
			continue
		}
		p := lookup(h)
		if p.Name == "" {
			continue
		}
		if counts[p] == 0 {
			order = append(order, p)
		}
		counts[p]++
	}
	slices.SortStableFunc(order, func(a, b Provider) int {
		if d := counts[b] - counts[a]; d != 0 {
			return d
		}
		return hasLogo(b) - hasLogo(a)
	})
	return order
}

func lastLabels(host string, n int) string {
	labels := strings.Split(host, ".")
	if len(labels) <= n {
		return host
	}
	return strings.Join(labels[len(labels)-n:], ".")
}

func hasLogo(p Provider) int {
	if p.Logo != "" {
		return 1
	}
	return 0
}
