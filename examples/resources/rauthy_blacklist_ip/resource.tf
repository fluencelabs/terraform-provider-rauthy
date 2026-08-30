# Block a known-bad address until a fixed point in time. The timestamp has to be
# a literal rather than something derived from the current time: a value that
# moves on every plan would show a permanent diff, and one that is already in the
# past is rejected at apply.
resource "rauthy_blacklist_ip" "abuse" {
  ip  = "203.0.113.7"
  exp = 4102444800 # 2100-01-01T00:00:00Z
}

resource "rauthy_blacklist_ip" "abuse_v6" {
  ip  = "2001:db8::2"
  exp = 4102444800
}
