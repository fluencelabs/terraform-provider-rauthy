# Clients are imported by their client id. The secret is read back from Rauthy,
# but secret_rotation_trigger and secret_cache_current_hours exist only in
# Terraform and stay unset after an import.
terraform import rauthy_client.backend example-backend
