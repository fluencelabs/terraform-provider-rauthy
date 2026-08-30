# Providers are imported by their Rauthy-assigned id. Rauthy has no
# GET /providers/{id}, so find it in the list — which is served on POST:
#   curl -X POST -H "Authorization: API-Key $RAUTHY_API_KEY" \
#     "$RAUTHY_URL/auth/v1/providers" | jq -r '.[] | "\(.id)\t\(.name)"'
#
# The upstream secret is not recovered. Rauthy does return it on a read, but
# client_secret_wo is write-only and the provider deliberately drops it rather
# than write a working credential into state — so put the secret back into the
# configuration yourself, and the first apply after the import re-asserts it.
terraform import rauthy_auth_provider.corp aBMZzO5vPucY8OurKcHQqTK1
