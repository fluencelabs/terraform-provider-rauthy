package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// Rauthy requires `roles` and `groups` to be present on a PUT. A nil slice
// marshals to null, which its deserializer rejects with a 400, so the empty
// case has to become [] rather than being dropped.
func TestSetToStringsNonNil_NeverReturnsNil(t *testing.T) {
	t.Parallel()

	cases := map[string]types.Set{
		"null set":  types.SetNull(types.StringType),
		"empty set": types.SetValueMust(types.StringType, nil),
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var diags diag.Diagnostics
			got := setToStringsNonNil(context.Background(), set, &diags)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got == nil {
				t.Fatal("got nil, which would marshal to null and be rejected")
			}
			if len(got) != 0 {
				t.Fatalf("got %v, want an empty slice", got)
			}
		})
	}
}

// A user Rauthy returns without groups must not spuriously differ from a
// configuration that wrote `groups = []`.
func TestApplyUser_KeepsEmptySetDistinctFromUnset(t *testing.T) {
	t.Parallel()

	emptySet := types.SetValueMust(types.StringType, nil)
	resp := &client.UserResponse{
		ID:       "u-1",
		Email:    "ada@example.com",
		Language: "en",
		Roles:    []string{"admin"},
	}

	wroteEmpty := &userResourceModel{Groups: emptySet}
	applyUser(wroteEmpty, resp)
	if !wroteEmpty.Groups.Equal(emptySet) {
		t.Errorf("groups = %v, want the empty set the configuration wrote", wroteEmpty.Groups)
	}

	unset := &userResourceModel{Groups: types.SetNull(types.StringType)}
	applyUser(unset, resp)
	if !unset.Groups.IsNull() {
		t.Errorf("groups = %v, want null", unset.Groups)
	}
}

// The password is never returned by Rauthy. Deriving it from a response would
// blank it out on the first refresh, so applyUser must leave it alone.
func TestApplyUser_LeavesPasswordAlone(t *testing.T) {
	t.Parallel()

	m := &userResourceModel{Password: types.StringValue("hunter2hunter2")}
	applyUser(m, &client.UserResponse{ID: "u-1", Email: "ada@example.com", Language: "en"})
	if m.Password.ValueString() != "hunter2hunter2" {
		t.Errorf("password = %v, want it untouched", m.Password)
	}
}

// user_values is only tracked when the configuration has the block; a user who
// set their own city must not show up as drift for a configuration that says
// nothing about it.
func TestApplyUser_IgnoresUserValuesWhenBlockAbsent(t *testing.T) {
	t.Parallel()

	city := "London"
	resp := &client.UserResponse{
		ID:         "u-1",
		Email:      "ada@example.com",
		Language:   "en",
		UserValues: client.UserValuesResponse{UserValues: client.UserValues{City: &city}},
	}

	absent := &userResourceModel{}
	applyUser(absent, resp)
	if absent.UserValues != nil {
		t.Errorf("user_values = %+v, want nil", absent.UserValues)
	}

	present := &userResourceModel{UserValues: &userValuesModel{}}
	applyUser(present, resp)
	if present.UserValues == nil || present.UserValues.City.ValueString() != "London" {
		t.Errorf("user_values = %+v, want the city from the response", present.UserValues)
	}
}
