package main

import "testing"

func TestANSIToHTML(t *testing.T) {
	tests := map[string]string{
		"plain <b>&":                            "plain &lt;b&gt;&amp;",
		"\x1b[1;38;5;33mkey\x1b[0m: v":          `<span style="color:#0087ff;font-weight:bold;">key</span>: v`,
		"\x1b[31m✗ none\x1b[0m":                 `<span style="color:#ff7b72;">✗ none</span>`,
		"\x1b[0;38;5;196ma\x1b[0;38;5;196mb":    `<span style="color:#ff0000;">ab</span>`,
		"\x1b[38;5;232mx\x1b[38;5;16my\x1b[39m": `<span style="color:#080808;">x</span><span style="color:#000000;">y</span>`,
		"\x1b[2Kcleared":                        "cleared",
		"\x1b[1mbold":                           `<span style="font-weight:bold;">bold</span>`,
	}
	for in, want := range tests {
		if got := ansiToHTML(in); got != want {
			t.Errorf("ansiToHTML(%q)\n got %s\nwant %s", in, got, want)
		}
	}
}

func TestNormalizeDomain(t *testing.T) {
	for in, want := range map[string]string{
		"Google.com.":           "google.com",
		"xn--bcher-kva.example": "xn--bcher-kva.example",
		"localhost":             "",
		"127.0.0.1":             "",
		"a..com":                "",
		"-a.com":                "",
	} {
		got, ok := normalizeDomain(in)
		if got != want || ok != (want != "") {
			t.Errorf("normalizeDomain(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestDetectOS(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36":                "Windows",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15":      "Mac",
		"Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0":                                 "Linux",
		"Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 Chrome/130.0 Safari/537.36":                 "Linux",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/130.0 Mobile Safari/537.36":          "",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148": "",
		"curl/8.7.1": "",
	} {
		if got := detectOS(ua); got != want {
			t.Errorf("detectOS(%q) = %q, want %q", ua, got, want)
		}
	}
}
