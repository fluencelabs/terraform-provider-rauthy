# Providers are imported by their Rauthy-assigned id. Rauthy has no
# GET /providers/{id}, so find it in the list — which is served on POST:
#   curl -X POST -H "Authorization: API-Key $RAUTHY_API_KEY" \
#     "$RAUTHY_URL/auth/v1/providers" | jq -r '.[] | "\(.id)\t\(.name)"'
#
# The upstream client_secret does come back on a read, so an import recovers
# the whole resource, secret included.
terraform import rauthy_auth_provider.corp aBMZzO5vPucY8OurKcHQqTK1
