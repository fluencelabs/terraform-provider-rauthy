package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

var (
	_ datasource.DataSource              = (*authProviderLookupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*authProviderLookupDataSource)(nil)
)

// NewAuthProviderLookupDataSource returns the rauthy_auth_provider_lookup data
// source.
func NewAuthProviderLookupDataSource() datasource.DataSource { return &authProviderLookupDataSource{} }

type authProviderLookupDataSource struct {
	api *client.Client
}

type authProviderLookupDataSourceModel struct {
	Issuer                types.String `tfsdk:"issuer"`
	MetadataURL           types.String `tfsdk:"metadata_url"`
	ResolvedIssuer        types.String `tfsdk:"resolved_issuer"`
	AuthorizationEndpoint types.String `tfsdk:"authorization_endpoint"`
	TokenEndpoint         types.String `tfsdk:"token_endpoint"`
	UserinfoEndpoint      types.String `tfsdk:"userinfo_endpoint"`
	JwksEndpoint          types.String `tfsdk:"jwks_endpoint"`
	Scopes                types.Set    `tfsdk:"scopes"`
	UsePKCE               types.Bool   `tfsdk:"use_pkce"`
	ClientSecretBasic     types.Bool   `tfsdk:"client_secret_basic"`
	ClientSecretPost      types.Bool   `tfsdk:"client_secret_post"`
}

func (d *authProviderLookupDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_auth_provider_lookup"
}

func (d *authProviderLookupDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Discovers an upstream identity provider's OIDC configuration, so a " +
			"`rauthy_auth_provider` can be written without transcribing four endpoints by hand.\n\n" +
			"The discovery request is made **by the Rauthy server**, not by Terraform. It fails if " +
			"Rauthy cannot reach the issuer even when the machine running `terraform` can, which " +
			"makes it a poor fit for an air-gapped instance — write the endpoints out literally " +
			"there instead.\n\n" +
			"Requires the `AuthProviders:read` API key right.",
		Attributes: map[string]schema.Attribute{
			"issuer": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Issuer to discover, for example `accounts.google.com`. Rauthy " +
					"appends the well-known discovery path itself. Exactly one of `issuer` and " +
					"`metadata_url` must be set.",
				Validators: []validator.String{
					validators.AuthProviderURI(),
					stringvalidator.ExactlyOneOf(path.MatchRoot("metadata_url")),
				},
			},
			"metadata_url": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Full URL of the discovery document, for upstreams that do not " +
					"serve it at the well-known path.",
				Validators: []validator.String{validators.AuthProviderURI()},
			},
			"resolved_issuer": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The issuer as the discovery document states it. This is not " +
					"necessarily the `issuer` that was asked for: a lookup of " +
					"`accounts.google.com` resolves to `https://accounts.google.com`.",
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
				MarkdownDescription: "The upstream's JWKS endpoint, if the document advertises one.",
			},
			"scopes": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Scopes the upstream advertises support for.",
			},
			"use_pkce": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the upstream advertises support for PKCE with S256.",
			},
			"client_secret_basic": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the upstream accepts client credentials as HTTP Basic.",
			},
			"client_secret_post": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the upstream accepts client credentials in the request body.",
			},
		},
	}
}

func (d *authProviderLookupDataSource) Configure(
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

func (d *authProviderLookupDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config authProviderLookupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.api.LookupAuthProvider(ctx, client.AuthProviderLookupRequest{
		Issuer:      stringPtr(config.Issuer),
		MetadataURL: stringPtr(config.MetadataURL),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Could not discover the upstream provider's configuration",
			"Rauthy itself fetches the discovery document, so this fails when the Rauthy instance "+
				"cannot reach the issuer.\n\nError: "+err.Error(),
		)
		return
	}

	config.ResolvedIssuer = types.StringValue(got.Issuer)
	config.AuthorizationEndpoint = types.StringValue(got.AuthorizationEndpoint)
	config.TokenEndpoint = types.StringValue(got.TokenEndpoint)
	config.UserinfoEndpoint = types.StringValue(got.UserinfoEndpoint)
	config.JwksEndpoint = optionalString(got.JwksEndpoint)
	// This endpoint space-separates the scopes and leaves a trailing space,
	// where the read of a stored provider `+`-joins them. splitAuthProviderScope
	// copes with both.
	config.Scopes = stringsToSet(splitAuthProviderScope(got.Scope))
	config.UsePKCE = types.BoolValue(got.UsePKCE)
	config.ClientSecretBasic = types.BoolValue(got.ClientSecretBasic)
	config.ClientSecretPost = types.BoolValue(got.ClientSecretPost)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
