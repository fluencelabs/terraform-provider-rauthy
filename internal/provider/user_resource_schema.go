package provider

import (
	"context"
	"maps"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

const userResourceDescription = "Manages a user account in Rauthy.\n\n" +
	"Creating a user takes two API calls: Rauthy's create endpoint accepts only the email, " +
	"language, roles and groups, so everything else — `enabled`, `email_verified`, the profile " +
	"values and an initial `password` — is written by the update that follows. A user created " +
	"through the API has no password and no passkey until one is set, either by this resource " +
	"or by the account-initialisation email Rauthy sends.\n\n" +
	"`given_name` is required even though Rauthy's own API documentation calls it optional: " +
	"an update that does not carry one is rejected, so a user created without it could never " +
	"be changed again.\n\n" +
	"Every name in `roles` and `groups` must already exist on the instance; reference a " +
	"`rauthy_role` or `rauthy_group` resource to have Terraform order that for you.\n\n" +
	"Requires these API key rights: `Users` read, create, update, delete."

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := make(map[string]schema.Attribute)
	maps.Copy(attributes, userIdentityAttributes())
	maps.Copy(attributes, userAccessAttributes())
	maps.Copy(attributes, userProfileAttributes())

	resp.Schema = schema.Schema{
		MarkdownDescription: userResourceDescription,
		Attributes:          attributes,
	}
}

// userIdentityAttributes covers who the account is: the fields Rauthy already
// accepts on POST /users, plus the id it assigns.
func userIdentityAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Rauthy-assigned identifier of the user.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"email": schema.StringAttribute{
			Required: true,
			MarkdownDescription: "Email address, which is also the login identifier. Changing it " +
				"updates the account in place rather than replacing it.",
		},
		// Required, though Rauthy's own OpenAPI document calls it nullable:
		// PUT /users/{id} answers a null given_name with "'given_name' is
		// required", so a user created without one could be created and then
		// never updated again. Requiring it here turns that dead end into a
		// plan-time error. family_name really is optional — a PUT carrying
		// only given_name is accepted.
		"given_name": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Given name. Required: Rauthy rejects an update that does not carry one.",
			Validators:          []validator.String{validators.PersonName()},
		},
		"family_name": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Family name.",
			Validators:          []validator.String{validators.PersonName()},
		},
		"language": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString("en"),
			MarkdownDescription: "Language used for the account's emails and the login UI.",
			Validators:          []validator.String{validators.Language()},
		},
	}
}

// userAccessAttributes covers what the account may do and how it authenticates.
// None of it can be set at creation time; it all goes out in the PUT.
func userAccessAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"enabled": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
			MarkdownDescription: "Whether the account may log in. Disabling it does not delete anything.",
		},
		"email_verified": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
			MarkdownDescription: "Whether the email address counts as verified. Setting this to `true` " +
				"skips the verification step Rauthy would otherwise require.",
		},
		"roles": schema.SetAttribute{
			Optional:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Roles granted to the user. Each must already exist.",
			Validators:          []validator.Set{validators.RoleNameSet()},
		},
		"groups": schema.SetAttribute{
			Optional:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Groups the user belongs to. Each must already exist.",
			Validators:          []validator.Set{validators.GroupNameSet()},
		},
		"user_expires": schema.Int64Attribute{
			Optional: true,
			MarkdownDescription: "Unix timestamp in seconds at which the account expires. " +
				"Omit for an account that does not expire.",
		},
		"password": schema.StringAttribute{
			Optional:  true,
			Sensitive: true,
			MarkdownDescription: "Password to set on the account. Rauthy never returns it, so this " +
				"is write-only: the provider cannot detect a password changed elsewhere, and " +
				"removing the attribute leaves the last one set rather than clearing it. The value " +
				"is stored in the Terraform state in clear text — prefer leaving it unset and " +
				"letting the user set their own password through Rauthy's reset flow.",
		},
	}
}

// userProfileAttributes covers the optional profile block and the fields Rauthy
// only ever reports.
func userProfileAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"user_values": schema.SingleNestedAttribute{
			Optional: true,
			MarkdownDescription: "Optional profile values. Omit the block entirely to leave whatever " +
				"the user set themselves alone; once present, a field left out is cleared.",
			Attributes: map[string]schema.Attribute{
				"birthdate": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "Birthdate in `YYYY-MM-DD` form.",
					Validators:          []validator.String{validators.Birthdate()},
				},
				"city": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "City.",
					Validators:          []validator.String{validators.AddressPart()},
				},
				"country": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "Country.",
					Validators:          []validator.String{validators.AddressPart()},
				},
				"phone": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "Phone number in `+<digits>` form.",
					Validators:          []validator.String{validators.Phone()},
				},
				"street": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "Street address.",
					Validators:          []validator.String{validators.Street()},
				},
				"zip": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "Postal code.",
					Validators:          []validator.String{validators.Zip()},
				},
				"tz": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "IANA timezone, for example `Europe/Berlin`.",
					Validators:          []validator.String{validators.Timezone()},
				},
			},
		},
		"account_type": schema.StringAttribute{
			Computed: true,
			MarkdownDescription: "How the account authenticates, as Rauthy reports it: `new`, " +
				"`password`, `passkey`, `federated` and so on.",
		},
		"created_at": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Unix timestamp in seconds at which the account was created.",
		},
		"preferred_username": schema.StringAttribute{
			Computed: true,
			MarkdownDescription: "The username the user chose for themselves, if any. Rauthy has a " +
				"dedicated endpoint for this, so it is read-only here.",
		},
	}
}
