package provider

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// Every form a live 0.36.2 has been seen to return the scope list in has to
// parse to the same set. The `+`-joined form comes from a stored provider, the
// space-separated one with a trailing space comes from the lookup endpoint, and
// the collapsed-whitespace and tab cases are what Rauthy's own join leaves
// behind when the request used something other than single spaces.
func TestSplitAuthProviderScope(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"openid+profile+email": {"openid", "profile", "email"},
		"openid profile email": {"openid", "profile", "email"},
		// The lookup endpoint's trailing space.
		"openid profile email ": {"openid", "profile", "email"},
		// A tab survives Rauthy's join verbatim.
		"openid+profile\temail": {"openid", "profile", "email"},
		"openid":                {"openid"},
		// An empty scope must be an empty set, not a null one: the attribute
		// is required, and a null there is a "provider produced inconsistent
		// result" error rather than a clean plan.
		"": {},
	}

	for in, want := range tests {
		got := splitAuthProviderScope(in)
		if got == nil {
			t.Errorf("splitAuthProviderScope(%q) = nil, want a non-nil slice", in)
			continue
		}
		if !slices.Equal(got, want) {
			t.Errorf("splitAuthProviderScope(%q) = %v, want %v", in, got, want)
		}
	}
}

// The wire value must be stable regardless of the order the set happens to be
// iterated in, or an unrelated change would rewrite `scope` on the server.
func TestJoinAuthProviderScopes_IsOrderIndependent(t *testing.T) {
	t.Parallel()

	var diags diag.Diagnostics
	one := joinAuthProviderScopes(context.Background(), scopeSetOf("profile", "openid", "email"), &diags)
	two := joinAuthProviderScopes(context.Background(), scopeSetOf("email", "openid", "profile"), &diags)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if one != two || one != "email openid profile" {
		t.Errorf("got %q and %q, want a stable %q", one, two, "email openid profile")
	}
}

// The round trip that actually matters: what Rauthy sends back must, after
// being parsed and re-rendered, be something Rauthy will accept on the next
// write. It is not the same string, and that is the whole point.
func TestAuthProviderScope_ReadThenWriteIsAcceptable(t *testing.T) {
	t.Parallel()

	m := &authProviderResourceModel{}
	applyAuthProviderResponse(m, &client.AuthProviderResponse{Scope: "openid+profile+email"})

	var diags diag.Diagnostics
	got := buildAuthProviderRequest(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.Scope != "email openid profile" {
		t.Errorf("scope = %q, want the space-separated form Rauthy's validator accepts", got.Scope)
	}
}

// The secret is resent on every update on purpose: omitting it does not leave
// the stored secret alone, it erases it.
func TestBuildAuthProviderRequest_AlwaysCarriesTheSecret(t *testing.T) {
	t.Parallel()

	var diags diag.Diagnostics
	m := &authProviderResourceModel{
		ClientSecret: types.StringValue("upstream-secret"),
		Scopes:       scopeSetOf("openid"),
	}
	got := buildAuthProviderRequest(context.Background(), m, &diags)
	if got.ClientSecret == nil || *got.ClientSecret != "upstream-secret" {
		t.Errorf("client_secret = %v, want it resent", got.ClientSecret)
	}

	// A configuration with no secret sends none, which is how a secret is
	// removed from a provider.
	m.ClientSecret = types.StringNull()
	got = buildAuthProviderRequest(context.Background(), m, &diags)
	if got.ClientSecret != nil {
		t.Errorf("client_secret = %v, want it omitted", *got.ClientSecret)
	}
}

func scopeSetOf(values ...string) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}
