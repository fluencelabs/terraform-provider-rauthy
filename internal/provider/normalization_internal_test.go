package provider

import (
	"context"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNormalization_client_sets_and_scim(t *testing.T) {
	ctx := context.Background()
	got := fullResponse()
	m := clientModelOf(t, got)
	m.PostLogoutRedirectURIs = stringSet(t)
	m.AllowedOrigins = stringSet(t)
	m.Contacts = stringSet(t)
	m.Challenges = stringSet(t)
	m.Scim.BaseURI = types.StringValue(got.Scim.BaseURI + "/")
	want := m
	got.PostLogoutRedirectURIs = nil
	got.AllowedOrigins = nil
	got.Contacts = nil
	got.Challenges = nil
	var d diag.Diagnostics
	applyResponse(ctx, &m, got, &d)
	if !m.PostLogoutRedirectURIs.Equal(want.PostLogoutRedirectURIs) || !m.AllowedOrigins.Equal(want.AllowedOrigins) ||
		!m.Contacts.Equal(want.Contacts) ||
		!m.Challenges.Equal(want.Challenges) ||
		!m.Scim.BaseURI.Equal(want.Scim.BaseURI) {
		t.Fatalf("normalized response lost planned values: %+v", m)
	}
	got.Scim.BaseURI = "https://other.example/scim"
	applyResponse(ctx, &m, got, &d)
	if m.Scim.BaseURI.ValueString() != got.Scim.BaseURI {
		t.Fatal("real SCIM drift hidden")
	}
}

func TestNormalization_openid(t *testing.T) {
	ctx := context.Background()
	var sr resource.SchemaResponse
	NewClientResource().Schema(ctx, resource.SchemaRequest{}, &sr)
	for _, field := range []string{"scopes", "default_scopes"} {
		a := sr.Schema.Attributes[field].(schema.SetAttribute)
		for _, values := range [][]string{{"profile"}, {}, {"openid", "profile"}} {
			var d diag.Diagnostics
			for _, v := range a.Validators {
				var out validator.SetResponse
				v.ValidateSet(
					ctx,
					validator.SetRequest{Path: path.Root(field), ConfigValue: stringSet(t, values...)},
					&out,
				)
				d.Append(out.Diagnostics...)
			}
			wantError := len(values) < 2
			if d.HasError() != wantError {
				t.Fatalf("%s %v: %v", field, values, d)
			}
		}
	}
}

func TestNormalization_email(t *testing.T) {
	m := userResourceModel{Email: types.StringValue("Ada@Example.com")}
	applyUser(&m, &rauthy.UserResponse{Email: "ada@example.com"})
	if m.Email.ValueString() != "Ada@Example.com" {
		t.Fatal("normalized email replaced plan")
	}
	applyUser(&m, &rauthy.UserResponse{Email: "other@example.com"})
	if m.Email.ValueString() != "other@example.com" {
		t.Fatal("real email drift hidden")
	}
}

func TestNormalization_JSON_null_and_integers(t *testing.T) {
	ctx := context.Background()
	m := userAttributeResourceModel{DefaultValue: newJSONString(" null ")}
	applyUserAttr(&m, &rauthy.UserAttrResponse{Name: "attr"})
	if m.DefaultValue.ValueString() != " null " {
		t.Fatal("JSON null replaced plan")
	}
	same, d := newJSONString(
		`{"n":9007199254740992}`,
	).StringSemanticEquals(ctx, newJSONString(`{"n":9007199254740993}`))
	if d.HasError() || same {
		t.Fatal("large integer drift hidden")
	}
	m.DefaultValue = newJSONString(`1`)
	applyUserAttr(&m, &rauthy.UserAttrResponse{Name: "attr"})
	if !m.DefaultValue.IsNull() {
		t.Fatal("removed JSON default hidden")
	}
}

func TestNormalization_IPv6(t *testing.T) {
	ctx := context.Background()
	m := pamHostResourceModel{IPs: stringSet(t, "2001:0DB8:0000:0000:0000:0000:0000:0002")}
	want := m.IPs
	got := &rauthy.PamHostDetailsResponse{IPs: []string{"2001:db8::2"}, Aliases: []string{}}
	d := applyPamHostDetails(ctx, &m, got, rauthy.NewSecret("secret"))
	if d.HasError() || !m.IPs.Equal(want) {
		t.Fatalf("IPv6 changed: %v", m.IPs)
	}
	got.IPs = []string{"2001:db8::3"}
	d = applyPamHostDetails(ctx, &m, got, rauthy.NewSecret("secret"))
	if d.HasError() || !m.IPs.Equal(stringSet(t, "2001:db8::3")) {
		t.Fatal("real IP drift hidden")
	}
}

// An email Rauthy lowercased keeps its configured spelling; another is drift.
func TestEmailValue(t *testing.T) {
	t.Parallel()
	known := types.StringValue("Cold@Example.COM")
	if got := emailValue(known, "cold@example.com"); !got.Equal(known) {
		t.Errorf("emailValue dropped the configured value: %v", got)
	}
	if got := emailValue(known, "other@example.com"); got.ValueString() != "other@example.com" {
		t.Errorf("emailValue hid a real change: %v", got)
	}
}
