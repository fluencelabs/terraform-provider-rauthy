package client

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"
)

// IPBlacklistRequest is the body of POST /blacklist.
//
// Exp is a Unix timestamp in seconds. Rauthy refuses anything below a fixed
// lower bound rather than below "now" — see blacklistExpMin.
type IPBlacklistRequest struct {
	IP  string `json:"ip"`
	Exp int64  `json:"exp"`
}

// BlacklistedIP is one blacklist entry as Rauthy returns it from
// GET /blacklist. The IP is Rauthy's own rendering of the address, which is not
// necessarily the spelling it was created with: it parses the value into a Rust
// IpAddr and prints that back, so `2001:0DB8:0000::0002` returns as
// `2001:db8::2`.
type BlacklistedIP struct {
	IP  string `json:"ip"`
	Exp int64  `json:"exp"`
}

// blacklistListResponse wraps the list. As with /users/attr, GET /blacklist
// does not answer with a bare array — the entries sit under `ips`.
type blacklistListResponse struct {
	IPs []BlacklistedIP `json:"ips"`
}

const blacklistPathBase = "/blacklist"

func blacklistPath(ip string) string {
	return blacklistPathBase + "/" + url.PathEscape(ip)
}

// BlacklistIP issues POST /blacklist. Requires the Blacklist:Create right.
//
// This is an upsert, not a create: posting an IP that is already blacklisted
// replaces its expiry and still answers 200. That is what lets the resource
// change `exp` in place instead of having to replace itself.
//
// SILENT NO-OP, found against a live Rauthy 0.36.2 and invisible in the OpenAPI
// document: a POST whose `exp` is already in the past is accepted with 200 and
// then discarded — the entry never appears in GET /blacklist. Rauthy validates
// `exp` against a hardcoded lower bound (see blacklistExpMin), not against the
// current time, so a past-but-above-the-bound timestamp passes validation and
// is then dropped as already expired. Callers must read the entry back rather
// than trust the 200.
func (c *Client) BlacklistIP(ctx context.Context, req IPBlacklistRequest) error {
	return c.do(ctx, http.MethodPost, blacklistPathBase, req, nil)
}

// ListBlacklistedIPs issues GET /blacklist. Requires Blacklist:Read.
func (c *Client) ListBlacklistedIPs(ctx context.Context) ([]BlacklistedIP, error) {
	var out blacklistListResponse
	if err := c.do(ctx, http.MethodGet, blacklistPathBase, nil, &out); err != nil {
		return nil, err
	}
	return out.IPs, nil
}

// GetBlacklistedIP returns the entry for the given address, or a synthetic 404.
// The listing is the only read path Rauthy offers here, exactly as for roles,
// groups and scopes.
//
// Addresses are compared parsed rather than as strings: the caller holds the
// spelling from the Terraform configuration and Rauthy holds its own canonical
// rendering, and `2001:0DB8::2` and `2001:db8::2` are the same host. An
// unparseable value on either side falls back to a string comparison, which
// cannot match anything Rauthy would have accepted but keeps the function total.
func (c *Client) GetBlacklistedIP(ctx context.Context, ip string) (*BlacklistedIP, error) {
	entries, err := c.ListBlacklistedIPs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if sameIP(entries[i].IP, ip) {
			return &entries[i], nil
		}
	}
	return nil, notFoundError(blacklistPathBase, "ip "+ip+" is not blacklisted")
}

func sameIP(a, b string) bool {
	parsedA, errA := netip.ParseAddr(a)
	parsedB, errB := netip.ParseAddr(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return parsedA == parsedB
}

// DeleteBlacklistedIP issues DELETE /blacklist/{ip}. Requires Blacklist:Delete.
//
// The endpoint answers 200 whatever it is handed — an address that was never
// blacklisted, or a path segment that is not an address at all. There is
// therefore no 404 to distinguish "removed" from "was not there", which suits a
// Terraform delete but makes this useless as an existence check.
func (c *Client) DeleteBlacklistedIP(ctx context.Context, ip string) error {
	return c.do(ctx, http.MethodDelete, blacklistPath(ip), nil, nil)
}
