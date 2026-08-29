package client

import (
	"context"
	"net/http"
)

// PasswordPolicy is both the body of PUT /password_policy and the shape
// returned by GET. The policy is a singleton: there is exactly one per Rauthy
// instance and it cannot be created or deleted, only read and replaced.
//
// The optional fields are Option<i32> upstream. A null disables the rule
// rather than leaving it unchanged, because PUT replaces the policy wholesale.
type PasswordPolicy struct {
	LengthMin        int32  `json:"length_min"`
	LengthMax        int32  `json:"length_max"`
	IncludeLowerCase *int32 `json:"include_lower_case"`
	IncludeUpperCase *int32 `json:"include_upper_case"`
	IncludeDigits    *int32 `json:"include_digits"`
	IncludeSpecial   *int32 `json:"include_special"`
	ValidDays        *int32 `json:"valid_days"`
	NotRecentlyUsed  *int32 `json:"not_recently_used"`
}

// GetPasswordPolicy issues GET /password_policy.
func (c *Client) GetPasswordPolicy(ctx context.Context) (*PasswordPolicy, error) {
	var out PasswordPolicy
	if err := c.do(ctx, http.MethodGet, "/password_policy", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePasswordPolicy issues PUT /password_policy, replacing the policy.
func (c *Client) UpdatePasswordPolicy(ctx context.Context, req PasswordPolicy) (*PasswordPolicy, error) {
	var out PasswordPolicy
	if err := c.do(ctx, http.MethodPut, "/password_policy", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
