package netacl

import (
	"net/http/httptest"
	"net/netip"
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	_, text, err := Parse([]string{" 10.0.0.15 ", "10.20.30.40/16", "", "10.0.0.15/32", "2001:db8::1/48", "::ffff:192.168.1.7", "::ffff:192.168.0.0/112"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.15", "10.20.0.0/16", "2001:db8::/48", "192.168.1.7", "192.168.0.0/16"}
	if !slices.Equal(text, want) {
		t.Errorf("text = %v, want %v", text, want)
	}
	for _, bad := range []string{"10.0.0", "10.0.0.0/33", "example.com", "fe80::1%eth0", "10.0.0.1/x", "::ffff:0:0/90"} {
		if _, _, err := Parse([]string{bad}); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestContains(t *testing.T) {
	l, _, err := Parse([]string{"10.20.0.0/16", "192.168.1.7", "2001:db8::/32"})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"10.20.255.1":        true,
		"10.21.0.1":          false,
		"192.168.1.7":        true,
		"192.168.1.8":        false,
		"::ffff:192.168.1.7": true,
		"2001:db8:5::1":      true,
		"2001:db9::1":        false,
	}
	for ip, want := range cases {
		if got := l.Contains(netip.MustParseAddr(ip)); got != want {
			t.Errorf("%s: %v, want %v", ip, got, want)
		}
	}
	if (List{}).Contains(netip.MustParseAddr("10.0.0.1")) {
		t.Error("an empty list must hold nothing")
	}
	if l.Contains(netip.Addr{}) {
		t.Error("an invalid address must not match")
	}
}

func TestClientIP(t *testing.T) {
	proxies := MustParse([]string{"10.0.0.0/24", "10.0.1.5"})
	cases := []struct {
		name, remote string
		xff          []string
		trusted      List
		want         string
	}{
		{"no proxies: peer as is", "203.0.113.9:5555", []string{"10.20.0.5"}, nil, "203.0.113.9"},
		{"untrusted peer: header ignored", "203.0.113.9:5555", []string{"10.20.0.5"}, proxies, "203.0.113.9"},
		{"trusted proxy", "10.0.0.2:443", []string{"10.20.0.5"}, proxies, "10.20.0.5"},
		{"forged left entries ignored", "10.0.0.2:443", []string{"1.2.3.4, 10.20.0.5"}, proxies, "10.20.0.5"},
		{"chain of trusted proxies", "10.0.0.2:443", []string{"10.20.0.5, 10.0.1.5", "10.0.0.7"}, proxies, "10.20.0.5"},
		{"trusted proxy without header", "10.0.0.2:443", nil, proxies, "10.0.0.2"},
		{"malformed hop stops at the last trusted one", "10.0.0.2:443", []string{"10.20.0.5, garbage"}, proxies, "10.0.0.2"},
		{"all hops trusted", "10.0.0.2:443", []string{"10.0.0.9"}, proxies, "10.0.0.9"},
		{"ipv6 peer", "[2001:db8::7]:443", nil, proxies, "2001:db8::7"},
		{"mapped hop", "10.0.0.2:443", []string{"::ffff:10.20.0.5"}, proxies, "10.20.0.5"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/api/tv/x", nil)
		r.RemoteAddr = c.remote
		for _, v := range c.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		if got := ClientIP(r, c.trusted); got.String() != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}
