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

const accImageClientID = "tf-acc-branding"

// testAccClientImageConfig declares a host client plus whichever branding
// resources the caller wants hung off it.
func testAccClientImageConfig(branding string) string {
	return fmt.Sprintf(`
resource "rauthy_client" "branding" {
  id            = %q
  name          = "TF Acc Branding"
  confidential  = true
  redirect_uris = ["https://example.com/callback"]
}

%s
`, accImageClientID, branding)
}

const testAccLogoPNG = `
resource "rauthy_client_logo" "test" {
  client_id      = rauthy_client.branding.id
  content_base64 = filebase64("testdata/logo.png")
}
`

const testAccLogoPNGAlt = `
resource "rauthy_client_logo" "test" {
  client_id      = rauthy_client.branding.id
  content_base64 = filebase64("testdata/logo-alt.png")
}
`

const testAccLogoSVGAndFavicon = `
resource "rauthy_client_logo" "test" {
  client_id      = rauthy_client.branding.id
  content_base64 = filebase64("testdata/favicon.svg")
}

resource "rauthy_client_favicon" "test" {
  client_id      = rauthy_client.branding.id
  content_base64 = filebase64("testdata/favicon.svg")
}
`

// testAccCheckClientImagesDestroyed asserts the images are gone. It cannot look
// at the client, which the same config destroys: a 404 from the image endpoint
// covers both "no image" and "no client", which is exactly what we want here.
func testAccCheckClientImagesDestroyed(s *terraform.State) error {
	c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		var kind client.ImageKind
		switch rs.Type {
		case "rauthy_client_logo":
			kind = client.ImageLogo
		case "rauthy_client_favicon":
			kind = client.ImageFavicon
		default:
			continue
		}
		id := rs.Primary.Attributes["client_id"]
		_, lookupErr := c.GetClientImage(context.Background(), id, kind)
		if lookupErr == nil {
			return fmt.Errorf("%s of client %s still exists after destroy", kind, id)
		}
		if !client.IsNotFound(lookupErr) {
			return lookupErr
		}
	}
	return nil
}

// testAccCheckImagePresent confirms Rauthy really serves the image, and that
// what it serves has the content type the transcode is expected to produce.
func testAccCheckImagePresent(kind client.ImageKind, wantContentType string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
		if err != nil {
			return err
		}
		got, err := c.GetClientImage(context.Background(), accImageClientID, kind)
		if err != nil {
			return fmt.Errorf("read back %s: %w", kind, err)
		}
		if len(got.Data) == 0 {
			return fmt.Errorf("%s came back empty", kind)
		}
		if got.ContentType != wantContentType {
			return fmt.Errorf("%s content type = %q, want %q", kind, got.ContentType, wantContentType)
		}
		return nil
	}
}

func TestAccClientImage_lifecycle(t *testing.T) {
	factories := acctest.Setup(t)

	var firstHash string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckClientImagesDestroyed,
		Steps: []resource.TestStep{
			{
				// Upload. A PNG is stored transcoded to WebP, so stored_hash
				// can never equal a hash of the file on disk.
				Config: testAccClientImageConfig(testAccLogoPNG),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_client_logo.test", "client_id", accImageClientID),
					resource.TestCheckResourceAttr("rauthy_client_logo.test", "content_type", client.MimePNG),
					resource.TestCheckResourceAttrSet("rauthy_client_logo.test", "stored_hash"),
					testAccCheckImagePresent(client.ImageLogo, "image/webp"),
					extractAttr("rauthy_client_logo.test", "stored_hash", &firstHash),
				),
			},
			{
				// Replace in place: a different PNG must move stored_hash.
				Config: testAccClientImageConfig(testAccLogoPNGAlt),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckImagePresent(client.ImageLogo, "image/webp"),
					checkAttrDiffers("rauthy_client_logo.test", "stored_hash", &firstHash),
				),
			},
			{
				// Swap the logo to an SVG and add a favicon. SVG is the one
				// format Rauthy keeps as-is rather than transcoding, though it
				// still re-serialises it through the sanitiser.
				Config: testAccClientImageConfig(testAccLogoSVGAndFavicon),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rauthy_client_logo.test", "content_type", client.MimeSVG),
					resource.TestCheckResourceAttr("rauthy_client_favicon.test", "content_type", client.MimeSVG),
					testAccCheckImagePresent(client.ImageLogo, client.MimeSVG),
					testAccCheckImagePresent(client.ImageFavicon, client.MimeSVG),
				),
			},
			{
				// Removal: dropping the branding resources while keeping the
				// client must delete the images and leave the client alone.
				Config: testAccClientImageConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkImageAbsent(client.ImageLogo),
					checkImageAbsent(client.ImageFavicon),
					resource.TestCheckResourceAttr("rauthy_client.branding", "id", accImageClientID),
				),
			},
		},
	})
}

// TestAccClientImage_recreatedAfterOutOfBandDelete covers the one drift the
// resource does reconcile: the image disappearing from the server.
func TestAccClientImage_recreatedAfterOutOfBandDelete(t *testing.T) {
	factories := acctest.Setup(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckClientImagesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccClientImageConfig(testAccLogoPNG),
				Check:  testAccCheckImagePresent(client.ImageLogo, "image/webp"),
			},
			{
				// Delete behind Terraform's back; the refresh must notice the
				// 404 and plan the upload again.
				PreConfig: func() {
					c := acctest.RealClient(t)
					if err := c.DeleteClientImage(
						context.Background(), accImageClientID, client.ImageLogo,
					); err != nil {
						t.Fatalf("out-of-band delete: %v", err)
					}
				},
				Config: testAccClientImageConfig(testAccLogoPNG),
				Check:  testAccCheckImagePresent(client.ImageLogo, "image/webp"),
			},
		},
	})
}

func extractAttr(resourceName, attr string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not in state", resourceName)
		}
		*into = rs.Primary.Attributes[attr]
		return nil
	}
}

func checkAttrDiffers(resourceName, attr string, previous *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not in state", resourceName)
		}
		if got := rs.Primary.Attributes[attr]; got == *previous {
			return fmt.Errorf("%s.%s did not change: still %s", resourceName, attr, got)
		}
		return nil
	}
}

func checkImageAbsent(kind client.ImageKind) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(envOrEmpty("RAUTHY_URL"), envOrEmpty("RAUTHY_API_KEY"))
		if err != nil {
			return err
		}
		_, lookupErr := c.GetClientImage(context.Background(), accImageClientID, kind)
		if lookupErr == nil {
			return fmt.Errorf("%s is still served after its resource was removed", kind)
		}
		if !client.IsNotFound(lookupErr) {
			return lookupErr
		}
		return nil
	}
}
