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
	_ datasource.DataSource              = (*groupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*groupDataSource)(nil)
)

// NewGroupDataSource returns the rauthy_group data source.
func NewGroupDataSource() datasource.DataSource { return &groupDataSource{} }

type groupDataSource struct {
	api *client.Client
}

type groupDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (d *groupDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *groupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an existing Rauthy group by name, for referring to groups this " +
			"configuration does not manage.\n\nRequires the `Groups:read` API key right.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rauthy-assigned identifier of the group.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the group to look up.",
			},
		},
	}
}

func (d *groupDataSource) Configure(
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

func (d *groupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config groupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()
	got, err := d.api.GetGroupByName(ctx, name)
	if client.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Rauthy group "+name+" not found",
			"No group with that name exists on the configured Rauthy instance.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not look up Rauthy group "+name, err.Error())
		return
	}

	config.ID = types.StringValue(got.ID)
	config.Name = types.StringValue(got.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
