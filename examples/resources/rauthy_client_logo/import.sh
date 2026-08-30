# Branding is imported by the id of the client that owns it.
#
# Import recovers only the fact that a logo exists, plus the hash of what
# Rauthy serves. It cannot recover content_base64: Rauthy stores a transcode of
# the original upload, so there is no byte sequence to write back that would
# round-trip. Expect the first plan after an import to show content_base64
# being set, and applying it to re-upload the image the configuration names.
terraform import rauthy_client_logo.dashboard dashboard
