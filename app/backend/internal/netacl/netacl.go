// Package netacl checks client addresses against lists of networks and finds the client
// address of a request behind trusted reverse proxies.
package netacl

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"
)

// List is a set of networks; a single address is a /32 or /128 network.
type List []netip.Prefix

// Parse reads addresses and networks such as 10.0.0.15, 10.20.0.0/16 or 2001:db8::/48 and
// returns the list and the entries in canonical form (host bits cleared, single addresses
// without a prefix length). Empty entries are skipped; duplicates are dropped.
func Parse(entries []string) (List, []string, error) {
	out, text := List{}, []string{}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		p, err := parseOne(e)
		if err != nil {
			return nil, nil, err
		}
		s := format(p)
		if contains(text, s) {
			continue
		}
		out, text = append(out, p), append(text, s)
	}
	return out, text, nil
}

// MustParse is Parse for lists already checked when they were saved; bad entries are skipped.
func MustParse(entries []string) List {
	out := List{}
	for _, e := range entries {
		if p, err := parseOne(strings.TrimSpace(e)); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func parseOne(e string) (netip.Prefix, error) {
	if strings.Contains(e, "/") {
		p, err := netip.ParsePrefix(e)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("%q is not a network such as 10.20.0.0/16", e)
		}
		if p.Addr().Is4In6() {
			bits := p.Bits() - 96
			if bits < 0 {
				return netip.Prefix{}, fmt.Errorf("%q is not a network such as 10.20.0.0/16", e)
			}
			p = netip.PrefixFrom(p.Addr().Unmap(), bits)
		}
		return p.Masked(), nil
	}
	a, err := netip.ParseAddr(e)
	if err != nil || a.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("%q is not an address such as 10.0.0.15", e)
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

func format(p netip.Prefix) string {
	if p.IsSingleIP() {
		return p.Addr().String()
	}
	return p.String()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Contains reports whether the address is in one of the networks. An empty list holds nothing.
func (l List) Contains(a netip.Addr) bool {
	if !a.IsValid() {
		return false
	}
	a = a.Unmap()
	for _, p := range l {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ClientIP is the address of whoever sent the request. The peer address is taken as it is
// unless it is a trusted proxy; then X-Forwarded-For is read from the right, skipping trusted
// proxies, and the first address that is not one is the client. A malformed entry stops the
// walk at the last hop that was still trusted, so a forged header cannot pick an address.
func ClientIP(r *http.Request, trusted List) netip.Addr {
	peer := RemoteAddr(r.RemoteAddr)
	if !trusted.Contains(peer) {
		return peer
	}
	hops := forwarded(r.Header.Values("X-Forwarded-For"))
	client := peer
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(hops[i])
		if err != nil || a.Zone() != "" {
			return client
		}
		client = a.Unmap()
		if !trusted.Contains(client) {
			return client
		}
	}
	return client
}

// RemoteAddr parses the peer address of a request ("host:port" or a bare address).
func RemoteAddr(s string) netip.Addr {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap()
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap()
	}
	return netip.Addr{}
}

func forwarded(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
