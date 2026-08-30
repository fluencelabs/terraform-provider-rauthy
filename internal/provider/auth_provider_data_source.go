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
	_ datasource.DataSource              = (*authProviderDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*authProviderDataSource)(nil)
)

// NewAuthProviderDataSource returns the rauthy_auth_provider data source.
func NewAuthProviderDataSource() datasource.DataSource { return &authProviderDataSource{} }

type authProviderDataSource struct {
	api *client.Client
}

type authProviderDataSourceModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Type                  types.String `tfsdk:"type"`
	Enabled               types.Bool   `tfsdk:"enabled"`
	Issuer                types.String `tfsdk:"issuer"`
	AuthorizationEndpoint types.String `tfsdk:"authorization_endpoint"`
	TokenEndpoint         types.String `tfsdk:"token_endpoint"`
	UserinfoEndpoint      types.String `tfsdk:"userinfo_endpoint"`
	JwksEndpoint          types.String `tfsdk:"jwks_endpoint"`
	ClientID              types.String `tfsdk:"client_id"`
	Scopes                types.Set    `tfsdk:"scopes"`
}

func (d *authProviderDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_auth_provider"
}

func (d *authProviderDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an existing upstream authentication provider by name, for " +
			"referring to a provider this configuration does not manage.\n\n" +
			"Rauthy does not enforce unique provider names, so a name shared by two providers is " +
			"an error here rather than an arbitrary pick.\n\n" +
			"The upstream `client_secret` is deliberately not exposed. Rauthy does return it in " +
			"the clear on a read, but a lookup of somebody else's provider has no use for it and " +
			"writing it into this state file would only spread it further. `rauthy_auth_provider` " +
			"now drops it for the same reason; see its `client_secret_wo`.\n\n" +
			"Requires the `AuthProviders:read` API key right.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name of the provider to look up.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rauthy-assigned identifier of the provider.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Provider flavour: `custom`, `github`, `google` or `oidc`.",
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the provider is offered as a login option.",
			},
			"issuer": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The upstream's issuer.",
			},
			"authorization_endpoint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The upstream's authorization endpoint.",
			},
			"token_endpoint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The upstream's token endpoint.",
			},
			"userinfo_endpoint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The upstream's userinfo endpoint.",
			},
			"jwks_endpoint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The upstream's JWKS endpoint, if it has one.",
			},
			"client_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The client id Rauthy was issued by the upstream.",
			},
			"scopes": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Scopes Rauthy requests from the upstream.",
			},
		},
	}
}

func (d *authProviderDataSource) Configure(
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

func (d *authProviderDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config authProviderDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()
	got, err := d.api.GetAuthProviderByName(ctx, name)
	if client.IsNotFound(err) {
		// GetAuthProviderByName reports an ambiguous name as a 404 too, so the
		// underlying message is what distinguishes the two cases.
		resp.Diagnostics.AddError("Could not resolve Rauthy auth provider "+name, err.Error())
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not look up Rauthy auth provider "+name, err.Error())
		return
	}

	config.ID = types.StringValue(got.ID)
	config.Name = types.StringValue(got.Name)
	config.Type = types.StringValue(got.Typ)
	config.Enabled = types.BoolValue(got.Enabled)
	config.Issuer = types.StringValue(got.Issuer)
	config.AuthorizationEndpoint = types.StringValue(got.AuthorizationEndpoint)
	config.TokenEndpoint = types.StringValue(got.TokenEndpoint)
	config.UserinfoEndpoint = types.StringValue(got.UserinfoEndpoint)
	config.JwksEndpoint = optionalString(got.JwksEndpoint)
	config.ClientID = types.StringValue(got.ClientID)
	config.Scopes = stringsToSet(splitAuthProviderScope(got.Scope))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
