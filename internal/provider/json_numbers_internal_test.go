package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestJSONNumberValidation(t *testing.T) {
	cases := []struct{ document, rejected string }{
		{`-974132620847463.82`, `-974132620847463.82`},
		{`{"nested":[{"n":-974132620847463.82}]}`, `-974132620847463.82`},
		{`-9230.91995e-68`, `-9230.91995e-68`},
		{`1.2345678901234568`, `1.2345678901234568`},
		{`-18446744073709551615`, `-18446744073709551615`},
		{`18446744073709551616`, `18446744073709551616`},
		{`1.0000000000000001`, `1.0000000000000001`},
		{`1e9999999999999999`, `1e9999999999999999`},
		{`1e16`, `1e16`},
		{`1e-16`, `1e-16`},
		{`123456789012345.0`, `123456789012345.0`},
		{`-2336.817646828`, ""},
		{`1.49`, ""},
		{`690.7535244162`, ""},
		{`1e15`, ""},
		{`1e-15`, ""},
		{`123456789012345`, ""},
		{`0.123456789012345`, ""},
		{`18446744073709551615`, ""},
		{`-9223372036854775808`, ""},
		{`9007199254740993`, ""},
		{`{"nested":[0.1,12.345,1e2],"text":"-974132620847463.82"}`, ""},
		{`null`, ""},
	}
	ctx := context.Background()
	r := &userAttributeResource{}
	for _, tc := range cases {
		t.Run(tc.document, func(t *testing.T) {
			model := userAttributeResourceModel{
				Name:         types.StringValue("numbers"),
				DefaultValue: newJSONString(tc.document),
			}
			state := resourceState(t, r, &model)
			var resp resource.ValidateConfigResponse
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config(state)}, &resp)
			if resp.Diagnostics.HasError() != (tc.rejected != "") {
				t.Fatalf("unexpected validation: %v", resp.Diagnostics)
			}
			if tc.rejected != "" &&
				(!strings.Contains(resp.Diagnostics.Errors()[0].Detail(), tc.rejected) || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "round trip")) {
				t.Fatalf("missing number or cause: %v", resp.Diagnostics)
			}
		})
	}
	for _, value := range []jsonString{nullJSONString(), {StringValue: types.StringUnknown()}} {
		model := userAttributeResourceModel{Name: types.StringValue("numbers"), DefaultValue: value}
		state := resourceState(t, r, &model)
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config(state)}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
	}
}
