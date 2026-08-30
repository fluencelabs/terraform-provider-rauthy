resource "rauthy_client" "dashboard" {
  id            = "dashboard"
  name          = "Internal Dashboard"
  confidential  = true
  redirect_uris = ["https://dashboard.example.com/callback"]
}

# Rauthy applies its 84-pixel floor to favicons as well, so a conventional 16
# or 32 pixel .ico is rejected on two counts: too small, and not one of the
# three accepted upload types (PNG, JPEG, SVG).
resource "rauthy_client_favicon" "dashboard" {
  client_id      = rauthy_client.dashboard.id
  content_base64 = filebase64("${path.module}/favicon.svg")
}
