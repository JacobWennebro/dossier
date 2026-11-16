> [!NOTE]
> **AI disclosure:** This project was built with the help of AI, using
> Claude Code.

<h1 align="center">
  <img src="assets/logo.svg" width="236" height="72" alt="dossier">
</h1>

<p align="center">A one-screen profile of any domain, in the style of fastfetch.</p>


**dossier** shows everything worth knowing about a domain on one screen:
who registered it, who runs its DNS, where it points, who handles its mail,
and what its website says about itself.

It's a single, dependency-free binary for Windows, macOS and Linux.

```
                    .#:                bunny.net
          +%*=:     *@@*               ---------
          :%@@@%*=. =%@@%-             Registrar: Hosting Concepts B.V. d/b/a Registrar.eu
           :#@@@@@@%*==+#@*.           Registered: 1999-11-22 (26 years ago)
             :+#%@@@@@@%*==+-          Expires: 2029-11-22 (in 3 years)
                  .::-==+#@#+-.        Status: locked: transfer
  :+*-                    %@@@@#+-     DNSSEC: unsigned
  *@@@.  .=*#%@@%#*=:   -#@@@@@@@@%*   DNS Provider: bunny.net
       :#@@@@@@@@@@@@@%@@@@@@@@%#+-    Nameservers: coco.bunny.net
      =%@@@@@@%#**#%@@@@@@@@#:.                     kiki.bunny.net
     .%@@@@@*:      -%@@@@@#.          IPv4: 185.24.11.22
     -@@@@@*   -++.  .%@@@#            IPv6: 2400:52e0:1e08::731:1
      #@@@@=   %@@+   #@@#             Mail: Google (Gmail)
       +%@@%.   :.   =@@#              HTTP: 200 OK · HTTP/2
        .*@@%+.    :#@@*               Server: BunnyCDN-AT1-731
          .*%@%*: :==-.                TLS: TLS 1.3 · expires 2027-01-27 (in 74 days)
            .+%@@+                          Let's Encrypt (YR2)
               =%+                     Title: bunny.net - The Global Edge Platform that truly Hops
                                       Description: Hop on bunny.net and speed up your web presence…
                                       Generator: Gatsby 4.25.9
                                       JSON-LD: Product (bunny.net)
                                       Took: 141ms
```

## Inspiration

The look is inspired by [fastfetch](https://github.com/fastfetch-cli/fastfetch):
a colored ASCII logo on the left, and the details as `Key: value` lines on the
right. fastfetch shows your machine's OS logo. dossier shows the logo of the
domain's DNS provider, chosen from its nameservers.

## What it shows

| Section | Details | Source |
|---|---|---|
| Registration | Registrar, registration and expiry dates, status and locks, DNSSEC | [RDAP](https://about.rdap.org/), the modern replacement for WHOIS |
| DNS | DNS provider (from the nameservers), nameservers, CNAME, IPv4, IPv6 (a red ✗ if missing) | Your system's DNS resolver |
| Mail | Mail provider, e.g. Google (Gmail), Microsoft (Outlook), Proton, Fastmail, or Self-hosted | The MX records |
| Website | HTTP status and protocol, redirects, server, TLS version, certificate issuer and expiry, title, description, generator, [JSON-LD](https://json-ld.org/) entities and social links | One request to the homepage |

DNS, RDAP and the homepage request all run in parallel, so a typical lookup
takes 100–500 ms.

Logos exist for Akamai, Amazon Route 53, Azure DNS, bunny.net, Cloudflare,
DigitalOcean, GoDaddy, Google, Hetzner, Namecheap, one.com, Vercel, Weebly and Wix. Other
providers are still recognized by name and get a generic globe logo.

**Missing a logo or a mail provider name?** Contributions are very welcome.
If your DNS provider gets the generic globe, or your mail provider shows up
as a bare hostname like `mx.example.net` instead of a name like
`Google (Gmail)`, it only takes a few lines to fix. See
[CONTRIBUTING.md](CONTRIBUTING.md) for how to add one and which providers
qualify. Pull requests that improve the existing logos are welcome too.

## Install

**Download a binary** from the [Releases](../../releases) page. Each release
includes builds for Linux, macOS and Windows on x86-64 and ARM64. Unpack it
and put `dossier` (or `dossier.exe`) somewhere on your `PATH`.

> On macOS, the first run of a downloaded binary may be blocked by
> Gatekeeper because the builds aren't signed. Clear it with
> `xattr -d com.apple.quarantine dossier`.

**Or build from source** with Go 1.26 or newer:

```sh
git clone https://github.com/JacobWennebro/dossier.git && cd dossier
go install ./cmd/dossier      # installs to $(go env GOPATH)/bin
```

## Examples

Look up a domain:

```sh
dossier example.com
```

Subdomains work too. The registration data comes from the registered domain:

```sh
dossier www.bbc.co.uk
```

DNS and registration only, without contacting the website:

```sh
dossier -no-http github.com
```

```
   __ ___      _____     github.com
  / _' \ \ /\ / / __|    ----------
 | (_| |\ V  V /\__ \    Registrar: MarkMonitor Inc.
  \__,_| \_/\_/ |___/    Registered: 2007-10-09 (19 years ago)
 '-.,_          _,.-'/   Expires: 2028-10-09 (in 1 year)
      '-......-'  '-'    Status: locked: delete, transfer, update
                         DNSSEC: unsigned
                         DNS Provider: Amazon Route 53 + NS1
                         Nameservers: dns1.p08.nsone.net
                                      …
                         IPv4: 4.163.8.144
                         IPv6: ✗ no AAAA record
                         Mail: Microsoft (Outlook)
                         Took: 113ms
```

Plain output without a logo or colors, handy for scripts and `grep`:

```sh
dossier -logo none -color never example.com | grep Expires
```

Preview another provider's logo, or allow more time on a slow connection:

```sh
dossier -logo cloudflare example.com
dossier -timeout 10s example.com
```

## Usage

```
dossier [flags] <domain>

  -color     auto (default), always or never; NO_COLOR is respected
  -logo      force a logo: akamai, aws, azure, bunny, cloudflare,
             digitalocean, generic, godaddy, google, hetzner, namecheap, onecom,
             vercel, weebly, wix or none
  -no-http   skip the homepage request
  -no-rdap   skip the RDAP registration lookup
  -timeout   overall timeout for DNS, RDAP and HTTP (default 5s)
  -version   print version and exit
```

The exit code is 0 on success, 1 if a DNS lookup failed, and 2 for invalid
usage. If the website or RDAP can't be reached, dossier shows that in the
output but still exits 0.

## Notes

- **Privacy:** dossier contacts your DNS resolver, the domain's RDAP server
  (plus IANA's RDAP server list about once a week), and the domain's own
  website. Requests identify themselves as `dossier/<version>`.
- **RDAP coverage:** some country-code domains don't offer RDAP yet. For
  those, the registration section shows "unavailable".
- **Cache:** the RDAP server list is cached in `~/Library/Caches/dossier`
  (macOS), `~/.cache/dossier` (Linux) or `%LocalAppData%\dossier` (Windows).

## Development

```sh
make build    # ./bin/dossier
make test     # go test -race ./...
make run ARGS="example.com"
```

| Package | Purpose |
|---|---|
| `cmd/dossier` | Entry point: signal handling and exit code |
| `internal/cli` | Flags, running the lookups in parallel, building the output rows |
| `internal/lookup` | DNS lookups (NS, MX, CNAME, A, AAAA) |
| `internal/rdap` | RDAP lookup and the cached IANA server list |
| `internal/web` | Homepage request and HTML/JSON-LD parsing |
| `internal/provider` | DNS and mail provider detection from hostnames |
| `internal/render` | Logo and info-column layout, and the ASCII logos |
| `internal/term` | Terminal detection and Windows color support |

### Adding providers

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to add a DNS provider logo or
a mail provider name, and the requirements a provider must meet.

### Releases

Every push to `main` runs the tests on Linux, macOS and Windows, then
publishes a GitHub release. The version is the latest `vX.Y.Z` tag with
the patch number increased, starting at `v0.1.0`. For a minor or major
version bump, push a tag such as `v0.2.0` yourself, and later releases
continue from it.

Build the release archives locally with:

```sh
scripts/release-build.sh v0.0.0-dev
```

## License

dossier is free software, released under the
[GNU General Public License v3.0](LICENSE). You may use, study, share and
change it. If you distribute it or a modified version, you must do so under
the same license and make the source code available.
