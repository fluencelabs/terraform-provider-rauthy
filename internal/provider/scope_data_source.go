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
	_ datasource.DataSource              = (*scopeDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*scopeDataSource)(nil)
)

// NewScopeDataSource returns the rauthy_scope data source.
func NewScopeDataSource() datasource.DataSource { return &scopeDataSource{} }

type scopeDataSource struct {
	api *client.Client
}

type scopeDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	AttrIncludeAccess types.Set    `tfsdk:"attr_include_access"`
	AttrIncludeID     types.Set    `tfsdk:"attr_include_id"`
}

func (d *scopeDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_scope"
}

func (d *scopeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an existing Rauthy scope by name, for referring to scopes this " +
			"configuration does not manage.\n\nRequires the `Scopes:read` API key right.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rauthy-assigned identifier of the scope.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the scope to look up.",
			},
			"attr_include_access": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "User attributes this scope adds to the access token.",
			},
			"attr_include_id": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "User attributes this scope adds to the ID token.",
			},
		},
	}
}

func (d *scopeDataSource) Configure(
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

func (d *scopeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config scopeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()
	got, err := d.api.GetScopeByName(ctx, name)
	if client.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Rauthy scope "+name+" not found",
			"No scope with that name exists on the configured Rauthy instance.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not look up Rauthy scope "+name, err.Error())
		return
	}

	config.ID = types.StringValue(got.ID)
	config.Name = types.StringValue(got.Name)
	config.AttrIncludeAccess = stringsToSet(got.AttrIncludeAccess)
	config.AttrIncludeID = stringsToSet(got.AttrIncludeID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
