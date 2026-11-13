package lookup

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"
)

type fakeResolver struct{}

func (fakeResolver) LookupNS(context.Context, string) ([]*net.NS, error) {
	return []*net.NS{{Host: "b.ns.test."}, {Host: "a.ns.test."}}, nil
}
func (fakeResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return nil, &net.DNSError{Err: "no such host", IsNotFound: true}
}
func (fakeResolver) LookupCNAME(context.Context, string) (string, error) {
	return "target.test.", nil
}
func (fakeResolver) LookupIP(_ context.Context, network, _ string) ([]net.IP, error) {
	if network == "ip6" {
		return nil, errors.New("boom")
	}
	return []net.IP{net.ParseIP("192.0.2.2"), net.ParseIP("192.0.2.1")}, nil
}

func TestDomain(t *testing.T) {
	res := Domain(context.Background(), fakeResolver{}, " example.test. ")

	if res.Domain != "example.test" {
		t.Errorf("Domain = %q", res.Domain)
	}
	if res.CNAME != "target.test" {
		t.Errorf("CNAME = %q", res.CNAME)
	}
	if want := []string{"a.ns.test", "b.ns.test"}; !slices.Equal(res.NS, want) {
		t.Errorf("NS = %v, want %v", res.NS, want)
	}
	if want := []string{"192.0.2.1", "192.0.2.2"}; !slices.Equal(res.A, want) {
		t.Errorf("A = %v, want %v", res.A, want)
	}
	if len(res.MX) != 0 {
		t.Errorf("MX = %v, want empty", res.MX)
	}
	if _, ok := res.Errors["MX"]; ok {
		t.Error("not-found MX should not be reported as an error")
	}
	if _, ok := res.Errors["AAAA"]; !ok {
		t.Error("expected AAAA error")
	}
}
