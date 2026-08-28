// Package acctest provides the setup helper for acceptance tests, which run
// against a real Rauthy instance. Tests skip unless TF_ACC=1, RAUTHY_URL and
// RAUTHY_API_KEY are all set.
package acctest

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider"
)

// Setup verifies the environment and returns provider factories pointed at the
// configured Rauthy instance, or skips the test with a reason.
func Setup(t *testing.T) map[string]func() (tfprotov6.ProviderServer, error) {
	t.Helper()

	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run acceptance tests")
	}
	if os.Getenv("RAUTHY_URL") == "" {
		t.Skip("set RAUTHY_URL to run acceptance tests")
	}
	if os.Getenv("RAUTHY_API_KEY") == "" {
		t.Skip("set RAUTHY_API_KEY to run acceptance tests")
	}

	return map[string]func() (tfprotov6.ProviderServer, error){
		"rauthy": providerserver.NewProtocol6WithError(provider.New("acc")()),
	}
}

// RealClient returns a client pointed at the same instance the provider under
// test talks to, for assertions that need to look at Rauthy directly.
func RealClient(t *testing.T) *client.Client {
	t.Helper()

	c, err := client.New(os.Getenv("RAUTHY_URL"), os.Getenv("RAUTHY_API_KEY"))
	if err != nil {
		t.Fatalf("build acceptance test client: %v", err)
	}
	return c
}
