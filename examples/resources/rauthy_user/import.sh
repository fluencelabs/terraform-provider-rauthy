# Users are imported by their Rauthy-assigned id, not by email address. Find it
# in the Admin UI or with:
#   curl -H "Authorization: API-Key $RAUTHY_API_KEY" \
#     "$RAUTHY_URL/auth/v1/users/email/ada@example.com" | jq -r .id
terraform import rauthy_user.ada 9f9a1a1e-0d3f-4f1e-9a1b-2c3d4e5f6a7b
