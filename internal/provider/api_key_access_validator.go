package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type apiKeyUniqueGroups struct{}

func (apiKeyUniqueGroups) Description(context.Context) string {
	return "Each access group must appear once. Combine its rights in one entry because Rauthy uses only the first matching group."
}

func (v apiKeyUniqueGroups) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v apiKeyUniqueGroups) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	groups := make(map[string]types.Object)
	for _, element := range req.ConfigValue.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			continue
		}
		value, err := object.ToTerraformValue(ctx)
		if err != nil {
			resp.Diagnostics.AddAttributeError(req.Path, "Could not validate API key access", err.Error())
			return
		}
		if !value.IsFullyKnown() {
			// Unknown rights can resolve to an identical element, which Terraform collapses.
			continue
		}
		group, ok := object.Attributes()["group"].(types.String)
		if !ok || group.IsNull() || group.IsUnknown() {
			continue
		}
		name := group.ValueString()
		if previous, exists := groups[name]; exists && !previous.Equal(object) {
			resp.Diagnostics.AddAttributeError(req.Path, "Duplicate API key access group", v.Description(ctx))
			return
		}
		groups[name] = object
	}
}
