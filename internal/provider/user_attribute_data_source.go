package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

var (
	_ datasource.DataSource              = (*userAttributeDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*userAttributeDataSource)(nil)
)

// NewUserAttributeDataSource returns the rauthy_user_attribute data source.
func NewUserAttributeDataSource() datasource.DataSource { return &userAttributeDataSource{} }

type userAttributeDataSource struct {
	api *client.Client
}

type userAttributeDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Desc         types.String `tfsdk:"desc"`
	DefaultValue types.String `tfsdk:"default_value"`
	UserEditable types.Bool   `tfsdk:"user_editable"`
}

func (d *userAttributeDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_user_attribute"
}

func (d *userAttributeDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a custom user attribute definition by name, for referring from a " +
			"`rauthy_scope` mapping to an attribute this configuration does not manage. Reading it through " +
			"the data source turns a name that no longer exists into a plan-time error rather than a " +
			"mapping Rauthy silently drops.\n\nRequires the `UserAttributes:read` API key right.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the attribute, which is its name.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the attribute to look up.",
			},
			"desc": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the attribute.",
			},
			"default_value": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Default value of the attribute, as compacted JSON.",
			},
			"user_editable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether users may change this attribute on themselves.",
			},
		},
	}
}

func (d *userAttributeDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("Expected *client.Client, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	d.api = api
}

func (d *userAttributeDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config userAttributeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()
	got, err := d.api.GetUserAttr(ctx, name)
	if client.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Rauthy user attribute "+name+" not found",
			"No user attribute with that name is defined on the configured Rauthy instance.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not look up Rauthy user attribute "+name, err.Error())
		return
	}

	config.ID = types.StringValue(got.Name)
	config.Name = types.StringValue(got.Name)
	config.Desc = optionalString(got.Desc)
	// The data source reports Rauthy's own spelling of the default; there is no
	// configuration here for it to disagree with, so it stays a plain string.
	config.DefaultValue = defaultValueString(got.DefaultValue).StringValue
	config.UserEditable = types.BoolValue(got.UserEditable)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
