resource "st-alicloud_ims_user_sso_settings" "foo" {
  sso_enabled           = true
  metadata_document     = "PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0iVVRGLTgiPz4KPEVudGl0eURxxxxxxxxx"
  sso_login_with_domain = true
  auxiliary_domain      = "xxx.com"
}
