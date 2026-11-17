package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAPIKey_duplicateGroups(t *testing.T) {
	factories := acctest.Setup(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: testAccAPIKeyConfig(
					"tf-acc-duplicate",
					`[{ group = "Clients", access_rights = ["read"] }, { group = "Clients", access_rights = ["update"] }]`,
					"",
				),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("Duplicate API key access group"),
			},
		},
	})
}

func TestAccAPIKey_unknownRights(t *testing.T) {
	factories := acctest.Setup(t)
	for _, update := range []bool{false, true} {
		for _, rights := range []string{"read", "update"} {
			t.Run(fmt.Sprintf("update=%t/rights=%s", update, rights), func(t *testing.T) {
				name := fmt.Sprintf("tf-rights-%t-%s", update, rights)
				steps := []resource.TestStep{}
				if update {
					steps = append(steps, resource.TestStep{
						Config: testAccAPIKeyConfig(
							name,
							`[{group = "Clients", access_rights = ["read"]}]`,
							"",
						),
					})
				}
				step := resource.TestStep{
					Config: fmt.Sprintf(`resource "terraform_data" "rights" { input = [%q] }`, rights) +
						testAccAPIKeyConfig(
							name,
							`[{group = "Clients", access_rights = ["read"]}, {group = "Clients", access_rights = terraform_data.rights.output}]`,
							"",
						),
					Check: resource.TestCheckResourceAttr("rauthy_api_key.test", "access.#", "1"),
				}
				if rights != "read" {
					step.ExpectError = regexp.MustCompile("Duplicate API key access group")
					step.Check = nil
				}
				steps = append(steps, step)
				resource.Test(t, resource.TestCase{
					ProtoV6ProviderFactories: factories,
					CheckDestroy:             testAccCheckAPIKeyDestroyed,
					Steps:                    steps,
				})
			})
		}
	}
}

func TestAccAuthProvider_incompleteClaimPairs(t *testing.T) {
	factories := acctest.Setup(t)
	for _, prefix := range []string{"admin", "mfa"} {
		for _, field := range []string{"path", "value"} {
			t.Run(prefix+"/"+field, func(t *testing.T) {
				config := fmt.Sprintf(`
resource "rauthy_auth_provider" "incomplete" {
 name = "TP3 incomplete pair"
 type = "oidc"
 issuer = "https://idp.example.com"
 authorization_endpoint = "https://idp.example.com/authorize"
 token_endpoint = "https://idp.example.com/token"
 userinfo_endpoint = "https://idp.example.com/userinfo"
 client_id = "tp3"
 scopes = ["openid"]
 %s_claim_%s = "member"
}
`, prefix, field)
				resource.Test(t, resource.TestCase{
					ProtoV6ProviderFactories: factories,
					Steps: []resource.TestStep{
						{Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("must be specified when")},
					},
				})
			})
		}
	}
}

func TestAccClient_unknownSCIMPlan(t *testing.T) {
	factories := acctest.Setup(t)
	config := func(uri, scim string) string {
		return fmt.Sprintf(`
resource "terraform_data" "scim" {
 input = {
  base_uri = %q
  bearer_token = "synthetic-token"
  sync_groups = false
 }
}
resource "rauthy_client" "test" {
 id = "tf-acc-unknown-scim"
 confidential = true
 redirect_uris = ["https://app.example.com/callback"]
 %s
}
`, uri, scim)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckClientDestroyed,
		Steps: []resource.TestStep{
			{Config: config("https://app.example.com/scim", "")},
			{
				Config: config("https://app.example.com/scim/v2", "scim = terraform_data.scim.output"),
				Check: resource.TestCheckResourceAttr(
					"rauthy_client.test",
					"scim.base_uri",
					"https://app.example.com/scim/v2",
				),
			},
		},
	})
}
