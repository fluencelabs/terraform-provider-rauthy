package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clientOpenIDScopes struct{}

func (clientOpenIDScopes) Description(context.Context) string {
	return "must include openid, which Rauthy always grants"
}
func (v clientOpenIDScopes) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v clientOpenIDScopes) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, value := range req.ConfigValue.Elements() {
		scope, ok := value.(types.String)
		if !ok || scope.IsUnknown() || scope.ValueString() == "openid" {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Missing openid scope", v.Description(ctx))
}
