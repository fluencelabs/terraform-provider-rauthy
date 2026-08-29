package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

var (
	_ datasource.DataSource              = (*userDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*userDataSource)(nil)
)

// NewUserDataSource returns the rauthy_user data source.
func NewUserDataSource() datasource.DataSource { return &userDataSource{} }

type userDataSource struct {
	api *client.Client
}

type userDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Email             types.String `tfsdk:"email"`
	GivenName         types.String `tfsdk:"given_name"`
	FamilyName        types.String `tfsdk:"family_name"`
	Language          types.String `tfsdk:"language"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	EmailVerified     types.Bool   `tfsdk:"email_verified"`
	Roles             types.Set    `tfsdk:"roles"`
	Groups            types.Set    `tfsdk:"groups"`
	UserExpires       types.Int64  `tfsdk:"user_expires"`
	AccountType       types.String `tfsdk:"account_type"`
	CreatedAt         types.Int64  `tfsdk:"created_at"`
	PreferredUsername types.String `tfsdk:"preferred_username"`
}

func (d *userDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an existing Rauthy user by `id` or by `email`, for referring to " +
			"accounts this configuration does not manage. Exactly one of the two must be set.\n\n" +
			"The profile values a user maintains themselves are not exposed here; only the fields an " +
			"administrator manages are.\n\nRequires the `Users:read` API key right.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Rauthy-assigned identifier of the user. Set this or `email`.",
			},
			"email": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Email address of the user. Set this or `id`.",
			},
			"given_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Given name.",
			},
			"family_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Family name.",
			},
			"language": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Language used for the account's emails and the login UI.",
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the account may log in.",
			},
			"email_verified": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the email address counts as verified.",
			},
			"roles": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Roles granted to the user.",
			},
			"groups": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Groups the user belongs to.",
			},
			"user_expires": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Unix timestamp in seconds at which the account expires, if it does.",
			},
			"account_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "How the account authenticates, as Rauthy reports it.",
			},
			"created_at": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Unix timestamp in seconds at which the account was created.",
			},
			"preferred_username": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The username the user chose for themselves, if any.",
			},
		},
	}
}

func (d *userDataSource) Configure(
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

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config userDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	email := config.Email.ValueString()
	switch {
	case id == "" && email == "":
		resp.Diagnostics.AddError(
			"Missing Rauthy user selector",
			"Set either `id` or `email` to select the user to look up.",
		)
		return
	case id != "" && email != "":
		resp.Diagnostics.AddAttributeError(
			path.Root("email"),
			"Conflicting Rauthy user selectors",
			"Set either `id` or `email`, not both.",
		)
		return
	}

	var (
		got   *client.UserResponse
		err   error
		label string
	)
	if id != "" {
		label = "user " + id
		got, err = d.api.GetUser(ctx, id)
	} else {
		label = "user " + email
		got, err = d.api.GetUserByEmail(ctx, email)
	}
	if client.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Rauthy "+label+" not found",
			"No such user exists on the configured Rauthy instance.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not look up Rauthy "+label, err.Error())
		return
	}

	config.ID = types.StringValue(got.ID)
	config.Email = types.StringValue(got.Email)
	config.GivenName = types.StringPointerValue(got.GivenName)
	config.FamilyName = types.StringPointerValue(got.FamilyName)
	config.Language = types.StringValue(got.Language)
	config.Enabled = types.BoolValue(got.Enabled)
	config.EmailVerified = types.BoolValue(got.EmailVerified)
	config.Roles = stringsToSet(got.Roles)
	config.Groups = stringsToSet(got.Groups)
	config.UserExpires = types.Int64PointerValue(got.UserExpires)
	config.AccountType = types.StringValue(got.AccountType)
	config.CreatedAt = types.Int64Value(got.CreatedAt)
	config.PreferredUsername = types.StringPointerValue(got.UserValues.PreferredUsername)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
