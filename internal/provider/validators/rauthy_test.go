package validators_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

func acceptsString(v validator.String, in string) bool {
	req := validator.StringRequest{Path: path.Root("attr"), ConfigValue: types.StringValue(in)}
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), req, resp)
	return !resp.Diagnostics.HasError()
}

func acceptsSet(vs []validator.Set, values ...string) bool {
	elems := make([]attr.Value, 0, len(values))
	for _, s := range values {
		elems = append(elems, types.StringValue(s))
	}
	req := validator.SetRequest{
		Path:        path.Root("attr"),
		ConfigValue: types.SetValueMust(types.StringType, elems),
	}
	for _, v := range vs {
		resp := &validator.SetResponse{}
		v.ValidateSet(context.Background(), req, resp)
		if resp.Diagnostics.HasError() {
			return false
		}
	}
	return true
}

func checkString(t *testing.T, name string, v validator.String, valid, invalid []string) {
	t.Helper()
	for _, in := range valid {
		if !acceptsString(v, in) {
			t.Errorf("%s rejected %q, want accepted", name, in)
		}
	}
	for _, in := range invalid {
		if acceptsString(v, in) {
			t.Errorf("%s accepted %q, want rejected", name, in)
		}
	}
}

func TestClientID(t *testing.T) {
	t.Parallel()
	checkString(t, "ClientID", validators.ClientID(),
		[]string{
			"app",
			"my-app_2",
			"app.example.com",
			"MixedCase.App-1_2",
		},
		[]string{
			"",             // shorter than 2
			"a",            // shorter than 2
			"has space",    // space is not in the class
			"quote\"quote", // double quote is not in the class
			"<script>",
			"https://app.example.com", // 0.36 dropped `:` and `/` from RE_CLIENT_ID
			"a+b%c",                   // and `+` and `%` with them
		},
	)
}

func TestClientName(t *testing.T) {
	t.Parallel()
	checkString(t, "ClientName", validators.ClientName(),
		[]string{
			"Example App",
			"Anwendung Über", // Latin-1 supplement, inside À-ɏ
			"アプリ",            // katakana, explicitly allowed upstream
			"App-2",
		},
		[]string{
			"a",             // shorter than 2
			"app@example",   // '@' is not in the class
			"app/dashboard", // '/' is not in the class
			// Rauthy's RE_CLIENT_NAME covers Latin (À-ɏ) and CJK but not
			// Cyrillic or Greek, so those names are rejected upstream too.
			"Приложение",
		},
	)
}

func TestURI(t *testing.T) {
	t.Parallel()
	checkString(t, "URI", validators.URI(),
		[]string{
			"https://app.example.com/callback",
			"https://app.example.com/cb?x=1#frag",
			"ops@example.com",
		},
		[]string{
			"",
			"https://app.example.com/a b",
			`https://app.example.com/"`,
		},
	)
}

func TestSigningAlg(t *testing.T) {
	t.Parallel()
	checkString(t, "SigningAlg", validators.SigningAlg(),
		[]string{"RS256", "RS384", "RS512", "EdDSA"},
		[]string{"", "HS256", "ES256", "eddsa", "RS128"},
	)
}

func TestGroupPrefix(t *testing.T) {
	t.Parallel()
	checkString(t, "GroupPrefix", validators.GroupPrefix(),
		[]string{"staff", "org:eng/*", "a b"},
		[]string{"a", "team@corp", "prefix#1"},
	)
}

func TestOriginSet(t *testing.T) {
	t.Parallel()

	if !acceptsSet([]validator.Set{validators.OriginSet()}, "https://app.example.com", "http://localhost:8080") {
		t.Error("OriginSet rejected valid origins")
	}
	// Rauthy's origin pattern is scheme + host only; a path or an uppercase
	// host does not match.
	for _, bad := range []string{"https://app.example.com/callback", "https://App.Example.com", "app.example.com"} {
		if acceptsSet([]validator.Set{validators.OriginSet()}, bad) {
			t.Errorf("OriginSet accepted %q, want rejected", bad)
		}
	}
}

func TestScopeSet(t *testing.T) {
	t.Parallel()

	if !acceptsSet([]validator.Set{validators.ScopeSet()}, "openid", "profile", "urn:custom") {
		t.Error("ScopeSet rejected valid scopes")
	}
	for _, bad := range []string{"a", "scope with space", "scope@x"} {
		if acceptsSet([]validator.Set{validators.ScopeSet()}, bad) {
			t.Errorf("ScopeSet accepted %q, want rejected", bad)
		}
	}
}

func TestContactSet(t *testing.T) {
	t.Parallel()

	if !acceptsSet([]validator.Set{validators.ContactSet()}, "ops@example.com", "+1234") {
		t.Error("ContactSet rejected valid contacts")
	}
	if acceptsSet([]validator.Set{validators.ContactSet()}, "ops example") {
		t.Error("ContactSet accepted a value with a space")
	}
}

// Rauthy rejects flows_enabled and challenges when they are present but empty,
// so the size bound is part of the contract rather than a nicety.
func TestFlowsEnabledSet(t *testing.T) {
	t.Parallel()

	vs := validators.FlowsEnabledSet()
	if !acceptsSet(vs, "authorization_code", "refresh_token") {
		t.Error("FlowsEnabledSet rejected valid flows")
	}
	if !acceptsSet(vs, "urn:ietf:params:oauth:grant-type:device_code") {
		t.Error("FlowsEnabledSet rejected the device code grant")
	}
	if acceptsSet(vs) {
		t.Error("FlowsEnabledSet accepted an empty set, which Rauthy rejects")
	}
	if acceptsSet(vs, "implicit") {
		t.Error("FlowsEnabledSet accepted an unsupported flow")
	}
}

func TestChallengeSet(t *testing.T) {
	t.Parallel()

	vs := validators.ChallengeSet()
	if !acceptsSet(vs, "S256") || !acceptsSet(vs, "plain", "S256") {
		t.Error("ChallengeSet rejected valid challenge methods")
	}
	if acceptsSet(vs) {
		t.Error("ChallengeSet accepted an empty set, which Rauthy rejects")
	}
	if acceptsSet(vs, "s256") {
		t.Error("ChallengeSet accepted lowercase s256; the upstream pattern is case sensitive")
	}
}

func TestLifetimeBounds(t *testing.T) {
	t.Parallel()

	acceptsInt := func(v validator.Int64, in int64) bool {
		req := validator.Int64Request{Path: path.Root("attr"), ConfigValue: types.Int64Value(in)}
		resp := &validator.Int64Response{}
		v.ValidateInt64(context.Background(), req, resp)
		return !resp.Diagnostics.HasError()
	}

	cases := []struct {
		name          string
		v             validator.Int64
		low, ok, high int64
		lowOK, highOK bool
	}{
		{"auth_code_lifetime", validators.AuthCodeLifetime(), 9, 60, 301, false, false},
		{"access_token_lifetime", validators.AccessTokenLifetime(), 9, 600, 86401, false, false},
		{"cache_current_hours", validators.CacheCurrentHours(), 0, 12, 25, false, false},
	}
	for _, tc := range cases {
		if !acceptsInt(tc.v, tc.ok) {
			t.Errorf("%s rejected %d", tc.name, tc.ok)
		}
		if acceptsInt(tc.v, tc.low) != tc.lowOK {
			t.Errorf("%s accepted %d below the lower bound", tc.name, tc.low)
		}
		if acceptsInt(tc.v, tc.high) != tc.highOK {
			t.Errorf("%s accepted %d above the upper bound", tc.name, tc.high)
		}
	}
}

func TestIPAddress(t *testing.T) {
	t.Parallel()

	v := validators.IPAddress()
	for _, in := range []string{"203.0.113.7", "2001:db8::2", "2001:0DB8:0000:0000:0000:0000:0000:0002"} {
		if !acceptsString(v, in) {
			t.Errorf("IPAddress rejected %q", in)
		}
	}
	// A CIDR prefix, a port and a hostname are all things a user might reach
	// for; Rauthy accepts none of them.
	for _, in := range []string{"203.0.113.0/24", "203.0.113.7:443", "example.com", ""} {
		if acceptsString(v, in) {
			t.Errorf("IPAddress accepted %q", in)
		}
	}
}

func TestBlacklistExp(t *testing.T) {
	t.Parallel()

	v := validators.BlacklistExp()
	req := func(n int64) bool {
		resp := &validator.Int64Response{}
		v.ValidateInt64(context.Background(),
			validator.Int64Request{Path: path.Root("exp"), ConfigValue: types.Int64Value(n)}, resp)
		return !resp.Diagnostics.HasError()
	}

	if req(1719784799) {
		t.Error("BlacklistExp accepted a value below Rauthy's fixed lower bound")
	}
	if !req(1719784800) {
		t.Error("BlacklistExp rejected Rauthy's own lower bound")
	}
	if !req(4102444800) {
		t.Error("BlacklistExp rejected a far-future timestamp")
	}
}
