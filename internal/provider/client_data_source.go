package provider

import (
	"context"
	"fmt"
	"maps"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

var (
	_ datasource.DataSource              = (*clientDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clientDataSource)(nil)
)

// NewClientDataSource returns the rauthy_client data source.
func NewClientDataSource() datasource.DataSource { return &clientDataSource{} }

type clientDataSource struct {
	api *client.Client
}

type scimDataSourceModel struct {
	BearerToken     types.String `tfsdk:"bearer_token"`
	BaseURI         types.String `tfsdk:"base_uri"`
	SyncGroups      types.Bool   `tfsdk:"sync_groups"`
	GroupSyncPrefix types.String `tfsdk:"group_sync_prefix"`
}

type clientDataSourceModel struct {
	ID                     types.String         `tfsdk:"id"`
	Name                   types.String         `tfsdk:"name"`
	Confidential           types.Bool           `tfsdk:"confidential"`
	Enabled                types.Bool           `tfsdk:"enabled"`
	RedirectURIs           types.Set            `tfsdk:"redirect_uris"`
	PostLogoutRedirectURIs types.Set            `tfsdk:"post_logout_redirect_uris"`
	AllowedOrigins         types.Set            `tfsdk:"allowed_origins"`
	FlowsEnabled           types.Set            `tfsdk:"flows_enabled"`
	AccessTokenAlg         types.String         `tfsdk:"access_token_alg"`
	IDTokenAlg             types.String         `tfsdk:"id_token_alg"`
	AuthCodeLifetime       types.Int64          `tfsdk:"auth_code_lifetime"`
	AccessTokenLifetime    types.Int64          `tfsdk:"access_token_lifetime"`
	Scopes                 types.Set            `tfsdk:"scopes"`
	DefaultScopes          types.Set            `tfsdk:"default_scopes"`
	Challenges             types.Set            `tfsdk:"challenges"`
	ForceMFA               types.Bool           `tfsdk:"force_mfa"`
	ClientURI              types.String         `tfsdk:"client_uri"`
	Contacts               types.Set            `tfsdk:"contacts"`
	BackchannelLogoutURI   types.String         `tfsdk:"backchannel_logout_uri"`
	RestrictGroupPrefix    types.String         `tfsdk:"restrict_group_prefix"`
	Scim                   *scimDataSourceModel `tfsdk:"scim"`
}

func (d *clientDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_client"
}

// clientDataSourceDescription explains, deliberately, why `secret` is absent.
//
// Exposing it here would mean the data source needs `Secrets:read` on top of
// `Clients:read` just to look up a client's public settings, and would put a
// credential for a client this configuration does not own into Terraform
// state and plan output. rauthy_client (the resource) already covers the
// legitimate case — reading the secret of a client Terraform itself created
// and is responsible for. If a genuine need to read the secret of an
// unmanaged client shows up, it should be its own explicit, separately
// documented lookup, not a field bundled into general client attributes.
const clientDataSourceDescription = "Looks up an existing Rauthy OIDC client by `id`, for referring to " +
	"clients this configuration does not manage.\n\nRequires the `Clients:read` API key right.\n\n" +
	"The client secret is intentionally not exposed here. Reading it requires the separate " +
	"`Secrets:read` right and a second API call, and a data source is the wrong place to write a " +
	"credential into state for a client this configuration does not own and is not responsible for " +
	"rotating. Use the `rauthy_client` resource, which does expose `secret`, for a client Terraform " +
	"manages."

func (d *clientDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := make(map[string]schema.Attribute)
	maps.Copy(attributes, clientDataSourceCoreAttributes())
	maps.Copy(attributes, clientDataSourceDescriptiveAttributes())

	resp.Schema = schema.Schema{
		MarkdownDescription: clientDataSourceDescription,
		Attributes:          attributes,
	}
}

// clientDataSourceCoreAttributes covers identity, redirects and the OIDC
// behaviour settings — everything but the informational/access-restriction
// fields.
func clientDataSourceCoreAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The client id to look up, as used in OIDC requests.",
		},
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Human-readable name, shown on the login page.",
		},
		"confidential": schema.BoolAttribute{
			Computed: true,
			MarkdownDescription: "Whether the client authenticates with a secret. A public client " +
				"(`false`) has no secret and Rauthy requires S256 PKCE from it.",
		},
		"enabled": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether the client may be used to log in.",
		},
		"redirect_uris": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Allowed redirect URIs.",
		},
		"post_logout_redirect_uris": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Allowed redirect URIs after a logout.",
		},
		"allowed_origins": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Additional CORS origins, as `scheme://host[:port]` with no path.",
		},
		"flows_enabled": schema.SetAttribute{
			Computed:    true,
			ElementType: types.StringType,
			MarkdownDescription: "Enabled OAuth 2.0 flows: `authorization_code`, `client_credentials`, " +
				"`urn:ietf:params:oauth:grant-type:device_code`, `password`, `refresh_token`.",
		},
		"access_token_alg": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Signing algorithm for access tokens: `RS256`, `RS384`, `RS512` or `EdDSA`.",
		},
		"id_token_alg": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Signing algorithm for id tokens: `RS256`, `RS384`, `RS512` or `EdDSA`.",
		},
		"auth_code_lifetime": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Lifetime of an authorization code in seconds.",
		},
		"access_token_lifetime": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Lifetime of an access token in seconds.",
		},
		"scopes": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Scopes the client may request.",
		},
		"default_scopes": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Scopes granted even when the client does not ask for them.",
		},
		"challenges": schema.SetAttribute{
			Computed:    true,
			ElementType: types.StringType,
			MarkdownDescription: "Accepted PKCE challenge methods: `plain`, `S256`. Rauthy requires " +
				"`S256` from public clients regardless of this setting.",
		},
		"force_mfa": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether MFA is required for users logging in through this client.",
		},
	}
}

// clientDataSourceDescriptiveAttributes covers the informational and
// access-restriction fields: they change nothing about the OIDC exchange
// itself.
func clientDataSourceDescriptiveAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"client_uri": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Informational URI of the application.",
		},
		"contacts": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Contact addresses for the application's maintainers.",
		},
		"backchannel_logout_uri": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "OIDC back-channel logout endpoint of the application.",
		},
		"restrict_group_prefix": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Restricts logins to users in groups starting with this prefix.",
		},
		"scim": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "SCIM provisioning configured against the application.",
			Attributes: map[string]schema.Attribute{
				"bearer_token": schema.StringAttribute{
					Computed:            true,
					Sensitive:           true,
					MarkdownDescription: "Bearer token Rauthy presents to the SCIM endpoint.",
				},
				"base_uri": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Base URI of the SCIM endpoint.",
				},
				"sync_groups": schema.BoolAttribute{
					Computed:            true,
					MarkdownDescription: "Whether groups are synchronised as well as users.",
				},
				"group_sync_prefix": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Only groups starting with this prefix are synchronised.",
				},
			},
		},
	}
}

func (d *clientDataSource) Configure(
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

func (d *clientDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clientDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	got, err := d.api.GetClient(ctx, id)
	if client.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Rauthy client "+id+" not found",
			"No client with that id exists on the configured Rauthy instance.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not look up Rauthy client "+id, err.Error())
		return
	}

	config.ID = types.StringValue(got.ID)
	config.Name = optionalString(got.Name)
	config.Confidential = types.BoolValue(got.Confidential)
	config.Enabled = types.BoolValue(got.Enabled)
	config.RedirectURIs = stringsToSet(got.RedirectURIs)
	config.PostLogoutRedirectURIs = stringsToSet(got.PostLogoutRedirectURIs)
	config.AllowedOrigins = stringsToSet(got.AllowedOrigins)
	config.FlowsEnabled = stringsToSet(got.FlowsEnabled)
	config.AccessTokenAlg = types.StringValue(got.AccessTokenAlg)
	config.IDTokenAlg = types.StringValue(got.IDTokenAlg)
	config.AuthCodeLifetime = types.Int64Value(got.AuthCodeLifetime)
	config.AccessTokenLifetime = types.Int64Value(got.AccessTokenLifetime)
	config.Scopes = stringsToSet(got.Scopes)
	config.DefaultScopes = stringsToSet(got.DefaultScopes)
	config.Challenges = stringsToSet(got.Challenges)
	config.ForceMFA = types.BoolValue(got.ForceMFA)
	config.ClientURI = optionalString(got.ClientURI)
	config.Contacts = stringsToSet(got.Contacts)
	config.BackchannelLogoutURI = optionalString(got.BackchannelLogoutURI)
	config.RestrictGroupPrefix = optionalString(got.RestrictGroupPrefix)

	if got.Scim == nil {
		config.Scim = nil
	} else {
		config.Scim = &scimDataSourceModel{
			BearerToken:     types.StringValue(got.Scim.BearerToken),
			BaseURI:         types.StringValue(got.Scim.BaseURI),
			SyncGroups:      types.BoolValue(got.Scim.SyncGroups),
			GroupSyncPrefix: optionalString(got.Scim.GroupSyncPrefix),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
