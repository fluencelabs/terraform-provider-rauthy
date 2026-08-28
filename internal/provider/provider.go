package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// Environment variables used as fallbacks for the provider configuration.
const (
	EnvURL    = "RAUTHY_URL"
	EnvAPIKey = "RAUTHY_API_KEY"
)

var _ provider.Provider = (*rauthyProvider)(nil)

type rauthyProvider struct {
	version string
}

// New returns the provider constructor used by main and by the test harness.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &rauthyProvider{version: version}
	}
}

type providerModel struct {
	URL    types.String `tfsdk:"url"`
	APIKey types.String `tfsdk:"api_key"`
}

func (p *rauthyProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "rauthy"
	resp.Version = p.version
}

func (p *rauthyProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages OIDC clients in a [Rauthy](https://github.com/sebadob/rauthy) identity provider " +
			"through its admin API.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Base URL of the Rauthy instance, for example `https://auth.example.com`. " +
					"The `/auth/v1` API path is appended by the provider. " +
					"Falls back to the `" + EnvURL + "` environment variable.",
			},
			"api_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "Rauthy API key in `<name>$<secret>` form. Falls back to the `" + EnvAPIKey +
					"` environment variable.\n\n" +
					"The key needs these access rights: `Clients` read, create, update, delete, and " +
					"`Secrets` read and update. `Secrets:read` is used on every refresh of a confidential " +
					"client, `Secrets:update` only when rotating a secret.",
			},
		},
	}
}

func (p *rauthyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if cfg.URL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("url"),
			"Unknown Rauthy URL",
			"The Rauthy URL is not known at plan time. Set it to a static value or via the "+EnvURL+" environment variable.",
		)
	}
	if cfg.APIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Unknown Rauthy API key",
			"The Rauthy API key is not known at plan time. Set it to a static value or via the "+EnvAPIKey+" environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	url := os.Getenv(EnvURL)
	if !cfg.URL.IsNull() {
		url = cfg.URL.ValueString()
	}
	apiKey := os.Getenv(EnvAPIKey)
	if !cfg.APIKey.IsNull() {
		apiKey = cfg.APIKey.ValueString()
	}

	if url == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("url"),
			"Missing Rauthy URL",
			"Set the `url` provider attribute or the "+EnvURL+" environment variable.",
		)
	}
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing Rauthy API key",
			"Set the `api_key` provider attribute or the "+EnvAPIKey+" environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c, err := client.New(url, apiKey)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Rauthy provider configuration", err.Error())
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *rauthyProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

func (p *rauthyProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}
