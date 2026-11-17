package provider_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/acctest"
)

// The password policy is a singleton that cannot be destroyed, so there is no
// CheckDestroy here: the test asserts the applied values instead.
//
// The plan-emptiness checks are the point of this test. Every attribute is
// Optional and not Computed, so anything the resource writes into state beyond
// the plan — a normalised value, or a zero value from an empty response body —
// shows up either as an apply-time "inconsistent result" failure or as a
// non-empty plan on the next step.
func TestAccPasswordPolicy_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)
	c := acctest.RealClient(t)
	ctx := context.Background()
	user, err := c.CreateUser(
		ctx,
		rauthy.NewUserRequest{
			Email:     "tf-policy@example.com",
			GivenName: new("Policy"),
			Language:  "en",
			Roles:     []string{"user"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if deleteErr := c.DeleteUser(ctx, user.ID); deleteErr != nil {
			t.Errorf("delete policy test user: %v", deleteErr)
		}
	})
	checkPassword := func(rejected bool) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			_, updateErr := c.UpdateUser(ctx, user.ID, rauthy.UpdateUserRequest{
				Email: user.Email, GivenName: user.GivenName, Roles: user.Roles, Enabled: true,
				Password: new(rauthy.NewSecret("Short1!a")),
			})
			if !rejected {
				return updateErr
			}
			if updateErr == nil {
				return errors.New("short password accepted, want HTTP 400")
			}
			var apiErr *rauthy.APIError
			if !errors.As(updateErr, &apiErr) || apiErr.StatusCode != http.StatusBadRequest ||
				!strings.Contains(apiErr.Message, "Minimum password length is 12") {
				return fmt.Errorf("short password: want minimum length 12 rejection: %w", updateErr)
			}
			return nil
		}
	}

	const strict = `
resource "rauthy_password_policy" "test" {
  length_min = 12
  length_max = 128

  include_lower_case = 1
  include_upper_case = 1
  include_digits     = 1
  include_special    = 1

  not_recently_used = 3
}
`

	// The same policy with several rules dropped. Unset means disabled, so this
	// must actually clear them rather than leave the previous values in place.
	const relaxed = `
resource "rauthy_password_policy" "test" {
  length_min = 8
  length_max = 64

  include_digits = 1
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: strict,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkPassword(true),
					resource.TestCheckResourceAttr("rauthy_password_policy.test", "length_min", "12"),
					resource.TestCheckResourceAttr("rauthy_password_policy.test", "length_max", "128"),
					resource.TestCheckResourceAttr("rauthy_password_policy.test", "include_special", "1"),
					resource.TestCheckResourceAttr("rauthy_password_policy.test", "not_recently_used", "3"),
				),
			},
			{
				// Re-applying the same config must be a no-op.
				Config:   strict,
				PlanOnly: true,
			},
			{
				Config: relaxed,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkPassword(false),
					resource.TestCheckResourceAttr("rauthy_password_policy.test", "length_min", "8"),
					resource.TestCheckResourceAttr("rauthy_password_policy.test", "length_max", "64"),
					resource.TestCheckNoResourceAttr("rauthy_password_policy.test", "include_special"),
					resource.TestCheckNoResourceAttr("rauthy_password_policy.test", "not_recently_used"),
				),
			},
			{
				Config:   relaxed,
				PlanOnly: true,
			},
			{
				// Import cannot work against Rauthy v0.36.2: it needs
				// GET /password_policy, which accepts only session auth.
				// Asserting the refusal keeps us honest — if a future Rauthy
				// opens the endpoint, this step fails and import can be
				// enabled for real.
				ResourceName:  "rauthy_password_policy.test",
				ImportState:   true,
				ImportStateId: "singleton",
				ExpectError:   regexp.MustCompile("does not allow importing the password policy"),
			},
		},
	})
}
