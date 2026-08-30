package provider_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
)

// These run against a live Rauthy instance and are gated on TF_ACC; see
// internal/provider/acctest.
//
// What cannot be exercised here: nothing in this file makes a real federated
// login work. Rauthy stores the endpoints without contacting them, so a
// provider pointing at idp.example.com is created, read, updated and deleted
// exactly like a real one — but the callback, link and login flows, and
// anything that depends on the upstream's actual claims (auto_link,
// auto_onboarding, the admin and MFA claim mappings), are only stored and read
// back here, never observed doing their job. Exercising those would need a
// second identity provider the acceptance suite could stand up alongside
// Rauthy.
//
// rauthy_auth_provider_lookup is not exercised here either, for a different
// reason: the discovery request is made by the Rauthy container, so a test for
// it would be a test of the container's internet access. It was verified by
// hand against accounts.google.com on a live 0.36.2 — see the note in
// splitAuthProviderScope about the space-separated form that endpoint returns.

const accAuthProviderName = "TF Acc Upstream"

func testAccAuthProviderConfig(name, secret string, scopes string) string {
	return fmt.Sprintf(`
resource "rauthy_auth_provider" "test" {
  name = %q
  type = "oidc"

  issuer                 = "https://idp.example.com"
  authorization_endpoint = "https://idp.example.com/authorize"
  token_endpoint         = "https://idp.example.com/token"
  userinfo_endpoint      = "https://idp.example.com/userinfo"
  jwks_endpoint          = "https://idp.example.com/jwks"

  client_id        = "rauthy-acc"
  client_secret_wo = %q
  scopes           = %s

  # A write-only value is invisible to the plan, so the trigger is what makes a
  # changed secret produce an apply at all. Keying it on the secret itself is
  # fine in a test; a real configuration would use a version counter.
  client_secret_rotation_trigger = %q
}
`, name, secret, scopes, secret)
}

func testAccCheckAuthProviderDestroyed(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rauthy_auth_provider" {
			continue
		}
		c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
		if err != nil {
			return err
		}
		_, err = c.GetAuthProvider(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("auth provider %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// testAccCheckAuthProviderSecretInRauthy asks Rauthy itself what secret it is
// holding for a provider.
//
// This is the check a write-only attribute needs and a state-based one cannot
// give. `client_secret_wo` is null in state whether the provider forwarded the
// configured value faithfully or dropped it on the floor, so asserting on state
// would pass just as happily against a provider that sends nothing at all.
// Rauthy returns the upstream secret in the clear on a read, which is what
// makes the round trip observable from out here. Pass "" to assert Rauthy holds
// no secret at all.
func testAccCheckAuthProviderSecretInRauthy(resourceName, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}
		c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
		if err != nil {
			return err
		}
		got, err := c.GetAuthProvider(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		stored := ""
		if got.ClientSecret != nil {
			stored = *got.ClientSecret
		}
		if stored != want {
			return fmt.Errorf("Rauthy holds client_secret %q for %s, want %q",
				stored, rs.Primary.ID, want)
		}
		return nil
	}
}

func TestAccAuthProvider_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		// client_secret_wo is a write-only attribute, which Terraform only
		// understands from 1.11 onwards.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckAuthProviderDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAuthProviderConfig(accAuthProviderName, "acc-upstream-secret",
					`["openid", "profile", "email"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rauthy_auth_provider.test", "id"),
					resource.TestCheckResourceAttr("rauthy_auth_provider.test", "name", accAuthProviderName),
					resource.TestCheckResourceAttr("rauthy_auth_provider.test", "type", "oidc"),
					resource.TestCheckResourceAttr("rauthy_auth_provider.test", "scopes.#", "3"),
					resource.TestCheckTypeSetElemAttr("rauthy_auth_provider.test", "scopes.*", "openid"),
					// The secret is write-only, so it is absent from state by
					// construction. The check that carries weight is the next
					// one: that Rauthy actually received it.
					resource.TestCheckNoResourceAttr("rauthy_auth_provider.test", "client_secret_wo"),
					testAccCheckAuthProviderSecretInRauthy("rauthy_auth_provider.test",
						"acc-upstream-secret"),
					// Defaults the configuration does not mention.
					resource.TestCheckResourceAttr("rauthy_auth_provider.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rauthy_auth_provider.test", "use_pkce", "true"),
					resource.TestCheckResourceAttr("rauthy_auth_provider.test", "auto_link", "false"),
				),
			},
			{
				// A rename, a new secret and a shorter scope list in one apply.
				// The scope change is the interesting half: it is the field
				// Rauthy hands back in a form it will not accept.
				Config: testAccAuthProviderConfig("TF Acc Upstream Renamed", "acc-upstream-secret-2",
					`["openid", "email"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_auth_provider.test", "name",
						"TF Acc Upstream Renamed"),
					testAccCheckAuthProviderSecretInRauthy("rauthy_auth_provider.test",
						"acc-upstream-secret-2"),
					resource.TestCheckResourceAttr("rauthy_auth_provider.test", "scopes.#", "2"),
					resource.TestCheckNoResourceAttr("rauthy_auth_provider.test", "admin_claim_path"),
				),
			},
			{
				ResourceName: "rauthy_auth_provider.test",
				ImportState:  true,
				// Every attribute of this resource survives a round trip
				// through Rauthy — except the two carrying the secret, which
				// are not in state to survive anything. Rauthy would hand the
				// secret back; the provider deliberately drops it.
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"client_secret_wo", "client_secret_rotation_trigger",
				},
			},
		},
	})
}

// Applying the same configuration twice must produce an empty plan. That is not
// a formality for this resource: Rauthy rewrites `scope` on the way in, so a
// provider that stored the string it sent rather than the string Rauthy kept
// would show a permanent diff here.
func TestAccAuthProvider_scopesAreStableAcrossApplies(t *testing.T) {
	factories := acctest.Setup(t)

	cfg := `
resource "rauthy_auth_provider" "scopes" {
  name = "TF Acc Scopes"
  type = "custom"

  issuer                 = "https://idp2.example.com"
  authorization_endpoint = "https://idp2.example.com/authorize"
  token_endpoint         = "https://idp2.example.com/token"
  userinfo_endpoint      = "https://idp2.example.com/userinfo"

  client_id = "rauthy-acc-2"
  scopes    = ["openid", "profile", "email", "groups"]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckAuthProviderDestroyed,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_auth_provider.scopes", "scopes.#", "4"),
					// No secret was configured, so Rauthy holds none.
					testAccCheckAuthProviderSecretInRauthy("rauthy_auth_provider.scopes", ""),
				),
			},
			{
				Config:   cfg,
				PlanOnly: true,
			},
		},
	})
}

// The claim mappings and the onboarding switches are stored and read back
// verbatim. What they do at login time is not observable without a real
// upstream; this pins the configuration surface only.
func TestAccAuthProvider_claimMappings(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckAuthProviderDestroyed,
		Steps: []resource.TestStep{
			{
				Config: `
resource "rauthy_auth_provider" "claims" {
  name    = "TF Acc Claims"
  type    = "custom"
  enabled = false

  issuer                 = "https://idp3.example.com"
  authorization_endpoint = "https://idp3.example.com/authorize"
  token_endpoint         = "https://idp3.example.com/token"
  userinfo_endpoint      = "https://idp3.example.com/userinfo"

  client_id = "rauthy-acc-3"
  scopes    = ["openid"]

  admin_claim_path  = "$.roles"
  admin_claim_value = "rauthy-admin"
  mfa_claim_path    = "$.amr"
  mfa_claim_value   = "mfa"

  auto_onboarding = true
  auto_link       = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_auth_provider.claims", "enabled", "false"),
					resource.TestCheckResourceAttr("rauthy_auth_provider.claims", "admin_claim_path", "$.roles"),
					resource.TestCheckResourceAttr("rauthy_auth_provider.claims", "mfa_claim_value", "mfa"),
					resource.TestCheckResourceAttr("rauthy_auth_provider.claims", "auto_onboarding", "true"),
					resource.TestCheckResourceAttr("rauthy_auth_provider.claims", "auto_link", "true"),
				),
			},
		},
	})
}

// The data source resolves a provider the same configuration created, which is
// the only way to look one up: there is no GET /providers/{id}.
func TestAccAuthProviderDataSource_byName(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		// client_secret_wo is a write-only attribute, which Terraform only
		// understands from 1.11 onwards.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckAuthProviderDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAuthProviderConfig("TF Acc Lookup Target", "acc-secret",
					`["openid", "profile"]`) + `
data "rauthy_auth_provider" "found" {
  name       = rauthy_auth_provider.test.name
  depends_on = [rauthy_auth_provider.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.rauthy_auth_provider.found", "id",
						"rauthy_auth_provider.test", "id"),
					resource.TestCheckResourceAttr("data.rauthy_auth_provider.found", "type", "oidc"),
					resource.TestCheckResourceAttr("data.rauthy_auth_provider.found", "scopes.#", "2"),
					resource.TestCheckResourceAttr("data.rauthy_auth_provider.found",
						"issuer", "https://idp.example.com"),
					// The data source deliberately does not carry the secret.
					resource.TestCheckNoResourceAttr("data.rauthy_auth_provider.found", "client_secret"),
					// And the resource did put the configured one into Rauthy.
					testAccCheckAuthProviderSecretInRauthy("rauthy_auth_provider.test", "acc-secret"),
				),
			},
		},
	})
}
