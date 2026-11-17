package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// A refused destruction must stop the whole plan before independent creates.
func TestAccSubstringGuardDeletionPlan(t *testing.T) {
	factories := acctest.Setup(t)
	for _, kind := range []string{"role", "group", "scope"} {
		t.Run(kind, func(t *testing.T) {
			short := fmt.Sprintf(`resource "rauthy_%s" "short" { name = "dev" }
`, kind)
			long := fmt.Sprintf(`resource "rauthy_%s" "long" { name = "developer" }
`, kind)
			independent := fmt.Sprintf(`resource "rauthy_role" "independent" { name = "tr1-independent-%s" }
`, kind)
			api, err := rauthy.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
			if err != nil {
				t.Fatal(err)
			}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{
					{Config: short + long},
					{
						Config:      long + independent,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile("Unsafe " + kind + " deletion"),
					},
					{Config: long + independent, ExpectError: regexp.MustCompile("Unsafe " + kind + " deletion")},
					{Config: short, PreConfig: func() {
						_, lookupErr := api.GetRoleByName(context.Background(), "tr1-independent-"+kind)
						if !rauthy.IsNotFound(lookupErr) {
							t.Fatalf("independent resource must not be created, lookup: %v", lookupErr)
						}
						t.Log("plan and apply refused unsafe " + kind + " deletion, independent resource absent")
					}},
				},
			})
		})
	}
}
