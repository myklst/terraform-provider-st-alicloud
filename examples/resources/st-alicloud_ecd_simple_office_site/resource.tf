resource "st-alicloud_ecd_simple_office_site" "test" {
  office_site_name    = "test-office-site"
  cidr_block          = "172.16.0.0/12"
  desktop_access_type = "Internet"
  enable_admin_access = true
  vpc_type            = "customized"
  vpc_id              = "vpc-test"
}
