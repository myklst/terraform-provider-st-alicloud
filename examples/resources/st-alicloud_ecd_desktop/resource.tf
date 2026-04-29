resource "st-alicloud_ecd_desktop" "test" {
  office_site_id  = "test"
  bundle_id       = "test"
  policy_group_id = "test"
  desktop_name    = "test-desktop"
  payment_type    = "PayAsYouGo"
  vswitch_id      = "vsw-test"

  tags = {
    Brand = "a"
    Env   = "test"
  }
}
