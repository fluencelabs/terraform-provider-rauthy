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
	_ datasource.DataSource              = (*roleDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*roleDataSource)(nil)
)

// NewRoleDataSource returns the rauthy_role data source.
func NewRoleDataSource() datasource.DataSource { return &roleDataSource{} }

type roleDataSource struct {
	api *client.Client
}

type roleDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (d *roleDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (d *roleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an existing Rauthy role by name, for referring to roles this " +
			"configuration does not manage.\n\nRequires the `Roles:read` API key right.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rauthy-assigned identifier of the role.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the role to look up.",
			},
		},
	}
}

func (d *roleDataSource) Configure(
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

func (d *roleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config roleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()
	got, err := d.api.GetRoleByName(ctx, name)
	if client.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Rauthy role "+name+" not found",
			"No role with that name exists on the configured Rauthy instance.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not look up Rauthy role "+name, err.Error())
		return
	}

	config.ID = types.StringValue(got.ID)
	config.Name = types.StringValue(got.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
