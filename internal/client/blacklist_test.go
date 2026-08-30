package client_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// The listing is the only read path, and the caller looks the entry up by the
// spelling from its Terraform configuration while Rauthy stores its own
// canonical rendering. The lookup therefore compares parsed addresses.
func TestGetBlacklistedIP_MatchesNonCanonicalSpelling(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ips":[{"ip":"2001:db8::2","exp":4102444800}]}`))
	})

	got, err := c.GetBlacklistedIP(context.Background(), "2001:0DB8:0000:0000:0000:0000:0000:0002")
	if err != nil {
		t.Fatalf("GetBlacklistedIP: %v", err)
	}
	if got.IP != "2001:db8::2" {
		t.Errorf("IP = %q, want Rauthy's canonical rendering", got.IP)
	}
	if got.Exp != 4102444800 {
		t.Errorf("Exp = %d, want 4102444800", got.Exp)
	}
}

// An address that is not blacklisted has to look like a 404 so the resource can
// drop itself from state; Rauthy itself never says so.
func TestGetBlacklistedIP_MissingEntryIsNotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ips":[]}`))
	})

	_, err := c.GetBlacklistedIP(context.Background(), "203.0.113.7")
	if !client.IsNotFound(err) {
		t.Fatalf("err = %v, want a synthetic 404", err)
	}
}

// Different hosts must not collide just because one string contains the other.
func TestGetBlacklistedIP_DoesNotMatchADifferentAddress(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ips":[{"ip":"203.0.113.70","exp":4102444800}]}`))
	})

	if _, err := c.GetBlacklistedIP(context.Background(), "203.0.113.7"); !client.IsNotFound(err) {
		t.Fatalf("err = %v, want a synthetic 404", err)
	}
}

// The IPv6 colons must survive into the path rather than being percent-encoded
// into something Rauthy cannot parse.
func TestDeleteBlacklistedIP_PutsTheAddressInThePath(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	c := newTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
	})

	if err := c.DeleteBlacklistedIP(context.Background(), "2001:db8::2"); err != nil {
		t.Fatalf("DeleteBlacklistedIP: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/auth/v1/blacklist/2001:db8::2" {
		t.Errorf("got %s %s, want DELETE /auth/v1/blacklist/2001:db8::2", gotMethod, gotPath)
	}
}
