# Contributing to dossier

Thanks for helping out! Bug fixes, new providers and better logos are all
welcome. This page explains the requirements for adding a provider and how
to do it.

## General

- Run `gofmt -w .` and `make test` before opening a pull request.
- Keep the binary dependency-free and cross-platform (Windows, macOS, Linux).
- For larger changes, please open an issue first so we can agree on the
  approach.

## Requirements for new providers

dossier shows a logo for DNS providers and a display name for mail providers.
To keep the list useful and maintainable, a new provider must be
**established**, **public** and **verifiable**.

### 1. Established: meets at least one usage measurement

Show at least one of these in the pull request, with a link to the source:

| Measurement | Threshold |
|---|---|
| Listed in the [W3Techs](https://w3techs.com/technologies) usage statistics for DNS server providers or email server providers | Listed at all |
| Domains in the [Tranco](https://tranco-list.eu/) top 1 million that use the provider's nameservers (for a logo) or MX hosts (for a mail provider) | At least 1,000 |
| Total domains using the provider, according to a public dataset such as [DNSlytics](https://dnslytics.com/) or [BuiltWith](https://builtwith.com/) | At least 100,000 |

The provider must also have been operating for at least **2 years**.

### 2. Public: anyone can sign up

The service must be available to the general public, either as a
stand-alone product or as part of a hosting or registrar offering. A
company's private nameservers or mail servers don't qualify. dossier
already labels those as `Self-hosted`.

### 3. Verifiable: real hostnames

Include at least one real nameserver or MX hostname the provider gives its
customers, plus a public domain that uses it, so a reviewer can run
`dossier <domain>` to check.

Providers already in dossier were added before these requirements existed and
are kept as they are.

## Adding a provider logo

A logo is chosen **only from the domain's nameservers**. Logos based on IP
addresses, hosting or HTTP headers aren't accepted, because the logo should
always mean one thing: who runs the domain's DNS.

1. **Add a nameserver rule** to `dnsRules` in `internal/provider/provider.go`,
   and set its `Logo` field to the name you'll use for the logo:

   ```go
   {provider: Provider{"Example DNS", "exampledns"}, suffixes: []string{"exampledns.net"}},
   ```

   Suffixes match whole hostname labels, so `exampledns.net` matches
   `ns1.exampledns.net` but not `notexampledns.net`. Only list domains that
   are used by that provider and nothing else.

2. **Add the artwork** to `Logos` in `internal/render/logos.go`. `$1`, `$2`, …
   switch between the colors listed in `Colors`, which are
   [SGR](https://en.wikipedia.org/wiki/ANSI_escape_code#SGR) codes such as
   `38;5;208` (256-color orange).

3. **Add a test case** to `TestLookup` in
   `internal/provider/provider_test.go` using a real nameserver hostname.

4. **Preview it** with `go run ./cmd/dossier -logo exampledns example.com`.

### Logo guidelines

- **Size:** at most 20 lines tall and 50 characters wide.
- **Characters:** plain ASCII only, so it renders in every terminal,
  including older Windows consoles.
- **Colors:** use up to 4 colors, close to the provider's brand colors.
- **Recognizable:** it should be a fair likeness of the official logo or
  wordmark. To generate a starting point from an image, any
  image-to-ASCII tool works. Clean up the result by hand.
- **Trademarks:** logos are used only to identify the provider. If a
  provider asks for its logo to be removed, it will be removed.

## Adding a mail provider

1. **Add a rule** to `mailRules` in `internal/provider/provider.go` with the
   display name and the domain its MX hosts end in:

   ```go
   {provider: Provider{Name: "Fastmail"}, suffixes: []string{"messagingengine.com"}},
   ```

2. **Add a test case** to `TestDetectMail` in
   `internal/provider/provider_test.go` using a real MX hostname.

### Display name guidelines

- Use the provider's official name, spelled the way the provider writes it
  (`Mailbox.org`, `Fastmail`).
- When the company and the mail product have different names, use
  `Company (Product)`, e.g. `Google (Gmail)` or `Microsoft (Outlook)`.
- Keep it short: at most 30 characters.

## Pull request checklist

- [ ] The provider meets the [requirements](#requirements-for-new-providers),
      with a link to the usage source
- [ ] A real hostname and a public example domain are in the description
- [ ] A test case is included, and `make test` passes
- [ ] `gofmt -l .` prints nothing
- [ ] For logos: the size and ASCII-only guidelines are met, and a screenshot or
      pasted output is in the description
