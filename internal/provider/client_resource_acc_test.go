package provider_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
)

// These run against a live Rauthy instance and are gated on TF_ACC; see
// internal/provider/acctest.

const accClientID = "tf-acc-client"

func testAccClientConfig(name string, redirect string) string {
	return fmt.Sprintf(`
resource "rauthy_client" "test" {
  id            = %q
  name          = %q
  confidential  = true
  redirect_uris = [%q]

  scopes                = ["openid", "profile"]
  default_scopes        = ["openid"]
  access_token_lifetime = 600
}
`, accClientID, name, redirect)
}

func envOrEmpty(k string) string { return os.Getenv(k) }

func testAccCheckClientDestroyed(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rauthy_client" {
			continue
		}
		c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
		if err != nil {
			return err
		}
		_, err = c.GetClient(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("client %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func TestAccClient_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckClientDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccClientConfig("TF Acc Client", "https://app.example.com/callback"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_client.test", "id", accClientID),
					resource.TestCheckResourceAttr("rauthy_client.test", "confidential", "true"),
					resource.TestCheckResourceAttr("rauthy_client.test", "access_token_lifetime", "600"),
					// A confidential client must come back with a secret, and
					// the attributes left unset must be filled from Rauthy's
					// defaults rather than staying unknown.
					resource.TestCheckResourceAttrSet("rauthy_client.test", "secret"),
					resource.TestCheckResourceAttrSet("rauthy_client.test", "access_token_alg"),
					resource.TestCheckResourceAttrSet("rauthy_client.test", "auth_code_lifetime"),
					resource.TestCheckResourceAttr("rauthy_client.test", "enabled", "true"),
				),
			},
			{
				Config: testAccClientConfig("TF Acc Client Renamed", "https://app.example.com/callback2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_client.test", "name", "TF Acc Client Renamed"),
					resource.TestCheckResourceAttr("rauthy_client.test", "redirect_uris.#", "1"),
				),
			},
			{
				ResourceName:      "rauthy_client.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Neither exists in Rauthy, so an import cannot recover them.
				ImportStateVerifyIgnore: []string{
					"secret_rotation_trigger",
					"secret_cache_current_hours",
				},
			},
		},
	})
}

// The secret must change when, and only when, the trigger changes.
func TestAccClient_secretRotation(t *testing.T) {
	factories := acctest.Setup(t)

	cfg := func(trigger string) string {
		return fmt.Sprintf(`
resource "rauthy_client" "rotate" {
  id            = "tf-acc-rotate"
  confidential  = true
  redirect_uris = ["https://app.example.com/callback"]

  secret_rotation_trigger    = %q
  secret_cache_current_hours = 1
}
`, trigger)
	}

	var first, second, third string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckClientDestroyed,
		Steps: []resource.TestStep{
			{
				Config: cfg("v1"),
				Check:  captureAttr("rauthy_client.rotate", "secret", &first),
			},
			{
				// Same trigger: nothing should be rotated.
				Config: cfg("v1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureAttr("rauthy_client.rotate", "secret", &second),
					func(*terraform.State) error {
						if first != second {
							return errors.New("secret changed without a trigger change")
						}
						return nil
					},
				),
			},
			{
				Config: cfg("v2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureAttr("rauthy_client.rotate", "secret", &third),
					func(*terraform.State) error {
						if third == second {
							return errors.New("secret unchanged after the trigger changed")
						}
						return nil
					},
				),
			},
		},
	})
}

// A public client has no secret at all.
func TestAccClient_publicHasNoSecret(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckClientDestroyed,
		Steps: []resource.TestStep{
			{
				Config: `
resource "rauthy_client" "public" {
  id            = "tf-acc-public"
  confidential  = false
  redirect_uris = ["https://spa.example.com/callback"]
}
`,
				Check: resource.TestCheckNoResourceAttr("rauthy_client.public", "secret"),
			},
		},
	})
}

func captureAttr(resourceName, attr string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not in state", resourceName)
		}
		v, ok := rs.Primary.Attributes[attr]
		if !ok {
			return fmt.Errorf("attribute %s not set on %s", attr, resourceName)
		}
		*into = v
		return nil
	}
}
