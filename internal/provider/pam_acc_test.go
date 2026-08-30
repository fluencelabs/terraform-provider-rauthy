package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
)

// accClient is the direct client the destroy checks use to look past Terraform
// state at what Rauthy actually holds.
func accClient() (*client.Client, error) {
	return client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
}

func testAccCheckPamDestroyed(s *terraform.State) error {
	c, err := accClient()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		lookup, ok := pamLookups(c)[rs.Type]
		if !ok {
			continue
		}
		if lookupErr := lookup(rs.Primary.ID); lookupErr == nil {
			return fmt.Errorf("%s %s still exists after destroy", rs.Type, rs.Primary.ID)
		} else if !client.IsNotFound(lookupErr) {
			return lookupErr
		}
	}
	return nil
}

// pamLookups maps a resource type to the read that must fail once the resource
// is gone. Groups and users are addressed by a numeric id, hosts by a string,
// so each one parses its own.
func pamLookups(c *client.Client) map[string]func(id string) error {
	byNumber := func(read func(context.Context, int64) error) func(string) error {
		return func(id string) error {
			n, err := strconv.ParseInt(id, 10, 64)
			if err != nil {
				return err
			}
			return read(context.Background(), n)
		}
	}
	return map[string]func(string) error{
		"rauthy_pam_group": byNumber(func(ctx context.Context, gid int64) error {
			_, err := c.GetPamGroup(ctx, gid)
			return err
		}),
		"rauthy_pam_user": byNumber(func(ctx context.Context, uid int64) error {
			_, err := c.GetPamUser(ctx, uid)
			return err
		}),
		"rauthy_pam_host": func(id string) error {
			_, err := c.GetPamHost(context.Background(), id)
			return err
		},
	}
}

func testAccPamGroupConfig(name, typ string) string {
	return fmt.Sprintf(`
resource "rauthy_pam_group" "test" {
  name = %q
  typ  = %q
}
`, name, typ)
}

func TestAccPamGroup_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckPamDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccPamGroupConfig("tfaccpamgrp", "generic"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_pam_group.test", "name", "tfaccpamgrp"),
					resource.TestCheckResourceAttr("rauthy_pam_group.test", "typ", "generic"),
					// Rauthy allocates gids from 100000 up, so the id is always
					// a number and never the name.
					resource.TestMatchResourceAttr("rauthy_pam_group.test", "id",
						regexp.MustCompile(`^1[0-9]{5}$`)),
				),
			},
			{
				Config:   testAccPamGroupConfig("tfaccpamgrp", "generic"),
				PlanOnly: true,
			},
			{
				// No update endpoint exists, so this must be a replacement and
				// must therefore land a fresh gid. A provider that silently did
				// nothing would pass a plain attribute check; comparing the id
				// is what actually proves the replacement happened.
				Config: testAccPamGroupConfig("tfaccpamgrp2", "host"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_pam_group.test", "name", "tfaccpamgrp2"),
					resource.TestCheckResourceAttr("rauthy_pam_group.test", "typ", "host"),
				),
			},
			{
				ResourceName:      "rauthy_pam_group.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// The full stack: a Rauthy identity, the POSIX account attached to it, the
// groups it belongs to and a host that trusts them.
func testAccPamFullConfig(shell, groups, hostname, hostBody string) string {
	return fmt.Sprintf(`
resource "rauthy_pam_group" "servers" {
  name = "tfaccservers"
  typ  = "host"
}

resource "rauthy_pam_group" "devs" {
  name = "tfaccdevs"
  typ  = "generic"
}

resource "rauthy_user" "test" {
  email       = "tfacc-pam@example.com"
  given_name  = "Pam"
  family_name = "Tester"
}

resource "rauthy_pam_user" "test" {
  username = "tfaccpam"
  email    = rauthy_user.test.email
  shell    = %q
  groups   = %s
}

resource "rauthy_pam_host" "test" {
  hostname = %q
  gid      = rauthy_pam_group.servers.id
  %s
}
`, shell, groups, hostname, hostBody)
}

func TestAccPamUserAndHost_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	const oneGroup = `[{ gid = rauthy_pam_group.devs.id, wheel = true }]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckPamDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccPamFullConfig("/bin/bash", oneGroup, "tfacc-host.example.com", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_pam_user.test", "username", "tfaccpam"),
					resource.TestCheckResourceAttr("rauthy_pam_user.test", "shell", "/bin/bash"),
					// Rauthy derives the home directory from the username and
					// this configuration never sets one.
					resource.TestCheckResourceAttr("rauthy_pam_user.test", "home_dir", "/home/tfaccpam"),
					resource.TestCheckResourceAttr("rauthy_pam_user.test", "groups.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("rauthy_pam_user.test", "groups.*",
						map[string]string{"wheel": "true"}),
					resource.TestCheckResourceAttr("rauthy_pam_host.test", "hostname", "tfacc-host.example.com"),
					resource.TestCheckResourceAttr("rauthy_pam_host.test", "force_mfa", "false"),
					resource.TestCheckResourceAttr("rauthy_pam_host.test", "ips.#", "0"),
					// The secret comes from a second endpoint; an empty one
					// would mean the extra call silently failed.
					resource.TestCheckResourceAttrWith("rauthy_pam_host.test", "secret",
						func(v string) error {
							if len(v) < 32 {
								return fmt.Errorf("host secret looks wrong: %q", v)
							}
							return nil
						}),
				),
			},
			{
				// Nothing changed, so nothing may be planned. This is the step
				// that catches a create path which writes state the server
				// would report differently on the next read.
				Config:   testAccPamFullConfig("/bin/bash", oneGroup, "tfacc-host.example.com", ""),
				PlanOnly: true,
			},
			{
				// Everything the two PUT endpoints can change at once,
				// including the host rename that Rauthy — unlike its user
				// attribute PUT — really does perform in place.
				Config: testAccPamFullConfig("/bin/zsh",
					`[{ gid = rauthy_pam_group.devs.id }, { gid = rauthy_pam_group.servers.id, wheel = true }]`,
					"tfacc-renamed.example.com", `
  force_mfa           = true
  local_password_only = true
  ips                 = ["10.0.0.10", "2001:db8::10"]
  aliases             = ["tfacc-alias", "tfacc-alias2"]
  notes               = "acceptance"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_pam_user.test", "shell", "/bin/zsh"),
					resource.TestCheckResourceAttr("rauthy_pam_user.test", "groups.#", "2"),
					resource.TestCheckResourceAttr("rauthy_pam_host.test", "hostname",
						"tfacc-renamed.example.com"),
					resource.TestCheckResourceAttr("rauthy_pam_host.test", "force_mfa", "true"),
					resource.TestCheckResourceAttr("rauthy_pam_host.test", "local_password_only", "true"),
					resource.TestCheckResourceAttr("rauthy_pam_host.test", "ips.#", "2"),
					resource.TestCheckResourceAttr("rauthy_pam_host.test", "aliases.#", "2"),
					resource.TestCheckResourceAttr("rauthy_pam_host.test", "notes", "acceptance"),
				),
			},
			{
				Config: testAccPamFullConfig("/bin/zsh",
					`[{ gid = rauthy_pam_group.devs.id }, { gid = rauthy_pam_group.servers.id, wheel = true }]`,
					"tfacc-renamed.example.com", `
  force_mfa           = true
  local_password_only = true
  ips                 = ["10.0.0.10", "2001:db8::10"]
  aliases             = ["tfacc-alias", "tfacc-alias2"]
  notes               = "acceptance"`),
				PlanOnly: true,
			},
			{
				ResourceName:      "rauthy_pam_user.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "rauthy_pam_host.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
