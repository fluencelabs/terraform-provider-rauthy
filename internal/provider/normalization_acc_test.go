package provider_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const normalizationConfig = `
resource "rauthy_client" "normalization" {
 id = "tf-normalization"
 confidential = true
 redirect_uris = ["https://app.example.com/callback"]
 post_logout_redirect_uris = []
 allowed_origins = []
 contacts = []
 challenges = ["S256"]
 scopes = ["openid", "profile"]
 default_scopes = ["openid"]
 scim = {
  base_uri = "https://127.0.0.1:20120/scim/v2/"
  bearer_token = "synthetic-scim-token"
  sync_groups = false
 }
}
resource "rauthy_user" "normalization" {
 email = "Mixed@Example.com"
 given_name = "Round"
}
resource "rauthy_user_attribute" "jsonnull" {
 name = "jsonnull"
 default_value = "null"
 user_editable = true
}
resource "rauthy_user_attribute" "bigint" {
 name = "bigint"
 default_value = "{\"n\":9007199254740992}"
}
resource "rauthy_pam_group" "normalization" {
 name = "normhost"
 typ = "host"
}
resource "rauthy_pam_host" "normalization" {
 hostname = "normhost"
 gid = rauthy_pam_group.normalization.id
 ips = ["2001:0DB8:0000:0000:0000:0000:0000:0002"]
}
`

func TestAccNormalization(t *testing.T) {
	factories := acctest.Setup(t)
	api := acctest.RealClient(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: normalizationConfig, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("rauthy_client.normalization", "allowed_origins.#", "0"),
				resource.TestCheckResourceAttr("rauthy_client.normalization", "challenges.#", "1"),
				resource.TestCheckResourceAttr("rauthy_user.normalization", "email", "Mixed@Example.com"),
				resource.TestCheckResourceAttr("rauthy_user_attribute.jsonnull", "default_value", "null"),
				resource.TestCheckResourceAttr("rauthy_user_attribute.jsonnull", "user_editable", "true"),
				resource.TestCheckTypeSetElemAttr(
					"rauthy_pam_host.normalization",
					"ips.*",
					"2001:0DB8:0000:0000:0000:0000:0000:0002",
				),
			)},
			{Config: normalizationConfig, PlanOnly: true},
			{Config: normalizationConfig, PlanOnly: true, ExpectNonEmptyPlan: true, PreConfig: func() {
				editable := false
				err := api.UpdateUserAttr(
					context.Background(),
					"bigint",
					rauthy.UserAttrRequest{
						Name:         "bigint",
						DefaultValue: []byte(`{"n":9007199254740993}`),
						UserEditable: &editable,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
			}},
		},
	})
}

func TestAccScopesPlan(t *testing.T) {
	factories := acctest.Setup(t)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{
		Config: `resource "rauthy_client" "test" {
 id = "tf-normalization-scope"
 confidential = true
 redirect_uris = ["https://example.com/callback"]
 scopes = ["profile"]
 default_scopes = ["profile"]
}`, PlanOnly: true, ExpectError: regexp.MustCompile("Missing openid scope"),
	}}})
}

// A PAM user takes the email of a user written in mixed case. Rauthy returns
// it lowercased for both; both keep the configured value.
func TestAccPamUserEmailNormalization(t *testing.T) {
	factories := acctest.Setup(t)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{
		Config: `
resource "rauthy_user" "pam_mixed" {
 email = "PamMixed@Example.com"
 given_name = "Pam"
}
resource "rauthy_pam_user" "pam_mixed" {
 username = "pammixed"
 email = rauthy_user.pam_mixed.email
}
`,
		Check: resource.TestCheckResourceAttr("rauthy_pam_user.pam_mixed", "email", "PamMixed@Example.com"),
	}}})
}
