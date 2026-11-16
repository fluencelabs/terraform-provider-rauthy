// Package validators adapts Rauthy constraints to Terraform schema validators.
package validators

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	rauthy "github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var (
	// Attribute names in a scope's attr_include_* mapping.
	userAttrName = regexp.MustCompile(`^[a-zA-Z0-9-_/]{2,128}$`)
	// The name of a user attribute *definition*. Same character class as the
	// mapping above but a tighter length bound: Rauthy accepts a 128-character
	// name in a scope's mapping while refusing to define one longer than 32,
	// so an over-long name is rejected at the point where it is created.
	userAttrConfigName = regexp.MustCompile(`^[a-zA-Z0-9-_/]{2,32}$`)
	birthdate          = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	phone              = regexp.MustCompile(`^\+[0-9]{0,32}$`)
	zip                = regexp.MustCompile(`^[a-zA-Z0-9]{0,24}$`)
	// A PAM host's hostname and each of its aliases. This one Rauthy does
	// publish, in the field descriptions of PamHostCreateRequest and
	// PamHostUpdateRequest.
	pamHostname = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-.]*[a-zA-Z0-9]$`)
)

const (
	passwordLengthMin    = 8
	passwordLengthMax    = 128
	passwordCharClassMin = 1
	passwordCharClassMax = 32
	passwordValidDaysMin = 1
	passwordValidDaysMax = 3650
	notRecentlyUsedMin   = 1
	notRecentlyUsedMax   = 10
)

//nolint:gochecknoglobals // Constraints not exported by rauthy-go.
var (
	pamGroupTypes = []string{"immutable", "host", "user", "generic", "local"}
)

// ClientID validates a client id against RE_CLIENT_ID.
func ClientID() validator.String {
	return stringvalidator.RegexMatches(rauthy.ClientIDPattern(), "must match "+rauthy.ClientIDPattern().String())
}

// PathSegment refuses an identifier the API client cannot put into a request
// path: an empty one, "." or "..". RE_CLIENT_ID accepts "..", and Rauthy would
// create such a client, but no request addressed to it can be sent, since a
// normalising hop would turn the path into another endpoint's.
func PathSegment() validator.String {
	return stringvalidator.NoneOf("", ".", "..")
}

// ClientName validates a client name against RE_CLIENT_NAME.
func ClientName() validator.String {
	return stringvalidator.RegexMatches(rauthy.ClientNamePattern(), "must match "+rauthy.ClientNamePattern().String())
}

// URI validates a single URI-ish string against RE_URI.
func URI() validator.String {
	return stringvalidator.RegexMatches(rauthy.URIPattern(), "must match "+rauthy.URIPattern().String())
}

// URISet validates every element of a set against RE_URI.
func URISet() validator.Set {
	return setvalidator.ValueStringsAre(URI())
}

// OriginSet validates every element of a set as an allowed origin.
func OriginSet() validator.Set {
	return setvalidator.ValueStringsAre(
		stringvalidator.RegexMatches(rauthy.OriginPattern(), "must match "+rauthy.OriginPattern().String()),
	)
}

// ContactSet validates every element of a set as a contact.
func ContactSet() validator.Set {
	return setvalidator.ValueStringsAre(
		stringvalidator.RegexMatches(rauthy.ContactPattern(), "must match "+rauthy.ContactPattern().String()),
	)
}

// ScopeSet validates every element of a set as a scope name.
func ScopeSet() validator.Set {
	return setvalidator.ValueStringsAre(
		stringvalidator.RegexMatches(rauthy.ScopePattern(), "must match "+rauthy.ScopePattern().String()),
	)
}

// GroupPrefix validates a group prefix against RE_GROUPS.
func GroupPrefix() validator.String {
	return stringvalidator.RegexMatches(rauthy.GroupPattern(), "must match "+rauthy.GroupPattern().String())
}

// SigningAlg validates a JWK signing algorithm.
func SigningAlg() validator.String {
	return stringvalidator.OneOf(rauthy.SigningAlgs()...)
}

// FlowsEnabledSet validates the OAuth flows. Rauthy rejects an empty list, so
// the size bound is part of the contract rather than a nicety.
func FlowsEnabledSet() []validator.Set {
	return []validator.Set{
		setvalidator.SizeAtLeast(1),
		setvalidator.ValueStringsAre(stringvalidator.OneOf(rauthy.GrantTypes()...)),
	}
}

// ChallengeSet validates the PKCE challenge methods. As with flows, an empty
// list is rejected by Rauthy when the field is present at all.
func ChallengeSet() []validator.Set {
	return []validator.Set{
		setvalidator.SizeAtLeast(1),
		setvalidator.ValueStringsAre(stringvalidator.OneOf(rauthy.ChallengeMethods()...)),
	}
}

// AuthCodeLifetime bounds auth_code_lifetime (seconds).
func AuthCodeLifetime() validator.Int64 {
	return int64validator.Between(rauthy.AuthCodeLifetimeMin, rauthy.AuthCodeLifetimeMax)
}

// AccessTokenLifetime bounds access_token_lifetime (seconds).
func AccessTokenLifetime() validator.Int64 {
	return int64validator.Between(rauthy.AccessTokenLifetimeMin, rauthy.AccessTokenLifetimeMax)
}

// CacheCurrentHours bounds cache_current_hours on a secret rotation.
func CacheCurrentHours() validator.Int64 {
	return int64validator.Between(rauthy.CacheCurrentHoursMin, rauthy.CacheCurrentHoursMax)
}

// RoleName validates a role name against RE_ROLES_SCOPES, the same pattern
// Rauthy applies to scopes.
func RoleName() validator.String {
	return stringvalidator.RegexMatches(rauthy.ScopePattern(), "must match "+rauthy.ScopePattern().String())
}

// GroupName validates a group name against RE_GROUPS.
func GroupName() validator.String {
	return stringvalidator.RegexMatches(rauthy.GroupPattern(), "must match "+rauthy.GroupPattern().String())
}

// PasswordLength validates length_min and length_max.
func PasswordLength() validator.Int64 {
	return int64validator.Between(passwordLengthMin, passwordLengthMax)
}

// PasswordCharClassCount validates the include_* counts.
func PasswordCharClassCount() validator.Int64 {
	return int64validator.Between(passwordCharClassMin, passwordCharClassMax)
}

// PasswordValidDays validates valid_days.
func PasswordValidDays() validator.Int64 {
	return int64validator.Between(passwordValidDaysMin, passwordValidDaysMax)
}

// PasswordNotRecentlyUsed validates not_recently_used.
func PasswordNotRecentlyUsed() validator.Int64 {
	return int64validator.Between(notRecentlyUsedMin, notRecentlyUsedMax)
}

// ScopeName validates a scope name against RE_ROLES_SCOPES.
func ScopeName() validator.String {
	return stringvalidator.RegexMatches(rauthy.ScopePattern(), "must match "+rauthy.ScopePattern().String())
}

// UserAttrNameSet validates every element of a set as a user-attribute name,
// the form accepted in a scope's attr_include_access and attr_include_id.
func UserAttrNameSet() validator.Set {
	return setvalidator.ValueStringsAre(stringvalidator.RegexMatches(userAttrName,
		`must match ^[a-zA-Z0-9-_/]{2,128}$`))
}

// UserAttrConfigName validates the name of a user attribute definition.
func UserAttrConfigName() validator.String {
	return stringvalidator.RegexMatches(userAttrConfigName,
		`must match ^[a-zA-Z0-9-_/]{2,32}$`)
}

// UserAttrDesc validates the description of a user attribute definition.
func UserAttrDesc() validator.String {
	return stringvalidator.RegexMatches(
		rauthy.AttributeDescriptionPattern(),
		"must match "+rauthy.AttributeDescriptionPattern().String(),
	)
}

// Language validates a UI/email language against Rauthy's Language enum.
func Language() validator.String {
	return stringvalidator.OneOf(rauthy.Languages()...)
}

// PersonName validates a given or family name.
func PersonName() validator.String {
	return stringvalidator.RegexMatches(rauthy.UserNamePattern(), "must match "+rauthy.UserNamePattern().String())
}

// RoleNameSet validates every element of a set as a role name.
func RoleNameSet() validator.Set {
	return setvalidator.ValueStringsAre(RoleName())
}

// GroupNameSet validates every element of a set as a group name.
func GroupNameSet() validator.Set {
	return setvalidator.ValueStringsAre(GroupName())
}

// Birthdate validates a user's birthdate, which Rauthy stores as YYYY-MM-DD.
func Birthdate() validator.String {
	return stringvalidator.RegexMatches(birthdate, "must be a date in the form YYYY-MM-DD")
}

// Phone validates a user's phone number.
func Phone() validator.String {
	return stringvalidator.RegexMatches(phone, "must be in the form +<digits>, at most 32 digits")
}

// Zip validates a user's postal code.
func Zip() validator.String {
	return stringvalidator.RegexMatches(zip, "must be at most 24 letters or digits")
}

// Street validates a user's street.
func Street() validator.String {
	return stringvalidator.RegexMatches(rauthy.StreetPattern(), "must match "+rauthy.StreetPattern().String())
}

// AddressPart validates a user's city or country.
func AddressPart() validator.String {
	return stringvalidator.RegexMatches(rauthy.CityPattern(), "must match "+rauthy.CityPattern().String())
}

// Timezone validates a user's timezone. Rauthy only checks that the value
// parses as an IANA name, so this is a shape check rather than a lookup.
func Timezone() validator.String {
	return stringvalidator.RegexMatches(rauthy.TimezonePattern(), "must match "+rauthy.TimezonePattern().String())
}

// Bound from #[validate(length(max = 256))] on ProviderRequest::client_secret.
const authProviderSecretMax = 256

//nolint:gochecknoglobals // Auth provider types are not exported by rauthy-go.
var authProviderTypes = []string{"custom", "github", "google", "oidc"}

// AuthProviderName validates an upstream provider's display name.
func AuthProviderName() validator.String {
	return stringvalidator.RegexMatches(rauthy.ClientNamePattern(), "must match "+rauthy.ClientNamePattern().String())
}

// AuthProviderType validates an upstream provider's type.
func AuthProviderType() validator.String {
	return stringvalidator.OneOf(authProviderTypes...)
}

// AuthProviderURI validates one of an upstream provider's endpoints or claim
// paths.
func AuthProviderURI() validator.String {
	return stringvalidator.RegexMatches(rauthy.URIPattern(), "must match "+rauthy.URIPattern().String())
}

// AuthProviderSecret bounds an upstream client secret's length.
func AuthProviderSecret() validator.String {
	return stringvalidator.LengthAtMost(authProviderSecretMax)
}

// AuthProviderScopeSet validates the joined upstream scopes and their set representation.
func AuthProviderScopeSet() validator.Set {
	return authProviderScopesValidator{}
}

type authProviderScopesValidator struct{}

func (authProviderScopesValidator) Description(context.Context) string {
	return "must contain individual nonempty scopes whose space-joined value matches " + rauthy.AuthProviderScopePattern().
		String()
}

func (v authProviderScopesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v authProviderScopesValidator) ValidateSet(
	ctx context.Context,
	req validator.SetRequest,
	resp *validator.SetResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	values := make([]string, 0, len(req.ConfigValue.Elements()))
	for _, element := range req.ConfigValue.Elements() {
		value, ok := element.(types.String)
		if !ok || value.IsUnknown() {
			return
		}
		text := value.ValueString()
		if text == "" || strings.ContainsFunc(text, unicode.IsSpace) {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid scope",
				"Each set element must be one nonempty scope without whitespace.",
			)
			return
		}
		values = append(values, text)
	}
	if !rauthy.AuthProviderScopePattern().MatchString(strings.Join(values, " ")) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid scopes", v.Description(ctx))
	}
}

// PamGroupType validates a PAM group's `typ` against Rauthy's PamGroupType
// enum.
func PamGroupType() validator.String {
	return stringvalidator.OneOf(pamGroupTypes...)
}

// PamHostname validates a PAM host's hostname. The bound is implicit in the
// pattern rather than stated: the two anchored character classes make two
// characters the shortest legal hostname, which a live 0.36.2 confirms by
// rejecting a one-character alias.
func PamHostname() validator.String {
	return stringvalidator.RegexMatches(pamHostname,
		`must match ^[a-zA-Z0-9][a-zA-Z0-9-.]*[a-zA-Z0-9]$ (at least two characters)`)
}

// PamHostAliasSet validates every alias of a PAM host. Aliases carry the same
// pattern as the hostname itself.
func PamHostAliasSet() validator.Set {
	return setvalidator.ValueStringsAre(PamHostname())
}

// PamHostIPSet validates every element of a PAM host's address list as an IP
// address. Rauthy parses these into a Rust `IpAddr` before its own validation
// runs, so a malformed entry comes back as a bare "Deserialization error" with
// a 422 and no indication of which field was at fault; checking here turns that
// into a plan-time error naming the attribute.
func PamHostIPSet() validator.Set {
	return setvalidator.ValueStringsAre(ipAddressValidator{})
}

// Constraints on an API key, from src/api_types/src/api_keys.rs.
//
// The key's name. Note the `/` inside the character class: it is part of the
// upstream pattern as written (`^[a-zA-Z0-9_-/]{2,24}$`), which Rust's regex
// crate reads as a literal range end rather than as an escape.
var apiKeyName = regexp.MustCompile(`^[a-zA-Z0-9_\-/]{2,24}$`)

//nolint:gochecknoglobals // Constraints not exported by rauthy-go.
var (
	accessGroups = []string{
		"Blacklist", "Clients", "Events", "Generic", "Groups", "Roles", "Secrets",
		"Sessions", "Scopes", "UserAttributes", "Users", "Pam", "AuthProviders", "ApiKeys",
	}

	accessRights = []string{"read", "create", "update", "delete"}
)

// APIKeyName validates an API key's name.
func APIKeyName() validator.String {
	return stringvalidator.RegexMatches(apiKeyName,
		`must match ^[a-zA-Z0-9_\-/]{2,24}$`)
}

// AccessGroup validates one access group of an API key.
func AccessGroup() validator.String {
	return stringvalidator.OneOf(accessGroups...)
}

// AccessRightsSet validates every element of an access-rights set.
func AccessRightsSet() validator.Set {
	return setvalidator.ValueStringsAre(stringvalidator.OneOf(accessRights...))
}

// blacklistExpMin is the lower bound Rauthy puts on a blacklist entry's `exp`.
// It is a fixed timestamp (2024-07-01T00:00:00Z) rather than "now", read off a
// live 0.36.2 rejection: `"exp": range, params: {"min": 1719784800}`. It rules
// out obviously nonsensical values while still accepting a timestamp that is
// merely in the recent past — which the server then silently discards. See the
// note on Client.BlacklistIP.
const blacklistExpMin = 1719784800

// BlacklistExp bounds a blacklist entry's expiry. It deliberately does not
// check that the value lies in the future: a bound derived from time.Now()
// would make a saved plan expire between plan and apply.
func BlacklistExp() validator.Int64 {
	return int64validator.AtLeast(blacklistExpMin)
}

// ipAddressValidator checks that a string parses as an IPv4 or IPv6 address.
// Rauthy deserialises the field into a Rust IpAddr and answers a bare 400 with
// a serde message when it does not parse; validating here names the attribute
// instead.
type ipAddressValidator struct{}

func (ipAddressValidator) Description(context.Context) string {
	return "must be an IPv4 or IPv6 address"
}

func (v ipAddressValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v ipAddressValidator) ValidateString(
	ctx context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !rauthy.ValidIPAddress(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid IP address",
			"Attribute "+req.Path.String()+" "+v.Description(ctx)+
				", got: "+req.ConfigValue.ValueString(),
		)
	}
}

// IPAddress validates a bare IPv4 or IPv6 address, with no port and no prefix
// length.
func IPAddress() validator.String {
	return ipAddressValidator{}
}
