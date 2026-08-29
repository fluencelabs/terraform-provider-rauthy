package provider_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
)

// These run against a live Rauthy instance and are gated on TF_ACC; see
// internal/provider/acctest.

func testAccRoleConfig(name string) string {
	return fmt.Sprintf(`
resource "rauthy_role" "test" {
  name = %q
}
`, name)
}

func testAccGroupConfig(name string) string {
	return fmt.Sprintf(`
resource "rauthy_group" "test" {
  name = %q
}
`, name)
}

// testAccCheckRolesGroupsDestroyed asserts that every role and group this test
// created is gone from Rauthy, not merely from state.
func testAccCheckRolesGroupsDestroyed(s *terraform.State) error {
	c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
	if err != nil {
		return err
	}

	for _, rs := range s.RootModule().Resources {
		var lookupErr error
		switch rs.Type {
		case "rauthy_role":
			_, lookupErr = c.GetRole(context.Background(), rs.Primary.ID)
		case "rauthy_group":
			_, lookupErr = c.GetGroup(context.Background(), rs.Primary.ID)
		default:
			continue
		}

		if lookupErr == nil {
			return fmt.Errorf("%s %s still exists after destroy", rs.Type, rs.Primary.ID)
		}
		if !client.IsNotFound(lookupErr) {
			return lookupErr
		}
	}
	return nil
}

func TestAccRole_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckRolesGroupsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccRoleConfig("tf-acc-role"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_role.test", "name", "tf-acc-role"),
					resource.TestCheckResourceAttrSet("rauthy_role.test", "id"),
				),
			},
			{
				// A rename is an in-place update, not a replacement.
				Config: testAccRoleConfig("tf-acc-role-renamed"),
				Check: resource.TestCheckResourceAttr(
					"rauthy_role.test", "name", "tf-acc-role-renamed"),
			},
			{
				ResourceName:      "rauthy_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccGroup_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckRolesGroupsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccGroupConfig("tf-acc-group"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_group.test", "name", "tf-acc-group"),
					resource.TestCheckResourceAttrSet("rauthy_group.test", "id"),
				),
			},
			{
				Config: testAccGroupConfig("tf-acc-group-renamed"),
				Check: resource.TestCheckResourceAttr(
					"rauthy_group.test", "name", "tf-acc-group-renamed"),
			},
			{
				ResourceName:      "rauthy_group.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// The data sources resolve a name to the id the resource just created.
func TestAccRoleGroupDataSources_lookupByName(t *testing.T) {
	factories := acctest.Setup(t)

	config := `
resource "rauthy_role" "test" {
  name = "tf-acc-ds-role"
}

resource "rauthy_group" "test" {
  name = "tf-acc-ds-group"
}

data "rauthy_role" "test" {
  name       = rauthy_role.test.name
  depends_on = [rauthy_role.test]
}

data "rauthy_group" "test" {
  name       = rauthy_group.test.name
  depends_on = [rauthy_group.test]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckRolesGroupsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.rauthy_role.test", "id", "rauthy_role.test", "id"),
					resource.TestCheckResourceAttrPair(
						"data.rauthy_group.test", "id", "rauthy_group.test", "id"),
				),
			},
		},
	})
}
