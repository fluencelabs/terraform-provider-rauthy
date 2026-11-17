package provider

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/rauthy-go"
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
	resp := &rauthy.UserResponse{
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

// The password is write-only, so it comes from the configuration rather than
// from the model, and applyUser has nothing to say about it either way.
func TestBuildUpdateUserRequest_CarriesTheWriteOnlyPassword(t *testing.T) {
	t.Parallel()

	var diags diag.Diagnostics
	m := &userResourceModel{Email: types.StringValue("ada@example.com")}

	got := buildUpdateUserRequest(context.Background(), m, types.StringValue("hunter2hunter2"), &diags)
	if got.Password == nil || got.Password.Reveal() != "hunter2hunter2" {
		t.Errorf("password = %v, want the configured value sent to Rauthy", got.Password)
	}

	// No password configured means none sent — not an empty one, which Rauthy
	// would reject against the password policy.
	got = buildUpdateUserRequest(context.Background(), m, types.StringNull(), &diags)
	if got.Password != nil {
		t.Errorf("password = %q, want it omitted", *got.Password)
	}
}

// user_values is only tracked when the configuration has the block; a user who
// set their own city must not show up as drift for a configuration that says
// nothing about it.
func TestApplyUser_IgnoresUserValuesWhenBlockAbsent(t *testing.T) {
	t.Parallel()

	city := "London"
	resp := &rauthy.UserResponse{
		ID:         "u-1",
		Email:      "ada@example.com",
		Language:   "en",
		UserValues: rauthy.UserValuesResponse{UserValues: rauthy.UserValues{City: &city}},
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

func TestUserUpdate_PreservesProfileAndControlsPassword(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after types.String
		password      types.String
		wantPassword  bool
	}{
		{"unchanged", types.StringValue("1"), types.StringValue("1"), types.StringValue("SyntheticPassword!42"), false},
		{"unset", types.StringNull(), types.StringNull(), types.StringValue("SyntheticPassword!42"), false},
		{"set", types.StringNull(), types.StringValue("1"), types.StringValue("SyntheticPassword!42"), true},
		{"changed", types.StringValue("1"), types.StringValue("2"), types.StringValue("SyntheticPassword!42"), true},
		{"removed", types.StringValue("1"), types.StringNull(), types.StringValue("SyntheticPassword!42"), true},
		{"absent password", types.StringValue("1"), types.StringValue("2"), types.StringNull(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkUserUpdate(t, tc.before, tc.after, tc.password, tc.wantPassword)
		})
	}
}

func TestUserUpdate_ReadFailureDoesNotWrite(t *testing.T) {
	fake, api := newFakeRauthy(t)
	fake.handle("GET /users/u-1", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	r := &userResource{api: api}
	m := userResourceModel{
		ID:     types.StringValue("u-1"),
		Roles:  types.SetNull(types.StringType),
		Groups: types.SetNull(types.StringType),
	}
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(
		context.Background(),
		resource.UpdateRequest{Plan: resourcePlan(t, r, &m), State: resourceState(t, r, &m)},
		&resp,
	)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected read failure")
	}
	if len(fake.bodies("PUT /users/u-1")) != 0 {
		t.Fatal("wrote after failed read")
	}
}

func TestUserCreate_SendsPassword(t *testing.T) {
	fake, api := newFakeRauthy(t)
	current := &rauthy.UserResponse{ID: "u-1", Email: "ada@example.com", Language: "en"}
	fake.answer("POST /users", current)
	fake.answer("PUT /users/u-1", current)
	r := &userResource{api: api}
	var plan userResourceModel
	applyUser(&plan, current)
	config := plan
	config.PasswordWO = types.StringValue("SyntheticPassword!42")
	cp := resourcePlan(t, r, &config)
	resp := resource.CreateResponse{State: emptyState(t, r)}
	r.Create(
		context.Background(),
		resource.CreateRequest{Plan: resourcePlan(t, r, &plan), Config: tfsdk.Config(cp)},
		&resp,
	)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var sent rauthy.UpdateUserRequest
	fake.lastBody(t, "PUT /users/u-1", &sent)
	if sent.Password == nil || sent.Password.Reveal() != config.PasswordWO.ValueString() {
		t.Fatal("creation password lost")
	}
}

func TestUserUpdate_ManagedProfileClearsOmittedFields(t *testing.T) {
	fake, api := newFakeRauthy(t)
	city := "Paris"
	current := &rauthy.UserResponse{
		ID:         "u-1",
		Email:      "ada@example.com",
		Language:   "en",
		UserValues: rauthy.UserValuesResponse{UserValues: rauthy.UserValues{City: &city}},
	}
	fake.answer("PUT /users/u-1", current)
	r := &userResource{api: api}
	var plan userResourceModel
	applyUser(&plan, current)
	plan.UserValues = &userValuesModel{City: types.StringValue(city)}
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(
		context.Background(),
		resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &plan)},
		&resp,
	)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var sent rauthy.UpdateUserRequest
	fake.lastBody(t, "PUT /users/u-1", &sent)
	if !reflect.DeepEqual(sent.UserValues, &rauthy.UserValues{City: &city}) {
		t.Fatalf("managed profile = %+v", sent.UserValues)
	}
}

func checkUserUpdate(t *testing.T, before, after, password types.String, wantPassword bool) {
	t.Helper()
	city, empty, birth, country, phone, street, zip, tz, username := "London", "", "2000-01-02", "UK", "+44123456789", "Street", "12345", "Europe/London", "ada"
	for _, values := range []rauthy.UserValues{
		{Birthdate: &birth, City: &city, Country: &country, Phone: &phone, Street: &street, Zip: &zip, TZ: &tz},
		{City: &empty, Country: nil},
	} {
		current := &rauthy.UserResponse{
			ID:         "u-1",
			Email:      "ada@example.com",
			Language:   "en",
			UserValues: rauthy.UserValuesResponse{UserValues: values, PreferredUsername: &username},
		}
		fake, api := newFakeRauthy(t)
		fake.answer("GET /users/u-1", current)
		fake.answer("PUT /users/u-1", current)
		r := &userResource{api: api}
		var state userResourceModel
		applyUser(&state, current)
		state.PasswordRotationTrigger = before
		plan := state
		plan.Enabled = types.BoolValue(false)
		plan.PasswordRotationTrigger = after
		config := plan
		config.PasswordWO = password
		configPlan := resourcePlan(t, r, &config)
		resp := resource.UpdateResponse{State: emptyState(t, r)}
		r.Update(context.Background(), resource.UpdateRequest{
			Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state),
			Config: tfsdk.Config(configPlan),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		var sent rauthy.UpdateUserRequest
		fake.lastBody(t, "PUT /users/u-1", &sent)
		if !reflect.DeepEqual(sent.UserValues, &values) {
			t.Errorf("profile = %+v, want %+v", sent.UserValues, values)
		}
		if wantPassword {
			if sent.Password == nil || sent.Password.Reveal() != password.ValueString() {
				t.Errorf("password = %v", sent.Password)
			}
		} else if sent.Password != nil {
			t.Error("unexpected password")
		}
		var got userResourceModel
		if d := resp.State.Get(context.Background(), &got); d.HasError() {
			t.Fatal(d)
		}
		if got.PreferredUsername.ValueString() != username {
			t.Error("preferred_username lost")
		}
		if got.UserValues != nil {
			t.Error("unmanaged profile entered state")
		}
	}
}
