# API keys are imported by their name, which is also their identity in Rauthy:
#   curl -H "Authorization: API-Key $RAUTHY_API_KEY" \
#     "$RAUTHY_URL/auth/v1/api_keys" | jq -r '.keys[].name'
#
# The secret does NOT come back. Rauthy discloses it once, when the key is
# minted, and has no endpoint that reads it afterwards — so an imported key has
# a null `secret` until you rotate it by setting secret_rotation_trigger.
terraform import rauthy_api_key.ci ci-clients
