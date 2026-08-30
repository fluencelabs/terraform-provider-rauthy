package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// writeOnlyString reads a write-only attribute out of the configuration.
//
// A write-only value exists nowhere else. Terraform sends it in the config of
// the call that applies it and then forgets it: the plan reports it as null,
// the state never holds it, and a Read is handed nothing at all. So every
// caller that needs one has to reach past the plan into `req.Config` — hence
// this helper, rather than the usual `req.Plan.Get` into the model.
//
// Null is a legitimate answer: the practitioner may simply not have set the
// attribute. It is also what a Read or a Delete would see, which is precisely
// why neither may call this.
func writeOnlyString(
	ctx context.Context,
	config tfsdk.Config,
	p path.Path,
	diags *diag.Diagnostics,
) types.String {
	var v types.String
	diags.Append(config.GetAttribute(ctx, p, &v)...)
	return v
}
