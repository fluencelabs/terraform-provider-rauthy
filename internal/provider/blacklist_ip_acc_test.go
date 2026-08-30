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

// Far enough in the future that these never expire while the suite runs; the
// point of the resource is long blocks, and a short one would make the test
// depend on the wall clock.
const (
	testAccBlacklistExp        = 4102444800 // 2100-01-01T00:00:00Z
	testAccBlacklistExpUpdated = 4102444900
)

var regexpDiscardedBlacklistEntry = regexp.MustCompile(`discarded the blacklist entry`)

func testAccBlacklistIPConfig(ip string, exp int64) string {
	return fmt.Sprintf(`
resource "rauthy_blacklist_ip" "test" {
  ip  = %q
  exp = %d
}
`, ip, exp)
}

func testAccCheckBlacklistIPsDestroyed(s *terraform.State) error {
	c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rauthy_blacklist_ip" {
			continue
		}
		_, lookupErr := c.GetBlacklistedIP(context.Background(), rs.Primary.ID)
		if lookupErr == nil {
			return fmt.Errorf("ip %s is still blacklisted after destroy", rs.Primary.ID)
		}
		if !client.IsNotFound(lookupErr) {
			return lookupErr
		}
	}
	return nil
}

func TestAccBlacklistIP_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckBlacklistIPsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccBlacklistIPConfig("203.0.113.7", testAccBlacklistExp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_blacklist_ip.test", "ip", "203.0.113.7"),
					resource.TestCheckResourceAttr("rauthy_blacklist_ip.test", "id", "203.0.113.7"),
					resource.TestCheckResourceAttr("rauthy_blacklist_ip.test", "exp",
						strconv.Itoa(testAccBlacklistExp)),
				),
			},
			{
				Config:   testAccBlacklistIPConfig("203.0.113.7", testAccBlacklistExp),
				PlanOnly: true,
			},
			{
				// POST is an upsert, so a new expiry is an in-place update
				// rather than a replace.
				Config: testAccBlacklistIPConfig("203.0.113.7", testAccBlacklistExpUpdated),
				Check: resource.TestCheckResourceAttr("rauthy_blacklist_ip.test", "exp",
					strconv.Itoa(testAccBlacklistExpUpdated)),
			},
			{
				ResourceName:      "rauthy_blacklist_ip.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// The address is the identity: changing it destroys the old
				// entry rather than moving it.
				Config: testAccBlacklistIPConfig("203.0.113.8", testAccBlacklistExp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_blacklist_ip.test", "ip", "203.0.113.8"),
					resource.TestCheckResourceAttr("rauthy_blacklist_ip.test", "id", "203.0.113.8"),
				),
			},
		},
	})
}

// Rauthy renders addresses canonically, so a non-canonical configuration must
// still round-trip: `ip` keeps what was written, `id` holds Rauthy's spelling,
// and re-applying is a no-op instead of a permanent diff.
func TestAccBlacklistIP_nonCanonicalIPv6(t *testing.T) {
	factories := acctest.Setup(t)

	config := testAccBlacklistIPConfig("2001:0DB8:0000:0000:0000:0000:0000:0002", testAccBlacklistExp)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckBlacklistIPsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_blacklist_ip.test", "ip",
						"2001:0DB8:0000:0000:0000:0000:0000:0002"),
					resource.TestCheckResourceAttr("rauthy_blacklist_ip.test", "id", "2001:db8::2"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

// An expiry that has already passed is accepted by Rauthy with 200 and then
// discarded. The apply has to fail rather than write state for a block that
// does not exist.
func TestAccBlacklistIP_expiredEntryIsAnError(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckBlacklistIPsDestroyed,
		Steps: []resource.TestStep{
			{
				// Above Rauthy's fixed lower bound of 1719784800, so it passes
				// server-side validation, but long past.
				Config:      testAccBlacklistIPConfig("203.0.113.9", 1719784801),
				ExpectError: regexpDiscardedBlacklistEntry,
			},
		},
	})
}
