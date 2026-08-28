package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

func strPtr(s string) *string { return &s }

func setOf(t *testing.T, values ...string) types.Set {
	t.Helper()
	return stringsToSet(values)
}

// fullResponse is a client as Rauthy reports it, with every optional field set.
func fullResponse() *client.ClientResponse {
	return &client.ClientResponse{
		ID:                     "example-app",
		Name:                   strPtr("Example App"),
		Enabled:                true,
		Confidential:           true,
		RedirectURIs:           []string{"https://app.example.com/callback"},
		PostLogoutRedirectURIs: []string{"https://app.example.com/"},
		AllowedOrigins:         []string{"https://app.example.com"},
		FlowsEnabled:           []string{"authorization_code", "refresh_token"},
		AccessTokenAlg:         "EdDSA",
		IDTokenAlg:             "RS256",
		AuthCodeLifetime:       60,
		AccessTokenLifetime:    600,
		Scopes:                 []string{"openid", "profile"},
		DefaultScopes:          []string{"openid"},
		Challenges:             []string{"S256"},
		ForceMFA:               true,
		ClientURI:              strPtr("https://app.example.com"),
		Contacts:               []string{"ops@example.com"},
		BackchannelLogoutURI:   strPtr("https://app.example.com/logout"),
		RestrictGroupPrefix:    strPtr("staff"),
		Scim: &client.ScimClientRequestResponse{
			BearerToken:     "scim-token",
			BaseURI:         "https://app.example.com/scim/v2",
			SyncGroups:      true,
			GroupSyncPrefix: strPtr("staff"),
		},
	}
}

func TestSchemaIsValid(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	resp := &fwresource.SchemaResponse{}
	NewClientResource().Schema(ctx, fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid schema: %v", diags)
	}
}

func TestApplyResponse_RoundTripsEveryField(t *testing.T) {
	t.Parallel()

	var m clientResourceModel
	var diags diag.Diagnostics
	applyResponse(context.Background(), &m, fullResponse(), &diags)
	if diags.HasError() {
		t.Fatalf("applyResponse: %v", diags)
	}

	if m.ID.ValueString() != "example-app" || m.Name.ValueString() != "Example App" {
		t.Errorf("id/name = %v/%v", m.ID, m.Name)
	}
	if !m.Enabled.ValueBool() || !m.Confidential.ValueBool() || !m.ForceMFA.ValueBool() {
		t.Errorf("booleans lost: enabled=%v confidential=%v force_mfa=%v", m.Enabled, m.Confidential, m.ForceMFA)
	}
	if m.AccessTokenAlg.ValueString() != "EdDSA" || m.IDTokenAlg.ValueString() != "RS256" {
		t.Errorf("algorithms swapped: access=%v id=%v", m.AccessTokenAlg, m.IDTokenAlg)
	}
	if m.AuthCodeLifetime.ValueInt64() != 60 || m.AccessTokenLifetime.ValueInt64() != 600 {
		t.Errorf("lifetimes swapped: auth_code=%v access_token=%v", m.AuthCodeLifetime, m.AccessTokenLifetime)
	}
	if m.Scim == nil || m.Scim.BaseURI.ValueString() != "https://app.example.com/scim/v2" {
		t.Errorf("scim lost: %+v", m.Scim)
	}
}

// The model must survive a round trip back into an update request: what Rauthy
// reported is what we would send back.
func TestBuildUpdateRequest_RoundTripsEveryField(t *testing.T) {
	t.Parallel()

	var m clientResourceModel
	var diags diag.Diagnostics
	applyResponse(context.Background(), &m, fullResponse(), &diags)

	req := buildUpdateRequest(context.Background(), &m, &diags)
	if diags.HasError() {
		t.Fatalf("buildUpdateRequest: %v", diags)
	}
	orig := fullResponse()
	if req.ID != orig.ID || *req.Name != *orig.Name || req.Confidential != orig.Confidential {
		t.Errorf("identity lost on round trip: %+v", req)
	}
	if req.AccessTokenAlg != orig.AccessTokenAlg || req.AccessTokenLifetime != orig.AccessTokenLifetime {
		t.Errorf("token settings lost on round trip: %+v", req)
	}
	if !req.Enabled || !req.ForceMFA {
		t.Errorf("booleans lost on round trip: enabled=%v force_mfa=%v", req.Enabled, req.ForceMFA)
	}
	if len(req.FlowsEnabled) != 2 || len(req.Scopes) != 2 || len(req.DefaultScopes) != 1 {
		t.Errorf("lists lost on round trip: %+v", req)
	}
	if req.Scim == nil || req.Scim.BearerToken != "scim-token" || !req.Scim.SyncGroups {
		t.Errorf("scim lost on round trip: %+v", req.Scim)
	}
	if req.RestrictGroupPrefix == nil || *req.RestrictGroupPrefix != "staff" {
		t.Errorf("restrict_group_prefix lost on round trip: %v", req.RestrictGroupPrefix)
	}
}

// Rauthy distinguishes "no list" from "empty list"; a null set must not turn
// into [] on the wire, or a PUT would clear a field the practitioner never
// mentioned.
func TestApplyResponse_AbsentOptionalListsBecomeNull(t *testing.T) {
	t.Parallel()

	resp := fullResponse()
	resp.PostLogoutRedirectURIs = nil
	resp.AllowedOrigins = nil
	resp.Contacts = nil
	resp.Challenges = nil
	resp.Scim = nil
	resp.Name = nil
	resp.ClientURI = nil

	var m clientResourceModel
	var diags diag.Diagnostics
	applyResponse(context.Background(), &m, resp, &diags)

	for name, v := range map[string]types.Set{
		"post_logout_redirect_uris": m.PostLogoutRedirectURIs,
		"allowed_origins":           m.AllowedOrigins,
		"contacts":                  m.Contacts,
		"challenges":                m.Challenges,
	} {
		if !v.IsNull() {
			t.Errorf("%s = %v, want null", name, v)
		}
	}
	if !m.Name.IsNull() || !m.ClientURI.IsNull() {
		t.Errorf("absent strings not null: name=%v client_uri=%v", m.Name, m.ClientURI)
	}
	if m.Scim != nil {
		t.Errorf("scim = %+v, want nil", m.Scim)
	}

	req := buildUpdateRequest(context.Background(), &m, &diags)
	if req.PostLogoutRedirectURIs != nil || req.AllowedOrigins != nil || req.Contacts != nil || req.Challenges != nil {
		t.Errorf("null sets marshalled as empty lists: %+v", req)
	}
	if req.Name != nil || req.ClientURI != nil || req.Scim != nil {
		t.Errorf("null attributes marshalled as values: %+v", req)
	}
}

// After the POST half of a create, everything the practitioner left unset is
// unknown. The PUT that follows needs concrete values, and Rauthy's own
// defaults are the only sensible source.
func TestFillUnknownFromResponse_AdoptsServerDefaults(t *testing.T) {
	t.Parallel()

	m := clientResourceModel{
		ID:                  types.StringValue("example-app"),
		Confidential:        types.BoolValue(true),
		RedirectURIs:        setOf(t, "https://app.example.com/callback"),
		Enabled:             types.BoolUnknown(),
		FlowsEnabled:        types.SetUnknown(types.StringType),
		AccessTokenAlg:      types.StringUnknown(),
		IDTokenAlg:          types.StringUnknown(),
		AuthCodeLifetime:    types.Int64Unknown(),
		AccessTokenLifetime: types.Int64Unknown(),
		Scopes:              types.SetUnknown(types.StringType),
		DefaultScopes:       types.SetUnknown(types.StringType),
		Challenges:          types.SetUnknown(types.StringType),
		ForceMFA:            types.BoolUnknown(),
		Secret:              types.StringUnknown(),
	}

	var diags diag.Diagnostics
	fillUnknownFromResponse(context.Background(), &m, fullResponse(), &diags)

	req := buildUpdateRequest(context.Background(), &m, &diags)
	if diags.HasError() {
		t.Fatalf("buildUpdateRequest: %v", diags)
	}

	// Rauthy rejects a PUT that omits these, so none of them may still be zero.
	if req.AccessTokenAlg == "" || req.IDTokenAlg == "" {
		t.Errorf("algorithms unresolved: %+v", req)
	}
	if req.AuthCodeLifetime == 0 || req.AccessTokenLifetime == 0 {
		t.Errorf("lifetimes unresolved: %+v", req)
	}
	if len(req.FlowsEnabled) == 0 || len(req.Scopes) == 0 || len(req.DefaultScopes) == 0 {
		t.Errorf("required lists unresolved: %+v", req)
	}
	if !req.Enabled {
		t.Error("enabled unresolved; a PUT without it is a 400")
	}
}

// Values the practitioner did configure must survive; the server's defaults
// only fill the gaps.
func TestFillUnknownFromResponse_KeepsConfiguredValues(t *testing.T) {
	t.Parallel()

	m := clientResourceModel{
		ID:                  types.StringValue("example-app"),
		Confidential:        types.BoolValue(true),
		AccessTokenAlg:      types.StringValue("RS512"),
		AccessTokenLifetime: types.Int64Value(1200),
		Scopes:              setOf(t, "openid", "email"),
		Enabled:             types.BoolValue(false),
		FlowsEnabled:        types.SetUnknown(types.StringType),
		IDTokenAlg:          types.StringUnknown(),
		AuthCodeLifetime:    types.Int64Unknown(),
		DefaultScopes:       types.SetUnknown(types.StringType),
		Challenges:          types.SetUnknown(types.StringType),
		ForceMFA:            types.BoolUnknown(),
	}

	var diags diag.Diagnostics
	fillUnknownFromResponse(context.Background(), &m, fullResponse(), &diags)

	if m.AccessTokenAlg.ValueString() != "RS512" {
		t.Errorf("access_token_alg = %v, want RS512 from the configuration", m.AccessTokenAlg)
	}
	if m.AccessTokenLifetime.ValueInt64() != 1200 {
		t.Errorf("access_token_lifetime = %v, want 1200", m.AccessTokenLifetime)
	}
	if m.Enabled.ValueBool() {
		t.Error("enabled overwritten by the server default")
	}
	if m.IDTokenAlg.ValueString() != "RS256" {
		t.Errorf("id_token_alg = %v, want the server default RS256", m.IDTokenAlg)
	}
}

// A public client has no secret, so the unknown must resolve to null rather
// than staying unknown after apply.
func TestFillUnknownFromResponse_SecretResolvesToNull(t *testing.T) {
	t.Parallel()

	m := clientResourceModel{Secret: types.StringUnknown()}
	var diags diag.Diagnostics
	fillUnknownFromResponse(context.Background(), &m, fullResponse(), &diags)
	if !m.Secret.IsNull() {
		t.Errorf("secret = %v, want null before it is read from the secret endpoint", m.Secret)
	}
}

func TestRotationRequested(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		state, plan types.String
		want        bool
	}{
		{"unset on both sides", types.StringNull(), types.StringNull(), false},
		{"unchanged", types.StringValue("v1"), types.StringValue("v1"), false},
		{"changed", types.StringValue("v1"), types.StringValue("v2"), true},
		{"newly set", types.StringNull(), types.StringValue("v1"), true},
		{"removed", types.StringValue("v1"), types.StringNull(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := rotationRequested(
				&clientResourceModel{SecretRotationTrigger: tc.state},
				&clientResourceModel{SecretRotationTrigger: tc.plan},
			)
			if got != tc.want {
				t.Errorf("rotationRequested = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSetConversionsRoundTrip(t *testing.T) {
	t.Parallel()

	var diags diag.Diagnostics
	in := []string{"openid", "profile", "email"}
	got := setToStrings(context.Background(), stringsToSet(in), &diags)
	if diags.HasError() {
		t.Fatalf("conversion: %v", diags)
	}
	if len(got) != len(in) {
		t.Fatalf("round trip lost elements: %v -> %v", in, got)
	}

	if v := setToStrings(context.Background(), types.SetNull(types.StringType), &diags); v != nil {
		t.Errorf("null set became %v, want nil", v)
	}
	if v := setToStrings(context.Background(), types.SetUnknown(types.StringType), &diags); v != nil {
		t.Errorf("unknown set became %v, want nil", v)
	}
}
