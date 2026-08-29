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

func testAccScopeConfig(name string, attrs string) string {
	return fmt.Sprintf(`
resource "rauthy_scope" "test" {
  name = %q
  %s
}
`, name, attrs)
}

func testAccCheckScopesDestroyed(s *terraform.State) error {
	c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rauthy_scope" {
			continue
		}
		_, lookupErr := c.GetScope(context.Background(), rs.Primary.ID)
		if lookupErr == nil {
			return fmt.Errorf("scope %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFound(lookupErr) {
			return lookupErr
		}
	}
	return nil
}

func TestAccScope_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckScopesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccScopeConfig("tf-acc-scope", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_scope.test", "name", "tf-acc-scope"),
					resource.TestCheckResourceAttrSet("rauthy_scope.test", "id"),
					resource.TestCheckNoResourceAttr("rauthy_scope.test", "attr_include_access"),
				),
			},
			{
				// The round trip that matters: sets go out as arrays and come
				// back as one comma-joined string.
				Config: testAccScopeConfig("tf-acc-scope",
					`attr_include_access = ["department", "cost_center"]
  attr_include_id     = ["department"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_scope.test", "attr_include_access.#", "2"),
					resource.TestCheckTypeSetElemAttr("rauthy_scope.test", "attr_include_access.*", "department"),
					resource.TestCheckTypeSetElemAttr("rauthy_scope.test", "attr_include_access.*", "cost_center"),
					resource.TestCheckResourceAttr("rauthy_scope.test", "attr_include_id.#", "1"),
				),
			},
			{
				// Re-applying must be a no-op; a mangled split would show here.
				Config: testAccScopeConfig("tf-acc-scope",
					`attr_include_access = ["department", "cost_center"]
  attr_include_id     = ["department"]`),
				PlanOnly: true,
			},
			{
				Config: testAccScopeConfig("tf-acc-scope-renamed",
					`attr_include_access = ["department", "cost_center"]
  attr_include_id     = ["department"]`),
				Check: resource.TestCheckResourceAttr("rauthy_scope.test", "name", "tf-acc-scope-renamed"),
			},
			{
				ResourceName:      "rauthy_scope.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// A scope is only useful if a client can reference it by name.
func TestAccScope_referencedByClient(t *testing.T) {
	factories := acctest.Setup(t)

	config := `
resource "rauthy_scope" "test" {
  name = "tf-acc-scope-ref"
}

resource "rauthy_client" "test" {
  id            = "tf-acc-scope-client"
  name          = "TF Acc Scope Client"
  confidential  = false
  redirect_uris = ["https://app.example.com/callback"]

  scopes         = ["openid", rauthy_scope.test.name]
  default_scopes = ["openid"]
}

data "rauthy_scope" "test" {
  name       = rauthy_scope.test.name
  depends_on = [rauthy_scope.test]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckScopesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemAttr("rauthy_client.test", "scopes.*", "tf-acc-scope-ref"),
					resource.TestCheckResourceAttrPair(
						"data.rauthy_scope.test", "id", "rauthy_scope.test", "id"),
				),
			},
		},
	})
}
