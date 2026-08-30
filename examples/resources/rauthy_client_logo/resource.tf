resource "rauthy_client" "dashboard" {
  id            = "dashboard"
  name          = "Internal Dashboard"
  confidential  = true
  redirect_uris = ["https://dashboard.example.com/callback"]
}

# An SVG is the better choice: Rauthy stores it as SVG, while every raster
# upload is transcoded to WebP and must be at least 84 pixels on a side.
resource "rauthy_client_logo" "dashboard" {
  client_id      = rauthy_client.dashboard.id
  content_base64 = filebase64("${path.module}/logo.svg")
}

# A PNG works too; the content type is derived from the file's own bytes.
resource "rauthy_client_logo" "raster" {
  client_id      = "legacy-app"
  content_base64 = filebase64("${path.module}/logo.png")
}
