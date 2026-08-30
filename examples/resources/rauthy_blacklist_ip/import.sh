# Blacklist entries are imported by the address itself. Use the spelling Rauthy
# shows in GET /auth/v1/blacklist: it prints addresses canonically, and an
# import that differs only in spelling produces a one-time diff on `id`.
terraform import rauthy_blacklist_ip.abuse 203.0.113.7
