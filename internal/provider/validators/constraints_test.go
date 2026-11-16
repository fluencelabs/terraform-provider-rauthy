package validators_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// TestConstraintCases uses the unchanged fixture from rauthy-go v0.2.0
// (515768c), testdata/constraints-cases.json, including both-version negatives.
func TestConstraintCases(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/constraints-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		ID        string `json:"id"`
		Resource  string `json:"resource"`
		Field     string `json:"field"`
		Value     string `json:"value"`
		Validator string `json:"validator"`
		Expected  bool   `json:"expected"`
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	stringsByName := map[string]validator.String{
		"ClientName": validators.ClientName(), "SigningAlgs": validators.SigningAlg(),
		"UserName": validators.PersonName(), "AttributeDescription": validators.UserAttrDesc(),
		"URI": validators.URI(), "Languages": validators.Language(), "IPAddress": validators.IPAddress(),
		"Timezone": validators.Timezone(), "Group": validators.GroupName(), "City": validators.AddressPart(),
		"Street": validators.Street(), "Scope": validators.ScopeName(),
	}
	setsByName := map[string][]validator.Set{
		"Contact": {validators.ContactSet()}, "GrantTypes": validators.FlowsEnabledSet(),
		"Origin": {validators.OriginSet()}, "AuthProviderScope": {validators.AuthProviderScopeSet()},
	}
	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			t.Parallel()
			var got bool
			switch {
			case tc.Resource == "provider" && tc.Field == "name":
				got = acceptsString(validators.AuthProviderName(), tc.Value)
			case tc.Resource == "provider" && tc.Field == "client_id":
				got = acceptsString(validators.AuthProviderURI(), tc.Value)
			case stringsByName[tc.Validator] != nil:
				got = acceptsString(stringsByName[tc.Validator], tc.Value)
			case setsByName[tc.Validator] != nil:
				values := []string{tc.Value}
				if tc.Validator == "AuthProviderScope" {
					values = strings.Split(tc.Value, " ")
				}
				got = acceptsSet(setsByName[tc.Validator], values...)
			default:
				t.Fatalf("unmapped validator %s", tc.Validator)
			}
			if got != tc.Expected {
				t.Errorf("%s.%s %q: accepted=%v, want %v", tc.Resource, tc.Field, tc.Value, got, tc.Expected)
			}
		})
	}
}

func TestAuthProviderScopesWireValue(t *testing.T) {
	t.Parallel()
	vs := []validator.Set{validators.AuthProviderScopeSet()}
	for _, values := range [][]string{
		{"openid read.profile"}, {""}, {"openid\tprofile"},
		{strings.Repeat("a", 256), strings.Repeat("b", 256)},
	} {
		if acceptsSet(vs, values...) {
			t.Errorf("accepted invalid scope set %q", values)
		}
	}
	if !acceptsSet(vs, strings.Repeat("a", 256), strings.Repeat("b", 255)) {
		t.Error("rejected 512-character joined scope string")
	}
	if !acceptsSet([]validator.Set{validators.PamHostIPSet()}, "2001:db8::1") {
		t.Error("rejected valid PAM IP")
	}
	if acceptsSet([]validator.Set{validators.PamHostIPSet()}, "fe80::1%eth0") {
		t.Error("accepted scoped PAM IP")
	}
}
