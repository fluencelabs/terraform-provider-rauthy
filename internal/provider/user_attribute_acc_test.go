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

func testAccUserAttributeConfig(name, body string) string {
	return fmt.Sprintf(`
resource "rauthy_user_attribute" "test" {
  name = %q
  %s
}
`, name, body)
}

func testAccCheckUserAttributesDestroyed(s *terraform.State) error {
	c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rauthy_user_attribute" {
			continue
		}
		_, lookupErr := c.GetUserAttr(context.Background(), rs.Primary.ID)
		if lookupErr == nil {
			return fmt.Errorf("user attribute %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFound(lookupErr) {
			return lookupErr
		}
	}
	return nil
}

func TestAccUserAttribute_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckUserAttributesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccUserAttributeConfig("tf-acc-attr", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_user_attribute.test", "name", "tf-acc-attr"),
					// Rauthy keys attributes by name; the id mirrors it.
					resource.TestCheckResourceAttr("rauthy_user_attribute.test", "id", "tf-acc-attr"),
					resource.TestCheckResourceAttr("rauthy_user_attribute.test", "user_editable", "false"),
					resource.TestCheckNoResourceAttr("rauthy_user_attribute.test", "desc"),
				),
			},
			{
				Config: testAccUserAttributeConfig("tf-acc-attr", `
  desc          = "cost-center"
  default_value = jsonencode("engineering")
  user_editable = true`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_user_attribute.test", "desc", "cost-center"),
					resource.TestCheckResourceAttr(
						"rauthy_user_attribute.test", "default_value", `"engineering"`),
					resource.TestCheckResourceAttr("rauthy_user_attribute.test", "user_editable", "true"),
				),
			},
			{
				// Re-applying must be a no-op. A default Rauthy re-serialises
				// differently from the configuration would show up here.
				Config: testAccUserAttributeConfig("tf-acc-attr", `
  desc          = "cost-center"
  default_value = jsonencode("engineering")
  user_editable = true`),
				PlanOnly: true,
			},
			{
				// A structured default written with whitespace Rauthy will not
				// give back: it stores the parsed document and re-serialises it
				// compacted. Post-apply state must still hold what the
				// configuration wrote.
				Config: testAccUserAttributeConfig("tf-acc-attr", `
  default_value = "{\"code\": 7, \"label\": \"eng\"}"`),
				Check: resource.TestCheckResourceAttr(
					"rauthy_user_attribute.test", "default_value", `{"code": 7, "label": "eng"}`),
			},
			{
				// The perpetual diff this resource used to have: without JSON
				// semantic equality the refreshed state is Rauthy's compacted
				// spelling and every plan wants to rewrite it.
				Config: testAccUserAttributeConfig("tf-acc-attr", `
  default_value = "{\"code\": 7, \"label\": \"eng\"}"`),
				PlanOnly: true,
			},
			{
				// A rename replaces the attribute. Rauthy's in-place rename
				// leaves the old name occupied — invisible in GET /users/attr
				// but still rejecting a POST — so the provider deletes and
				// recreates instead.
				Config: testAccUserAttributeConfig("tf-acc-attr-renamed", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"rauthy_user_attribute.test", "name", "tf-acc-attr-renamed"),
					resource.TestCheckResourceAttr(
						"rauthy_user_attribute.test", "id", "tf-acc-attr-renamed"),
				),
			},
			{
				// Renaming back proves the point: had the rename gone through
				// PUT, the original name would still be occupied and this
				// create would fail with "User attribute config does already
				// exist".
				Config: testAccUserAttributeConfig("tf-acc-attr", ""),
				Check: resource.TestCheckResourceAttr(
					"rauthy_user_attribute.test", "name", "tf-acc-attr"),
			},
			{
				ResourceName:      "rauthy_user_attribute.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// The reason this resource exists: a scope's attr_include_* mapping is filtered
// against the defined attributes and unknown names are dropped silently, so
// before this resource a scope could only map attributes created by hand. Here
// Terraform creates both, and the dependency orders them.
func TestAccUserAttribute_referencedByScope(t *testing.T) {
	factories := acctest.Setup(t)

	config := `
resource "rauthy_user_attribute" "department" {
  name = "tf-acc-attr-dept"
  desc = "department"
}

resource "rauthy_user_attribute" "cost_center" {
  name = "tf-acc-attr-cc"
}

resource "rauthy_scope" "test" {
  name = "tf-acc-attr-scope"

  attr_include_access = [
    rauthy_user_attribute.department.name,
    rauthy_user_attribute.cost_center.name,
  ]
  attr_include_id = [rauthy_user_attribute.department.name]
}

data "rauthy_user_attribute" "department" {
  name       = rauthy_user_attribute.department.name
  depends_on = [rauthy_user_attribute.department]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckUserAttributesDestroyed,
			testAccCheckScopesDestroyed,
		),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					// If Rauthy had dropped either name, the scope resource
					// would have failed the apply outright.
					resource.TestCheckResourceAttr("rauthy_scope.test", "attr_include_access.#", "2"),
					resource.TestCheckTypeSetElemAttr(
						"rauthy_scope.test", "attr_include_access.*", "tf-acc-attr-dept"),
					resource.TestCheckTypeSetElemAttr(
						"rauthy_scope.test", "attr_include_access.*", "tf-acc-attr-cc"),
					resource.TestCheckResourceAttr("rauthy_scope.test", "attr_include_id.#", "1"),
					resource.TestCheckResourceAttr(
						"data.rauthy_user_attribute.department", "desc", "department"),
					resource.TestCheckResourceAttrPair(
						"data.rauthy_user_attribute.department", "id",
						"rauthy_user_attribute.department", "id"),
				),
			},
		},
	})
}
