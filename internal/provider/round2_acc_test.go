package provider_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const round2Config = `
resource "rauthy_client" "round2" {
 id = "tf-round2"
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
resource "rauthy_user" "round2" {
 email = "RoundTwo@Example.com"
 given_name = "Round"
}
resource "rauthy_user_attribute" "round2null" {
 name = "round2null"
 default_value = "null"
 user_editable = true
}
resource "rauthy_user_attribute" "round2big" {
 name = "round2big"
 default_value = "{\"n\":9007199254740992}"
}
resource "rauthy_pam_group" "round2" {
 name = "round2host"
 typ = "host"
}
resource "rauthy_pam_host" "round2" {
 hostname = "round2host"
 gid = rauthy_pam_group.round2.id
 ips = ["2001:0DB8:0000:0000:0000:0000:0000:0002"]
}
`

func TestAccRound2Normalization(t *testing.T) {
	factories := acctest.Setup(t)
	api := acctest.RealClient(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: round2Config, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("rauthy_client.round2", "allowed_origins.#", "0"),
				resource.TestCheckResourceAttr("rauthy_client.round2", "challenges.#", "1"),
				resource.TestCheckResourceAttr("rauthy_user.round2", "email", "RoundTwo@Example.com"),
				resource.TestCheckResourceAttr("rauthy_user_attribute.round2null", "default_value", "null"),
				resource.TestCheckResourceAttr("rauthy_user_attribute.round2null", "user_editable", "true"),
				resource.TestCheckTypeSetElemAttr(
					"rauthy_pam_host.round2",
					"ips.*",
					"2001:0DB8:0000:0000:0000:0000:0000:0002",
				),
			)},
			{Config: round2Config, PlanOnly: true},
			{Config: round2Config, PlanOnly: true, ExpectNonEmptyPlan: true, PreConfig: func() {
				editable := false
				err := api.UpdateUserAttr(
					context.Background(),
					"round2big",
					rauthy.UserAttrRequest{
						Name:         "round2big",
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

func TestAccRound2ScopesPlan(t *testing.T) {
	factories := acctest.Setup(t)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{
		Config: `resource "rauthy_client" "test" {
 id = "tf-round2-scope"
 confidential = true
 redirect_uris = ["https://example.com/callback"]
 scopes = ["profile"]
 default_scopes = ["profile"]
}`, PlanOnly: true, ExpectError: regexp.MustCompile("Missing openid scope"),
	}}})
}
