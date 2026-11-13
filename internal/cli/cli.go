// Package cli implements the dossier command line interface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"dossier/internal/lookup"
	"dossier/internal/provider"
	"dossier/internal/rdap"
	"dossier/internal/render"
	"dossier/internal/term"
	"dossier/internal/web"
)

// Set at build time via -ldflags "-X dossier/internal/cli.version=...".
var version = "dev"

// maxValueWidth truncates long values so rows stay on one line.
const maxValueWidth = 72

const usage = `Usage: dossier [flags] <domain>

Flags:
`

// maxListItems caps multi-value rows such as JSON-LD entities and social links.
const maxListItems = 6

// Overridable in tests. A nil resolver means the system default.
var (
	resolver  lookup.Resolver
	fetchPage = func(ctx context.Context, domain string) (*web.Page, error) {
		return web.FetchDomain(ctx, web.NewClient("dossier/"+version), domain)
	}
	lookupRDAP = func(ctx context.Context, domain string) (*rdap.Domain, error) {
		c := &rdap.Client{HTTP: web.NewClient("dossier/" + version)}
		if dir, err := os.UserCacheDir(); err == nil {
			c.CacheDir = filepath.Join(dir, "dossier")
		}
		return c.Lookup(ctx, domain)
	}
)

// Run parses args and executes the command. It returns a process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dossier", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}

	var (
		showVersion bool
		timeout     time.Duration
		colorMode   string
		logo        string
		noHTTP      bool
		noRDAP      bool
	)
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.DurationVar(&timeout, "timeout", 5*time.Second, "overall timeout for DNS, RDAP and HTTP")
	fs.StringVar(&colorMode, "color", "auto", "colorize output: auto, always or never")
	fs.BoolVar(&noHTTP, "no-http", false, "skip fetching the homepage")
	fs.BoolVar(&noRDAP, "no-rdap", false, "skip the RDAP registration lookup")
	fs.StringVar(&logo, "logo", "", "force a logo instead of detecting it: "+strings.Join(render.LogoNames(), ", ")+" or none")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if showVersion {
		fmt.Fprintf(stdout, "dossier %s\n", version)
		return 0
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	if logo != "" && logo != "none" && !slices.Contains(render.LogoNames(), logo) {
		fmt.Fprintf(stderr, "error: unknown logo %q (available: %s, none)\n", logo, strings.Join(render.LogoNames(), ", "))
		return 2
	}
	color, err := useColor(colorMode, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	domain := strings.TrimSuffix(strings.TrimSpace(fs.Arg(0)), ".")
	start := time.Now()

	// RDAP and the homepage fetch run alongside the DNS lookups.
	var (
		wg      sync.WaitGroup
		page    *web.Page
		pageErr error
		reg     *rdap.Domain
		regErr  error
	)
	if !noHTTP {
		wg.Go(func() { page, pageErr = fetchPage(ctx, domain) })
	}
	if !noRDAP {
		wg.Go(func() { reg, regErr = lookupRDAP(ctx, domain) })
	}
	res := lookup.Domain(ctx, resolver, domain)
	wg.Wait()

	card := buildCard(res, provider.Detect(res.Domain, res.NS))
	if !noRDAP {
		card.Rows = append(rdapRows(reg, regErr), card.Rows...)
	}
	if !noHTTP {
		card.Rows = append(card.Rows, webRows(page, pageErr)...)
	}
	card.Rows = append(card.Rows, render.Row{Key: "Took", Values: []string{time.Since(start).Round(time.Millisecond).String()}})
	for i := range card.Rows {
		for j, v := range card.Rows[i].Values {
			card.Rows[i].Values[j] = truncate(v, maxValueWidth)
		}
	}
	if logo != "" {
		card.Logo = logo
	}
	if err := render.Render(stdout, card, color); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if len(res.Errors) > 0 {
		for _, k := range slices.Sorted(maps.Keys(res.Errors)) {
			fmt.Fprintf(stderr, "error: %s lookup: %v\n", k, res.Errors[k])
		}
		return 1
	}
	return 0
}

func buildCard(r *lookup.Result, providers []provider.Provider) render.Card {
	card := render.Card{Title: r.Domain, Logo: "generic"}

	add := func(key string, values ...string) {
		if len(values) == 0 {
			return
		}
		card.Rows = append(card.Rows, render.Row{Key: key, Values: values})
	}

	if len(providers) > 0 {
		if providers[0].Logo != "" {
			card.Logo = providers[0].Logo
		}
		names := make([]string, len(providers))
		for i, p := range providers {
			names[i] = p.Name
		}
		add("DNS Provider", strings.Join(names, " + "))
	}
	add("Nameservers", r.NS...)
	if r.CNAME != "" {
		add("CNAME", r.CNAME)
	}
	add("IPv4", r.A...)
	add("IPv6", r.AAAA...)
	// Flag missing IPv6 on domains that otherwise resolve, unless the lookup failed.
	if _, failed := r.Errors["AAAA"]; len(r.AAAA) == 0 && !failed && len(card.Rows) > 0 {
		card.Rows = append(card.Rows, render.Row{Key: "IPv6", Values: []string{"✗ no AAAA record"}, Bad: true})
	}
	add("Mail", mailProvider(r)...)

	if len(card.Rows) == 0 {
		add("Records", "none found (does the domain exist?)")
	}
	return card
}

func rdapRows(d *rdap.Domain, err error) []render.Row {
	switch {
	case errors.Is(err, rdap.ErrNotFound):
		return []render.Row{{Key: "Registration", Values: []string{"not registered"}}}
	case errors.Is(err, rdap.ErrNoServer):
		return []render.Row{{Key: "Registration", Values: []string{"unavailable (TLD has no RDAP service)"}}}
	case err != nil:
		return []render.Row{{Key: "Registration", Values: []string{"unavailable (" + err.Error() + ")"}}}
	}

	var rows []render.Row
	add := func(key, value string) {
		if value != "" {
			rows = append(rows, render.Row{Key: key, Values: []string{value}})
		}
	}
	add("Registrar", d.Registrar)
	add("Registered", relativeDate(d.Registered))
	add("Expires", relativeDate(d.Expires))
	add("Status", summarizeStatus(d.Status))
	if d.DNSSEC != nil {
		add("DNSSEC", map[bool]string{true: "signed", false: "unsigned"}[*d.DNSSEC])
	}
	return rows
}

// relativeDate formats t as "2027-08-13 (in 273 days)" or "1995-08-14 (31 years ago)".
func relativeDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	days := int(time.Until(t).Hours() / 24)
	var rel string
	switch abs := max(days, -days); {
	case abs >= 365:
		rel = plural(abs/365, "year")
	default:
		rel = plural(abs, "day")
	}
	if days >= 0 {
		rel = "in " + rel
	} else {
		rel += " ago"
	}
	return t.Format(time.DateOnly) + " (" + rel + ")"
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// summarizeStatus folds the "* prohibited" lock statuses into one entry:
// "active · locked: delete, transfer, update".
func summarizeStatus(statuses []string) string {
	var other, locks []string
	for _, s := range statuses {
		s = strings.ToLower(s)
		if action, ok := strings.CutSuffix(s, " prohibited"); ok {
			action = strings.TrimPrefix(strings.TrimPrefix(action, "client "), "server ")
			locks = append(locks, action)
			continue
		}
		other = append(other, s)
	}
	slices.Sort(locks)
	locks = slices.Compact(locks)
	if len(locks) > 0 {
		other = append(other, "locked: "+strings.Join(locks, ", "))
	}
	return strings.Join(other, " · ")
}

func webRows(p *web.Page, err error) []render.Row {
	var rows []render.Row
	add := func(key string, values ...string) {
		values = slices.DeleteFunc(values, func(v string) bool { return v == "" })
		if len(values) > 0 {
			rows = append(rows, render.Row{Key: key, Values: values})
		}
	}
	if err != nil {
		add("HTTP", "unreachable ("+err.Error()+")")
		return rows
	}

	status := p.Status + " · " + p.Proto
	var final string
	if p.Redirects > 0 {
		final = "→ " + p.URL
	}
	add("HTTP", status, final)
	add("Server", p.Server)
	switch {
	case p.HTTPSError != "":
		add("TLS", "failed ("+p.HTTPSError+")")
	case p.TLSVersion != "":
		days := int(time.Until(p.CertExpiry).Hours() / 24)
		expiry := fmt.Sprintf("expires %s (in %s)", p.CertExpiry.Format(time.DateOnly), plural(days, "day"))
		if days < 0 {
			expiry = fmt.Sprintf("EXPIRED %s", p.CertExpiry.Format(time.DateOnly))
		}
		add("TLS", p.TLSVersion+" · "+expiry, p.CertIssuer)
	}

	title := p.Title
	if title == "" {
		title = p.SiteName
	}
	add("Title", title)
	add("Description", p.Description)
	add("Generator", p.Generator)

	var entities, social []string
	for _, e := range p.JSONLD {
		s := strings.Join(e.Types, "/")
		if e.Name != "" {
			s += " (" + e.Name + ")"
		}
		entities = append(entities, s)
		social = append(social, e.SameAs...)
	}
	add("JSON-LD", capList(dedupe(entities))...)
	add("Social", capList(dedupe(social))...)
	return rows
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	return slices.DeleteFunc(s, func(v string) bool {
		if seen[v] {
			return true
		}
		seen[v] = true
		return false
	})
}

func capList(s []string) []string {
	if len(s) <= maxListItems {
		return s
	}
	return append(s[:maxListItems:maxListItems], fmt.Sprintf("… and %d more", len(s)-maxListItems))
}

func mailProvider(r *lookup.Result) []string {
	if len(r.MX) == 0 {
		return nil
	}
	hosts := make([]string, len(r.MX))
	for i, m := range r.MX {
		hosts[i] = m.Host
	}
	providers := provider.DetectMail(r.Domain, hosts)
	if len(providers) == 0 {
		return []string{"none (domain accepts no mail)"} // only a null MX ("0 .")
	}
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name
	}
	return []string{strings.Join(names, " + ")}
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func useColor(mode string, stdout io.Writer) (bool, error) {
	f, isFile := stdout.(*os.File)
	switch mode {
	case "never":
		return false, nil
	case "always":
		if isFile {
			term.EnableVT(f)
		}
		return true, nil
	case "auto":
		if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || !isFile {
			return false, nil
		}
		return term.IsTerminal(f) && term.EnableVT(f), nil
	default:
		return false, fmt.Errorf("invalid -color %q (want auto, always or never)", mode)
	}
}
