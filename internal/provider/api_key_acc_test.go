package provider_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
)

// These run against a live Rauthy instance and are gated on TF_ACC; see
// internal/provider/acctest.
//
// The key the suite authenticates with needs `ApiKeys` read, create, update and
// delete — scripts/rauthy-up.sh bootstraps it with them. Without that group
// every step here 403s, and on Rauthy 0.35 and earlier the group does not exist
// at all, so this whole file is 0.36-and-later territory.

const accAPIKeyName = "tf-acc-key"

func testAccAPIKeyConfig(name, access, extra string) string {
	return fmt.Sprintf(`
resource "rauthy_api_key" "test" {
  name = %q

  access = %s
%s
}
`, name, access, extra)
}

func testAccCheckAPIKeyDestroyed(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rauthy_api_key" {
			continue
		}
		c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
		if err != nil {
			return err
		}
		// The resource has no `id` attribute, so Primary.ID is empty here and
		// the name has to come out of the attribute map — reading Primary.ID
		// would make this check pass without asking Rauthy anything.
		name := rs.Primary.Attributes["name"]
		if name == "" {
			return errors.New("rauthy_api_key in state without a name")
		}
		_, err = c.GetAPIKey(context.Background(), name)
		if err == nil {
			return fmt.Errorf("api key %s still exists after destroy", name)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// checkAPIKeySecretShape asserts the secret is the whole credential rather than
// its second half: it is meant to be usable as `api_key` verbatim.
func checkAPIKeySecretShape(name string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith("rauthy_api_key.test", "secret", func(value string) error {
		if !strings.HasPrefix(value, name+"$") || len(value) < len(name)+16 {
			return fmt.Errorf("secret %q is not in <name>$<secret> form", value)
		}
		return nil
	})
}

func TestAccAPIKey_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	// Captured across steps so the rotation step can prove the secret actually
	// changed rather than merely being present.
	var firstSecret string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckAPIKeyDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAPIKeyConfig(accAPIKeyName,
					`[
    {
      group         = "Users"
      access_rights = ["read", "create"]
    },
    {
      group         = "Roles"
      access_rights = ["read"]
    },
  ]`, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_api_key.test", "name", accAPIKeyName),
					resource.TestCheckResourceAttrSet("rauthy_api_key.test", "created_at"),
					resource.TestCheckNoResourceAttr("rauthy_api_key.test", "expires_at"),
					resource.TestCheckResourceAttr("rauthy_api_key.test", "access.#", "2"),
					checkAPIKeySecretShape(accAPIKeyName),
					resource.TestCheckResourceAttrWith("rauthy_api_key.test", "secret",
						func(value string) error {
							firstSecret = value
							return nil
						}),
				),
			},
			// Changing only the grants must leave the secret alone: an update
			// that silently rotated would break every consumer of the key.
			{
				Config: testAccAPIKeyConfig(accAPIKeyName,
					`[
    {
      group         = "Users"
      access_rights = ["read", "create", "update", "delete"]
    },
    {
      group         = "Clients"
      access_rights = []
    },
  ]`, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_api_key.test", "access.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("rauthy_api_key.test", "access.*",
						map[string]string{"group": "Clients", "access_rights.#": "0"}),
					resource.TestCheckResourceAttrWith("rauthy_api_key.test", "secret",
						func(value string) error {
							if value != firstSecret {
								return errors.New("secret changed on a grants-only update")
							}
							return nil
						}),
				),
			},
			// Rotation, driven by the trigger and nothing else.
			{
				Config: testAccAPIKeyConfig(accAPIKeyName,
					`[
    {
      group         = "Users"
      access_rights = ["read", "create", "update", "delete"]
    },
    {
      group         = "Clients"
      access_rights = []
    },
  ]`, "\n  secret_rotation_trigger = \"v2\"\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAPIKeySecretShape(accAPIKeyName),
					resource.TestCheckResourceAttrWith("rauthy_api_key.test", "secret",
						func(value string) error {
							if value == firstSecret {
								return errors.New("secret unchanged after a rotation")
							}
							return nil
						}),
				),
			},
			// The secret can never be read back, so it cannot be verified on
			// import — and neither can the trigger, which lives only in
			// Terraform.
			{
				ResourceName:  "rauthy_api_key.test",
				ImportState:   true,
				ImportStateId: accAPIKeyName,
				// The resource has no `id`: a key's identity is its name, and
				// inventing a second attribute holding the same string would be
				// a permanent lie about Rauthy's model.
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerify:                    true,
				ImportStateVerifyIgnore: []string{
					"secret",
					"secret_rotation_trigger",
				},
			},
		},
	})
}

// A rename cannot be an update: Rauthy compares the name in the body against
// the path and rejects a mismatch, so the resource must plan a replacement.
func TestAccAPIKey_renameReplaces(t *testing.T) {
	factories := acctest.Setup(t)

	const access = `[
    {
      group         = "Roles"
      access_rights = ["read"]
    },
  ]`

	var firstSecret string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckAPIKeyDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAPIKeyConfig("tf-acc-rename-a", access, ""),
				Check: resource.TestCheckResourceAttrWith("rauthy_api_key.test", "secret",
					func(value string) error {
						firstSecret = value
						return nil
					}),
			},
			{
				Config: testAccAPIKeyConfig("tf-acc-rename-b", access, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_api_key.test", "name", "tf-acc-rename-b"),
					// A replacement mints a new key, so the secret must differ.
					resource.TestCheckResourceAttrWith("rauthy_api_key.test", "secret",
						func(value string) error {
							if value == firstSecret {
								return errors.New("secret survived a replacement")
							}
							if !strings.HasPrefix(value, "tf-acc-rename-b$") {
								return fmt.Errorf("secret %q does not belong to the new key", value)
							}
							return nil
						}),
					// And the old one must be gone.
					func(*terraform.State) error {
						c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
						if err != nil {
							return err
						}
						_, err = c.GetAPIKey(context.Background(), "tf-acc-rename-a")
						if !client.IsNotFound(err) {
							return fmt.Errorf("tf-acc-rename-a still exists after the rename: %w", err)
						}
						return nil
					},
				),
			},
		},
	})
}

// A key the provider minted has to actually work — otherwise the resource is
// producing decorative strings. This is also the closest thing to an end-to-end
// test of the bootstrapping story the resource exists for.
func TestAccAPIKey_mintedKeyAuthenticates(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckAPIKeyDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAPIKeyConfig("tf-acc-usable",
					`[
    {
      group         = "Roles"
      access_rights = ["read"]
    },
  ]`, ""),
				Check: resource.TestCheckResourceAttrWith("rauthy_api_key.test", "secret",
					func(value string) error {
						c, err := client.New(envOrEmpty("RAUTHY_URL"), value)
						if err != nil {
							return err
						}
						if _, listErr := c.ListRoles(context.Background()); listErr != nil {
							return fmt.Errorf("the minted key cannot read roles: %w", listErr)
						}
						// And it must not reach past what it was granted — least
						// of all into /api_keys, where it could grant itself
						// anything.
						_, keysErr := c.ListAPIKeys(context.Background())
						if !client.IsForbidden(keysErr) {
							return fmt.Errorf(
								"the minted key reached /api_keys without an ApiKeys grant: %w", keysErr)
						}
						return nil
					}),
			},
		},
	})
}
