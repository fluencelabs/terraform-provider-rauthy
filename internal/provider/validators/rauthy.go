// Package validators holds the value constraints Rauthy enforces server-side,
// restated as Terraform schema validators.
//
// They are restated rather than left to the server for two reasons: a plan-time
// error names the offending attribute, and Rauthy's own OpenAPI document does
// not carry them. Rauthy generates that document with utoipa, which does not
// emit the `validator` crate's ranges and regexes into the schema — they survive
// only as prose in the field descriptions. So the contract tests can police
// field sets and types but not values, and these validators are the only
// mechanical guard we have.
//
// Every pattern and bound below is transcribed from Rauthy v0.35.2:
// src/common/src/regex.rs, src/api_types/src/cust_validation.rs and the
// #[validate] attributes in src/api_types/src/clients.rs.
package validators

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// Patterns from src/common/src/regex.rs.
var (
	// RE_CLIENT_ID.
	clientID = regexp.MustCompile(`^[a-zA-Z0-9,.:/_\-&?=~#!$'()*+%]{2,256}$`)

	// RE_CLIENT_NAME. The CJK ranges are part of the upstream pattern.
	clientName = regexp.MustCompile(`^[a-zA-Z0-9À-ɏ\-\s` +
		`\x{3041}-\x{3096}\x{30A0}-\x{30FF}\x{3400}-\x{4DB5}\x{4E00}-\x{9FCB}` +
		`\x{F900}-\x{FA6A}\x{2E80}-\x{2FD5}\x{FF66}-\x{FF9F}\x{FFA1}-\x{FFDC}\x{31F0}-\x{31FF}]{2,128}$`)

	// RE_URI.
	uri = regexp.MustCompile(`^[a-zA-Z0-9,.:/_\-&?=~#!$'()*+%@]+$`)

	// RE_ROLES_SCOPES, used for both scopes and default_scopes.
	scope = regexp.MustCompile(`^[a-zA-Z0-9-_/,:*.]{2,64}$`)

	// RE_GROUPS.
	groups = regexp.MustCompile(`^[a-zA-Z0-9-_/,:*\s]{2,64}$`)

	// validate_vec_origin.
	origin = regexp.MustCompile(`^(http|https)://[a-z0-9.:-]+$`)

	// validate_vec_contact.
	contact = regexp.MustCompile(`^[a-zA-Z0-9+.@/-]{0,48}$`)

	// Attribute names in a scope's attr_include_* mapping.
	userAttrName = regexp.MustCompile(`^[a-zA-Z0-9-_/]{2,128}$`)

	// The name of a user attribute *definition*. Same character class as the
	// mapping above but a tighter length bound: Rauthy accepts a 128-character
	// name in a scope's mapping while refusing to define one longer than 32,
	// so an over-long name is rejected at the point where it is created.
	userAttrConfigName = regexp.MustCompile(`^[a-zA-Z0-9-_/]{2,32}$`)

	// The description of a user attribute definition. Note the character
	// class: Rauthy does not allow spaces in it, so this is not free text.
	userAttrDesc = regexp.MustCompile(`^[a-zA-Z0-9-_/]{0,128}$`)

	// A user's given_name and family_name.
	personName = regexp.MustCompile(`^[a-zA-Z0-9À-ÿ\-'\s]{1,32}$`)

	// The user_values fields, from validate_* in cust_validation.rs.
	birthdate   = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	phone       = regexp.MustCompile(`^\+[0-9]{0,32}$`)
	zip         = regexp.MustCompile(`^[a-zA-Z0-9]{0,24}$`)
	street      = regexp.MustCompile(`^[a-zA-Z0-9À-ÿ\-.\s]{0,48}$`)
	addressPart = regexp.MustCompile(`^[a-zA-Z0-9À-ÿ\-\s]{0,48}$`)
	timezone    = regexp.MustCompile(`^[A-Za-z]+(?:/[A-Za-z0-9_+-]+)*$`)
)

// Bounds from the #[validate(range(...))] attributes in
// src/api_types/src/clients.rs.
const (
	authCodeLifetimeMin    = 10
	authCodeLifetimeMax    = 300
	accessTokenLifetimeMin = 10
	accessTokenLifetimeMax = 86400
	cacheCurrentHoursMin   = 1
	cacheCurrentHoursMax   = 24
)

// Bounds from the #[validate(range(...))] attributes in
// src/api_types/src/generic.rs (PasswordPolicyRequest).
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

// Enumerations from src/api_types/src/cust_validation.rs.
//
//nolint:gochecknoglobals // transcribed upstream enumerations, read-only
var (
	// JwkKeyPairAlg.
	signingAlgs = []string{"RS256", "RS384", "RS512", "EdDSA"}

	// validate_vec_grant_types.
	grantTypes = []string{
		"authorization_code",
		"client_credentials",
		"urn:ietf:params:oauth:grant-type:device_code",
		"password",
		"refresh_token",
	}

	// RE_CODE_CHALLENGE_METHOD.
	challengeMethods = []string{"plain", "S256"}

	// Language.
	languages = []string{"de", "en", "fr", "ko", "nb", "ru", "uk", "zhhans"}
)

// ClientID validates a client id against RE_CLIENT_ID.
func ClientID() validator.String {
	return stringvalidator.RegexMatches(clientID,
		`must match ^[a-zA-Z0-9,.:/_\-&?=~#!$'()*+%]{2,256}$`)
}

// ClientName validates a client name against RE_CLIENT_NAME.
func ClientName() validator.String {
	return stringvalidator.RegexMatches(clientName,
		"must be 2-128 characters of letters, digits, spaces or '-'")
}

// URI validates a single URI-ish string against RE_URI.
func URI() validator.String {
	return stringvalidator.RegexMatches(uri,
		`must match ^[a-zA-Z0-9,.:/_\-&?=~#!$'()*+%@]+$`)
}

// URISet validates every element of a set against RE_URI.
func URISet() validator.Set {
	return setvalidator.ValueStringsAre(URI())
}

// OriginSet validates every element of a set as an allowed origin.
func OriginSet() validator.Set {
	return setvalidator.ValueStringsAre(stringvalidator.RegexMatches(origin,
		"must match ^(http|https)://[a-z0-9.:-]+$ (scheme and host only, no path)"))
}

// ContactSet validates every element of a set as a contact.
func ContactSet() validator.Set {
	return setvalidator.ValueStringsAre(stringvalidator.RegexMatches(contact,
		"must be at most 48 characters of letters, digits or '+.@/-'"))
}

// ScopeSet validates every element of a set as a scope name.
func ScopeSet() validator.Set {
	return setvalidator.ValueStringsAre(stringvalidator.RegexMatches(scope,
		`must match ^[a-zA-Z0-9-_/,:*.]{2,64}$`))
}

// GroupPrefix validates a group prefix against RE_GROUPS.
func GroupPrefix() validator.String {
	return stringvalidator.RegexMatches(groups,
		`must match ^[a-zA-Z0-9-_/,:*\s]{2,64}$`)
}

// SigningAlg validates a JWK signing algorithm.
func SigningAlg() validator.String {
	return stringvalidator.OneOf(signingAlgs...)
}

// FlowsEnabledSet validates the OAuth flows. Rauthy rejects an empty list, so
// the size bound is part of the contract rather than a nicety.
func FlowsEnabledSet() []validator.Set {
	return []validator.Set{
		setvalidator.SizeAtLeast(1),
		setvalidator.ValueStringsAre(stringvalidator.OneOf(grantTypes...)),
	}
}

// ChallengeSet validates the PKCE challenge methods. As with flows, an empty
// list is rejected by Rauthy when the field is present at all.
func ChallengeSet() []validator.Set {
	return []validator.Set{
		setvalidator.SizeAtLeast(1),
		setvalidator.ValueStringsAre(stringvalidator.OneOf(challengeMethods...)),
	}
}

// AuthCodeLifetime bounds auth_code_lifetime (seconds).
func AuthCodeLifetime() validator.Int64 {
	return int64validator.Between(authCodeLifetimeMin, authCodeLifetimeMax)
}

// AccessTokenLifetime bounds access_token_lifetime (seconds).
func AccessTokenLifetime() validator.Int64 {
	return int64validator.Between(accessTokenLifetimeMin, accessTokenLifetimeMax)
}

// CacheCurrentHours bounds cache_current_hours on a secret rotation.
func CacheCurrentHours() validator.Int64 {
	return int64validator.Between(cacheCurrentHoursMin, cacheCurrentHoursMax)
}

// RoleName validates a role name against RE_ROLES_SCOPES, the same pattern
// Rauthy applies to scopes.
func RoleName() validator.String {
	return stringvalidator.RegexMatches(scope,
		`must match ^[a-zA-Z0-9-_/,:*.]{2,64}$`)
}

// GroupName validates a group name against RE_GROUPS.
func GroupName() validator.String {
	return stringvalidator.RegexMatches(groups,
		`must match ^[a-zA-Z0-9-_/,:*\s]{2,64}$`)
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
	return stringvalidator.RegexMatches(scope,
		`must match ^[a-zA-Z0-9-_/,:*.]{2,64}$`)
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
	return stringvalidator.RegexMatches(userAttrDesc,
		`must match ^[a-zA-Z0-9-_/]{0,128}$ (no spaces)`)
}

// Language validates a UI/email language against Rauthy's Language enum.
func Language() validator.String {
	return stringvalidator.OneOf(languages...)
}

// PersonName validates a given or family name.
func PersonName() validator.String {
	return stringvalidator.RegexMatches(personName,
		"must be 1-32 characters of letters, digits, spaces, '-' or '\\''")
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
	return stringvalidator.RegexMatches(street,
		"must be at most 48 characters of letters, digits, spaces, '-' or '.'")
}

// AddressPart validates a user's city or country.
func AddressPart() validator.String {
	return stringvalidator.RegexMatches(addressPart,
		"must be at most 48 characters of letters, digits, spaces or '-'")
}

// Timezone validates a user's timezone. Rauthy only checks that the value
// parses as an IANA name, so this is a shape check rather than a lookup.
func Timezone() validator.String {
	return stringvalidator.RegexMatches(timezone,
		"must be an IANA timezone such as Europe/Berlin")
}
