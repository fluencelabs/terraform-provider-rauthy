package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAPIKeyAccessUniqueGroups(t *testing.T) {
	ctx := context.Background()
	grant := func(group types.String, rights types.Set) attr.Value {
		return types.ObjectValueMust(
			apiKeyAccessAttrTypes,
			map[string]attr.Value{"group": group, "access_rights": rights},
		)
	}
	read := stringsToSet([]string{"read"})
	update := stringsToSet([]string{"update"})
	clients := types.StringValue("Clients")
	duplicate := []attr.Value{grant(clients, read), grant(clients, update)}
	cases := []struct {
		name  string
		value types.Set
		bad   bool
	}{
		{"duplicate", types.SetValueMust(apiKeyAccessObjectType(), duplicate), true},
		{"reversed", types.SetValueMust(apiKeyAccessObjectType(), []attr.Value{duplicate[1], duplicate[0]}), true},
		{
			"combined",
			accessSetOf(t, rauthy.APIKeyAccess{Group: "Clients", AccessRights: []string{"read", "update"}}),
			false,
		},
		{
			"distinct",
			accessSetOf(
				t,
				rauthy.APIKeyAccess{Group: "Clients", AccessRights: []string{"read"}},
				rauthy.APIKeyAccess{Group: "Users", AccessRights: []string{"update"}},
			),
			false,
		},
		{"empty", types.SetValueMust(apiKeyAccessObjectType(), nil), false},
		{"unknown-set", types.SetUnknown(apiKeyAccessObjectType()), false},
		{
			"unknown-group",
			types.SetValueMust(
				apiKeyAccessObjectType(),
				[]attr.Value{grant(types.StringUnknown(), read), grant(clients, update)},
			),
			false,
		},
		{
			"unknown-rights",
			types.SetValueMust(
				apiKeyAccessObjectType(),
				[]attr.Value{grant(clients, read), grant(clients, types.SetUnknown(types.StringType))},
			),
			true,
		},
		{
			"unknown-object",
			types.SetValueMust(
				apiKeyAccessObjectType(),
				[]attr.Value{types.ObjectUnknown(apiKeyAccessAttrTypes), grant(clients, read)},
			),
			false,
		},
	}
	a := apiKeyAccessAttributes()["access"].(schema.SetNestedAttribute)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d diag.Diagnostics
			for _, v := range a.Validators {
				var resp validator.SetResponse
				v.ValidateSet(ctx, validator.SetRequest{Path: path.Root("access"), ConfigValue: tc.value}, &resp)
				d.Append(resp.Diagnostics...)
			}
			if d.HasError() != tc.bad {
				t.Fatalf("validation = %v, want error %t", d, tc.bad)
			}
			if tc.bad && !strings.Contains(d.Errors()[0].Detail(), "Combine") {
				t.Fatalf("missing remedy: %v", d)
			}
		})
	}
}

func TestAuthProviderClaimPairs(t *testing.T) {
	ctx := context.Background()
	r := &authProviderResource{}
	for _, prefix := range []string{"admin", "mfa"} {
		for _, tc := range []struct {
			name        string
			path, value types.String
			bad         bool
		}{
			{"absent", types.StringNull(), types.StringNull(), false},
			{"complete", types.StringValue("$.roles.*"), types.StringValue("member"), false},
			{"path-only", types.StringValue("$.roles.*"), types.StringNull(), true},
			{"value-only", types.StringNull(), types.StringValue("member"), true},
			{"unknown-path", types.StringUnknown(), types.StringValue("member"), false},
			{"unknown-value", types.StringValue("$.roles.*"), types.StringUnknown(), false},
			{"unknown-both", types.StringUnknown(), types.StringUnknown(), false},
		} {
			t.Run(prefix+"/"+tc.name, func(t *testing.T) {
				state := resourceState(t, r, &authProviderResourceModel{Scopes: types.SetNull(types.StringType)})
				if d := state.SetAttribute(ctx, path.Root(prefix+"_claim_path"), tc.path); d.HasError() {
					t.Fatal(d)
				}
				if d := state.SetAttribute(ctx, path.Root(prefix+"_claim_value"), tc.value); d.HasError() {
					t.Fatal(d)
				}
				config := tfsdk.Config(state)
				d := validateClaimPair(ctx, config, prefix, tc.path, tc.value)
				if d.HasError() != tc.bad {
					t.Fatalf("validation = %v, want error %t", d, tc.bad)
				}
			})
		}
	}
}

func validateClaimPair(
	ctx context.Context,
	config tfsdk.Config,
	prefix string,
	claimPath, value types.String,
) diag.Diagnostics {
	var d diag.Diagnostics
	for field, v := range map[string]types.String{prefix + "_claim_path": claimPath, prefix + "_claim_value": value} {
		a := authProviderClaimAttributes()[field].(schema.StringAttribute)
		for _, check := range a.Validators {
			var resp validator.StringResponse
			check.ValidateString(
				ctx,
				validator.StringRequest{
					Path:           path.Root(field),
					PathExpression: path.MatchRoot(field),
					Config:         config,
					ConfigValue:    v,
				},
				&resp,
			)
			d.Append(resp.Diagnostics...)
		}
	}
	return d
}

func TestClientPlanUnknownSCIM(t *testing.T) {
	ctx := context.Background()
	r := &clientResource{}
	old := clientModelOf(t, fullResponse())
	old.Secret = types.StringValue("synthetic-secret")
	old.SecretRotationTrigger = types.StringValue("one")
	state := resourceState(t, r, &old)
	var scim types.Object
	if d := state.GetAttribute(ctx, path.Root("scim"), &scim); d.HasError() {
		t.Fatal(d)
	}
	attrs := scim.Attributes()
	attrs["base_uri"] = types.StringUnknown()
	partial := types.ObjectValueMust(scim.AttributeTypes(ctx), attrs)
	for _, tc := range []struct {
		name           string
		scim           types.Object
		rotate, public bool
	}{
		{"unknown-object", types.ObjectUnknown(scim.AttributeTypes(ctx)), false, false},
		{"unknown-field", partial, false, false},
		{"null", types.ObjectNull(scim.AttributeTypes(ctx)), false, false},
		{"rotate", types.ObjectUnknown(scim.AttributeTypes(ctx)), true, false},
		{"public", types.ObjectUnknown(scim.AttributeTypes(ctx)), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			planned := old
			if tc.rotate {
				planned.SecretRotationTrigger = types.StringValue("two")
			}
			if tc.public {
				planned.Confidential = types.BoolValue(false)
			}
			plan := resourcePlan(t, r, &planned)
			if d := plan.SetAttribute(ctx, path.Root("scim"), tc.scim); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state, Plan: plan, Config: tfsdk.Config(plan)}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			checkUnknownSCIMPlan(t, resp.Plan, tc.scim, old.Secret, tc.rotate || tc.public)
		})
	}
}

func checkUnknownSCIMPlan(t *testing.T, plan tfsdk.Plan, want types.Object, oldSecret types.String, changed bool) {
	t.Helper()
	ctx := context.Background()
	var got types.Object
	if d := plan.GetAttribute(ctx, path.Root("scim"), &got); d.HasError() || !got.Equal(want) {
		t.Fatalf("SCIM changed: %v %v", got, d)
	}
	var secret types.String
	if d := plan.GetAttribute(ctx, path.Root("secret"), &secret); d.HasError() {
		t.Fatal(d)
	}
	if changed {
		if !secret.IsUnknown() {
			t.Fatal("secret must become unknown")
		}
	} else if !secret.Equal(oldSecret) {
		t.Fatal("unchanged secret lost")
	}
}
