# PAM users are imported by their numeric uid, not by username.
terraform import rauthy_pam_user.alice 100000
