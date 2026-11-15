package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/fluencelabs/rauthy-go"
)

// Rauthy does not keep auth provider names unique. A lookup by a name two
// providers share must fail and say why, rather than pick one or report the
// name as missing.
func TestAuthProviderDataSource_AmbiguousNameIsAnError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fake, api := newFakeRauthy(t)
	fake.answer("POST /providers", []rauthy.AuthProviderResponse{
		{ID: "a", Name: "corp"},
		{ID: "b", Name: "corp"},
	})
	d := &authProviderDataSource{api: api}

	config := dataSourceConfig(t, d, &authProviderDataSourceModel{
		Name:   types.StringValue("corp"),
		Scopes: types.SetNull(types.StringType),
	})
	resp := datasource.ReadResponse{State: tfsdk.State{
		Schema: config.Schema,
		Raw:    tftypes.NewValue(config.Raw.Type(), nil),
	}}
	d.Read(ctx, datasource.ReadRequest{Config: config}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("an ambiguous name resolved without an error")
	}
	got := resp.Diagnostics.Errors()[0]
	if !strings.Contains(got.Summary(), "Ambiguous") {
		t.Errorf("summary = %q, want it to name the ambiguity", got.Summary())
	}
	if !strings.Contains(got.Detail(), "distinct names") {
		t.Errorf("detail does not point to a way out:\n%s", got.Detail())
	}
}
