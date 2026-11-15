package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fluencelabs/rauthy-go"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
)

// PUT /scopes/{id} replaces the scope wholesale. claims_at_root set once must
// survive an update that is about something else.
func TestAccScope_claimsAtRoot(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckScopesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccScopeConfig("tf-acc-scope-root", "claims_at_root = true"),
				Check:  resource.TestCheckResourceAttr("rauthy_scope.test", "claims_at_root", "true"),
			},
			{
				// Renamed, with claims_at_root no longer in the configuration.
				Config: testAccScopeConfig("tf-acc-scope-root2", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_scope.test", "name", "tf-acc-scope-root2"),
					resource.TestCheckResourceAttr("rauthy_scope.test", "claims_at_root", "true"),
				),
			},
			{
				Config:   testAccScopeConfig("tf-acc-scope-root2", ""),
				PlanOnly: true,
			},
			{
				Config: testAccScopeConfig("tf-acc-scope-root2", "claims_at_root = false"),
				Check:  resource.TestCheckResourceAttr("rauthy_scope.test", "claims_at_root", "false"),
			},
		},
	})
}

const accFullReplaceClientID = "tf-acc-full-replace"

func testAccFullReplaceClientConfig(lifetime int) string {
	return fmt.Sprintf(`
resource "rauthy_client" "test" {
  id                    = %q
  confidential          = false
  redirect_uris         = ["https://app.example.com/callback"]
  access_token_lifetime = %d
}
`, accFullReplaceClientID, lifetime)
}

// rauthy_client has no attributes for these; the test sets them behind
// Terraform's back, the way the Admin UI would.
const accClaims = `{"tier":"gold"}`

func accDefaultAud() []string { return []string{"billing-api"} }

func testAccSetServerOnlyClientFields(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	c, err := rauthy.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := c.GetClient(ctx, accFullReplaceClientID)
	if err != nil {
		t.Fatal(err)
	}
	req := cur.UpdateRequest()
	req.DefaultAud = accDefaultAud()
	req.Claims = json.RawMessage(accClaims)
	req.ClaimsAtRoot = true
	if _, err = c.UpdateClient(ctx, accFullReplaceClientID, req); err != nil {
		t.Fatal(err)
	}
}

func testAccCheckServerOnlyClientFieldsKept(*terraform.State) error {
	c, err := rauthy.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
	if err != nil {
		return err
	}
	got, err := c.GetClient(context.Background(), accFullReplaceClientID)
	if err != nil {
		return err
	}
	var want, have any
	_ = json.Unmarshal([]byte(accClaims), &want)
	_ = json.Unmarshal(got.Claims, &have)
	if !slices.Equal(got.DefaultAud, accDefaultAud()) || !got.ClaimsAtRoot ||
		!bytes.Equal(mustJSON(want), mustJSON(have)) {
		return fmt.Errorf("the update reset settings set outside Terraform: default_aud=%v claims=%s "+
			"claims_at_root=%v", got.DefaultAud, got.Claims, got.ClaimsAtRoot)
	}
	return nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// PUT /clients/{id} replaces the client wholesale. Settings rauthy_client does
// not model must survive an apply that changes something it does.
func TestAccClient_keepsSettingsItDoesNotModel(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckClientDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccFullReplaceClientConfig(600),
			},
			{
				PreConfig: func() { testAccSetServerOnlyClientFields(t) },
				Config:    testAccFullReplaceClientConfig(900),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_client.test", "access_token_lifetime", "900"),
					testAccCheckServerOnlyClientFieldsKept,
				),
			},
		},
	})
}
