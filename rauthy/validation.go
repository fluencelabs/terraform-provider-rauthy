package rauthy

import "regexp"

// Value constraints Rauthy enforces server-side on OIDC clients.
//
// Rauthy's OpenAPI document does not carry them: it is generated with utoipa,
// which does not emit the `validator` crate's ranges and regexes into the
// schema. Every pattern and bound below is transcribed from Rauthy v0.36.2:
// src/common/src/regex.rs, src/api_types/src/cust_validation.rs and the
// #[validate] attributes in src/api_types/src/clients.rs. Checking a value
// against them before a request turns a server-side 400 into an error that
// names the offending field.

// Patterns from src/common/src/regex.rs and cust_validation.rs.
var (
	// ClientIDPattern is RE_CLIENT_ID, the pattern of a client id. Rauthy 0.36
	// narrowed it: `POST /clients` now rejects everything outside letters,
	// digits, dot, underscore and hyphen, so ids that used to be legal
	// (anything with `/`, `,`, `:` and friends) are refused by a live server.
	ClientIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._\-]{2,256}$`)

	// ClientNamePattern is RE_CLIENT_NAME, the pattern of a client's display
	// name. The CJK ranges are part of the upstream pattern.
	ClientNamePattern = regexp.MustCompile(`^[a-zA-Z0-9À-ɏ\-\s` +
		`\x{3041}-\x{3096}\x{30A0}-\x{30FF}\x{3400}-\x{4DB5}\x{4E00}-\x{9FCB}` +
		`\x{F900}-\x{FA6A}\x{2E80}-\x{2FD5}\x{FF66}-\x{FF9F}\x{FFA1}-\x{FFDC}\x{31F0}-\x{31FF}]{2,128}$`)

	// URIPattern is RE_URI as applied to a client's redirect URIs and other
	// URI-valued fields.
	URIPattern = regexp.MustCompile(`^[a-zA-Z0-9,.:/_\-&?=~#!$'()*+%@]+$`)

	// ScopePattern is RE_ROLES_SCOPES, used for a client's scopes and
	// default_scopes. Rauthy applies the same pattern to role and scope names.
	ScopePattern = regexp.MustCompile(`^[a-zA-Z0-9-_/,:*.]{2,64}$`)

	// GroupPattern is RE_GROUPS, used for a client's groups prefix. Rauthy
	// applies the same pattern to group names.
	GroupPattern = regexp.MustCompile(`^[a-zA-Z0-9-_/,:*\s]{2,64}$`)

	// OriginPattern is validate_vec_origin, the pattern of each allowed
	// origin: scheme and host only, no path.
	OriginPattern = regexp.MustCompile(`^(http|https)://[a-z0-9.:-]+$`)

	// ContactPattern is validate_vec_contact, the pattern of each client
	// contact.
	ContactPattern = regexp.MustCompile(`^[a-zA-Z0-9+.@/-]{0,48}$`)
)

// Bounds from the #[validate(range(...))] attributes in
// src/api_types/src/clients.rs, inclusive.
const (
	// AuthCodeLifetimeMin and AuthCodeLifetimeMax bound auth_code_lifetime,
	// in seconds.
	AuthCodeLifetimeMin = 10
	AuthCodeLifetimeMax = 300

	// AccessTokenLifetimeMin and AccessTokenLifetimeMax bound
	// access_token_lifetime, in seconds.
	AccessTokenLifetimeMin = 10
	AccessTokenLifetimeMax = 86400

	// CacheCurrentHoursMin and CacheCurrentHoursMax bound cache_current_hours
	// on a client secret rotation: how long the previous secret stays valid.
	CacheCurrentHoursMin = 1
	CacheCurrentHoursMax = 24
)

// SigningAlgs returns the JWK signing algorithms Rauthy accepts for a client's
// access and id tokens (JwkKeyPairAlg). Each call returns a fresh slice.
func SigningAlgs() []string {
	return []string{"RS256", "RS384", "RS512", "EdDSA"}
}

// GrantTypes returns the OAuth flows Rauthy accepts in a client's
// flows_enabled (validate_vec_grant_types). Rauthy rejects an empty list. Each
// call returns a fresh slice.
func GrantTypes() []string {
	return []string{
		"authorization_code",
		"client_credentials",
		"urn:ietf:params:oauth:grant-type:device_code",
		"password",
		"refresh_token",
	}
}

// ChallengeMethods returns the PKCE challenge methods Rauthy accepts in a
// client's challenges (RE_CODE_CHALLENGE_METHOD). Rauthy rejects an empty list
// when the field is present. Each call returns a fresh slice.
func ChallengeMethods() []string {
	return []string{"plain", "S256"}
}
