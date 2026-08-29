package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
)

var (
	regexpMissingSelector      = regexp.MustCompile(`Missing Rauthy user selector`)
	regexpConflictingSelectors = regexp.MustCompile(`Conflicting Rauthy user selectors`)
)

func testAccUserConfig(email, extra string) string {
	return fmt.Sprintf(`
resource "rauthy_user" "test" {
  email      = %q
  given_name = "Ada"
  %s
}
`, email, extra)
}

func testAccCheckUsersDestroyed(s *terraform.State) error {
	c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rauthy_user" {
			continue
		}
		_, lookupErr := c.GetUser(context.Background(), rs.Primary.ID)
		if lookupErr == nil {
			return fmt.Errorf("user %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFound(lookupErr) {
			return lookupErr
		}
	}
	return nil
}

func TestAccUser_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckUsersDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig("tf-acc-user@example.com", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rauthy_user.test", "id"),
					resource.TestCheckResourceAttr("rauthy_user.test", "language", "en"),
					// The defaults must survive the PUT that follows the
					// create; a user Rauthy just made is disabled until then.
					resource.TestCheckResourceAttr("rauthy_user.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rauthy_user.test", "email_verified", "false"),
					resource.TestCheckResourceAttrSet("rauthy_user.test", "created_at"),
					resource.TestCheckResourceAttrSet("rauthy_user.test", "account_type"),
				),
			},
			{
				Config:   testAccUserConfig("tf-acc-user@example.com", ""),
				PlanOnly: true,
			},
			{
				Config: testAccUserConfig("tf-acc-user@example.com", `
  family_name    = "Lovelace"
  language       = "de"
  email_verified = true
  enabled        = false
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_user.test", "family_name", "Lovelace"),
					resource.TestCheckResourceAttr("rauthy_user.test", "language", "de"),
					resource.TestCheckResourceAttr("rauthy_user.test", "email_verified", "true"),
					resource.TestCheckResourceAttr("rauthy_user.test", "enabled", "false"),
				),
			},
			{
				// Profile values round-trip through the nested block.
				Config: testAccUserConfig("tf-acc-user@example.com", `
  user_values = {
    city = "London"
    tz   = "Europe/London"
  }
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_user.test", "user_values.city", "London"),
					resource.TestCheckResourceAttr("rauthy_user.test", "user_values.tz", "Europe/London"),
				),
			},
			{
				Config: testAccUserConfig("tf-acc-user@example.com", `
  user_values = {
    city = "London"
    tz   = "Europe/London"
  }
`),
				PlanOnly: true,
			},
			{
				// Changing the email updates the account in place; the id must
				// not change, which ImportStateVerify below would not catch.
				Config: testAccUserConfig("tf-acc-user-renamed@example.com", ""),
				Check: resource.TestCheckResourceAttr(
					"rauthy_user.test", "email", "tf-acc-user-renamed@example.com"),
			},
			{
				ResourceName:      "rauthy_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Never returned by Rauthy, so it cannot be verified against a
				// fresh import.
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

// The point of the resource is a user that carries roles and groups, which
// only works if Terraform creates them first.
func TestAccUser_withRolesAndGroups(t *testing.T) {
	factories := acctest.Setup(t)

	config := `
resource "rauthy_role" "test" {
  name = "tf-acc-user-role"
}

resource "rauthy_group" "test" {
  name = "tf-acc-user-group"
}

resource "rauthy_user" "test" {
  email      = "tf-acc-user-roles@example.com"
  given_name = "Ada"
  roles      = [rauthy_role.test.name]
  groups     = [rauthy_group.test.name]
}

data "rauthy_user" "by_email" {
  email      = rauthy_user.test.email
  depends_on = [rauthy_user.test]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckUsersDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_user.test", "roles.#", "1"),
					resource.TestCheckTypeSetElemAttr(
						"rauthy_user.test", "roles.*", "tf-acc-user-role"),
					resource.TestCheckResourceAttr("rauthy_user.test", "groups.#", "1"),
					resource.TestCheckTypeSetElemAttr(
						"rauthy_user.test", "groups.*", "tf-acc-user-group"),
					resource.TestCheckResourceAttrPair(
						"data.rauthy_user.by_email", "id", "rauthy_user.test", "id"),
					resource.TestCheckResourceAttr(
						"data.rauthy_user.by_email", "roles.#", "1"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

// Selecting nothing, or both, must fail at plan time rather than silently
// picking one.
func TestAccUserDataSource_selectorErrors(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:      `data "rauthy_user" "test" {}`,
				ExpectError: regexpMissingSelector,
			},
			{
				Config: `
data "rauthy_user" "test" {
  id    = "some-id"
  email = "someone@example.com"
}
`,
				ExpectError: regexpConflictingSelectors,
			},
		},
	})
}
