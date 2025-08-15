resource "st-alicloud_ims_user_sso_settings" "example" {
  sso_enabled           = true
  metadata_document     = "PD94bWwgdmVyc2lvbj0iMS4wIiBlbxxxxxxxxxxxx"
  sso_login_with_domain = true
  auxiliary_domain      = "example.com"
}
